package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/jwt"
	"github.com/CAPELLAX02/agora/backend/internal/platform/metrics"
)

func TestHealthz(t *testing.T) {
	app := &application{
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		version:   "test",
		startedAt: time.Now().Add(-90 * time.Second),
	}

	rec := httptest.NewRecorder()
	app.healthz(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("durum = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json; charset=utf-8")
	}

	var got healthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("gövde çözülemedi: %v\n%s", err, rec.Body.String())
	}

	want := healthResponse{Status: "ok", Version: "test", Uptime: "1m30s"}
	if got != want {
		t.Errorf("healthz = %+v, want %+v", got, want)
	}
}

// TestRoutes, uygulamanın gerçek route tablosunu ve middleware zincirini birlikte doğrular.
// Veritabanına inmeyen yollar test edilir: istek handler'a ulaşmadan reddedilenler.
func TestRoutes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	app := &application{
		logger:  logger,
		metrics: metrics.NewHTTP(prometheus.NewRegistry()),
		// Test edilen yollar token doğrulamasına ve iptal listesine hiç ulaşmaz.
		authenticator:  authn.New(jwt.NewVerifier("agora", "agora-api", nil, 0), nil, logger, time.Now),
		loginRateLimit: passThrough,
		version:        "test",
		startedAt:      time.Now(),
	}

	srv := httptest.NewServer(app.routes())
	defer srv.Close()

	tests := []struct {
		method     string
		path       string
		wantStatus int
	}{
		{http.MethodGet, "/healthz", http.StatusOK},
		{http.MethodGet, "/readyz", http.StatusOK},
		{http.MethodPost, "/healthz", http.StatusMethodNotAllowed},
		{http.MethodGet, "/olmayan-yol", http.StatusNotFound},
		{http.MethodPost, "/api/v1/auth/login", http.StatusBadRequest}, // X-Agora-Client yok
		{http.MethodGet, "/api/v1/auth/login", http.StatusMethodNotAllowed},
		{http.MethodGet, "/api/v1/me", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, srv.URL+tt.path, nil)
			if err != nil {
				t.Fatal(err)
			}

			resp, err := srv.Client().Do(req)
			if err != nil {
				t.Fatalf("istek başarısız: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("durum = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
			if resp.Header.Get("X-Request-Id") == "" {
				t.Error("yanıtta X-Request-Id başlığı yok")
			}
			if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
				t.Error("yanıtta güvenlik başlıkları yok")
			}
		})
	}
}

// passThrough, isteği olduğu gibi geçiren middleware'dir. Testlerde Redis'e bağlı
// middleware'lerin yerine kullanılır.
func passThrough(next http.Handler) http.Handler { return next }

// fakePinger, testlerde gerçek bir veritabanı yerine kullanılan sahte bağımlılıktır.
type fakePinger struct {
	err error
}

func (f fakePinger) Ping(ctx context.Context) error {
	return f.err
}

func TestReadyz(t *testing.T) {
	tests := []struct {
		name       string
		checks     map[string]pinger
		wantStatus int
		want       readinessResponse
	}{
		{
			name:       "tüm bağımlılıklar hazır",
			checks:     map[string]pinger{"postgres": fakePinger{}},
			wantStatus: http.StatusOK,
			want:       readinessResponse{Status: "ready", Checks: map[string]string{"postgres": "ok"}},
		},
		{
			name: "bir bağımlılık yanıt vermiyor",
			checks: map[string]pinger{
				"postgres": fakePinger{err: errors.New("bağlantı reddedildi")},
				"redis":    fakePinger{},
			},
			wantStatus: http.StatusServiceUnavailable,
			want: readinessResponse{
				Status: "not_ready",
				Checks: map[string]string{"postgres": "unavailable", "redis": "ok"},
			},
		},
		{
			name: "pingerFunc ile uyarlanmış bağımlılık",
			checks: map[string]pinger{
				"postgres": fakePinger{},
				"redis":    pingerFunc(func(ctx context.Context) error { return errors.New("bağlantı yok") }),
			},
			wantStatus: http.StatusServiceUnavailable,
			want: readinessResponse{
				Status: "not_ready",
				Checks: map[string]string{"postgres": "ok", "redis": "unavailable"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &application{
				logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
				checks: tt.checks,
			}

			rec := httptest.NewRecorder()
			app.readyz(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

			if rec.Code != tt.wantStatus {
				t.Errorf("durum = %d, want %d", rec.Code, tt.wantStatus)
			}

			var got readinessResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("gövde çözülemedi: %v\n%s", err, rec.Body.String())
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("readyz = %+v, want %+v", got, tt.want)
			}
			if strings.Contains(rec.Body.String(), "reddedildi") {
				t.Error("hata ayrıntısı istemciye sızdı")
			}
		})
	}
}
