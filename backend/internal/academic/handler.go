package academic

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/CAPELLAX02/agora/backend/internal/org"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

const (
	permRead   = "calendar:read"
	permManage = "calendar:manage"
)

// Store, akademik takvim işlemleridir. *Repository bunu sağlar.
type Store interface {
	AcademicYears(ctx context.Context) ([]AcademicYear, error)
	Terms(ctx context.Context) ([]Term, error)
	Term(ctx context.Context, id string) (Term, error)
	CurrentTerm(ctx context.Context) (Term, error)
	CreateAcademicYear(ctx context.Context, actorID string, y AcademicYear) (string, error)
	CreateTerm(ctx context.Context, actorID, yearID string, in TermInput) (string, error)
	UpdateTerm(ctx context.Context, actorID, id string, version int, in TermInput) error
	MakeCurrent(ctx context.Context, actorID, id string) error
	EventTypes(ctx context.Context) ([]EventType, error)
	Events(ctx context.Context, f EventFilter) ([]Event, error)
	Event(ctx context.Context, id string) (Event, error)
	CreateEvent(ctx context.Context, actorID, termID string, in EventInput) (string, error)
	UpdateEvent(ctx context.Context, actorID, id string, version int, in EventInput) error
	DeleteEvent(ctx context.Context, actorID, id string) error
	Windows(ctx context.Context, termID string, target WindowTarget, at time.Time) ([]Window, error)
}

// TargetResolver, birimlerin yetki hedeflerini çözer. *org.Targets bunu sağlar.
type TargetResolver interface {
	Faculty(ctx context.Context, id string) (authz.Target, error)
	Program(ctx context.Context, id string) (authz.Target, error)
}

// Handler, akademik takvim uçlarını sunar.
type Handler struct {
	store   Store
	targets TargetResolver
	logger  *slog.Logger
	now     func() time.Time
}

// NewHandler, bir Handler oluşturur.
func NewHandler(store Store, targets TargetResolver, logger *slog.Logger, now func() time.Time) *Handler {
	return &Handler{store: store, targets: targets, logger: logger, now: now}
}

// Register, route'ları kaydeder.
func (h *Handler) Register(rt *authz.Router) {
	rt.HandleFunc("GET /api/v1/academic-years", authz.Permission(permRead), h.listYears)
	rt.HandleFunc("POST /api/v1/academic-years", authz.Permission(permManage), h.createYear)
	rt.HandleFunc("POST /api/v1/academic-years/{id}/terms", authz.Permission(permManage), h.createTerm)

	rt.HandleFunc("GET /api/v1/terms", authz.Permission(permRead), h.listTerms)
	rt.HandleFunc("GET /api/v1/terms/current", authz.Permission(permRead), h.currentTerm)
	rt.HandleFunc("GET /api/v1/terms/{id}", authz.Permission(permRead), h.getTerm)
	rt.HandleFunc("PUT /api/v1/terms/{id}", authz.Permission(permManage), h.updateTerm)
	rt.HandleFunc("POST /api/v1/terms/{id}/current", authz.Permission(permManage), h.makeCurrent)

	rt.HandleFunc("GET /api/v1/calendar/event-types", authz.Permission(permRead), h.listEventTypes)
	rt.HandleFunc("GET /api/v1/calendar/windows", authz.Permission(permRead), h.windows)
	rt.HandleFunc("GET /api/v1/terms/{id}/events", authz.Permission(permRead), h.listEvents)
	rt.HandleFunc("POST /api/v1/terms/{id}/events", authz.Permission(permManage), h.createEvent)
	rt.HandleFunc("GET /api/v1/calendar-events/{id}", authz.Permission(permRead), h.getEvent)
	rt.HandleFunc("PUT /api/v1/calendar-events/{id}", authz.Permission(permManage), h.updateEvent)
	rt.HandleFunc("DELETE /api/v1/calendar-events/{id}", authz.Permission(permManage), h.deleteEvent)
}

// --- Yanıt tipleri -------------------------------------------------------------

