package curriculum

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/CAPELLAX02/agora/backend/internal/org"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

func (h *Handler) registerCurricula(rt *authz.Router) {
	rt.HandleFunc("GET /api/v1/programs/{id}/curricula", authz.Permission(permCurriculumRead), h.listCurricula)
	rt.HandleFunc("POST /api/v1/programs/{id}/curricula", authz.Permission(permCurriculumManage), h.createCurriculum)
	rt.HandleFunc("GET /api/v1/curricula/{id}", authz.Permission(permCurriculumRead), h.getCurriculum)
	rt.HandleFunc("PUT /api/v1/curricula/{id}", authz.Permission(permCurriculumManage), h.updateCurriculum)
	rt.HandleFunc("DELETE /api/v1/curricula/{id}", authz.Permission(permCurriculumManage), h.deleteCurriculum)
	rt.HandleFunc("POST /api/v1/curricula/{id}/activate", authz.Permission(permCurriculumManage), h.activateCurriculum)
	rt.HandleFunc("POST /api/v1/curricula/{id}/archive", authz.Permission(permCurriculumManage), h.archiveCurriculum)
	rt.HandleFunc("POST /api/v1/curricula/{id}/items", authz.Permission(permCurriculumManage), h.addItem)
	rt.HandleFunc("PUT /api/v1/curricula/{id}/items/{itemId}", authz.Permission(permCurriculumManage), h.updateItem)
	rt.HandleFunc("DELETE /api/v1/curricula/{id}/items/{itemId}", authz.Permission(permCurriculumManage), h.deleteItem)

	rt.HandleFunc("GET /api/v1/me/curricula", authz.Permission(permCurriculumRead), h.myCurricula)
}

// --- Yanıt tipleri -------------------------------------------------------------

type programRefResponse struct {
	ID     string `json:"id"`
	Code   string `json:"code"`
	NameTR string `json:"name_tr"`
	NameEN string `json:"name_en"`
}

type curriculumResponse struct {
	ID                string             `json:"id"`
	Program           programRefResponse `json:"program"`
	NameTR            string             `json:"name_tr"`
	NameEN            string             `json:"name_en"`
	EffectiveFromYear int                `json:"effective_from_year"`
	EffectiveToYear   *int               `json:"effective_to_year"`
	TotalECTSRequired float64            `json:"total_ects_required"`
	Status            string             `json:"status"`
	DecisionRef       *string            `json:"decision_ref"`
	CopiedFromID      *string            `json:"copied_from_id"`
	ActivatedAt       *time.Time         `json:"activated_at"`
	ArchivedAt        *time.Time         `json:"archived_at"`
	StudentCount      int                `json:"student_count"`
	Version           int                `json:"version"`
}

type curriculumItemResponse struct {
	ID               string             `json:"id"`
	SemesterNo       int                `json:"semester_no"`
	ItemType         string             `json:"item_type"`
	Code             string             `json:"code"`
	NameTR           string             `json:"name_tr"`
	NameEN           string             `json:"name_en"`
	Course           *courseRefResponse `json:"course"`
	CourseKind       *string            `json:"course_kind"`
	HasPrerequisites bool               `json:"has_prerequisites"`
	ElectiveGroup    *groupRefResponse  `json:"elective_group"`
	TheoryHours      int                `json:"theory_hours"`
	PracticeHours    int                `json:"practice_hours"`
	NationalCredit   float64            `json:"national_credit"`
	ECTS             float64            `json:"ects"`
	IsCompulsory     bool               `json:"is_compulsory"`
	Position         int                `json:"position"`
}

type semesterSummaryResponse struct {
	SemesterNo     int     `json:"semester_no"`
	ECTS           float64 `json:"ects"`
	NationalCredit float64 `json:"national_credit"`
	ItemCount      int     `json:"item_count"`
}

type kindSummaryResponse struct {
	Kind string  `json:"kind"`
	ECTS float64 `json:"ects"`
}

type curriculumSummaryResponse struct {
	TotalECTS      float64                   `json:"total_ects"`
	TotalCredit    float64                   `json:"total_credit"`
	CompulsoryECTS float64                   `json:"compulsory_ects"`
	ElectiveECTS   float64                   `json:"elective_ects"`
	Semesters      []semesterSummaryResponse `json:"semesters"`
	ElectiveKinds  []kindSummaryResponse     `json:"elective_kinds"`
}

