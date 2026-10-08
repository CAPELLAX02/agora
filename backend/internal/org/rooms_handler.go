package org

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

// RoomStore, bina ve derslik işlemleridir. *Repository bunu sağlar.
type RoomStore interface {
	ListBuildings(ctx context.Context, campusID string, includeInactive bool) ([]Building, error)
	GetBuilding(ctx context.Context, id string) (Building, error)
	CreateBuilding(ctx context.Context, actorID string, in BuildingInput) (string, error)
	UpdateBuilding(ctx context.Context, actorID, id string, version int, in BuildingInput) error
	ListClassrooms(ctx context.Context, f ClassroomFilter) ([]Classroom, bool, error)
	GetClassroom(ctx context.Context, id string) (Classroom, error)
	CreateClassroom(ctx context.Context, actorID string, in ClassroomInput) (string, error)
	UpdateClassroom(ctx context.Context, actorID, id string, version int, in ClassroomInput) error
}

// RoomsHandler, bina ve derslik uçlarını sunar. Okuma herkese açıktır (ders
// programında derslik adları görünür), yazma classroom:manage yetkisi ister ve
// yetkinin binanın birimini kapsaması gerekir.
type RoomsHandler struct {
	store  RoomStore
	logger *slog.Logger
}

// NewRoomsHandler, bir RoomsHandler oluşturur.
func NewRoomsHandler(store RoomStore, logger *slog.Logger) *RoomsHandler {
	return &RoomsHandler{store: store, logger: logger}
}

// Register, route'ları kaydeder.
func (h *RoomsHandler) Register(rt *authz.Router) {
	rt.HandleFunc("GET /api/v1/buildings", authz.Public, h.listBuildings)
	rt.HandleFunc("GET /api/v1/buildings/{id}", authz.Public, h.getBuilding)
	rt.HandleFunc("POST /api/v1/buildings", authz.Permission(permManage), h.createBuilding)
	rt.HandleFunc("PUT /api/v1/buildings/{id}", authz.Permission(permManage), h.updateBuilding)

	rt.HandleFunc("GET /api/v1/classrooms", authz.Public, h.listClassrooms)
	rt.HandleFunc("GET /api/v1/classrooms/{id}", authz.Public, h.getClassroom)
	rt.HandleFunc("POST /api/v1/classrooms", authz.Permission(permManage), h.createClassroom)
	rt.HandleFunc("PUT /api/v1/classrooms/{id}", authz.Permission(permManage), h.updateClassroom)
}

const permManage = "classroom:manage"

type buildingRefResponse struct {
	ID   string `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

type buildingResponse struct {
	ID             string         `json:"id"`
	Code           string         `json:"code"`
	Name           string         `json:"name"`
	Campus         campusResponse `json:"campus"`
	Faculty        *refResponse   `json:"faculty"`
	IsActive       bool           `json:"is_active"`
	ClassroomCount int            `json:"classroom_count"`
	Version        int            `json:"version"`
}

type classroomResponse struct {
	ID           string              `json:"id"`
	Code         string              `json:"code"`
	Name         string              `json:"name"`
	Building     buildingRefResponse `json:"building"`
	Campus       campusResponse      `json:"campus"`
	Capacity     int                 `json:"capacity"`
	ExamCapacity int                 `json:"exam_capacity"`
	RoomType     string              `json:"room_type"`
	Features     []string            `json:"features"`
	IsActive     bool                `json:"is_active"`
	Version      int                 `json:"version"`
}

func toBuildingResponse(b Building) buildingResponse {
	res := buildingResponse{
		ID: b.ID, Code: b.Code, Name: b.Name,
		Campus:   campusResponse{ID: b.Campus.ID, Code: b.Campus.Code, Name: b.Campus.Name},
		IsActive: b.IsActive, ClassroomCount: b.ClassroomCount, Version: b.Version,
	}
	if b.Faculty != nil {
		res.Faculty = &refResponse{ID: b.Faculty.ID, Code: b.Faculty.Code, NameTR: b.Faculty.NameTR}
	}
	return res
}

func toClassroomResponse(c Classroom) classroomResponse {
	features := c.Features
	if features == nil {
		features = []string{}
	}
	return classroomResponse{
		ID: c.ID, Code: c.Code, Name: c.Name,
		Building:     buildingRefResponse{ID: c.Building.ID, Code: c.Building.Code, Name: c.Building.Name},
		Campus:       campusResponse{ID: c.Campus.ID, Code: c.Campus.Code, Name: c.Campus.Name},
		Capacity:     c.Capacity,
		ExamCapacity: c.ExamCapacity,
		RoomType:     string(c.RoomType),
		Features:     features,
		IsActive:     c.IsActive,
		Version:      c.Version,
	}
}

// --- Binalar ------------------------------------------------------------------

func (h *RoomsHandler) listBuildings(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var errs []httpx.FieldError
	campusID := q.Get("campus_id")
	if campusID != "" && !httpx.ValidUUID(campusID) {
		errs = append(errs, httpx.FieldError{Field: "campus_id", Message: "geçerli bir UUID olmalı"})
	}
	includeInactive := parseIncludeInactive(q, &errs)
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}

	buildings, err := h.store.ListBuildings(r.Context(), campusID, includeInactive)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	h.writeJSON(w, r, http.StatusOK, httpx.ListResponse[buildingResponse]{Items: mapSlice(buildings, toBuildingResponse)})
}

func (h *RoomsHandler) getBuilding(w http.ResponseWriter, r *http.Request) {
	b, ok := h.building(w, r)
	if !ok {
		return
	}
	w.Header().Set("ETag", httpx.ETag(b.Version))
	h.writeJSON(w, r, http.StatusOK, toBuildingResponse(b))
}

type buildingRequest struct {
	CampusID  string `json:"campus_id"`
	FacultyID string `json:"faculty_id"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	IsActive  *bool  `json:"is_active"`
}

