package curriculum

import (
	"context"
	"errors"
	"log/slog"
	"math"
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
	permCourseRead       = "course:read"
	permCourseManage     = "course:manage"
	permCurriculumRead   = "curriculum:read"
	permCurriculumManage = "curriculum:manage"
)

// Store, katalog ve müfredat işlemleridir. *Repository bunu sağlar.
type Store interface {
	ListCourses(ctx context.Context, f CourseFilter) ([]Course, bool, error)
	Course(ctx context.Context, id string) (Course, error)
	CourseDetail(ctx context.Context, id string) (CourseDetail, error)
	CreateCourse(ctx context.Context, actorID string, in CourseInput) (string, error)
	UpdateCourse(ctx context.Context, actorID, id string, version int, in CourseInput) error
	SetPrerequisites(ctx context.Context, actorID, courseID string, items []PrerequisiteInput) error
	AddEquivalence(ctx context.Context, actorID, courseID string, in EquivalenceInput) (string, error)
	DeleteEquivalence(ctx context.Context, actorID, courseID, equivalenceID string) error

	ListGroups(ctx context.Context, f GroupFilter) ([]ElectiveGroup, error)
	Group(ctx context.Context, id string) (ElectiveGroup, error)
	GroupCourses(ctx context.Context, groupID string) ([]Course, error)
	CreateGroup(ctx context.Context, actorID string, in GroupInput) (string, error)
	UpdateGroup(ctx context.Context, actorID, id string, version int, in GroupInput) error
	AddGroupCourse(ctx context.Context, actorID, groupID, courseID string) error
	RemoveGroupCourse(ctx context.Context, actorID, groupID, courseID string) error
}

// TargetResolver, birimlerin yetki hedeflerini çözer. *org.Targets bunu sağlar.
type TargetResolver interface {
	Department(ctx context.Context, id string) (authz.Target, error)
}

// Handler, ders kataloğu ve müfredat uçlarını sunar.
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
	rt.HandleFunc("GET /api/v1/courses", authz.Permission(permCourseRead), h.listCourses)
	rt.HandleFunc("POST /api/v1/courses", authz.Permission(permCourseManage), h.createCourse)
	rt.HandleFunc("GET /api/v1/courses/{id}", authz.Permission(permCourseRead), h.getCourse)
	rt.HandleFunc("PUT /api/v1/courses/{id}", authz.Permission(permCourseManage), h.updateCourse)
	rt.HandleFunc("PUT /api/v1/courses/{id}/prerequisites", authz.Permission(permCourseManage), h.setPrerequisites)
	rt.HandleFunc("POST /api/v1/courses/{id}/equivalences", authz.Permission(permCourseManage), h.addEquivalence)
	rt.HandleFunc("DELETE /api/v1/courses/{id}/equivalences/{equivalenceId}", authz.Permission(permCourseManage), h.deleteEquivalence)

	rt.HandleFunc("GET /api/v1/elective-groups", authz.Permission(permCurriculumRead), h.listGroups)
	rt.HandleFunc("POST /api/v1/elective-groups", authz.Permission(permCurriculumManage), h.createGroup)
	rt.HandleFunc("GET /api/v1/elective-groups/{id}", authz.Permission(permCurriculumRead), h.getGroup)
	rt.HandleFunc("PUT /api/v1/elective-groups/{id}", authz.Permission(permCurriculumManage), h.updateGroup)
	rt.HandleFunc("PUT /api/v1/elective-groups/{id}/courses/{courseId}", authz.Permission(permCurriculumManage), h.addGroupCourse)
	rt.HandleFunc("DELETE /api/v1/elective-groups/{id}/courses/{courseId}", authz.Permission(permCurriculumManage), h.removeGroupCourse)
}

// --- Yanıt tipleri -------------------------------------------------------------

type departmentRefResponse struct {
	ID     string `json:"id"`
	Code   string `json:"code"`
	NameTR string `json:"name_tr"`
	NameEN string `json:"name_en"`
}

