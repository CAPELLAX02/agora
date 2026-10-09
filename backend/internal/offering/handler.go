package offering

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/CAPELLAX02/agora/backend/internal/org"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

const (
	permRead     = "course:read" // açılan dersler ve haftalık program katalog gibi herkese açıktır
	permOffering = "offering:manage"
	permSection  = "section:manage"
	permQuota    = "quota:manage"
	permSchedule = "schedule:manage"
)

// Store, ders açma işlemleridir. *Repository bunu sağlar.
type Store interface {
	TermExists(ctx context.Context, id string) (bool, error)
	CurrentTermID(ctx context.Context) (string, error)
	ListOfferings(ctx context.Context, f OfferingFilter) ([]Offering, bool, error)
	Offering(ctx context.Context, id string) (Offering, error)
	CreateOffering(ctx context.Context, actorID, termID string, in OfferingInput) (string, error)
	UpdateOffering(ctx context.Context, actorID, id string, version int, in OfferingUpdate) error
	DeleteOffering(ctx context.Context, actorID, id string) error

	Sections(ctx context.Context, offeringID string) ([]Section, error)
	Section(ctx context.Context, id string) (Section, error)
	CreateSection(ctx context.Context, actorID, offeringID string, in SectionInput) (string, error)
	UpdateSection(ctx context.Context, actorID, id string, version int, in SectionInput) error
	DeleteSection(ctx context.Context, actorID, id string) error
	SetInstructors(ctx context.Context, actorID, sectionID string, items []InstructorInput) error
	SetQuotas(ctx context.Context, actorID, sectionID string, items []QuotaInput) error

	Slot(ctx context.Context, id string) (Slot, error)
	AddSlot(ctx context.Context, actorID, sectionID string, in SlotInput) (string, error)
	UpdateSlot(ctx context.Context, actorID, slotID string, in SlotInput) error
	DeleteSlot(ctx context.Context, actorID, slotID string) error

	Schedule(ctx context.Context, f ScheduleFilter) ([]ScheduleEntry, error)
	StaffByUser(ctx context.Context, userID string) (string, error)
	SearchInstructors(ctx context.Context, query, departmentID string, limit int) ([]StaffSummary, error)
}

// TargetResolver, bölümlerin yetki hedeflerini çözer. *org.Targets bunu sağlar.
type TargetResolver interface {
	Department(ctx context.Context, id string) (authz.Target, error)
}

// Handler, ders açma uçlarını sunar.
type Handler struct {
	store   Store
	targets TargetResolver
	logger  *slog.Logger
}

// NewHandler, bir Handler oluşturur.
func NewHandler(store Store, targets TargetResolver, logger *slog.Logger) *Handler {
	return &Handler{store: store, targets: targets, logger: logger}
}

// Register, route'ları kaydeder.
func (h *Handler) Register(rt *authz.Router) {
	rt.HandleFunc("GET /api/v1/terms/{id}/offerings", authz.Permission(permRead), h.listOfferings)
	rt.HandleFunc("POST /api/v1/terms/{id}/offerings", authz.Permission(permOffering), h.createOffering)
	rt.HandleFunc("GET /api/v1/offerings/{id}", authz.Permission(permRead), h.getOffering)
	rt.HandleFunc("PUT /api/v1/offerings/{id}", authz.Permission(permOffering), h.updateOffering)
	rt.HandleFunc("DELETE /api/v1/offerings/{id}", authz.Permission(permOffering), h.deleteOffering)

	rt.HandleFunc("POST /api/v1/offerings/{id}/sections", authz.Permission(permSection), h.createSection)
	rt.HandleFunc("GET /api/v1/sections/{id}", authz.Permission(permRead), h.getSection)
	rt.HandleFunc("PUT /api/v1/sections/{id}", authz.Permission(permSection), h.updateSection)
	rt.HandleFunc("DELETE /api/v1/sections/{id}", authz.Permission(permSection), h.deleteSection)
	rt.HandleFunc("PUT /api/v1/sections/{id}/instructors", authz.Permission(permSection), h.setInstructors)
	rt.HandleFunc("PUT /api/v1/sections/{id}/quotas", authz.Permission(permQuota), h.setQuotas)
	rt.HandleFunc("POST /api/v1/sections/{id}/slots", authz.Permission(permSchedule), h.addSlot)
	rt.HandleFunc("PUT /api/v1/schedule-slots/{id}", authz.Permission(permSchedule), h.updateSlot)
	rt.HandleFunc("DELETE /api/v1/schedule-slots/{id}", authz.Permission(permSchedule), h.deleteSlot)

	rt.HandleFunc("GET /api/v1/terms/{id}/schedule", authz.Permission(permRead), h.termSchedule)
	rt.HandleFunc("GET /api/v1/me/teaching", authz.Authenticated, h.myTeaching)
	rt.HandleFunc("GET /api/v1/instructors", authz.Permission(permSection), h.searchInstructors)
}

// --- Yanıt tipleri -------------------------------------------------------------

type refResponse struct {
	ID     string `json:"id"`
	Code   string `json:"code"`
	NameTR string `json:"name_tr"`
	NameEN string `json:"name_en"`
}