type curriculumDetailResponse struct {
	curriculumResponse
	Items   []curriculumItemResponse  `json:"items"`
	Summary curriculumSummaryResponse `json:"summary"`
}

type myCurriculumResponse struct {
	StudentProgramID string                    `json:"student_program_id"`
	EnrollmentKind   string                    `json:"enrollment_kind"`
	AdmissionYear    int                       `json:"admission_year"`
	Program          programRefResponse        `json:"program"`
	Curriculum       *curriculumDetailResponse `json:"curriculum"`
}

func toProgramRef(p ProgramRef) programRefResponse {
	return programRefResponse{ID: p.ID, Code: p.Code, NameTR: p.NameTR, NameEN: p.NameEN}
}

func toCurriculumResponse(c Curriculum) curriculumResponse {
	return curriculumResponse{
		ID: c.ID, Program: toProgramRef(c.Program), NameTR: c.NameTR, NameEN: c.NameEN,
		EffectiveFromYear: c.EffectiveFromYear, EffectiveToYear: c.EffectiveToYear, TotalECTSRequired: c.TotalECTSRequired,
		Status: string(c.Status), DecisionRef: optional(c.DecisionRef), CopiedFromID: optional(c.CopiedFromID),
		ActivatedAt: c.ActivatedAt, ArchivedAt: c.ArchivedAt, StudentCount: c.StudentCount, Version: c.Version,
	}
}

func toCurriculumItemResponse(it CurriculumItem) curriculumItemResponse {
	res := curriculumItemResponse{
		ID: it.ID, SemesterNo: it.SemesterNo, ItemType: string(it.Type), Code: it.Code(),
		HasPrerequisites: it.HasPrerequisites, TheoryHours: it.TheoryHours, PracticeHours: it.PracticeHours,
		NationalCredit: it.NationalCredit, ECTS: it.ECTS, IsCompulsory: it.IsCompulsory, Position: it.Position,
	}
	if it.Course != nil {
		c := toCourseRef(*it.Course)
		kind := string(it.CourseKind)
		res.Course, res.CourseKind, res.NameTR, res.NameEN = &c, &kind, c.NameTR, c.NameEN
	}
	if it.Group != nil {
		g := toGroupRef(*it.Group)
		res.ElectiveGroup, res.NameTR, res.NameEN = &g, g.NameTR, g.NameEN
	}
	return res
}

func toSummaryResponse(s CurriculumSummary) curriculumSummaryResponse {
	res := curriculumSummaryResponse{
		TotalECTS: round1(s.TotalECTS), TotalCredit: round1(s.TotalCredit),
		CompulsoryECTS: round1(s.CompulsoryECTS), ElectiveECTS: round1(s.ElectiveECTS),
		Semesters:     make([]semesterSummaryResponse, 0, len(s.Semesters)),
		ElectiveKinds: make([]kindSummaryResponse, 0, len(s.ElectiveKinds)),
	}
	for _, sem := range s.Semesters {
		res.Semesters = append(res.Semesters, semesterSummaryResponse{
			SemesterNo: sem.SemesterNo, ECTS: round1(sem.ECTS), NationalCredit: round1(sem.NationalCredit), ItemCount: sem.ItemCount,
		})
	}
	for _, k := range s.ElectiveKinds {
		res.ElectiveKinds = append(res.ElectiveKinds, kindSummaryResponse{Kind: string(k.Kind), ECTS: round1(k.ECTS)})
	}
	return res
}

// round1, toplamaların kayan nokta artığını (239.99999) bir ondalığa yuvarlar.
func round1(v float64) float64 {
	return math.Round(v*10) / 10
}

// --- Okuma ---------------------------------------------------------------------

// programTarget, yoldaki programın yetki hedefini çözer. Program yoksa 404 yazar.
func (h *Handler) programTarget(w http.ResponseWriter, r *http.Request) (string, authz.Target, bool) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return "", authz.Target{}, false
	}
	t, err := h.targets.Program(r.Context(), id)
	if errors.Is(err, org.ErrNotFound) {
		httpx.NotFound(w, r)
		return "", authz.Target{}, false
	}
	if err != nil {
		h.serverError(w, r, err)
		return "", authz.Target{}, false
	}
	return id, t, true
}

