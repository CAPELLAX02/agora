package lms

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/CAPELLAX02/agora/backend/internal/files"
	"github.com/CAPELLAX02/agora/backend/internal/grading"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
	"github.com/CAPELLAX02/agora/backend/internal/platform/sheet"
)

// registerAssignments, ödev uçlarını kaydeder. Öğrenci kendi teslimini "my-submission" altında
// yönetir; öğretim elemanları teslim listesini görür, puanlar, yorum yazar ve puanları yayımlar
// (şubenin bölümünü yöneten personel listeyi salt okunur görür).
func (h *Handler) registerAssignments(rt *authz.Router) {
	rt.HandleFunc("GET /api/v1/activities/{id}/my-submission", authz.Authenticated, h.mySubmission)
	rt.HandleFunc("PUT /api/v1/activities/{id}/my-submission", authz.Authenticated, h.saveDraft)
	rt.HandleFunc("POST /api/v1/activities/{id}/my-submission/submit", authz.Authenticated, h.submit)
	rt.HandleFunc("POST /api/v1/activities/{id}/my-submission/attempts", authz.Authenticated, h.newAttempt)
	rt.HandleFunc("POST /api/v1/activities/{id}/my-submission/comments", authz.Authenticated, h.myComment)
	rt.HandleFunc("GET /api/v1/activities/{id}/my-submission/files/{fileId}/download-url", authz.Authenticated, h.myFileURL)
	rt.HandleFunc("GET /api/v1/activities/{id}/submissions", authz.Authenticated, h.roster)
	rt.HandleFunc("GET /api/v1/activities/{id}/submissions/{userId}", authz.Authenticated, h.studentWork)
	rt.HandleFunc("PUT /api/v1/activities/{id}/submissions/{userId}/grade", authz.Authenticated, h.setGrade)
	rt.HandleFunc("POST /api/v1/activities/{id}/submissions/{userId}/comments", authz.Authenticated, h.teacherComment)
	rt.HandleFunc("GET /api/v1/activities/{id}/submissions/{userId}/files/{fileId}/download-url", authz.Authenticated, h.studentFileURL)
	rt.HandleFunc("POST /api/v1/activities/{id}/grades/release", authz.Authenticated, h.releaseGrades)
	rt.HandleFunc("GET /api/v1/activities/{id}/link", authz.Authenticated, h.getLink)
	rt.HandleFunc("PUT /api/v1/activities/{id}/link", authz.Authenticated, h.setLink)
	rt.HandleFunc("GET /api/v1/activities/{id}/grades.xlsx", authz.Authenticated, h.gradeSheet)
	rt.HandleFunc("POST /api/v1/activities/{id}/grades/import/preview", authz.Authenticated, h.previewGradeImport)
	rt.HandleFunc("POST /api/v1/activities/{id}/grades/import", authz.Authenticated, h.importGrades)
}

// --- İstek ve yanıtlar -----------------------------------------------------------

type assignmentRequest struct {
	OpensAt         *time.Time `json:"opens_at"`
	DueAt           *time.Time `json:"due_at"`
	CutoffAt        *time.Time `json:"cutoff_at"`
	MaxAttempts     *int       `json:"max_attempts"`
	SubmissionTypes []string   `json:"submission_types"`
	MaxFiles        *int       `json:"max_files"`
	MaxFileSize     *int64     `json:"max_file_size_bytes"`
	Extensions      []string   `json:"allowed_extensions"`
	MaxScore        *float64   `json:"max_score"`
	LatePolicy      string     `json:"late_policy"`
	PenaltyPerDay   *float64   `json:"late_penalty_pct_per_day"`
}

// points, en çok iki ondalıklı sayıyı yüzde birlik tamsayıya çevirir.
func points(field string, v float64) (grading.Points, error) {
	p := math.Round(v * 100)
	if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(p-v*100) > 1e-6 {
		return 0, &ValidationError{Field: field, Message: "En çok iki ondalıklı bir sayı olmalı."}
	}
	return grading.Points(p), nil
}

