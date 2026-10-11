package registration

import (
	"math"
	"net/http"

	"github.com/CAPELLAX02/agora/backend/internal/enrollment"
	"github.com/CAPELLAX02/agora/backend/internal/registration/rules"
)

type attemptRefResponse struct {
	CourseID     string  `json:"course_id"`
	CourseCode   string  `json:"course_code"`
	CourseNameTR string  `json:"course_name_tr"`
	CourseNameEN string  `json:"course_name_en"`
	TermCode     string  `json:"term_code"`
	Letter       *string `json:"letter"`
	ECTS         float64 `json:"ects"`
}

type progressLineResponse struct {
	ItemID          string               `json:"item_id"`
	SemesterNo      int                  `json:"semester_no"`
	ItemType        string               `json:"item_type"`
	Code            string               `json:"code"`
	NameTR          string               `json:"name_tr"`
	NameEN          string               `json:"name_en"`
	ECTS            float64              `json:"ects"`
	IsCompulsory    bool                 `json:"is_compulsory"`
	SlotCourseCount int                  `json:"slot_course_count"`
	Status          string               `json:"status"`
	LastAttempt     *attemptRefResponse  `json:"last_attempt"`
	Filled          []attemptRefResponse `json:"filled"`
	InProgress      []attemptRefResponse `json:"in_progress"`
	Remaining       int                  `json:"remaining"`
}

type progressSummaryResponse struct {
	RequiredECTS     float64 `json:"required_ects"`
	CompletedECTS    float64 `json:"completed_ects"`
	InProgressECTS   float64 `json:"in_progress_ects"`
	RemainingCourses int     `json:"remaining_courses"`
}

type curriculumRefResponse struct {
	ID     string `json:"id"`
	NameTR string `json:"name_tr"`
	NameEN string `json:"name_en"`
}

type progressResponse struct {
	StudentProgramID string                  `json:"student_program_id"`
	Curriculum       *curriculumRefResponse  `json:"curriculum"`
	CurrentSemester  int                     `json:"current_semester"`
	GPA              *float64                `json:"gpa"`
	Summary          progressSummaryResponse `json:"summary"`
	Lines            []progressLineResponse  `json:"lines"`
	Extra            []attemptRefResponse    `json:"extra"`
}

// progress, öğrencinin izlediği müfredata göre ders durumunu döndürür: her satır için
// geçti, kaldı, devam ediyor ya da alınmadı; seçmeli yuvalarında dolan dersler.
func (h *Handler) progress(w http.ResponseWriter, r *http.Request) {
	sp, ok := h.readableProgram(w, r)
	if !ok {
		return
	}
	res := progressResponse{
		StudentProgramID: sp.ID, CurrentSemester: sp.CurrentSemester, GPA: sp.GPA,
		Lines: []progressLineResponse{}, Extra: []attemptRefResponse{},
	}
	history, err := h.students.CourseHistory(r.Context(), sp.ID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	if sp.Curriculum == nil {
		for _, a := range latestAttempts(history) {
			res.Extra = append(res.Extra, attemptRef(a))
		}
		h.writeJSON(w, r, http.StatusOK, res)
		return
	}
	res.Curriculum = &curriculumRefResponse{ID: sp.Curriculum.ID, NameTR: sp.Curriculum.NameTR, NameEN: sp.Curriculum.NameEN}

	lines, err := h.repo.Lines(r.Context(), sp.Curriculum.ID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	cat, err := h.repo.Catalog(r.Context(), sp.Curriculum.ID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	p := rules.ComputeProgress(RuleLines(lines), Attempts(history), cat)
	find := attemptFinder(history)

	for i, lp := range p.Lines {
		l := lines[i]
		line := progressLineResponse{
			ItemID: l.ID, SemesterNo: l.Semester, ItemType: "COURSE", Code: l.Code, NameTR: l.NameTR, NameEN: l.NameEN,
			ECTS: l.ECTS, IsCompulsory: l.Compulsory, SlotCourseCount: l.SlotCount, Status: string(lp.Status),
			Filled: []attemptRefResponse{}, InProgress: []attemptRefResponse{}, Remaining: lp.Remaining,
		}
		res.Summary.RequiredECTS += l.ECTS
		if l.IsSlot() {
			line.ItemType = "ELECTIVE_SLOT"
			for _, a := range lp.Filled {
				line.Filled = append(line.Filled, attemptRef(find(a)))
				res.Summary.CompletedECTS += a.ECTS
			}
			for _, a := range lp.InProgress {
				line.InProgress = append(line.InProgress, attemptRef(find(a)))
				res.Summary.InProgressECTS += a.ECTS
			}
		} else {
			if lp.Last != nil {
				ref := attemptRef(find(*lp.Last))
				line.LastAttempt = &ref
			}
			switch lp.Status {
			case rules.Passed:
				res.Summary.CompletedECTS += l.ECTS
			case rules.InProgress:
				res.Summary.InProgressECTS += l.ECTS
			}
		}
		res.Lines = append(res.Lines, line)
	}
	for _, a := range p.Extra {
		res.Extra = append(res.Extra, attemptRef(find(a)))
	}
	res.Summary.RemainingCourses = rules.RemainingCourses(p)
	res.Summary.RequiredECTS = round1(res.Summary.RequiredECTS)
	res.Summary.CompletedECTS = round1(res.Summary.CompletedECTS)
	res.Summary.InProgressECTS = round1(res.Summary.InProgressECTS)
	h.writeJSON(w, r, http.StatusOK, res)
}

// attemptFinder, kural motorunun denemesinden ders geçmişindeki satırı bulur.
func attemptFinder(history []enrollment.CourseAttempt) func(rules.Attempt) enrollment.CourseAttempt {
	type key struct {
		course string
		order  int
	}
	m := make(map[key]enrollment.CourseAttempt, len(history))
	for _, h := range history {
		m[key{h.CourseID, h.TermOrder}] = h
	}
	return func(a rules.Attempt) enrollment.CourseAttempt {
		return m[key{a.CourseID, a.TermOrder}]
	}
}

// latestAttempts, her dersin son denemesidir (dönem sırasıyla).
func latestAttempts(history []enrollment.CourseAttempt) []enrollment.CourseAttempt {
	idx := map[string]int{}
	var out []enrollment.CourseAttempt
	for _, h := range history { // geçmiş dönem sırasıyla gelir
		if i, ok := idx[h.CourseID]; ok {
			out[i] = h
			continue
		}
		idx[h.CourseID] = len(out)
		out = append(out, h)
	}
	return out
}

func attemptRef(a enrollment.CourseAttempt) attemptRefResponse {
	return attemptRefResponse{
		CourseID: a.CourseID, CourseCode: a.CourseCode, CourseNameTR: a.CourseNameTR, CourseNameEN: a.CourseNameEN,
		TermCode: a.TermCode, Letter: optional(a.Letter), ECTS: a.ECTS,
	}
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}
