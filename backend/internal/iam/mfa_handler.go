package iam

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

// registerMFA, iki adımlı doğrulama uçlarını kaydeder.
func (h *Handler) registerMFA(rt *authz.Router, limits Limits) {
	// Girişin ikinci adımı, girişle aynı hız sınırını paylaşır: kod denemesi de bir
	// giriş denemesidir.
	rt.HandleFunc("POST /api/v1/auth/mfa/verify", authz.Public, h.verifyMFA, limits.Login)

	rt.HandleFunc("GET /api/v1/me/mfa", authz.SelfService, h.mfaStatus)
	rt.HandleFunc("POST /api/v1/me/mfa/setup", authz.SelfService, h.startMFASetup)
	rt.HandleFunc("POST /api/v1/me/mfa/enable", authz.SelfService, h.enableMFA)
	rt.HandleFunc("POST /api/v1/me/mfa/disable", authz.SelfService, h.disableMFA)
	rt.HandleFunc("POST /api/v1/me/mfa/recovery-codes", authz.SelfService, h.regenerateRecoveryCodes)
}

type mfaChallengeResponse struct {
	MFARequired bool   `json:"mfa_required"` // her zaman true: istemci yanıt türünü buna bakarak ayırır
	MFAToken    string `json:"mfa_token"`
	ExpiresIn   int    `json:"expires_in"` // saniye
}

type mfaVerifyRequest struct {
	MFAToken     string `json:"mfa_token"`
	Code         string `json:"code"`
	RecoveryCode string `json:"recovery_code"`
}

type mfaStatusResponse struct {
	Enabled                bool       `json:"enabled"`
	EnabledAt              *time.Time `json:"enabled_at"`
	RecoveryCodesRemaining int        `json:"recovery_codes_remaining"`
}

type mfaSetupResponse struct {
	Secret     string `json:"secret"`
	OTPAuthURI string `json:"otpauth_uri"`
}

type enableMFARequest struct {
	CurrentPassword string `json:"current_password"`
	Code            string `json:"code"`
}

type disableMFARequest struct {
	CurrentPassword string `json:"current_password"`
	Code            string `json:"code"`
	RecoveryCode    string `json:"recovery_code"`
}

type regenerateRecoveryCodesRequest struct {
	Code string `json:"code"`
}

type recoveryCodesResponse struct {
	RecoveryCodes []string `json:"recovery_codes"`
}

// writeMFAChallenge, parolası doğru ama ikinci adımı bekleyen girişin yanıtıdır.
// Oturum henüz açılmadığı için çerez yazılmaz.
func (h *Handler) writeMFAChallenge(w http.ResponseWriter, r *http.Request, c *MFARequiredError) {
	w.Header().Set("Cache-Control", "no-store")
	h.writeJSON(w, r, http.StatusOK, mfaChallengeResponse{
		MFARequired: true,
		MFAToken:    c.Token,
		ExpiresIn:   secondsUntil(c.ExpiresAt),
	})
}

// secondFactorErrors, "kod ya da kurtarma kodundan tam olarak biri" kuralını denetler.
func secondFactorErrors(code, recoveryCode string, allowRecovery bool) []httpx.FieldError {
	switch {
	case code != "" && recoveryCode != "":
		return []httpx.FieldError{{Field: "code", Message: "Doğrulama kodu ya da kurtarma kodundan sadece biri gönderilmeli."}}
	case code == "" && (recoveryCode == "" || !allowRecovery):
		return []httpx.FieldError{{Field: "code", Message: "Doğrulama kodu zorunlu."}}
	case len(code) > 16 || len(recoveryCode) > 32:
		return []httpx.FieldError{{Field: "code", Message: "Kod geçersiz."}}
	}
	return nil
}

func (h *Handler) verifyMFA(w http.ResponseWriter, r *http.Request) {
	client, ok := h.clientType(w, r)
	if !ok {
		return
	}
	var req mfaVerifyRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	req.Code, req.RecoveryCode = strings.TrimSpace(req.Code), strings.TrimSpace(req.RecoveryCode)
	errs := secondFactorErrors(req.Code, req.RecoveryCode, true)
	if req.MFAToken == "" {
		errs = append(errs, httpx.FieldError{Field: "mfa_token", Message: "Doğrulama oturumu zorunlu."})
	}
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}

	tokens, err := h.auth.VerifyMFA(r.Context(), MFAVerifyInput{
		Token:        req.MFAToken,
		Code:         req.Code,
		RecoveryCode: req.RecoveryCode,
		Client:       client,
		IP:           httpx.ClientInfoFrom(r.Context()).IP,
		UserAgent:    httpx.ClientInfoFrom(r.Context()).UserAgent,
	})
	if err != nil {
		h.authError(w, r, client, err)
		return
	}
	h.writeTokens(w, r, client, tokens)
}

