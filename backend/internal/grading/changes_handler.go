package grading

import (
	"errors"
	"math"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

const (
	permChangeRequest = "grade_change:request"
	permChangeApprove = "grade_change:approve"
)

func (h *GradebookHandler) registerChanges(rt *authz.Router) {
	rt.HandleFunc("POST /api/v1/grade-change-requests/preview", authz.Permission(permChangeRequest), h.previewChange)
	rt.HandleFunc("POST /api/v1/grade-change-requests", authz.Permission(permChangeRequest), h.requestChange)
	rt.HandleFunc("GET /api/v1/grade-change-requests", authz.Authenticated, h.listChanges)
	rt.HandleFunc("GET /api/v1/grade-change-requests/{id}", authz.Authenticated, h.getChange)
	rt.HandleFunc("POST /api/v1/grade-change-requests/{id}/approve", authz.Permission(permChangeApprove), h.approveChange)
	rt.HandleFunc("POST /api/v1/grade-change-requests/{id}/reject", authz.Permission(permChangeApprove), h.rejectChange)
	rt.HandleFunc("POST /api/v1/grade-change-requests/{id}/withdraw", authz.Permission(permChangeRequest), h.withdrawChange)
}

// mayReviewChange: talebin şubesini kapsayan `grade_change:approve` (bölüm başkanı, fakülte
// öğrenci işleri). Talep eden kendi talebini onaylayamaz (servis ve veritabanı da denetler).
func mayReviewChange(r *http.Request, c GradeChangeRequest) bool {
	perms, ok := authz.PermissionsFrom(r.Context())
	return ok && perms.Allows(permChangeApprove, authz.Target{FacultyID: c.FacultyID, DepartmentID: c.DepartmentID})
}

// --- Yanıt tipleri -------------------------------------------------------------

type gradeValueResponse struct {
	Letter        string   `json:"letter"`
	WeightedScore *float64 `json:"weighted_score"`
	Reason        string   `json:"reason"`
	Score         *float64 `json:"score"`
	Absent        bool     `json:"absent"`
}

func toGradeValue(v GradeValue) gradeValueResponse {
	return gradeValueResponse{Letter: v.Letter, WeightedScore: pointsPtr(v.Weighted), Reason: v.Reason, Score: pointsPtr(v.Score), Absent: v.Absent}
}

type changeComponentResponse struct {
	ID      string  `json:"id"`
	LabelTR string  `json:"label_tr"`
	LabelEN string  `json:"label_en"`
	Weight  float64 `json:"weight"`
}

type gradeChangePreviewResponse struct {
	Kind      string                   `json:"kind"`
	Component *changeComponentResponse `json:"component"`
	Old       gradeValueResponse       `json:"old"`
	New       gradeValueResponse       `json:"new"`
}

type gradeChangeResponse struct {
	ID           string                   `json:"id"`
	Status       string                   `json:"status"`
	Kind         string                   `json:"kind"`
	EnrollmentID string                   `json:"enrollment_id"`
	Student      studentRefResponse       `json:"student"`
	Course       changeCourseResponse     `json:"course"`
	Section      changeSectionResponse    `json:"section"`
	TermCode     string                   `json:"term_code"`
	Component    *changeComponentResponse `json:"component"`
	Old          gradeValueResponse       `json:"old"`
	New          gradeValueResponse       `json:"new"`
	Reason       string                   `json:"reason"`
	RequestedBy  string                   `json:"requested_by"`
	RequestedAt  time.Time                `json:"requested_at"`
	ReviewedBy   *string                  `json:"reviewed_by"`
	ReviewNote   *string                  `json:"review_note"`
	DecidedAt    *time.Time               `json:"decided_at"`
	CanApprove   bool                     `json:"can_approve"`
	CanWithdraw  bool                     `json:"can_withdraw"`
}

type changeCourseResponse struct {
	Code   string `json:"code"`
	NameTR string `json:"name_tr"`
	NameEN string `json:"name_en"`
}

type changeSectionResponse struct {
	ID   string `json:"id"`
	Code string `json:"code"`
}

func toChangeComponent(c *Component, numbered bool) *changeComponentResponse {
	if c == nil {
		return nil
	}
	return &changeComponentResponse{ID: c.ID, LabelTR: c.Label(false, numbered), LabelEN: c.Label(true, numbered), Weight: c.Weight}
}

func toGradeChangeResponse(r *http.Request, c GradeChangeRequest) gradeChangeResponse {
	actor := actorID(r)
	pending := c.Status == ChangePending
	return gradeChangeResponse{
		ID: c.ID, Status: c.Status, Kind: c.Kind, EnrollmentID: c.EnrollmentID,
		Student:   studentRefResponse{ID: c.StudentID, StudentNo: c.StudentNo, FirstName: c.FirstName, LastName: c.LastName},
		Course:    changeCourseResponse{Code: c.CourseCode, NameTR: c.CourseNameTR, NameEN: c.CourseNameEN},
		Section:   changeSectionResponse{ID: c.SectionID, Code: c.SectionCode},
		TermCode:  c.TermCode,
		Component: toChangeComponent(c.Component, c.ComponentNumbered),
		Old:       toGradeValue(c.Old), New: toGradeValue(c.New), Reason: c.Reason,
		RequestedBy: c.RequestedByName, RequestedAt: c.RequestedAt,
		ReviewedBy: optional(c.ReviewedByName), ReviewNote: optional(c.ReviewNote), DecidedAt: c.DecidedAt,
		CanApprove:  pending && c.RequestedBy != actor && mayReviewChange(r, c),
		CanWithdraw: pending && c.RequestedBy == actor,
	}
}

// --- İstek ---------------------------------------------------------------------

type gradeChangeRequest struct {
	EnrollmentID string   `json:"enrollment_id"`
	ComponentID  *string  `json:"component_id"`
	Score        *float64 `json:"score"`
	Absent       bool     `json:"absent"`
	Letter       *string  `json:"letter"`
	Reason       string   `json:"reason"`
}

// input, isteği doğrular; gerekçe sadece talepte zorunludur (önizleme gerekçesiz yapılabilir).
func (req gradeChangeRequest) input(requireReason bool) (GradeChangeInput, []httpx.FieldError) {
	in := GradeChangeInput{EnrollmentID: req.EnrollmentID, Absent: req.Absent, Reason: strings.TrimSpace(req.Reason)}
	var errs []httpx.FieldError
	if !httpx.ValidUUID(req.EnrollmentID) {
		errs = append(errs, httpx.FieldError{Field: "enrollment_id", Message: "geçerli bir UUID olmalı"})
	}
	if req.ComponentID != nil {
		in.ComponentID = *req.ComponentID
		switch {
		case !httpx.ValidUUID(in.ComponentID):
			errs = append(errs, httpx.FieldError{Field: "component_id", Message: "geçerli bir UUID olmalı"})
		case req.Letter != nil:
			errs = append(errs, httpx.FieldError{Field: "letter", Message: "Bileşen notu düzeltilirken harf verilmez; harf yeniden hesaplanır."})
		case req.Absent && req.Score != nil:
			errs = append(errs, httpx.FieldError{Field: "score", Message: "Girmedi işaretliyken puan verilmez."})
		case !req.Absent && req.Score == nil:
			errs = append(errs, httpx.FieldError{Field: "score", Message: "Düzeltilmiş puanı ya da girmedi işaretini verin."})
		case req.Score != nil && (*req.Score < 0 || *req.Score > 100 || math.Round(*req.Score*100) != *req.Score*100):
			errs = append(errs, httpx.FieldError{Field: "score", Message: "0 ile 100 arasında, en fazla iki ondalık basamaklı olmalı."})
		case req.Score != nil:
			p := Points(math.Round(*req.Score * 100))
			in.Score = &p
		}
	} else {
		if req.Letter != nil {
			in.Letter = strings.TrimSpace(*req.Letter)
		}
		if in.Letter == "" {
			errs = append(errs, httpx.FieldError{Field: "letter", Message: "Düzeltilecek bileşeni ya da yeni harf notunu verin."})
		}
	}
	if n := utf8.RuneCountInString(in.Reason); requireReason && (n < 10 || n > 500) {
		errs = append(errs, httpx.FieldError{Field: "reason", Message: "10 ile 500 karakter arasında bir gerekçe yazın."})
	}
	return in, errs
}

// --- Uçlar ---------------------------------------------------------------------

// prepareChange, isteği okur ve düzeltmeyi hesaplar: talep sadece şubenin sorumlu ya da ortak
// öğretim elemanından gelir.
func (h *GradebookHandler) prepareChange(w http.ResponseWriter, r *http.Request, requireReason bool) (GradeChangeInput, GradeChange, bool) {
	var req gradeChangeRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return GradeChangeInput{}, GradeChange{}, false
	}
	in, errs := req.input(requireReason)
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return GradeChangeInput{}, GradeChange{}, false
	}
	ch, err := h.svc.PreviewGradeChange(r.Context(), actorID(r), in)
	if h.changeFailed(w, r, err) {
		return GradeChangeInput{}, GradeChange{}, false
	}
	if !teaches(ch.Section) {
		h.problem(w, r, http.StatusForbidden, "FORBIDDEN", "Not düzeltmesini sadece şubenin öğretim elemanı talep edebilir.")
		return GradeChangeInput{}, GradeChange{}, false
	}
	return in, ch, true
}

