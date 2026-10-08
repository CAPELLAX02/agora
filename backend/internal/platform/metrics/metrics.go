// Package metrics, uygulamanın Prometheus metriklerini tanımlar ve sunar.
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const namespace = "agora"

// NewRegistry, Go çalışma zamanı ve süreç metriklerini içeren yeni bir registry oluşturur.
//
// client_golang'in paket seviyesindeki global registry'si bilinçli olarak kullanılmaz:
// hangi metriklerin yayınlandığı burada açıkça görünür ve testler birbirini etkilemez.
func NewRegistry() *prometheus.Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return reg
}

// Handler, registry'deki metrikleri Prometheus'un okuyacağı metin biçiminde sunar.
func Handler(reg *prometheus.Registry) http.Handler {
	return promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg})
}

// HTTP, HTTP katmanının RED (Rate, Errors, Duration) metrikleridir.
// httpx.RequestObserver interface'ini örtük olarak sağlar.
type HTTP struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	inFlight prometheus.Gauge
}

// NewHTTP, HTTP metriklerini oluşturur ve registry'ye kaydeder.
func NewHTTP(reg prometheus.Registerer) *HTTP {
	m := &HTTP{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "Tamamlanan HTTP isteklerinin sayısı.",
		}, []string{"method", "route", "status"}),

		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "HTTP isteklerinin işlenme süresi (saniye).",
			Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
		}, []string{"method", "route"}),

		inFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: "http",
			Name:      "requests_in_flight",
			Help:      "Şu anda işlenmekte olan HTTP isteklerinin sayısı.",
		}),
	}

	reg.MustRegister(m.requests, m.duration, m.inFlight)
	return m
}

// RequestStarted, işlenmeye başlayan bir isteği sayar.
func (m *HTTP) RequestStarted() {
	m.inFlight.Inc()
}

// RequestFinished, tamamlanan bir isteğin sayısını ve süresini kaydeder.
func (m *HTTP) RequestFinished(method, route string, status int, duration time.Duration) {
	m.inFlight.Dec()

	method = normalizeMethod(method)
	m.requests.WithLabelValues(method, route, strconv.Itoa(status)).Inc()
	m.duration.WithLabelValues(method, route).Observe(duration.Seconds())
}

// normalizeMethod, standart dışı HTTP metotlarını tek bir etikette toplar.
// İstemci "FOO" gibi rastgele metotlar göndererek sınırsız sayıda metrik serisi
// oluşturamasın diye.
func normalizeMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodOptions:
		return method
	}
	return "OTHER"
}
