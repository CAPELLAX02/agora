package lms

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/grading"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// Student, şubenin bir öğrencisidir (bırakmamış kesin kaydı olan).
type Student struct {
	UserID    string
	StudentNo string
	FirstName string
	LastName  string
}

// RosterRow, teslim listesinde bir öğrencidir: son denemesi, puanı ve yorum sayısı.
type RosterRow struct {
	Student
	Latest   *Submission
	Grade    *Grade
	Comments int
}

// students, şubenin öğrencileridir (soyadına göre, Türkçe sıralama).
func (s *Service) students(ctx context.Context, q db.Querier, sectionID string) ([]Student, error) {
	rows, err := q.Query(ctx, `
		SELECT u.id, st.student_no, p.first_name, p.last_name
		FROM enrollment.enrollments e
		JOIN enrollment.student_programs sp ON sp.id = e.student_program_id
		JOIN people.students st ON st.id = sp.student_id
		JOIN people.persons p ON p.id = st.person_id
		JOIN iam.users u ON u.person_id = p.id
		WHERE e.section_id = $1 AND e.status <> 'DROPPED'
		ORDER BY p.last_name COLLATE "tr-x-icu", p.first_name COLLATE "tr-x-icu", st.student_no`, sectionID)
	if err != nil {
		return nil, fmt.Errorf("lms: öğrenciler okunamadı: %w", err)
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Student, error) {
		var st Student
		err := row.Scan(&st.UserID, &st.StudentNo, &st.FirstName, &st.LastName)
		return st, err
	})
	if err != nil {
		return nil, fmt.Errorf("lms: öğrenciler okunamadı: %w", err)
	}
	return list, nil
}

