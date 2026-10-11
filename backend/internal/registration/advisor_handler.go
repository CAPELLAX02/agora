package registration

import (
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/enrollment"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

// registrationProgram, kaydın program kaydını ve dönemini okur; isteği yapanın öğrenciyi
// görebildiğini doğrular. Erişim yoksa 404 yazar.
func (h *Handler) registrationProgram(w http.ResponseWriter, r *http.Request, id string) (sp enrollment.StudentProgram, termID string, ext *time.Time, ok bool) {
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return sp, "", nil, false
	}
	var spID string
	err := h.pool.QueryRow(r.Context(), `
		SELECT student_program_id, term_id, extension_until FROM enrollment.registrations WHERE id = $1`, id,
	).Scan(&spID, &termID, &ext)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.NotFound(w, r)
		return sp, "", nil, false
	}
	if err != nil {
		h.serverError(w, r, err)
		return sp, "", nil, false
	}
	sp, err = h.students.StudentProgram(r.Context(), spID)
	if err != nil {
		h.serverError(w, r, err)
		return sp, "", nil, false
	}
	principal, _ := authn.PrincipalFrom(r.Context())
	perms, _ := authz.PermissionsFrom(r.Context())
	readable, err := h.studentSvc.CanReadProgram(r.Context(), perms, principal.UserID, sp)
	if err != nil {
		h.serverError(w, r, err)
		return sp, "", nil, false
	}
	if !readable {
		httpx.NotFound(w, r)
		return sp, "", nil, false
	}
	return sp, termID, ext, true
}

func (h *Handler) getByID(w http.ResponseWriter, r *http.Request) {
	sp, termID, _, ok := h.registrationProgram(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	h.respond(w, r, sp, termID, http.StatusOK, nil)
}

type decisionRequest struct {
	Note string `json:"note"`
}

// decider, kararı verecek olanı belirler: danışman pencere açıkken kendisi olarak karar
// verir; pencere kapalıysa ya da danışman değilse öğrenci işleri müdahale (override) yapar.
func (h *Handler) decider(r *http.Request, sp enrollment.StudentProgram, termID string, ext *time.Time, a actor) (Decider, bool, error) {
	if !a.advisor && !a.override {
		return Decider{}, false, nil
	}
	d := Decider{UserID: a.userID}
	if a.override {
		usable := false
		if a.advisor {
			w, err := h.service.window(r.Context(), sp, termID, WindowApproval, ext)
			if err != nil {
				return Decider{}, false, err
			}
			usable = w.Usable()
		}
		d.Override = !usable
	}
	return d, true, nil
}

func (h *Handler) decide(decision Decision) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		sp, termID, ext, ok := h.registrationProgram(w, r, id)
		if !ok {
			return
		}
		a, err := h.actorFor(r, sp)
		if err != nil {
			h.serverError(w, r, err)
			return
		}
		d, allowed, err := h.decider(r, sp, termID, ext, a)
		if err != nil {
			h.serverError(w, r, err)
			return
		}
		if !allowed {
			h.problem(w, r, http.StatusForbidden, "FORBIDDEN", "Bu öğrencinin danışmanı değilsiniz.")
			return
		}
		version, ok := httpx.IfMatchVersion(r)
		if !ok {
			httpx.PreconditionRequired(w, r)
			return
		}
		var req decisionRequest
		if err := httpx.ReadJSON(w, r, &req); err != nil {
			httpx.InvalidBody(w, r, err)
			return
		}
		note := strings.TrimSpace(req.Note)
		if utf8.RuneCountInString(note) > 1000 {
			httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "note", Message: "En fazla 1000 karakter."}})
			return
		}
		if (decision != Approve || d.Override) && utf8.RuneCountInString(note) < 5 {
			httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "note", Message: "En az 5 karakterlik bir gerekçe yazın."}})
			return
		}
		if err := h.service.Decide(r.Context(), d, sp, id, version, decision, note); err != nil {
			h.writeError(w, r, err)
			return
		}
		h.respond(w, r, sp, termID, http.StatusOK, nil)
	}
}

type bulkRequest struct {
	Items []struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
	} `json:"items"`
	Note string `json:"note"`
}

type bulkResult struct {
	ID      string  `json:"id"`
	OK      bool    `json:"ok"`
	Code    *string `json:"code"`
	Message *string `json:"message"`
}