// settings, isteği ayarlara çevirir; verilmeyenler varsayılandır (1 deneme, 5 dosya, platformun
// boyut sınırı, 100 puan, geç teslim serbest).
func (req assignmentRequest) settings(maxUpload int64) (AssignmentSettings, error) {
	a := AssignmentSettings{
		OpensAt: req.OpensAt, DueAt: req.DueAt, CutoffAt: req.CutoffAt, MaxAttempts: 1,
		SubmissionTypes: req.SubmissionTypes, MaxFiles: 5, MaxFileSize: maxUpload, Extensions: req.Extensions,
		MaxScore: 10_000, LatePolicy: req.LatePolicy,
	}
	if a.SubmissionTypes == nil {
		a.SubmissionTypes = []string{SubmitFile}
	}
	if a.Extensions == nil {
		a.Extensions = []string{}
	}
	if a.LatePolicy == "" {
		a.LatePolicy = LateAllow
	}
	if req.MaxAttempts != nil {
		a.MaxAttempts = *req.MaxAttempts
	}
	if req.MaxFiles != nil {
		a.MaxFiles = *req.MaxFiles
	}
	if req.MaxFileSize != nil {
		a.MaxFileSize = *req.MaxFileSize
	}
	var err error
	if req.MaxScore != nil {
		if a.MaxScore, err = points("assignment.max_score", *req.MaxScore); err != nil {
			return a, err
		}
	}
	if req.PenaltyPerDay != nil {
		if a.PenaltyPerDay, err = points("assignment.late_penalty_pct_per_day", *req.PenaltyPerDay); err != nil {
			return a, err
		}
	}
	return a, nil
}

type assignmentResponse struct {
	OpensAt         *time.Time      `json:"opens_at"`
	DueAt           *time.Time      `json:"due_at"`
	CutoffAt        *time.Time      `json:"cutoff_at"`
	MaxAttempts     int             `json:"max_attempts"`
	SubmissionTypes []string        `json:"submission_types"`
	MaxFiles        int             `json:"max_files"`
	MaxFileSize     int64           `json:"max_file_size_bytes"`
	Extensions      []string        `json:"allowed_extensions"`
	MaxScore        float64         `json:"max_score"`
	LatePolicy      string          `json:"late_policy"`
	PenaltyPerDay   float64         `json:"late_penalty_pct_per_day"`
	ReleasedAt      *time.Time      `json:"grades_released_at"`
	State           SubmissionState `json:"submission_state"`
}

func toAssignment(a AssignmentSettings, now time.Time) assignmentResponse {
	return assignmentResponse{
		OpensAt: a.OpensAt, DueAt: a.DueAt, CutoffAt: a.CutoffAt, MaxAttempts: a.MaxAttempts,
		SubmissionTypes: a.SubmissionTypes, MaxFiles: a.MaxFiles, MaxFileSize: a.MaxFileSize, Extensions: a.Extensions,
		MaxScore: a.MaxScore.Float(), LatePolicy: a.LatePolicy, PenaltyPerDay: a.PenaltyPerDay.Float(),
		ReleasedAt: a.ReleasedAt, State: a.StateAt(now),
	}
}

type submissionResponse struct {
	ID          string            `json:"id"`
	AttemptNo   int               `json:"attempt_no"`
	Status      string            `json:"status"`
	Text        string            `json:"online_text_html"`
	Files       []fileRefResponse `json:"files"`
	SubmittedAt *time.Time        `json:"submitted_at"`
	Late        bool              `json:"is_late"`
	Version     int               `json:"version"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

func toSubmission(s Submission) submissionResponse {
	res := submissionResponse{ID: s.ID, AttemptNo: s.AttemptNo, Status: s.Status, Text: s.Text,
		Files: make([]fileRefResponse, 0, len(s.Files)), SubmittedAt: s.SubmittedAt, Late: s.Late, Version: s.Version, UpdatedAt: s.UpdatedAt}
	for _, f := range s.Files {
		res.Files = append(res.Files, fileRefResponse(f))
	}
	return res
}

type gradeResponse struct {
	Score      *float64  `json:"score"`
	PenaltyPct float64   `json:"late_penalty_pct"`
	Final      *float64  `json:"final_score"`
	MaxScore   float64   `json:"max_score"`
	Feedback   string    `json:"feedback_html"`
	AttemptNo  *int      `json:"attempt_no"`
	GradedBy   string    `json:"graded_by"`
	GradedAt   time.Time `json:"graded_at"`
	Version    int       `json:"version"`
}

func toGrade(g *Grade, st AssignmentSettings) *gradeResponse {
	if g == nil {
		return nil
	}
	res := &gradeResponse{PenaltyPct: g.Penalty.Float(), MaxScore: st.MaxScore.Float(), Feedback: g.Feedback,
		AttemptNo: g.AttemptNo, GradedBy: g.GradedBy, GradedAt: g.GradedAt, Version: g.Version}
	if g.Score != nil {
		sc, fin := g.Score.Float(), g.Final.Float()
		res.Score, res.Final = &sc, &fin
	}
	return res
}

type commentResponse struct {
	ID         string    `json:"id"`
	AuthorID   string    `json:"author_id"`
	AuthorName string    `json:"author_name"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
}