// Roster, ödevin teslim listesidir: şubenin bütün öğrencileri (teslim etmeyenler dahil).
func (s *Service) Roster(ctx context.Context, st AssignmentSettings, sectionID, activityID string) ([]RosterRow, error) {
	students, err := s.students(ctx, s.pool, sectionID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(students))
	out := make([]RosterRow, len(students))
	index := map[string]int{}
	for i, st := range students {
		ids[i], out[i].Student, index[st.UserID] = st.UserID, st, i
	}
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT ON (sub.user_id) sub.user_id, sub.id, sub.attempt_no, sub.status, sub.submitted_at, sub.is_late,
		       sub.version, sub.updated_at, (SELECT count(*) FROM lms.submission_files sf WHERE sf.submission_id = sub.id)
		FROM lms.assignment_submissions sub
		WHERE sub.activity_id = $1 AND sub.user_id = ANY($2)
		ORDER BY sub.user_id, sub.attempt_no DESC`, activityID, ids)
	if err != nil {
		return nil, fmt.Errorf("lms: teslimler okunamadı: %w", err)
	}
	var (
		userID string
		sub    Submission
	)
	_, err = pgx.ForEachRow(rows, []any{&userID, &sub.ID, &sub.AttemptNo, &sub.Status, &sub.SubmittedAt, &sub.Late,
		&sub.Version, &sub.UpdatedAt, &sub.FileCount}, func() error {
		latest := sub
		out[index[userID]].Latest = &latest
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("lms: teslimler okunamadı: %w", err)
	}
	grades, err := s.grades(ctx, st, activityID, ids)
	if err != nil {
		return nil, err
	}
	for id, g := range grades {
		out[index[id]].Grade = &g
	}
	rows, err = s.pool.Query(ctx, `
		SELECT student_id, count(*) FROM lms.submission_comments WHERE activity_id = $1 GROUP BY student_id`, activityID)
	if err != nil {
		return nil, fmt.Errorf("lms: yorumlar okunamadı: %w", err)
	}
	var n int
	_, err = pgx.ForEachRow(rows, []any{&userID, &n}, func() error {
		if i, ok := index[userID]; ok {
			out[i].Comments = n
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("lms: yorumlar okunamadı: %w", err)
	}
	return out, nil
}

// SectionStudent, kullanıcının şubenin öğrencisi olduğunu denetler ve bilgisini döndürür.
func (s *Service) SectionStudent(ctx context.Context, sectionID, userID string) (Student, error) {
	students, err := s.students(ctx, s.pool, sectionID)
	if err != nil {
		return Student{}, err
	}
	for _, st := range students {
		if st.UserID == userID {
			return st, nil
		}
	}
	return Student{}, ErrNotFound
}

// GradeInput, bir öğrencinin puanı (boşsa puansız) ve geri bildirimidir.
type GradeInput struct {
	Score    *grading.Points
	Feedback string
}

// SetGrade, öğrencinin puanını ve geri bildirimini yazar. Puan o anki son denemeye verilir.
// Hiç puan yoksa oluşturulur (version 0); varsa sürüm tutmalıdır.
func (s *Service) SetGrade(ctx context.Context, actorID, sectionID, activityID, studentID string, version int, in GradeInput) error {
	if len(in.Feedback) > 50_000 {
		return &ValidationError{Field: "feedback_html", Message: "Metin çok uzun."}
	}
	if _, err := s.SectionStudent(ctx, sectionID, studentID); err != nil {
		return err
	}
	released := false
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		st, err := lockAssignment(ctx, tx, activityID)
		if err != nil {
			return err
		}
		released = st.ReleasedAt != nil
		if in.Score != nil && (*in.Score < 0 || *in.Score > st.MaxScore) {
			return &ValidationError{Field: "score", Message: fmt.Sprintf("0 ile %s arasında olmalı.", formatPoints(st.MaxScore))}
		}
		var attempt *int16
		if err := tx.QueryRow(ctx, `SELECT max(attempt_no) FROM lms.assignment_submissions WHERE activity_id = $1 AND user_id = $2`,
			activityID, studentID).Scan(&attempt); err != nil {
			return fmt.Errorf("lms: teslim okunamadı: %w", err)
		}
		var score *int64
		if in.Score != nil {
			v := int64(*in.Score)
			score = &v
		}
		feedback := Sanitize(in.Feedback)
		if emptyHTML(feedback) {
			feedback = ""
		}
		var current int
		err = tx.QueryRow(ctx, `SELECT version FROM lms.assignment_grades WHERE activity_id = $1 AND user_id = $2 FOR UPDATE`,
			activityID, studentID).Scan(&current)
		switch {
		case errors.Is(err, pgx.ErrNoRows) && version != 0:
			return ErrVersionMismatch
		case errors.Is(err, pgx.ErrNoRows):
			tag, err := tx.Exec(ctx, `
				INSERT INTO lms.assignment_grades (activity_id, user_id, score, feedback_html, attempt_no, graded_by)
				VALUES ($1, $2, $3::numeric / 100, $4, $5, $6) ON CONFLICT DO NOTHING`,
				activityID, studentID, score, feedback, attempt, actorID)
			if err != nil {
				return fmt.Errorf("lms: puan yazılamadı: %w", err)
			}
			if tag.RowsAffected() == 0 {
				return ErrVersionMismatch
			}
		case err != nil:
			return fmt.Errorf("lms: puan okunamadı: %w", err)
		case version == 0:
			return ErrPreconditionRequired
		case version != current:
			return ErrVersionMismatch
		default:
			if _, err := tx.Exec(ctx, `
				UPDATE lms.assignment_grades SET score = $3::numeric / 100, feedback_html = $4, attempt_no = $5, graded_by = $6,
				       graded_at = now(), version = version + 1
				WHERE activity_id = $1 AND user_id = $2`, activityID, studentID, score, feedback, attempt, actorID); err != nil {
				return fmt.Errorf("lms: puan yazılamadı: %w", err)
			}
		}
		return audit.Record(ctx, tx, audit.Entry{ActorUserID: actorID, Action: "lms_grade.update", EntityType: "lms_activity",
			EntityID: activityID, After: map[string]any{"student_id": studentID, "score": score, "attempt_no": attempt}})
	})
	if err == nil {
		s.metrics.Graded("manual", 1)
	}
	// Yayımlanmış ödevin puanı değişince bağlı bileşenin notu da değişir.
	if err == nil && released {
		s.syncActivity(ctx, actorID, activityID)
	}
	return err
}

// ReleaseGrades, ödevin puanlarını öğrencilere yayımlar; sonraki puanlar hemen görünür.
func (s *Service) ReleaseGrades(ctx context.Context, actorID, activityID string) error {
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE lms.assignments SET grades_released_at = now() WHERE activity_id = $1 AND grades_released_at IS NULL`,
			activityID)
		if err != nil {
			return fmt.Errorf("lms: puanlar yayımlanamadı: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return &ConflictError{Code: "ALREADY_RELEASED", Detail: "Puanlar zaten yayımlandı."}
		}
		if err := notifyReleased(ctx, tx, actorID, activityID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorUserID: actorID, Action: "lms_assignment.release", EntityType: "lms_activity", EntityID: activityID})
	})
	if err == nil {
		s.syncActivity(ctx, actorID, activityID)
	}
	return err
}

// formatPoints, puanı Türkçe yazar (75,5).
func formatPoints(p grading.Points) string {
	whole, frac := int64(p)/100, int64(p)%100
	switch {
	case frac == 0:
		return fmt.Sprintf("%d", whole)
	case frac%10 == 0:
		return fmt.Sprintf("%d,%d", whole, frac/10)
	}
	return fmt.Sprintf("%d,%02d", whole, frac)
}