type courseResponse struct {
	ID               string                 `json:"id"`
	Code             string                 `json:"code"`
	NameTR           string                 `json:"name_tr"`
	NameEN           string                 `json:"name_en"`
	OwnerDepartment  *departmentRefResponse `json:"owner_department"`
	TheoryHours      int                    `json:"theory_hours"`
	PracticeHours    int                    `json:"practice_hours"`
	NationalCredit   float64                `json:"national_credit"`
	ECTS             float64                `json:"ects"`
	Language         string                 `json:"language"`
	Kind             string                 `json:"kind"`
	GradingMode      string                 `json:"grading_mode"`
	DescriptionTR    *string                `json:"description_tr"`
	DescriptionEN    *string                `json:"description_en"`
	LearningOutcomes []string               `json:"learning_outcomes"`
	IsActive         bool                   `json:"is_active"`
	Version          int                    `json:"version"`
}

type courseRefResponse struct {
	ID     string  `json:"id"`
	Code   string  `json:"code"`
	NameTR string  `json:"name_tr"`
	NameEN string  `json:"name_en"`
	ECTS   float64 `json:"ects"`
}

type prerequisiteResponse struct {
	Course      courseRefResponse `json:"course"`
	Requirement string            `json:"requirement"`
	GroupNo     int               `json:"group_no"`
}

type equivalenceResponse struct {
	ID              string            `json:"id"`
	Relation        string            `json:"relation"`
	Course          courseRefResponse `json:"course"`
	IsBidirectional bool              `json:"is_bidirectional"`
	ValidFromYear   *int              `json:"valid_from_year"`
	Note            *string           `json:"note"`
}

type groupRefResponse struct {
	ID     string `json:"id"`
	Code   string `json:"code"`
	NameTR string `json:"name_tr"`
	NameEN string `json:"name_en"`
	Kind   string `json:"kind"`
}

type courseDetailResponse struct {
	courseResponse
	Prerequisites  []prerequisiteResponse `json:"prerequisites"`
	RequiredBy     []courseRefResponse    `json:"required_by"`
	Equivalences   []equivalenceResponse  `json:"equivalences"`
	ElectiveGroups []groupRefResponse     `json:"elective_groups"`
}

type groupResponse struct {
	ID              string                 `json:"id"`
	Code            string                 `json:"code"`
	NameTR          string                 `json:"name_tr"`
	NameEN          string                 `json:"name_en"`
	OwnerDepartment *departmentRefResponse `json:"owner_department"`
	Kind            string                 `json:"kind"`
	IsActive        bool                   `json:"is_active"`
	CourseCount     int                    `json:"course_count"`
	Version         int                    `json:"version"`
}

type groupDetailResponse struct {
	groupResponse
	Courses []courseResponse `json:"courses"`
}

func toDepartmentRef(d *DepartmentRef) *departmentRefResponse {
	if d == nil {
		return nil
	}
	return &departmentRefResponse{ID: d.ID, Code: d.Code, NameTR: d.NameTR, NameEN: d.NameEN}
}

func toCourseResponse(c Course) courseResponse {
	return courseResponse{
		ID: c.ID, Code: c.Code, NameTR: c.NameTR, NameEN: c.NameEN, OwnerDepartment: toDepartmentRef(c.OwnerDepartment),
		TheoryHours: c.TheoryHours, PracticeHours: c.PracticeHours, NationalCredit: c.NationalCredit, ECTS: c.ECTS,
		Language: c.Language, Kind: string(c.Kind), GradingMode: string(c.GradingMode),
		DescriptionTR: optional(c.DescriptionTR), DescriptionEN: optional(c.DescriptionEN),
		LearningOutcomes: c.LearningOutcomes, IsActive: c.IsActive, Version: c.Version,
	}
}

func toCourseRef(c CourseRef) courseRefResponse {
	return courseRefResponse(c)
}

func toGroupRef(g GroupRef) groupRefResponse {
	return groupRefResponse{ID: g.ID, Code: g.Code, NameTR: g.NameTR, NameEN: g.NameEN, Kind: string(g.Kind)}
}