func toComments(list []Comment) []commentResponse {
	out := make([]commentResponse, 0, len(list))
	for _, c := range list {
		out = append(out, commentResponse(c))
	}
	return out
}

func toAttempts(list []Submission) []submissionResponse {
	out := make([]submissionResponse, 0, len(list))
	for _, s := range list {
		out = append(out, toSubmission(s))
	}
	return out
}

type mySubmissionResponse struct {
	Assignment    assignmentResponse   `json:"assignment"`
	Attempts      []submissionResponse `json:"attempts"`
	CanEdit       bool                 `json:"can_edit"`
	CanSubmit     bool                 `json:"can_submit"`
	CanNewAttempt bool                 `json:"can_new_attempt"`
	Grade         *gradeResponse       `json:"grade"`
	Comments      []commentResponse    `json:"comments"`
}

type studentResponse struct {
	UserID    string `json:"user_id"`
	StudentNo string `json:"student_no"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

type latestResponse struct {
	AttemptNo   int        `json:"attempt_no"`
	Status      string     `json:"status"`
	SubmittedAt *time.Time `json:"submitted_at"`
	Late        bool       `json:"is_late"`
	FileCount   int        `json:"file_count"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type rosterRowResponse struct {
	Student      studentResponse `json:"student"`
	Latest       *latestResponse `json:"latest"`
	Grade        *gradeResponse  `json:"grade"`
	Comments     int             `json:"comments"`
	NeedsGrading bool            `json:"needs_grading"`
}

type rosterSummary struct {
	Students     int `json:"students"`
	Submitted    int `json:"submitted"`
	Drafts       int `json:"drafts"`
	NotStarted   int `json:"not_started"`
	Late         int `json:"late"`
	Graded       int `json:"graded"`
	NeedsGrading int `json:"needs_grading"`
}

type rosterResponse struct {
	Assignment assignmentResponse  `json:"assignment"`
	CanGrade   bool                `json:"can_grade"`
	Summary    rosterSummary       `json:"summary"`
	Items      []rosterRowResponse `json:"items"`
}

type studentWorkResponse struct {
	Student    studentResponse      `json:"student"`
	Assignment assignmentResponse   `json:"assignment"`
	CanGrade   bool                 `json:"can_grade"`
	Attempts   []submissionResponse `json:"attempts"`
	Grade      *gradeResponse       `json:"grade"`
	Comments   []commentResponse    `json:"comments"`
}

// --- Erişim ----------------------------------------------------------------------

// assignment, path'teki ödevi, ayarlarını, şubesini ve isteği yapanın rolünü çözer. review
// true ise kapanmış etkinliğe de izin verilir (okuma); değilse etkinlik açık olmalı (yazma).
func (h *Handler) assignment(w http.ResponseWriter, r *http.Request, review bool) (Activity, AssignmentSettings, Section, Role, bool) {
	id, sec, role, ok := h.activity(w, r)
	if !ok {
		return Activity{}, AssignmentSettings{}, Section{}, RoleNone, false
	}
	a, _, err := h.svc.Activity(r.Context(), id)
	if h.failed(w, r, err) {
		return Activity{}, AssignmentSettings{}, Section{}, RoleNone, false
	}
	if a.Type != TypeAssignment || a.Assignment == nil {
		httpx.NotFound(w, r)
		return Activity{}, AssignmentSettings{}, Section{}, RoleNone, false
	}
	check := h.svc.Accessible
	if review {
		check = h.svc.Reviewable
	}
	if h.failed(w, r, check(r.Context(), a, role)) {
		return Activity{}, AssignmentSettings{}, Section{}, RoleNone, false
	}
	return a, *a.Assignment, sec, role, true
}

// learner, öğrenci uçlarını sadece şubenin öğrencisine açar.
func (h *Handler) learner(w http.ResponseWriter, r *http.Request, review bool) (Activity, AssignmentSettings, bool) {
	a, st, _, role, ok := h.assignment(w, r, review)
	if !ok {
		return Activity{}, AssignmentSettings{}, false
	}
	if role != RoleStudent {
		_ = httpx.WriteProblem(w, r, httpx.Problem{Status: http.StatusForbidden, Code: "FORBIDDEN",
			Detail: "Ödev teslimi şubenin öğrencilerine özeldir."})
		return Activity{}, AssignmentSettings{}, false
	}
	return a, st, true
}

// teacher, teslim listesi uçlarını öğretim elemanlarına ve izleyiciye açar; write true ise
// sadece öğretim elemanlarına (puanlama, yorum, yayımlama).
func (h *Handler) teacher(w http.ResponseWriter, r *http.Request, write bool) (Activity, AssignmentSettings, Section, Role, bool) {
	a, st, sec, role, ok := h.assignment(w, r, true)
	if !ok {
		return Activity{}, AssignmentSettings{}, Section{}, RoleNone, false
	}
	if !role.CanEdit() && (write || role != RoleViewer) {
		_ = httpx.WriteProblem(w, r, httpx.Problem{Status: http.StatusForbidden, Code: "FORBIDDEN",
			Detail: "Teslimleri sadece şubenin öğretim elemanları puanlayabilir."})
		return Activity{}, AssignmentSettings{}, Section{}, RoleNone, false
	}
	return a, st, sec, role, true
}

// --- Öğrenci ---------------------------------------------------------------------

func (h *Handler) writeMySubmission(w http.ResponseWriter, r *http.Request, a Activity, st AssignmentSettings, status int) {
	work, err := h.svc.Work(r.Context(), st, a.ID, userID(r), st.ReleasedAt != nil)
	if h.failed(w, r, err) {
		return
	}
	now := h.svc.now()
	open := st.StateAt(now) == SubmissionOpen && a.Window.At(now) == Open
	latest := work.Latest()
	res := mySubmissionResponse{
		Assignment: toAssignment(st, now), Attempts: toAttempts(work.Attempts),
		CanEdit:       open && (latest == nil || latest.Status == StatusDraft),
		CanSubmit:     open && latest != nil && latest.Status == StatusDraft && hasContent(st, latest.Text, len(latest.Files)),
		CanNewAttempt: open && latest != nil && latest.Status == StatusSubmitted && latest.AttemptNo < st.MaxAttempts,
		Grade:         toGrade(work.Grade, st), Comments: toComments(work.Comments),
	}
	h.writeJSON(w, r, status, res)
}

func (h *Handler) mySubmission(w http.ResponseWriter, r *http.Request) {
	if a, st, ok := h.learner(w, r, true); ok {
		h.writeMySubmission(w, r, a, st, http.StatusOK)
	}
}

func (h *Handler) saveDraft(w http.ResponseWriter, r *http.Request) {
	a, st, ok := h.learner(w, r, false)
	if !ok {
		return
	}
	var req struct {
		Text    string   `json:"online_text_html"`
		FileIDs []string `json:"file_ids"`
	}
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	for _, f := range req.FileIDs {
		if !httpx.ValidUUID(f) {
			httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "file_ids", Message: "Geçerli UUID'ler olmalı."}})
			return
		}
	}
	// If-Match yoksa yeni taslak oluşturulur; taslak varsa sürüm zorunludur.
	version, _ := httpx.IfMatchVersion(r)
	if h.failed(w, r, h.svc.SaveDraft(r.Context(), userID(r), a.ID, version, DraftInput{Text: req.Text, FileIDs: req.FileIDs})) {
		return
	}
	h.writeMySubmission(w, r, a, st, http.StatusOK)
}