type courseResponse struct {
	refResponse
	TheoryHours    int     `json:"theory_hours"`
	PracticeHours  int     `json:"practice_hours"`
	NationalCredit float64 `json:"national_credit"`
	ECTS           float64 `json:"ects"`
	Language       string  `json:"language"`
}

type termRefResponse struct {
	ID   string `json:"id"`
	Code string `json:"code"`
}

type offeringResponse struct {
	ID            string          `json:"id"`
	Term          termRefResponse `json:"term"`
	Course        courseResponse  `json:"course"`
	Department    refResponse     `json:"department"`
	Status        string          `json:"status"`
	ExternalRef   *string         `json:"external_ref"`
	Note          *string         `json:"note"`
	SectionCount  int             `json:"section_count"`
	TotalCapacity int             `json:"total_capacity"`
	TotalEnrolled int             `json:"total_enrolled"`
	Version       int             `json:"version"`
}

type instructorResponse struct {
	StaffID   string  `json:"staff_id"`
	StaffNo   string  `json:"staff_no"`
	Title     *string `json:"title"`
	FirstName string  `json:"first_name"`
	LastName  string  `json:"last_name"`
	Role      string  `json:"role"`
}

type quotaResponse struct {
	Program  refResponse `json:"program"`
	Quota    int         `json:"quota"`
	Enrolled int         `json:"enrolled"`
}

type classroomResponse struct {
	ID           string `json:"id"`
	Code         string `json:"code"`
	Name         string `json:"name"`
	BuildingCode string `json:"building_code"`
	Capacity     int    `json:"capacity"`
}

type slotResponse struct {
	ID          string             `json:"id"`
	DayOfWeek   int                `json:"day_of_week"`
	StartTime   string             `json:"start_time"`
	EndTime     string             `json:"end_time"`
	SessionType string             `json:"session_type"`
	Classroom   *classroomResponse `json:"classroom"`
}

type sectionResponse struct {
	ID              string               `json:"id"`
	OfferingID      string               `json:"offering_id"`
	SectionCode     string               `json:"section_code"`
	Capacity        int                  `json:"capacity"`
	EnrolledCount   int                  `json:"enrolled_count"`
	QuotaMode       string               `json:"quota_mode"`
	InstructionMode string               `json:"instruction_mode"`
	Language        string               `json:"language"`
	Status          string               `json:"status"`
	Instructors     []instructorResponse `json:"instructors"`
	Quotas          []quotaResponse      `json:"quotas"`
	Slots           []slotResponse       `json:"slots"`
	Version         int                  `json:"version"`
}

type offeringDetailResponse struct {
	offeringResponse
	Sections []sectionResponse `json:"sections"`
}

type scheduleEntryResponse struct {
	slotResponse
	TermID      string               `json:"term_id"`
	OfferingID  string               `json:"offering_id"`
	SectionID   string               `json:"section_id"`
	SectionCode string               `json:"section_code"`
	Course      refResponse          `json:"course"`
	Instructors []instructorResponse `json:"instructors"`
}

type staffResponse struct {
	StaffID    string       `json:"staff_id"`
	StaffNo    string       `json:"staff_no"`
	Title      *string      `json:"title"`
	FirstName  string       `json:"first_name"`
	LastName   string       `json:"last_name"`
	Department *refResponse `json:"department"`
}

func toRef(r Ref) refResponse {
	return refResponse(r)
}

func toOfferingResponse(o Offering) offeringResponse {
	return offeringResponse{
		ID: o.ID, Term: termRefResponse{ID: o.TermID, Code: o.TermCode},
		Course: courseResponse{
			refResponse: toRef(o.Course.Ref), TheoryHours: o.Course.TheoryHours, PracticeHours: o.Course.PracticeHours,
			NationalCredit: o.Course.NationalCredit, ECTS: o.Course.ECTS, Language: o.Course.Language,
		},
		Department: toRef(o.Department), Status: string(o.Status),
		ExternalRef: optional(o.ExternalRef), Note: optional(o.Note),
		SectionCount: o.SectionCount, TotalCapacity: o.TotalCapacity, TotalEnrolled: o.TotalEnrolled, Version: o.Version,
	}
}

func toInstructorResponse(i Instructor) instructorResponse {
	return instructorResponse{StaffID: i.StaffID, StaffNo: i.StaffNo, Title: optional(i.Title), FirstName: i.FirstName, LastName: i.LastName, Role: i.Role}
}

func toSlotResponse(s Slot) slotResponse {
	res := slotResponse{ID: s.ID, DayOfWeek: s.DayOfWeek, StartTime: s.Start, EndTime: s.End, SessionType: s.SessionType}
	if s.Classroom != nil {
		c := classroomResponse(*s.Classroom)
		res.Classroom = &c
	}
	return res
}