// bulkApprove, danışmanın seçtiği kayıtları tek tek (her biri kendi transaction'ında)
// onaylar ve her kaydın sonucunu döndürür: biri başarısız olunca diğerleri etkilenmez.
func (h *Handler) bulkApprove(w http.ResponseWriter, r *http.Request) {
	var req bulkRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	if len(req.Items) == 0 || len(req.Items) > 100 {
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "items", Message: "1-100 kayıt seçilmeli."}})
		return
	}
	note := strings.TrimSpace(req.Note)
	res := struct {
		Approved int          `json:"approved"`
		Failed   int          `json:"failed"`
		Results  []bulkResult `json:"results"`
	}{Results: []bulkResult{}}

	fail := func(id, code, msg string) {
		res.Failed++
		res.Results = append(res.Results, bulkResult{ID: id, Code: &code, Message: &msg})
	}
	for _, it := range req.Items {
		if !httpx.ValidUUID(it.ID) {
			fail(it.ID, "NOT_FOUND", "Kayıt bulunamadı.")
			continue
		}
		rec := &probeRecorder{header: http.Header{}}
		sp, termID, ext, ok := h.registrationProgram(rec, r, it.ID)
		if !ok {
			fail(it.ID, "NOT_FOUND", "Kayıt bulunamadı.")
			continue
		}
		a, err := h.actorFor(r, sp)
		if err != nil {
			h.serverError(w, r, err)
			return
		}
		d, allowed, err := h.decider(r, sp, termID, ext, a)
		if err != nil {
			h.serverError(w, r, err)
			return
		}
		if !allowed || d.Override {
			// Toplu onay danışman ekranıdır: pencere dışı müdahale tek tek, gerekçeyle yapılır.
			fail(it.ID, "FORBIDDEN", "Bu kaydı danışman olarak onaylayamazsınız (danışmanlık ya da onay penceresi).")
			continue
		}
		err = h.service.Decide(r.Context(), d, sp, it.ID, it.Version, Approve, note)
		if err != nil {
			code, msg := errorCode(err)
			if code == "" {
				h.serverError(w, r, err)
				return
			}
			fail(it.ID, code, msg)
			continue
		}
		res.Approved++
		res.Results = append(res.Results, bulkResult{ID: it.ID, OK: true})
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

// errorCode, beklenen iş hatalarının makine okunur kodu ve mesajıdır. Beklenmeyen hatada
// kod boştur.
func errorCode(err error) (string, string) {
	var re *RuleError
	switch {
	case errors.As(err, &re):
		for _, v := range re.Violations {
			if v.Severity == "ERROR" {
				return "RULE_VIOLATION", v.Message
			}
		}
		return "RULE_VIOLATION", "Ders seçme kuralları bu işleme izin vermiyor."
	case errors.Is(err, ErrVersionMismatch):
		return "VERSION_MISMATCH", "Kayıt siz incelerken değişmiş; yenileyip tekrar deneyin."
	case errors.Is(err, ErrInvalidState):
		return "INVALID_REGISTRATION_STATE", "Kayıt onay bekleyen durumda değil."
	case errors.Is(err, ErrWindowClosed):
		return "WINDOW_CLOSED", "Danışman onayı penceresi kapalı."
	case errors.Is(err, ErrGradesEntered):
		return "GRADES_ENTERED", "Kaydın derslerine not girilmiş; onay geri alınamaz."
	case errors.Is(err, ErrNotFound):
		return "NOT_FOUND", "Kayıt bulunamadı."
	}
	return "", ""
}

// probeRecorder, yanıtı yazmayan bir ResponseWriter'dır: toplu işlemde tek bir kaydın
// erişim denetimi isteğin yanıtını bozmasın diye.
type probeRecorder struct {
	header http.Header
}

func (p *probeRecorder) Header() http.Header         { return p.header }
func (p *probeRecorder) Write(b []byte) (int, error) { return len(b), nil }
func (p *probeRecorder) WriteHeader(int)             {}

// --- Listeler --------------------------------------------------------------------------------

type summaryStudentResponse struct {
	ID        string `json:"id"`
	StudentNo string `json:"student_no"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

type summaryResponse struct {
	StudentProgramID string                 `json:"student_program_id"`
	Student          summaryStudentResponse `json:"student"`
	ProgramCode      string                 `json:"program_code"`
	Semester         int                    `json:"semester"`
	GPA              *float64               `json:"gpa"`
	RegistrationID   *string                `json:"registration_id"`
	Status           string                 `json:"status"`
	TotalECTS        float64                `json:"total_ects"`
	ItemCount        int                    `json:"item_count"`
	SubmittedAt      *time.Time             `json:"submitted_at"`
	Version          int                    `json:"version"`
	Blocked          bool                   `json:"blocked"`
}

func toSummaryResponse(s Summary) summaryResponse {
	return summaryResponse{
		StudentProgramID: s.StudentProgramID,
		Student:          summaryStudentResponse{ID: s.StudentID, StudentNo: s.StudentNo, FirstName: s.FirstName, LastName: s.LastName},
		ProgramCode:      s.ProgramCode, Semester: s.Semester, GPA: s.GPA, RegistrationID: optional(s.RegistrationID),
		Status: string(s.Status), TotalECTS: s.TotalECTS, ItemCount: s.ItemCount, SubmittedAt: s.SubmittedAt,
		Version: s.Version, Blocked: s.Blocked,
	}
}

func statusParam(w http.ResponseWriter, r *http.Request) (Status, bool) {
	s := Status(r.URL.Query().Get("status"))
	switch s {
	case "", StatusNotStarted, StatusDraft, StatusSubmitted, StatusApproved, StatusRejected:
		return s, true
	}
	httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "status", Message: "geçersiz durum"}})
	return "", false
}

type summaryListResponse struct {
	Term       refResponse       `json:"term"`
	Items      []summaryResponse `json:"items"`
	NextCursor *string           `json:"next_cursor"`
}

func (h *Handler) adviseeRegistrations(w http.ResponseWriter, r *http.Request) {
	termID, ok := h.termParam(w, r)
	if !ok {
		return
	}
	status, ok := statusParam(w, r)
	if !ok {
		return
	}
	code, err := h.repo.TermCode(r.Context(), termID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	res := summaryListResponse{Term: refResponse{ID: termID, Code: code}, Items: []summaryResponse{}}
	principal, _ := authn.PrincipalFrom(r.Context())
	staffID, err := h.students.StaffByUser(r.Context(), principal.UserID)
	if errors.Is(err, enrollment.ErrNotFound) {
		h.writeJSON(w, r, http.StatusOK, res)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	list, err := h.repo.AdviseeRegistrations(r.Context(), staffID, termID, status)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	for _, s := range list {
		res.Items = append(res.Items, toSummaryResponse(s))
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

func (h *Handler) listRegistrations(w http.ResponseWriter, r *http.Request) {
	termID, ok := h.termParam(w, r)
	if !ok {
		return
	}
	status, ok := statusParam(w, r)
	if !ok {
		return
	}
	perms, _ := authz.PermissionsFrom(r.Context())
	q := r.URL.Query()
	f := ListFilter{
		Scope: perms.ScopesOf("registration:override"), TermID: termID, Status: status,
		FacultyID: q.Get("faculty_id"), DepartmentID: q.Get("department_id"), ProgramID: q.Get("program_id"),
		Query: strings.TrimSpace(q.Get("q")),
	}
	var errs []httpx.FieldError
	for field, v := range map[string]string{"faculty_id": f.FacultyID, "department_id": f.DepartmentID, "program_id": f.ProgramID} {
		if v != "" && !httpx.ValidUUID(v) {
			errs = append(errs, httpx.FieldError{Field: field, Message: "geçerli bir UUID olmalı"})
		}
	}
	if utf8.RuneCountInString(f.Query) > 100 {
		errs = append(errs, httpx.FieldError{Field: "q", Message: "en fazla 100 karakter olabilir"})
	}
	limit, limitErr := httpx.ParseLimit(q)
	if limitErr != nil {
		errs = append(errs, *limitErr)
	}
	f.Limit = limit
	if c := q.Get("cursor"); c != "" {
		cur, err := httpx.DecodeCursor[SummaryCursor](c)
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
	code, err := h.repo.TermCode(r.Context(), termID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	items, hasMore, err := h.repo.ListRegistrations(r.Context(), f)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	res := summaryListResponse{Term: refResponse{ID: termID, Code: code}, Items: make([]summaryResponse, 0, len(items))}
	for _, s := range items {
		res.Items = append(res.Items, toSummaryResponse(s))
	}
	if hasMore {
		last := items[len(items)-1]
		next, err := httpx.EncodeCursor(SummaryCursor{StudentNo: last.StudentNo, ID: last.StudentProgramID})
		if err != nil {
			h.serverError(w, r, err)
			return
		}
		res.NextCursor = &next
	}
	h.writeJSON(w, r, http.StatusOK, res)
}
