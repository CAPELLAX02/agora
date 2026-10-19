package attendance

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// Entry, yoklama listesinin bir satırıdır: şubede kesin kaydı olan öğrenci ve oturumdaki durumu
// (yoklama alınmamışsa boş).
type Entry struct {
	EnrollmentID, StudentNo, FirstName, LastName string
	Status                                       string
}

// Roll, oturumun yoklama listesidir: şubede bırakmamış kesin kaydı olan öğrenciler, numara sırasıyla.
func (s *Service) Roll(ctx context.Context, sessionID string) (Session, []Entry, error) {
	x, err := s.Session(ctx, s.pool, sessionID)
	if err != nil {
		return Session{}, nil, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT e.id, st.student_no, p.first_name, p.last_name, coalesce(r.status, '')
		FROM enrollment.enrollments e
		JOIN enrollment.student_programs sp ON sp.id = e.student_program_id
		JOIN people.students st ON st.id = sp.student_id
		JOIN people.persons p ON p.id = st.person_id
		LEFT JOIN attendance.attendance_records r ON r.term_id = $2 AND r.session_id = $3 AND r.enrollment_id = e.id
		WHERE e.section_id = $1 AND e.status <> 'DROPPED'
		ORDER BY st.student_no`, x.SectionID, x.TermID, x.ID)
	if err != nil {
		return Session{}, nil, fmt.Errorf("attendance: yoklama listesi okunamadı: %w", err)
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Entry, error) {
		var e Entry
		return e, row.Scan(&e.EnrollmentID, &e.StudentNo, &e.FirstName, &e.LastName, &e.Status)
	})
	if err != nil {
		return Session{}, nil, fmt.Errorf("attendance: yoklama listesi okunamadı: %w", err)
	}
	return x, list, nil
}

// earlyTake, oturum başlamadan ne kadar önce yoklama alınabileceğidir.
const earlyTake = 30 * time.Minute

// Take, oturumun yoklamasını yazar (iyimser kilitle): statuses kesin kayıt başına durumdur,
// listede olmayan öğrenciler gelmiş sayılır. Oturum yoklaması alınmış (HELD) olur; tekrar
// yazmak düzeltmedir. İptal edilmiş ve henüz başlamamış oturumun yoklaması alınmaz.
func (s *Service) Take(ctx context.Context, actorID, sessionID string, version int, statuses map[string]string) error {
	for id, st := range statuses {
		if st != Present && st != Absent && st != Late && st != Excused {
			return &ValidationError{"items.status", fmt.Sprintf("%s: PRESENT, ABSENT, LATE ya da EXCUSED olmalı.", id)}
		}
	}
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var (
			sectionID, termID, status string
			current                   int
			early                     bool
		)
		err := tx.QueryRow(ctx, `
			SELECT section_id, term_id, status, version, starts_at - $2::interval > now()
			FROM attendance.class_sessions WHERE id = $1 FOR UPDATE`,
			sessionID, fmt.Sprintf("%d seconds", int(earlyTake.Seconds()))).Scan(&sectionID, &termID, &status, &current, &early)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("attendance: oturum okunamadı: %w", err)
		}
		switch {
		case current != version:
			return ErrVersionMismatch
		case status == StatusCancelled:
			return ErrCancelled
		case early:
			return &ValidationError{"session", "Oturum henüz başlamadı; yoklama en erken 30 dakika önce alınır."}
		}
		rows, err := tx.Query(ctx, `SELECT id::text FROM enrollment.enrollments WHERE section_id = $1 AND status <> 'DROPPED'`, sectionID)
		if err != nil {
			return fmt.Errorf("attendance: öğrenciler okunamadı: %w", err)
		}
		roster, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return fmt.Errorf("attendance: öğrenciler okunamadı: %w", err)
		}
		enrolled := make(map[string]bool, len(roster))
		for _, id := range roster {
			enrolled[id] = true
		}
		for id := range statuses {
			if !enrolled[id] {
				return &ValidationError{"items.enrollment_id", "Şubede kesin kaydı olmayan öğrenci: " + id}
			}
		}
		ids := make([]string, 0, len(roster))
		values := make([]string, 0, len(roster))
		absent := 0
		for _, id := range roster {
			st, ok := statuses[id]
			if !ok {
				st = Present
			}
			if st == Absent {
				absent++
			}
			ids, values = append(ids, id), append(values, st)
		}
		if _, err := tx.Exec(ctx, `SELECT attendance.ensure_term_partition($1)`, termID); err != nil {
			return fmt.Errorf("attendance: dönem partition'ı açılamadı: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO attendance.attendance_records (term_id, session_id, enrollment_id, status, recorded_by)
			SELECT $1, $2, e, s, $5 FROM unnest($3::uuid[], $4::text[]) AS x(e, s)
			ON CONFLICT (term_id, session_id, enrollment_id) DO UPDATE
			SET status = excluded.status, recorded_by = excluded.recorded_by, recorded_at = now()
			WHERE attendance_records.status <> excluded.status`,
			termID, sessionID, ids, values, actorID); err != nil {
			return fmt.Errorf("attendance: yoklama yazılamadı: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE attendance.class_sessions SET status = 'HELD', taken_at = now(), taken_by = $2, version = version + 1, updated_at = now()
			WHERE id = $1`, sessionID, actorID); err != nil {
			return fmt.Errorf("attendance: oturum güncellenemedi: %w", err)
		}
		return audit.Record(ctx, tx, audit.Entry{ActorUserID: actorID, Action: "attendance.take", EntityType: "class_session",
			EntityID: sessionID, After: map[string]any{"students": len(roster), "absent": absent, "correction": status == StatusHeld}})
	})
}
