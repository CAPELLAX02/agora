package iam

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/CAPELLAX02/agora/backend/internal/iam/password"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
	"github.com/CAPELLAX02/agora/backend/internal/platform/useragent"
)

const (
	// ClientHeader, istemcinin türünü (web veya mobile) bildirdiği başlıktır.
	ClientHeader = "X-Agora-Client"

	// RefreshCookieName, web istemcisinde refresh token'ı taşıyan çerezdir. Path
	// sayesinde tarayıcı onu sadece /api/v1/auth altındaki isteklere ekler.
	RefreshCookieName = "agora_refresh"
	refreshCookiePath = "/api/v1/auth"

	maxUsernameLength = 64
)

// AuthService, handler'ın ihtiyaç duyduğu kimlik doğrulama işlemleridir. *Auth bunu sağlar.
type AuthService interface {
	Login(ctx context.Context, in LoginInput) (Tokens, error)
	Refresh(ctx context.Context, rawToken string) (Tokens, error)
	Logout(ctx context.Context, rawToken string) error
	ChangePassword(ctx context.Context, in ChangePasswordInput) error
	RequestPasswordReset(ctx context.Context, identifier string) error
	VerifyResetToken(ctx context.Context, rawToken string) (ResetTokenInfo, error)
	ResetPassword(ctx context.Context, rawToken, newPassword string) error
	Sessions(ctx context.Context, userID string) ([]SessionInfo, error)
	EndSession(ctx context.Context, userID, sessionID string) error
	EndOtherSessions(ctx context.Context, userID, currentSessionID string) (int, error)
}

// Limits, kimliği doğrulanmamış uçların hız sınırlarıdır.
type Limits struct {
	Login         httpx.Middleware // giriş denemeleri
	PasswordReset httpx.Middleware // sıfırlama isteği ve bağlantı kullanımı
}

// ProfileStore, profil okuma işlemidir. *Repository bunu sağlar.
type ProfileStore interface {
	Profile(ctx context.Context, userID string, at time.Time) (Profile, error)
}

// Handler, kimlik doğrulama ve profil uç noktalarını sunar.
type Handler struct {
	auth         AuthService
	profiles     ProfileStore
	logger       *slog.Logger
	secureCookie bool
}

// NewHandler, bir Handler oluşturur. secureCookie, refresh çerezinin sadece HTTPS
// üzerinden gönderilmesini sağlar ve yerel geliştirme dışında her zaman true olmalı.
func NewHandler(auth AuthService, profiles ProfileStore, logger *slog.Logger, secureCookie bool) *Handler {
	return &Handler{auth: auth, profiles: profiles, logger: logger, secureCookie: secureCookie}
}

// Register, route'ları erişim politikalarıyla kaydeder.
func (h *Handler) Register(rt *authz.Router, limits Limits) {
	// Hız sınırı handler'dan önce çalışır: reddedilen istek gövde okuma ve argon2id
	// (64 MiB, ~50 ms) maliyetine hiç girmez.
	rt.HandleFunc("POST /api/v1/auth/login", authz.Public, h.login, limits.Login)
	rt.HandleFunc("POST /api/v1/auth/refresh", authz.Public, h.refresh)
	rt.HandleFunc("POST /api/v1/auth/logout", authz.Public, h.logout)

	rt.HandleFunc("POST /api/v1/auth/password/forgot", authz.Public, h.forgotPassword, limits.PasswordReset)
	rt.HandleFunc("POST /api/v1/auth/password/reset/verify", authz.Public, h.verifyResetToken, limits.PasswordReset)
	rt.HandleFunc("POST /api/v1/auth/password/reset", authz.Public, h.resetPassword, limits.PasswordReset)

	// Kendi hesabıyla ilgili uçlar: parolasını değiştirmesi gereken kullanıcı da erişir.
	rt.HandleFunc("GET /api/v1/me", authz.SelfService, h.me)
	rt.HandleFunc("GET /api/v1/me/permissions", authz.SelfService, h.myPermissions)
	rt.HandleFunc("POST /api/v1/me/password", authz.SelfService, h.changePassword)
	rt.HandleFunc("GET /api/v1/me/sessions", authz.SelfService, h.listSessions)
	rt.HandleFunc("DELETE /api/v1/me/sessions/{id}", authz.SelfService, h.endSession)
	rt.HandleFunc("DELETE /api/v1/me/sessions", authz.SelfService, h.endOtherSessions)

	rt.HandleFunc("GET /api/v1/users/{id}", authz.Permission("user:read"), h.getUser)
}