func curriculumTarget(c Curriculum) authz.Target {
	return authz.Target{FacultyID: c.Program.FacultyID, DepartmentID: c.Program.DepartmentID, ProgramID: c.Program.ID}
}

func (h *Handler) listCurricula(w http.ResponseWriter, r *http.Request) {
	programID, target, ok := h.programTarget(w, r)
	if !ok {
		return
	}
	// Taslakları sadece programın müfredatını yönetebilenler görür.
	list, err := h.store.ProgramCurricula(r.Context(), programID, mayAccess(r, permCurriculumManage, target))
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	h.writeJSON(w, r, http.StatusOK, httpx.ListResponse[curriculumResponse]{Items: mapSlice(list, toCurriculumResponse)})
}

// loadCurriculum, yoldaki müfredatı bulur. Taslak sürüm, yönetme yetkisi olmayana 404'tür.
func (h *Handler) loadCurriculum(w http.ResponseWriter, r *http.Request) (Curriculum, bool) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return Curriculum{}, false
	}
	c, err := h.store.Curriculum(r.Context(), id)
	if errors.Is(err, ErrNotFound) || (err == nil && c.Status == CurriculumDraft && !mayAccess(r, permCurriculumManage, curriculumTarget(c))) {
		httpx.NotFound(w, r)
		return Curriculum{}, false
	}
	if err != nil {
		h.serverError(w, r, err)
		return Curriculum{}, false
	}
	return c, true
}

func (h *Handler) detail(r *http.Request, c Curriculum) (curriculumDetailResponse, error) {
	items, err := h.store.CurriculumItems(r.Context(), c.ID)
	if err != nil {
		return curriculumDetailResponse{}, err
	}
	return curriculumDetailResponse{
		curriculumResponse: toCurriculumResponse(c),
		Items:              mapSlice(items, toCurriculumItemResponse),
		Summary:            toSummaryResponse(Summarize(items)),
	}, nil
}

