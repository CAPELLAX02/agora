package org

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

// Store, handler'ın ihtiyaç duyduğu veri erişim işlemleridir.
// *Repository bu interface'i örtük olarak sağlar, testlerde sahtesi kullanılır.
type Store interface {
	ListFaculties(ctx context.Context, filter FacultyFilter) ([]Faculty, error)
	GetFaculty(ctx context.Context, id string) (Faculty, error)
	ListDepartments(ctx context.Context, facultyID string, includeInactive bool) ([]Department, error)
	GetDepartment(ctx context.Context, id string) (Department, error)
	ListPrograms(ctx context.Context, filter ProgramFilter) ([]Program, bool, error)
	GetProgram(ctx context.Context, id string) (Program, error)
}

// Handler, org modülünün HTTP uç noktalarını sunar.
type Handler struct {
	store  Store
	logger *slog.Logger
}

// NewHandler, verilen veri katmanını kullanan bir Handler oluşturur.
func NewHandler(store Store, logger *slog.Logger) *Handler {
	return &Handler{store: store, logger: logger}
}

// Register, modülün route'larını kaydeder. Organizasyon kataloğu herkese açıktır.
func (h *Handler) Register(rt *authz.Router) {
	rt.HandleFunc("GET /api/v1/faculties", authz.Public, h.listFaculties)
	rt.HandleFunc("GET /api/v1/faculties/{id}", authz.Public, h.getFaculty)
	rt.HandleFunc("GET /api/v1/faculties/{id}/departments", authz.Public, h.listFacultyDepartments)
	rt.HandleFunc("GET /api/v1/departments/{id}", authz.Public, h.getDepartment)
	rt.HandleFunc("GET /api/v1/programs", authz.Public, h.listPrograms)
	rt.HandleFunc("GET /api/v1/programs/{id}", authz.Public, h.getProgram)
}

// --- Yanıt tipleri -----------------------------------------------------------

