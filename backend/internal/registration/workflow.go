package registration

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/enrollment"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
	"github.com/CAPELLAX02/agora/backend/internal/platform/outbox"
	"github.com/CAPELLAX02/agora/backend/internal/registration/rules"
)

// Submit, sepeti danışman onayına gönderir: ders seçme penceresi açık olmalı ve sepette
// kural hatası olmamalı (gönderim anı kuralları: 1. sınıf zorunlu dersleri, alttan ders
// önceliği). Gönderim anındaki AKTS sınırı ve GANO kayda yazılır.
func (s *Service) Submit(ctx context.Context, actorID string, sp enrollment.StudentProgram, termID string, version int) error {
	cw, err := s.calendarWindow(ctx, sp, termID, WindowRegistration)
	if err != nil {
		return err
	}
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := lockStudent(ctx, tx, sp.ID); err != nil {
			return err
		}
		reg, ok, err := lockedRegistration(ctx, tx, sp.ID, termID)
		if err != nil {
			return err
		}
		if !ok {
			return &RuleError{Violations: []rules.Violation{{Code: rules.CodeEmptyBasket, Severity: rules.Error, Message: "Sepetiniz boş."}}}
		}
		if reg.Version != version {
			return ErrVersionMismatch
		}
		if reg.Status != StatusDraft && reg.Status != StatusRejected {
			return ErrInvalidState
		}
		if !s.withExtension(cw, reg.ExtensionUntil).Usable() {
			return ErrWindowClosed
		}
		items, err := NewRepository(tx).items(ctx, reg.ID, sp.Program.ID)
		if err != nil {
			return err
		}
		b, err := s.load(ctx, tx, sp, termID, items)
		if err != nil {
			return err
		}
		ev := rules.Evaluate(b.input)
		if ev.Blocking() {
			return &RuleError{Violations: ev.Violations}
		}
		if _, err := tx.Exec(ctx, `
			UPDATE enrollment.registrations SET status = 'SUBMITTED', submitted_at = now(),
				max_ects_allowed = $2, gpa_snapshot = $3, decided_at = NULL, decided_by = NULL, decision_note = NULL,
				version = version + 1, updated_at = now()
			WHERE id = $1`, reg.ID, ev.ECTSLimit, sp.GPA); err != nil {
			return fmt.Errorf("registration: kayıt gönderilemedi: %w", err)
		}
		if err := event(ctx, tx, reg.ID, "SUBMITTED", actorID, map[string]any{
			"total_ects": ev.TotalECTS, "ects_limit": ev.ECTSLimit, "warnings": len(ev.Violations),
		}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "registration.submit", EntityType: "registration", EntityID: reg.ID,
			After: map[string]any{"status": StatusSubmitted, "total_ects": ev.TotalECTS, "items": len(items)},
		})
	})
	if err == nil && s.metrics != nil {
		s.metrics.Submitted()
	}
	return err
}

// Withdraw, danışman henüz karar vermemişse gönderimi geri çeker: sepet yeniden taslak olur.
func (s *Service) Withdraw(ctx context.Context, actorID string, sp enrollment.StudentProgram, termID string, version int) error {
	cw, err := s.calendarWindow(ctx, sp, termID, WindowRegistration)
	if err != nil {
		return err
	}
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := lockStudent(ctx, tx, sp.ID); err != nil {
			return err
		}
		reg, ok, err := lockedRegistration(ctx, tx, sp.ID, termID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrNotFound
		}
		if reg.Version != version {
			return ErrVersionMismatch
		}
		if reg.Status != StatusSubmitted {
			return ErrInvalidState
		}
		if !s.withExtension(cw, reg.ExtensionUntil).Usable() {
			return ErrWindowClosed
		}
		if _, err := tx.Exec(ctx, `
			UPDATE enrollment.registrations SET status = 'DRAFT', version = version + 1, updated_at = now()
			WHERE id = $1`, reg.ID); err != nil {
			return fmt.Errorf("registration: gönderim geri çekilemedi: %w", err)
		}
		return event(ctx, tx, reg.ID, "WITHDRAWN", actorID, nil)
	})
}

// Decider, danışman kararını verendir. Override, öğrenci işlerinin müdahalesidir: pencere
// dışında da karar verebilir, gerekçe zorunludur ve olay günlüğünde işaretlenir.
type Decider struct {
	UserID   string
	Override bool
}