func toSectionResponse(s Section) sectionResponse {
	return sectionResponse{
		ID: s.ID, OfferingID: s.OfferingID, SectionCode: s.Code, Capacity: s.Capacity, EnrolledCount: s.EnrolledCount,
		QuotaMode: s.QuotaMode, InstructionMode: s.InstructionMode, Language: s.Language, Status: s.Status, Version: s.Version,
		Instructors: mapSlice(s.Instructors, toInstructorResponse),
		Quotas: mapSlice(s.Quotas, func(q Quota) quotaResponse {
			return quotaResponse{Program: toRef(q.Program), Quota: q.Quota, Enrolled: q.Enrolled}
		}),
		Slots: mapSlice(s.Slots, toSlotResponse),
	}
}

func toScheduleEntryResponse(e ScheduleEntry) scheduleEntryResponse {
	return scheduleEntryResponse{
		slotResponse: toSlotResponse(e.Slot), TermID: e.TermID, OfferingID: e.OfferingID, SectionID: e.SectionID,
		SectionCode: e.SectionCode, Course: toRef(e.Course), Instructors: mapSlice(e.Instructors, toInstructorResponse),
	}
}

// --- Açılan dersler ------------------------------------------------------------

func (h *Handler) term(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return "", false
	}
	ok, err := h.store.TermExists(r.Context(), id)
	if err != nil {
		h.serverError(w, r, err)
		return "", false
	}
	if !ok {
		httpx.NotFound(w, r)
		return "", false
	}
	return id, true
}

func (h *Handler) listOfferings(w http.ResponseWriter, r *http.Request) {
	termID, ok := h.term(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	f := OfferingFilter{TermID: termID, DepartmentID: q.Get("department_id"), Query: strings.TrimSpace(q.Get("q")), Status: Status(q.Get("status"))}
	var errs []httpx.FieldError
	if f.DepartmentID != "" && !httpx.ValidUUID(f.DepartmentID) {
		errs = append(errs, httpx.FieldError{Field: "department_id", Message: "Geçerli bir UUID olmalı."})
	}
	if utf8.RuneCountInString(f.Query) > 100 {
		errs = append(errs, httpx.FieldError{Field: "q", Message: "En fazla 100 karakter."})
	}
	if f.Status != "" && !f.Status.Valid() {
		errs = append(errs, httpx.FieldError{Field: "status", Message: "Geçersiz durum."})
	}
	limit, limitErr := httpx.ParseLimit(q)
	if limitErr != nil {
		errs = append(errs, *limitErr)
	}
	f.Limit = limit
	if c := q.Get("cursor"); c != "" {
		after, err := httpx.DecodeCursor[OfferingCursor](c)
		if err != nil || after.Code == "" {
			errs = append(errs, httpx.FieldError{Field: "cursor", Message: "Geçersiz cursor."})
		} else {
			f.After = &after
		}
	}
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	list, more, err := h.store.ListOfferings(r.Context(), f)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	res := httpx.ListResponse[offeringResponse]{Items: mapSlice(list, toOfferingResponse)}
	if more {
		if res.NextCursor, err = httpx.EncodeCursor(OfferingCursor{Code: list[len(list)-1].Course.Code}); err != nil {
			h.serverError(w, r, err)
			return
		}
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

func (h *Handler) loadOffering(w http.ResponseWriter, r *http.Request, id string) (Offering, bool) {
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return Offering{}, false
	}
	o, err := h.store.Offering(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.NotFound(w, r)
		return Offering{}, false
	}
	if err != nil {
		h.serverError(w, r, err)
		return Offering{}, false
	}
	return o, true
}

func (h *Handler) writeOfferingDetail(w http.ResponseWriter, r *http.Request, status int, id string) {
	o, ok := h.loadOffering(w, r, id)
	if !ok {
		return
	}
	sections, err := h.store.Sections(r.Context(), id)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	w.Header().Set("ETag", httpx.ETag(o.Version))
	h.writeJSON(w, r, status, offeringDetailResponse{offeringResponse: toOfferingResponse(o), Sections: mapSlice(sections, toSectionResponse)})
}

func (h *Handler) getOffering(w http.ResponseWriter, r *http.Request) {
	h.writeOfferingDetail(w, r, http.StatusOK, r.PathValue("id"))
}

type offeringRequest struct {
	CourseID     string `json:"course_id"`
	DepartmentID string `json:"department_id"`
	Status       string `json:"status"`
	ExternalRef  string `json:"external_ref"`
	Note         string `json:"note"`
}

func (req offeringRequest) commonErrors() []httpx.FieldError {
	var errs []httpx.FieldError
	if utf8.RuneCountInString(strings.TrimSpace(req.ExternalRef)) > 50 {
		errs = append(errs, httpx.FieldError{Field: "external_ref", Message: "En fazla 50 karakter."})
	}
	if utf8.RuneCountInString(strings.TrimSpace(req.Note)) > 1000 {
		errs = append(errs, httpx.FieldError{Field: "note", Message: "En fazla 1000 karakter."})
	}
	return errs
}

func (h *Handler) createOffering(w http.ResponseWriter, r *http.Request) {
	termID, ok := h.term(w, r)
	if !ok {
		return
	}
	var req offeringRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	errs := req.commonErrors()
	if !httpx.ValidUUID(req.CourseID) {
		errs = append(errs, httpx.FieldError{Field: "course_id", Message: "Zorunlu."})
	}
	if !httpx.ValidUUID(req.DepartmentID) {
		errs = append(errs, httpx.FieldError{Field: "department_id", Message: "Zorunlu."})
	}
	if req.Status != "" {
		errs = append(errs, httpx.FieldError{Field: "status", Message: "Yeni açılan ders planlama durumunda başlar."})
	}
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	target, err := h.targets.Department(r.Context(), req.DepartmentID)
	if errors.Is(err, org.ErrNotFound) {
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "department_id", Message: "Bölüm bulunamadı."}})
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	if !h.allowed(w, r, permOffering, target) {
		return
	}
	id, err := h.store.CreateOffering(r.Context(), actorID(r), termID, OfferingInput{
		CourseID: req.CourseID, DepartmentID: req.DepartmentID,
		ExternalRef: strings.TrimSpace(req.ExternalRef), Note: strings.TrimSpace(req.Note),
	})
	if h.failed(w, r, err, "course_id") {
		return
	}
	w.Header().Set("Location", "/api/v1/offerings/"+id)
	h.writeOfferingDetail(w, r, http.StatusCreated, id)
}

