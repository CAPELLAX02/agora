package attendance

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

// Handler, yoklamanın HTTP uçlarıdır. Şubeye özgü uçlar sadece kimlik ister, kararı şubeyle
// ilişki verir (canTakeAttendance: şubenin öğretim elemanı, asistan dahil); öğrenci kendi
// devamsızlığını attendance:read_own ile görür.
type Handler struct {
	svc    *Service
	logger *slog.Logger
}

// NewHandler, bir Handler oluşturur.
func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// Register, uçları yönlendiriciye ekler.
func (h *Handler) Register(rt *authz.Router) {
	rt.HandleFunc("GET /api/v1/sections/{id}/sessions", authz.Authenticated, h.sessions)
	rt.HandleFunc("POST /api/v1/sections/{id}/sessions", authz.Authenticated, h.addMakeup)
	rt.HandleFunc("GET /api/v1/sections/{id}/attendance-summary", authz.Authenticated, h.summary)
	rt.HandleFunc("PUT /api/v1/sessions/{id}", authz.Authenticated, h.updateSession)
	rt.HandleFunc("GET /api/v1/sessions/{id}/attendance", authz.Authenticated, h.roll)
	rt.HandleFunc("PUT /api/v1/sessions/{id}/attendance", authz.Authenticated, h.take)
	rt.HandleFunc("GET /api/v1/me/attendance", authz.Permission("attendance:read_own"), h.mine)
}

// --- Yanıtlar -----------------------------------------------------------------------------

type sectionDTO struct {
	ID           string `json:"id"`
	Code         string `json:"section_code"`
	CourseCode   string `json:"course_code"`
	CourseNameTR string `json:"course_name_tr"`
	CourseNameEN string `json:"course_name_en"`
}

type sessionDTO struct {
	ID        string     `json:"id"`
	Type      string     `json:"session_type"`
	Kind      string     `json:"kind"`
	Status    string     `json:"status"`
	StartsAt  time.Time  `json:"starts_at"`
	EndsAt    time.Time  `json:"ends_at"`
	Classroom *string    `json:"classroom"`
	Topic     string     `json:"topic"`
	TakenAt   *time.Time `json:"taken_at"`
	Present   int        `json:"present"`
	Absent    int        `json:"absent"`
	Version   int        `json:"version"`
}

func toSession(x Session) sessionDTO {
	d := sessionDTO{ID: x.ID, Type: x.Type, Kind: x.Kind, Status: x.Status, StartsAt: x.StartsAt, EndsAt: x.EndsAt,
		Topic: x.Topic, TakenAt: x.TakenAt, Present: x.Present, Absent: x.Absent, Version: x.Version}
	if x.Classroom != "" {
		c := x.Classroom
		d.Classroom = &c
	}
	return d
}

type sessionsResponse struct {
	Section sectionDTO   `json:"section"`
	Items   []sessionDTO `json:"items"`
}

type entryDTO struct {
	EnrollmentID string  `json:"enrollment_id"`
	StudentNo    string  `json:"student_no"`
	FirstName    string  `json:"first_name"`
	LastName     string  `json:"last_name"`
	Status       *string `json:"status"` // yoklama alınmamışsa null
}

type rollResponse struct {
	Section sectionDTO `json:"section"`
	Session sessionDTO `json:"session"`
	Items   []entryDTO `json:"items"`
}

type usageDTO struct {
	Group    string `json:"group"`
	Planned  int    `json:"planned"`
	Held     int    `json:"held"`
	Absences int    `json:"absences"`
	Pct      int    `json:"max_absence_pct"`
	Limit    *int   `json:"limit"`
	Near     bool   `json:"near_limit"`
	Exceeded bool   `json:"exceeded"`
}

func toUsage(list []Usage) []usageDTO {
	out := make([]usageDTO, 0, len(list))
	for _, u := range list {
		out = append(out, usageDTO{Group: u.Group, Planned: u.Planned, Held: u.Held, Absences: u.Absences, Pct: u.Pct,
			Limit: u.Limit, Near: u.Near(), Exceeded: u.Exceeded()})
	}
	return out
}

type studentUsageDTO struct {
	EnrollmentID     string     `json:"enrollment_id"`
	StudentNo        string     `json:"student_no"`
	FirstName        string     `json:"first_name"`
	LastName         string     `json:"last_name"`
	AttendanceStatus string     `json:"attendance_status"`
	Usage            []usageDTO `json:"usage"`
}

type summaryResponse struct {
	Section sectionDTO        `json:"section"`
	Items   []studentUsageDTO `json:"items"`
}

type studentSessionDTO struct {
	ID       string    `json:"id"`
	Type     string    `json:"session_type"`
	Kind     string    `json:"kind"`
	Status   string    `json:"status"`
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
	Topic    string    `json:"topic"`
	Record   *string   `json:"record"` // kendi yoklama durumu; alınmamışsa null
}