func toCourseDetailResponse(d CourseDetail) courseDetailResponse {
	res := courseDetailResponse{
		courseResponse: toCourseResponse(d.Course),
		Prerequisites:  make([]prerequisiteResponse, 0, len(d.Prerequisites)),
		RequiredBy:     mapSlice(d.RequiredBy, toCourseRef),
		Equivalences:   make([]equivalenceResponse, 0, len(d.Equivalences)),
		ElectiveGroups: mapSlice(d.ElectiveGroups, toGroupRef),
	}
	for _, p := range d.Prerequisites {
		res.Prerequisites = append(res.Prerequisites, prerequisiteResponse{
			Course: toCourseRef(p.Course), Requirement: string(p.Requirement), GroupNo: p.GroupNo,
		})
	}
	for _, e := range d.Equivalences {
		res.Equivalences = append(res.Equivalences, equivalenceResponse{
			ID: e.ID, Relation: string(e.Relation), Course: toCourseRef(e.Course),
			IsBidirectional: e.IsBidirectional, ValidFromYear: e.ValidFromYear, Note: optional(e.Note),
		})
	}
	return res
}

func toGroupResponse(g ElectiveGroup) groupResponse {
	return groupResponse{
		ID: g.ID, Code: g.Code, NameTR: g.NameTR, NameEN: g.NameEN, OwnerDepartment: toDepartmentRef(g.OwnerDepartment),
		Kind: string(g.Kind), IsActive: g.IsActive, CourseCount: g.CourseCount, Version: g.Version,
	}
}

// --- Dersler -------------------------------------------------------------------