func (h *Handler) updateOffering(w http.ResponseWriter, r *http.Request) {
	version, ok := httpx.IfMatchVersion(r)
	if !ok {
		httpx.PreconditionRequired(w, r)
		return
	}
	o, ok := h.loadOffering(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	var req offeringRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	errs := req.commonErrors()
	if req.CourseID != "" || req.DepartmentID != "" {
		errs = append(errs, httpx.FieldError{Field: "course_id", Message: "Ders ve açan bölüm değiştirilemez."})
	}
	status := Status(req.Status)
	if !status.Valid() {
		errs = append(errs, httpx.FieldError{Field: "status", Message: "PLANNED, OPEN, CLOSED ya da CANCELLED olmalı."})
	}
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	if !h.allowed(w, r, permOffering, offeringTarget(o)) {
		return
	}
	err := h.store.UpdateOffering(r.Context(), actorID(r), o.ID, version, OfferingUpdate{
		Status: status, ExternalRef: strings.TrimSpace(req.ExternalRef), Note: strings.TrimSpace(req.Note),
	})
	if h.failed(w, r, err, "") {
		return
	}
	h.writeOfferingDetail(w, r, http.StatusOK, o.ID)
}

func (h *Handler) deleteOffering(w http.ResponseWriter, r *http.Request) {
	o, ok := h.loadOffering(w, r, r.PathValue("id"))
	if !ok || !h.allowed(w, r, permOffering, offeringTarget(o)) {
		return
	}
	if h.failed(w, r, h.store.DeleteOffering(r.Context(), actorID(r), o.ID), "") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func offeringTarget(o Offering) authz.Target {
	return authz.Target{FacultyID: o.FacultyID, DepartmentID: o.Department.ID}
}

// --- Şubeler -------------------------------------------------------------------

var sectionCodePattern = regexp.MustCompile(`^[A-Z0-9]{1,3}$`)

type sectionRequest struct {
	SectionCode     string `json:"section_code"`
	Capacity        *int   `json:"capacity"`
	QuotaMode       string `json:"quota_mode"`
	InstructionMode string `json:"instruction_mode"`
	Language        string `json:"language"`
	Status          string `json:"status"`
}

// validate, isteği doğrular. Boş bırakılan alanlar defaults'tan alınır (oluşturmada
// dersin dili ve varsayılanlar, güncellemede şubenin mevcut değerleri).
func (req sectionRequest) validate(creating bool, defaults SectionInput) (SectionInput, []httpx.FieldError) {
	in := SectionInput{
		Code: strings.ToUpper(strings.TrimSpace(req.SectionCode)), QuotaMode: cmpOr(req.QuotaMode, defaults.QuotaMode),
		InstructionMode: cmpOr(req.InstructionMode, defaults.InstructionMode), Language: cmpOr(req.Language, defaults.Language),
		Status: cmpOr(req.Status, defaults.Status), Capacity: defaults.Capacity,
	}
	var errs []httpx.FieldError
	switch {
	case creating && !sectionCodePattern.MatchString(in.Code):
		errs = append(errs, httpx.FieldError{Field: "section_code", Message: "1-3 büyük harf ya da rakam olmalı (ör. 1, A)."})
	case !creating && req.SectionCode != "":
		errs = append(errs, httpx.FieldError{Field: "section_code", Message: "Şube kodu değiştirilemez."})
	}
	switch {
	case req.Capacity != nil:
		in.Capacity = *req.Capacity
	case creating:
		errs = append(errs, httpx.FieldError{Field: "capacity", Message: "Zorunlu."})
	}
	if in.Capacity < 0 || in.Capacity > 2000 {
		errs = append(errs, httpx.FieldError{Field: "capacity", Message: "0 ile 2000 arasında olmalı."})
	}
	if in.QuotaMode != "OPEN" && in.QuotaMode != "RESERVED" {
		errs = append(errs, httpx.FieldError{Field: "quota_mode", Message: "OPEN ya da RESERVED olmalı."})
	}
	if in.InstructionMode != "IN_PERSON" && in.InstructionMode != "ONLINE" && in.InstructionMode != "HYBRID" {
		errs = append(errs, httpx.FieldError{Field: "instruction_mode", Message: "IN_PERSON, ONLINE ya da HYBRID olmalı."})
	}
	if in.Language != "TR" && in.Language != "EN" {
		errs = append(errs, httpx.FieldError{Field: "language", Message: "TR ya da EN olmalı."})
	}
	switch {
	case creating && req.Status != "":
		errs = append(errs, httpx.FieldError{Field: "status", Message: "Yeni şube etkin başlar."})
	case in.Status != "ACTIVE" && in.Status != "CANCELLED":
		errs = append(errs, httpx.FieldError{Field: "status", Message: "ACTIVE ya da CANCELLED olmalı."})
	}
	return in, errs
}

func cmpOr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func (h *Handler) createSection(w http.ResponseWriter, r *http.Request) {
	o, ok := h.loadOffering(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	var req sectionRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	in, errs := req.validate(true, SectionInput{QuotaMode: "OPEN", InstructionMode: "IN_PERSON", Language: o.Course.Language, Status: "ACTIVE"})
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	if !h.allowed(w, r, permSection, offeringTarget(o)) {
		return
	}
	_, err := h.store.CreateSection(r.Context(), actorID(r), o.ID, in)
	if h.failed(w, r, err, "") {
		return
	}
	h.writeOfferingDetail(w, r, http.StatusCreated, o.ID)
}

// loadSection, yoldaki şubeyi ve açılan dersini bulur.
func (h *Handler) loadSection(w http.ResponseWriter, r *http.Request, id string) (Section, Offering, bool) {
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return Section{}, Offering{}, false
	}
	s, err := h.store.Section(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.NotFound(w, r)
		return Section{}, Offering{}, false
	}
	if err != nil {
		h.serverError(w, r, err)
		return Section{}, Offering{}, false
	}
	o, ok := h.loadOffering(w, r, s.OfferingID)
	return s, o, ok
}

func (h *Handler) writeSection(w http.ResponseWriter, r *http.Request, status int, id string) {
	s, err := h.store.Section(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.NotFound(w, r)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	w.Header().Set("ETag", httpx.ETag(s.Version))
	h.writeJSON(w, r, status, toSectionResponse(s))
}

func (h *Handler) getSection(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return
	}
	h.writeSection(w, r, http.StatusOK, id)
}

func (h *Handler) updateSection(w http.ResponseWriter, r *http.Request) {
	version, ok := httpx.IfMatchVersion(r)
	if !ok {
		httpx.PreconditionRequired(w, r)
		return
	}
	s, o, ok := h.loadSection(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	var req sectionRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	in, errs := req.validate(false, SectionInput{
		Capacity: s.Capacity, QuotaMode: s.QuotaMode, InstructionMode: s.InstructionMode, Language: s.Language, Status: s.Status,
	})
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	if !h.allowed(w, r, permSection, offeringTarget(o)) {
		return
	}
	if h.failed(w, r, h.store.UpdateSection(r.Context(), actorID(r), s.ID, version, in), "") {
		return
	}
	h.writeSection(w, r, http.StatusOK, s.ID)
}

func (h *Handler) deleteSection(w http.ResponseWriter, r *http.Request) {
	s, o, ok := h.loadSection(w, r, r.PathValue("id"))
	if !ok || !h.allowed(w, r, permSection, offeringTarget(o)) {
		return
	}
	if h.failed(w, r, h.store.DeleteSection(r.Context(), actorID(r), s.ID), "") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type instructorsRequest struct {
	Items []struct {
		StaffID string `json:"staff_id"`
		Role    string `json:"role"`
	} `json:"items"`
}

func (h *Handler) setInstructors(w http.ResponseWriter, r *http.Request) {
	s, o, ok := h.loadSection(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	var req instructorsRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	var errs []httpx.FieldError
	if req.Items == nil {
		errs = append(errs, httpx.FieldError{Field: "items", Message: "Zorunlu; hepsini kaldırmak için boş dizi gönderin."})
	}
	if len(req.Items) > 10 {
		errs = append(errs, httpx.FieldError{Field: "items", Message: "En fazla 10 öğretim elemanı."})
	}
	items := make([]InstructorInput, 0, len(req.Items))
	seen := map[string]bool{}
	for _, it := range req.Items {
		switch {
		case !httpx.ValidUUID(it.StaffID):
			errs = append(errs, httpx.FieldError{Field: "items.staff_id", Message: "Geçerli bir UUID olmalı."})
		case seen[it.StaffID]:
			errs = append(errs, httpx.FieldError{Field: "items.staff_id", Message: "Aynı kişi birden fazla kez verilemez."})
		}
		if it.Role != "PRIMARY" && it.Role != "CO_INSTRUCTOR" && it.Role != "ASSISTANT" {
			errs = append(errs, httpx.FieldError{Field: "items.role", Message: "PRIMARY, CO_INSTRUCTOR ya da ASSISTANT olmalı."})
		}
		seen[it.StaffID] = true
		items = append(items, InstructorInput{StaffID: it.StaffID, Role: it.Role})
	}
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	if !h.allowed(w, r, permSection, offeringTarget(o)) {
		return
	}
	if h.failed(w, r, h.store.SetInstructors(r.Context(), actorID(r), s.ID, items), "items.staff_id") {
		return
	}
	h.writeSection(w, r, http.StatusOK, s.ID)
}

type quotasRequest struct {
	Items []struct {
		ProgramID string `json:"program_id"`
		Quota     int    `json:"quota"`
	} `json:"items"`
}

func (h *Handler) setQuotas(w http.ResponseWriter, r *http.Request) {
	s, o, ok := h.loadSection(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	var req quotasRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	var errs []httpx.FieldError
	if req.Items == nil {
		errs = append(errs, httpx.FieldError{Field: "items", Message: "Zorunlu; hepsini kaldırmak için boş dizi gönderin."})
	}
	if len(req.Items) > 50 {
		errs = append(errs, httpx.FieldError{Field: "items", Message: "En fazla 50 program."})
	}
	items := make([]QuotaInput, 0, len(req.Items))
	seen := map[string]bool{}
	for _, it := range req.Items {
		switch {
		case !httpx.ValidUUID(it.ProgramID):
			errs = append(errs, httpx.FieldError{Field: "items.program_id", Message: "Geçerli bir UUID olmalı."})
		case seen[it.ProgramID]:
			errs = append(errs, httpx.FieldError{Field: "items.program_id", Message: "Aynı program birden fazla kez verilemez."})
		}
		if it.Quota < 0 || it.Quota > 2000 {
			errs = append(errs, httpx.FieldError{Field: "items.quota", Message: "0 ile 2000 arasında olmalı."})
		}
		seen[it.ProgramID] = true
		items = append(items, QuotaInput{ProgramID: it.ProgramID, Quota: it.Quota})
	}
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	if !h.allowed(w, r, permQuota, offeringTarget(o)) {
		return
	}
	if h.failed(w, r, h.store.SetQuotas(r.Context(), actorID(r), s.ID, items), "items.program_id") {
		return
	}
	h.writeSection(w, r, http.StatusOK, s.ID)
}

// --- Oturumlar -----------------------------------------------------------------

var clockPattern = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

type slotRequest struct {
	DayOfWeek   int    `json:"day_of_week"`
	StartTime   string `json:"start_time"`
	EndTime     string `json:"end_time"`
	ClassroomID string `json:"classroom_id"`
	SessionType string `json:"session_type"`
}

func (req slotRequest) validate() (SlotInput, []httpx.FieldError) {
	in := SlotInput{DayOfWeek: req.DayOfWeek, Start: req.StartTime, End: req.EndTime, ClassroomID: req.ClassroomID, SessionType: cmpOr(req.SessionType, "THEORY")}
	var errs []httpx.FieldError
	if in.DayOfWeek < 1 || in.DayOfWeek > 7 {
		errs = append(errs, httpx.FieldError{Field: "day_of_week", Message: "1 (pazartesi) ile 7 (pazar) arasında olmalı."})
	}
	validClock := true
	for field, v := range map[string]string{"start_time": in.Start, "end_time": in.End} {
		if !clockPattern.MatchString(v) {
			errs = append(errs, httpx.FieldError{Field: field, Message: "SS:DD biçiminde olmalı (ör. 09:00)."})
			validClock = false
		}
	}
	// Aynı biçimdeki saatler metin olarak da doğru sıralanır.
	if validClock && (in.Start < "07:00" || in.End > "23:00" || in.Start >= in.End) {
		errs = append(errs, httpx.FieldError{Field: "end_time", Message: "Oturum 07:00 ile 23:00 arasında olmalı ve bitiş başlangıçtan sonra gelmeli."})
	}
	if in.ClassroomID != "" && !httpx.ValidUUID(in.ClassroomID) {
		errs = append(errs, httpx.FieldError{Field: "classroom_id", Message: "Geçerli bir UUID olmalı."})
	}
	if in.SessionType != "THEORY" && in.SessionType != "PRACTICE" && in.SessionType != "LAB" {
		errs = append(errs, httpx.FieldError{Field: "session_type", Message: "THEORY, PRACTICE ya da LAB olmalı."})
	}
	return in, errs
}

func (h *Handler) addSlot(w http.ResponseWriter, r *http.Request) {
	s, o, ok := h.loadSection(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	var req slotRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	in, errs := req.validate()
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	if !h.allowed(w, r, permSchedule, offeringTarget(o)) {
		return
	}
	_, err := h.store.AddSlot(r.Context(), actorID(r), s.ID, in)
	if h.failed(w, r, err, "classroom_id") {
		return
	}
	h.writeSection(w, r, http.StatusCreated, s.ID)
}

// loadSlot, yoldaki oturumu ve şubesinin açılan dersini bulur.
func (h *Handler) loadSlot(w http.ResponseWriter, r *http.Request) (Slot, Offering, bool) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return Slot{}, Offering{}, false
	}
	sl, err := h.store.Slot(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.NotFound(w, r)
		return Slot{}, Offering{}, false
	}
	if err != nil {
		h.serverError(w, r, err)
		return Slot{}, Offering{}, false
	}
	_, o, ok := h.loadSection(w, r, sl.SectionID)
	return sl, o, ok
}

func (h *Handler) updateSlot(w http.ResponseWriter, r *http.Request) {
	sl, o, ok := h.loadSlot(w, r)
	if !ok {
		return
	}
	var req slotRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	in, errs := req.validate()
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	if !h.allowed(w, r, permSchedule, offeringTarget(o)) {
		return
	}
	if h.failed(w, r, h.store.UpdateSlot(r.Context(), actorID(r), sl.ID, in), "classroom_id") {
		return
	}
	h.writeSection(w, r, http.StatusOK, sl.SectionID)
}

func (h *Handler) deleteSlot(w http.ResponseWriter, r *http.Request) {
	sl, o, ok := h.loadSlot(w, r)
	if !ok || !h.allowed(w, r, permSchedule, offeringTarget(o)) {
		return
	}
	if h.failed(w, r, h.store.DeleteSlot(r.Context(), actorID(r), sl.ID), "") {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Görünümler ----------------------------------------------------------------

// termSchedule, dönemin haftalık programını bölüm, derslik ya da öğretim elemanına göre
// döndürür. Süzgeçlerden tam olarak biri verilmelidir.
func (h *Handler) termSchedule(w http.ResponseWriter, r *http.Request) {
	termID, ok := h.term(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	f := ScheduleFilter{TermID: termID, DepartmentID: q.Get("department_id"), ClassroomID: q.Get("classroom_id"), StaffID: q.Get("staff_id")}
	given := 0
	var errs []httpx.FieldError
	for field, v := range map[string]string{"department_id": f.DepartmentID, "classroom_id": f.ClassroomID, "staff_id": f.StaffID} {
		if v == "" {
			continue
		}
		given++
		if !httpx.ValidUUID(v) {
			errs = append(errs, httpx.FieldError{Field: field, Message: "Geçerli bir UUID olmalı."})
		}
	}
	if given != 1 {
		errs = append(errs, httpx.FieldError{Field: "department_id", Message: "department_id, classroom_id ya da staff_id süzgeçlerinden tam olarak biri verilmeli."})
	}
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	h.writeSchedule(w, r, f)
}

func (h *Handler) writeSchedule(w http.ResponseWriter, r *http.Request, f ScheduleFilter) {
	entries, err := h.store.Schedule(r.Context(), f)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	h.writeJSON(w, r, http.StatusOK, httpx.ListResponse[scheduleEntryResponse]{Items: mapSlice(entries, toScheduleEntryResponse)})
}

// myTeaching, öğretim elemanının dönemdeki (varsayılan aktif dönem) haftalık programıdır.
// Personel olmayan ya da aktif dönem tanımlı olmayan kullanıcıya boş liste döner.
func (h *Handler) myTeaching(w http.ResponseWriter, r *http.Request) {
	empty := httpx.ListResponse[scheduleEntryResponse]{Items: []scheduleEntryResponse{}}
	p, _ := authn.PrincipalFrom(r.Context())
	staffID, err := h.store.StaffByUser(r.Context(), p.UserID)
	if errors.Is(err, ErrNotFound) {
		h.writeJSON(w, r, http.StatusOK, empty)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	termID := r.URL.Query().Get("term_id")
	switch {
	case termID != "" && !httpx.ValidUUID(termID):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "term_id", Message: "Geçerli bir UUID olmalı."}})
		return
	case termID == "":
		termID, err = h.store.CurrentTermID(r.Context())
		if errors.Is(err, ErrNotFound) {
			h.writeJSON(w, r, http.StatusOK, empty)
			return
		}
		if err != nil {
			h.serverError(w, r, err)
			return
		}
	}
	h.writeSchedule(w, r, ScheduleFilter{TermID: termID, StaffID: staffID})
}

func (h *Handler) searchInstructors(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query, departmentID := strings.TrimSpace(q.Get("q")), q.Get("department_id")
	var errs []httpx.FieldError
	if utf8.RuneCountInString(query) > 100 {
		errs = append(errs, httpx.FieldError{Field: "q", Message: "En fazla 100 karakter."})
	}
	if departmentID != "" && !httpx.ValidUUID(departmentID) {
		errs = append(errs, httpx.FieldError{Field: "department_id", Message: "Geçerli bir UUID olmalı."})
	}
	limit, limitErr := httpx.ParseLimit(q)
	if limitErr != nil {
		errs = append(errs, *limitErr)
	}
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	list, err := h.store.SearchInstructors(r.Context(), query, departmentID, limit)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	h.writeJSON(w, r, http.StatusOK, httpx.ListResponse[staffResponse]{Items: mapSlice(list, func(s StaffSummary) staffResponse {
		res := staffResponse{StaffID: s.StaffID, StaffNo: s.StaffNo, Title: optional(s.Title), FirstName: s.FirstName, LastName: s.LastName}
		if s.Department != nil {
			d := toRef(*s.Department)
			res.Department = &d
		}
		return res
	})})
}

// --- Hatalar ve yetki ----------------------------------------------------------

// failed, yazma hatasını problem yanıtına çevirir. unknownField, başvurulan kaydın
// bulunamadığı durumda gösterilecek alandır.
func (h *Handler) failed(w http.ResponseWriter, r *http.Request, err error, unknownField string) bool {
	var (
		classroom  *ClassroomConflictError
		instructor *InstructorConflictError
		small      *ClassroomTooSmallError
	)
	switch {
	case err == nil:
		return false
	case errors.Is(err, ErrNotFound):
		httpx.NotFound(w, r)
	case errors.Is(err, ErrVersionMismatch):
		httpx.PreconditionFailed(w, r)
	case errors.Is(err, ErrConflict) && unknownField == "course_id":
		h.problem(w, r, http.StatusConflict, "COURSE_ALREADY_OFFERED", "Bu ders bu dönemde zaten açılmış; yeni öğrenci grubu için şube ekleyin.")
	case errors.Is(err, ErrConflict):
		h.problem(w, r, http.StatusConflict, "SECTION_CODE_TAKEN", "Bu kodla bir şube zaten var.")
	case errors.Is(err, ErrUnknownReference):
		field := unknownField
		if field == "" {
			field = "id"
		}
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: field, Message: "Başvurulan kayıt bulunamadı."}})
	case errors.Is(err, ErrTermClosed):
		h.problem(w, r, http.StatusConflict, "TERM_CLOSED", "Dönem kapanmış; ders açma değiştirilemez.")
	case errors.Is(err, ErrInvalidTransition):
		h.problem(w, r, http.StatusConflict, "INVALID_STATUS_TRANSITION", "Açılan dersin şu anki durumunda bu işlem yapılamaz.")
	case errors.Is(err, ErrHasEnrollments):
		h.problem(w, r, http.StatusConflict, "HAS_ENROLLMENTS", "Kayıtlı öğrenci varken bu işlem yapılamaz.")
	case errors.Is(err, ErrBelowEnrolled):
		h.problem(w, r, http.StatusConflict, "BELOW_ENROLLED", "Kontenjan kayıtlı öğrenci sayısının altına indirilemez.")
	case errors.Is(err, ErrQuotaExceeds):
		h.problem(w, r, http.StatusConflict, "QUOTA_EXCEEDS_CAPACITY", "Program kontenjanlarının toplamı şube kontenjanını aşıyor.")
	case errors.Is(err, ErrSectionOverlap):
		h.problem(w, r, http.StatusConflict, "SECTION_OVERLAP", "Şubenin bu saatte başka bir oturumu var.")
	case errors.Is(err, ErrSectionCancelled):
		h.problem(w, r, http.StatusConflict, "SECTION_CANCELLED", "Şube iptal edilmiş.")
	case errors.Is(err, ErrClassroomInactive):
		h.problem(w, r, http.StatusConflict, "CLASSROOM_INACTIVE", "Derslik kullanımda değil.")
	case errors.Is(err, ErrPrimaryRequired):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "items.role", Message: "Tam olarak bir sorumlu öğretim elemanı (PRIMARY) olmalı."}})
	case errors.Is(err, ErrNotInstructor):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "items.staff_id", Message: "Sadece görevdeki akademik personel atanabilir."}})
	case errors.As(err, &classroom):
		detail := "Derslik bu saatte başka bir oturuma verilmiş."
		if classroom.With.CourseCode != "" {
			detail = "Derslik bu saatte dolu: " + classroom.With.String() + "."
		}
		h.problem(w, r, http.StatusConflict, "CLASSROOM_CONFLICT", detail)
	case errors.As(err, &instructor):
		h.problem(w, r, http.StatusConflict, "INSTRUCTOR_CONFLICT",
			instructor.StaffName+" bu saatte başka bir derste: "+instructor.With.String()+".")
	case errors.As(err, &small):
		h.problem(w, r, http.StatusConflict, "CLASSROOM_TOO_SMALL", fmt.Sprintf(
			"%s %d kişilik, şube kontenjanı %d; teorik oturum için daha büyük bir derslik seçin.", small.Classroom, small.Capacity, small.Needed))
	default:
		h.serverError(w, r, err)
	}
	return true
}

// allowed, yetkinin açılan dersin bölümünü kapsayıp kapsamadığına bakar.
func (h *Handler) allowed(w http.ResponseWriter, r *http.Request, perm string, target authz.Target) bool {
	perms, ok := authz.PermissionsFrom(r.Context())
	if ok && perms.Allows(perm, target) {
		return true
	}
	h.problem(w, r, http.StatusForbidden, "FORBIDDEN", "Bu bölümün ders açma işlemlerini yönetme yetkiniz yok.")
	return false
}

// --- Yardımcılar ---------------------------------------------------------------

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

func mapSlice[T, U any](in []T, f func(T) U) []U {
	out := make([]U, 0, len(in))
	for _, v := range in {
		out = append(out, f(v))
	}
	return out
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