// --- İstek ve yanıt tipleri --------------------------------------------------

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// refreshRequest, mobil istemcinin refresh ve logout gövdesidir. Web istemcisi
// token'ı gövdede değil çerezde gönderir.
type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type forgotPasswordRequest struct {
	Identifier string `json:"identifier"` // öğrenci/personel numarası ya da e-posta
}

type resetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

type verifyResetTokenRequest struct {
	Token string `json:"token"`
}

type resetTokenResponse struct {
	Purpose   string    `json:"purpose"`
	ExpiresAt time.Time `json:"expires_at"`
}

type sessionResponse struct {
	ID         string    `json:"id"`
	Client     string    `json:"client"`
	Browser    *string   `json:"browser"`
	OS         *string   `json:"os"`
	Device     string    `json:"device"`
	IP         *string   `json:"ip"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	Current    bool      `json:"current"` // isteği yapan oturum
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type tokenResponse struct {
	AccessToken        string `json:"access_token"`
	TokenType          string `json:"token_type"`
	ExpiresIn          int    `json:"expires_in"`                   // saniye
	RefreshToken       string `json:"refresh_token,omitempty"`      // sadece mobil
	RefreshExpiresIn   int    `json:"refresh_expires_in,omitempty"` // sadece mobil
	SessionID          string `json:"session_id"`
	MustChangePassword bool   `json:"must_change_password"`
}

type roleResponse struct {
	Role       string     `json:"role"`
	RoleName   string     `json:"role_name"`
	ScopeType  string     `json:"scope_type"`
	ScopeID    *string    `json:"scope_id"`
	ScopeName  *string    `json:"scope_name"`
	ValidUntil *time.Time `json:"valid_until"`
}

type profileResponse struct {
	ID                 string         `json:"id"`
	Username           string         `json:"username"`
	Email              string         `json:"email"`
	FirstName          string         `json:"first_name"`
	LastName           string         `json:"last_name"`
	Status             string         `json:"status"`
	MustChangePassword bool           `json:"must_change_password"`
	LastLoginAt        *time.Time     `json:"last_login_at"`
	Roles              []roleResponse `json:"roles"`
}

// meResponse, profili oturum bilgisiyle genişletir. Gömülü (embedded) struct'ın
// alanları JSON'da düz olarak, aynı seviyede yazılır.
type meResponse struct {
	profileResponse
	SessionID string `json:"session_id"`
}

type grantResponse struct {
	Permission string  `json:"permission"`
	ScopeType  string  `json:"scope_type"`
	ScopeID    *string `json:"scope_id"`
}

func toProfileResponse(p Profile) profileResponse {
	res := profileResponse{
		ID:                 p.UserID,
		Username:           p.Username,
		Email:              p.Email,
		FirstName:          p.FirstName,
		LastName:           p.LastName,
		Status:             string(p.Status),
		MustChangePassword: p.MustChangePassword,
		LastLoginAt:        p.LastLoginAt,
		Roles:              make([]roleResponse, 0, len(p.Roles)),
	}
	for _, ra := range p.Roles {
		res.Roles = append(res.Roles, roleResponse{
			Role:       ra.Role,
			RoleName:   ra.RoleName,
			ScopeType:  string(ra.ScopeType),
			ScopeID:    optional(ra.ScopeID),
			ScopeName:  optional(ra.ScopeName),
			ValidUntil: ra.ValidUntil,
		})
	}
	return res
}

// --- Handler'lar ---------------------------------------------------------------

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	client, ok := h.clientType(w, r)
	if !ok {
		return
	}

	var req loginRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	var errs []httpx.FieldError
	switch {
	case req.Username == "":
		errs = append(errs, httpx.FieldError{Field: "username", Message: "Kullanıcı adı zorunlu."})
	case len(req.Username) > maxUsernameLength || strings.ContainsFunc(req.Username, unicode.IsControl):
		errs = append(errs, httpx.FieldError{Field: "username", Message: "Kullanıcı adı geçersiz."})
	}
	if req.Password == "" {
		errs = append(errs, httpx.FieldError{Field: "password", Message: "Parola zorunlu."})
	}
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}

	tokens, err := h.auth.Login(r.Context(), LoginInput{
		Username:  req.Username,
		Password:  req.Password,
		Client:    client,
		IP:        httpx.ClientInfoFrom(r.Context()).IP,
		UserAgent: httpx.ClientInfoFrom(r.Context()).UserAgent,
	})
	if err != nil {
		h.authError(w, r, client, err)
		return
	}
	h.writeTokens(w, r, client, tokens)
}

func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	client, ok := h.clientType(w, r)
	if !ok {
		return
	}
	raw, ok := h.readRefreshToken(w, r, client)
	if !ok {
		return
	}

	tokens, err := h.auth.Refresh(r.Context(), raw)
	if err != nil {
		h.authError(w, r, client, err)
		return
	}
	h.writeTokens(w, r, client, tokens)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	client, ok := h.clientType(w, r)
	if !ok {
		return
	}
	raw, ok := h.readRefreshToken(w, r, client)
	if !ok {
		return
	}

	if err := h.auth.Logout(r.Context(), raw); err != nil {
		h.serverError(w, r, err)
		return
	}
	if client == ClientWeb {
		h.clearRefreshCookie(w)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	principal, ok := authn.PrincipalFrom(r.Context())
	if !ok {
		// Route, Require olmadan kaydedilmiş demektir: istemcinin değil bizim hatamız.
		h.serverError(w, r, errors.New("iam: /me kimlik doğrulama middleware'i olmadan çağrıldı"))
		return
	}

	p, err := h.profiles.Profile(r.Context(), principal.UserID, time.Now())
	if err != nil {
		h.serverError(w, r, err)
		return
	}

	h.writeJSON(w, r, http.StatusOK, meResponse{
		profileResponse: toProfileResponse(p),
		SessionID:       principal.SessionID,
	})
}

// changePassword, oturum açmış kullanıcının parolasını değiştirir. İsteği yapan
// oturum açık kalır, diğer bütün oturumlar kapatılır.
func (h *Handler) changePassword(w http.ResponseWriter, r *http.Request) {
	principal, ok := authn.PrincipalFrom(r.Context())
	if !ok {
		h.serverError(w, r, errors.New("iam: /me/password kimlik doğrulama olmadan çağrıldı"))
		return
	}

	var req changePasswordRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	var errs []httpx.FieldError
	if req.CurrentPassword == "" {
		errs = append(errs, httpx.FieldError{Field: "current_password", Message: "Mevcut parola zorunlu."})
	}
	if req.NewPassword == "" {
		errs = append(errs, httpx.FieldError{Field: "new_password", Message: "Yeni parola zorunlu."})
	}
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}

	err := h.auth.ChangePassword(r.Context(), ChangePasswordInput{
		UserID:          principal.UserID,
		SessionID:       principal.SessionID,
		CurrentPassword: req.CurrentPassword,
		NewPassword:     req.NewPassword,
	})
	var policyErr *PolicyError
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.As(err, &policyErr):
		httpx.ValidationFailed(w, r, policyFieldErrors("new_password", policyErr))
	case errors.Is(err, ErrInvalidCurrentPassword):
		_ = httpx.WriteProblem(w, r, httpx.Problem{
			Status: http.StatusBadRequest,
			Code:   "INVALID_CURRENT_PASSWORD",
			Detail: "Mevcut parola hatalı.",
			Errors: []httpx.FieldError{{Field: "current_password", Message: "Mevcut parola hatalı."}},
		})
	default:
		h.authError(w, r, ClientMobile, err) // kilit (429) ve beklenmeyen hatalar; çereze dokunulmaz
	}
}

// forgotPassword, sıfırlama bağlantısı ister. Hesap var olsun ya da olmasın aynı
// yanıtı (202) döner: yanıttan bir hesabın kayıtlı olup olmadığı anlaşılamaz.
func (h *Handler) forgotPassword(w http.ResponseWriter, r *http.Request) {
	var req forgotPasswordRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	id := strings.TrimSpace(req.Identifier)
	if id == "" || len(id) > 254 || strings.ContainsFunc(id, unicode.IsControl) {
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "identifier", Message: "Kullanıcı adı ya da e-posta zorunlu."}})
		return
	}

	if err := h.auth.RequestPasswordReset(r.Context(), id); err != nil {
		h.serverError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (h *Handler) verifyResetToken(w http.ResponseWriter, r *http.Request) {
	var req verifyResetTokenRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	info, err := h.auth.VerifyResetToken(r.Context(), req.Token)
	if err != nil {
		h.resetError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	h.writeJSON(w, r, http.StatusOK, resetTokenResponse{Purpose: info.Purpose, ExpiresAt: info.ExpiresAt})
}

// resetPassword, bağlantıyla yeni parolayı belirler (ya da hesabı etkinleştirir).
func (h *Handler) resetPassword(w http.ResponseWriter, r *http.Request) {
	var req resetPasswordRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	if req.NewPassword == "" {
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "new_password", Message: "Yeni parola zorunlu."}})
		return
	}

	if err := h.auth.ResetPassword(r.Context(), req.Token, req.NewPassword); err != nil {
		h.resetError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) resetError(w http.ResponseWriter, r *http.Request, err error) {
	var policyErr *PolicyError
	switch {
	case errors.As(err, &policyErr):
		httpx.ValidationFailed(w, r, policyFieldErrors("new_password", policyErr))
	case errors.Is(err, ErrInvalidResetToken):
		h.problem(w, r, http.StatusBadRequest, "INVALID_RESET_TOKEN",
			"Bağlantı geçersiz, kullanılmış ya da süresi dolmuş. Lütfen yeni bir bağlantı isteyin.")
	default:
		h.serverError(w, r, err)
	}
}

func (h *Handler) listSessions(w http.ResponseWriter, r *http.Request) {
	principal, ok := authn.PrincipalFrom(r.Context())
	if !ok {
		h.serverError(w, r, errors.New("iam: /me/sessions kimlik doğrulama olmadan çağrıldı"))
		return
	}

	sessions, err := h.auth.Sessions(r.Context(), principal.UserID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}

	res := httpx.ListResponse[sessionResponse]{Items: make([]sessionResponse, 0, len(sessions))}
	for _, s := range sessions {
		ua := useragent.Parse(s.UserAgent)
		res.Items = append(res.Items, sessionResponse{
			ID:         s.ID,
			Client:     string(s.ClientType),
			Browser:    optional(ua.Browser),
			OS:         optional(ua.OS),
			Device:     ua.Device,
			IP:         optional(s.IP),
			CreatedAt:  s.CreatedAt,
			LastSeenAt: s.LastSeenAt,
			ExpiresAt:  s.ExpiresAt,
			Current:    s.ID == principal.SessionID,
		})
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

func (h *Handler) endSession(w http.ResponseWriter, r *http.Request) {
	principal, ok := authn.PrincipalFrom(r.Context())
	if !ok {
		h.serverError(w, r, errors.New("iam: oturum kapatma kimlik doğrulama olmadan çağrıldı"))
		return
	}
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return
	}

	err := h.auth.EndSession(r.Context(), principal.UserID, id)
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.NotFound(w, r)
	case err != nil:
		h.serverError(w, r, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

type endSessionsResponse struct {
	Revoked int `json:"revoked"`
}

func (h *Handler) endOtherSessions(w http.ResponseWriter, r *http.Request) {
	principal, ok := authn.PrincipalFrom(r.Context())
	if !ok {
		h.serverError(w, r, errors.New("iam: oturum kapatma kimlik doğrulama olmadan çağrıldı"))
		return
	}

	n, err := h.auth.EndOtherSessions(r.Context(), principal.UserID, principal.SessionID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	h.writeJSON(w, r, http.StatusOK, endSessionsResponse{Revoked: n})
}

// violationMessages, parola politikası ihlallerinin kullanıcıya gösterilen karşılıklarıdır.
var violationMessages = map[password.Violation]string{
	password.TooShort:             "Parola en az 10 karakter olmalı.",
	password.TooLong:              "Parola en fazla 128 karakter olabilir.",
	password.TooCommon:            "Bu parola çok yaygın ve kolay tahmin edilir.",
	password.ContainsPersonalInfo: "Parola kullanıcı adınızı, adınızı ya da e-postanızı içermemeli.",
	password.SameAsCurrent:        "Yeni parola mevcut parolanızla aynı olamaz.",
}

// policyFieldErrors, politika ihlallerini alan hatalarına çevirir. Code makine
// okunur ihlal kodudur, istemci kendi mesajını seçebilir.
func policyFieldErrors(field string, e *PolicyError) []httpx.FieldError {
	errs := make([]httpx.FieldError, 0, len(e.Violations))
	for _, v := range e.Violations {
		errs = append(errs, httpx.FieldError{Field: field, Message: violationMessages[v], Code: string(v)})
	}
	return errs
}

// myPermissions, kullanıcının yetkilerini döndürür. Web arayüzü menüleri ve
// butonları buna göre gösterir. Asıl kontrol her zaman sunucudadır.
func (h *Handler) myPermissions(w http.ResponseWriter, r *http.Request) {
	perms, ok := authz.PermissionsFrom(r.Context())
	if !ok {
		h.serverError(w, r, errors.New("iam: /me/permissions yetki çözümü olmadan çağrıldı"))
		return
	}

	grants := perms.Grants()
	res := httpx.ListResponse[grantResponse]{Items: make([]grantResponse, 0, len(grants))}
	for _, g := range grants {
		res.Items = append(res.Items, grantResponse{
			Permission: g.Permission,
			ScopeType:  g.ScopeType,
			ScopeID:    optional(g.ScopeID),
		})
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

// getUser, bir kullanıcının profilini döndürür. user:read yetkisi gerekir.
func (h *Handler) getUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return
	}

	p, err := h.profiles.Profile(r.Context(), id, time.Now())
	if errors.Is(err, ErrNotFound) {
		httpx.NotFound(w, r)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	h.writeJSON(w, r, http.StatusOK, toProfileResponse(p))
}

// --- Yardımcılar ---------------------------------------------------------------

// clientType, X-Agora-Client başlığını okur. Başlık zorunludur: refresh token'ın
// çerezde mi gövdede mi taşınacağını o belirler. Ayrıca tarayıcılar başka bir
// siteden bu başlıkla istek göndermeden önce CORS izni sorar (preflight), bu da
// çerezli uçları sahte isteklere (CSRF) karşı korur.
func (h *Handler) clientType(w http.ResponseWriter, r *http.Request) (ClientType, bool) {
	switch strings.ToLower(r.Header.Get(ClientHeader)) {
	case "web":
		return ClientWeb, true
	case "mobile":
		return ClientMobile, true
	}
	_ = httpx.WriteProblem(w, r, httpx.Problem{
		Status: http.StatusBadRequest,
		Code:   "INVALID_CLIENT",
		Detail: ClientHeader + " başlığı web veya mobile olmalı.",
	})
	return "", false
}

// readRefreshToken, refresh token'ı web istemcisinde çerezden, mobilde gövdeden okur.
// Token yoksa boş string döner: servis bunu geçersiz token olarak reddeder.
func (h *Handler) readRefreshToken(w http.ResponseWriter, r *http.Request, client ClientType) (string, bool) {
	if client == ClientWeb {
		c, err := r.Cookie(RefreshCookieName)
		if err != nil {
			return "", true
		}
		return c.Value, true
	}

	var req refreshRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return "", false
	}
	return req.RefreshToken, true
}

func (h *Handler) writeTokens(w http.ResponseWriter, r *http.Request, client ClientType, t Tokens) {
	res := tokenResponse{
		AccessToken:        t.AccessToken,
		TokenType:          "Bearer",
		ExpiresIn:          secondsUntil(t.AccessExpiresAt),
		SessionID:          t.SessionID,
		MustChangePassword: t.MustChangePassword,
	}

	if client == ClientWeb {
		// Web'de refresh token JavaScript'e hiç verilmez: XSS ile çalınamaz.
		http.SetCookie(w, &http.Cookie{
			Name:     RefreshCookieName,
			Value:    t.RefreshToken,
			Path:     refreshCookiePath,
			MaxAge:   secondsUntil(t.RefreshExpiresAt),
			HttpOnly: true,
			Secure:   h.secureCookie,
			SameSite: http.SameSiteStrictMode,
		})
	} else {
		res.RefreshToken = t.RefreshToken
		res.RefreshExpiresIn = secondsUntil(t.RefreshExpiresAt)
	}

	// Token içeren yanıtlar hiçbir önbellekte saklanmamalı (RFC 6749 §5.1).
	w.Header().Set("Cache-Control", "no-store")
	h.writeJSON(w, r, http.StatusOK, res)
}

func (h *Handler) clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     RefreshCookieName,
		Value:    "",
		Path:     refreshCookiePath,
		MaxAge:   -1, // "Max-Age=0": tarayıcı çerezi hemen siler
		HttpOnly: true,
		Secure:   h.secureCookie,
		SameSite: http.SameSiteStrictMode,
	})
}

// authError, servis hatalarını HTTP yanıtlarına çevirir.
func (h *Handler) authError(w http.ResponseWriter, r *http.Request, client ClientType, err error) {
	var locked *LockedError

	switch {
	case errors.As(err, &locked):
		w.Header().Set("Retry-After", strconv.Itoa(secondsUntil(locked.Until)))
		h.problem(w, r, http.StatusTooManyRequests, "ACCOUNT_LOCKED",
			"Çok sayıda başarısız deneme yapıldı. Hesap geçici olarak kilitlendi.")

	case errors.Is(err, ErrInvalidCredentials):
		h.problem(w, r, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Kullanıcı adı veya parola hatalı.")

	case errors.Is(err, ErrAccountDisabled):
		if client == ClientWeb {
			h.clearRefreshCookie(w)
		}
		h.problem(w, r, http.StatusForbidden, "ACCOUNT_DISABLED", "Hesap aktif değil.")

	case errors.Is(err, ErrRefreshTokenReused):
		h.logger.Warn("refresh token yeniden kullanıldı, oturum iptal edildi",
			"ip", httpx.ClientIP(r), "request_id", httpx.RequestIDFrom(r.Context()))
		fallthrough

	case errors.Is(err, ErrInvalidRefreshToken):
		if client == ClientWeb {
			h.clearRefreshCookie(w)
		}
		h.problem(w, r, http.StatusUnauthorized, "INVALID_REFRESH_TOKEN",
			"Oturum geçersiz ya da süresi dolmuş. Lütfen tekrar giriş yapın.")

	default:
		h.serverError(w, r, err)
	}
}

func (h *Handler) problem(w http.ResponseWriter, r *http.Request, status int, code, detail string) {
	_ = httpx.WriteProblem(w, r, httpx.Problem{Status: status, Code: code, Detail: detail})
}

func (h *Handler) writeJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	if err := httpx.WriteJSON(w, status, v); err != nil {
		h.logger.Error("yanıt yazılamadı",
			"err", err, "path", r.URL.Path, "request_id", httpx.RequestIDFrom(r.Context()))
	}
}

func (h *Handler) serverError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.Error("beklenmeyen hata",
		"err", err, "path", r.URL.Path, "request_id", httpx.RequestIDFrom(r.Context()))
	httpx.InternalServerError(w, r)
}

// secondsUntil, t'ye kalan süreyi yukarı yuvarlanmış saniye olarak döndürür.
// Yukarı yuvarlamak önemli: Retry-After kilidin bitmesinden önceki bir anı göstermemeli.
func secondsUntil(t time.Time) int {
	d := time.Until(t)
	if d <= 0 {
		return 0
	}
	return int((d + time.Second - 1) / time.Second)
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
