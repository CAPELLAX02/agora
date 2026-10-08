package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CAPELLAX02/agora/backend/internal/platform/metrics"
)

func TestAdminRoutes(t *testing.T) {
	reg := metrics.NewRegistry()
	app := &application{
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		metrics:   metrics.NewHTTP(reg),
		version:   "test",
		startedAt: time.Now(),
	}

	// API'ye bir istek at, metriğin admin tarafında görünmesini bekle.
	app.routes().ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/healthz", nil))

	admin := adminRoutes(reg)

	rec := httptest.NewRecorder()
	admin.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/metrics durum = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`agora_http_requests_total{method="GET",route="/healthz",status="200"} 1`,
		"agora_http_request_duration_seconds_bucket",
		"go_goroutines",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/metrics çıktısında %q yok", want)
		}
	}

	rec = httptest.NewRecorder()
	admin.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "goroutine") {
		t.Errorf("pprof dizini açılmadı: durum %d", rec.Code)
	}

	// Admin uç noktaları genel API'de olmamalı.
	rec = httptest.NewRecorder()
	app.routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("genel API'de /metrics durum = %d, want 404", rec.Code)
	}
}