var codePattern = regexp.MustCompile(`^[A-Z0-9-]{1,20}$`)

func (h *RoomsHandler) createBuilding(w http.ResponseWriter, r *http.Request) {
	var req buildingRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	var errs []httpx.FieldError
	if !httpx.ValidUUID(req.CampusID) {
		errs = append(errs, httpx.FieldError{Field: "campus_id", Message: "Yerleşke zorunlu."})
	}
	if !codePattern.MatchString(req.Code) {
		errs = append(errs, httpx.FieldError{Field: "code", Message: "1-20 büyük harf, rakam ya da tire olmalı."})
	}
	errs = append(errs, validateBuilding(req)...)
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	if !h.allowed(w, r, req.FacultyID) {
		return
	}

	id, err := h.store.CreateBuilding(r.Context(), actorID(r), BuildingInput{
		CampusID: req.CampusID, FacultyID: req.FacultyID, Code: req.Code, Name: strings.TrimSpace(req.Name),
	})
	if h.writeFailed(w, r, err, "Bu yerleşkede aynı kodla bir bina var.", "campus_id", "Yerleşke ya da birim bulunamadı.") {
		return
	}
	b, err := h.store.GetBuilding(r.Context(), id)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/buildings/"+id)
	w.Header().Set("ETag", httpx.ETag(b.Version))
	h.writeJSON(w, r, http.StatusCreated, toBuildingResponse(b))
}

func (h *RoomsHandler) updateBuilding(w http.ResponseWriter, r *http.Request) {
	version, ok := httpx.IfMatchVersion(r)
	if !ok {
		httpx.PreconditionRequired(w, r)
		return
	}
	current, ok := h.building(w, r)
	if !ok {
		return
	}

	var req buildingRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	errs := validateBuilding(req)
	if req.IsActive == nil {
		errs = append(errs, httpx.FieldError{Field: "is_active", Message: "Zorunlu."})
	}
	if req.CampusID != "" || req.Code != "" {
		errs = append(errs, httpx.FieldError{Field: "code", Message: "Yerleşke ve kod değiştirilemez."})
	}
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	// Hem mevcut hem yeni birim yetki kapsamında olmalı: kişi kendi birimindeki bir
	// binayı başka birime "taşıyıp" kontrolü dışına çıkaramamalı ya da tersi.
	if !h.allowed(w, r, facultyOf(current)) || !h.allowed(w, r, req.FacultyID) {
		return
	}

	err := h.store.UpdateBuilding(r.Context(), actorID(r), current.ID, version, BuildingInput{
		FacultyID: req.FacultyID, Name: strings.TrimSpace(req.Name), IsActive: *req.IsActive,
	})
	if h.writeFailed(w, r, err, "", "faculty_id", "Birim bulunamadı.") {
		return
	}
	h.getBuilding(w, r)
}

func validateBuilding(req buildingRequest) []httpx.FieldError {
	var errs []httpx.FieldError
	if req.FacultyID != "" && !httpx.ValidUUID(req.FacultyID) {
		errs = append(errs, httpx.FieldError{Field: "faculty_id", Message: "Geçerli bir UUID olmalı."})
	}
	if n := utf8.RuneCountInString(strings.TrimSpace(req.Name)); n == 0 || n > 200 {
		errs = append(errs, httpx.FieldError{Field: "name", Message: "Zorunlu, en fazla 200 karakter."})
	}
	return errs
}

func (h *RoomsHandler) building(w http.ResponseWriter, r *http.Request) (Building, bool) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return Building{}, false
	}
	b, err := h.store.GetBuilding(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.NotFound(w, r)
		return Building{}, false
	}
	if err != nil {
		h.serverError(w, r, err)
		return Building{}, false
	}
	return b, true
}

