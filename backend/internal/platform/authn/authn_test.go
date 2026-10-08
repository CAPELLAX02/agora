package authn

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/CAPELLAX02/agora/backend/internal/platform/jwt"
)

// fakeVerifier, sadece "iyi" token'ını kabul eden sahte doğrulayıcıdır.
type fakeVerifier struct {
	gotNow time.Time
}

func (f *fakeVerifier) Verify(token string, now time.Time) (jwt.Claims, error) {
	f.gotNow = now
	if token != "iyi" {
		return jwt.Claims{}, jwt.ErrInvalidSignature
	}
	return jwt.Claims{Subject: "u1", SessionID: "s1", ID: "j1", AMR: []string{"pwd"}}, nil
}

func TestRequire(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	verifier := &fakeVerifier{}
	a := New(verifier, func() time.Time { return now })

	var got Principal
	var reached bool
	h := a.Require(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		got, _ = PrincipalFrom(r.Context())
	}))

	tests := []struct {
		name          string
		authorization string
		wantReached   bool
		wantChallenge string
	}{
		{"geçerli", "Bearer iyi", true, ""},
		{"şema büyük/küçük harf duyarsız", "bEaReR iyi", true, ""},
		{"başlık yok", "", false, `Bearer realm="agora"`},
		{"sadece şema", "Bearer", false, `Bearer realm="agora"`},
		{"başka şema", "Basic iyi", false, `Bearer realm="agora"`},
		{"geçersiz token", "Bearer kotu", false, `Bearer realm="agora", error="invalid_token"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reached, got = false, Principal{}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
			if tt.authorization != "" {
				req.Header.Set("Authorization", tt.authorization)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if reached != tt.wantReached {
				t.Fatalf("handler'a ulaşıldı = %v, want %v", reached, tt.wantReached)
			}
			if !tt.wantReached {
				if rec.Code != http.StatusUnauthorized {
					t.Errorf("durum = %d, want 401", rec.Code)
				}
				if c := rec.Header().Get("WWW-Authenticate"); c != tt.wantChallenge {
					t.Errorf("WWW-Authenticate = %q, want %q", c, tt.wantChallenge)
				}
				return
			}

			want := Principal{UserID: "u1", SessionID: "s1", TokenID: "j1", AMR: []string{"pwd"}}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("principal = %+v, want %+v", got, want)
			}
			if !verifier.gotNow.Equal(now) {
				t.Errorf("doğrulama zamanı = %v, verilen saat kullanılmalı", verifier.gotNow)
			}
		})
	}
}

func TestPrincipalFromEmptyContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if _, ok := PrincipalFrom(req.Context()); ok {
		t.Error("boş context'te principal olmamalı")
	}
}
