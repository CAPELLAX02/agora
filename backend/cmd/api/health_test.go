package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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
func TestRoutes(t *testing.T) {
	app := &application{
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		version:   "test",
		startedAt: time.Now(),
	}

	srv := httptest.NewServer(app.routes())
	defer srv.Close()

	tests := []struct {
		method     string
		path       string
		wantStatus int
	}{
		{http.MethodGet, "/healthz", http.StatusOK},
		{http.MethodPost, "/healthz", http.StatusMethodNotAllowed},
		{http.MethodGet, "/olmayan-yol", http.StatusNotFound},
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
		})
	}
}