type termResponse struct {
	ID           string `json:"id"`
	Code         string `json:"code"`
	AcademicYear string `json:"academic_year"`
	StartYear    int    `json:"start_year"`
	TermType     string `json:"term_type"`
	StartsOn     string `json:"starts_on"`
	EndsOn       string `json:"ends_on"`
	Status       string `json:"status"`
	IsCurrent    bool   `json:"is_current"`
	Version      int    `json:"version"`
}

type yearResponse struct {
	ID        string         `json:"id"`
	StartYear int            `json:"start_year"`
	Label     string         `json:"label"`
	StartsOn  string         `json:"starts_on"`
	EndsOn    string         `json:"ends_on"`
	Terms     []termResponse `json:"terms"`
}

type eventTypeResponse struct {
	Code           string `json:"code"`
	NameTR         string `json:"name_tr"`
	NameEN         string `json:"name_en"`
	Category       string `json:"category"`
	IsActionWindow bool   `json:"is_action_window"`
}

type scopeRefResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type eventResponse struct {
	ID          string            `json:"id"`
	TermID      string            `json:"term_id"`
	Type        eventTypeResponse `json:"type"`
	TitleTR     *string           `json:"title_tr"`
	TitleEN     *string           `json:"title_en"`
	StartsAt    time.Time         `json:"starts_at"`
	EndsAt      time.Time         `json:"ends_at"`
	ScopeType   string            `json:"scope_type"`
	Scope       *scopeRefResponse `json:"scope"`
	IsPublished bool              `json:"is_published"`
	Note        *string           `json:"note"`
	Version     int               `json:"version"`
}

type windowResponse struct {
	Type      eventTypeResponse `json:"type"`
	ScopeType *string           `json:"scope_type"`
	Open      bool              `json:"open"`
	Current   *eventResponse    `json:"current"`
	Next      *eventResponse    `json:"next"`
	Events    []eventResponse   `json:"events"`
}

type windowsResponse struct {
	Term  termResponse     `json:"term"`
	At    time.Time        `json:"at"`
	Items []windowResponse `json:"items"`
}

func toTermResponse(t Term) termResponse {
	return termResponse{
		ID: t.ID, Code: t.Code, AcademicYear: yearLabel(t.StartYear), StartYear: t.StartYear,
		TermType: string(t.Type), StartsOn: dateOnly(t.StartsOn), EndsOn: dateOnly(t.EndsOn),
		Status: string(t.Status), IsCurrent: t.IsCurrent, Version: t.Version,
	}
}

func toEventTypeResponse(t EventType) eventTypeResponse {
	return eventTypeResponse{Code: t.Code, NameTR: t.NameTR, NameEN: t.NameEN, Category: t.Category, IsActionWindow: t.IsActionWindow}
}

func toEventResponse(e Event) eventResponse {
	res := eventResponse{
		ID: e.ID, TermID: e.TermID, Type: toEventTypeResponse(e.Type),
		TitleTR: optional(e.TitleTR), TitleEN: optional(e.TitleEN),
		StartsAt: e.StartsAt, EndsAt: e.EndsAt, ScopeType: string(e.ScopeType),
		IsPublished: e.IsPublished, Note: optional(e.Note), Version: e.Version,
	}
	if e.ScopeID != "" {
		res.Scope = &scopeRefResponse{ID: e.ScopeID, Name: e.ScopeName}
	}
	return res
}

// --- Yıllar ve dönemler --------------------------------------------------------