func (h *Handler) writeCurriculumDetail(w http.ResponseWriter, r *http.Request, status int, id string) {
	c, err := h.store.Curriculum(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.NotFound(w, r)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	res, err := h.detail(r, c)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	w.Header().Set("ETag", httpx.ETag(c.Version))
	h.writeJSON(w, r, status, res)
}

func (h *Handler) getCurriculum(w http.ResponseWriter, r *http.Request) {
	c, ok := h.loadCurriculum(w, r)
	if !ok {
		return
	}
	h.writeCurriculumDetail(w, r, http.StatusOK, c.ID)
}

// myCurricula, öğrencinin her program kaydı için izlediği müfredatı döndürür (çift anadal
// ve yandal öğrencisinin birden fazla ders planı olur). Öğrenci değilse liste boştur.
func (h *Handler) myCurricula(w http.ResponseWriter, r *http.Request) {
	p, _ := authn.PrincipalFrom(r.Context())
	refs, err := h.store.StudentCurricula(r.Context(), p.UserID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	res := httpx.ListResponse[myCurriculumResponse]{Items: make([]myCurriculumResponse, 0, len(refs))}
	for _, ref := range refs {
		item := myCurriculumResponse{
			StudentProgramID: ref.StudentProgramID, EnrollmentKind: ref.EnrollmentKind,
			AdmissionYear: ref.AdmissionYear, Program: toProgramRef(ref.Program),
		}
		if ref.CurriculumID != "" {
			c, err := h.store.Curriculum(r.Context(), ref.CurriculumID)
			if err != nil {
				h.serverError(w, r, err)
				return
			}
			d, err := h.detail(r, c)
			if err != nil {
				h.serverError(w, r, err)
				return
			}
			item.Curriculum = &d
		}
		res.Items = append(res.Items, item)
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

// --- Sürüm yaşam döngüsü -------------------------------------------------------

type curriculumRequest struct {
	NameTR            string  `json:"name_tr"`
	NameEN            string  `json:"name_en"`
	EffectiveFromYear int     `json:"effective_from_year"`
	EffectiveToYear   *int    `json:"effective_to_year"`
	TotalECTSRequired float64 `json:"total_ects_required"`
	DecisionRef       string  `json:"decision_ref"`
	CopyFromID        string  `json:"copy_from_id"`
}

func (req curriculumRequest) validate() (CurriculumInput, []httpx.FieldError) {
	in := CurriculumInput{
		NameTR: strings.TrimSpace(req.NameTR), NameEN: strings.TrimSpace(req.NameEN),
		EffectiveFromYear: req.EffectiveFromYear, EffectiveToYear: req.EffectiveToYear,
		TotalECTSRequired: req.TotalECTSRequired, DecisionRef: strings.TrimSpace(req.DecisionRef),
	}
	var errs []httpx.FieldError
	errs = append(errs, requiredName("name_tr", in.NameTR)...)
	errs = append(errs, requiredName("name_en", in.NameEN)...)
	if in.EffectiveFromYear < 1946 || in.EffectiveFromYear > 2100 {
		errs = append(errs, httpx.FieldError{Field: "effective_from_year", Message: "1946 ile 2100 arasında olmalı."})
	}
	if in.EffectiveToYear != nil && (*in.EffectiveToYear < in.EffectiveFromYear || *in.EffectiveToYear > 2100) {
		errs = append(errs, httpx.FieldError{Field: "effective_to_year", Message: "Başlangıç yılından önce olamaz."})
	}
	if v := in.TotalECTSRequired; v <= 0 || v > 999 || math.Round(v*10) != v*10 {
		errs = append(errs, httpx.FieldError{Field: "total_ects_required", Message: "0'dan büyük, en fazla bir ondalık basamaklı olmalı."})
	}
	if utf8.RuneCountInString(in.DecisionRef) > 200 {
		errs = append(errs, httpx.FieldError{Field: "decision_ref", Message: "En fazla 200 karakter."})
	}
	return in, errs
}

func (h *Handler) createCurriculum(w http.ResponseWriter, r *http.Request) {
	programID, target, ok := h.programTarget(w, r)
	if !ok {
		return
	}
	var req curriculumRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	in, errs := req.validate()
	if req.CopyFromID != "" && !httpx.ValidUUID(req.CopyFromID) {
		errs = append(errs, httpx.FieldError{Field: "copy_from_id", Message: "Geçerli bir UUID olmalı."})
	}
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	if !h.allowedTarget(w, r, permCurriculumManage, target) {
		return
	}
	if req.CopyFromID != "" {
		src, err := h.store.Curriculum(r.Context(), req.CopyFromID)
		if errors.Is(err, ErrNotFound) || (err == nil && src.Status == CurriculumDraft && !mayAccess(r, permCurriculumManage, curriculumTarget(src))) {
			httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "copy_from_id", Message: "Kopyalanacak müfredat bulunamadı."}})
			return
		}
		if err != nil {
			h.serverError(w, r, err)
			return
		}
	}
	id, err := h.store.CreateCurriculum(r.Context(), actorID(r), programID, in, req.CopyFromID)
	if h.curriculumWriteFailed(w, r, err) {
		return
	}
	w.Header().Set("Location", "/api/v1/curricula/"+id)
	h.writeCurriculumDetail(w, r, http.StatusCreated, id)
}

func (h *Handler) updateCurriculum(w http.ResponseWriter, r *http.Request) {
	version, ok := httpx.IfMatchVersion(r)
	if !ok {
		httpx.PreconditionRequired(w, r)
		return
	}
	c, ok := h.loadCurriculum(w, r)
	if !ok {
		return
	}
	var req curriculumRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	in, errs := req.validate()
	if req.CopyFromID != "" {
		errs = append(errs, httpx.FieldError{Field: "copy_from_id", Message: "Sadece oluşturmada verilir."})
	}
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	if !h.allowedTarget(w, r, permCurriculumManage, curriculumTarget(c)) {
		return
	}
	if h.curriculumWriteFailed(w, r, h.store.UpdateCurriculum(r.Context(), actorID(r), c.ID, version, in)) {
		return
	}
	h.writeCurriculumDetail(w, r, http.StatusOK, c.ID)
}

func (h *Handler) deleteCurriculum(w http.ResponseWriter, r *http.Request) {
	c, ok := h.loadCurriculum(w, r)
	if !ok || !h.allowedTarget(w, r, permCurriculumManage, curriculumTarget(c)) {
		return
	}
	if h.curriculumWriteFailed(w, r, h.store.DeleteCurriculum(r.Context(), actorID(r), c.ID)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) activateCurriculum(w http.ResponseWriter, r *http.Request) {
	c, ok := h.loadCurriculum(w, r)
	if !ok || !h.allowedTarget(w, r, permCurriculumManage, curriculumTarget(c)) {
		return
	}
	_, err := h.store.Activate(r.Context(), actorID(r), c.ID)
	if h.curriculumWriteFailed(w, r, err) {
		return
	}
	h.writeCurriculumDetail(w, r, http.StatusOK, c.ID)
}

func (h *Handler) archiveCurriculum(w http.ResponseWriter, r *http.Request) {
	c, ok := h.loadCurriculum(w, r)
	if !ok || !h.allowedTarget(w, r, permCurriculumManage, curriculumTarget(c)) {
		return
	}
	if h.curriculumWriteFailed(w, r, h.store.Archive(r.Context(), actorID(r), c.ID)) {
		return
	}
	h.writeCurriculumDetail(w, r, http.StatusOK, c.ID)
}

func (h *Handler) curriculumWriteFailed(w http.ResponseWriter, r *http.Request, err error) bool {
	var mismatch *TotalMismatchError
	switch {
	case err == nil:
		return false
	case errors.Is(err, ErrNotFound):
		httpx.NotFound(w, r)
	case errors.Is(err, ErrVersionMismatch):
		httpx.PreconditionFailed(w, r)
	case errors.Is(err, ErrNotDraft):
		h.problem(w, r, http.StatusConflict, "CURRICULUM_NOT_DRAFT",
			"Bu değişiklik sadece taslak sürümde yapılabilir. Yürürlükteki sürümde sadece adlar, karar bilgisi ve yeni girişlere kapanış yılı değişir.")
	case errors.Is(err, ErrNotActive):
		h.problem(w, r, http.StatusConflict, "CURRICULUM_NOT_ACTIVE", "Sadece yürürlükteki sürüm arşivlenebilir.")
	case errors.Is(err, ErrCurriculumOverlap):
		h.problem(w, r, http.StatusConflict, "CURRICULUM_OVERLAP", "Giriş yılları programın yürürlükteki başka bir müfredat sürümüyle çakışıyor.")
	case errors.Is(err, ErrCurriculumInUse):
		h.problem(w, r, http.StatusConflict, "CURRICULUM_IN_USE", "Bu sürümü izleyen ve öğrenimi süren öğrenciler var.")
	case errors.Is(err, ErrEmptyCurriculum):
		h.problem(w, r, http.StatusConflict, "CURRICULUM_EMPTY", "Müfredatta hiç ders yok.")
	case errors.Is(err, ErrSemesterRange):
		h.problem(w, r, http.StatusConflict, "SEMESTER_OUT_OF_RANGE", "Bazı satırlar programın süresinden sonraki bir yarıyılda.")
	case errors.As(err, &mismatch):
		h.problem(w, r, http.StatusConflict, "ECTS_TOTAL_MISMATCH",
			fmt.Sprintf("Satırların AKTS toplamı %.1f, müfredatın gerektirdiği %.1f.", mismatch.Actual, mismatch.Required))
	case errors.Is(err, ErrDuplicate):
		h.problem(w, r, http.StatusConflict, "COURSE_ALREADY_IN_CURRICULUM", "Bu ders müfredatta zaten var.")
	case errors.Is(err, ErrUnknownReference):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "course_id", Message: "Ders ya da seçmeli grup bulunamadı."}})
	default:
		h.serverError(w, r, err)
	}
	return true
}

