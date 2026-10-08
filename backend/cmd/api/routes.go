package main

import (
	"net/http"

	"github.com/CAPELLAX02/agora/backend/internal/iam"
	"github.com/CAPELLAX02/agora/backend/internal/org"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

func (app *application) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", app.healthz)
	mux.HandleFunc("GET /readyz", app.readyz)

	org.NewHandler(org.NewRepository(app.db), app.logger).Register(mux)

	// Refresh çerezi yerel geliştirmede (http://localhost) Secure olamaz.
	secureCookie := !app.cfg.IsDevelopment()
	iam.NewHandler(app.auth, iam.NewRepository(app.db), app.logger, secureCookie).
		Register(mux, app.authenticator.Require, app.loginRateLimit)

	return httpx.Chain(
		httpx.WithProblemFallback(mux),
		httpx.RequestID,
		httpx.AccessLog(app.logger),
		httpx.Observe(app.metrics),
		httpx.Recover(app.logger),
	)
}
