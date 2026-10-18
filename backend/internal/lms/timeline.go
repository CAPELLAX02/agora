package lms

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
)

// Zaman çizelgesi: kullanıcının dönemdeki ders alanlarında yapılacaklar. Öğrenciye son tarihi
// yaklaşan ve geçmiş ama hâlâ teslim edilebilen (gönderilmemiş) ödevler, öğretim elemanına
// puanlanmayı bekleyen teslimleri olan ödevler.

// Zaman çizelgesi öğeleri.
const (
	TimelineDue     = "DUE"      // son tarihi yaklaşan
	TimelineOverdue = "OVERDUE"  // son tarih geçti, geç teslim hâlâ açık
	TimelineToGrade = "TO_GRADE" // puanlanmayı bekleyen teslimler
)

// timelineHorizon, yaklaşan ödevlerin ne kadar ilerisine bakıldığıdır.
const timelineHorizon = 60 * 24 * time.Hour

// TimelineItem, zaman çizelgesindeki bir ödevdir.
type TimelineItem struct {
	Kind         string
	ActivityID   string
	SectionID    string
	SectionCode  string
	CourseID     string
	CourseCode   string
	CourseNameTR string
	CourseNameEN string
	Title        string
	DueAt        *time.Time
	CutoffAt     *time.Time
	NeedsGrading int
}

// Timeline, kullanıcının dönemdeki zaman çizelgesidir: önce gecikenler, sonra yaklaşanlar (son
// tarihe göre), sonra puanlanacaklar.
func (s *Service) Timeline(ctx context.Context, userID, termID string) ([]TimelineItem, error) {
	now := s.now()
	due, err := s.dueItems(ctx, userID, termID, now)
	if err != nil {
		return nil, err
	}
	grade, err := s.gradingItems(ctx, userID, termID)
	if err != nil {
		return nil, err
	}
	rank := map[string]int{TimelineOverdue: 0, TimelineDue: 1, TimelineToGrade: 2}
	items := append(due, grade...)
	slices.SortStableFunc(items, func(a, b TimelineItem) int {
		if rank[a.Kind] != rank[b.Kind] {
			return rank[a.Kind] - rank[b.Kind]
		}
		switch {
		case a.DueAt == nil && b.DueAt == nil:
			return 0
		case a.DueAt == nil:
			return 1
		case b.DueAt == nil:
			return -1
		}
		return a.DueAt.Compare(*b.DueAt)
	})
	return items, nil
}

const timelineCourse = `s.section_code, c.id, c.code, c.name_tr, c.name_en
	FROM offering.sections s
	JOIN offering.course_offerings o ON o.id = s.offering_id
	JOIN curriculum.courses c ON c.id = o.course_id`

