package enrollment

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

// registerHolds, kayıt engeli ve program kaydı durumu route'larını kaydeder.
func (h *Handler) registerHolds(rt *authz.Router) {
	rt.HandleFunc("POST /api/v1/student-programs/{id}/status", authz.Permission("student_program:manage"), h.changeStatus)
	rt.HandleFunc("GET /api/v1/student-programs/{id}/status-history", authz.Authenticated, h.statusHistory)
	// Öğrenci kendi engellerini, danışman danışmanı olduğu öğrencinin engellerini görür.
	rt.HandleFunc("GET /api/v1/student-programs/{id}/holds", authz.Authenticated, h.programHolds)
	rt.HandleFunc("POST /api/v1/student-programs/{id}/holds", authz.Permission("hold:manage"), h.placeHold)
	rt.HandleFunc("GET /api/v1/holds", authz.Permission("hold:manage"), h.listHolds)
	rt.HandleFunc("POST /api/v1/holds/{id}/release", authz.Permission("hold:manage"), h.releaseHold)
	rt.HandleFunc("GET /api/v1/student-programs/{id}/history", authz.Authenticated, h.courseHistory)
}

type holdProgramResponse struct {
	ID     string `json:"id"`
	Code   string `json:"code"`
	NameTR string `json:"name_tr"`
}

type holdResponse struct {
	ID               string              `json:"id"`
	StudentProgramID string              `json:"student_program_id"`
	Student          studentRefResponse  `json:"student"`
	Program          holdProgramResponse `json:"program"`
	HoldType         string              `json:"hold_type"`
	Blocks           []string            `json:"blocks"`
	Reason           string              `json:"reason"`
	PlacedBy         *string             `json:"placed_by"`
	PlacedAt         time.Time           `json:"placed_at"`
	ReleasedAt       *time.Time          `json:"released_at"`
	ReleasedBy       *string             `json:"released_by"`
	ReleaseReason    *string             `json:"release_reason"`
	Active           bool                `json:"active"`
}

func toHoldResponse(x Hold) holdResponse {
	res := holdResponse{
		ID: x.ID, StudentProgramID: x.StudentProgramID,
		Student:  studentRefResponse{ID: x.StudentID, StudentNo: x.StudentNo, FirstName: x.FirstName, LastName: x.LastName},
		Program:  holdProgramResponse{ID: x.Program.ID, Code: x.Program.Code, NameTR: x.Program.NameTR},
		HoldType: string(x.Type), Blocks: make([]string, 0, len(x.Blocks)), Reason: x.Reason,
		PlacedBy: optional(x.PlacedBy), PlacedAt: x.PlacedAt,
		ReleasedAt: x.ReleasedAt, ReleasedBy: optional(x.ReleasedBy), ReleaseReason: optional(x.ReleaseReason),
		Active: x.Active(),
	}
	for _, b := range x.Blocks {
		res.Blocks = append(res.Blocks, string(b))
	}
	return res
}

type statusChangeResponse struct {
	FromStatus  *string   `json:"from_status"`
	ToStatus    string    `json:"to_status"`
	Reason      *string   `json:"reason"`
	DecisionRef *string   `json:"decision_ref"`
	ChangedBy   *string   `json:"changed_by"`
	ChangedAt   time.Time `json:"changed_at"`
}

// --- Program kaydı durumu ------------------------------------------------------------

type changeStatusRequest struct {
	Status      string  `json:"status"`
	Reason      string  `json:"reason"`
	DecisionRef *string `json:"decision_ref"`
	GraduatedOn *string `json:"graduated_on"`
}

