package main

import (
	"net/http"
	"time"

	"github.com/CAPELLAX02/agora/backend/internal/academic"
	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/curriculum"
	"github.com/CAPELLAX02/agora/backend/internal/enrollment"
	"github.com/CAPELLAX02/agora/backend/internal/iam"
	"github.com/CAPELLAX02/agora/backend/internal/offering"
	"github.com/CAPELLAX02/agora/backend/internal/org"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

func (app *application) routes() http.Handler {
	mux := http.NewServeMux()

	// Bütün route'lar router üzerinden, erişim politikasıyla kaydedilir.
	rt := authz.NewRouter(mux, app.authenticator.Require, app.permissions, app.logger)

	rt.HandleFunc("GET /healthz", authz.Public, app.healthz)
	rt.HandleFunc("GET /readyz", authz.Public, app.readyz)

	org.NewHandler(org.NewRepository(app.db), app.logger).Register(rt)
	org.NewRoomsHandler(org.NewRepository(app.db), app.logger).Register(rt)

	// Refresh çerezi yerel geliştirmede (http://localhost) Secure olamaz.
	secureCookie := !app.cfg.IsDevelopment()
	iam.NewHandler(app.auth, iam.NewRepository(app.db), app.logger, secureCookie).
		Register(rt, app.limits)
	iam.NewUsersHandler(app.auth, iam.NewRepository(app.db), app.logger).Register(rt)

	audit.NewHandler(audit.NewRepository(app.db), app.logger).Register(rt)
	enrollment.NewHandler(enrollment.NewRepository(app.db), enrollment.NewService(app.db), app.logger).Register(rt)
	academic.NewHandler(academic.NewRepository(app.db), org.NewTargets(app.db), app.logger, time.Now).Register(rt)
	curriculum.NewHandler(curriculum.NewRepository(app.db), org.NewTargets(app.db), app.logger).Register(rt)
	offering.NewHandler(offering.NewRepository(app.db), org.NewTargets(app.db), app.logger).Register(rt)

	return httpx.Chain(
		httpx.WithProblemFallback(mux),
		httpx.RequestID,
		httpx.WithClientInfo,
		httpx.AccessLog(app.logger),
		httpx.Observe(app.metrics),
		httpx.Recover(app.logger),
		// Güvenlik başlıkları hata yanıtlarına da eklensin diye zincirin başında.
		// HSTS sadece HTTPS arkasında anlamlı: yerel geliştirmede gönderilmez.
		httpx.SecurityHeaders(!app.cfg.IsDevelopment()),
		// Preflight (OPTIONS) istekleri route tablosuna hiç ulaşmadan yanıtlanır.
		httpx.CORS(app.cfg.CORSOrigins),
	)
}