// Decision, danışmanın bir kayıt hakkındaki kararıdır.
type Decision string

// Kararlar.
const (
	Approve Decision = "APPROVE"
	Reject  Decision = "REJECT"
	Revoke  Decision = "REVOKE" // onayı geri al
)

// Decide, danışman kararını uygular. Onay ve red gönderilmiş kayıtta, onayı geri alma
// onaylanmış kayıtta yapılır. Red ve geri almada gerekçe (not) zorunludur. Onayda sepet
// kurallara göre yeniden değerlendirilir (gönderimden sonra engel konmuş olabilir) ve
// her ders için kesin kayıt (enrollment) oluşur; onay geri alınınca bu kayıtlar silinir.
func (s *Service) Decide(ctx context.Context, d Decider, sp enrollment.StudentProgram, registrationID string, version int, decision Decision, note string) error {
	if (decision != Approve || d.Override) && note == "" {
		return ErrReasonRequired
	}
	var regTerm string
	err := s.pool.QueryRow(ctx, `SELECT term_id FROM enrollment.registrations WHERE id = $1 AND student_program_id = $2`,
		registrationID, sp.ID).Scan(&regTerm)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("registration: kayıt okunamadı: %w", err)
	}
	cw, err := s.calendarWindow(ctx, sp, regTerm, WindowApproval)
	if err != nil {
		return err
	}
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := lockStudent(ctx, tx, sp.ID); err != nil {
			return err
		}
		var (
			termID, status string
			current        int
			ext            *time.Time
		)
		err := tx.QueryRow(ctx, `
			SELECT term_id, status, version, extension_until FROM enrollment.registrations
			WHERE id = $1 AND student_program_id = $2 FOR UPDATE`, registrationID, sp.ID,
		).Scan(&termID, &status, &current, &ext)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("registration: kayıt okunamadı: %w", err)
		}
		if current != version {
			return ErrVersionMismatch
		}
		want := StatusSubmitted
		if decision == Revoke {
			want = StatusApproved
		}
		if Status(status) != want {
			return ErrInvalidState
		}
		if !d.Override && !s.withExtension(cw, ext).Usable() {
			return ErrWindowClosed
		}

		repo := NewRepository(tx)
		items, err := repo.items(ctx, registrationID, sp.Program.ID)
		if err != nil {
			return err
		}
		next, eventType := StatusRejected, "REJECTED"
		switch decision {
		case Approve:
			b, err := s.load(ctx, tx, sp, termID, items)
			if err != nil {
				return err
			}
			if ev := rules.Evaluate(b.input); ev.Blocking() {
				return &RuleError{Violations: ev.Violations}
			}
			if err := createEnrollments(ctx, tx, sp, termID, items, b, "REGISTRATION"); err != nil {
				return err
			}
			next, eventType = StatusApproved, "APPROVED"
		case Revoke:
			// Not girilmiş kesin kayıt silinmez: önce notlar (ya da ders) ele alınmalı.
			var graded bool
			if err := tx.QueryRow(ctx, `
				SELECT EXISTS (
					SELECT 1 FROM grading.assessment_scores sc
					JOIN enrollment.enrollments e ON e.id = sc.enrollment_id
					JOIN enrollment.registration_items i ON i.id = e.registration_item_id
					WHERE i.registration_id = $1)`, registrationID).Scan(&graded); err != nil {
				return fmt.Errorf("registration: notlar okunamadı: %w", err)
			}
			if graded {
				return ErrGradesEntered
			}
			if _, err := tx.Exec(ctx, `
				DELETE FROM enrollment.enrollments WHERE status = 'ENROLLED' AND registration_item_id IN (
					SELECT id FROM enrollment.registration_items WHERE registration_id = $1)`, registrationID); err != nil {
				return fmt.Errorf("registration: kesin kayıtlar silinemedi: %w", err)
			}
			eventType = "APPROVAL_REVOKED"
		}

		if _, err := tx.Exec(ctx, `
			UPDATE enrollment.registrations SET status = $2, decided_at = now(), decided_by = $3, decision_note = $4,
				version = version + 1, updated_at = now()
			WHERE id = $1`, registrationID, string(next), nullable(d.UserID), nullable(note)); err != nil {
			return fmt.Errorf("registration: karar kaydedilemedi: %w", err)
		}
		payload := map[string]any{"note": note, "items": len(items)}
		if d.Override {
			payload["override"] = true
		}
		if err := event(ctx, tx, registrationID, eventType, d.UserID, payload); err != nil {
			return err
		}
		// Karar olayı diğer modüller içindir (ör. analitik); bildirim olay günlüğüyle yazıldı.
		sections := make([]string, len(items))
		for i, it := range items {
			sections[i] = it.Section.ID
		}
		if err := outbox.Publish(ctx, tx, "registration."+strings.ToLower(eventType), "registration", registrationID, DecisionEvent{
			RegistrationID: registrationID, StudentProgramID: sp.ID, TermID: termID, Status: next,
			Note: note, Override: d.Override, SectionIDs: sections,
		}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: d.UserID, Action: "registration." + decisionAction[decision],
			EntityType: "registration", EntityID: registrationID,
			Before: map[string]any{"status": status},
			After:  map[string]any{"status": next, "note": note, "override": d.Override},
		})
	})
}