func facultyOf(b Building) string {
	if b.Faculty == nil {
		return ""
	}
	return b.Faculty.ID
}

// --- Derslikler ---------------------------------------------------------------

func (h *RoomsHandler) listClassrooms(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var errs []httpx.FieldError

	f := ClassroomFilter{
		BuildingID: q.Get("building_id"),
		CampusID:   q.Get("campus_id"),
		RoomType:   RoomType(q.Get("room_type")),
		Query:      parseQuery(q, &errs),
	}
	for field, v := range map[string]string{"building_id": f.BuildingID, "campus_id": f.CampusID} {
		if v != "" && !httpx.ValidUUID(v) {
			errs = append(errs, httpx.FieldError{Field: field, Message: "geçerli bir UUID olmalı"})
		}
	}
	if f.RoomType != "" && !f.RoomType.Valid() {
		errs = append(errs, httpx.FieldError{Field: "room_type", Message: "geçersiz derslik türü"})
	}
	if v := q.Get("min_capacity"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			errs = append(errs, httpx.FieldError{Field: "min_capacity", Message: "0 ya da pozitif bir tamsayı olmalı"})
		}
		f.MinCapacity = n
	}
	f.IncludeInactive = parseIncludeInactive(q, &errs)
	limit, limitErr := httpx.ParseLimit(q)
	if limitErr != nil {
		errs = append(errs, *limitErr)
	}
	f.Limit = limit
	if c := q.Get("cursor"); c != "" {
		cur, err := httpx.DecodeCursor[ClassroomCursor](c)
		if err != nil || !httpx.ValidUUID(cur.ID) {
			errs = append(errs, httpx.FieldError{Field: "cursor", Message: "geçersiz cursor"})
		} else {
			f.After = &cur
		}
	}
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}

	rooms, hasMore, err := h.store.ListClassrooms(r.Context(), f)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	res := httpx.ListResponse[classroomResponse]{Items: mapSlice(rooms, toClassroomResponse)}
	if hasMore {
		last := rooms[len(rooms)-1]
		if res.NextCursor, err = httpx.EncodeCursor(ClassroomCursor{BuildingCode: last.Building.Code, Code: last.Code, ID: last.ID}); err != nil {
			h.serverError(w, r, err)
			return
		}
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

func (h *RoomsHandler) getClassroom(w http.ResponseWriter, r *http.Request) {
	c, ok := h.classroom(w, r)
	if !ok {
		return
	}
	w.Header().Set("ETag", httpx.ETag(c.Version))
	h.writeJSON(w, r, http.StatusOK, toClassroomResponse(c))
}

type classroomRequest struct {
	BuildingID   string   `json:"building_id"`
	Code         string   `json:"code"`
	Name         string   `json:"name"`
	Capacity     *int     `json:"capacity"`
	ExamCapacity *int     `json:"exam_capacity"`
	RoomType     string   `json:"room_type"`
	Features     []string `json:"features"`
	IsActive     *bool    `json:"is_active"`
}

func (h *RoomsHandler) createClassroom(w http.ResponseWriter, r *http.Request) {
	var req classroomRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	var errs []httpx.FieldError
	if !httpx.ValidUUID(req.BuildingID) {
		errs = append(errs, httpx.FieldError{Field: "building_id", Message: "Bina zorunlu."})
	}
	if !codePattern.MatchString(req.Code) {
		errs = append(errs, httpx.FieldError{Field: "code", Message: "1-20 büyük harf, rakam ya da tire olmalı."})
	}
	errs = append(errs, validateClassroom(req)...)
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}

	b, err := h.store.GetBuilding(r.Context(), req.BuildingID)
	if errors.Is(err, ErrNotFound) {
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "building_id", Message: "Bina bulunamadı."}})
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	if !h.allowed(w, r, facultyOf(b)) {
		return
	}

	id, err := h.store.CreateClassroom(r.Context(), actorID(r), ClassroomInput{
		BuildingID: req.BuildingID, Code: req.Code, Name: strings.TrimSpace(req.Name),
		Capacity: *req.Capacity, ExamCapacity: *req.ExamCapacity, RoomType: RoomType(req.RoomType), Features: req.Features,
	})
	if h.writeFailed(w, r, err, "Bu binada aynı kodla bir derslik var.", "building_id", "Bina bulunamadı.") {
		return
	}
	c, err := h.store.GetClassroom(r.Context(), id)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/classrooms/"+id)
	w.Header().Set("ETag", httpx.ETag(c.Version))
	h.writeJSON(w, r, http.StatusCreated, toClassroomResponse(c))
}

