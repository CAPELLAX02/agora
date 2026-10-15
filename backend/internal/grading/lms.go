package grading

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// Ders alanının (LMS) beslediği bileşenler: bir ödev şubenin dönem içi bir bileşenine
// bağlanınca bileşenin notları bağlı ödevlerin yayımlanmış puanlarından hesaplanır ve buraya
// kaynak LMS olarak yazılır. Bu bileşenlere elle not girilmez; harf notları ilan edilince
// notlar donar (yazma reddedilir).

// lmsFed, şubenin ders alanındaki ödevlerin beslediği bileşenleridir.
func lmsFed(ctx context.Context, q db.Querier, sectionID string) (map[string]bool, error) {
	rows, err := q.Query(ctx, `
		SELECT DISTINCT l.component_id FROM lms.assignment_links l
		JOIN grading.assessment_components ac ON ac.id = l.component_id
		JOIN lms.activities a ON a.id = l.activity_id
		WHERE ac.section_id = $1 AND a.deleted_at IS NULL`, sectionID)
	if err != nil {
		return nil, fmt.Errorf("grading: bağlı bileşenler okunamadı: %w", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("grading: bağlı bileşenler okunamadı: %w", err)
	}
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

// ApplyLMSScores, bileşenin notlarını ders alanının hesapladığı puanlara eşitler (kaynak LMS):
// scores öğrenci kaydı → puan. Listede olmayan öğrencinin LMS kaynaklı notu silinir; elle
// girilmiş not listede yoksa korunur. Not girişi penceresi aranmaz; plan kilitli olmalı, harf
// notları ilan edilmemiş olmalı (ErrGradesPublished: notlar dondu). Değişen not sayısını döndürür.
func (s *Service) ApplyLMSScores(ctx context.Context, actorID, componentID string, scores map[string]Points) (int, error) {
	changed := 0
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var (
			sectionID string
			locked    *time.Time
			category  string
		)
		err := tx.QueryRow(ctx, `
			SELECT ac.section_id, s.assessment_plan_locked_at, t.category
			FROM grading.assessment_components ac
			JOIN offering.sections s ON s.id = ac.section_id
			JOIN grading.assessment_types t ON t.code = ac.type_code
			WHERE ac.id = $1`, componentID).Scan(&sectionID, &locked, &category)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("grading: bileşen okunamadı: %w", err)
		}
		if Category(category) != CategoryInTerm {
			return ErrNotFound // LMS sadece dönem içi bileşenleri besler
		}
		if err := lockSection(ctx, tx, sectionID, false); err != nil {
			return err
		}
		if locked == nil {
			return ErrPlanNotLocked
		}
		if _, err := tx.Exec(ctx, `SELECT 1 FROM grading.assessment_components WHERE id = $1 FOR UPDATE`, componentID); err != nil {
			return fmt.Errorf("grading: bileşen kilitlenemedi: %w", err)
		}
		rows, err := tx.Query(ctx, `
			SELECT e.id, sc.status, coalesce((sc.score * 100)::int8, 0), coalesce(sc.source, '')
			FROM enrollment.enrollments e
			LEFT JOIN grading.assessment_scores sc ON sc.enrollment_id = e.id AND sc.component_id = $2
			WHERE e.section_id = $1 AND e.status <> 'DROPPED'`, sectionID, componentID)
		if err != nil {
			return fmt.Errorf("grading: notlar okunamadı: %w", err)
		}
		type current struct {
			status *string
			points Points
			source string
		}
		existing := map[string]current{}
		var (
			id  string
			cur current
			pts int64
		)
		_, err = pgx.ForEachRow(rows, []any{&id, &cur.status, &pts, &cur.source}, func() error {
			cur.points = Points(pts)
			existing[id] = cur
			return nil
		})
		if err != nil {
			return fmt.Errorf("grading: notlar okunamadı: %w", err)
		}
		var before, after []map[string]any
		for enrollmentID, cur := range existing {
			want, ok := scores[enrollmentID]
			switch {
			case ok && (cur.status == nil || *cur.status != "SCORED" || cur.points != want || cur.source != string(sourceLMS)):
				if _, err := tx.Exec(ctx, `
					INSERT INTO grading.assessment_scores (component_id, enrollment_id, status, score, entered_by, source)
					VALUES ($1, $2, 'SCORED', $3::numeric / 100, $4, 'LMS')
					ON CONFLICT (component_id, enrollment_id) DO UPDATE
					SET status = 'SCORED', score = excluded.score, source = 'LMS', entered_by = excluded.entered_by, entered_at = now()`,
					componentID, enrollmentID, int64(want), nullable(actorID)); err != nil {
					return fmt.Errorf("grading: not yazılamadı: %w", err)
				}
			case !ok && cur.status != nil && cur.source == string(sourceLMS):
				if _, err := tx.Exec(ctx, `DELETE FROM grading.assessment_scores WHERE component_id = $1 AND enrollment_id = $2`,
					componentID, enrollmentID); err != nil {
					return fmt.Errorf("grading: not silinemedi: %w", err)
				}
			default:
				continue
			}
			changed++
			old := map[string]any{"enrollment_id": enrollmentID, "score": nil}
			if cur.status != nil && *cur.status == "SCORED" {
				old["score"] = cur.points.Float()
			}
			next := map[string]any{"enrollment_id": enrollmentID, "score": nil}
			if ok {
				next["score"] = want.Float()
			}
			before, after = append(before, old), append(after, next)
		}
		if changed == 0 {
			return nil
		}
		if _, err := tx.Exec(ctx, `UPDATE grading.assessment_components SET scores_version = scores_version + 1 WHERE id = $1`, componentID); err != nil {
			return fmt.Errorf("grading: not sürümü artırılamadı: %w", err)
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "assessment_scores.lms", EntityType: "assessment_component", EntityID: componentID,
			Before: map[string]any{"scores": before}, After: map[string]any{"scores": after},
		})
	})
	if err != nil {
		return 0, err
	}
	if changed > 0 {
		s.metrics.ScoresEntered("lms", changed)
	}
	return changed, nil
}
