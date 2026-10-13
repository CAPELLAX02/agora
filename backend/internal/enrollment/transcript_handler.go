package enrollment

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/CAPELLAX02/agora/backend/internal/grading"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

func (h *Handler) registerTranscript(rt *authz.Router) {
	// Öğrenci kendisininkini, danışman danışmanlığındakileri, birimler kapsamlarındakileri
	// görür: erişime politika fonksiyonu karar verir.
	rt.HandleFunc("GET /api/v1/student-programs/{id}/transcript", authz.Authenticated, h.transcript)
	// Sanal transkript: transkripti görebilen varsayımsal harflerle hesaplatabilir; bir şey
	// kaydedilmez.
	rt.HandleFunc("POST /api/v1/student-programs/{id}/transcript/simulate", authz.Authenticated, h.simulateTranscript)
}

type transcriptCourseResponse struct {
	EnrollmentID  string                `json:"enrollment_id"`
	Course        attemptCourseResponse `json:"course"`
	ECTS          float64               `json:"ects"`
	Credit        float64               `json:"credit"`
	Status        string                `json:"status"`
	LetterGrade   *string               `json:"letter_grade"`
	Coefficient   *float64              `json:"coefficient"`
	IsPassing     bool                  `json:"is_passing"`
	CountsInGPA   bool                  `json:"counts_in_gpa"`
	AttemptNo     int                   `json:"attempt_no"`
	IsRepeat      bool                  `json:"is_repeat"`
	Superseded    bool                  `json:"superseded"`
	Source        string                `json:"source"`
	LetterOptions []string              `json:"letter_options"`
}

type transcriptResultResponse struct {
	AttemptedECTS  float64  `json:"attempted_ects"`
	EarnedECTS     float64  `json:"earned_ects"`
	TermGPA        *float64 `json:"term_gpa"`
	CumulativeECTS float64  `json:"cumulative_ects"`
	CumulativeGPA  *float64 `json:"cumulative_gpa"`
	Standing       string   `json:"standing"`
}

type transcriptTermResponse struct {
	Term    transcriptTermRef          `json:"term"`
	Courses []transcriptCourseResponse `json:"courses"`
	Result  *transcriptResultResponse  `json:"result"`
}

type transcriptTermRef struct {
	ID       string `json:"id"`
	Code     string `json:"code"`
	TermType string `json:"term_type"`
}

type transcriptSummaryResponse struct {
	GPA              *float64 `json:"gpa"`
	EarnedECTS       float64  `json:"earned_ects"`
	RequiredECTS     float64  `json:"required_ects"`
	CompletedCourses int      `json:"completed_courses"`
	InProgress       int      `json:"in_progress"`
	HonorTerms       int      `json:"honor_terms"`
	HighHonorTerms   int      `json:"high_honor_terms"`
}

type transcriptResponse struct {
	StudentProgram studentProgramResponse    `json:"student_program"`
	ProgramNameEN  string                    `json:"program_name_en"`
	DegreeLevel    string                    `json:"degree_level"`
	Terms          []transcriptTermResponse  `json:"terms"`
	Summary        transcriptSummaryResponse `json:"summary"`
	GeneratedAt    time.Time                 `json:"generated_at"`
}

func toTranscriptResponse(t Transcript) transcriptResponse {
	res := transcriptResponse{
		StudentProgram: toStudentProgramResponse(t.StudentProgram),
		ProgramNameEN:  t.ProgramNameEN, DegreeLevel: t.DegreeLevel,
		Terms:       make([]transcriptTermResponse, 0, len(t.Terms)),
		Summary:     transcriptSummaryResponse{RequiredECTS: t.RequiredECTS},
		GeneratedAt: t.GeneratedAt,
	}
	for _, term := range t.Terms {
		tr := transcriptTermResponse{
			Term:    transcriptTermRef{ID: term.TermID, Code: term.TermCode, TermType: term.TermType},
			Courses: make([]transcriptCourseResponse, 0, len(term.Courses)),
		}
		for _, c := range term.Courses {
			tr.Courses = append(tr.Courses, transcriptCourseResponse{
				EnrollmentID: c.EnrollmentID,
				Course:       attemptCourseResponse{ID: c.CourseID, Code: c.CourseCode, NameTR: c.CourseNameTR, NameEN: c.CourseNameEN},
				ECTS:         c.ECTS, Credit: c.Credit, Status: string(c.Status), LetterGrade: optional(c.Letter),
				Coefficient: c.Coefficient, IsPassing: c.Passing, CountsInGPA: c.CountsInGPA, AttemptNo: c.AttemptNo,
				IsRepeat: c.Repeat, Superseded: c.Superseded, Source: c.Source, LetterOptions: c.LetterOptions,
			})
			switch c.Status {
			case EnrollmentCompleted:
				res.Summary.CompletedCourses++
			case EnrollmentEnrolled:
				res.Summary.InProgress++
			}
		}
		if r := term.Result; r != nil {
			tr.Result = &transcriptResultResponse{
				AttemptedECTS: r.AttemptedECTS, EarnedECTS: r.EarnedECTS, TermGPA: r.TermGPA,
				CumulativeECTS: r.CumulativeECTS, CumulativeGPA: r.CumulativeGPA, Standing: r.Standing,
			}
			switch r.Standing {
			case "HONOR":
				res.Summary.HonorTerms++
			case "HIGH_HONOR":
				res.Summary.HighHonorTerms++
			}
		}
		res.Terms = append(res.Terms, tr)
	}
	if last := t.Latest(); last != nil {
		res.Summary.GPA, res.Summary.EarnedECTS = last.CumulativeGPA, last.CumulativeECTS
	}
	return res
}

