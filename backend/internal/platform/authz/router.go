package authz

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

type policyKind int

const (
	_ policyKind = iota // sıfır değer: politika verilmemiş
	public
	selfService
	authenticated
	permission
)

// Policy, bir route'a kimin erişebileceğidir.
type Policy struct {
	kind       policyKind
	permission string
}

// Erişim politikaları.
var (
	// Public, herkesin erişebildiği route'lardır (giriş, sağlık, katalog).
	Public = Policy{kind: public}

	// SelfService, kullanıcının kendi hesabıyla ilgili route'lardır (/me, parola
	// değiştirme, oturumlarım). Parolasını değiştirmesi gereken kullanıcı da erişebilir:
	// ilk girişte zorunlu parola değişikliği bu route'larla tamamlanır.
	SelfService = Policy{kind: selfService}

	// Authenticated, giriş yapmış ve hesabı aktif her kullanıcının erişebildiği route'lardır.
	Authenticated = Policy{kind: authenticated}
)

// Permission, verilen yetkiye (herhangi bir kapsamda) sahip kullanıcıların erişebildiği
// route'lardır. Kaynağa özgü kapsam ve sahiplik kontrolü handler ya da serviste yapılır.
func Permission(code string) Policy {
	return Policy{kind: permission, permission: code}
}

func (p Policy) String() string {
	switch p.kind {
	case public:
		return "public"
	case selfService:
		return "self-service"
	case authenticated:
		return "authenticated"
	case permission:
		return "permission:" + p.permission
	default:
		return "tanımsız"
	}
}

// Router, route'ları erişim politikalarıyla birlikte kaydeder. Politikasız bir route
// kaydedilemez: unutulan bir kontrol "herkese açık" değil, açılışta hata demektir
// (varsayılan ret, deny by default).
type Router struct {
	mux          *http.ServeMux
	authenticate httpx.Middleware
	resolver     Resolver
	logger       *slog.Logger
}

// NewRouter, bir Router oluşturur. authenticate, access token'ı doğrulayan middleware'dir.
func NewRouter(mux *http.ServeMux, authenticate httpx.Middleware, resolver Resolver, logger *slog.Logger) *Router {
	return &Router{mux: mux, authenticate: authenticate, resolver: resolver, logger: logger}
}

// Handle, pattern'i politikasıyla kaydeder. mws, politika kontrolünden sonra ve
// handler'dan önce çalışır. Politika geçersizse panic olur: bu bir programlama
// hatasıdır ve uygulama hiç ayağa kalkmamalıdır.
func (rt *Router) Handle(pattern string, policy Policy, h http.Handler, mws ...httpx.Middleware) {
	h = httpx.Chain(h, mws...)

	switch policy.kind {
	case public:
	case selfService, authenticated, permission:
		h = httpx.Chain(h, rt.authenticate, rt.authorize(policy))
	default:
		panic(fmt.Sprintf("authz: %q route'u için erişim politikası tanımlanmamış", pattern))
	}

	rt.mux.Handle(pattern, h)
}

// HandleFunc, Handle'ın fonksiyon alan biçimidir.
func (rt *Router) HandleFunc(pattern string, policy Policy, h http.HandlerFunc, mws ...httpx.Middleware) {
	rt.Handle(pattern, policy, h, mws...)
}

// authorize, kullanıcının güncel yetkilerini çözer ve politikayı uygular. Yetkiler
// context'e konur: handler ve servis kaynağa özgü kontrolleri bunlarla yapar.
func (rt *Router) authorize(policy Policy) httpx.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := authn.PrincipalFrom(r.Context())
			if !ok {
				panic("authz: authorize, kimlik doğrulamadan önce çalıştı")
			}

			perms, err := rt.resolver.Permissions(r.Context(), principal.UserID)
			if errors.Is(err, ErrAccountInactive) {
				problem(w, r, http.StatusForbidden, "ACCOUNT_DISABLED", "Hesap aktif değil.")
				return
			}
			if err != nil {
				rt.logger.Error("yetkiler çözülemedi",
					"err", err, "user_id", principal.UserID, "request_id", httpx.RequestIDFrom(r.Context()))
				httpx.InternalServerError(w, r)
				return
			}

			// İki adımlı doğrulama yapılmamış oturumda MFA gerektiren yetkiler yok sayılır.
			if !principal.MFA() {
				perms = perms.WithoutMFA()
			}

			if perms.PasswordChangeRequired() && policy.kind != selfService {
				problem(w, r, http.StatusForbidden, "PASSWORD_CHANGE_REQUIRED",
					"Devam etmek için parolanızı değiştirmelisiniz.")
				return
			}

			if policy.kind == permission && !perms.Has(policy.permission) {
				if perms.WithheldForMFA(policy.permission) {
					problem(w, r, http.StatusForbidden, "MFA_REQUIRED",
						"Bu işlem için iki adımlı doğrulamayla giriş yapmalısınız.")
					return
				}
				problem(w, r, http.StatusForbidden, "FORBIDDEN", "Bu işlem için yetkiniz yok.")
				return
			}

			next.ServeHTTP(w, r.WithContext(WithPermissions(r.Context(), perms)))
		})
	}
}

func problem(w http.ResponseWriter, r *http.Request, status int, code, detail string) {
	_ = httpx.WriteProblem(w, r, httpx.Problem{Status: status, Code: code, Detail: detail})
}