func (h *RoomsHandler) updateClassroom(w http.ResponseWriter, r *http.Request) {
	version, ok := httpx.IfMatchVersion(r)
	if !ok {
		httpx.PreconditionRequired(w, r)
		return
	}
	current, ok := h.classroom(w, r)
	if !ok {
		return
	}

	var req classroomRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	errs := validateClassroom(req)
	if req.IsActive == nil {
		errs = append(errs, httpx.FieldError{Field: "is_active", Message: "Zorunlu."})
	}
	if req.BuildingID != "" || req.Code != "" {
		errs = append(errs, httpx.FieldError{Field: "code", Message: "Bina ve kod değiştirilemez."})
	}
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	if !h.allowed(w, r, current.FacultyID) {
		return
	}

	err := h.store.UpdateClassroom(r.Context(), actorID(r), current.ID, version, ClassroomInput{
		Name: strings.TrimSpace(req.Name), Capacity: *req.Capacity, ExamCapacity: *req.ExamCapacity,
		RoomType: RoomType(req.RoomType), Features: req.Features, IsActive: *req.IsActive,
	})
	if h.writeFailed(w, r, err, "", "", "") {
		return
	}
	h.getClassroom(w, r)
}

func validateClassroom(req classroomRequest) []httpx.FieldError {
	var errs []httpx.FieldError
	add := func(field, msg string) { errs = append(errs, httpx.FieldError{Field: field, Message: msg}) }

	if n := utf8.RuneCountInString(strings.TrimSpace(req.Name)); n == 0 || n > 200 {
		add("name", "Zorunlu, en fazla 200 karakter.")
	}
	switch {
	case req.Capacity == nil || *req.Capacity < 0 || *req.Capacity > 2000:
		add("capacity", "0 ile 2000 arasında olmalı.")
	case req.ExamCapacity == nil || *req.ExamCapacity < 0 || *req.ExamCapacity > *req.Capacity:
		add("exam_capacity", "0 ile derslik kapasitesi arasında olmalı.")
	}
	if !RoomType(req.RoomType).Valid() {
		add("room_type", "LECTURE, LAB, AMPHI, OFFICE ya da ONLINE olmalı.")
	}
	for _, f := range req.Features {
		if !slices.Contains(ClassroomFeatures, f) {
			add("features", "Bilinmeyen donanım: "+f)
		}
	}
	return errs
}

func (h *RoomsHandler) classroom(w http.ResponseWriter, r *http.Request) (Classroom, bool) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return Classroom{}, false
	}
	c, err := h.store.GetClassroom(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.NotFound(w, r)
		return Classroom{}, false
	}
	if err != nil {
		h.serverError(w, r, err)
		return Classroom{}, false
	}
	return c, true
}

// --- Ortak --------------------------------------------------------------------

// allowed, classroom:manage yetkisinin verilen birimi kapsayıp kapsamadığına bakar.
// Birim boşsa (bir birime ait olmayan bina) sadece üniversite kapsamı yeterlidir.
func (h *RoomsHandler) allowed(w http.ResponseWriter, r *http.Request, facultyID string) bool {
	perms, ok := authz.PermissionsFrom(r.Context())
	if ok && perms.Allows(permManage, authz.Target{FacultyID: facultyID}) {
		return true
	}
	_ = httpx.WriteProblem(w, r, httpx.Problem{
		Status: http.StatusForbidden, Code: "FORBIDDEN",
		Detail: "Bu birimin binalarını ve dersliklerini yönetme yetkiniz yok.",
	})
	return false
}

// writeFailed, yazma hatalarını yanıta çevirir. Hata yoksa false döner.
func (h *RoomsHandler) writeFailed(w http.ResponseWriter, r *http.Request, err error, conflictMsg, refField, refMsg string) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, ErrNotFound):
		httpx.NotFound(w, r)
	case errors.Is(err, ErrVersionMismatch):
		httpx.PreconditionFailed(w, r)
	case errors.Is(err, ErrConflict):
		_ = httpx.WriteProblem(w, r, httpx.Problem{Status: http.StatusConflict, Code: "CODE_TAKEN", Detail: conflictMsg})
	case errors.Is(err, ErrUnknownReference):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: refField, Message: refMsg}})
	default:
		h.serverError(w, r, err)
	}
	return true
}

func actorID(r *http.Request) string {
	p, _ := authn.PrincipalFrom(r.Context())
	return p.UserID
}

func (h *RoomsHandler) writeJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	if err := httpx.WriteJSON(w, status, v); err != nil {
		h.logger.Error("yanıt yazılamadı", "err", err, "request_id", httpx.RequestIDFrom(r.Context()))
	}
}

func (h *RoomsHandler) serverError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.Error("beklenmeyen hata", "err", err, "path", r.URL.Path, "request_id", httpx.RequestIDFrom(r.Context()))
	httpx.InternalServerError(w, r)
}
