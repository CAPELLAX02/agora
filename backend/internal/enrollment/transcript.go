package enrollment

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
)

// TranscriptCourse, transkriptteki bir ders satırıdır: dersin bir dönemde alınışı.
type TranscriptCourse struct {
	EnrollmentID string
	CourseID     string
	CourseCode   string
	CourseNameTR string
	CourseNameEN string
	ECTS         float64
	Credit       float64
	Status       EnrollmentStatus // COMPLETED: notu verildi, ENROLLED: dönem sürüyor
	Letter       string           // notu yoksa boş
	Coefficient  *float64
	Passing      bool
	CountsInGPA  bool
	AttemptNo    int
	// Repeat, dersin (ya da eşdeğerinin) tekrar alınışı ya da kalınan seçmelinin yerine
	// alınan ders olduğunu söyler.
	Repeat bool
	// Superseded, yerini sonraki bir denemenin aldığını söyler: son not geçerli olduğu için bu
	// satır sonraki dönemlerin genel ortalamasına girmez.
	Superseded bool
	Source     string // REGISTRATION, ADD_DROP, IMPORT
}

// TranscriptResult, bir dönemin kayıtlı sonucudur (enrollment.term_results).
type TranscriptResult struct {
	AttemptedECTS  float64
	EarnedECTS     float64
	TermGPA        *float64
	CumulativeECTS float64
	CumulativeGPA  *float64
	Standing       string // NORMAL, HONOR, HIGH_HONOR
}

// TranscriptTerm, transkriptin bir dönemidir.
type TranscriptTerm struct {
	TermID   string
	TermCode string
	TermType string // FALL, SPRING, SUMMER
	Courses  []TranscriptCourse
	Result   *TranscriptResult // notu verilmiş dersi yoksa nil
}

// Transcript, program kaydının dönem dönem ders ve not dökümüdür.
type Transcript struct {
	StudentProgram
	ProgramNameEN string
	DegreeLevel   string
	RequiredECTS  float64 // izlenen müfredatın (yoksa programın) mezuniyet için istediği AKTS
	Terms         []TranscriptTerm
	GeneratedAt   time.Time
}

// Latest, son dönem sonucudur (güncel GANO ve kazanılan AKTS); sonuç yoksa nil.
func (t Transcript) Latest() *TranscriptResult {
	for i := len(t.Terms) - 1; i >= 0; i-- {
		if t.Terms[i].Result != nil {
			return t.Terms[i].Result
		}
	}
	return nil
}

