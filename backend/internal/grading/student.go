package grading

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// ErrNoTerm: öğrencinin istenen dönemde (ya da hiç) kesin kaydı yok.
var ErrNoTerm = errors.New("grading: bu dönemde kesin kayıt yok")

// StudentTerm, öğrencinin ders aldığı bir dönemdir.
type StudentTerm struct {
	ID       string
	Code     string
	TermType string
	Current  bool
}

// StudentComponent, öğrencinin bir değerlendirme bileşenindeki durumudur. Not ve sınıf
// ortalaması sadece bileşen yayımlandıktan sonra görünür.
type StudentComponent struct {
	Component
	Numbered    bool // aynı türden birden fazla bileşen var: etiket numaralı
	PublishedAt *time.Time
	Score       *Points // yayımlanmamışsa ya da girmediyse nil
	Absent      bool
	Average     *Points // sınıfın girilen puanlarının ortalaması (girmedi hariç)
}

// ClassStats, şubenin sınıf istatistiğidir (harf notları ilan edilince).
type ClassStats struct {
	Students int
	Mean     *Points
	Median   *Points
}

// StudentCourse, öğrencinin bir dönemdeki bir dersidir.
type StudentCourse struct {
	EnrollmentID string
	SectionID    string
	SectionCode  string
	CourseID     string
	CourseCode   string
	CourseNameTR string
	CourseNameEN string
	ECTS         float64
	Status       string // ENROLLED, COMPLETED
	Attendance   Attendance
	Letter       string
	Passing      *bool
	Weighted     *Points
	PublishedAt  *time.Time // harf notunun ilanı
	Components   []StudentComponent
	ClassStats   *ClassStats
}

// StudentGrades, öğrencinin bir dönemdeki notlarıdır.
type StudentGrades struct {
	Terms   []StudentTerm // ders aldığı dönemler, eskiden yeniye
	Term    StudentTerm
	Courses []StudentCourse
}

