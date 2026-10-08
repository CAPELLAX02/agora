package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityHeaders(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	for _, hsts := range []bool{false, true} {
		rec := httptest.NewRecorder()
		SecurityHeaders(hsts)(ok).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

		want := map[string]string{
			"Content-Security-Policy":      "default-src 'none'; frame-ancestors 'none'",
			"X-Content-Type-Options":       "nosniff",
			"X-Frame-Options":              "DENY",
			"Referrer-Policy":              "no-referrer",
			"Cross-Origin-Resource-Policy": "same-origin",
		}
		for k, v := range want {
			if got := rec.Header().Get(k); got != v {
				t.Errorf("hsts=%v: %s = %q, want %q", hsts, k, got, v)
			}
		}
		if got := rec.Header().Get("Strict-Transport-Security"); (got != "") != hsts {
			t.Errorf("hsts=%v: Strict-Transport-Security = %q", hsts, got)
		}
	}
}

func TestCORS(t *testing.T) {
	var reached bool
	h := CORS([]string{"http://localhost:5173"})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
	}))

	do := func(method, origin, requestMethod string) *httptest.ResponseRecorder {
		reached = false
		req := httptest.NewRequest(method, "/api/v1/auth/login", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if requestMethod != "" {
			req.Header.Set("Access-Control-Request-Method", requestMethod)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	t.Run("izinli origin preflight", func(t *testing.T) {
		rec := do(http.MethodOptions, "http://localhost:5173", "POST")
		if rec.Code != http.StatusNoContent || reached {
			t.Fatalf("durum = %d, handler'a ulaşıldı = %v", rec.Code, reached)
		}
		for k, v := range map[string]string{
			"Access-Control-Allow-Origin":      "http://localhost:5173",
			"Access-Control-Allow-Credentials": "true",
			"Access-Control-Allow-Methods":     corsAllowedMethods,
			"Access-Control-Allow-Headers":     corsAllowedHeaders,
			"Access-Control-Max-Age":           "600",
		} {
			if got := rec.Header().Get(k); got != v {
				t.Errorf("%s = %q, want %q", k, got, v)
			}
		}
	})

	t.Run("izinsiz origin preflight", func(t *testing.T) {
		rec := do(http.MethodOptions, "https://kotu.example", "POST")
		if rec.Code != http.StatusForbidden || reached {
			t.Fatalf("durum = %d", rec.Code)
		}
		if rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Error("izinsiz origin'e izin başlığı dönmemeli")
		}
	})

	t.Run("izinli origin gerçek istek", func(t *testing.T) {
		rec := do(http.MethodPost, "http://localhost:5173", "")
		if !reached || rec.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
			t.Errorf("izin başlığı = %q", rec.Header().Get("Access-Control-Allow-Origin"))
		}
		if rec.Header().Get("Access-Control-Expose-Headers") != corsExposedHeaders {
			t.Error("Retry-After gibi başlıklar JavaScript'e açılmalı")
		}
		if rec.Header().Get("Vary") != "Origin" {
			t.Errorf("Vary = %q", rec.Header().Get("Vary"))
		}
	})

	t.Run("izinsiz origin gerçek istek", func(t *testing.T) {
		// İstek işlenir ama izin başlığı yoktur: tarayıcı yanıtı JavaScript'e vermez.
		rec := do(http.MethodGet, "https://kotu.example", "")
		if !reached || rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("izin başlığı = %q", rec.Header().Get("Access-Control-Allow-Origin"))
		}
	})

	t.Run("Origin'siz istek (curl, mobil)", func(t *testing.T) {
		rec := do(http.MethodOptions, "", "")
		if !reached || rec.Header().Get("Vary") != "" {
			t.Error("Origin'siz istek olduğu gibi geçmeli")
		}
	})
}

func TestValidOrigin(t *testing.T) {
	for s, want := range map[string]bool{
		"http://localhost:5173":     true,
		"https://agora.example.edu": true,
		"https://agora.test/":       false,
		"https://agora.test/giris":  false,
		"agora.test":                false,
		"*":                         false,
		"https://*.agora.test":      false,
		"ftp://agora.test":          false,
		"https://":                  false,
	} {
		if got := ValidOrigin(s); got != want {
			t.Errorf("ValidOrigin(%q) = %v, want %v", s, got, want)
		}
	}
}
