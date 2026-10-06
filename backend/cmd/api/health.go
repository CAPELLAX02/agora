package main

import (
	"log"
	"net/http"
	"time"

	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

type healthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	Uptime  string `json:"uptime"`
}

func (app *application) healthz(w http.ResponseWriter, r *http.Request) {
	res := healthResponse{
		Status:  "ok",
		Version: app.version,
		Uptime:  time.Since(app.startedAt).Round(time.Second).String(),
	}

	if err := httpx.WriteJSON(w, http.StatusOK, res); err != nil {
		log.Printf("Healthz: %v", err)
	}
}