func (h *Handler) listYears(w http.ResponseWriter, r *http.Request) {
	years, err := h.store.AcademicYears(r.Context())
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	terms, err := h.store.Terms(r.Context())
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	res := httpx.ListResponse[yearResponse]{Items: make([]yearResponse, 0, len(years))}
	for _, y := range years {
		yr := yearResponse{
			ID: y.ID, StartYear: y.StartYear, Label: y.Label(),
			StartsOn: dateOnly(y.StartsOn), EndsOn: dateOnly(y.EndsOn), Terms: []termResponse{},
		}
		for _, t := range terms {
			if t.AcademicYearID == y.ID {
				yr.Terms = append(yr.Terms, toTermResponse(t))
			}
		}
		res.Items = append(res.Items, yr)
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

type yearRequest struct {
	StartYear int    `json:"start_year"`
	StartsOn  string `json:"starts_on"`
	EndsOn    string `json:"ends_on"`
}

func (h *Handler) createYear(w http.ResponseWriter, r *http.Request) {
	var req yearRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	var errs []httpx.FieldError
	if req.StartYear < 1946 || req.StartYear > 2100 {
		errs = append(errs, httpx.FieldError{Field: "start_year", Message: "1946 ile 2100 arasında olmalı."})
	}
	starts, ends, dateErrs := parsePeriod(req.StartsOn, req.EndsOn)
	errs = append(errs, dateErrs...)
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	if !h.allowed(w, r, ScopeUniversity, "") {
		return
	}
	id, err := h.store.CreateAcademicYear(r.Context(), actorID(r), AcademicYear{StartYear: req.StartYear, StartsOn: starts, EndsOn: ends})
	if errors.Is(err, ErrConflict) {
		h.problem(w, r, http.StatusConflict, "ACADEMIC_YEAR_EXISTS", "Bu akademik yıl zaten tanımlı.")
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/academic-years")
	h.writeJSON(w, r, http.StatusCreated, yearResponse{
		ID: id, StartYear: req.StartYear, Label: yearLabel(req.StartYear),
		StartsOn: dateOnly(starts), EndsOn: dateOnly(ends), Terms: []termResponse{},
	})
}

type termRequest struct {
	TermType string `json:"term_type"`
	StartsOn string `json:"starts_on"`
	EndsOn   string `json:"ends_on"`
	Status   string `json:"status"`
}

func (h *Handler) createTerm(w http.ResponseWriter, r *http.Request) {
	yearID := r.PathValue("id")
	if !httpx.ValidUUID(yearID) {
		httpx.NotFound(w, r)
		return
	}
	var req termRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	var errs []httpx.FieldError
	if !TermType(req.TermType).Valid() {
		errs = append(errs, httpx.FieldError{Field: "term_type", Message: "FALL, SPRING ya da SUMMER olmalı."})
	}
	starts, ends, dateErrs := parsePeriod(req.StartsOn, req.EndsOn)
	errs = append(errs, dateErrs...)
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	if !h.allowed(w, r, ScopeUniversity, "") {
		return
	}
	id, err := h.store.CreateTerm(r.Context(), actorID(r), yearID, TermInput{Type: TermType(req.TermType), StartsOn: starts, EndsOn: ends})
	if h.termWriteFailed(w, r, err) {
		return
	}
	t, err := h.store.Term(r.Context(), id)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/terms/"+id)
	w.Header().Set("ETag", httpx.ETag(t.Version))
	h.writeJSON(w, r, http.StatusCreated, toTermResponse(t))
}

func (h *Handler) listTerms(w http.ResponseWriter, r *http.Request) {
	terms, err := h.store.Terms(r.Context())
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	res := httpx.ListResponse[termResponse]{Items: make([]termResponse, 0, len(terms))}
	for _, t := range terms {
		res.Items = append(res.Items, toTermResponse(t))
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

func (h *Handler) currentTerm(w http.ResponseWriter, r *http.Request) {
	t, err := h.store.CurrentTerm(r.Context())
	if errors.Is(err, ErrNoCurrentTerm) {
		h.problem(w, r, http.StatusNotFound, "NO_CURRENT_TERM", "Aktif dönem tanımlı değil.")
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	w.Header().Set("ETag", httpx.ETag(t.Version))
	h.writeJSON(w, r, http.StatusOK, toTermResponse(t))
}

func (h *Handler) getTerm(w http.ResponseWriter, r *http.Request) {
	t, ok := h.term(w, r)
	if !ok {
		return
	}
	w.Header().Set("ETag", httpx.ETag(t.Version))
	h.writeJSON(w, r, http.StatusOK, toTermResponse(t))
}

func (h *Handler) updateTerm(w http.ResponseWriter, r *http.Request) {
	version, ok := httpx.IfMatchVersion(r)
	if !ok {
		httpx.PreconditionRequired(w, r)
		return
	}
	t, ok := h.term(w, r)
	if !ok {
		return
	}
	var req termRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	var errs []httpx.FieldError
	if req.TermType != "" {
		errs = append(errs, httpx.FieldError{Field: "term_type", Message: "Dönem türü değiştirilemez."})
	}
	if !TermStatus(req.Status).Valid() {
		errs = append(errs, httpx.FieldError{Field: "status", Message: "PLANNED, ACTIVE ya da CLOSED olmalı."})
	}
	starts, ends, dateErrs := parsePeriod(req.StartsOn, req.EndsOn)
	errs = append(errs, dateErrs...)
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	if !h.allowed(w, r, ScopeUniversity, "") {
		return
	}
	err := h.store.UpdateTerm(r.Context(), actorID(r), t.ID, version, TermInput{StartsOn: starts, EndsOn: ends, Status: TermStatus(req.Status)})
	if h.termWriteFailed(w, r, err) {
		return
	}
	h.getTerm(w, r)
}

func (h *Handler) makeCurrent(w http.ResponseWriter, r *http.Request) {
	t, ok := h.term(w, r)
	if !ok {
		return
	}
	// Yıllar ve dönemler üniversite geneli tanımlardır: birim kapsamlı yetki yetmez.
	if !h.allowed(w, r, ScopeUniversity, "") {
		return
	}
	if h.termWriteFailed(w, r, h.store.MakeCurrent(r.Context(), actorID(r), t.ID)) {
		return
	}
	h.getTerm(w, r)
}

func (h *Handler) termWriteFailed(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, ErrNotFound):
		httpx.NotFound(w, r)
	case errors.Is(err, ErrVersionMismatch):
		httpx.PreconditionFailed(w, r)
	case errors.Is(err, ErrConflict):
		h.problem(w, r, http.StatusConflict, "TERM_EXISTS", "Bu akademik yılda bu dönem zaten tanımlı.")
	case errors.Is(err, ErrOutsideYear):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "starts_on", Message: "Dönem akademik yılın tarihleri içinde olmalı."}})
	case errors.Is(err, ErrTermClosed):
		h.problem(w, r, http.StatusConflict, "TERM_CLOSED", "Kapanmış bir dönem aktif dönem yapılamaz.")
	default:
		h.serverError(w, r, err)
	}
	return true
}

func (h *Handler) term(w http.ResponseWriter, r *http.Request) (Term, bool) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return Term{}, false
	}
	t, err := h.store.Term(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.NotFound(w, r)
		return Term{}, false
	}
	if err != nil {
		h.serverError(w, r, err)
		return Term{}, false
	}
	return t, true
}