// --- Satırlar ------------------------------------------------------------------

type itemRequest struct {
	SemesterNo      int     `json:"semester_no"`
	ItemType        string  `json:"item_type"`
	CourseID        string  `json:"course_id"`
	ElectiveGroupID string  `json:"elective_group_id"`
	TheoryHours     int     `json:"theory_hours"`
	PracticeHours   int     `json:"practice_hours"`
	NationalCredit  float64 `json:"national_credit"`
	ECTS            float64 `json:"ects"`
	IsCompulsory    *bool   `json:"is_compulsory"`
	Position        int     `json:"position"`
}

func (req itemRequest) validate() (ItemInput, []httpx.FieldError) {
	in := ItemInput{
		SemesterNo: req.SemesterNo, Type: ItemType(req.ItemType), CourseID: req.CourseID, GroupID: req.ElectiveGroupID,
		TheoryHours: req.TheoryHours, PracticeHours: req.PracticeHours, NationalCredit: req.NationalCredit, ECTS: req.ECTS,
		IsCompulsory: req.IsCompulsory == nil || *req.IsCompulsory, Position: req.Position,
	}
	var errs []httpx.FieldError
	if in.SemesterNo < 1 || in.SemesterNo > 12 {
		errs = append(errs, httpx.FieldError{Field: "semester_no", Message: "1 ile 12 arasında olmalı."})
	}
	if in.Position < 0 || in.Position > 999 {
		errs = append(errs, httpx.FieldError{Field: "position", Message: "0 ile 999 arasında olmalı."})
	}
	switch in.Type {
	case ItemCourse:
		if !httpx.ValidUUID(in.CourseID) {
			errs = append(errs, httpx.FieldError{Field: "course_id", Message: "Ders satırında zorunlu."})
		}
		if in.GroupID != "" {
			errs = append(errs, httpx.FieldError{Field: "elective_group_id", Message: "Ders satırında boş bırakılmalı."})
		}
	case ItemElectiveSlot:
		if !httpx.ValidUUID(in.GroupID) {
			errs = append(errs, httpx.FieldError{Field: "elective_group_id", Message: "Seçmeli yuvada zorunlu."})
		}
		if in.CourseID != "" {
			errs = append(errs, httpx.FieldError{Field: "course_id", Message: "Seçmeli yuvada boş bırakılmalı."})
		}
		if req.IsCompulsory != nil && *req.IsCompulsory {
			errs = append(errs, httpx.FieldError{Field: "is_compulsory", Message: "Seçmeli yuva zorunlu olamaz."})
		}
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
	default:
		errs = append(errs, httpx.FieldError{Field: "item_type", Message: "COURSE ya da ELECTIVE_SLOT olmalı."})
	}
	return in, errs
}