// Transcript, program kaydının transkriptini okur: bırakılmamış bütün ders kayıtları dönem
// sırasıyla, dönem sonuçlarıyla.
func (r *Repository) Transcript(ctx context.Context, sp StudentProgram, now time.Time) (Transcript, error) {
	t := Transcript{StudentProgram: sp, GeneratedAt: now}
	if err := r.db.QueryRow(ctx, `
		SELECT p.name_en, p.degree_level, coalesce(c.total_ects_required, p.total_ects_required)::float8
		FROM enrollment.student_programs sp
		JOIN org.programs p ON p.id = sp.program_id
		LEFT JOIN curriculum.curricula c ON c.id = sp.curriculum_id
		WHERE sp.id = $1`, sp.ID).Scan(&t.ProgramNameEN, &t.DegreeLevel, &t.RequiredECTS); err != nil {
		return Transcript{}, fmt.Errorf("enrollment: program okunamadı: %w", err)
	}

	rows, err := r.db.Query(ctx, `
		SELECT t.id, t.code, t.term_type, e.id, c.id, c.code, c.name_tr, c.name_en,
		       e.ects_snapshot::float8, e.credit_snapshot::float8, e.status, coalesce(e.letter_grade, ''),
		       gi.coefficient::float8, coalesce(gi.is_passing, false), coalesce(gi.counts_in_gpa, false), e.attempt_no,
		       e.attempt_no > 1 OR coalesce(ri.item_kind, 'NEW') <> 'NEW', e.superseded_by_id IS NOT NULL, e.source
		FROM enrollment.enrollments e
		JOIN academic.terms t     ON t.id = e.term_id
		JOIN curriculum.courses c ON c.id = e.course_id
		LEFT JOIN curriculum.grade_scale_items gi ON gi.grade_scale_id = e.grade_scale_id AND gi.letter = e.letter_grade
		LEFT JOIN enrollment.registration_items ri ON ri.id = e.registration_item_id
		WHERE e.student_program_id = $1 AND e.status <> 'DROPPED'
		ORDER BY t.starts_on, c.code`, sp.ID)
	if err != nil {
		return Transcript{}, fmt.Errorf("enrollment: transkript okunamadı: %w", err)
	}
	type line struct {
		term   TranscriptTerm
		course TranscriptCourse
	}
	lines, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (line, error) {
		var (
			l      line
			status string
		)
		c := &l.course
		err := row.Scan(&l.term.TermID, &l.term.TermCode, &l.term.TermType, &c.EnrollmentID, &c.CourseID, &c.CourseCode,
			&c.CourseNameTR, &c.CourseNameEN, &c.ECTS, &c.Credit, &status, &c.Letter, &c.Coefficient, &c.Passing,
			&c.CountsInGPA, &c.AttemptNo, &c.Repeat, &c.Superseded, &c.Source)
		c.Status = EnrollmentStatus(status)
		return l, err
	})
	if err != nil {
		return Transcript{}, fmt.Errorf("enrollment: transkript okunamadı: %w", err)
	}
	for _, l := range lines {
		if n := len(t.Terms); n == 0 || t.Terms[n-1].TermID != l.term.TermID {
			t.Terms = append(t.Terms, l.term)
		}
		term := &t.Terms[len(t.Terms)-1]
		term.Courses = append(term.Courses, l.course)
	}

	rows, err = r.db.Query(ctx, `
		SELECT term_id, attempted_ects::float8, earned_ects::float8, term_gpa::float8, cumulative_ects::float8,
		       cumulative_gpa::float8, standing
		FROM enrollment.term_results WHERE student_program_id = $1`, sp.ID)
	if err != nil {
		return Transcript{}, fmt.Errorf("enrollment: dönem sonuçları okunamadı: %w", err)
	}
	type termResult struct {
		termID string
		TranscriptResult
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (termResult, error) {
		var res termResult
		err := row.Scan(&res.termID, &res.AttemptedECTS, &res.EarnedECTS, &res.TermGPA,
			&res.CumulativeECTS, &res.CumulativeGPA, &res.Standing)
		return res, err
	})
	if err != nil {
		return Transcript{}, fmt.Errorf("enrollment: dönem sonuçları okunamadı: %w", err)
	}
	results := make(map[string]TranscriptResult, len(list))
	for _, res := range list {
		results[res.termID] = res.TranscriptResult
	}
	for i := range t.Terms {
		if res, ok := results[t.Terms[i].TermID]; ok {
			t.Terms[i].Result = &res
		}
	}
	return t, nil
}

// CanReadTranscript, transkripti görebilecekler: öğrencinin kendisi (`transcript:read_own`),
// kapsamındaki birimler ve öğrencinin şu anki danışmanı (`transcript:read_scoped`; danışmanın
// yetkisi ilişkiden gelir).
func (s *Service) CanReadTranscript(ctx context.Context, perms *authz.Permissions, userID string, sp StudentProgram) (bool, error) {
	r := NewRepository(s.pool)
	st, err := r.Student(ctx, sp.StudentID)
	if err != nil {
		return false, err
	}
	switch {
	case st.UserID != "" && st.UserID == userID:
		return perms.Has("transcript:read_own"), nil
	case perms.Allows("transcript:read_scoped", sp.Program.Target()):
		return true, nil
	case !perms.Has("transcript:read_scoped"):
		return false, nil
	}
	staffID, err := r.StaffByUser(ctx, userID)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return r.IsCurrentAdvisor(ctx, staffID, st.ID)
}
