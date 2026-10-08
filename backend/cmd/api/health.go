package main

import (
	"context"
	"net/http"
	"time"

	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

// pinger, hazır olma kontrolünde yoklanabilen bir bağımlılıktır.
// *pgxpool.Pool bu interface'i örtük olarak sağlar.
type pinger interface {
	Ping(ctx context.Context) error
}

// pingerFunc, sıradan bir fonksiyonu pinger'a dönüştürür. http.HandlerFunc ile aynı
// desen: Ping metodu farklı imzalı bağımlılıklar (ör. Redis) bununla uyarlanır.
type pingerFunc func(ctx context.Context) error

// Ping, f'yi çağırır.
func (f pingerFunc) Ping(ctx context.Context) error {
	return f(ctx)
}

type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	Uptime  string `json:"uptime"`
}

type readinessResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

// healthz, canlılık (liveness) kontrolüdür: süreç ayakta mı?
// Bağımlılıklara bakmaz, aksi halde DB kesintisi tüm instance'ları yeniden başlatırdı.
func (app *application) healthz(w http.ResponseWriter, r *http.Request) {
	res := healthResponse{
		Status:  "ok",
		Version: app.version,
		Uptime:  time.Since(app.startedAt).Round(time.Second).String(),
	}

	if err := httpx.WriteJSON(w, http.StatusOK, res); err != nil {
		app.logger.Error("healthz yanıtı yazılamadı", "err", err)
	}
}

// readyz, hazır olma (readiness) kontrolüdür: bu instance trafik almaya hazır mı?
// Bağımlılıklardan biri yanıt vermezse 503 döner, yük dengeleyici trafiği başka yere yönlendirir.
func (app *application) readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	res := readinessResponse{
		Status: "ready",
		Checks: make(map[string]string, len(app.checks)),
	}
	status := http.StatusOK

	for name, p := range app.checks {
		if err := p.Ping(ctx); err != nil {
			app.logger.Warn("hazır olma kontrolü başarısız", "check", name, "err", err)
			res.Checks[name] = "unavailable"
			res.Status = "not_ready"
			status = http.StatusServiceUnavailable
			continue
		}
		res.Checks[name] = "ok"
	}

	if err := httpx.WriteJSON(w, status, res); err != nil {
		app.logger.Error("readyz yanıtı yazılamadı", "err", err)
	}
}
