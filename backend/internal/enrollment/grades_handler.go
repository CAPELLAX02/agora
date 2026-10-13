package enrollment

import (
	"errors"
	"net/http"
	"time"

	"github.com/CAPELLAX02/agora/backend/internal/grading"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

func (h *Handler) registerGrades(rt *authz.Router) {
	// Öğrenci kendininkini, danışman danışmanlığındakileri, birimler kapsamlarındakileri görür:
	// erişime politika fonksiyonu karar verir.
	rt.HandleFunc("GET /api/v1/student-programs/{id}/grades", authz.Authenticated, h.grades)
}

type gradesTermResponse struct {
	ID        string `json:"id"`
	Code      string `json:"code"`
	TermType  string `json:"term_type"`
	IsCurrent bool   `json:"is_current"`
}

type gradesComponentResponse struct {
	ID          string     `json:"id"`
	TypeCode    string     `json:"type_code"`
	Category    string     `json:"category"`
	LabelTR     string     `json:"label_tr"`
	LabelEN     string     `json:"label_en"`
	Weight      float64    `json:"weight"`
	ScheduledOn *string    `json:"scheduled_on"`
	PublishedAt *time.Time `json:"published_at"`
	Score       *float64   `json:"score"`
	Absent      bool       `json:"absent"`
	Average     *float64   `json:"class_average"`
}

type gradesClassStatsResponse struct {
	StudentCount int      `json:"student_count"`
	Mean         *float64 `json:"mean"`
	Median       *float64 `json:"median"`
}

type gradesCourseResponse struct {
	EnrollmentID     string                    `json:"enrollment_id"`
	Course           attemptCourseResponse     `json:"course"`
	SectionCode      string                    `json:"section_code"`
	ECTS             float64                   `json:"ects"`
	Status           string                    `json:"status"`
	AttendanceStatus string                    `json:"attendance_status"`
	LetterGrade      *string                   `json:"letter_grade"`
	IsPassing        *bool                     `json:"is_passing"`
	WeightedScore    *float64                  `json:"weighted_score"`
	GradePublishedAt *time.Time                `json:"grade_published_at"`
	Components       []gradesComponentResponse `json:"components"`
	ClassStats       *gradesClassStatsResponse `json:"class_stats"`
}

type gradesResponse struct {
	StudentProgramID string                 `json:"student_program_id"`
	Terms            []gradesTermResponse   `json:"terms"`
	Term             gradesTermResponse     `json:"term"`
	Courses          []gradesCourseResponse `json:"courses"`
}

func toGradesTerm(t grading.StudentTerm) gradesTermResponse {
	return gradesTermResponse{ID: t.ID, Code: t.Code, TermType: t.TermType, IsCurrent: t.Current}
}

func pointsFloat(p *grading.Points) *float64 {
	if p == nil {
		return nil
	}
	v := p.Float()
	return &v
}

func (h *Handler) grades(w http.ResponseWriter, r *http.Request) {
	sp, ok := h.recordProgram(w, r, h.service.CanReadGrades)
	if !ok {
		return
	}
	termID := r.URL.Query().Get("term_id")
	if termID != "" && !httpx.ValidUUID(termID) {
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "term_id", Message: "geçerli bir UUID olmalı"}})
		return
	}
	g, err := grading.StudentGradesFor(r.Context(), h.service.pool, sp.ID, termID)
	switch {
	case errors.Is(err, grading.ErrNoTerm) && termID == "":
		// Henüz ders almamış öğrenci: boş liste.
		h.writeJSON(w, r, http.StatusOK, gradesResponse{StudentProgramID: sp.ID, Terms: []gradesTermResponse{}, Courses: []gradesCourseResponse{}})
		return
	case errors.Is(err, grading.ErrNoTerm):
		httpx.NotFound(w, r)
		return
	case err != nil:
		h.serverError(w, r, err)
		return
	}
	res := gradesResponse{
		StudentProgramID: sp.ID, Term: toGradesTerm(g.Term),
		Terms:   make([]gradesTermResponse, 0, len(g.Terms)),
		Courses: make([]gradesCourseResponse, 0, len(g.Courses)),
	}
	for _, t := range g.Terms {
		res.Terms = append(res.Terms, toGradesTerm(t))
	}
	for _, c := range g.Courses {
		cr := gradesCourseResponse{
			EnrollmentID: c.EnrollmentID,
			Course:       attemptCourseResponse{ID: c.CourseID, Code: c.CourseCode, NameTR: c.CourseNameTR, NameEN: c.CourseNameEN},
			SectionCode:  c.SectionCode, ECTS: c.ECTS, Status: c.Status, AttendanceStatus: string(c.Attendance),
			LetterGrade: optional(c.Letter), IsPassing: c.Passing, WeightedScore: pointsFloat(c.Weighted),
			GradePublishedAt: c.PublishedAt, Components: make([]gradesComponentResponse, 0, len(c.Components)),
		}
		for _, comp := range c.Components {
			item := gradesComponentResponse{
				ID: comp.ID, TypeCode: comp.Type.Code, Category: string(comp.Type.Category),
				LabelTR: comp.Label(false, comp.Numbered), LabelEN: comp.Label(true, comp.Numbered),
				Weight: comp.Weight, PublishedAt: comp.PublishedAt, Score: pointsFloat(comp.Score),
				Absent: comp.Absent, Average: pointsFloat(comp.Average),
			}
			if comp.ScheduledOn != nil {
				d := comp.ScheduledOn.Format(time.DateOnly)
				item.ScheduledOn = &d
			}
			cr.Components = append(cr.Components, item)
		}
		if st := c.ClassStats; st != nil {
			cr.ClassStats = &gradesClassStatsResponse{StudentCount: st.Students, Mean: pointsFloat(st.Mean), Median: pointsFloat(st.Median)}
		}
		res.Courses = append(res.Courses, cr)
	}
	h.writeJSON(w, r, http.StatusOK, res)
}