func (h *Handler) mfaStatus(w http.ResponseWriter, r *http.Request) {
	principal, _ := authn.PrincipalFrom(r.Context())
	s, err := h.auth.MFAStatus(r.Context(), principal.UserID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	h.writeJSON(w, r, http.StatusOK, mfaStatusResponse(s))
}

func (h *Handler) startMFASetup(w http.ResponseWriter, r *http.Request) {
	principal, _ := authn.PrincipalFrom(r.Context())
	setup, err := h.auth.StartMFASetup(r.Context(), principal.UserID)
	if err != nil {
		h.mfaError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	h.writeJSON(w, r, http.StatusOK, mfaSetupResponse{Secret: setup.Secret, OTPAuthURI: setup.URI})
}

func (h *Handler) enableMFA(w http.ResponseWriter, r *http.Request) {
	principal, _ := authn.PrincipalFrom(r.Context())
	var req enableMFARequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	req.Code = strings.TrimSpace(req.Code)
	errs := secondFactorErrors(req.Code, "", false)
	if req.CurrentPassword == "" {
		errs = append(errs, httpx.FieldError{Field: "current_password", Message: "Mevcut parola zorunlu."})
	}
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}

	codes, err := h.auth.EnableMFA(r.Context(), EnableMFAInput{
		UserID: principal.UserID, SessionID: principal.SessionID,
		CurrentPassword: req.CurrentPassword, Code: req.Code,
	})
	if err != nil {
		h.mfaError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	h.writeJSON(w, r, http.StatusOK, recoveryCodesResponse{RecoveryCodes: codes})
}

func (h *Handler) disableMFA(w http.ResponseWriter, r *http.Request) {
	principal, _ := authn.PrincipalFrom(r.Context())
	var req disableMFARequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	req.Code, req.RecoveryCode = strings.TrimSpace(req.Code), strings.TrimSpace(req.RecoveryCode)
	errs := secondFactorErrors(req.Code, req.RecoveryCode, true)
	if req.CurrentPassword == "" {
		errs = append(errs, httpx.FieldError{Field: "current_password", Message: "Mevcut parola zorunlu."})
	}
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}

	err := h.auth.DisableMFA(r.Context(), DisableMFAInput{
		UserID: principal.UserID, SessionID: principal.SessionID,
		CurrentPassword: req.CurrentPassword, Code: req.Code, RecoveryCode: req.RecoveryCode,
	})
	if err != nil {
		h.mfaError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) regenerateRecoveryCodes(w http.ResponseWriter, r *http.Request) {
	principal, _ := authn.PrincipalFrom(r.Context())
	var req regenerateRecoveryCodesRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	req.Code = strings.TrimSpace(req.Code)
	if errs := secondFactorErrors(req.Code, "", false); len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}

	codes, err := h.auth.RegenerateRecoveryCodes(r.Context(), principal.UserID, req.Code)
	if err != nil {
		h.mfaError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	h.writeJSON(w, r, http.StatusOK, recoveryCodesResponse{RecoveryCodes: codes})
}

// mfaError, oturum açmış kullanıcının MFA işlemlerindeki hataları yanıtlara çevirir.
// Buradaki yanlış kod 400'dür (girişteki 401 değil): oturum geçerli, sadece alan hatalı.
func (h *Handler) mfaError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrInvalidCurrentPassword):
		_ = httpx.WriteProblem(w, r, httpx.Problem{
			Status: http.StatusBadRequest, Code: "INVALID_CURRENT_PASSWORD", Detail: "Mevcut parola hatalı.",
			Errors: []httpx.FieldError{{Field: "current_password", Message: "Mevcut parola hatalı."}},
		})
	case errors.Is(err, ErrInvalidMFACode):
		_ = httpx.WriteProblem(w, r, httpx.Problem{
			Status: http.StatusBadRequest, Code: "INVALID_MFA_CODE", Detail: "Doğrulama kodu hatalı.",
			Errors: []httpx.FieldError{{Field: "code", Message: "Doğrulama kodu hatalı."}},
		})
	case errors.Is(err, ErrMFAAlreadyEnabled):
		h.problem(w, r, http.StatusConflict, "MFA_ALREADY_ENABLED", "İki adımlı doğrulama zaten açık.")
	case errors.Is(err, ErrMFANotEnabled):
		h.problem(w, r, http.StatusConflict, "MFA_NOT_ENABLED", "İki adımlı doğrulama açık değil.")
	case errors.Is(err, ErrMFASetupRequired):
		h.problem(w, r, http.StatusConflict, "MFA_SETUP_REQUIRED", "Önce iki adımlı doğrulama kurulumunu başlatın.")
	default:
		h.authError(w, r, ClientMobile, err) // kilit (429) ve beklenmeyen hatalar; çereze dokunulmaz
	}
}
