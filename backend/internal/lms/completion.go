package lms

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Etkinlik tamamlama: öğrenci başına ve kendiliğinden (sayfa, klasör, forum ve bağlantı
// görüntülenince; dosya indirilince; ödev gönderilince). Etiket ilerlemeye girmez.

// Countable, etkinliğin ilerlemeye girdiğini söyler.
func (t ActivityType) Countable() bool { return t != TypeLabel }

// completesOnView, etkinliğin görüntülenince tamamlandığını söyler (ödev gönderilince, dosya
// indirilince tamamlanır).
func (t ActivityType) completesOnView() bool {
	return t == TypePage || t == TypeFolder || t == TypeForum || t == TypeURL
}

// MarkComplete, öğrencinin etkinliği tamamladığını kaydeder (tekrarı zararsız; ilk tamamlama sayılır).
func (s *Service) MarkComplete(ctx context.Context, userID, activityID string, typ ActivityType) error {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO lms.activity_completions (activity_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, activityID, userID)
	if err != nil {
		return fmt.Errorf("lms: tamamlama yazılamadı: %w", err)
	}
	if tag.RowsAffected() > 0 {
		s.metrics.Completed(strings.ToLower(string(typ)))
	}
	return nil
}

// completions, öğrencinin şubede tamamladığı etkinliklerdir.
func (s *Service) completions(ctx context.Context, userID, sectionID string) (map[string]bool, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT c.activity_id FROM lms.activity_completions c JOIN lms.activities a ON a.id = c.activity_id
		WHERE c.user_id = $1 AND a.section_id = $2`, userID, sectionID)
	if err != nil {
		return nil, fmt.Errorf("lms: tamamlamalar okunamadı: %w", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("lms: tamamlamalar okunamadı: %w", err)
	}
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

// Progress, ilerlemedir: görünür (etiket dışı) etkinlikler ve tamamlananlar.
type Progress struct {
	Completed int
	Total     int
}

// progressOf, öğrencinin gördüğü bölümlerden ilerlemeyi hesaplar.
func progressOf(topics []Topic) Progress {
	var p Progress
	for _, t := range topics {
		for _, a := range t.Activities {
			if a.Type.Countable() {
				p.Total++
				if a.Completed {
					p.Completed++
				}
			}
		}
	}
	return p
}