func (h *Handler) listCourses(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := CourseFilter{
		Query:           strings.TrimSpace(q.Get("q")),
		DepartmentID:    q.Get("department_id"),
		Kind:            CourseKind(q.Get("kind")),
		IncludeInactive: q.Get("include_inactive") == "true",
	}
	var errs []httpx.FieldError
	if utf8.RuneCountInString(f.Query) > 100 {
		errs = append(errs, httpx.FieldError{Field: "q", Message: "En fazla 100 karakter."})
	}
	if f.DepartmentID != "" && !httpx.ValidUUID(f.DepartmentID) {
		errs = append(errs, httpx.FieldError{Field: "department_id", Message: "Geçerli bir UUID olmalı."})
	}
	if f.Kind != "" && !f.Kind.Valid() {
		errs = append(errs, httpx.FieldError{Field: "kind", Message: "Geçersiz ders türü."})
	}
	limit, limitErr := httpx.ParseLimit(q)
	if limitErr != nil {
		errs = append(errs, *limitErr)
	}
	f.Limit = limit
	if c := q.Get("cursor"); c != "" {
		after, err := httpx.DecodeCursor[CourseCursor](c)
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
	courses, hasMore, err := h.store.ListCourses(r.Context(), f)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	res := httpx.ListResponse[courseResponse]{Items: mapSlice(courses, toCourseResponse)}
	if hasMore {
		if res.NextCursor, err = httpx.EncodeCursor(CourseCursor{Code: courses[len(courses)-1].Code}); err != nil {
			h.serverError(w, r, err)
			return
		}
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

func (h *Handler) getCourse(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return
	}
	h.writeCourseDetail(w, r, http.StatusOK, id)
}

func (h *Handler) writeCourseDetail(w http.ResponseWriter, r *http.Request, status int, id string) {
	d, err := h.store.CourseDetail(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.NotFound(w, r)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	w.Header().Set("ETag", httpx.ETag(d.Version))
	h.writeJSON(w, r, status, toCourseDetailResponse(d))
}

type courseRequest struct {
	Code              string   `json:"code"`
	OwnerDepartmentID string   `json:"owner_department_id"`
	NameTR            string   `json:"name_tr"`
	NameEN            string   `json:"name_en"`
	TheoryHours       int      `json:"theory_hours"`
	PracticeHours     int      `json:"practice_hours"`
	NationalCredit    float64  `json:"national_credit"`
	ECTS              float64  `json:"ects"`
	Language          string   `json:"language"`
	Kind              string   `json:"kind"`
	GradingMode       string   `json:"grading_mode"`
	DescriptionTR     string   `json:"description_tr"`
	DescriptionEN     string   `json:"description_en"`
	LearningOutcomes  []string `json:"learning_outcomes"`
	IsActive          *bool    `json:"is_active"`
}

var (
	courseCodePattern = regexp.MustCompile(`^[A-Z]{2,6}[0-9]{3,4}$`)
	groupCodePattern  = regexp.MustCompile(`^[A-Z0-9]{3,20}$`)
)

// validate, isteği doğrular. current, güncellenen derstir (oluşturmada nil).
func (req courseRequest) validate(current *Course) (CourseInput, []httpx.FieldError) {
	var errs []httpx.FieldError
	in := CourseInput{
		Code: strings.ToUpper(strings.TrimSpace(req.Code)), OwnerDepartmentID: req.OwnerDepartmentID,
		NameTR: strings.TrimSpace(req.NameTR), NameEN: strings.TrimSpace(req.NameEN),
		TheoryHours: req.TheoryHours, PracticeHours: req.PracticeHours,
		NationalCredit: req.NationalCredit, ECTS: req.ECTS, Language: req.Language,
		Kind: CourseKind(req.Kind), GradingMode: GradingMode(req.GradingMode),
		DescriptionTR: strings.TrimSpace(req.DescriptionTR), DescriptionEN: strings.TrimSpace(req.DescriptionEN),
		IsActive: req.IsActive == nil || *req.IsActive,
	}
	switch {
	case current == nil && !courseCodePattern.MatchString(in.Code):
		errs = append(errs, httpx.FieldError{Field: "code", Message: "2-6 büyük harf ve 3-4 rakam olmalı (ör. COM1001)."})
	case current != nil && in.Code != "" && in.Code != current.Code:
		errs = append(errs, httpx.FieldError{Field: "code", Message: "Ders kodu değiştirilemez; değişen ders yeni kodla açılıp eşdeğerlikle bağlanır."})
	}
	if current != nil && req.IsActive == nil {
		in.IsActive = current.IsActive
	}
	if in.OwnerDepartmentID != "" && !httpx.ValidUUID(in.OwnerDepartmentID) {
		errs = append(errs, httpx.FieldError{Field: "owner_department_id", Message: "Geçerli bir UUID olmalı."})
	}
	errs = append(errs, requiredName("name_tr", in.NameTR)...)
	errs = append(errs, requiredName("name_en", in.NameEN)...)
	for field, v := range map[string]int{"theory_hours": in.TheoryHours, "practice_hours": in.PracticeHours} {
		if v < 0 || v > 40 {
			errs = append(errs, httpx.FieldError{Field: field, Message: "0 ile 40 arasında olmalı."})
		}
	}
	for field, v := range map[string]float64{"national_credit": in.NationalCredit, "ects": in.ECTS} {
		if v < 0 || v > 60 || math.Round(v*10) != v*10 {
			errs = append(errs, httpx.FieldError{Field: field, Message: "0 ile 60 arasında, en fazla bir ondalık basamaklı olmalı."})
		}
	}
	if in.Language != "TR" && in.Language != "EN" {
		errs = append(errs, httpx.FieldError{Field: "language", Message: "TR ya da EN olmalı."})
	}
	if in.Kind == "" {
		in.Kind = KindRegular
	}
	if !in.Kind.Valid() {
		errs = append(errs, httpx.FieldError{Field: "kind", Message: "Geçersiz ders türü."})
	}
	if in.GradingMode == "" {
		in.GradingMode = GradingLetter
	}
	if !in.GradingMode.Valid() {
		errs = append(errs, httpx.FieldError{Field: "grading_mode", Message: "LETTER ya da PASS_FAIL olmalı."})
	}
	for field, v := range map[string]string{"description_tr": in.DescriptionTR, "description_en": in.DescriptionEN} {
		if utf8.RuneCountInString(v) > 4000 {
			errs = append(errs, httpx.FieldError{Field: field, Message: "En fazla 4000 karakter."})
		}
	}
	if len(req.LearningOutcomes) > 30 {
		errs = append(errs, httpx.FieldError{Field: "learning_outcomes", Message: "En fazla 30 öğrenme çıktısı."})
	}
	in.LearningOutcomes = make([]string, 0, len(req.LearningOutcomes))
	for _, o := range req.LearningOutcomes {
		o = strings.TrimSpace(o)
		if utf8.RuneCountInString(o) > 500 {
			errs = append(errs, httpx.FieldError{Field: "learning_outcomes", Message: "Her çıktı en fazla 500 karakter."})
			break
		}
		in.LearningOutcomes = append(in.LearningOutcomes, o)
	}
	return in, errs
}

func requiredName(field, v string) []httpx.FieldError {
	switch n := utf8.RuneCountInString(v); {
	case n == 0:
		return []httpx.FieldError{{Field: field, Message: "Zorunlu."}}
	case n > 200:
		return []httpx.FieldError{{Field: field, Message: "En fazla 200 karakter."}}
	}
	return nil
}

func (h *Handler) createCourse(w http.ResponseWriter, r *http.Request) {
	var req courseRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	in, errs := req.validate(nil)
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	if !h.allowed(w, r, permCourseManage, in.OwnerDepartmentID) {
		return
	}
	id, err := h.store.CreateCourse(r.Context(), actorID(r), in)
	if h.courseWriteFailed(w, r, err) {
		return
	}
	w.Header().Set("Location", "/api/v1/courses/"+id)
	h.writeCourseDetail(w, r, http.StatusCreated, id)
}

func (h *Handler) updateCourse(w http.ResponseWriter, r *http.Request) {
	version, ok := httpx.IfMatchVersion(r)
	if !ok {
		httpx.PreconditionRequired(w, r)
		return
	}
	current, ok := h.course(w, r)
	if !ok {
		return
	}
	var req courseRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	in, errs := req.validate(&current)
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	// Ders başka bölüme devredilirken hem eski hem yeni sahip yetki alanında olmalı.
	if !h.allowed(w, r, permCourseManage, ownerID(current.OwnerDepartment)) || !h.allowed(w, r, permCourseManage, in.OwnerDepartmentID) {
		return
	}
	if h.courseWriteFailed(w, r, h.store.UpdateCourse(r.Context(), actorID(r), current.ID, version, in)) {
		return
	}
	h.writeCourseDetail(w, r, http.StatusOK, current.ID)
}

type prerequisitesRequest struct {
	Items []struct {
		CourseID    string `json:"course_id"`
		Requirement string `json:"requirement"`
		GroupNo     int    `json:"group_no"`
	} `json:"items"`
}

func (h *Handler) setPrerequisites(w http.ResponseWriter, r *http.Request) {
	current, ok := h.course(w, r)
	if !ok {
		return
	}
	var req prerequisitesRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	var errs []httpx.FieldError
	if req.Items == nil {
		errs = append(errs, httpx.FieldError{Field: "items", Message: "Zorunlu; ön koşulları kaldırmak için boş dizi gönderin."})
	}
	if len(req.Items) > 20 {
		errs = append(errs, httpx.FieldError{Field: "items", Message: "En fazla 20 ön koşul."})
	}
	items := make([]PrerequisiteInput, 0, len(req.Items))
	seen := make(map[string]bool, len(req.Items))
	for _, it := range req.Items {
		in := PrerequisiteInput{CourseID: it.CourseID, Requirement: Requirement(it.Requirement), GroupNo: it.GroupNo}
		if in.Requirement == "" {
			in.Requirement = RequirePassed
		}
		if in.GroupNo == 0 {
			in.GroupNo = 1
		}
		switch {
		case !httpx.ValidUUID(in.CourseID):
			errs = append(errs, httpx.FieldError{Field: "items.course_id", Message: "Geçerli bir UUID olmalı."})
		case in.CourseID == current.ID:
			errs = append(errs, httpx.FieldError{Field: "items.course_id", Message: "Ders kendisinin ön koşulu olamaz."})
		case seen[in.CourseID]:
			errs = append(errs, httpx.FieldError{Field: "items.course_id", Message: "Aynı ders birden fazla kez verilemez."})
		}
		if !in.Requirement.Valid() {
			errs = append(errs, httpx.FieldError{Field: "items.requirement", Message: "PASSED ya da ATTENDED olmalı."})
		}
		if in.GroupNo < 1 || in.GroupNo > 20 {
			errs = append(errs, httpx.FieldError{Field: "items.group_no", Message: "1 ile 20 arasında olmalı."})
		}
		seen[in.CourseID] = true
		items = append(items, in)
	}
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	if !h.allowed(w, r, permCourseManage, ownerID(current.OwnerDepartment)) {
		return
	}
	err := h.store.SetPrerequisites(r.Context(), actorID(r), current.ID, items)
	switch {
	case errors.Is(err, ErrUnknownReference):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "items.course_id", Message: "Ön koşul dersi bulunamadı."}})
		return
	case errors.Is(err, ErrPrerequisiteCycle):
		h.problem(w, r, http.StatusConflict, "PREREQUISITE_CYCLE",
			"Bu ön koşullar bir döngü oluşturuyor: seçilen derslerden biri zaten bu dersi (doğrudan ya da dolaylı) ön koşul olarak istiyor.")
		return
	case h.courseWriteFailed(w, r, err):
		return
	}
	h.writeCourseDetail(w, r, http.StatusOK, current.ID)
}

