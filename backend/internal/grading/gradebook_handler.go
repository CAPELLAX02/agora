package grading

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

const (
	permScore      = "score:enter"
	permReadScoped = "grade:read_scoped"
)

// GradebookHandler, not tablosu ve not girişi uçlarını sunar.
type GradebookHandler struct {
	svc *Service
	// Plan handler'ının ortak yardımcıları (problem, writeJSON, serverError, failed).
	*Handler
}

// NewGradebookHandler, bir GradebookHandler oluşturur.
func NewGradebookHandler(svc *Service, logger *slog.Logger) *GradebookHandler {
	return &GradebookHandler{svc: svc, Handler: &Handler{logger: logger}}
}

// Register, route'ları kaydeder. Not girişi bölüm kapsamlı `score:enter` yetkisi (MFA)
// ister ama şubeye not girme hakkı ilişkiden gelir (sorumlu ya da ortak öğretim elemanı);
// kararı handler verir. Not tablosunu görmek de ilişki ya da kapsamlı okuma yetkisidir.
func (h *GradebookHandler) Register(rt *authz.Router) {
	rt.HandleFunc("GET /api/v1/sections/{id}/gradebook", authz.Authenticated, h.get)
	rt.HandleFunc("PUT /api/v1/assessment-components/{id}/scores", authz.Permission(permScore), h.setScores)
	rt.HandleFunc("POST /api/v1/assessment-components/{id}/publish", authz.Permission(permScore), h.publish)
	rt.HandleFunc("PUT /api/v1/sections/{id}/attendance", authz.Permission(permScore), h.setAttendance)
	h.registerLetters(rt)
}

// --- Yetki ---------------------------------------------------------------------

// mayViewGradebook: şubenin öğretim elemanları (asistan dahil) ve şubeyi kapsayan
// `grade:read_scoped` yetkisi olanlar (bölüm başkanı, öğrenci işleri, dekan).
func mayViewGradebook(r *http.Request, sc SectionContext) bool {
	if sc.InstructorRole != "" {
		return true
	}
	perms, ok := authz.PermissionsFrom(r.Context())
	return ok && perms.Allows(permReadScoped, sectionTarget(sc))
}

// teaches, kullanıcının şubenin sorumlu ya da ortak öğretim elemanı olduğunu söyler.
func teaches(sc SectionContext) bool {
	return sc.InstructorRole == "PRIMARY" || sc.InstructorRole == "CO_INSTRUCTOR"
}

// mayEnterScores, not girebilecekler: şubenin sorumlu ya da ortak öğretim elemanı,
// MFA'lı oturumda `score:enter` ile. Asistan not girmez.
func mayEnterScores(r *http.Request, sc SectionContext) bool {
	perms, ok := authz.PermissionsFrom(r.Context())
	return ok && teaches(sc) && perms.Has(permScore)
}

// --- Yanıt tipleri -------------------------------------------------------------

type entryWindowResponse struct {
	Open         bool       `json:"open"`
	StartsAt     *time.Time `json:"starts_at"`
	EndsAt       *time.Time `json:"ends_at"`
	NextStartsAt *time.Time `json:"next_starts_at"`
}

type gradebookComponentResponse struct {
	ID          string              `json:"id"`
	Type        typeResponse        `json:"type"`
	SequenceNo  int                 `json:"sequence_no"`
	LabelTR     string              `json:"label_tr"`
	LabelEN     string              `json:"label_en"`
	Weight      float64             `json:"weight"`
	Version     int                 `json:"version"`
	PublishedAt *time.Time          `json:"published_at"`
	EntryWindow entryWindowResponse `json:"entry_window"`
	Editable    bool                `json:"editable"`
	ScoredCount int                 `json:"scored_count"`
	AbsentCount int                 `json:"absent_count"`
	Average     *float64            `json:"average"`
}

type componentScoreResponse struct {
	Score  *float64 `json:"score"`
	Absent bool     `json:"absent"`
}

type programRefResponse struct {
	Code   string `json:"code"`
	NameTR string `json:"name_tr"`
	NameEN string `json:"name_en"`
}

