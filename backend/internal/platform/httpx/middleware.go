package httpx

import (
	"context"
	"crypto/rand"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

// Middleware, bir handler'ı sarıp yeni bir handler döndüren fonksiyondur.
type Middleware func(http.Handler) http.Handler

// Chain, middleware'ları verilen sırayla uygular: Chain(h, a, b, c) == a(b(c(h))).
// İlk middleware en dışta olur, istek önce ona girer.
func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// RequestIDHeader, istek kimliğinin taşındığı HTTP başlığıdır.
const RequestIDHeader = "X-Request-Id"

type ctxKey int

const requestIDKey ctxKey = 0

// RequestID, her isteğe bir kimlik atar. Gelen geçerli bir X-Request-Id başlığı varsa
// (örn. ters vekilden yani reverse proxy'den) onu korur, yoksa yenisini üretir.
// Kimlik yanıt başlığına da yazılır.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get(RequestIDHeader)
			if !validRequestID(id) {
				id = rand.Text()
			}
			w.Header().Set(RequestIDHeader, id)
			ctx := context.WithValue(r.Context(), requestIDKey, id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
}

// RequestIDFrom, context'teki istek kimliğini döndürür. Yoksa boş string döner.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// validRequestID, dışarıdan gelen kimliğin log'a güvenle yazılabilir olduğunu kontrol eder.
func validRequestID(id string) bool {
	if len(id) == 0 || len(id) > 64 {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		isAlnum := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
		if !isAlnum && c != '-' && c != '_' {
			return false
		}
	}
	return true
}

// Recover, handler'larda oluşan panic'leri yakalar, stack trace ile loglar ve 500 döner.
func Recover(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				if rec == http.ErrAbortHandler {
					panic(rec) // net/http'nin kasıtlı iptal sinyali, olduğu gibi bırak
				}

				logger.Error("panic yakalandı",
					"panic", rec,
					"request_id", RequestIDFrom(r.Context()),
					"stack", string(debug.Stack()))

				w.Header().Set("Connection", "close")
				InternalServerError(w, r)
			}()

			next.ServeHTTP(w, r)
		})
	}
}

// AccessLog, her isteği metot, yol, durum, süre ve istek kimliğiyle loglar.
// 5xx yanıtlar ERROR, 4xx yanıtlar WARN, diğerleri INFO seviyesinde yazılır.
func AccessLog(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				start := time.Now()
				rec := &statusRecorder{ResponseWriter: w}

				next.ServeHTTP(rec, r)

				if rec.status == 0 {
					rec.status = http.StatusOK // handler hiçbir şey yazmadıysa 200 döner
				}

				level := slog.LevelInfo
				switch {
				case rec.status >= 500:
					level = slog.LevelError
				case rec.status >= 400:
					level = slog.LevelWarn
				}

				logger.Log(
					r.Context(), level, "http isteği",
					"method", r.Method,
					"path", r.URL.Path,
					"status", rec.status,
					"bytes", rec.bytes,
					"duration_ms", float64(time.Since(start).Microseconds())/1000,
					"request_id", RequestIDFrom(r.Context()),
					"remote_addr", r.RemoteAddr,
				)
			})
	}
}

// statusRecorder, yazılan durum kodunu ve byte sayısını yakalamak için ResponseWriter'ı sarar.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (sr *statusRecorder) WriteHeader(code int) {
	if sr.status == 0 {
		sr.status = code
	}
	sr.ResponseWriter.WriteHeader(code)
}

func (sr *statusRecorder) Write(b []byte) (int, error) {
	if sr.status == 0 {
		sr.status = http.StatusOK
	}
	n, err := sr.ResponseWriter.Write(b)
	sr.bytes += n
	return n, err
}

// Unwrap, http.ResponseController'ın alttaki writer'a (Flusher vb.) ulaşmasını sağlar.
// İleride SSE yazarken buna ihtiyaç olacak.
func (sr *statusRecorder) Unwrap() http.ResponseWriter {
	return sr.ResponseWriter
}
