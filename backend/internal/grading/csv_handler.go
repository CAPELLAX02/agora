package grading

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

// registerCSV, not tablosunun CSV dışa aktarma ve bir bileşene CSV'den not aktarma uçlarıdır.
// Dışa aktarma not tablosunu görme yetkisiyle, içe aktarma not yazma yetkisiyle aynıdır.
func (h *GradebookHandler) registerCSV(rt *authz.Router) {
	rt.HandleFunc("GET /api/v1/sections/{id}/gradebook.csv", authz.Authenticated, h.exportCSV)
	rt.HandleFunc("POST /api/v1/assessment-components/{id}/scores/import/preview", authz.Permission(permScore), h.previewImport)
	rt.HandleFunc("POST /api/v1/assessment-components/{id}/scores/import", authz.Permission(permScore), h.importScores)
}

func (h *GradebookHandler) exportCSV(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return
	}
	lang := r.URL.Query().Get("lang")
	if lang != "" && lang != "tr" && lang != "en" {
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "lang", Message: "tr ya da en olmalı."}})
		return
	}
	g, err := h.svc.Gradebook(r.Context(), id, actorID(r))
	if h.gradebookFailed(w, r, err) {
		return
	}
	if !mayViewGradebook(r, g.SectionContext) {
		h.problem(w, r, http.StatusForbidden, "FORBIDDEN", "Bu şubenin notlarını sadece öğretim elemanları ve yetkili birimler görebilir.")
		return
	}
	// Önce belleğe: yazarken hata olursa yarım dosya yerine 500 dönülür.
	var buf bytes.Buffer
	if err := WriteGradebookCSV(&buf, g, lang == "en"); err != nil {
		h.serverError(w, r, err)
		return
	}
	suffix := "notlar"
	if lang == "en" {
		suffix = "grades"
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s_%s_%s.csv"`,
		g.CourseCode, g.SectionCode, g.TermCode, suffix))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	w.WriteHeader(http.StatusOK)
	_, _ = buf.WriteTo(w)
}

// --- İçe aktarma ---------------------------------------------------------------

type importRowResponse struct {
	Line         int                     `json:"line"`
	StudentNo    string                  `json:"student_no"`
	Student      *studentRefResponse     `json:"student"`
	EnrollmentID *string                 `json:"enrollment_id"`
	Status       ImportStatus            `json:"status"`
	Current      *componentScoreResponse `json:"current"`
	Next         *componentScoreResponse `json:"next"`
	ErrorCode    *string                 `json:"error_code"`
	Error        *string                 `json:"error"`
}

type importSummaryResponse struct {
	Changed   int `json:"changed"`
	Unchanged int `json:"unchanged"`
	Empty     int `json:"empty"`
	Errors    int `json:"errors"`
}

type scoreImportResponse struct {
	ComponentID string                `json:"component_id"`
	Version     int                   `json:"version"`
	Column      *string               `json:"column"`
	Rows        []importRowResponse   `json:"rows"`
	Missing     []studentRefResponse  `json:"missing"`
	Summary     importSummaryResponse `json:"summary"`
}

func toScoreResponse(s *Score) *componentScoreResponse {
	if s == nil {
		return nil
	}
	res := &componentScoreResponse{Absent: s.Absent}
	if !s.Absent {
		v := s.Points.Float()
		res.Score = &v
	}
	return res
}

func toStudentRef(st GradebookStudent) studentRefResponse {
	return studentRefResponse{ID: st.StudentID, StudentNo: st.StudentNo, FirstName: st.FirstName, LastName: st.LastName}
}

func toImportRowResponse(row ImportRow) importRowResponse {
	res := importRowResponse{
		Line: row.Line, StudentNo: row.StudentNo, Status: row.Status,
		Current: toScoreResponse(row.Current), Next: toScoreResponse(row.Next),
		ErrorCode: optional(row.ErrorCode), Error: optional(row.Error),
	}
	if row.Student != nil {
		ref := toStudentRef(*row.Student)
		res.Student, res.EnrollmentID = &ref, &row.Student.EnrollmentID
	}
	return res
}

func toScoreImportResponse(si ScoreImport) scoreImportResponse {
	res := scoreImportResponse{
		ComponentID: si.ComponentID, Version: si.Version, Column: optional(si.Column),
		Rows:    make([]importRowResponse, 0, len(si.Rows)),
		Missing: make([]studentRefResponse, 0, len(si.Missing)),
		Summary: importSummaryResponse{Changed: si.Changed, Unchanged: si.Unchanged, Empty: si.Empty, Errors: si.Errors},
	}
	for _, row := range si.Rows {
		res.Rows = append(res.Rows, toImportRowResponse(row))
	}
	for _, st := range si.Missing {
		res.Missing = append(res.Missing, toStudentRef(st))
	}
	return res
}

func (h *GradebookHandler) previewImport(w http.ResponseWriter, r *http.Request) {
	ref, ok := h.component(w, r)
	if !ok {
		return
	}
	data, err := httpx.ReadBody(w, r, "text/csv")
	if err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	si, err := h.svc.PreviewImport(r.Context(), ref, data)
	if h.importFailed(w, r, err) {
		return
	}
	h.writeJSON(w, r, http.StatusOK, toScoreImportResponse(si))
}

func (h *GradebookHandler) importScores(w http.ResponseWriter, r *http.Request) {
	version, ok := httpx.IfMatchVersion(r)
	if !ok {
		httpx.PreconditionRequired(w, r)
		return
	}
	ref, ok := h.component(w, r)
	if !ok {
		return
	}
	data, err := httpx.ReadBody(w, r, "text/csv")
	if err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	if _, err := h.svc.ImportScores(r.Context(), actorID(r), ref, version, data); h.importFailed(w, r, err) {
		return
	}
	h.writeGradebook(w, r, ref.Section.SectionID)
}

func (h *GradebookHandler) importFailed(w http.ResponseWriter, r *http.Request, err error) bool {
	var (
		csvErr  *CSVError
		rowsErr *ImportRowsError
	)
	switch {
	case errors.As(err, &csvErr):
		h.problem(w, r, http.StatusBadRequest, "INVALID_CSV", csvErr.Detail)
	case errors.As(err, &rowsErr):
		fields := make([]httpx.FieldError, 0, len(rowsErr.Rows))
		for _, row := range rowsErr.Rows {
			fields = append(fields, httpx.FieldError{
				Field: "rows." + strconv.Itoa(row.Line), Code: row.ErrorCode, Message: row.Error,
			})
		}
		httpx.ValidationFailed(w, r, fields)
	default:
		return h.gradebookFailed(w, r, err)
	}
	return true
}
