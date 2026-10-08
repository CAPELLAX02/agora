package main

import (
	"net/http"
	"net/http/pprof"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/CAPELLAX02/agora/backend/internal/platform/metrics"
)

// adminRoutes, sadece iç ağdan erişilmesi gereken uç noktaları sunar: Prometheus
// metrikleri ve Go profil araçları (pprof). Bunlar genel API portunda yayınlanmaz,
// çünkü sistemin iç yapısı hakkında saldırgana değerli bilgi verirler.
func adminRoutes(reg *prometheus.Registry) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /metrics", metrics.Handler(reg))

	mux.HandleFunc("GET /debug/pprof/", pprof.Index)
	mux.HandleFunc("GET /debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("GET /debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("GET /debug/pprof/trace", pprof.Trace)

	return mux
}