// itemTarget, satır uçlarının ortak girişidir: müfredatı bulur, isteği doğrular ve
// programın müfredatını yönetme yetkisine bakar.
func (h *Handler) itemTarget(w http.ResponseWriter, r *http.Request, withBody bool) (Curriculum, ItemInput, bool) {
	c, ok := h.loadCurriculum(w, r)
	if !ok {
		return Curriculum{}, ItemInput{}, false
	}
	var in ItemInput
	if withBody {
		var req itemRequest
		if err := httpx.ReadJSON(w, r, &req); err != nil {
			httpx.InvalidBody(w, r, err)
			return Curriculum{}, ItemInput{}, false
		}
		var errs []httpx.FieldError
		if in, errs = req.validate(); len(errs) > 0 {
			httpx.ValidationFailed(w, r, errs)
			return Curriculum{}, ItemInput{}, false
		}
	}
	if !h.allowedTarget(w, r, permCurriculumManage, curriculumTarget(c)) {
		return Curriculum{}, ItemInput{}, false
	}
	return c, in, true
}

func (h *Handler) addItem(w http.ResponseWriter, r *http.Request) {
	c, in, ok := h.itemTarget(w, r, true)
	if !ok {
		return
	}
	_, err := h.store.AddItem(r.Context(), actorID(r), c.ID, in)
	if h.curriculumWriteFailed(w, r, err) {
		return
	}
	h.writeCurriculumDetail(w, r, http.StatusCreated, c.ID)
}

func (h *Handler) updateItem(w http.ResponseWriter, r *http.Request) {
	itemID := r.PathValue("itemId")
	if !httpx.ValidUUID(itemID) {
		httpx.NotFound(w, r)
		return
	}
	c, in, ok := h.itemTarget(w, r, true)
	if !ok {
		return
	}
	if h.curriculumWriteFailed(w, r, h.store.UpdateItem(r.Context(), actorID(r), c.ID, itemID, in)) {
		return
	}
	h.writeCurriculumDetail(w, r, http.StatusOK, c.ID)
}

func (h *Handler) deleteItem(w http.ResponseWriter, r *http.Request) {
	itemID := r.PathValue("itemId")
	if !httpx.ValidUUID(itemID) {
		httpx.NotFound(w, r)
		return
	}
	c, _, ok := h.itemTarget(w, r, false)
	if !ok {
		return
	}
	if h.curriculumWriteFailed(w, r, h.store.DeleteItem(r.Context(), actorID(r), c.ID, itemID)) {
		return
	}
	h.writeCurriculumDetail(w, r, http.StatusOK, c.ID)
}