// --- Olaylar -------------------------------------------------------------------

func (h *Handler) listEventTypes(w http.ResponseWriter, r *http.Request) {
	types, err := h.store.EventTypes(r.Context())
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	res := httpx.ListResponse[eventTypeResponse]{Items: make([]eventTypeResponse, 0, len(types))}
	for _, t := range types {
		res.Items = append(res.Items, toEventTypeResponse(t))
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

func (h *Handler) listEvents(w http.ResponseWriter, r *http.Request) {
	t, ok := h.term(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	target, ok := h.targetParams(w, r, q.Get("faculty_id"), q.Get("program_id"))
	if !ok {
		return
	}
	perms, _ := authz.PermissionsFrom(r.Context())
	events, err := h.store.Events(r.Context(), EventFilter{
		TermID: t.ID, TypeCode: q.Get("type"), Target: target,
		// Yayımlanmamış (taslak) olayları sadece takvimi yönetenler görür.
		IncludeUnpublished: perms != nil && perms.Has(permManage),
	})
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	res := httpx.ListResponse[eventResponse]{Items: make([]eventResponse, 0, len(events))}
	for _, e := range events {
		res.Items = append(res.Items, toEventResponse(e))
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

type eventRequest struct {
	Type        string `json:"type"`
	TitleTR     string `json:"title_tr"`
	TitleEN     string `json:"title_en"`
	StartsAt    string `json:"starts_at"`
	EndsAt      string `json:"ends_at"`
	ScopeType   string `json:"scope_type"`
	ScopeID     string `json:"scope_id"`
	IsPublished *bool  `json:"is_published"`
	Note        string `json:"note"`
}

func (req eventRequest) validate(creating bool) (EventInput, []httpx.FieldError) {
	var errs []httpx.FieldError
	in := EventInput{
		TypeCode: req.Type, TitleTR: strings.TrimSpace(req.TitleTR), TitleEN: strings.TrimSpace(req.TitleEN),
		ScopeType: ScopeType(req.ScopeType), ScopeID: req.ScopeID, IsPublished: req.IsPublished == nil || *req.IsPublished,
		Note: strings.TrimSpace(req.Note),
	}
	if creating && req.Type == "" {
		errs = append(errs, httpx.FieldError{Field: "type", Message: "Olay türü zorunlu."})
	}
	if !creating && req.Type != "" {
		errs = append(errs, httpx.FieldError{Field: "type", Message: "Olay türü değiştirilemez."})
	}
	var err error
	if in.StartsAt, err = time.Parse(time.RFC3339, req.StartsAt); err != nil {
		errs = append(errs, httpx.FieldError{Field: "starts_at", Message: "RFC 3339 biçiminde bir zaman olmalı."})
	}
	if in.EndsAt, err = time.Parse(time.RFC3339, req.EndsAt); err != nil {
		errs = append(errs, httpx.FieldError{Field: "ends_at", Message: "RFC 3339 biçiminde bir zaman olmalı."})
	}
	if !in.StartsAt.IsZero() && !in.EndsAt.IsZero() && !in.EndsAt.After(in.StartsAt) {
		errs = append(errs, httpx.FieldError{Field: "ends_at", Message: "Bitiş başlangıçtan sonra olmalı."})
	}
	switch {
	case !in.ScopeType.Valid():
		errs = append(errs, httpx.FieldError{Field: "scope_type", Message: "UNIVERSITY, FACULTY ya da PROGRAM olmalı."})
	case in.ScopeType == ScopeUniversity && in.ScopeID != "":
		errs = append(errs, httpx.FieldError{Field: "scope_id", Message: "Üniversite kapsamında boş bırakılmalı."})
	case in.ScopeType != ScopeUniversity && !httpx.ValidUUID(in.ScopeID):
		errs = append(errs, httpx.FieldError{Field: "scope_id", Message: "Birim ya da program zorunlu."})
	}
	for field, v := range map[string]string{"title_tr": in.TitleTR, "title_en": in.TitleEN} {
		if utf8.RuneCountInString(v) > 200 {
			errs = append(errs, httpx.FieldError{Field: field, Message: "En fazla 200 karakter."})
		}
	}
	if utf8.RuneCountInString(in.Note) > 1000 {
		errs = append(errs, httpx.FieldError{Field: "note", Message: "En fazla 1000 karakter."})
	}
	return in, errs
}

func (h *Handler) createEvent(w http.ResponseWriter, r *http.Request) {
	t, ok := h.term(w, r)
	if !ok {
		return
	}
	var req eventRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	in, errs := req.validate(true)
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	if !h.allowed(w, r, in.ScopeType, in.ScopeID) {
		return
	}
	id, err := h.store.CreateEvent(r.Context(), actorID(r), t.ID, in)
	if h.eventWriteFailed(w, r, err) {
		return
	}
	e, err := h.store.Event(r.Context(), id)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/calendar-events/"+id)
	w.Header().Set("ETag", httpx.ETag(e.Version))
	h.writeJSON(w, r, http.StatusCreated, toEventResponse(e))
}

func (h *Handler) getEvent(w http.ResponseWriter, r *http.Request) {
	e, ok := h.event(w, r)
	if !ok {
		return
	}
	perms, _ := authz.PermissionsFrom(r.Context())
	if !e.IsPublished && (perms == nil || !perms.Has(permManage)) {
		httpx.NotFound(w, r)
		return
	}
	w.Header().Set("ETag", httpx.ETag(e.Version))
	h.writeJSON(w, r, http.StatusOK, toEventResponse(e))
}

func (h *Handler) updateEvent(w http.ResponseWriter, r *http.Request) {
	version, ok := httpx.IfMatchVersion(r)
	if !ok {
		httpx.PreconditionRequired(w, r)
		return
	}
	current, ok := h.event(w, r)
	if !ok {
		return
	}
	var req eventRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	in, errs := req.validate(false)
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	// Hem mevcut hem yeni kapsam yetki alanında olmalı.
	if !h.allowed(w, r, current.ScopeType, current.ScopeID) || !h.allowed(w, r, in.ScopeType, in.ScopeID) {
		return
	}
	if h.eventWriteFailed(w, r, h.store.UpdateEvent(r.Context(), actorID(r), current.ID, version, in)) {
		return
	}
	h.getEvent(w, r)
}

func (h *Handler) deleteEvent(w http.ResponseWriter, r *http.Request) {
	current, ok := h.event(w, r)
	if !ok {
		return
	}
	if !h.allowed(w, r, current.ScopeType, current.ScopeID) {
		return
	}
	if h.eventWriteFailed(w, r, h.store.DeleteEvent(r.Context(), actorID(r), current.ID)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) eventWriteFailed(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, ErrNotFound):
		httpx.NotFound(w, r)
	case errors.Is(err, ErrVersionMismatch):
		httpx.PreconditionFailed(w, r)
	case errors.Is(err, ErrEventOverlap):
		h.problem(w, r, http.StatusConflict, "EVENT_OVERLAP",
			"Bu kapsamda aynı türden zaman olarak çakışan bir olay var.")
	case errors.Is(err, ErrUnknownEventType):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "type", Message: "Olay türü bulunamadı."}})
	case errors.Is(err, ErrUnknownScope):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "scope_id", Message: "Birim ya da program bulunamadı."}})
	default:
		h.serverError(w, r, err)
	}
	return true
}