func (h *Handler) changeStatus(w http.ResponseWriter, r *http.Request) {
	sp, ok := h.studentProgram(w, r)
	if !ok {
		return
	}
	if !h.allowed(w, r, "student_program:manage", sp.Program.Target()) {
		return
	}
	version, ok := httpx.IfMatchVersion(r)
	if !ok {
		httpx.PreconditionRequired(w, r)
		return
	}

	var req changeStatusRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	in := StatusChangeInput{
		StudentProgramID: sp.ID, Version: version, To: Status(req.Status), Reason: strings.TrimSpace(req.Reason),
	}
	var errs []httpx.FieldError
	add := func(field, msg string) { errs = append(errs, httpx.FieldError{Field: field, Message: msg}) }
	if !in.To.Valid() {
		add("status", "Geçersiz durum.")
	}
	if in.Reason == "" || utf8.RuneCountInString(in.Reason) > 500 {
		add("reason", "Gerekçe zorunlu, en fazla 500 karakter.")
	}
	if req.DecisionRef != nil {
		in.DecisionRef = strings.TrimSpace(*req.DecisionRef)
		if utf8.RuneCountInString(in.DecisionRef) > 100 {
			add("decision_ref", "En fazla 100 karakter.")
		}
	}
	switch {
	case in.To == StatusGraduated && req.GraduatedOn == nil:
		add("graduated_on", "Mezuniyet tarihi zorunlu.")
	case in.To != StatusGraduated && req.GraduatedOn != nil:
		add("graduated_on", "Sadece mezuniyette verilir.")
	case req.GraduatedOn != nil:
		t, err := time.Parse(time.DateOnly, *req.GraduatedOn)
		if err != nil {
			add("graduated_on", "YYYY-AA-GG biçiminde bir tarih olmalı.")
		}
		in.GraduatedOn = &t
	}
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}

	err := h.service.ChangeStatus(r.Context(), actorID(r), in)
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.NotFound(w, r)
		return
	case errors.Is(err, ErrVersionMismatch):
		httpx.PreconditionFailed(w, r)
		return
	case errors.Is(err, ErrInvalidTransition):
		h.problem(w, r, http.StatusConflict, "INVALID_STATUS_TRANSITION",
			"Kayıt "+string(sp.Status)+" durumundan "+string(in.To)+" durumuna geçirilemez.")
		return
	case err != nil:
		h.serverError(w, r, err)
		return
	}

	updated, err := h.repo.StudentProgram(r.Context(), sp.ID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	w.Header().Set("ETag", httpx.ETag(updated.Version))
	h.writeJSON(w, r, http.StatusOK, toStudentProgramResponse(updated))
}

