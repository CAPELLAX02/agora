package httpx

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
)

// SecurityHeaders, her yanıta tarayıcıya yönelik güvenlik başlıklarını ekler.
// API sadece JSON döndürdüğü için politika en sıkı haliyle verilir: yanıt bir sayfa
// gibi yorumlanamaz, çerçeveye alınamaz, başka sitelere yönlendiren bağlantılara
// adres sızdırmaz. hsts, yanıtın sadece HTTPS üzerinden sunulduğu ortamlarda true olmalı.
func SecurityHeaders(hsts bool) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "no-referrer")
			h.Set("Cross-Origin-Resource-Policy", "same-origin")
			if hsts {
				h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CORS ayarları. İzin verilen başlıklar, web istemcisinin gönderdiği özel başlıklardır.
var (
	corsAllowedMethods = "GET, POST, PUT, PATCH, DELETE"
	corsAllowedHeaders = "Authorization, Content-Type, If-Match, Idempotency-Key, X-Agora-Client, X-Request-Id"
	corsExposedHeaders = "Retry-After, X-Request-Id, ETag, Location"
	corsMaxAge         = strconv.Itoa(600) // preflight sonucu 10 dk önbellekte tutulur
)

// CORS, sadece listelenen origin'lerin tarayıcıdan API'ye istek göndermesine izin
// verir. Liste boşsa hiçbir başka origin'e izin verilmez: production'da web arayüzü
// API ile aynı origin'den sunulur ve CORS'a hiç gerek kalmaz.
//
// İzin verilen origin'lere kimlik bilgisi (çerez) gönderme izni de verilir, bu yüzden
// "*" joker karakteri kabul edilmez: her origin tam adıyla yazılmalıdır.
func CORS(allowedOrigins []string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}

			h := w.Header()
			// Yanıt Origin'e göre değiştiği için ara önbellekler onu origin bazında saklamalı.
			h.Add("Vary", "Origin")

			allowed := slices.Contains(allowedOrigins, origin)
			preflight := r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != ""

			if preflight {
				h.Add("Vary", "Access-Control-Request-Method")
				h.Add("Vary", "Access-Control-Request-Headers")
				if !allowed {
					_ = WriteProblem(w, r, Problem{
						Status: http.StatusForbidden,
						Code:   "CORS_ORIGIN_NOT_ALLOWED",
						Detail: "Bu origin'den gelen isteklere izin verilmiyor.",
					})
					return
				}
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Access-Control-Allow-Credentials", "true")
				h.Set("Access-Control-Allow-Methods", corsAllowedMethods)
				h.Set("Access-Control-Allow-Headers", corsAllowedHeaders)
				h.Set("Access-Control-Max-Age", corsMaxAge)
				w.WriteHeader(http.StatusNoContent)
				return
			}

			if allowed {
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Access-Control-Allow-Credentials", "true")
				h.Set("Access-Control-Expose-Headers", corsExposedHeaders)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ValidOrigin, s'nin "scheme://host[:port]" biçiminde, yolu ve sondaki eğik çizgisi
// olmayan bir origin olup olmadığını söyler. Tarayıcının gönderdiği Origin başlığı
// bu biçimdedir ve karşılaştırma tam eşleşmeyle yapılır.
func ValidOrigin(s string) bool {
	scheme, rest, ok := strings.Cut(s, "://")
	if !ok || (scheme != "http" && scheme != "https") || rest == "" {
		return false
	}
	return !strings.ContainsAny(rest, "/?#*")
}