type equivalenceRequest struct {
	EquivalentCourseID string `json:"equivalent_course_id"`
	IsBidirectional    *bool  `json:"is_bidirectional"`
	ValidFromYear      *int   `json:"valid_from_year"`
	Note               string `json:"note"`
}

func (h *Handler) addEquivalence(w http.ResponseWriter, r *http.Request) {
	current, ok := h.course(w, r)
	if !ok {
		return
	}
	var req equivalenceRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	in := EquivalenceInput{
		EquivalentCourseID: req.EquivalentCourseID, IsBidirectional: req.IsBidirectional == nil || *req.IsBidirectional,
		ValidFromYear: req.ValidFromYear, Note: strings.TrimSpace(req.Note),
	}
	var errs []httpx.FieldError
	switch {
	case !httpx.ValidUUID(in.EquivalentCourseID):
		errs = append(errs, httpx.FieldError{Field: "equivalent_course_id", Message: "Geçerli bir UUID olmalı."})
	case in.EquivalentCourseID == current.ID:
		errs = append(errs, httpx.FieldError{Field: "equivalent_course_id", Message: "Ders kendisine eşdeğer olamaz."})
	}
	if in.ValidFromYear != nil && (*in.ValidFromYear < 1946 || *in.ValidFromYear > 2100) {
		errs = append(errs, httpx.FieldError{Field: "valid_from_year", Message: "1946 ile 2100 arasında olmalı."})
	}
	if utf8.RuneCountInString(in.Note) > 500 {
		errs = append(errs, httpx.FieldError{Field: "note", Message: "En fazla 500 karakter."})
	}
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	if !h.allowed(w, r, permCourseManage, ownerID(current.OwnerDepartment)) {
		return
	}
	_, err := h.store.AddEquivalence(r.Context(), actorID(r), current.ID, in)
	switch {
	case errors.Is(err, ErrUnknownReference):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "equivalent_course_id", Message: "Ders bulunamadı."}})
		return
	case errors.Is(err, ErrDuplicate):
		h.problem(w, r, http.StatusConflict, "EQUIVALENCE_EXISTS", "Bu iki ders arasında zaten bir eşdeğerlik var.")
		return
	case h.courseWriteFailed(w, r, err):
		return
	}
	h.writeCourseDetail(w, r, http.StatusCreated, current.ID)
}