type campusResponse struct {
	ID   string `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

type refResponse struct {
	ID     string `json:"id"`
	Code   string `json:"code"`
	NameTR string `json:"name_tr"`
}

type facultyResponse struct {
	ID       string          `json:"id"`
	Code     string          `json:"code"`
	NameTR   string          `json:"name_tr"`
	NameEN   string          `json:"name_en"`
	UnitType string          `json:"unit_type"`
	Campus   *campusResponse `json:"campus"`
	IsActive bool            `json:"is_active"`
}

type departmentResponse struct {
	ID       string      `json:"id"`
	Code     string      `json:"code"`
	NameTR   string      `json:"name_tr"`
	NameEN   string      `json:"name_en"`
	Faculty  refResponse `json:"faculty"`
	IsActive bool        `json:"is_active"`
}

type programResponse struct {
	ID                string      `json:"id"`
	Code              string      `json:"code"`
	YoksisCode        *string     `json:"yoksis_code"`
	NameTR            string      `json:"name_tr"`
	NameEN            string      `json:"name_en"`
	DegreeLevel       string      `json:"degree_level"`
	Language          string      `json:"language"`
	EducationType     string      `json:"education_type"`
	DurationSemesters int         `json:"duration_semesters"`
	MaxDurationYears  int         `json:"max_duration_years"`
	TotalECTSRequired float64     `json:"total_ects_required"`
	HasPrepClass      bool        `json:"has_prep_class"`
	Department        refResponse `json:"department"`
	Faculty           refResponse `json:"faculty"`
	IsActive          bool        `json:"is_active"`
}

func toFacultyResponse(f Faculty) facultyResponse {
	res := facultyResponse{
		ID:       f.ID,
		Code:     f.Code,
		NameTR:   f.NameTR,
		NameEN:   f.NameEN,
		UnitType: string(f.UnitType),
		IsActive: f.IsActive,
	}
	if f.Campus != nil {
		res.Campus = &campusResponse{ID: f.Campus.ID, Code: f.Campus.Code, Name: f.Campus.Name}
	}
	return res
}

func toDepartmentResponse(d Department) departmentResponse {
	return departmentResponse{
		ID:       d.ID,
		Code:     d.Code,
		NameTR:   d.NameTR,
		NameEN:   d.NameEN,
		Faculty:  refResponse{ID: d.Faculty.ID, Code: d.Faculty.Code, NameTR: d.Faculty.NameTR},
		IsActive: d.IsActive,
	}
}

func toProgramResponse(p Program) programResponse {
	return programResponse{
		ID:                p.ID,
		Code:              p.Code,
		YoksisCode:        p.YoksisCode,
		NameTR:            p.NameTR,
		NameEN:            p.NameEN,
		DegreeLevel:       string(p.DegreeLevel),
		Language:          string(p.Language),
		EducationType:     string(p.EducationType),
		DurationSemesters: p.DurationSemesters,
		MaxDurationYears:  p.MaxDurationYears,
		TotalECTSRequired: p.TotalECTSRequired,
		HasPrepClass:      p.HasPrepClass,
		Department:        refResponse{ID: p.Department.ID, Code: p.Department.Code, NameTR: p.Department.NameTR},
		Faculty:           refResponse{ID: p.Faculty.ID, Code: p.Faculty.Code, NameTR: p.Faculty.NameTR},
		IsActive:          p.IsActive,
	}
}

// mapSlice, bir slice'ın her elemanını f ile dönüştürür. Sonuç hiçbir zaman nil
// olmaz, böylece boş liste JSON'a null değil [] olarak yazılır.
func mapSlice[T, R any](items []T, f func(T) R) []R {
	out := make([]R, 0, len(items))
	for _, item := range items {
		out = append(out, f(item))
	}
	return out
}

// --- Birimler ----------------------------------------------------------------

func (h *Handler) listFaculties(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var errs []httpx.FieldError

	filter := FacultyFilter{
		UnitType: UnitType(q.Get("type")),
		Query:    parseQuery(q, &errs),
	}
	if filter.UnitType != "" && !filter.UnitType.Valid() {
		errs = append(errs, httpx.FieldError{Field: "type", Message: "geçersiz birim türü"})
	}
	filter.IncludeInactive = parseIncludeInactive(q, &errs)

	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}

	faculties, err := h.store.ListFaculties(r.Context(), filter)
	if err != nil {
		h.serverError(w, r, err)
		return
	}

	h.writeJSON(w, r, http.StatusOK, httpx.ListResponse[facultyResponse]{
		Items: mapSlice(faculties, toFacultyResponse),
	})
}

func (h *Handler) getFaculty(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return
	}

	f, err := h.store.GetFaculty(r.Context(), id)
	if err != nil {
		h.storeError(w, r, err)
		return
	}

	h.writeJSON(w, r, http.StatusOK, toFacultyResponse(f))
}

// --- Bölümler ----------------------------------------------------------------

func (h *Handler) listFacultyDepartments(w http.ResponseWriter, r *http.Request) {
	facultyID := r.PathValue("id")
	if !httpx.ValidUUID(facultyID) {
		httpx.NotFound(w, r)
		return
	}

	var errs []httpx.FieldError
	includeInactive := parseIncludeInactive(r.URL.Query(), &errs)
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}

	// Birim yoksa 404, varsa ama bölümü yoksa 200 + boş liste.
	if _, err := h.store.GetFaculty(r.Context(), facultyID); err != nil {
		h.storeError(w, r, err)
		return
	}

	departments, err := h.store.ListDepartments(r.Context(), facultyID, includeInactive)
	if err != nil {
		h.serverError(w, r, err)
		return
	}

	h.writeJSON(w, r, http.StatusOK, httpx.ListResponse[departmentResponse]{
		Items: mapSlice(departments, toDepartmentResponse),
	})
}

func (h *Handler) getDepartment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return
	}

	d, err := h.store.GetDepartment(r.Context(), id)
	if err != nil {
		h.storeError(w, r, err)
		return
	}

	h.writeJSON(w, r, http.StatusOK, toDepartmentResponse(d))
}

// --- Programlar --------------------------------------------------------------

func (h *Handler) listPrograms(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var errs []httpx.FieldError

	filter := ProgramFilter{
		FacultyID:     q.Get("faculty_id"),
		DepartmentID:  q.Get("department_id"),
		DegreeLevel:   DegreeLevel(q.Get("degree_level")),
		Language:      Language(q.Get("language")),
		EducationType: EducationType(q.Get("education_type")),
		Query:         parseQuery(q, &errs),
	}
	filter.IncludeInactive = parseIncludeInactive(q, &errs)

	if filter.FacultyID != "" && !httpx.ValidUUID(filter.FacultyID) {
		errs = append(errs, httpx.FieldError{Field: "faculty_id", Message: "geçerli bir UUID olmalı"})
	}
	if filter.DepartmentID != "" && !httpx.ValidUUID(filter.DepartmentID) {
		errs = append(errs, httpx.FieldError{Field: "department_id", Message: "geçerli bir UUID olmalı"})
	}
	if filter.DegreeLevel != "" && !filter.DegreeLevel.Valid() {
		errs = append(errs, httpx.FieldError{Field: "degree_level", Message: "geçersiz derece"})
	}
	if filter.Language != "" && !filter.Language.Valid() {
		errs = append(errs, httpx.FieldError{Field: "language", Message: "geçersiz öğretim dili"})
	}
	if filter.EducationType != "" && !filter.EducationType.Valid() {
		errs = append(errs, httpx.FieldError{Field: "education_type", Message: "geçersiz öğretim türü"})
	}

	limit, limitErr := httpx.ParseLimit(q)
	if limitErr != nil {
		errs = append(errs, *limitErr)
	}
	filter.Limit = limit

	if c := q.Get("cursor"); c != "" {
		after, err := httpx.DecodeCursor[ProgramCursor](c)
		if err != nil || !httpx.ValidUUID(after.ID) {
			errs = append(errs, httpx.FieldError{Field: "cursor", Message: "geçersiz cursor"})
		} else {
			filter.After = &after
		}
	}

	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}

	programs, hasMore, err := h.store.ListPrograms(r.Context(), filter)
	if err != nil {
		h.serverError(w, r, err)
		return
	}

	res := httpx.ListResponse[programResponse]{Items: mapSlice(programs, toProgramResponse)}
	if hasMore {
		last := programs[len(programs)-1]
		next, err := httpx.EncodeCursor(ProgramCursor{NameTR: last.NameTR, ID: last.ID})
		if err != nil {
			h.serverError(w, r, err)
			return
		}
		res.NextCursor = next
	}

	h.writeJSON(w, r, http.StatusOK, res)
}

func (h *Handler) getProgram(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return
	}

	p, err := h.store.GetProgram(r.Context(), id)
	if err != nil {
		h.storeError(w, r, err)
		return
	}

	h.writeJSON(w, r, http.StatusOK, toProgramResponse(p))
}

// --- Ortak yardımcılar -------------------------------------------------------

const maxQueryLength = 100

// parseQuery, "q" arama parametresini okur ve uzunluğunu doğrular.
func parseQuery(q url.Values, errs *[]httpx.FieldError) string {
	v := strings.TrimSpace(q.Get("q"))
	if utf8.RuneCountInString(v) > maxQueryLength {
		*errs = append(*errs, httpx.FieldError{Field: "q", Message: "en fazla 100 karakter olabilir"})
	}
	return v
}

// parseIncludeInactive, "include_inactive" parametresini okur. Yoksa false döner.
func parseIncludeInactive(q url.Values, errs *[]httpx.FieldError) bool {
	v := q.Get("include_inactive")
	if v == "" {
		return false
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		*errs = append(*errs, httpx.FieldError{Field: "include_inactive", Message: "true veya false olmalı"})
	}
	return b
}

func (h *Handler) writeJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	if err := httpx.WriteJSON(w, status, v); err != nil {
		h.logger.Error("yanıt yazılamadı",
			"err", err, "path", r.URL.Path, "request_id", httpx.RequestIDFrom(r.Context()))
	}
}

// storeError, veri katmanından gelen hatayı yanıta çevirir: ErrNotFound → 404, diğerleri → 500.
func (h *Handler) storeError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, ErrNotFound) {
		httpx.NotFound(w, r)
		return
	}
	h.serverError(w, r, err)
}

func (h *Handler) serverError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.Error("beklenmeyen hata",
		"err", err, "path", r.URL.Path, "request_id", httpx.RequestIDFrom(r.Context()))
	httpx.InternalServerError(w, r)
}