// transcriptProgram, path'teki program kaydını okur ve transkripti görme yetkisini denetler:
// yetkisi olmayana kayıt yokmuş gibi 404.
func (h *Handler) transcriptProgram(w http.ResponseWriter, r *http.Request) (StudentProgram, bool) {
	return h.recordProgram(w, r, h.service.CanReadTranscript)
}

// recordProgram, path'teki program kaydını okur ve can ile görme yetkisini denetler: yetkisi
// olmayana kayıt yokmuş gibi 404.
func (h *Handler) recordProgram(w http.ResponseWriter, r *http.Request,
	can func(context.Context, *authz.Permissions, string, StudentProgram) (bool, error)) (StudentProgram, bool) {
	sp, ok := h.studentProgram(w, r)
	if !ok {
		return StudentProgram{}, false
	}
	principal, _ := authn.PrincipalFrom(r.Context())
	perms, _ := authz.PermissionsFrom(r.Context())
	allowed, err := can(r.Context(), perms, principal.UserID, sp)
	if err != nil {
		h.serverError(w, r, err)
		return StudentProgram{}, false
	}
	if !allowed {
		httpx.NotFound(w, r)
		return StudentProgram{}, false
	}
	return sp, true
}

func (h *Handler) transcript(w http.ResponseWriter, r *http.Request) {
	sp, ok := h.transcriptProgram(w, r)
	if !ok {
		return
	}
	t, err := h.repo.Transcript(r.Context(), sp, time.Now())
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	h.writeJSON(w, r, http.StatusOK, toTranscriptResponse(t))
}

type simulateRequest struct {
	Grades []struct {
		EnrollmentID string `json:"enrollment_id"`
		LetterGrade  string `json:"letter_grade"`
	} `json:"grades"`
}

type simulatedTermResponse struct {
	Term      transcriptTermRef         `json:"term"`
	Actual    *transcriptResultResponse `json:"actual"`
	Simulated *transcriptResultResponse `json:"simulated"`
}

type simulationResponse struct {
	Terms   []simulatedTermResponse `json:"terms"`
	Summary struct {
		ActualGPA           *float64 `json:"actual_gpa"`
		SimulatedGPA        *float64 `json:"simulated_gpa"`
		ActualEarnedECTS    float64  `json:"actual_earned_ects"`
		SimulatedEarnedECTS float64  `json:"simulated_earned_ects"`
	} `json:"summary"`
}

func toResultResponse(r *TranscriptResult) *transcriptResultResponse {
	if r == nil {
		return nil
	}
	return &transcriptResultResponse{
		AttemptedECTS: r.AttemptedECTS, EarnedECTS: r.EarnedECTS, TermGPA: r.TermGPA,
		CumulativeECTS: r.CumulativeECTS, CumulativeGPA: r.CumulativeGPA, Standing: r.Standing,
	}
}

func (h *Handler) simulateTranscript(w http.ResponseWriter, r *http.Request) {
	sp, ok := h.transcriptProgram(w, r)
	if !ok {
		return
	}
	var req simulateRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	if len(req.Grades) == 0 || len(req.Grades) > 200 {
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "grades", Message: "1 ile 200 arasında ders olmalı."}})
		return
	}
	var errs []httpx.FieldError
	seen := map[string]bool{}
	changes := make([]grading.Hypothetical, 0, len(req.Grades))
	for _, g := range req.Grades {
		letter := strings.TrimSpace(g.LetterGrade)
		switch {
		case !httpx.ValidUUID(g.EnrollmentID):
			errs = append(errs, httpx.FieldError{Field: "grades.enrollment_id", Message: "geçerli bir UUID olmalı"})
		case seen[g.EnrollmentID]:
			errs = append(errs, httpx.FieldError{Field: "grades.enrollment_id", Message: "Bir ders birden fazla kez verilmiş."})
		case letter == "":
			errs = append(errs, httpx.FieldError{Field: "grades.letter_grade", Message: "Harf notu boş olamaz."})
		}
		seen[g.EnrollmentID] = true
		changes = append(changes, grading.Hypothetical{EnrollmentID: g.EnrollmentID, Letter: letter})
	}
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	terms, err := h.service.SimulateTranscript(r.Context(), sp, changes)
	var notInProgram *grading.NotInProgramError
	switch {
	case errors.As(err, &notInProgram):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{
			Field: "grades.enrollment_id", Message: "Bu programın kesin kaydı değil: " + strings.Join(notInProgram.IDs, ", "),
		}})
		return
	case errors.Is(err, grading.ErrLetterNotAllowed):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "grades.letter_grade", Message: "Bu harf bu derse verilemez."}})
		return
	case err != nil:
		h.serverError(w, r, err)
		return
	}
	res := simulationResponse{Terms: make([]simulatedTermResponse, 0, len(terms))}
	for _, t := range terms {
		res.Terms = append(res.Terms, simulatedTermResponse{
			Term:   transcriptTermRef{ID: t.TermID, Code: t.TermCode, TermType: t.TermType},
			Actual: toResultResponse(t.Actual), Simulated: toResultResponse(t.Simulated),
		})
		if t.Actual != nil {
			res.Summary.ActualGPA, res.Summary.ActualEarnedECTS = t.Actual.CumulativeGPA, t.Actual.CumulativeECTS
		}
		if t.Simulated != nil {
			res.Summary.SimulatedGPA, res.Summary.SimulatedEarnedECTS = t.Simulated.CumulativeGPA, t.Simulated.CumulativeECTS
		}
	}
	h.writeJSON(w, r, http.StatusOK, res)
}
