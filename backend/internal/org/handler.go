package org

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

// Store, handler'ın ihtiyaç duyduğu veri erişim işlemleridir.
// *Repository bu interface'i örtük olarak sağlar, testlerde sahtesi kullanılır.
type Store interface {
	ListFaculties(ctx context.Context, filter FacultyFilter) ([]Faculty, error)
	GetFaculty(ctx context.Context, id string) (Faculty, error)
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

// Register, modülün route'larını mux'a kaydeder.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/faculties", h.listFaculties)
	mux.HandleFunc("GET /api/v1/faculties/{id}", h.getFaculty)
}

type campusResponse struct {
	ID   string `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
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

const maxQueryLength = 100

func (h *Handler) listFaculties(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	filter := FacultyFilter{
		UnitType: UnitType(q.Get("type")),
		Query:    strings.TrimSpace(q.Get("q")),
	}

	var errs []httpx.FieldError
	if filter.UnitType != "" && !filter.UnitType.Valid() {
		errs = append(errs, httpx.FieldError{Field: "type", Message: "geçersiz birim türü"})
	}
	if utf8.RuneCountInString(filter.Query) > maxQueryLength {
		errs = append(errs, httpx.FieldError{Field: "q", Message: "en fazla 100 karakter olabilir"})
	}
	if v := q.Get("include_inactive"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			errs = append(errs, httpx.FieldError{Field: "include_inactive", Message: "true veya false olmalı"})
		}
		filter.IncludeInactive = b
	}
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}

	faculties, err := h.store.ListFaculties(r.Context(), filter)
	if err != nil {
		h.serverError(w, r, err)
		return
	}

	items := make([]facultyResponse, 0, len(faculties))
	for _, f := range faculties {
		items = append(items, toFacultyResponse(f))
	}

	h.writeJSON(w, r, http.StatusOK, httpx.ListResponse[facultyResponse]{Items: items})
}

func (h *Handler) getFaculty(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return
	}

	f, err := h.store.GetFaculty(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.NotFound(w, r)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}

	h.writeJSON(w, r, http.StatusOK, toFacultyResponse(f))
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
