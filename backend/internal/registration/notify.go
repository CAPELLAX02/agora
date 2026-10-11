package registration

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/mail"
	"github.com/CAPELLAX02/agora/backend/internal/platform/outbox"
)

// DecisionEvent, danışman kararı olaylarının (registration.approved, registration.rejected,
// registration.approval_revoked) verisidir.
type DecisionEvent struct {
	RegistrationID   string   `json:"registration_id"`
	StudentProgramID string   `json:"student_program_id"`
	TermID           string   `json:"term_id"`
	Status           Status   `json:"status"`
	Note             string   `json:"note"`
	Override         bool     `json:"override"`
	SectionIDs       []string `json:"section_ids"`
}

// Notifier, danışman kararlarını öğrenciye e-postayla bildiren outbox tüketicisidir.
// E-posta, "işlendi" kaydıyla aynı transaction'da kuyruğa alınır: olay yeniden dağıtılsa da
// öğrenci aynı bildirimi iki kez almaz.
type Notifier struct {
	WebBaseURL string // e-postadaki bağlantının kökü
}

// Name, tüketicinin kalıcı adıdır.
func (Notifier) Name() string { return "registration-notifier" }

// Handles, karar olaylarını işler.
func (Notifier) Handles(eventType string) bool {
	switch eventType {
	case "registration.approved", "registration.rejected", "registration.approval_revoked":
		return true
	}
	return false
}

// Handle, öğrencinin hesabı varsa karar e-postasını kuyruğa alır.
func (n Notifier) Handle(ctx context.Context, tx pgx.Tx, e outbox.Event) error {
	var ev DecisionEvent
	if err := e.Decode(&ev); err != nil {
		return fmt.Errorf("registration: olay verisi çözülemedi: %w", err)
	}
	var (
		email, first, term string
		total              float64
		courses            int
	)
	err := tx.QueryRow(ctx, `
		SELECT u.email, pe.first_name, t.code, r.total_ects::float8,
		       (SELECT count(*) FROM enrollment.registration_items i WHERE i.registration_id = r.id AND i.status = 'ACTIVE')
		FROM enrollment.registrations r
		JOIN academic.terms t ON t.id = r.term_id
		JOIN enrollment.student_programs sp ON sp.id = r.student_program_id
		JOIN people.students s ON s.id = sp.student_id
		JOIN people.persons pe ON pe.id = s.person_id
		JOIN iam.users u ON u.person_id = s.person_id AND u.status = 'ACTIVE'
		WHERE r.id = $1`, ev.RegistrationID).Scan(&email, &first, &term, &total, &courses)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // öğrencinin etkin hesabı yok: bildirilecek kimse yok
	}
	if err != nil {
		return fmt.Errorf("registration: bildirim alıcısı okunamadı: %w", err)
	}
	decision := map[string]string{
		"registration.approved": "APPROVED", "registration.rejected": "REJECTED", "registration.approval_revoked": "REVOKED",
	}[e.Type]
	_, err = mail.Enqueue(ctx, tx, mail.Email{
		To: email, Template: mail.TemplateRegistrationDecision,
		Data: map[string]any{
			"name": first, "term": term, "decision": decision, "note": ev.Note,
			"courses": courses, "total_ects": total, "link": n.WebBaseURL + "/ders-secimi",
		},
	})
	return err
}