type courseAttendanceDTO struct {
	EnrollmentID     string              `json:"enrollment_id"`
	SectionID        string              `json:"section_id"`
	SectionCode      string              `json:"section_code"`
	CourseCode       string              `json:"course_code"`
	CourseNameTR     string              `json:"course_name_tr"`
	CourseNameEN     string              `json:"course_name_en"`
	AttendanceStatus string              `json:"attendance_status"`
	Usage            []usageDTO          `json:"usage"`
	Sessions         []studentSessionDTO `json:"sessions"`
}

func toSection(s Section) sectionDTO {
	return sectionDTO{ID: s.ID, Code: s.Code, CourseCode: s.CourseCode, CourseNameTR: s.CourseNameTR, CourseNameEN: s.CourseNameEN}
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// --- Uçlar --------------------------------------------------------------------------------

// teaching, yoldaki şubeyi döndürür; istekte bulunan şubenin öğretim elemanı değilse 403 yazar.
func (h *Handler) teaching(w http.ResponseWriter, r *http.Request, sectionID string) (Section, bool) {
	if !httpx.ValidUUID(sectionID) {
		httpx.NotFound(w, r)
		return Section{}, false
	}
	sec, teaches, err := h.svc.Teaching(r.Context(), h.svc.pool, sectionID, userID(r))
	if h.failed(w, r, err) {
		return Section{}, false
	}
	if !teaches {
		h.failed(w, r, ErrForbidden)
		return Section{}, false
	}
	return sec, true
}

// session, yoldaki oturumu ve şubesini döndürür (şubenin öğretim elemanına).
func (h *Handler) session(w http.ResponseWriter, r *http.Request) (Session, Section, bool) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return Session{}, Section{}, false
	}
	x, err := h.svc.Session(r.Context(), h.svc.pool, id)
	if h.failed(w, r, err) {
		return Session{}, Section{}, false
	}
	sec, ok := h.teaching(w, r, x.SectionID)
	return x, sec, ok
}

