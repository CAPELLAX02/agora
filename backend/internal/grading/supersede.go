package grading

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// supersedeTargets, bir kesin kayda harf notu verildiğinde yerini alacağı önceki denemelerdir
// (son not geçerli, yönetmelik md. 39):
//   - aynı dersin ya da eşdeğerinin önceki dönemlerdeki kapanmamış denemeleri (course_id yeni,
//     equivalent_course_id eski ders: yeni ders eskinin yerine her zaman, eski ders yeninin yerine
//     sadece çift yönlü eşdeğerlikte sayılır);
//   - kayıt kalemi kalınan seçmelinin yerine (ELECTIVE_REPLACEMENT) alınmışsa, aynı havuzdaki en
//     eski kalınmış ve kapanmamış deneme.
//
// İlan bu denemeleri kapatır; sanal transkript aynı kuralla hesaplar.
func supersedeTargets(ctx context.Context, q db.Querier, enrollmentID string) ([]string, error) {
	rows, err := q.Query(ctx, `
		WITH n AS (
			SELECT e.id, e.student_program_id, e.course_id, t.starts_on, ri.item_kind, ci.elective_group_id
			FROM enrollment.enrollments e
			JOIN academic.terms t ON t.id = e.term_id
			LEFT JOIN enrollment.registration_items ri ON ri.id = e.registration_item_id
			LEFT JOIN curriculum.curriculum_items ci ON ci.id = ri.curriculum_item_id
			WHERE e.id = $1
		), same AS (
			SELECT n.course_id AS id FROM n
			UNION SELECT ce.equivalent_course_id FROM n JOIN curriculum.course_equivalences ce ON ce.course_id = n.course_id
			UNION SELECT ce.course_id FROM n JOIN curriculum.course_equivalences ce
			      ON ce.equivalent_course_id = n.course_id AND ce.is_bidirectional
		), previous AS (
			SELECT o.id, o.course_id, ot.starts_on, o.letter_grade, o.grade_scale_id
			FROM n JOIN enrollment.enrollments o ON o.student_program_id = n.student_program_id
			JOIN academic.terms ot ON ot.id = o.term_id
			WHERE o.id <> n.id AND o.status = 'COMPLETED' AND o.superseded_by_id IS NULL AND ot.starts_on < n.starts_on
		)
		SELECT p.id FROM previous p WHERE p.course_id IN (SELECT id FROM same)
		UNION ALL
		(SELECT p.id FROM previous p, n
		 JOIN curriculum.grade_scale_items gi ON true
		 WHERE n.item_kind = 'ELECTIVE_REPLACEMENT' AND gi.grade_scale_id = p.grade_scale_id AND gi.letter = p.letter_grade
		   AND NOT gi.is_passing AND p.course_id NOT IN (SELECT id FROM same)
		   AND p.course_id IN (SELECT course_id FROM curriculum.elective_group_courses WHERE elective_group_id = n.elective_group_id)
		 ORDER BY p.starts_on, p.id
		 LIMIT 1)`, enrollmentID)
	if err != nil {
		return nil, fmt.Errorf("grading: önceki denemeler okunamadı: %w", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("grading: önceki denemeler okunamadı: %w", err)
	}
	return ids, nil
}