type studentRefResponse struct {
	ID        string `json:"id"`
	StudentNo string `json:"student_no"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

type gradebookStudentResponse struct {
	EnrollmentID     string                            `json:"enrollment_id"`
	Student          studentRefResponse                `json:"student"`
	Program          programRefResponse                `json:"program"`
	AttemptNo        int                               `json:"attempt_no"`
	AttendanceStatus string                            `json:"attendance_status"`
	Scores           map[string]componentScoreResponse `json:"scores"`
	Letter           *string                           `json:"letter"`
	MakeupEligible   bool                              `json:"makeup_eligible"`
}

type courseRefResponse struct {
	ID          string `json:"id"`
	Code        string `json:"code"`
	NameTR      string `json:"name_tr"`
	NameEN      string `json:"name_en"`
	GradingMode string `json:"grading_mode"`
}

type termRefResponse struct {
	ID   string `json:"id"`
	Code string `json:"code"`
}

type gradebookSectionResponse struct {
	ID                string            `json:"id"`
	SectionCode       string            `json:"section_code"`
	Course            courseRefResponse `json:"course"`
	Term              termRefResponse   `json:"term"`
	PlanLocked        bool              `json:"plan_locked"`
	InstructorRole    *string           `json:"instructor_role"`
	GradesPublishedAt *time.Time        `json:"grades_published_at"`
	MakeupPublishedAt *time.Time        `json:"makeup_published_at"`
}

type gradebookResponse struct {
	Section     gradebookSectionResponse     `json:"section"`
	Components  []gradebookComponentResponse `json:"components"`
	Students    []gradebookStudentResponse   `json:"students"`
	CanEnter    bool                         `json:"can_enter"`
	MFARequired bool                         `json:"mfa_required"`
}

func toGradebookResponse(r *http.Request, g Gradebook) gradebookResponse {
	sc := g.SectionContext
	canEnter := mayEnterScores(r, sc)
	perms, _ := authz.PermissionsFrom(r.Context())
	res := gradebookResponse{
		Section: gradebookSectionResponse{
			ID: sc.SectionID, SectionCode: sc.SectionCode,
			Course: courseRefResponse{ID: sc.CourseID, Code: sc.CourseCode, NameTR: sc.CourseNameTR, NameEN: sc.CourseNameEN, GradingMode: sc.GradingMode},
			Term:   termRefResponse{ID: sc.TermID, Code: sc.TermCode}, PlanLocked: sc.LockedAt != nil, InstructorRole: optional(sc.InstructorRole),
			GradesPublishedAt: sc.GradesPublishedAt, MakeupPublishedAt: sc.MakeupPublishedAt,
		},
		Components:  make([]gradebookComponentResponse, 0, len(g.Components)),
		Students:    make([]gradebookStudentResponse, 0, len(g.Students)),
		CanEnter:    canEnter,
		MFARequired: teaches(sc) && perms != nil && perms.WithheldForMFA(permScore),
	}
	perType := map[string]int{}
	for _, c := range g.Components {
		perType[c.Type.Code]++
	}
	for _, c := range g.Components {
		numbered := perType[c.Type.Code] > 1
		cr := gradebookComponentResponse{
			ID: c.ID, Type: toTypeResponse(c.Type), SequenceNo: c.SequenceNo,
			LabelTR: c.Label(false, numbered), LabelEN: c.Label(true, numbered), Weight: c.Weight,
			Version: c.Version, PublishedAt: c.PublishedAt,
			EntryWindow: toWindowResponse(c.Window),
			Editable:    canEnter && sc.LockedAt != nil && c.Window.Open && editablePhase(sc, c.Type.Category),
			ScoredCount: c.Scored, AbsentCount: c.Absent,
		}
		if c.Average != nil {
			avg := c.Average.Float()
			cr.Average = &avg
		}
		res.Components = append(res.Components, cr)
	}
	for _, st := range g.Students {
		sr := gradebookStudentResponse{
			EnrollmentID: st.EnrollmentID,
			Student:      studentRefResponse{ID: st.StudentID, StudentNo: st.StudentNo, FirstName: st.FirstName, LastName: st.LastName},
			Program:      programRefResponse{Code: st.ProgramCode, NameTR: st.ProgramNameTR, NameEN: st.ProgramNameEN},
			AttemptNo:    st.AttemptNo, AttendanceStatus: string(st.Attendance),
			Letter: optional(st.Letter), MakeupEligible: st.MakeupEligible,
			Scores: make(map[string]componentScoreResponse, len(st.Scores)),
		}
		for id, sc := range st.Scores {
			cs := componentScoreResponse{Absent: sc.Absent}
			if !sc.Absent {
				v := sc.Points.Float()
				cs.Score = &v
			}
			sr.Scores[id] = cs
		}
		res.Students = append(res.Students, sr)
	}
	return res
}

// editablePhase, bileşenin şubenin not aşamasında düzenlenebilir olup olmadığıdır: ilandan
// önce bütünleme dışındakiler, ilandan sonra (bütünleme sonuçları ilan edilene kadar)
// sadece bütünleme.
func editablePhase(sc SectionContext, c Category) bool {
	if c == CategoryMakeup {
		return sc.GradesPublishedAt != nil && sc.MakeupPublishedAt == nil
	}
	return sc.GradesPublishedAt == nil
}

// --- Uçlar ---------------------------------------------------------------------

func (h *GradebookHandler) writeGradebook(w http.ResponseWriter, r *http.Request, sectionID string) {
	g, err := h.svc.Gradebook(r.Context(), sectionID, actorID(r))
	if h.gradebookFailed(w, r, err) {
		return
	}
	h.writeJSON(w, r, http.StatusOK, toGradebookResponse(r, g))
}

func (h *GradebookHandler) get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
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
	h.writeJSON(w, r, http.StatusOK, toGradebookResponse(r, g))
}

// component, path'teki bileşeni okur ve isteği yapanın şubeye not girebildiğini doğrular.
func (h *GradebookHandler) component(w http.ResponseWriter, r *http.Request) (ComponentRef, bool) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return ComponentRef{}, false
	}
	ref, err := h.svc.ComponentRef(r.Context(), id, actorID(r))
	if h.gradebookFailed(w, r, err) {
		return ComponentRef{}, false
	}
	if !mayEnterScores(r, ref.Section) {
		h.problem(w, r, http.StatusForbidden, "FORBIDDEN", "Bu şubenin notlarını sadece sorumlu ve ortak öğretim elemanları girebilir.")
		return ComponentRef{}, false
	}
	return ref, true
}

type scoresRequest struct {
	Items []struct {
		EnrollmentID string   `json:"enrollment_id"`
		Score        *float64 `json:"score"`
		Absent       bool     `json:"absent"`
	} `json:"items"`
}

// validate, puanları doğrular: 0-100 arası, en fazla iki ondalık; "girmedi" ile puan
// birlikte verilemez; bir öğrenci bir kez geçer.
func (req scoresRequest) validate() ([]ScoreInput, []httpx.FieldError) {
	var errs []httpx.FieldError
	if len(req.Items) == 0 || len(req.Items) > 1000 {
		return nil, []httpx.FieldError{{Field: "items", Message: "1 ile 1000 arasında öğrenci olmalı."}}
	}
	seen := make(map[string]bool, len(req.Items))
	out := make([]ScoreInput, 0, len(req.Items))
	for _, it := range req.Items {
		if !httpx.ValidUUID(it.EnrollmentID) {
			errs = append(errs, httpx.FieldError{Field: "items.enrollment_id", Message: "geçerli bir UUID olmalı"})
			continue
		}
		if seen[it.EnrollmentID] {
			errs = append(errs, httpx.FieldError{Field: "items.enrollment_id", Message: "Bir öğrenci birden fazla kez verilmiş."})
		}
		seen[it.EnrollmentID] = true
		in := ScoreInput{EnrollmentID: it.EnrollmentID, Absent: it.Absent}
		if it.Score != nil {
			v := *it.Score
			switch {
			case it.Absent:
				errs = append(errs, httpx.FieldError{Field: "items.score", Message: "\"Girmedi\" işaretli öğrenciye puan verilemez."})
			case v < 0 || v > 100 || float64(cents(v)) != v*100:
				errs = append(errs, httpx.FieldError{Field: "items.score", Message: "0 ile 100 arasında, en fazla iki ondalık basamaklı olmalı."})
			default:
				p := Points(cents(v))
				in.Points = &p
			}
		}
		out = append(out, in)
	}
	return out, errs
}

func (h *GradebookHandler) setScores(w http.ResponseWriter, r *http.Request) {
	version, ok := httpx.IfMatchVersion(r)
	if !ok {
		httpx.PreconditionRequired(w, r)
		return
	}
	ref, ok := h.component(w, r)
	if !ok {
		return
	}
	var req scoresRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	items, errs := req.validate()
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	if _, err := h.svc.SetScores(r.Context(), actorID(r), ref, version, items); h.gradebookFailed(w, r, err) {
		return
	}
	h.writeGradebook(w, r, ref.Section.SectionID)
}

func (h *GradebookHandler) publish(w http.ResponseWriter, r *http.Request) {
	ref, ok := h.component(w, r)
	if !ok {
		return
	}
	if h.gradebookFailed(w, r, h.svc.PublishComponent(r.Context(), actorID(r), ref)) {
		return
	}
	h.writeGradebook(w, r, ref.Section.SectionID)
}

type attendanceRequest struct {
	Items []struct {
		EnrollmentID     string `json:"enrollment_id"`
		AttendanceStatus string `json:"attendance_status"`
	} `json:"items"`
}

func (h *GradebookHandler) setAttendance(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return
	}
	g, err := h.svc.Gradebook(r.Context(), id, actorID(r))
	if h.gradebookFailed(w, r, err) {
		return
	}
	if !mayEnterScores(r, g.SectionContext) {
		h.problem(w, r, http.StatusForbidden, "FORBIDDEN", "Devam durumunu sadece şubenin sorumlu ve ortak öğretim elemanları yazabilir.")
		return
	}
	var req attendanceRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	if len(req.Items) == 0 || len(req.Items) > 1000 {
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "items", Message: "1 ile 1000 arasında öğrenci olmalı."}})
		return
	}
	var errs []httpx.FieldError
	items := make([]AttendanceInput, 0, len(req.Items))
	for _, it := range req.Items {
		st := Attendance(it.AttendanceStatus)
		switch {
		case !httpx.ValidUUID(it.EnrollmentID):
			errs = append(errs, httpx.FieldError{Field: "items.enrollment_id", Message: "geçerli bir UUID olmalı"})
		case st != AttendanceContinuing && st != AttendanceFail:
			errs = append(errs, httpx.FieldError{Field: "items.attendance_status", Message: "CONTINUING ya da ATTENDANCE_FAIL olmalı."})
		default:
			items = append(items, AttendanceInput{EnrollmentID: it.EnrollmentID, Status: st})
		}
	}
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	if _, err := h.svc.SetAttendance(r.Context(), actorID(r), g.SectionContext, items); h.gradebookFailed(w, r, err) {
		return
	}
	h.writeGradebook(w, r, id)
}

func (h *GradebookHandler) gradebookFailed(w http.ResponseWriter, r *http.Request, err error) bool {
	var (
		unknown     *UnknownEnrollmentsError
		incomplete  *IncompleteError
		notEligible *NotEligibleError
	)
	switch {
	case err == nil:
		return false
	case errors.As(err, &unknown):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{
			Field: "items.enrollment_id", Message: "Şubenin kesin kayıtlı öğrencisi değil: " + strings.Join(unknown.IDs, ", "),
		}})
	case errors.Is(err, ErrPlanNotLocked):
		h.problem(w, r, http.StatusConflict, "ASSESSMENT_PLAN_NOT_LOCKED", "Notlar girilmeden önce değerlendirme planı kilitlenmeli.")
	case errors.Is(err, ErrEntryWindowClosed):
		h.problem(w, r, http.StatusConflict, "GRADE_ENTRY_WINDOW_CLOSED", "Bu bileşenin not girişi penceresi kapalı.")
	case errors.Is(err, ErrMakeupNotAvailable):
		h.problem(w, r, http.StatusConflict, "MAKEUP_NOT_AVAILABLE", "Bütünleme notları harf notları ilan edildikten sonra girilir.")
	case errors.Is(err, ErrNoScores):
		h.problem(w, r, http.StatusConflict, "NO_SCORES", "Bu bileşende yayımlanacak not yok.")
	case errors.As(err, &notEligible):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{
			Field: "items.enrollment_id", Message: "Bütünleme hakkı olmayan öğrenci: " + strings.Join(notEligible.IDs, ", "),
		}})
	case errors.Is(err, ErrMakeupPublished):
		h.problem(w, r, http.StatusConflict, "MAKEUP_PUBLISHED", "Bütünleme sonuçları ilan edildi; notlar artık sadece not düzeltme talebiyle değişir.")
	case errors.Is(err, ErrMakeupWindowClosed):
		h.problem(w, r, http.StatusConflict, "MAKEUP_WINDOW_CLOSED", "Bütünleme not girişi penceresi kapalı.")
	case errors.Is(err, ErrGradesPublished):
		h.problem(w, r, http.StatusConflict, "GRADES_PUBLISHED", "Harf notları ilan edildi; notlar artık sadece not düzeltme talebiyle değişir.")
	case errors.Is(err, ErrPublishWindowClosed):
		h.problem(w, r, http.StatusConflict, "PUBLISH_WINDOW_CLOSED", "Harf notları final not girişi ya da elle harf notu atama penceresinde ilan edilir.")
	case errors.Is(err, ErrOverrideWindowClosed):
		h.problem(w, r, http.StatusConflict, "MANUAL_LETTER_WINDOW_CLOSED", "Elle harf notu atama penceresi kapalı.")
	case errors.Is(err, ErrStatsNotAvailable):
		h.problem(w, r, http.StatusNotFound, "GRADES_NOT_PUBLISHED", "Bu şubenin harf notları henüz ilan edilmedi; sınıf istatistikleri ilandan sonra görünür.")
	case errors.Is(err, ErrLetterNotAllowed):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "items.letter", Message: "Bu harf bu derse elle verilemez."}})
	case errors.As(err, &incomplete):
		fields := make([]httpx.FieldError, 0, len(incomplete.Students))
		for _, s := range incomplete.Students {
			fields = append(fields, httpx.FieldError{Field: "students", Message: s})
		}
		_ = httpx.WriteProblem(w, r, httpx.Problem{
			Status: http.StatusConflict, Code: "GRADES_INCOMPLETE", Errors: fields,
			Detail: "Notu eksik öğrenciler var: eksik bileşenleri girin ya da elle harf notu verin.",
		})
	default:
		return h.failed(w, r, err)
	}
	return true
}