func (h *Handler) deleteEquivalence(w http.ResponseWriter, r *http.Request) {
	current, ok := h.course(w, r)
	if !ok {
		return
	}
	eqID := r.PathValue("equivalenceId")
	if !httpx.ValidUUID(eqID) {
		httpx.NotFound(w, r)
		return
	}
	if !h.allowed(w, r, permCourseManage, ownerID(current.OwnerDepartment)) {
		return
	}
	if h.courseWriteFailed(w, r, h.store.DeleteEquivalence(r.Context(), actorID(r), current.ID, eqID)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) courseWriteFailed(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, ErrNotFound):
		httpx.NotFound(w, r)
	case errors.Is(err, ErrVersionMismatch):
		httpx.PreconditionFailed(w, r)
	case errors.Is(err, ErrConflict):
		h.problem(w, r, http.StatusConflict, "COURSE_CODE_TAKEN", "Bu kodla bir ders zaten var.")
	case errors.Is(err, ErrUnknownReference):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "owner_department_id", Message: "Bölüm bulunamadı."}})
	default:
		h.serverError(w, r, err)
	}
	return true
}

func (h *Handler) course(w http.ResponseWriter, r *http.Request) (Course, bool) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return Course{}, false
	}
	c, err := h.store.Course(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.NotFound(w, r)
		return Course{}, false
	}
	if err != nil {
		h.serverError(w, r, err)
		return Course{}, false
	}
	return c, true
}

// --- Seçmeli gruplar -----------------------------------------------------------