// dueItems, öğrencinin gönderilmemiş, hâlâ teslim edilebilen ve son tarihi olan ödevleridir.
func (s *Service) dueItems(ctx context.Context, userID, termID string, now time.Time) ([]TimelineItem, error) {
	rows, err := s.pool.Query(ctx, `
		WITH mine AS (
			SELECT e.section_id FROM enrollment.enrollments e
			JOIN enrollment.student_programs sp ON sp.id = e.student_program_id
			JOIN people.students st ON st.id = sp.student_id JOIN iam.users u ON u.person_id = st.person_id
			WHERE u.id = $1 AND e.term_id = $2 AND e.status <> 'DROPPED'
		)
		SELECT a.id, a.section_id, a.title,
		       (SELECT sub.status FROM lms.assignment_submissions sub WHERE sub.activity_id = a.id AND sub.user_id = $1
		        ORDER BY sub.attempt_no DESC LIMIT 1),
		       `+assignmentColumns+`, `+timelineCourse+`
		JOIN mine m ON m.section_id = s.id
		JOIN lms.activities a ON a.section_id = s.id AND a.deleted_at IS NULL AND a.is_visible AND a.activity_type = 'ASSIGNMENT'
		JOIN lms.topics tp ON tp.id = a.topic_id AND tp.deleted_at IS NULL AND tp.is_visible
		JOIN lms.assignments asg ON asg.activity_id = a.id
		WHERE asg.due_at IS NOT NULL AND asg.due_at <= $3
		  AND s.status <> 'CANCELLED' AND o.status <> 'CANCELLED'
		  AND (a.available_from IS NULL OR a.available_from <= $4) AND (a.available_until IS NULL OR a.available_until > $4)
		  AND (tp.available_from IS NULL OR tp.available_from <= $4) AND (tp.available_until IS NULL OR tp.available_until > $4)`,
		userID, termID, now.Add(timelineHorizon), now)
	if err != nil {
		return nil, fmt.Errorf("lms: zaman çizelgesi okunamadı: %w", err)
	}
	defer rows.Close()
	var out []TimelineItem
	for rows.Next() {
		var (
			it     TimelineItem
			status *string
		)
		st, err := scanAssignmentRow(rows, []any{&it.ActivityID, &it.SectionID, &it.Title, &status},
			[]any{&it.SectionCode, &it.CourseID, &it.CourseCode, &it.CourseNameTR, &it.CourseNameEN})
		if err != nil {
			return nil, fmt.Errorf("lms: zaman çizelgesi okunamadı: %w", err)
		}
		if (status != nil && *status == StatusSubmitted) || st.StateAt(now) == SubmissionClosed {
			continue // teslim edildi ya da artık teslim edilemez
		}
		it.Kind, it.DueAt, it.CutoffAt = TimelineDue, st.DueAt, st.CutoffAt
		if st.LateAt(now) {
			it.Kind = TimelineOverdue
		}
		out = append(out, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("lms: zaman çizelgesi okunamadı: %w", err)
	}
	return out, nil
}

// gradingItems, öğretim elemanının dönemdeki şubelerinde puanlanmayı bekleyen teslimi olan
// ödevlerdir (son denemesi gönderilmiş, o deneme puanlanmamış, şubede kayıtlı öğrenciler).
func (s *Service) gradingItems(ctx context.Context, userID, termID string) ([]TimelineItem, error) {
	rows, err := s.pool.Query(ctx, `
		WITH mine AS (
			SELECT si.section_id FROM offering.section_instructors si
			JOIN people.staff st ON st.id = si.staff_id JOIN iam.users u ON u.person_id = st.person_id
			WHERE u.id = $1
		),
		items AS (
			SELECT a.id, a.section_id, a.title, asg.due_at, asg.cutoff_at,
			       (SELECT count(*) FROM (
			            SELECT DISTINCT ON (sub.user_id) sub.user_id, sub.attempt_no, sub.status
			            FROM lms.assignment_submissions sub WHERE sub.activity_id = a.id
			            ORDER BY sub.user_id, sub.attempt_no DESC) l
			        LEFT JOIN lms.assignment_grades g ON g.activity_id = a.id AND g.user_id = l.user_id
			        WHERE l.status = 'SUBMITTED'
			          AND (g.user_id IS NULL OR g.score IS NULL OR g.attempt_no IS NULL OR g.attempt_no < l.attempt_no)
			          AND EXISTS (SELECT 1 FROM enrollment.enrollments e
			                      JOIN enrollment.student_programs sp ON sp.id = e.student_program_id
			                      JOIN people.students ps ON ps.id = sp.student_id
			                      JOIN iam.users su ON su.person_id = ps.person_id
			                      WHERE e.section_id = a.section_id AND e.status <> 'DROPPED' AND su.id = l.user_id)) AS needs
			FROM mine m
			JOIN lms.activities a ON a.section_id = m.section_id AND a.deleted_at IS NULL AND a.activity_type = 'ASSIGNMENT'
			JOIN lms.assignments asg ON asg.activity_id = a.id
		)
		SELECT i.id, i.section_id, i.title, i.due_at, i.cutoff_at, i.needs, `+timelineCourse+`
		JOIN items i ON i.section_id = s.id
		WHERE o.term_id = $2 AND i.needs > 0 AND s.status <> 'CANCELLED' AND o.status <> 'CANCELLED'`, userID, termID)
	if err != nil {
		return nil, fmt.Errorf("lms: zaman çizelgesi okunamadı: %w", err)
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (TimelineItem, error) {
		it := TimelineItem{Kind: TimelineToGrade}
		err := row.Scan(&it.ActivityID, &it.SectionID, &it.Title, &it.DueAt, &it.CutoffAt, &it.NeedsGrading,
			&it.SectionCode, &it.CourseID, &it.CourseCode, &it.CourseNameTR, &it.CourseNameEN)
		return it, err
	})
	if err != nil {
		return nil, fmt.Errorf("lms: zaman çizelgesi okunamadı: %w", err)
	}
	return out, nil
}