func (h *Handler) event(w http.ResponseWriter, r *http.Request) (Event, bool) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return Event{}, false
	}
	e, err := h.store.Event(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.NotFound(w, r)
		return Event{}, false
	}
	if err != nil {
		h.serverError(w, r, err)
		return Event{}, false
	}
	return e, true
}

// --- Pencereler ----------------------------------------------------------------

// windows, işlem pencerelerinin hedefe göre çözümlenmiş durumudur: "bu program için ders
// seçme açık mı, ne zaman açılacak?". Dönem verilmezse aktif dönem kullanılır.
func (h *Handler) windows(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var (
		t   Term
		err error
	)
	if id := q.Get("term_id"); id != "" {
		if !httpx.ValidUUID(id) {
			httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "term_id", Message: "Geçerli bir UUID olmalı."}})
			return
		}
		t, err = h.store.Term(r.Context(), id)
	} else {
		t, err = h.store.CurrentTerm(r.Context())
	}
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "term_id", Message: "Dönem bulunamadı."}})
		return
	case errors.Is(err, ErrNoCurrentTerm):
		h.problem(w, r, http.StatusNotFound, "NO_CURRENT_TERM", "Aktif dönem tanımlı değil.")
		return
	case err != nil:
		h.serverError(w, r, err)
		return
	}
	target, ok := h.targetParams(w, r, q.Get("faculty_id"), q.Get("program_id"))
	if !ok {
		return
	}
	if target == nil {
		target = &WindowTarget{}
	}
	at := h.now()
	wins, err := h.store.Windows(r.Context(), t.ID, *target, at)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	res := windowsResponse{Term: toTermResponse(t), At: at.UTC(), Items: make([]windowResponse, 0, len(wins))}
	for _, win := range wins {
		wr := windowResponse{Type: toEventTypeResponse(win.Type), Open: win.Open, Events: make([]eventResponse, 0, len(win.Events))}
		if win.Scope != "" {
			s := string(win.Scope)
			wr.ScopeType = &s
		}
		if win.Current != nil {
			e := toEventResponse(*win.Current)
			wr.Current = &e
		}
		if win.Next != nil {
			e := toEventResponse(*win.Next)
			wr.Next = &e
		}
		for _, e := range win.Events {
			wr.Events = append(wr.Events, toEventResponse(e))
		}
		res.Items = append(res.Items, wr)
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

// targetParams, birim ve program sorgu parametrelerini hedefe çevirir. Program verilirse
// birimi programdan bulunur. İkisi de boşsa nil döner (sadece üniversite kapsamı).
func (h *Handler) targetParams(w http.ResponseWriter, r *http.Request, facultyID, programID string) (*WindowTarget, bool) {
	switch {
	case programID != "":
		if !httpx.ValidUUID(programID) {
			httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "program_id", Message: "Geçerli bir UUID olmalı."}})
			return nil, false
		}
		t, err := h.targets.Program(r.Context(), programID)
		if errors.Is(err, org.ErrNotFound) {
			httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "program_id", Message: "Program bulunamadı."}})
			return nil, false
		}
		if err != nil {
			h.serverError(w, r, err)
			return nil, false
		}
		return &WindowTarget{FacultyID: t.FacultyID, ProgramID: programID}, true
	case facultyID != "":
		if !httpx.ValidUUID(facultyID) {
			httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "faculty_id", Message: "Geçerli bir UUID olmalı."}})
			return nil, false
		}
		return &WindowTarget{FacultyID: facultyID}, true
	}
	return nil, true
}

