package iam

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/CAPELLAX02/agora/backend/internal/people"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

// AccountService, hesap yönetimi işlemleridir. *Auth bunu sağlar.
type AccountService interface {
	CreateAccount(ctx context.Context, actorID string, in NewAccount) (string, error)
	SetStatus(ctx context.Context, actorID, userID string, status UserStatus, reason string) error
	ResendActivation(ctx context.Context, actorID, userID string) error
	SendPasswordReset(ctx context.Context, actorID, userID string) error
}

// UserStore, hesap okuma işlemleridir. *Repository bunu sağlar.
type UserStore interface {
	ListUsers(ctx context.Context, f UserFilter) ([]UserSummary, bool, error)
	Profile(ctx context.Context, userID string, at time.Time) (Profile, error)
}

// UsersHandler, yöneticinin hesap yönetimi uçlarını sunar.
type UsersHandler struct {
	accounts AccountService
	users    UserStore
	logger   *slog.Logger
}

// NewUsersHandler, bir UsersHandler oluşturur.
func NewUsersHandler(accounts AccountService, users UserStore, logger *slog.Logger) *UsersHandler {
	return &UsersHandler{accounts: accounts, users: users, logger: logger}
}

// Register, route'ları kaydeder.
func (h *UsersHandler) Register(rt *authz.Router) {
	rt.HandleFunc("GET /api/v1/users", authz.Permission("user:read"), h.list)
	rt.HandleFunc("GET /api/v1/users/{id}", authz.Permission("user:read"), h.get)
	rt.HandleFunc("POST /api/v1/users", authz.Permission("user:manage"), h.create)
	rt.HandleFunc("PUT /api/v1/users/{id}/status", authz.Permission("user:manage"), h.setStatus)
	rt.HandleFunc("POST /api/v1/users/{id}/activation-email", authz.Permission("user:manage"), h.resendActivation)
	rt.HandleFunc("POST /api/v1/users/{id}/password-reset-email", authz.Permission("user:manage"), h.sendPasswordReset)
}