func (h *Handler) statusHistory(w http.ResponseWriter, r *http.Request) {
	sp, ok := h.studentProgram(w, r)
	if !ok {
		return
	}
	if _, ok := h.readableStudent(w, r, sp.StudentID); !ok {
		return
	}
	list, err := h.repo.StatusHistory(r.Context(), sp.ID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	res := httpx.ListResponse[statusChangeResponse]{Items: make([]statusChangeResponse, 0, len(list))}
	for _, c := range list {
		item := statusChangeResponse{
			ToStatus: string(c.To), Reason: optional(c.Reason), DecisionRef: optional(c.DecisionRef),
			ChangedBy: optional(c.ChangedBy), ChangedAt: c.ChangedAt,
		}
		if c.From != nil {
			from := string(*c.From)
			item.FromStatus = &from
		}
		res.Items = append(res.Items, item)
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

// --- Kayıt engelleri -------------------------------------------------------------------

func (h *Handler) programHolds(w http.ResponseWriter, r *http.Request) {
	sp, ok := h.studentProgram(w, r)
	if !ok {
		return
	}
	// Engel yöneticisi (mali işler) öğrenci bilgisini okuma yetkisi olmadan da engelleri görür.
	perms, _ := authz.PermissionsFrom(r.Context())
	if !perms.Allows("hold:manage", sp.Program.Target()) {
		if _, ok := h.readableStudent(w, r, sp.StudentID); !ok {
			return
		}
	}
	list, err := h.repo.Holds(r.Context(), sp.ID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	res := httpx.ListResponse[holdResponse]{Items: make([]holdResponse, 0, len(list))}
	for _, x := range list {
		res.Items = append(res.Items, toHoldResponse(x))
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

type placeHoldRequest struct {
	HoldType string   `json:"hold_type"`
	Blocks   []string `json:"blocks"`
	Reason   string   `json:"reason"`
}

func (h *Handler) placeHold(w http.ResponseWriter, r *http.Request) {
	sp, ok := h.studentProgram(w, r)
	if !ok {
		return
	}
	if !h.allowed(w, r, "hold:manage", sp.Program.Target()) {
		return
	}
	var req placeHoldRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	t, reason := HoldType(req.HoldType), strings.TrimSpace(req.Reason)
	var errs []httpx.FieldError
	if !t.Valid() {
		errs = append(errs, httpx.FieldError{Field: "hold_type", Message: "Geçersiz engel türü."})
	}
	blocks := t.DefaultBlocks()
	if req.Blocks != nil {
		blocks = nil
		for _, b := range req.Blocks {
			hb := HoldBlock(b)
			if !hb.Valid() || slices.Contains(blocks, hb) {
				errs = append(errs, httpx.FieldError{Field: "blocks", Message: "REGISTRATION, DOCUMENT ya da GRADUATION olmalı, tekrarsız."})
				break
			}
			blocks = append(blocks, hb)
		}
		if len(req.Blocks) == 0 {
			errs = append(errs, httpx.FieldError{Field: "blocks", Message: "En az bir işlem seçilmeli."})
		}
	}
	if reason == "" || utf8.RuneCountInString(reason) > 500 {
		errs = append(errs, httpx.FieldError{Field: "reason", Message: "Gerekçe zorunlu, en fazla 500 karakter."})
	}
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}

	id, err := h.service.PlaceHold(r.Context(), actorID(r), sp.ID, t, blocks, reason)
	switch {
	case errors.Is(err, ErrHoldExists):
		h.problem(w, r, http.StatusConflict, "HOLD_EXISTS", "Bu öğrencinin aynı türde aktif bir engeli var.")
		return
	case errors.Is(err, ErrNotActive):
		h.problem(w, r, http.StatusConflict, "ENROLLMENT_NOT_ONGOING", "Mezun olmuş ya da ayrılmış bir kayda engel konmaz.")
		return
	case errors.Is(err, ErrNotFound):
		httpx.NotFound(w, r)
		return
	case err != nil:
		h.serverError(w, r, err)
		return
	}
	x, err := h.repo.Hold(r.Context(), id)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	h.writeJSON(w, r, http.StatusCreated, toHoldResponse(x))
}

func (h *Handler) listHolds(w http.ResponseWriter, r *http.Request) {
	perms, _ := authz.PermissionsFrom(r.Context())
	q := r.URL.Query()
	var errs []httpx.FieldError

	f := HoldFilter{
		Scope:        perms.ScopesOf("hold:manage"),
		FacultyID:    q.Get("faculty_id"),
		DepartmentID: q.Get("department_id"),
		ProgramID:    q.Get("program_id"),
		Type:         HoldType(q.Get("type")),
		Active:       true,
		Query:        strings.TrimSpace(q.Get("q")),
	}
	for field, v := range map[string]string{"faculty_id": f.FacultyID, "department_id": f.DepartmentID, "program_id": f.ProgramID} {
		if v != "" && !httpx.ValidUUID(v) {
			errs = append(errs, httpx.FieldError{Field: field, Message: "geçerli bir UUID olmalı"})
		}
	}
	if f.Type != "" && !f.Type.Valid() {
		errs = append(errs, httpx.FieldError{Field: "type", Message: "geçersiz engel türü"})
	}
	switch q.Get("active") {
	case "", "true":
	case "false":
		f.Active = false
	default:
		errs = append(errs, httpx.FieldError{Field: "active", Message: "true ya da false olmalı"})
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
		cur, err := httpx.DecodeCursor[HoldCursor](c)
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

	items, hasMore, err := h.repo.ListHolds(r.Context(), f)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	res := httpx.ListResponse[holdResponse]{Items: make([]holdResponse, 0, len(items))}
	for _, x := range items {
		res.Items = append(res.Items, toHoldResponse(x))
	}
	if hasMore {
		last := items[len(items)-1]
		if res.NextCursor, err = httpx.EncodeCursor(HoldCursor{PlacedAt: last.PlacedAt, ID: last.ID}); err != nil {
			h.serverError(w, r, err)
			return
		}
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

type releaseHoldRequest struct {
	Reason string `json:"reason"`
}

func (h *Handler) releaseHold(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return
	}
	x, err := h.repo.Hold(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.NotFound(w, r)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	if !h.allowed(w, r, "hold:manage", x.Program.Target()) {
		return
	}

	var req releaseHoldRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" || utf8.RuneCountInString(reason) > 500 {
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "reason", Message: "Gerekçe zorunlu, en fazla 500 karakter."}})
		return
	}

	err = h.service.ReleaseHold(r.Context(), actorID(r), id, reason)
	switch {
	case errors.Is(err, ErrHoldReleased):
		h.problem(w, r, http.StatusConflict, "HOLD_RELEASED", "Engel zaten kaldırılmış.")
		return
	case err != nil:
		h.serverError(w, r, err)
		return
	}
	x, err = h.repo.Hold(r.Context(), id)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	h.writeJSON(w, r, http.StatusOK, toHoldResponse(x))
}

// --- Ders geçmişi ----------------------------------------------------------------------

type attemptTermResponse struct {
	ID   string `json:"id"`
	Code string `json:"code"`
}

type attemptCourseResponse struct {
	ID     string `json:"id"`
	Code   string `json:"code"`
	NameTR string `json:"name_tr"`
	NameEN string `json:"name_en"`
}

type courseAttemptResponse struct {
	ID             string                `json:"id"`
	Term           attemptTermResponse   `json:"term"`
	Course         attemptCourseResponse `json:"course"`
	SectionCode    string                `json:"section_code"`
	AttemptNo      int                   `json:"attempt_no"`
	Status         string                `json:"status"`
	LetterGrade    *string               `json:"letter_grade"`
	Coefficient    *float64              `json:"coefficient"`
	IsPassing      bool                  `json:"is_passing"`
	CountsInGPA    bool                  `json:"counts_in_gpa"`
	AttendanceFail bool                  `json:"attendance_fail"`
	ECTS           float64               `json:"ects"`
	Credit         float64               `json:"credit"`
	Source         string                `json:"source"`
	Superseded     bool                  `json:"superseded"`
}

func (h *Handler) courseHistory(w http.ResponseWriter, r *http.Request) {
	sp, ok := h.studentProgram(w, r)
	if !ok {
		return
	}
	if _, ok := h.readableStudent(w, r, sp.StudentID); !ok {
		return
	}
	list, err := h.repo.CourseHistory(r.Context(), sp.ID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	res := httpx.ListResponse[courseAttemptResponse]{Items: make([]courseAttemptResponse, 0, len(list))}
	for _, a := range list {
		res.Items = append(res.Items, courseAttemptResponse{
			ID:   a.ID,
			Term: attemptTermResponse{ID: a.TermID, Code: a.TermCode},
			Course: attemptCourseResponse{
				ID: a.CourseID, Code: a.CourseCode, NameTR: a.CourseNameTR, NameEN: a.CourseNameEN,
			},
			SectionCode: a.SectionCode, AttemptNo: a.AttemptNo, Status: string(a.Status),
			LetterGrade: optional(a.Letter), Coefficient: a.Coefficient, IsPassing: a.Passing,
			CountsInGPA: a.CountsInGPA, AttendanceFail: a.AbsenceFail, ECTS: a.ECTS, Credit: a.Credit,
			Source: a.Source, Superseded: a.Superseded,
		})
	}
	h.writeJSON(w, r, http.StatusOK, res)
}