func (h *GradebookHandler) previewChange(w http.ResponseWriter, r *http.Request) {
	_, ch, ok := h.prepareChange(w, r, false)
	if !ok {
		return
	}
	h.writeJSON(w, r, http.StatusOK, gradeChangePreviewResponse{
		Kind: ch.Kind, Component: toChangeComponent(ch.Component, ch.ComponentNumbered), Old: toGradeValue(ch.Old), New: toGradeValue(ch.New),
	})
}

func (h *GradebookHandler) requestChange(w http.ResponseWriter, r *http.Request) {
	in, _, ok := h.prepareChange(w, r, true)
	if !ok {
		return
	}
	id, err := h.svc.RequestGradeChange(r.Context(), actorID(r), in)
	if h.changeFailed(w, r, err) {
		return
	}
	h.writeChange(w, r, http.StatusCreated, id)
}

func (h *GradebookHandler) writeChange(w http.ResponseWriter, r *http.Request, status int, id string) {
	c, err := h.svc.GradeChangeRequestByID(r.Context(), id)
	if h.changeFailed(w, r, err) {
		return
	}
	h.writeJSON(w, r, status, toGradeChangeResponse(r, c))
}

// listChanges: view=mine öğretim elemanının kendi talepleri; view=review (varsayılan) onaylayanın
// kapsamındaki talepler. status virgülle ayrılmış durumlar (varsayılan hepsi).
func (h *GradebookHandler) listChanges(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	view := query.Get("view")
	if view == "" {
		view = "review"
	}
	f := GradeChangeFilter{}
	if s := query.Get("status"); s != "" {
		for _, st := range strings.Split(s, ",") {
			switch st {
			case ChangePending, ChangeApplied, ChangeRejected, ChangeWithdrawn:
				f.Statuses = append(f.Statuses, st)
			default:
				httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "status", Message: "PENDING, APPLIED, REJECTED ya da WITHDRAWN olmalı."}})
				return
			}
		}
	}
	switch view {
	case "mine":
		f.RequestedBy = actorID(r)
	case "review":
	default:
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "view", Message: "mine ya da review olmalı."}})
		return
	}
	list, err := h.svc.GradeChangeRequests(r.Context(), f)
	if h.changeFailed(w, r, err) {
		return
	}
	res := httpx.ListResponse[gradeChangeResponse]{Items: []gradeChangeResponse{}}
	for _, c := range list {
		if view == "review" && !mayReviewChange(r, c) {
			continue
		}
		res.Items = append(res.Items, toGradeChangeResponse(r, c))
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

// loadChange, path'teki talebi okur: talep eden ve onaylayabilecekler görür, başkasına 404.
func (h *GradebookHandler) loadChange(w http.ResponseWriter, r *http.Request) (GradeChangeRequest, bool) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return GradeChangeRequest{}, false
	}
	c, err := h.svc.GradeChangeRequestByID(r.Context(), id)
	if h.changeFailed(w, r, err) {
		return GradeChangeRequest{}, false
	}
	if c.RequestedBy != actorID(r) && !mayReviewChange(r, c) {
		httpx.NotFound(w, r)
		return GradeChangeRequest{}, false
	}
	return c, true
}