func (h *Handler) submit(w http.ResponseWriter, r *http.Request) {
	version, ok := httpx.IfMatchVersion(r)
	if !ok {
		httpx.PreconditionRequired(w, r)
		return
	}
	a, st, ok := h.learner(w, r, false)
	if !ok {
		return
	}
	if h.failed(w, r, h.svc.Submit(r.Context(), userID(r), a.ID, version)) {
		return
	}
	h.writeMySubmission(w, r, a, st, http.StatusOK)
}

func (h *Handler) newAttempt(w http.ResponseWriter, r *http.Request) {
	a, st, ok := h.learner(w, r, false)
	if !ok {
		return
	}
	if h.failed(w, r, h.svc.NewAttempt(r.Context(), userID(r), a.ID)) {
		return
	}
	h.writeMySubmission(w, r, a, st, http.StatusCreated)
}

type commentRequest struct {
	Body string `json:"body"`
}

func (h *Handler) myComment(w http.ResponseWriter, r *http.Request) {
	a, st, ok := h.learner(w, r, true)
	if !ok {
		return
	}
	var req commentRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	if h.failed(w, r, h.svc.AddComment(r.Context(), userID(r), a.ID, userID(r), req.Body)) {
		return
	}
	h.writeMySubmission(w, r, a, st, http.StatusCreated)
}

