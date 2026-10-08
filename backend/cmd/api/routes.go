package main

import (
	"net/http"

	"github.com/CAPELLAX02/agora/backend/internal/org"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

func (app *application) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", app.healthz)
	mux.HandleFunc("GET /readyz", app.readyz)

	org.NewHandler(org.NewRepository(app.db), app.logger).Register(mux)

	return httpx.Chain(
		httpx.WithProblemFallback(mux),
		httpx.RequestID,
		httpx.AccessLog(app.logger),
		httpx.Recover(app.logger),
	)
}
