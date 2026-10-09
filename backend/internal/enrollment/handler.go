package enrollment

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

// Handler, öğrenci, program kaydı ve danışmanlık uçlarını sunar.
type Handler struct {
	repo    *Repository
	service *Service
	logger  *slog.Logger
}

// NewHandler, bir Handler oluşturur.
func NewHandler(repo *Repository, service *Service, logger *slog.Logger) *Handler {
	return &Handler{repo: repo, service: service, logger: logger}
}

// Register, route'ları kaydeder.
func (h *Handler) Register(rt *authz.Router) {
	rt.HandleFunc("GET /api/v1/students", authz.Permission("person:read"), h.listStudents)
	// Öğrenci kendini, danışman danışmanı olduğu öğrenciyi de görebilir: erişime
	// politika fonksiyonu karar verir, bu yüzden route sadece kimlik ister.
	rt.HandleFunc("GET /api/v1/students/{id}", authz.Authenticated, h.getStudent)
	rt.HandleFunc("POST /api/v1/students/{id}/programs", authz.Permission("student_program:manage"), h.createProgram)

	rt.HandleFunc("GET /api/v1/student-programs/{id}/advisors", authz.Authenticated, h.advisorHistory)
	rt.HandleFunc("PUT /api/v1/student-programs/{id}/advisor", authz.Permission("advisor:assign"), h.assignAdvisor)
	rt.HandleFunc("GET /api/v1/departments/{id}/advisors", authz.Permission("advisor:assign"), h.eligibleAdvisors)

	rt.HandleFunc("GET /api/v1/me/programs", authz.SelfService, h.myPrograms)
	rt.HandleFunc("GET /api/v1/me/advisees", authz.Authenticated, h.myAdvisees)
}

// --- Yanıt tipleri -----------------------------------------------------------

type programRefResponse struct {
	ID         string `json:"id"`
	Code       string `json:"code"`
	NameTR     string `json:"name_tr"`
	Department refRes `json:"department"`
	Faculty    refRes `json:"faculty"`
}

type refRes struct {
	ID     string `json:"id"`
	NameTR string `json:"name_tr"`
}

type advisorResponse struct {
	StaffID   string    `json:"staff_id"`
	StaffNo   string    `json:"staff_no"`
	Title     *string   `json:"title"`
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name"`
	Since     time.Time `json:"since"`
}

type studentProgramResponse struct {
	ID              string             `json:"id"`
	Student         studentRefResponse `json:"student"`
	Program         programRefResponse `json:"program"`
	Kind            string             `json:"kind"`
	AdmissionType   string             `json:"admission_type"`
	AdmissionYear   int                `json:"admission_year"`
	AdmittedOn      string             `json:"admitted_on"` // YYYY-MM-DD
	Status          string             `json:"status"`
	ClassLevel      int                `json:"class_level"`
	CurrentSemester int                `json:"current_semester"`
	GPA             *float64           `json:"gpa"`
	EarnedECTS      float64            `json:"earned_ects"`
	Advisor         *advisorResponse   `json:"advisor"`
	Curriculum      *curriculumRefRes  `json:"curriculum"`
}

type curriculumRefRes struct {
	ID     string `json:"id"`
	NameTR string `json:"name_tr"`
	NameEN string `json:"name_en"`
}