func (h *Handler) myFileURL(w http.ResponseWriter, r *http.Request) {
	a, _, ok := h.learner(w, r, true)
	if !ok {
		return
	}
	h.submissionFileURL(w, r, a.ID, userID(r))
}

func (h *Handler) submissionFileURL(w http.ResponseWriter, r *http.Request, activityID, studentID string) {
	fileID := r.PathValue("fileId")
	if !httpx.ValidUUID(fileID) {
		httpx.NotFound(w, r)
		return
	}
	u, exp, err := h.svc.SubmissionFileURL(r.Context(), activityID, studentID, fileID)
	if h.failed(w, r, err) {
		return
	}
	h.writeJSON(w, r, http.StatusOK, files.URLResponse{URL: u, ExpiresAt: exp})
}

// --- Öğretim elemanı -------------------------------------------------------------

// needsGrading, son denemesi gönderilmiş ve o deneme henüz puanlanmamış öğrencidir.
func needsGrading(latest *Submission, g *Grade) bool {
	if latest == nil || latest.Status != StatusSubmitted {
		return false
	}
	return g == nil || g.Score == nil || g.AttemptNo == nil || *g.AttemptNo < latest.AttemptNo
}

func (h *Handler) writeRoster(w http.ResponseWriter, r *http.Request, a Activity, st AssignmentSettings, sec Section, role Role) {
	rows, err := h.svc.Roster(r.Context(), st, sec.ID, a.ID)
	if h.failed(w, r, err) {
		return
	}
	res := rosterResponse{Assignment: toAssignment(st, h.svc.now()), CanGrade: role.CanEdit(),
		Items: make([]rosterRowResponse, 0, len(rows))}
	res.Summary.Students = len(rows)
	for _, row := range rows {
		item := rosterRowResponse{Student: studentResponse(row.Student), Grade: toGrade(row.Grade, st), Comments: row.Comments,
			NeedsGrading: needsGrading(row.Latest, row.Grade)}
		switch {
		case row.Latest == nil:
			res.Summary.NotStarted++
		case row.Latest.Status == StatusSubmitted:
			res.Summary.Submitted++
			if row.Latest.Late {
				res.Summary.Late++
			}
		default:
			res.Summary.Drafts++
		}
		if row.Latest != nil {
			item.Latest = &latestResponse{AttemptNo: row.Latest.AttemptNo, Status: row.Latest.Status, SubmittedAt: row.Latest.SubmittedAt,
				Late: row.Latest.Late, FileCount: row.Latest.FileCount, UpdatedAt: row.Latest.UpdatedAt}
		}
		if row.Grade != nil && row.Grade.Score != nil {
			res.Summary.Graded++
		}
		if item.NeedsGrading {
			res.Summary.NeedsGrading++
		}
		res.Items = append(res.Items, item)
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

func (h *Handler) roster(w http.ResponseWriter, r *http.Request) {
	if a, st, sec, role, ok := h.teacher(w, r, false); ok {
		h.writeRoster(w, r, a, st, sec, role)
	}
}

// student, path'teki öğrenciyi çözer: şubenin öğrencisi olmalı.
func (h *Handler) student(w http.ResponseWriter, r *http.Request, sec Section) (Student, bool) {
	id := r.PathValue("userId")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return Student{}, false
	}
	st, err := h.svc.SectionStudent(r.Context(), sec.ID, id)
	if h.failed(w, r, err) {
		return Student{}, false
	}
	return st, true
}

func (h *Handler) writeStudentWork(w http.ResponseWriter, r *http.Request, a Activity, st AssignmentSettings, role Role, student Student, status int) {
	work, err := h.svc.Work(r.Context(), st, a.ID, student.UserID, true)
	if h.failed(w, r, err) {
		return
	}
	h.writeJSON(w, r, status, studentWorkResponse{
		Student: studentResponse(student), Assignment: toAssignment(st, h.svc.now()), CanGrade: role.CanEdit(),
		Attempts: toAttempts(work.Attempts), Grade: toGrade(work.Grade, st), Comments: toComments(work.Comments),
	})
}

func (h *Handler) studentWork(w http.ResponseWriter, r *http.Request) {
	a, st, sec, role, ok := h.teacher(w, r, false)
	if !ok {
		return
	}
	if student, ok := h.student(w, r, sec); ok {
		h.writeStudentWork(w, r, a, st, role, student, http.StatusOK)
	}
}

func (h *Handler) setGrade(w http.ResponseWriter, r *http.Request) {
	a, st, sec, role, ok := h.teacher(w, r, true)
	if !ok {
		return
	}
	student, ok := h.student(w, r, sec)
	if !ok {
		return
	}
	var req struct {
		Score    *float64 `json:"score"`
		Feedback string   `json:"feedback_html"`
	}
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	in := GradeInput{Feedback: req.Feedback}
	if req.Score != nil {
		p, err := points("score", *req.Score)
		if h.failed(w, r, err) {
			return
		}
		in.Score = &p
	}
	// If-Match yoksa ilk puan oluşturulur; puan varsa sürüm zorunludur.
	version, _ := httpx.IfMatchVersion(r)
	if h.failed(w, r, h.svc.SetGrade(r.Context(), userID(r), sec.ID, a.ID, student.UserID, version, in)) {
		return
	}
	h.writeStudentWork(w, r, a, st, role, student, http.StatusOK)
}

func (h *Handler) teacherComment(w http.ResponseWriter, r *http.Request) {
	a, st, sec, role, ok := h.teacher(w, r, true)
	if !ok {
		return
	}
	student, ok := h.student(w, r, sec)
	if !ok {
		return
	}
	var req commentRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	if h.failed(w, r, h.svc.AddComment(r.Context(), userID(r), a.ID, student.UserID, req.Body)) {
		return
	}
	h.writeStudentWork(w, r, a, st, role, student, http.StatusCreated)
}

func (h *Handler) studentFileURL(w http.ResponseWriter, r *http.Request) {
	a, _, sec, _, ok := h.teacher(w, r, false)
	if !ok {
		return
	}
	if student, ok := h.student(w, r, sec); ok {
		h.submissionFileURL(w, r, a.ID, student.UserID)
	}
}

func (h *Handler) releaseGrades(w http.ResponseWriter, r *http.Request) {
	a, _, sec, role, ok := h.teacher(w, r, true)
	if !ok {
		return
	}
	if h.failed(w, r, h.svc.ReleaseGrades(r.Context(), userID(r), a.ID)) {
		return
	}
	st, err := h.svc.Assignment(r.Context(), h.svc.pool, a.ID)
	if h.failed(w, r, err) {
		return
	}
	h.writeRoster(w, r, a, st, sec, role)
}

// --- Excel ile toplu puan --------------------------------------------------------

func (h *Handler) gradeSheet(w http.ResponseWriter, r *http.Request) {
	a, st, sec, _, ok := h.teacher(w, r, true)
	if !ok {
		return
	}
	lang := r.URL.Query().Get("lang")
	if lang != "" && lang != "tr" && lang != "en" {
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "lang", Message: "tr ya da en olmalı."}})
		return
	}
	rows, err := h.svc.Roster(r.Context(), st, sec.ID, a.ID)
	if h.failed(w, r, err) {
		return
	}
	// Önce belleğe: yazarken hata olursa yarım dosya yerine 500 dönülür.
	var buf bytes.Buffer
	if err := WriteGradeSheet(&buf, sec, a.ID, a.Title, st, rows, lang == "en", h.svc.now()); h.failed(w, r, err) {
		return
	}
	suffix := "puanlar"
	if lang == "en" {
		suffix = "grades"
	}
	w.Header().Set("Content-Type", sheet.MediaXLSX)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s_%s_%s.xlsx"; filename*=UTF-8''%s`,
		sec.CourseCode, sec.Code, asciiName(a.Title), suffix, url.PathEscape(sec.CourseCode+"-"+sec.Code+"_"+a.Title+"_"+suffix+".xlsx")))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	w.WriteHeader(http.StatusOK)
	_, _ = buf.WriteTo(w)
}

