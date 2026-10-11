package enrollment

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/platform/outbox"
)

// SemesterAdvancer, dönem geçişinde aktif öğrencilerin yarıyılını ilerleten outbox
// tüketicisidir (term.made_current olayı). Güz ya da bahar dönemi aktif yapılınca o
// döneme kadar kaydolmuş aktif öğrencilerin yarıyılı bir artar, sınıfı yarıyıla göre
// (programın süresiyle sınırlı) güncellenir. Hazırlık, dondurulmuş ve uzaklaştırılmış
// kayıtlar ilerlemez: dondurulan dönem süreye sayılmaz. Bir dönem için ilerletme bir kez
// yapılır; daha önceki bir dönem yeniden aktif yapılırsa yarıyıllar geri alınmaz ya da
// yeniden ilerletilmez.
type SemesterAdvancer struct{}

// Name, tüketicinin kalıcı adıdır.
func (SemesterAdvancer) Name() string { return "semester-advancer" }

// Handles, dönem değişikliği olayını işler.
func (SemesterAdvancer) Handles(eventType string) bool { return eventType == "term.made_current" }

// Handle, yarıyılları ilerletir.
func (SemesterAdvancer) Handle(ctx context.Context, tx pgx.Tx, e outbox.Event) error {
	var p struct {
		TermID   string `json:"term_id"`
		TermType string `json:"term_type"`
	}
	if err := e.Decode(&p); err != nil {
		return fmt.Errorf("enrollment: olay verisi çözülemedi: %w", err)
	}
	if p.TermType != "FALL" && p.TermType != "SPRING" {
		return nil // yaz okulu yarıyıl saymaz
	}
	// Aynı ya da daha sonraki bir dönem için ilerletme yapıldıysa bir şey yapılmaz.
	var done bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM enrollment.semester_advances a
			JOIN academic.terms t ON t.id = a.term_id
			WHERE t.starts_on >= (SELECT starts_on FROM academic.terms WHERE id = $1))`, p.TermID).Scan(&done); err != nil {
		return fmt.Errorf("enrollment: yarıyıl ilerletmesi denetlenemedi: %w", err)
	}
	if done {
		return nil
	}
	// Güz Y döneminde Y'den önce, bahar Y döneminde Y ve öncesinde kaydolanlar ilerler: güz Y'de
	// kaydolan öğrenci o güz 1. yarıyıldadır, baharda 2. yarıyıla geçer.
	tag, err := tx.Exec(ctx, `
		UPDATE enrollment.student_programs sp
		SET current_semester = sp.current_semester + 1,
		    class_level = least((sp.current_semester + 2) / 2, p.duration_semesters / 2),
		    version = sp.version + 1, updated_at = now()
		FROM org.programs p, academic.terms t, academic.academic_years ay
		WHERE p.id = sp.program_id AND t.id = $1 AND ay.id = t.academic_year_id
		  AND sp.status = 'ACTIVE' AND sp.class_level > 0 AND sp.current_semester < 20
		  AND (sp.admission_year < ay.start_year OR (t.term_type = 'SPRING' AND sp.admission_year = ay.start_year))`,
		p.TermID)
	if err != nil {
		return fmt.Errorf("enrollment: yarıyıllar ilerletilemedi: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO enrollment.semester_advances (term_id, advanced) VALUES ($1, $2)`,
		p.TermID, tag.RowsAffected()); err != nil {
		return fmt.Errorf("enrollment: yarıyıl ilerletmesi kaydedilemedi: %w", err)
	}
	return nil
}