func (h *GradebookHandler) getChange(w http.ResponseWriter, r *http.Request) {
	if c, ok := h.loadChange(w, r); ok {
		h.writeJSON(w, r, http.StatusOK, toGradeChangeResponse(r, c))
	}
}

type reviewRequest struct {
	Note string `json:"note"`
}

func (h *GradebookHandler) review(w http.ResponseWriter, r *http.Request, approve bool) {
	c, ok := h.loadChange(w, r)
	if !ok {
		return
	}
	if !mayReviewChange(r, c) {
		h.problem(w, r, http.StatusForbidden, "FORBIDDEN", "Bu şubenin not düzeltmelerini onaylama yetkiniz yok.")
		return
	}
	var req reviewRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	note := strings.TrimSpace(req.Note)
	if n := utf8.RuneCountInString(note); (!approve || n > 0) && (n < 5 || n > 500) {
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "note", Message: "5 ile 500 karakter arasında bir not yazın (retle zorunlu)."}})
		return
	}
	var err error
	if approve {
		err = h.svc.ApproveGradeChange(r.Context(), actorID(r), c, note)
	} else {
		err = h.svc.RejectGradeChange(r.Context(), actorID(r), c.ID, note)
	}
	if h.changeFailed(w, r, err) {
		return
	}
	h.writeChange(w, r, http.StatusOK, c.ID)
}