type studentRefResponse struct {
	ID        string `json:"id"`
	StudentNo string `json:"student_no"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

type studentResponse struct {
	ID        string                   `json:"id"`
	StudentNo string                   `json:"student_no"`
	FirstName string                   `json:"first_name"`
	LastName  string                   `json:"last_name"`
	Email     *string                  `json:"email"`
	Programs  []studentProgramResponse `json:"programs"`
}

type advisorHistoryResponse struct {
	advisorResponse
	Until      *time.Time `json:"until"`
	AssignedBy *string    `json:"assigned_by"`
	Reason     *string    `json:"reason"`
}

type eligibleAdvisorResponse struct {
	StaffID     string  `json:"staff_id"`
	StaffNo     string  `json:"staff_no"`
	Title       *string `json:"title"`
	FirstName   string  `json:"first_name"`
	LastName    string  `json:"last_name"`
	ActiveCount int     `json:"active_count"`
}

func toStudentProgramResponse(sp StudentProgram) studentProgramResponse {
	res := studentProgramResponse{
		ID:      sp.ID,
		Student: studentRefResponse{ID: sp.StudentID, StudentNo: sp.StudentNo, FirstName: sp.FirstName, LastName: sp.LastName},
		Program: programRefResponse{
			ID: sp.Program.ID, Code: sp.Program.Code, NameTR: sp.Program.NameTR,
			Department: refRes{ID: sp.Program.DepartmentID, NameTR: sp.Program.DepartmentName},
			Faculty:    refRes{ID: sp.Program.FacultyID, NameTR: sp.Program.FacultyName},
		},
		Kind: string(sp.Kind), AdmissionType: string(sp.AdmissionType), AdmissionYear: sp.AdmissionYear,
		AdmittedOn: sp.AdmittedOn.Format(time.DateOnly), Status: string(sp.Status),
		ClassLevel: sp.ClassLevel, CurrentSemester: sp.CurrentSemester, GPA: sp.GPA, EarnedECTS: sp.EarnedECTS,
	}
	if sp.Advisor != nil {
		a := toAdvisorResponse(*sp.Advisor)
		res.Advisor = &a
	}
	if sp.Curriculum != nil {
		res.Curriculum = &curriculumRefRes{ID: sp.Curriculum.ID, NameTR: sp.Curriculum.NameTR, NameEN: sp.Curriculum.NameEN}
	}
	return res
}

func toAdvisorResponse(a AdvisorRef) advisorResponse {
	return advisorResponse{
		StaffID: a.StaffID, StaffNo: a.StaffNo, Title: optional(a.Title),
		FirstName: a.FirstName, LastName: a.LastName, Since: a.Since,
	}
}

func toStudentResponse(s Student) studentResponse {
	res := studentResponse{
		ID: s.ID, StudentNo: s.StudentNo, FirstName: s.FirstName, LastName: s.LastName, Email: optional(s.Email),
		Programs: make([]studentProgramResponse, 0, len(s.Programs)),
	}
	for _, p := range s.Programs {
		res.Programs = append(res.Programs, toStudentProgramResponse(p))
	}
	return res
}

// --- Öğrenciler --------------------------------------------------------------

func (h *Handler) listStudents(w http.ResponseWriter, r *http.Request) {
	perms, _ := authz.PermissionsFrom(r.Context())
	q := r.URL.Query()
	var errs []httpx.FieldError

	f := StudentFilter{
		Scope:        perms.ScopesOf("person:read"),
		FacultyID:    q.Get("faculty_id"),
		DepartmentID: q.Get("department_id"),
		ProgramID:    q.Get("program_id"),
		Status:       Status(q.Get("status")),
		ClassLevel:   -1,
		Query:        strings.TrimSpace(q.Get("q")),
	}
	for field, v := range map[string]string{"faculty_id": f.FacultyID, "department_id": f.DepartmentID, "program_id": f.ProgramID} {
		if v != "" && !httpx.ValidUUID(v) {
			errs = append(errs, httpx.FieldError{Field: field, Message: "geçerli bir UUID olmalı"})
		}
	}
	if f.Status != "" && !f.Status.Valid() {
		errs = append(errs, httpx.FieldError{Field: "status", Message: "geçersiz durum"})
	}
	if v := q.Get("class_level"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 7 {
			errs = append(errs, httpx.FieldError{Field: "class_level", Message: "0 ile 7 arasında olmalı"})
		}
		f.ClassLevel = n
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
		cur, err := httpx.DecodeCursor[StudentCursor](c)
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

	items, hasMore, err := h.repo.ListStudentPrograms(r.Context(), f)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	res := httpx.ListResponse[studentProgramResponse]{Items: make([]studentProgramResponse, 0, len(items))}
	for _, sp := range items {
		res.Items = append(res.Items, toStudentProgramResponse(sp))
	}
	if hasMore {
		last := items[len(items)-1]
		if res.NextCursor, err = httpx.EncodeCursor(StudentCursor{StudentNo: last.StudentNo, ID: last.ID}); err != nil {
			h.serverError(w, r, err)
			return
		}
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

// getStudent, öğrenciyi program kayıtlarıyla döndürür. Erişim yetkisi yoksa 404 döner:
// öğrenci kimliğini tahmin eden biri, öyle bir öğrencinin var olup olmadığını öğrenemez.
func (h *Handler) getStudent(w http.ResponseWriter, r *http.Request) {
	st, ok := h.readableStudent(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	h.writeJSON(w, r, http.StatusOK, toStudentResponse(st))
}

func (h *Handler) readableStudent(w http.ResponseWriter, r *http.Request, studentID string) (Student, bool) {
	if !httpx.ValidUUID(studentID) {
		httpx.NotFound(w, r)
		return Student{}, false
	}
	st, err := h.repo.Student(r.Context(), studentID)
	if errors.Is(err, ErrNotFound) {
		httpx.NotFound(w, r)
		return Student{}, false
	}
	if err != nil {
		h.serverError(w, r, err)
		return Student{}, false
	}

	principal, _ := authn.PrincipalFrom(r.Context())
	perms, _ := authz.PermissionsFrom(r.Context())
	allowed, err := h.service.CanReadStudent(r.Context(), perms, principal.UserID, st)
	if err != nil {
		h.serverError(w, r, err)
		return Student{}, false
	}
	if !allowed {
		httpx.NotFound(w, r)
		return Student{}, false
	}
	return st, true
}

type createProgramRequest struct {
	ProgramID     string `json:"program_id"`
	Kind          string `json:"kind"`
	AdmissionType string `json:"admission_type"`
	AdmissionYear int    `json:"admission_year"`
	AdmittedOn    string `json:"admitted_on"`
	Status        string `json:"status"`
	ClassLevel    *int   `json:"class_level"`
}

func (h *Handler) createProgram(w http.ResponseWriter, r *http.Request) {
	studentID := r.PathValue("id")
	if !httpx.ValidUUID(studentID) {
		httpx.NotFound(w, r)
		return
	}
	var req createProgramRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	in, errs := req.validate(studentID)
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}

	target, err := h.repo.ProgramTarget(r.Context(), in.ProgramID)
	if errors.Is(err, ErrUnknownProgram) {
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "program_id", Message: "Program bulunamadı."}})
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	if !h.allowed(w, r, "student_program:manage", target) {
		return
	}

	id, err := h.service.CreateStudentProgram(r.Context(), actorID(r), in)
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.NotFound(w, r)
		return
	case errors.Is(err, ErrConflict):
		h.problem(w, r, http.StatusConflict, "ALREADY_ENROLLED", "Öğrenci bu programa zaten kayıtlı.")
		return
	case errors.Is(err, ErrProgramInactive):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "program_id", Message: "Program aktif değil."}})
		return
	case err != nil:
		h.serverError(w, r, err)
		return
	}

	sp, err := h.repo.StudentProgram(r.Context(), id)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/students/"+studentID)
	h.writeJSON(w, r, http.StatusCreated, toStudentProgramResponse(sp))
}

func (req createProgramRequest) validate(studentID string) (NewStudentProgram, []httpx.FieldError) {
	var errs []httpx.FieldError
	add := func(field, msg string) { errs = append(errs, httpx.FieldError{Field: field, Message: msg}) }

	in := NewStudentProgram{
		StudentID: studentID, ProgramID: req.ProgramID, Kind: Kind(req.Kind),
		AdmissionType: AdmissionType(req.AdmissionType), AdmissionYear: req.AdmissionYear,
		Status: Status(req.Status), ClassLevel: 1,
	}
	if in.Kind == "" {
		in.Kind = KindMajor
	}
	if in.Status == "" {
		in.Status = StatusActive
	}
	if req.ClassLevel != nil {
		in.ClassLevel = *req.ClassLevel
	}

	if !httpx.ValidUUID(in.ProgramID) {
		add("program_id", "Program zorunlu.")
	}
	if !in.Kind.Valid() {
		add("kind", "MAJOR, DOUBLE_MAJOR ya da MINOR olmalı.")
	}
	if !in.AdmissionType.Valid() {
		add("admission_type", "Geçersiz giriş türü.")
	}
	if in.AdmissionYear < 1946 || in.AdmissionYear > time.Now().Year()+1 {
		add("admission_year", "Geçerli bir yıl olmalı.")
	}
	t, err := time.Parse(time.DateOnly, req.AdmittedOn)
	if err != nil {
		add("admitted_on", "YYYY-AA-GG biçiminde bir tarih olmalı.")
	}
	in.AdmittedOn = t
	if in.Status != StatusActive && in.Status != StatusPrep {
		add("status", "Yeni kayıt ACTIVE ya da PREP olmalı.")
	}
	if in.ClassLevel < 0 || in.ClassLevel > 7 || (in.Status == StatusPrep) != (in.ClassLevel == 0) {
		add("class_level", "Hazırlık kaydında 0, diğerlerinde 1-7 olmalı.")
	}
	return in, errs
}

// --- Danışmanlık ---------------------------------------------------------------

type assignAdvisorRequest struct {
	StaffID string `json:"staff_id"`
	Reason  string `json:"reason"`
}

func (h *Handler) assignAdvisor(w http.ResponseWriter, r *http.Request) {
	sp, ok := h.studentProgram(w, r)
	if !ok {
		return
	}
	if !h.allowed(w, r, "advisor:assign", sp.Program.Target()) {
		return
	}

	var req assignAdvisorRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	var errs []httpx.FieldError
	if !httpx.ValidUUID(req.StaffID) {
		errs = append(errs, httpx.FieldError{Field: "staff_id", Message: "Danışman zorunlu."})
	}
	if req.Reason == "" || utf8.RuneCountInString(req.Reason) > 500 {
		errs = append(errs, httpx.FieldError{Field: "reason", Message: "Gerekçe zorunlu, en fazla 500 karakter."})
	}
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}

	err := h.service.AssignAdvisor(r.Context(), actorID(r), sp.ID, req.StaffID, req.Reason)
	switch {
	case errors.Is(err, ErrNotAnAdvisor):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "staff_id",
			Message: "Bu kişi programın bölümünde danışman olarak atanamaz (danışman rolü, aktif hesap ve görev gerekir)."}})
		return
	case errors.Is(err, ErrNotActive):
		h.problem(w, r, http.StatusConflict, "ENROLLMENT_NOT_ONGOING", "Mezun olmuş ya da ayrılmış bir kayda danışman atanamaz.")
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
	h.writeJSON(w, r, http.StatusOK, toStudentProgramResponse(updated))
}

func (h *Handler) advisorHistory(w http.ResponseWriter, r *http.Request) {
	sp, ok := h.studentProgram(w, r)
	if !ok {
		return
	}
	if _, ok := h.readableStudent(w, r, sp.StudentID); !ok {
		return
	}

	list, err := h.repo.AdvisorHistory(r.Context(), sp.ID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	res := httpx.ListResponse[advisorHistoryResponse]{Items: make([]advisorHistoryResponse, 0, len(list))}
	for _, a := range list {
		res.Items = append(res.Items, advisorHistoryResponse{
			advisorResponse: toAdvisorResponse(a.AdvisorRef),
			Until:           a.Until, AssignedBy: optional(a.AssignedBy), Reason: optional(a.Reason),
		})
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

func (h *Handler) eligibleAdvisors(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return
	}
	target, err := h.repo.DepartmentTarget(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.NotFound(w, r)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	if !h.allowed(w, r, "advisor:assign", target) {
		return
	}

	list, err := h.repo.EligibleAdvisors(r.Context(), id)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	res := httpx.ListResponse[eligibleAdvisorResponse]{Items: make([]eligibleAdvisorResponse, 0, len(list))}
	for _, a := range list {
		res.Items = append(res.Items, eligibleAdvisorResponse{
			StaffID: a.StaffID, StaffNo: a.StaffNo, Title: optional(a.Title),
			FirstName: a.FirstName, LastName: a.LastName, ActiveCount: a.ActiveCount,
		})
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

func (h *Handler) myPrograms(w http.ResponseWriter, r *http.Request) {
	principal, _ := authn.PrincipalFrom(r.Context())
	res := httpx.ListResponse[studentProgramResponse]{Items: []studentProgramResponse{}}

	studentID, err := h.repo.StudentByUser(r.Context(), principal.UserID)
	if errors.Is(err, ErrNotFound) {
		h.writeJSON(w, r, http.StatusOK, res) // öğrenci değil: boş liste
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	st, err := h.repo.Student(r.Context(), studentID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	for _, p := range st.Programs {
		res.Items = append(res.Items, toStudentProgramResponse(p))
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

func (h *Handler) myAdvisees(w http.ResponseWriter, r *http.Request) {
	principal, _ := authn.PrincipalFrom(r.Context())
	res := httpx.ListResponse[studentProgramResponse]{Items: []studentProgramResponse{}}

	staffID, err := h.repo.StaffByUser(r.Context(), principal.UserID)
	if errors.Is(err, ErrNotFound) {
		h.writeJSON(w, r, http.StatusOK, res)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	list, err := h.repo.Advisees(r.Context(), staffID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	for _, sp := range list {
		res.Items = append(res.Items, toStudentProgramResponse(sp))
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

// --- Ortak -------------------------------------------------------------------

func (h *Handler) studentProgram(w http.ResponseWriter, r *http.Request) (StudentProgram, bool) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return StudentProgram{}, false
	}
	sp, err := h.repo.StudentProgram(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.NotFound(w, r)
		return StudentProgram{}, false
	}
	if err != nil {
		h.serverError(w, r, err)
		return StudentProgram{}, false
	}
	return sp, true
}

// allowed, yetkinin hedefi kapsayıp kapsamadığına bakar.
func (h *Handler) allowed(w http.ResponseWriter, r *http.Request, permission string, t authz.Target) bool {
	perms, ok := authz.PermissionsFrom(r.Context())
	if ok && perms.Allows(permission, t) {
		return true
	}
	h.problem(w, r, http.StatusForbidden, "FORBIDDEN", "Bu birim için yetkiniz yok.")
	return false
}

func actorID(r *http.Request) string {
	p, _ := authn.PrincipalFrom(r.Context())
	return p.UserID
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

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