var decisionAction = map[Decision]string{Approve: "approve", Reject: "reject", Revoke: "revoke"}

// createEnrollments, onaylanan sepetin (ya da ekle-bırakta eklenen) her dersi için kesin
// kayıt oluşturur. Deneme numarası dersin (ve onun yerine sayılan derslerin) önceki
// alınışlarından hesaplanır. source: REGISTRATION ya da ADD_DROP.
func createEnrollments(ctx context.Context, tx pgx.Tx, sp enrollment.StudentProgram, termID string, items []Item, b *bundle, source string) error {
	var scaleID string
	if err := tx.QueryRow(ctx, `SELECT id FROM curriculum.grade_scales WHERE is_default`).Scan(&scaleID); err != nil {
		return fmt.Errorf("registration: varsayılan not ölçeği okunamadı: %w", err)
	}
	for _, it := range items {
		related := b.input.Catalog.Satisfiers(it.Section.CourseID)
		if line, _, ok := rules.Classify(b.input, it.Section.CourseID); ok && !line.IsSlot() {
			related = append(related, b.input.Catalog.Satisfiers(line.CourseID)...)
		}
		attempt := 1
		for _, h := range b.history {
			if slices.Contains(related, h.CourseID) {
				attempt++
			}
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO enrollment.enrollments (student_program_id, section_id, term_id, course_id, attempt_no,
				grade_scale_id, ects_snapshot, credit_snapshot, source, registration_item_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			sp.ID, it.Section.ID, termID, it.Section.CourseID, min(attempt, 20), scaleID, it.ECTS, it.Section.Credit,
			source, it.ID); err != nil {
			return fmt.Errorf("registration: kesin kayıt oluşturulamadı: %w", err)
		}
	}
	return nil
}

// GrantExtension, öğrenci işlerinin kayda özel ek süre tanımasıdır: pencere kapalıyken
// öğrenci until anına kadar sepetini düzenleyip gönderebilir, danışman da onaylayabilir.
// Kayıt yoksa taslak olarak açılır.
func (s *Service) GrantExtension(ctx context.Context, actorID string, sp enrollment.StudentProgram, termID string, until time.Time, reason string) error {
	if reason == "" {
		return ErrReasonRequired
	}
	if !until.After(s.now()) || until.After(s.now().AddDate(0, 0, 30)) {
		return ErrExtensionInvalid
	}
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := lockStudent(ctx, tx, sp.ID); err != nil {
			return err
		}
		reg, ok, err := lockedRegistration(ctx, tx, sp.ID, termID)
		if err != nil {
			return err
		}
		if !ok {
			if err := ensureEditable(ctx, tx, &reg, actorID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `
			UPDATE enrollment.registrations SET extension_until = $2, extension_reason = $3, extension_by = $4,
				version = version + 1, updated_at = now()
			WHERE id = $1`, reg.ID, until, reason, nullable(actorID)); err != nil {
			return fmt.Errorf("registration: ek süre tanınamadı: %w", err)
		}
		if err := event(ctx, tx, reg.ID, "EXTENSION_GRANTED", actorID, map[string]any{"until": until, "reason": reason}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "registration.extension", EntityType: "registration", EntityID: reg.ID,
			After: map[string]any{"until": until, "reason": reason, "override": true},
		})
	})
}