func (h *Handler) sessions(w http.ResponseWriter, r *http.Request) {
	sec, ok := h.teaching(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	list, err := h.svc.Sessions(r.Context(), sec.ID)
	if h.failed(w, r, err) {
		return
	}
	res := sessionsResponse{Section: toSection(sec), Items: make([]sessionDTO, 0, len(list))}
	for _, x := range list {
		res.Items = append(res.Items, toSession(x))
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

type makeupRequest struct {
	Type     string    `json:"session_type"`
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
	Topic    string    `json:"topic"`
}

func (h *Handler) addMakeup(w http.ResponseWriter, r *http.Request) {
	sec, ok := h.teaching(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	var req makeupRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	id, err := h.svc.AddMakeup(r.Context(), userID(r), sec.ID, MakeupInput(req))
	if h.failed(w, r, err) {
		return
	}
	x, err := h.svc.Session(r.Context(), h.svc.pool, id)
	if h.failed(w, r, err) {
		return
	}
	w.Header().Set("ETag", httpx.ETag(x.Version))
	h.writeJSON(w, r, http.StatusCreated, toSession(x))
}

type sessionUpdateRequest struct {
	Cancelled bool   `json:"cancelled"`
	Topic     string `json:"topic"`
}

func (h *Handler) updateSession(w http.ResponseWriter, r *http.Request) {
	x, _, ok := h.session(w, r)
	if !ok {
		return
	}
	version, ok := httpx.IfMatchVersion(r)
	if !ok {
		httpx.PreconditionRequired(w, r)
		return
	}
	var req sessionUpdateRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	if h.failed(w, r, h.svc.UpdateSession(r.Context(), userID(r), x.ID, version, SessionUpdate(req))) {
		return
	}
	x, err := h.svc.Session(r.Context(), h.svc.pool, x.ID)
	if h.failed(w, r, err) {
		return
	}
	w.Header().Set("ETag", httpx.ETag(x.Version))
	h.writeJSON(w, r, http.StatusOK, toSession(x))
}

func (h *Handler) roll(w http.ResponseWriter, r *http.Request) {
	x, sec, ok := h.session(w, r)
	if !ok {
		return
	}
	h.respondRoll(w, r, x.ID, sec)
}

func (h *Handler) respondRoll(w http.ResponseWriter, r *http.Request, id string, sec Section) {
	x, list, err := h.svc.Roll(r.Context(), id)
	if h.failed(w, r, err) {
		return
	}
	res := rollResponse{Section: toSection(sec), Session: toSession(x), Items: make([]entryDTO, 0, len(list))}
	for _, e := range list {
		res.Items = append(res.Items, entryDTO{EnrollmentID: e.EnrollmentID, StudentNo: e.StudentNo, FirstName: e.FirstName,
			LastName: e.LastName, Status: optional(e.Status)})
	}
	w.Header().Set("ETag", httpx.ETag(x.Version))
	h.writeJSON(w, r, http.StatusOK, res)
}

type takeRequest struct {
	Items []struct {
		EnrollmentID string `json:"enrollment_id"`
		Status       string `json:"status"`
	} `json:"items"`
}

func (h *Handler) take(w http.ResponseWriter, r *http.Request) {
	x, sec, ok := h.session(w, r)
	if !ok {
		return
	}
	version, ok := httpx.IfMatchVersion(r)
	if !ok {
		httpx.PreconditionRequired(w, r)
		return
	}
	var req takeRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	statuses := make(map[string]string, len(req.Items))
	for _, it := range req.Items {
		if !httpx.ValidUUID(it.EnrollmentID) {
			httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "items.enrollment_id", Message: "Geçerli bir UUID olmalı."}})
			return
		}
		statuses[it.EnrollmentID] = it.Status
	}
	if h.failed(w, r, h.svc.Take(r.Context(), userID(r), x.ID, version, statuses)) {
		return
	}
	h.respondRoll(w, r, x.ID, sec)
}

func (h *Handler) summary(w http.ResponseWriter, r *http.Request) {
	sec, ok := h.teaching(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	list, err := h.svc.Summary(r.Context(), sec.ID)
	if h.failed(w, r, err) {
		return
	}
	res := summaryResponse{Section: toSection(sec), Items: make([]studentUsageDTO, 0, len(list))}
	for _, x := range list {
		res.Items = append(res.Items, studentUsageDTO{EnrollmentID: x.EnrollmentID, StudentNo: x.StudentNo, FirstName: x.FirstName,
			LastName: x.LastName, AttendanceStatus: x.AttendanceStatus, Usage: toUsage(x.Usage)})
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

func (h *Handler) mine(w http.ResponseWriter, r *http.Request) {
	termID := r.URL.Query().Get("term_id")
	if termID != "" && !httpx.ValidUUID(termID) {
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "term_id", Message: "Geçerli bir UUID olmalı."}})
		return
	}
	if termID == "" {
		err := h.svc.pool.QueryRow(r.Context(), `SELECT id FROM academic.terms WHERE is_current`).Scan(&termID)
		if errors.Is(err, pgx.ErrNoRows) {
			h.writeJSON(w, r, http.StatusOK, httpx.ListResponse[courseAttendanceDTO]{Items: []courseAttendanceDTO{}})
			return
		}
		if h.failed(w, r, err) {
			return
		}
	}
	list, err := h.svc.Mine(r.Context(), userID(r), termID)
	if h.failed(w, r, err) {
		return
	}
	res := httpx.ListResponse[courseAttendanceDTO]{Items: make([]courseAttendanceDTO, 0, len(list))}
	for _, c := range list {
		d := courseAttendanceDTO{EnrollmentID: c.EnrollmentID, SectionID: c.SectionID, SectionCode: c.SectionCode, CourseCode: c.CourseCode,
			CourseNameTR: c.CourseNameTR, CourseNameEN: c.CourseNameEN, AttendanceStatus: c.AttendanceStatus, Usage: toUsage(c.Usage),
			Sessions: make([]studentSessionDTO, 0, len(c.Sessions))}
		for _, x := range c.Sessions {
			d.Sessions = append(d.Sessions, studentSessionDTO{ID: x.ID, Type: x.Type, Kind: x.Kind, Status: x.Status, StartsAt: x.StartsAt,
				EndsAt: x.EndsAt, Topic: x.Topic, Record: optional(x.Record)})
		}
		res.Items = append(res.Items, d)
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

// --- Yardımcılar --------------------------------------------------------------------------

func (h *Handler) writeJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	if err := httpx.WriteJSON(w, status, v); err != nil {
		h.logger.Error("yanıt yazılamadı", "err", err, "request_id", httpx.RequestIDFrom(r.Context()))
	}
}

func (h *Handler) failed(w http.ResponseWriter, r *http.Request, err error) bool {
	var verr *ValidationError
	switch {
	case err == nil:
		return false
	case errors.As(err, &verr):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: verr.Field, Message: verr.Message}})
	case errors.Is(err, ErrNotFound):
		httpx.NotFound(w, r)
	case errors.Is(err, ErrForbidden):
		_ = httpx.WriteProblem(w, r, httpx.Problem{Status: http.StatusForbidden, Code: "FORBIDDEN",
			Detail: "Bu şubenin yoklamasını sadece şubenin öğretim elemanları alabilir."})
	case errors.Is(err, ErrVersionMismatch):
		httpx.PreconditionFailed(w, r)
	case errors.Is(err, ErrCancelled):
		_ = httpx.WriteProblem(w, r, httpx.Problem{Status: http.StatusConflict, Code: "SESSION_CANCELLED",
			Detail: "İptal edilmiş oturumun yoklaması alınamaz."})
	case errors.Is(err, ErrHeld):
		_ = httpx.WriteProblem(w, r, httpx.Problem{Status: http.StatusConflict, Code: "SESSION_HELD",
			Detail: "Yoklaması alınmış oturum iptal edilemez."})
	default:
		h.logger.Error("yoklama isteği başarısız", "err", err, "request_id", httpx.RequestIDFrom(r.Context()))
		httpx.InternalServerError(w, r)
	}
	return true
}

func userID(r *http.Request) string {
	p, _ := authn.PrincipalFrom(r.Context())
	return p.UserID
}
