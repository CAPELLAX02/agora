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
	AssignRole(ctx context.Context, actorID, userID string, in AssignInput) (string, error)
	EndAssignment(ctx context.Context, actorID, userID, assignmentID, reason string) error
}

// UserStore, hesap okuma işlemleridir. *Repository bunu sağlar.
type UserStore interface {
	ListUsers(ctx context.Context, f UserFilter) ([]UserSummary, bool, error)
	Profile(ctx context.Context, userID string, at time.Time) (Profile, error)
	Roles(ctx context.Context) ([]RoleDef, error)
	UserAssignments(ctx context.Context, userID string) ([]Assignment, error)
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

	// Rol kataloğu gizli değildir: arayüz rol adlarını göstermek için okur.
	rt.HandleFunc("GET /api/v1/roles", authz.Authenticated, h.listRoles)
	rt.HandleFunc("GET /api/v1/users/{id}/roles", authz.Permission("user:read"), h.listAssignments)
	rt.HandleFunc("POST /api/v1/users/{id}/roles", authz.Permission("role:assign"), h.assignRole)
	rt.HandleFunc("POST /api/v1/users/{id}/roles/{assignment}/end", authz.Permission("role:assign"), h.endAssignment)
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

type roleDefResponse struct {
	Code        string   `json:"code"`
	NameTR      string   `json:"name_tr"`
	NameEN      string   `json:"name_en"`
	ScopeType   string   `json:"scope_type"`
	Description *string  `json:"description"`
	Permissions []string `json:"permissions"`
}

type assignmentResponse struct {
	ID         string     `json:"id"`
	Role       string     `json:"role"`
	RoleName   string     `json:"role_name"`
	ScopeType  string     `json:"scope_type"`
	ScopeID    *string    `json:"scope_id"`
	ScopeName  *string    `json:"scope_name"`
	ValidFrom  time.Time  `json:"valid_from"`
	ValidUntil *time.Time `json:"valid_until"`
	State      string     `json:"state"`
	AssignedBy *string    `json:"assigned_by"`
	Reason     *string    `json:"reason"`
	CreatedAt  time.Time  `json:"created_at"`
}

type assignRoleRequest struct {
	Role       string     `json:"role"`
	ScopeID    string     `json:"scope_id"`
	ValidFrom  *time.Time `json:"valid_from"`
	ValidUntil *time.Time `json:"valid_until"`
	Reason     string     `json:"reason"`
}

type endAssignmentRequest struct {
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

func (h *UsersHandler) listRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := h.users.Roles(r.Context())
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	res := httpx.ListResponse[roleDefResponse]{Items: make([]roleDefResponse, 0, len(roles))}
	for _, d := range roles {
		res.Items = append(res.Items, roleDefResponse{
			Code: d.Code, NameTR: d.NameTR, NameEN: d.NameEN, ScopeType: string(d.ScopeType),
			Description: optional(d.Description), Permissions: d.Permissions,
		})
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

func (h *UsersHandler) listAssignments(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return
	}
	list, err := h.users.UserAssignments(r.Context(), id)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	res := httpx.ListResponse[assignmentResponse]{Items: make([]assignmentResponse, 0, len(list))}
	for _, a := range list {
		res.Items = append(res.Items, toAssignmentResponse(a))
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

func toAssignmentResponse(a Assignment) assignmentResponse {
	return assignmentResponse{
		ID: a.ID, Role: a.Role, RoleName: a.RoleName, ScopeType: string(a.ScopeType),
		ScopeID: optional(a.ScopeID), ScopeName: optional(a.ScopeName),
		ValidFrom: a.ValidFrom, ValidUntil: a.ValidUntil, State: a.State,
		AssignedBy: optional(a.AssignedBy), Reason: optional(a.Reason), CreatedAt: a.CreatedAt,
	}
}

func (h *UsersHandler) assignRole(w http.ResponseWriter, r *http.Request) {
	actor, _ := authn.PrincipalFrom(r.Context())
	userID := r.PathValue("id")
	if !httpx.ValidUUID(userID) {
		httpx.NotFound(w, r)
		return
	}

	var req assignRoleRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	var errs []httpx.FieldError
	if req.Role == "" {
		errs = append(errs, httpx.FieldError{Field: "role", Message: "Rol zorunlu."})
	}
	if req.ScopeID != "" && !httpx.ValidUUID(req.ScopeID) {
		errs = append(errs, httpx.FieldError{Field: "scope_id", Message: "Geçerli bir UUID olmalı."})
	}
	if req.Reason == "" || utf8.RuneCountInString(req.Reason) > 500 {
		errs = append(errs, httpx.FieldError{Field: "reason", Message: "Gerekçe zorunlu, en fazla 500 karakter."})
	}
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}

	id, err := h.accounts.AssignRole(r.Context(), actor.UserID, userID, AssignInput(req))
	switch {
	case errors.Is(err, ErrUnknownRole):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "role", Message: "Rol bulunamadı."}})
		return
	case errors.Is(err, ErrInvalidScope):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "scope_id",
			Message: "Bu rolün kapsam türüne uygun bir birim seçilmeli (üniversite geneli rollerde boş bırakılır)."}})
		return
	case errors.Is(err, ErrUnknownScope):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "scope_id", Message: "Birim bulunamadı."}})
		return
	case errors.Is(err, ErrInvalidPeriod):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "valid_until", Message: "Bitiş başlangıçtan sonra olmalı."}})
		return
	case errors.Is(err, ErrConflict):
		h.problem(w, r, http.StatusConflict, "ROLE_ALREADY_ASSIGNED",
			"Bu rol bu kapsamda çakışan bir dönemde zaten atanmış.")
		return
	case err != nil:
		h.writeActionResult(w, r, err, 0)
		return
	}

	list, err := h.users.UserAssignments(r.Context(), userID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	for _, a := range list {
		if a.ID == id {
			w.Header().Set("Location", "/api/v1/users/"+userID+"/roles")
			h.writeJSON(w, r, http.StatusCreated, toAssignmentResponse(a))
			return
		}
	}
	h.serverError(w, r, errors.New("iam: oluşturulan atama okunamadı"))
}

func (h *UsersHandler) endAssignment(w http.ResponseWriter, r *http.Request) {
	actor, _ := authn.PrincipalFrom(r.Context())
	userID, assignmentID := r.PathValue("id"), r.PathValue("assignment")
	if !httpx.ValidUUID(userID) || !httpx.ValidUUID(assignmentID) {
		httpx.NotFound(w, r)
		return
	}

	var req endAssignmentRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if req.Reason == "" || utf8.RuneCountInString(req.Reason) > 500 {
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "reason", Message: "Gerekçe zorunlu, en fazla 500 karakter."}})
		return
	}

	err := h.accounts.EndAssignment(r.Context(), actor.UserID, userID, assignmentID, req.Reason)
	switch {
	case errors.Is(err, ErrRoleAssignmentNotFound):
		httpx.NotFound(w, r)
	case errors.Is(err, ErrLastSystemAdmin):
		h.problem(w, r, http.StatusConflict, "LAST_SYSTEM_ADMIN",
			"Son aktif sistem yöneticisinin rolü sonlandırılamaz. Önce başka bir yönetici atayın.")
	default:
		h.writeActionResult(w, r, err, http.StatusNoContent)
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
