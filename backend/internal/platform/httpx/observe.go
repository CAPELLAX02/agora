package httpx

import (
	"net/http"
	"strings"
	"time"
)

// RequestObserver, HTTP isteklerini ölçen bir bileşendir (ör. Prometheus metrikleri).
// httpx paketi Prometheus'u bilmez. metrics paketi bu interface'i örtük olarak sağlar.
type RequestObserver interface {
	RequestStarted()
	RequestFinished(method, route string, status int, duration time.Duration)
}

// Observe, her isteğin metodunu, route desenini, durumunu ve süresini observer'a bildirir.
func Observe(o RequestObserver) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			o.RequestStarted()

			rec := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)

			status := rec.status
			if status == 0 {
				status = http.StatusOK
			}
			o.RequestFinished(r.Method, Route(r), status, time.Since(start))
		})
	}
}

// Route, isteğin eşleştiği route desenini metot kısmı olmadan döndürür:
// "/api/v1/programs/{id}". ServeMux eşleşme sırasında r.Pattern alanını doldurur.
// Eşleşme yoksa (404, 405) "unmatched" döner.
//
// Metrik ve log etiketlerinde gerçek yol (/api/v1/programs/01a1...) yerine desen
// kullanılır. Aksi halde her farklı UUID ayrı bir zaman serisi oluşturur ve
// Prometheus'un belleği şişer (yüksek kardinalite problemi).
func Route(r *http.Request) string {
	if r.Pattern == "" {
		return "unmatched"
	}
	if _, path, ok := strings.Cut(r.Pattern, " "); ok {
		return path
	}
	return r.Pattern
}
