package enrollment

import (
	"net/http"
	"time"

	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

func (h *Handler) registerTranscript(rt *authz.Router) {
	// Öğrenci kendisininkini, danışman danışmanlığındakileri, birimler kapsamlarındakileri
	// görür: erişime politika fonksiyonu karar verir.
	rt.HandleFunc("GET /api/v1/student-programs/{id}/transcript", authz.Authenticated, h.transcript)
}

type transcriptCourseResponse struct {
	EnrollmentID string                `json:"enrollment_id"`
	Course       attemptCourseResponse `json:"course"`
	ECTS         float64               `json:"ects"`
	Credit       float64               `json:"credit"`
	Status       string                `json:"status"`
	LetterGrade  *string               `json:"letter_grade"`
	Coefficient  *float64              `json:"coefficient"`
	IsPassing    bool                  `json:"is_passing"`
	CountsInGPA  bool                  `json:"counts_in_gpa"`
	AttemptNo    int                   `json:"attempt_no"`
	IsRepeat     bool                  `json:"is_repeat"`
	Superseded   bool                  `json:"superseded"`
	Source       string                `json:"source"`
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
				IsRepeat: c.Repeat, Superseded: c.Superseded, Source: c.Source,
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

func (h *Handler) transcript(w http.ResponseWriter, r *http.Request) {
	sp, ok := h.studentProgram(w, r)
	if !ok {
		return
	}
	principal, _ := authn.PrincipalFrom(r.Context())
	perms, _ := authz.PermissionsFrom(r.Context())
	allowed, err := h.service.CanReadTranscript(r.Context(), perms, principal.UserID, sp)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	if !allowed {
		httpx.NotFound(w, r)
		return
	}
	t, err := h.repo.Transcript(r.Context(), sp, time.Now())
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	h.writeJSON(w, r, http.StatusOK, toTranscriptResponse(t))
}