func (h *GradebookHandler) approveChange(w http.ResponseWriter, r *http.Request) {
	h.review(w, r, true)
}
func (h *GradebookHandler) rejectChange(w http.ResponseWriter, r *http.Request) {
	h.review(w, r, false)
}

func (h *GradebookHandler) withdrawChange(w http.ResponseWriter, r *http.Request) {
	c, ok := h.loadChange(w, r)
	if !ok {
		return
	}
	if h.changeFailed(w, r, h.svc.WithdrawGradeChange(r.Context(), actorID(r), c.ID)) {
		return
	}
	h.writeChange(w, r, http.StatusOK, c.ID)
}

func (h *GradebookHandler) changeFailed(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, ErrGradesNotPublished):
		h.problem(w, r, http.StatusConflict, "GRADES_NOT_PUBLISHED", "Harf notları ilan edilmeden önce notlar not tablosundan değişir.")
	case errors.Is(err, ErrNotCompleted):
		h.problem(w, r, http.StatusConflict, "ENROLLMENT_NOT_GRADED", "Bu kesin kaydın notu verilmemiş.")
	case errors.Is(err, ErrMakeupNotPublishedYet):
		h.problem(w, r, http.StatusConflict, "MAKEUP_NOT_PUBLISHED", "Bütünleme notları bütünleme ilanından önce not tablosundan değişir.")
	case errors.Is(err, ErrComponentNotInSection):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "component_id", Message: "Bileşen bu şubenin değerlendirme planında değil."}})
	case errors.Is(err, ErrIncompleteGrade):
		h.problem(w, r, http.StatusConflict, "GRADE_INCOMPLETE", "Düzeltilen notla harf notu hesaplanamıyor: öğrencinin eksik bileşeni var.")
	case errors.Is(err, ErrNoGradeChange):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "score", Message: "Talep notu değiştirmiyor."}})
	case errors.Is(err, ErrGradeChangePending):
		h.problem(w, r, http.StatusConflict, "GRADE_CHANGE_PENDING", "Bu öğrencinin bu dersteki notu için bekleyen bir düzeltme talebi var.")
	case errors.Is(err, ErrGradeChangeNotPending):
		h.problem(w, r, http.StatusConflict, "GRADE_CHANGE_DECIDED", "Talep artık beklemede değil.")
	case errors.Is(err, ErrGradeChangeStale):
		h.problem(w, r, http.StatusConflict, "GRADE_CHANGED", "Not talepten sonra değişti; talebi geri çekip yeniden oluşturun.")
	case errors.Is(err, ErrSelfApproval):
		h.problem(w, r, http.StatusForbidden, "SELF_APPROVAL", "Kendi not düzeltme talebinizi onaylayamazsınız ya da reddedemezsiniz.")
	default:
		return h.gradebookFailed(w, r, err)
	}
	return true
}