// asciiName, dosya adının ASCII karşılığıdır (eski tarayıcılar için filename).
func asciiName(s string) string {
	repl := strings.NewReplacer("ç", "c", "Ç", "C", "ğ", "g", "Ğ", "G", "ı", "i", "İ", "I", "ö", "o", "Ö", "O", "ş", "s", "Ş", "S", "ü", "u", "Ü", "U")
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
			return r
		}
		return '_'
	}, repl.Replace(s))
}

type gradeImportRowResponse struct {
	Line            int              `json:"line"`
	StudentNo       string           `json:"student_no"`
	Student         *studentResponse `json:"student"`
	Status          string           `json:"status"`
	Current         *float64         `json:"current_score"`
	Next            *float64         `json:"next_score"`
	FeedbackChanged bool             `json:"feedback_changed"`
	ErrorCode       *string          `json:"error_code"`
	Error           *string          `json:"error"`
}

type gradeImportResponse struct {
	Version        int                      `json:"version"`
	ScoreColumn    *string                  `json:"score_column"`
	FeedbackColumn *string                  `json:"feedback_column"`
	Rows           []gradeImportRowResponse `json:"rows"`
	Missing        []studentResponse        `json:"missing"`
	Summary        struct {
		Changed   int `json:"changed"`
		Unchanged int `json:"unchanged"`
		Empty     int `json:"empty"`
		Errors    int `json:"errors"`
	} `json:"summary"`
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func pointsPtr(p *grading.Points) *float64 {
	if p == nil {
		return nil
	}
	v := p.Float()
	return &v
}

func toGradeImport(gi GradeImport) gradeImportResponse {
	res := gradeImportResponse{Version: gi.Version, ScoreColumn: optional(gi.ScoreColumn), FeedbackColumn: optional(gi.FeedbackColumn),
		Rows: make([]gradeImportRowResponse, 0, len(gi.Rows)), Missing: make([]studentResponse, 0, len(gi.Missing))}
	res.Summary.Changed, res.Summary.Unchanged, res.Summary.Empty, res.Summary.Errors = gi.Changed, gi.Unchanged, gi.Empty, gi.Errors
	for _, row := range gi.Rows {
		item := gradeImportRowResponse{Line: row.Line, StudentNo: row.StudentNo, Status: row.Status, Current: pointsPtr(row.Current),
			Next: pointsPtr(row.Next), FeedbackChanged: row.FeedbackChanged, ErrorCode: optional(row.ErrorCode), Error: optional(row.Error)}
		if row.Student != nil {
			st := studentResponse(*row.Student)
			item.Student = &st
		}
		res.Rows = append(res.Rows, item)
	}
	for _, st := range gi.Missing {
		res.Missing = append(res.Missing, studentResponse(st))
	}
	return res
}

func (h *Handler) previewGradeImport(w http.ResponseWriter, r *http.Request) {
	a, st, sec, _, ok := h.teacher(w, r, true)
	if !ok {
		return
	}
	data, mediaType, err := httpx.ReadBody(w, r, sheet.MediaCSV, sheet.MediaXLSX)
	if err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	gi, err := h.svc.PreviewGradeImport(r.Context(), sec, a.ID, a.Title, st, data, mediaType)
	if h.importFailed(w, r, err) {
		return
	}
	h.writeJSON(w, r, http.StatusOK, toGradeImport(gi))
}

func (h *Handler) importGrades(w http.ResponseWriter, r *http.Request) {
	version, ok := httpx.IfMatchVersion(r)
	if !ok {
		// Hiç puan yokken sürüm 0'dır: "0" da kabul edilir.
		if strings.Trim(r.Header.Get("If-Match"), `W/"`) != "0" {
			httpx.PreconditionRequired(w, r)
			return
		}
	}
	a, _, sec, role, ok := h.teacher(w, r, true)
	if !ok {
		return
	}
	data, mediaType, err := httpx.ReadBody(w, r, sheet.MediaCSV, sheet.MediaXLSX)
	if err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	if _, err := h.svc.ImportGrades(r.Context(), userID(r), sec, a.ID, a.Title, version, data, mediaType); h.importFailed(w, r, err) {
		return
	}
	st, err := h.svc.Assignment(r.Context(), h.svc.pool, a.ID)
	if h.failed(w, r, err) {
		return
	}
	h.writeRoster(w, r, a, st, sec, role)
}

func (h *Handler) importFailed(w http.ResponseWriter, r *http.Request, err error) bool {
	var (
		fileErr *sheet.Error
		rowsErr *GradeRowsError
	)
	switch {
	case errors.As(err, &fileErr):
		_ = httpx.WriteProblem(w, r, httpx.Problem{Status: http.StatusBadRequest, Code: "INVALID_FILE", Detail: fileErr.Detail})
	case errors.As(err, &rowsErr):
		fields := make([]httpx.FieldError, 0, len(rowsErr.Rows))
		for _, row := range rowsErr.Rows {
			fields = append(fields, httpx.FieldError{Field: "rows." + strconv.Itoa(row.Line), Code: row.ErrorCode, Message: row.Error})
		}
		httpx.ValidationFailed(w, r, fields)
	default:
		return h.failed(w, r, err)
	}
	return true
}

// --- Not tablosu bağlantısı ------------------------------------------------------

type linkResponse struct {
	Link *struct {
		ComponentID string  `json:"component_id"`
		Weight      float64 `json:"weight"`
	} `json:"link"`
	Components      []linkableComponentResponse `json:"components"`
	PlanLocked      bool                        `json:"plan_locked"`
	GradesPublished bool                        `json:"grades_published"`
	SyncState       *SyncState                  `json:"sync_state"`
}

type linkableComponentResponse struct {
	ID      string  `json:"id"`
	LabelTR string  `json:"label_tr"`
	LabelEN string  `json:"label_en"`
	Weight  float64 `json:"weight"`
	Linked  int     `json:"linked_assignments"`
}

func (h *Handler) writeLink(w http.ResponseWriter, r *http.Request, sec Section, activityID string, state *SyncState) {
	info, err := h.svc.LinkInfo(r.Context(), sec.ID, activityID)
	if h.failed(w, r, err) {
		return
	}
	res := linkResponse{Components: make([]linkableComponentResponse, 0, len(info.Components)),
		PlanLocked: info.PlanLocked, GradesPublished: info.GradesPublished, SyncState: state}
	if info.Link != nil {
		res.Link = &struct {
			ComponentID string  `json:"component_id"`
			Weight      float64 `json:"weight"`
		}{info.Link.ComponentID, info.Link.Weight.Float()}
	}
	for _, c := range info.Components {
		res.Components = append(res.Components, linkableComponentResponse(c))
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

func (h *Handler) getLink(w http.ResponseWriter, r *http.Request) {
	if a, _, sec, _, ok := h.teacher(w, r, false); ok {
		h.writeLink(w, r, sec, a.ID, nil)
	}
}

func (h *Handler) setLink(w http.ResponseWriter, r *http.Request) {
	a, _, sec, _, ok := h.teacher(w, r, true)
	if !ok {
		return
	}
	var req struct {
		ComponentID *string  `json:"component_id"`
		Weight      *float64 `json:"weight"`
	}
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	componentID, weight := "", grading.Points(100) // varsayılan ağırlık 1
	if req.ComponentID != nil {
		componentID = *req.ComponentID
		if !httpx.ValidUUID(componentID) {
			httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "component_id", Message: "Geçerli bir UUID olmalı."}})
			return
		}
	}
	if req.Weight != nil {
		p, err := points("weight", *req.Weight)
		if h.failed(w, r, err) {
			return
		}
		weight = p
	}
	state, err := h.svc.SetLink(r.Context(), userID(r), sec.ID, a.ID, componentID, weight)
	if h.failed(w, r, err) {
		return
	}
	h.writeLink(w, r, sec, a.ID, &state)
}
