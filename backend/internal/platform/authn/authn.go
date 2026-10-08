// Package authn, access token ile kimlik doğrulamayı (authentication) sağlar:
// isteği yapanın kim olduğunu belirler. Ne yapabileceği (authorization) ayrı bir konudur.
package authn

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
	"github.com/CAPELLAX02/agora/backend/internal/platform/jwt"
)

// Principal, kimliği doğrulanmış istek sahibidir.
type Principal struct {
	UserID    string
	SessionID string
	TokenID   string
	AMR       []string
}

type ctxKey struct{}

// WithPrincipal, p'yi taşıyan yeni bir context döndürür.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

// PrincipalFrom, context'teki kimliği döndürür. İstek Require'dan geçmediyse ok false olur.
func PrincipalFrom(ctx context.Context) (p Principal, ok bool) {
	p, ok = ctx.Value(ctxKey{}).(Principal)
	return p, ok
}

// TokenVerifier, access token doğrulayan bileşendir. *jwt.Verifier bunu sağlar.
type TokenVerifier interface {
	Verify(token string, now time.Time) (jwt.Claims, error)
}

// Authenticator, istekteki Bearer token'ı doğrular.
type Authenticator struct {
	verifier TokenVerifier
	now      func() time.Time
}

// New, bir Authenticator oluşturur.
func New(verifier TokenVerifier, now func() time.Time) *Authenticator {
	return &Authenticator{verifier: verifier, now: now}
}

// Require, geçerli bir access token taşımayan istekleri 401 ile reddeder. Geçerliyse
// kimliği context'e koyar ve isteği sonraki handler'a iletir.
func (a *Authenticator) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			unauthorized(w, r, "", "AUTHENTICATION_REQUIRED", "Bu işlem için giriş yapmalısınız.")
			return
		}

		claims, err := a.verifier.Verify(token, a.now())
		if err != nil {
			unauthorized(w, r, "invalid_token", "INVALID_TOKEN", "Access token geçersiz ya da süresi dolmuş.")
			return
		}

		ctx := WithPrincipal(r.Context(), Principal{
			UserID:    claims.Subject,
			SessionID: claims.SessionID,
			TokenID:   claims.ID,
			AMR:       claims.AMR,
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// bearerToken, "Authorization: Bearer <token>" başlığından token'ı çıkarır.
// Şema adı büyük/küçük harf duyarsızdır (RFC 9110 §11.1).
func bearerToken(r *http.Request) (string, bool) {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	return token, token != ""
}

// unauthorized, 401 yanıtını RFC 6750'nin istediği WWW-Authenticate başlığıyla yazar.
func unauthorized(w http.ResponseWriter, r *http.Request, bearerErr, code, detail string) {
	challenge := `Bearer realm="agora"`
	if bearerErr != "" {
		challenge += `, error="` + bearerErr + `"`
	}
	w.Header().Set("WWW-Authenticate", challenge)
	_ = httpx.WriteProblem(w, r, httpx.Problem{Status: http.StatusUnauthorized, Code: code, Detail: detail})
}