func (h *Handler) listGroups(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := GroupFilter{Query: strings.TrimSpace(q.Get("q")), Kind: GroupKind(q.Get("kind")), DepartmentID: q.Get("department_id")}
	var errs []httpx.FieldError
	if utf8.RuneCountInString(f.Query) > 100 {
		errs = append(errs, httpx.FieldError{Field: "q", Message: "En fazla 100 karakter."})
	}
	if f.Kind != "" && !f.Kind.Valid() {
		errs = append(errs, httpx.FieldError{Field: "kind", Message: "Geçersiz grup türü."})
	}
	if f.DepartmentID != "" && !httpx.ValidUUID(f.DepartmentID) {
		errs = append(errs, httpx.FieldError{Field: "department_id", Message: "Geçerli bir UUID olmalı."})
	}
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	groups, err := h.store.ListGroups(r.Context(), f)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	h.writeJSON(w, r, http.StatusOK, httpx.ListResponse[groupResponse]{Items: mapSlice(groups, toGroupResponse)})
}

func (h *Handler) getGroup(w http.ResponseWriter, r *http.Request) {
	g, ok := h.group(w, r)
	if !ok {
		return
	}
	h.writeGroupDetail(w, r, http.StatusOK, g.ID)
}

func (h *Handler) writeGroupDetail(w http.ResponseWriter, r *http.Request, status int, id string) {
	g, err := h.store.Group(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.NotFound(w, r)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	courses, err := h.store.GroupCourses(r.Context(), id)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	w.Header().Set("ETag", httpx.ETag(g.Version))
	h.writeJSON(w, r, status, groupDetailResponse{groupResponse: toGroupResponse(g), Courses: mapSlice(courses, toCourseResponse)})
}

type groupRequest struct {
	Code              string `json:"code"`
	NameTR            string `json:"name_tr"`
	NameEN            string `json:"name_en"`
	OwnerDepartmentID string `json:"owner_department_id"`
	Kind              string `json:"kind"`
	IsActive          *bool  `json:"is_active"`
}

func (req groupRequest) validate(current *ElectiveGroup) (GroupInput, []httpx.FieldError) {
	var errs []httpx.FieldError
	in := GroupInput{
		Code: strings.ToUpper(strings.TrimSpace(req.Code)), NameTR: strings.TrimSpace(req.NameTR), NameEN: strings.TrimSpace(req.NameEN),
		OwnerDepartmentID: req.OwnerDepartmentID, Kind: GroupKind(req.Kind), IsActive: req.IsActive == nil || *req.IsActive,
	}
	switch {
	case current == nil && !groupCodePattern.MatchString(in.Code):
		errs = append(errs, httpx.FieldError{Field: "code", Message: "3-20 büyük harf ya da rakam olmalı (ör. COMTE02)."})
	case current != nil && in.Code != "" && in.Code != current.Code:
		errs = append(errs, httpx.FieldError{Field: "code", Message: "Grup kodu değiştirilemez."})
	}
	if current != nil && req.IsActive == nil {
		in.IsActive = current.IsActive
	}
	errs = append(errs, requiredName("name_tr", in.NameTR)...)
	errs = append(errs, requiredName("name_en", in.NameEN)...)
	if in.OwnerDepartmentID != "" && !httpx.ValidUUID(in.OwnerDepartmentID) {
		errs = append(errs, httpx.FieldError{Field: "owner_department_id", Message: "Geçerli bir UUID olmalı."})
	}
	if !in.Kind.Valid() {
		errs = append(errs, httpx.FieldError{Field: "kind", Message: "Geçersiz grup türü."})
	}
	return in, errs
}

func (h *Handler) createGroup(w http.ResponseWriter, r *http.Request) {
	var req groupRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	in, errs := req.validate(nil)
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	if !h.allowed(w, r, permCurriculumManage, in.OwnerDepartmentID) {
		return
	}
	id, err := h.store.CreateGroup(r.Context(), actorID(r), in)
	if h.groupWriteFailed(w, r, err) {
		return
	}
	w.Header().Set("Location", "/api/v1/elective-groups/"+id)
	h.writeGroupDetail(w, r, http.StatusCreated, id)
}

func (h *Handler) updateGroup(w http.ResponseWriter, r *http.Request) {
	version, ok := httpx.IfMatchVersion(r)
	if !ok {
		httpx.PreconditionRequired(w, r)
		return
	}
	current, ok := h.group(w, r)
	if !ok {
		return
	}
	var req groupRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	in, errs := req.validate(&current)
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	if !h.allowed(w, r, permCurriculumManage, ownerID(current.OwnerDepartment)) || !h.allowed(w, r, permCurriculumManage, in.OwnerDepartmentID) {
		return
	}
	if h.groupWriteFailed(w, r, h.store.UpdateGroup(r.Context(), actorID(r), current.ID, version, in)) {
		return
	}
	h.writeGroupDetail(w, r, http.StatusOK, current.ID)
}

func (h *Handler) addGroupCourse(w http.ResponseWriter, r *http.Request) {
	g, courseID, ok := h.groupCourse(w, r)
	if !ok {
		return
	}
	err := h.store.AddGroupCourse(r.Context(), actorID(r), g.ID, courseID)
	if errors.Is(err, ErrUnknownReference) {
		httpx.NotFound(w, r) // ders yok (grup yukarıda bulundu)
		return
	}
	if h.groupWriteFailed(w, r, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) removeGroupCourse(w http.ResponseWriter, r *http.Request) {
	g, courseID, ok := h.groupCourse(w, r)
	if !ok {
		return
	}
	if h.groupWriteFailed(w, r, h.store.RemoveGroupCourse(r.Context(), actorID(r), g.ID, courseID)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// groupCourse, havuz üyeliği uçlarının ortak girişidir: grubu bulur, ders kimliğini
// doğrular ve grubu yönetme yetkisine bakar.
func (h *Handler) groupCourse(w http.ResponseWriter, r *http.Request) (ElectiveGroup, string, bool) {
	g, ok := h.group(w, r)
	if !ok {
		return ElectiveGroup{}, "", false
	}
	courseID := r.PathValue("courseId")
	if !httpx.ValidUUID(courseID) {
		httpx.NotFound(w, r)
		return ElectiveGroup{}, "", false
	}
	if !h.allowed(w, r, permCurriculumManage, ownerID(g.OwnerDepartment)) {
		return ElectiveGroup{}, "", false
	}
	return g, courseID, true
}

func (h *Handler) groupWriteFailed(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, ErrNotFound):
		httpx.NotFound(w, r)
	case errors.Is(err, ErrVersionMismatch):
		httpx.PreconditionFailed(w, r)
	case errors.Is(err, ErrConflict):
		h.problem(w, r, http.StatusConflict, "GROUP_CODE_TAKEN", "Bu kodla bir seçmeli grup zaten var.")
	case errors.Is(err, ErrUnknownReference):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "owner_department_id", Message: "Bölüm bulunamadı."}})
	default:
		h.serverError(w, r, err)
	}
	return true
}

