package registration

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/enrollment"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// ExpireDrafts, ders seçme penceresi bitmiş programlarda danışmana gönderilmemiş (taslak
// ya da reddedilmiş) sepetlerin ayırdığı kontenjanı serbest bırakır: sepet boşaltılır ve
// olay günlüğüne EXPIRED yazılır. Pencere bitmiş sayılır: açık değil, sırada yeni bir olay
// yok ve en az bir olay geçmişte kalmış. Kayda tanınan ek süre sürüyorsa dokunulmaz.
// Worker düzenli aralıklarla çalıştırır; tekrar çalıştırılabilir. Boşaltılan sepet sayısını
// döndürür.
func (s *Service) ExpireDrafts(ctx context.Context) (int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT r.id, r.term_id, sp.id, sp.program_id, d.faculty_id
		FROM enrollment.registrations r
		JOIN enrollment.student_programs sp ON sp.id = r.student_program_id
		JOIN org.programs p ON p.id = sp.program_id
		JOIN org.departments d ON d.id = p.department_id
		WHERE r.status IN ('DRAFT', 'REJECTED')
		  AND (r.extension_until IS NULL OR r.extension_until <= $1)
		  AND EXISTS (SELECT 1 FROM enrollment.registration_items i WHERE i.registration_id = r.id AND i.status = 'ACTIVE')`,
		s.now())
	if err != nil {
		return 0, fmt.Errorf("registration: taslaklar okunamadı: %w", err)
	}
	type candidate struct{ RegistrationID, TermID, StudentProgramID, ProgramID, FacultyID string }
	list, err := pgx.CollectRows(rows, pgx.RowToStructByPos[candidate])
	if err != nil {
		return 0, fmt.Errorf("registration: taslaklar okunamadı: %w", err)
	}

	type key struct{ term, program string }
	over := map[key]bool{}
	expired := 0
	for _, c := range list {
		k := key{c.TermID, c.ProgramID}
		ended, known := over[k]
		if !known {
			sp := enrollment.StudentProgram{Program: enrollment.ProgramRef{ID: c.ProgramID, FacultyID: c.FacultyID}}
			w, err := s.calendarWindow(ctx, sp, c.TermID, WindowRegistration)
			if err != nil {
				return expired, err
			}
			ended = !w.Open && w.Next == nil && len(w.Events) > 0
			over[k] = ended
		}
		if !ended {
			continue
		}
		ok, err := s.expire(ctx, c.StudentProgramID, c.RegistrationID)
		if err != nil {
			return expired, err
		}
		if ok {
			expired++
		}
	}
	return expired, nil
}

// expire, tek bir sepeti boşaltır. Kayıt bu arada gönderilmiş ya da ek süre almışsa
// dokunmaz (false döner).
func (s *Service) expire(ctx context.Context, studentProgramID, registrationID string) (bool, error) {
	done := false
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := lockStudent(ctx, tx, studentProgramID); err != nil {
			return err
		}
		var (
			status string
			ext    *time.Time
		)
		if err := tx.QueryRow(ctx, `SELECT status, extension_until FROM enrollment.registrations WHERE id = $1 FOR UPDATE`,
			registrationID).Scan(&status, &ext); err != nil {
			return fmt.Errorf("registration: kayıt okunamadı: %w", err)
		}
		if !Status(status).Editable() || (ext != nil && s.now().Before(*ext)) {
			return nil
		}
		rows, err := tx.Query(ctx, `
			DELETE FROM enrollment.registration_items WHERE registration_id = $1 AND status = 'ACTIVE'
			RETURNING section_id, coalesce(quota_program_id::text, '')`, registrationID)
		if err != nil {
			return fmt.Errorf("registration: sepet boşaltılamadı: %w", err)
		}
		type freed struct{ Section, Quota string }
		items, err := pgx.CollectRows(rows, pgx.RowToStructByPos[freed])
		if err != nil {
			return fmt.Errorf("registration: sepet boşaltılamadı: %w", err)
		}
		for _, it := range items {
			if err := release(ctx, tx, it.Section, it.Quota); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `
			UPDATE enrollment.registrations SET total_ects = 0, version = version + 1, updated_at = now() WHERE id = $1`,
			registrationID); err != nil {
			return fmt.Errorf("registration: kayıt güncellenemedi: %w", err)
		}
		done = len(items) > 0
		return event(ctx, tx, registrationID, "EXPIRED", "", map[string]any{"released": len(items)})
	})
	return done, err
}