// StudentGradesFor, program kaydının bir dönemdeki notlarını okur. termID boşsa içinde
// bulunulan dönem, o dönemde dersi yoksa ders aldığı son dönem. Bileşen notları ve sınıf
// ortalamaları sadece yayımlanan bileşenlerde görünür; bütünleme bileşeni sadece öğrencinin
// bütünleme notu varsa listelenir.
func StudentGradesFor(ctx context.Context, q db.Querier, studentProgramID, termID string) (StudentGrades, error) {
	var g StudentGrades
	rows, err := q.Query(ctx, `
		SELECT DISTINCT t.id, t.code, t.term_type, t.is_current, t.starts_on
		FROM enrollment.enrollments e JOIN academic.terms t ON t.id = e.term_id
		WHERE e.student_program_id = $1 AND e.status <> 'DROPPED'
		ORDER BY t.starts_on`, studentProgramID)
	if err != nil {
		return g, fmt.Errorf("grading: dönemler okunamadı: %w", err)
	}
	g.Terms, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (StudentTerm, error) {
		var (
			t        StudentTerm
			startsOn time.Time
		)
		return t, row.Scan(&t.ID, &t.Code, &t.TermType, &t.Current, &startsOn)
	})
	if err != nil {
		return g, fmt.Errorf("grading: dönemler okunamadı: %w", err)
	}
	if len(g.Terms) == 0 {
		return g, ErrNoTerm
	}
	found := false
	for _, t := range g.Terms {
		if (termID != "" && t.ID == termID) || (termID == "" && t.Current) {
			g.Term, found = t, true
		}
	}
	switch {
	case !found && termID != "":
		return g, ErrNoTerm
	case !found:
		g.Term = g.Terms[len(g.Terms)-1]
	}

	rows, err = q.Query(ctx, `
		SELECT e.id, s.id, s.section_code, c.id, c.code, c.name_tr, c.name_en, e.ects_snapshot::float8, e.status,
		       e.attendance_status, coalesce(e.letter_grade, ''), gi.is_passing, (e.weighted_score * 100)::int8,
		       e.grade_published_at, st.student_count, (st.mean * 100)::int8, (st.median * 100)::int8
		FROM enrollment.enrollments e
		JOIN offering.sections s  ON s.id = e.section_id
		JOIN curriculum.courses c ON c.id = e.course_id
		LEFT JOIN curriculum.grade_scale_items gi ON gi.grade_scale_id = e.grade_scale_id AND gi.letter = e.letter_grade
		LEFT JOIN grading.section_grade_stats st ON st.section_id = s.id
		WHERE e.student_program_id = $1 AND e.term_id = $2 AND e.status <> 'DROPPED'
		ORDER BY c.code`, studentProgramID, g.Term.ID)
	if err != nil {
		return g, fmt.Errorf("grading: dersler okunamadı: %w", err)
	}
	points := func(v *int64) *Points {
		if v == nil {
			return nil
		}
		p := Points(*v)
		return &p
	}
	g.Courses, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (StudentCourse, error) {
		var (
			c                      StudentCourse
			att                    string
			weighted, mean, median *int64
			students               *int
		)
		err := row.Scan(&c.EnrollmentID, &c.SectionID, &c.SectionCode, &c.CourseID, &c.CourseCode, &c.CourseNameTR,
			&c.CourseNameEN, &c.ECTS, &c.Status, &att, &c.Letter, &c.Passing, &weighted, &c.PublishedAt,
			&students, &mean, &median)
		c.Attendance, c.Weighted = Attendance(att), points(weighted)
		if students != nil {
			c.ClassStats = &ClassStats{Students: *students, Mean: points(mean), Median: points(median)}
		}
		return c, err
	})
	if err != nil {
		return g, fmt.Errorf("grading: dersler okunamadı: %w", err)
	}
	if len(g.Courses) == 0 {
		return g, nil
	}

	index := make(map[string]int, len(g.Courses))
	enrollments := make([]string, len(g.Courses))
	for i, c := range g.Courses {
		index[c.EnrollmentID], enrollments[i] = i, c.EnrollmentID
	}
	rows, err = q.Query(ctx, `
		SELECT e.id, ac.id, t.code, t.name_tr, t.name_en, t.category, ac.sequence_no, coalesce(ac.name_tr, ''),
		       coalesce(ac.name_en, ''), ac.weight::float8, ac.scheduled_on, ac.published_at,
		       count(*) OVER (PARTITION BY e.id, ac.type_code) > 1,
		       coalesce(my.status, ''), (my.score * 100)::int8,
		       coalesce(avg.sum, 0), coalesce(avg.n, 0)
		FROM enrollment.enrollments e
		JOIN grading.assessment_components ac ON ac.section_id = e.section_id
		JOIN grading.assessment_types t ON t.code = ac.type_code
		LEFT JOIN grading.assessment_scores my ON my.component_id = ac.id AND my.enrollment_id = e.id
		LEFT JOIN LATERAL (
			SELECT sum((x.score * 100)::int8) AS sum, count(*)::int AS n
			FROM grading.assessment_scores x
			JOIN enrollment.enrollments xe ON xe.id = x.enrollment_id AND xe.status <> 'DROPPED'
			WHERE x.component_id = ac.id AND x.status = 'SCORED'
		) avg ON ac.published_at IS NOT NULL
		WHERE e.id = ANY($1)
		ORDER BY e.id, t.category = 'MAKEUP', t.category = 'FINAL', ac.position, t.sort_order, ac.sequence_no`, enrollments)
	if err != nil {
		return g, fmt.Errorf("grading: bileşenler okunamadı: %w", err)
	}
	var (
		enrollmentID, category, status string
		seq                            int16
		score                          *int64
		sum                            int64
		n                              int
		c                              StudentComponent
	)
	_, err = pgx.ForEachRow(rows, []any{&enrollmentID, &c.ID, &c.Type.Code, &c.Type.NameTR, &c.Type.NameEN, &category,
		&seq, &c.NameTR, &c.NameEN, &c.Weight, &c.ScheduledOn, &c.PublishedAt, &c.Numbered, &status, &score, &sum, &n}, func() error {
		comp := c
		comp.Type.Category, comp.SequenceNo = Category(category), int(seq)
		comp.ScheduledOn, comp.PublishedAt = copyTime(c.ScheduledOn), copyTime(c.PublishedAt)
		if comp.PublishedAt != nil {
			comp.Absent = status == "ABSENT"
			comp.Score = points(score)
			if n > 0 {
				avg := mean(Points(sum), n)
				comp.Average = &avg
			}
		}
		// Bütünleme sadece öğrencinin bütünleme notu varsa listelenir.
		if comp.Type.Category == CategoryMakeup && status == "" {
			return nil
		}
		course := &g.Courses[index[enrollmentID]]
		course.Components = append(course.Components, comp)
		return nil
	})
	if err != nil {
		return g, fmt.Errorf("grading: bileşenler okunamadı: %w", err)
	}
	return g, nil
}

func copyTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	v := *t
	return &v
}