type userSummaryResponse struct {
	ID          string     `json:"id"`
	Username    string     `json:"username"`
	Email       string     `json:"email"`
	FirstName   string     `json:"first_name"`
	LastName    string     `json:"last_name"`
	Status      string     `json:"status"`
	Kind        *string    `json:"kind"`
	LastLoginAt *time.Time `json:"last_login_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

type createUserRequest struct {
	Kind          string `json:"kind"`
	Number        string `json:"number"`
	FirstName     string `json:"first_name"`
	LastName      string `json:"last_name"`
	Email         string `json:"email"`
	StaffType     string `json:"staff_type"`
	AcademicTitle string `json:"academic_title"`
	DepartmentID  string `json:"department_id"`
}

type setStatusRequest struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

const maxQueryLength = 100

func (h *UsersHandler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var errs []httpx.FieldError

	f := UserFilter{
		Query:  strings.TrimSpace(q.Get("q")),
		Status: UserStatus(q.Get("status")),
		Kind:   q.Get("kind"),
	}
	if utf8.RuneCountInString(f.Query) > maxQueryLength {
		errs = append(errs, httpx.FieldError{Field: "q", Message: "En fazla 100 karakter olabilir."})
	}
	switch f.Status {
	case "", StatusPending, StatusActive, StatusSuspended, StatusDisabled:
	default:
		errs = append(errs, httpx.FieldError{Field: "status", Message: "Geçersiz durum."})
	}
	switch f.Kind {
	case "", KindStudent, KindStaff:
	default:
		errs = append(errs, httpx.FieldError{Field: "kind", Message: "STUDENT ya da STAFF olmalı."})
	}
	limit, limitErr := httpx.ParseLimit(q)
	if limitErr != nil {
		errs = append(errs, *limitErr)
	}
	f.Limit = limit
	if c := q.Get("cursor"); c != "" {
		cur, err := httpx.DecodeCursor[UserCursor](c)
		if err != nil || !httpx.ValidUUID(cur.ID) {
			errs = append(errs, httpx.FieldError{Field: "cursor", Message: "Geçersiz cursor."})
		} else {
			f.After = &cur
		}
	}
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}

	users, hasMore, err := h.users.ListUsers(r.Context(), f)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	res := httpx.ListResponse[userSummaryResponse]{Items: make([]userSummaryResponse, 0, len(users))}
	for _, u := range users {
		res.Items = append(res.Items, userSummaryResponse{
			ID: u.ID, Username: u.Username, Email: u.Email, FirstName: u.FirstName, LastName: u.LastName,
			Status: string(u.Status), Kind: optional(u.Kind), LastLoginAt: u.LastLoginAt, CreatedAt: u.CreatedAt,
		})
	}
	if hasMore {
		last := users[len(users)-1]
		if res.NextCursor, err = httpx.EncodeCursor(UserCursor{Username: last.Username, ID: last.ID}); err != nil {
			h.serverError(w, r, err)
			return
		}
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

func (h *UsersHandler) get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return
	}
	p, err := h.users.Profile(r.Context(), id, time.Now())
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

var (
	studentNumberPattern = regexp.MustCompile(`^[0-9]{8,11}$`)
	staffNumberPattern   = regexp.MustCompile(`^[A-Za-z0-9]{3,20}$`)
)

func (h *UsersHandler) create(w http.ResponseWriter, r *http.Request) {
	actor, _ := authn.PrincipalFrom(r.Context())

	var req createUserRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	in, errs := req.validate()
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}

	userID, err := h.accounts.CreateAccount(r.Context(), actor.UserID, in)
	switch {
	case errors.Is(err, ErrConflict):
		h.problem(w, r, http.StatusConflict, "ACCOUNT_EXISTS", "Bu numara ya da e-posta başka bir hesapta kayıtlı.")
		return
	case errors.Is(err, ErrUnknownDepartment):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "department_id", Message: "Bölüm bulunamadı."}})
		return
	case errors.Is(err, ErrUnknownAcademicTitle):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "academic_title", Message: "Akademik unvan bulunamadı."}})
		return
	case err != nil:
		h.serverError(w, r, err)
		return
	}

	p, err := h.users.Profile(r.Context(), userID, time.Now())
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/users/"+userID)
	h.writeJSON(w, r, http.StatusCreated, toProfileResponse(p))
}

// validate, isteği doğrular ve servis girdisine çevirir.
func (req createUserRequest) validate() (NewAccount, []httpx.FieldError) {
	in := NewAccount{
		Kind:          req.Kind,
		Number:        strings.TrimSpace(req.Number),
		FirstName:     strings.TrimSpace(req.FirstName),
		LastName:      strings.TrimSpace(req.LastName),
		Email:         strings.TrimSpace(req.Email),
		StaffType:     people.StaffType(req.StaffType),
		AcademicTitle: req.AcademicTitle,
		DepartmentID:  req.DepartmentID,
	}
	var errs []httpx.FieldError
	add := func(field, msg string) { errs = append(errs, httpx.FieldError{Field: field, Message: msg}) }

	switch in.Kind {
	case KindStudent:
		if !studentNumberPattern.MatchString(in.Number) {
			add("number", "Öğrenci numarası 8-11 haneli olmalı.")
		}
		if in.StaffType != "" || in.AcademicTitle != "" || in.DepartmentID != "" {
			add("kind", "Personel alanları sadece personel hesaplarında kullanılabilir.")
		}
	case KindStaff:
		if !staffNumberPattern.MatchString(in.Number) {
			add("number", "Personel numarası 3-20 harf ya da rakam olmalı.")
		}
		switch in.StaffType {
		case people.StaffAcademic:
		case people.StaffAdministrative:
			if in.AcademicTitle != "" {
				add("academic_title", "Akademik unvan sadece akademik personelde olabilir.")
			}
		default:
			add("staff_type", "ACADEMIC ya da ADMINISTRATIVE olmalı.")
		}
		if in.DepartmentID != "" && !httpx.ValidUUID(in.DepartmentID) {
			add("department_id", "Geçerli bir UUID olmalı.")
		}
	default:
		add("kind", "STUDENT ya da STAFF olmalı.")
	}

	for field, v := range map[string]string{"first_name": in.FirstName, "last_name": in.LastName} {
		if v == "" || utf8.RuneCountInString(v) > 100 || strings.ContainsFunc(v, isControl) {
			add(field, "Zorunlu, en fazla 100 karakter.")
		}
	}
	if addr, err := mail.ParseAddress(in.Email); err != nil || addr.Address != in.Email || len(in.Email) > 254 {
		add("email", "Geçerli bir e-posta adresi olmalı.")
	}
	return in, errs
}

func isControl(r rune) bool {
	return r < 0x20 || r == 0x7f
}

func (h *UsersHandler) setStatus(w http.ResponseWriter, r *http.Request) {
	actor, _ := authn.PrincipalFrom(r.Context())
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return
	}

	var req setStatusRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	var errs []httpx.FieldError
	switch UserStatus(req.Status) {
	case StatusActive, StatusSuspended, StatusDisabled:
	default:
		errs = append(errs, httpx.FieldError{Field: "status", Message: "ACTIVE, SUSPENDED ya da DISABLED olmalı."})
	}
	if req.Reason == "" || utf8.RuneCountInString(req.Reason) > 500 {
		errs = append(errs, httpx.FieldError{Field: "reason", Message: "Gerekçe zorunlu, en fazla 500 karakter."})
	}
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}

	err := h.accounts.SetStatus(r.Context(), actor.UserID, id, UserStatus(req.Status), req.Reason)
	h.writeActionResult(w, r, err, http.StatusNoContent)
}

func (h *UsersHandler) resendActivation(w http.ResponseWriter, r *http.Request) {
	h.userAction(w, r, h.accounts.ResendActivation)
}

func (h *UsersHandler) sendPasswordReset(w http.ResponseWriter, r *http.Request) {
	h.userAction(w, r, h.accounts.SendPasswordReset)
}

func (h *UsersHandler) userAction(w http.ResponseWriter, r *http.Request, action func(ctx context.Context, actorID, userID string) error) {
	actor, _ := authn.PrincipalFrom(r.Context())
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return
	}
	h.writeActionResult(w, r, action(r.Context(), actor.UserID, id), http.StatusAccepted)
}

// writeActionResult, hesap işlemlerinin ortak hata eşlemesidir.
func (h *UsersHandler) writeActionResult(w http.ResponseWriter, r *http.Request, err error, okStatus int) {
	switch {
	case err == nil:
		w.WriteHeader(okStatus)
	case errors.Is(err, ErrNotFound):
		httpx.NotFound(w, r)
	case errors.Is(err, ErrSelfAction):
		h.problem(w, r, http.StatusForbidden, "SELF_ACTION_FORBIDDEN", "Bu işlemi kendi hesabınızda yapamazsınız.")
	case errors.Is(err, ErrInvalidStatusChange):
		h.problem(w, r, http.StatusConflict, "INVALID_STATUS_CHANGE",
			"Aktivasyonu tamamlanmamış bir hesap elle etkinleştirilemez. Aktivasyon e-postasını yeniden gönderin.")
	case errors.Is(err, ErrAccountNotPending):
		h.problem(w, r, http.StatusConflict, "ACCOUNT_NOT_PENDING", "Hesap aktivasyon beklemiyor.")
	case errors.Is(err, ErrAccountNotActive):
		h.problem(w, r, http.StatusConflict, "ACCOUNT_NOT_ACTIVE", "Hesap aktif değil.")
	default:
		h.serverError(w, r, err)
	}
}

func (h *UsersHandler) problem(w http.ResponseWriter, r *http.Request, status int, code, detail string) {
	_ = httpx.WriteProblem(w, r, httpx.Problem{Status: status, Code: code, Detail: detail})
}

func (h *UsersHandler) writeJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	if err := httpx.WriteJSON(w, status, v); err != nil {
		h.logger.Error("yanıt yazılamadı", "err", err, "request_id", httpx.RequestIDFrom(r.Context()))
	}
}

func (h *UsersHandler) serverError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.Error("beklenmeyen hata", "err", err, "path", r.URL.Path, "request_id", httpx.RequestIDFrom(r.Context()))
	httpx.InternalServerError(w, r)
}