func (h *Handler) group(w http.ResponseWriter, r *http.Request) (ElectiveGroup, bool) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return ElectiveGroup{}, false
	}
	g, err := h.store.Group(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.NotFound(w, r)
		return ElectiveGroup{}, false
	}
	if err != nil {
		h.serverError(w, r, err)
		return ElectiveGroup{}, false
	}
	return g, true
}

// --- Yetki ---------------------------------------------------------------------

// allowed, yetkinin verilen bölümü kapsayıp kapsamadığına bakar. Bölüm boşsa kayıt
// üniversite geneli sayılır (ortak dersler, üniversite havuzları) ve sadece üniversite
// kapsamlı yetki yeter.
func (h *Handler) allowed(w http.ResponseWriter, r *http.Request, perm, departmentID string) bool {
	var target authz.Target
	if departmentID != "" {
		t, err := h.targets.Department(r.Context(), departmentID)
		if errors.Is(err, org.ErrNotFound) {
			httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "owner_department_id", Message: "Bölüm bulunamadı."}})
			return false
		}
		if err != nil {
			h.serverError(w, r, err)
			return false
		}
		target = t
	}
	perms, ok := authz.PermissionsFrom(r.Context())
	if ok && perms.Allows(perm, target) {
		return true
	}
	h.problem(w, r, http.StatusForbidden, "FORBIDDEN", "Bu bölümün kaydını yönetme yetkiniz yok.")
	return false
}

func ownerID(d *DepartmentRef) string {
	if d == nil {
		return ""
	}
	return d.ID
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