// allowed, calendar:manage yetkisinin olayın kapsamını kapsayıp kapsamadığına bakar.
// Üniversite kapsamlı bir olayı sadece üniversite genelinde yetkisi olan yönetebilir.
func (h *Handler) allowed(w http.ResponseWriter, r *http.Request, scope ScopeType, scopeID string) bool {
	var target authz.Target
	switch scope {
	case ScopeFaculty:
		target.FacultyID = scopeID
	case ScopeProgram:
		t, err := h.targets.Program(r.Context(), scopeID)
		if errors.Is(err, org.ErrNotFound) {
			httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "scope_id", Message: "Program bulunamadı."}})
			return false
		}
		if err != nil {
			h.serverError(w, r, err)
			return false
		}
		target = t
	}
	perms, ok := authz.PermissionsFrom(r.Context())
	if ok && perms.Allows(permManage, target) {
		return true
	}
	h.problem(w, r, http.StatusForbidden, "FORBIDDEN", "Bu kapsamın takvimini yönetme yetkiniz yok.")
	return false
}

// --- Yardımcılar ---------------------------------------------------------------

// parsePeriod, YYYY-MM-DD biçimindeki başlangıç ve bitiş tarihlerini okur.
func parsePeriod(startsOn, endsOn string) (time.Time, time.Time, []httpx.FieldError) {
	var errs []httpx.FieldError
	starts, err := time.Parse(time.DateOnly, startsOn)
	if err != nil {
		errs = append(errs, httpx.FieldError{Field: "starts_on", Message: "YYYY-AA-GG biçiminde bir tarih olmalı."})
	}
	ends, err := time.Parse(time.DateOnly, endsOn)
	if err != nil {
		errs = append(errs, httpx.FieldError{Field: "ends_on", Message: "YYYY-AA-GG biçiminde bir tarih olmalı."})
	}
	if len(errs) == 0 && !ends.After(starts) {
		errs = append(errs, httpx.FieldError{Field: "ends_on", Message: "Bitiş başlangıçtan sonra olmalı."})
	}
	return starts, ends, errs
}

func actorID(r *http.Request) string {
	p, _ := authn.PrincipalFrom(r.Context())
	return p.UserID
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (h *Handler) problem(w http.ResponseWriter, r *http.Request, status int, code, detail string) {
	_ = httpx.WriteProblem(w, r, httpx.Problem{Status: status, Code: code, Detail: detail})
}

func (h *Handler) writeJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	if err := httpx.WriteJSON(w, status, v); err != nil {
		h.logger.Error("yanıt yazılamadı", "err", err, "request_id", httpx.RequestIDFrom(r.Context()))
	}
}

func (h *Handler) serverError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.Error("beklenmeyen hata", "err", err, "path", r.URL.Path, "request_id", httpx.RequestIDFrom(r.Context()))
	httpx.InternalServerError(w, r)
}
