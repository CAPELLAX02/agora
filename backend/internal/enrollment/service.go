package enrollment

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// Service, program kaydı ve danışman atama iş kurallarını uygular.
type Service struct {
	pool *pgxpool.Pool
}

// NewService, bir Service oluşturur.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool}
}

// NewStudentProgram, oluşturulacak program kaydıdır.
type NewStudentProgram struct {
	StudentID     string
	ProgramID     string
	Kind          Kind
	AdmissionType AdmissionType
	AdmissionYear int
	AdmittedOn    time.Time
	Status        Status // PREP ya da ACTIVE
	ClassLevel    int
}

// CreateStudentProgram, öğrenciyi bir programa kaydeder. Öğrenci zaten bu programa
// kayıtlıysa ErrConflict döner.
func (s *Service) CreateStudentProgram(ctx context.Context, actorID string, in NewStudentProgram) (string, error) {
	var id string
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM people.students WHERE id = $1)`, in.StudentID).Scan(&exists); err != nil {
			return fmt.Errorf("enrollment: öğrenci denetlenemedi: %w", err)
		}
		if !exists {
			return ErrNotFound
		}

		var active bool
		err := tx.QueryRow(ctx, `SELECT is_active FROM org.programs WHERE id = $1`, in.ProgramID).Scan(&active)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUnknownProgram
		}
		if err != nil {
			return fmt.Errorf("enrollment: program denetlenemedi: %w", err)
		}
		if !active {
			return ErrProgramInactive
		}

		err = tx.QueryRow(ctx, `
			INSERT INTO enrollment.student_programs
				(student_id, program_id, enrollment_kind, admission_type, admission_year, admitted_on, status, class_level)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING id`,
			in.StudentID, in.ProgramID, string(in.Kind), string(in.AdmissionType), in.AdmissionYear,
			in.AdmittedOn, string(in.Status), in.ClassLevel,
		).Scan(&id)
		if db.IsConflict(err) {
			return ErrConflict
		}
		if err != nil {
			return fmt.Errorf("enrollment: program kaydı oluşturulamadı: %w", err)
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO enrollment.student_program_status_history (student_program_id, to_status, reason, changed_by)
			VALUES ($1, $2, 'İlk kayıt', $3)`, id, string(in.Status), nullable(actorID)); err != nil {
			return fmt.Errorf("enrollment: durum geçmişi yazılamadı: %w", err)
		}

		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "student_program.create", EntityType: "student_program", EntityID: id,
			After: map[string]any{
				"student_id": in.StudentID, "program_id": in.ProgramID, "kind": in.Kind,
				"admission_type": in.AdmissionType, "admission_year": in.AdmissionYear, "status": in.Status,
			},
		})
	})
	return id, err
}

// AssignAdvisor, program kaydına danışman atar. Varsa önceki danışmanlık bu anda
// sona erer ve geçmişte kalır.
//
// Danışman, programın bölümünde aktif danışman rolü olan, hesabı aktif ve görevde
// olan bir öğretim elemanı olmalıdır. Mezun olmuş ya da ayrılmış bir kayda danışman
// atanmaz. Aynı kayda eşzamanlı iki atama, kaydın satır kilidiyle sıraya girer.
func (s *Service) AssignAdvisor(ctx context.Context, actorID, studentProgramID, staffID, reason string) error {
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var (
			status, departmentID string
		)
		err := tx.QueryRow(ctx, `
			SELECT sp.status, p.department_id FROM enrollment.student_programs sp
			JOIN org.programs p ON p.id = sp.program_id
			WHERE sp.id = $1
			FOR UPDATE OF sp`, studentProgramID).Scan(&status, &departmentID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("enrollment: program kaydı okunamadı: %w", err)
		}
		if !Status(status).Ongoing() {
			return ErrNotActive
		}

		r := NewRepository(tx)
		ok, err := r.isEligibleAdvisor(ctx, departmentID, staffID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrNotAnAdvisor
		}

		var current *string
		err = tx.QueryRow(ctx, `
			SELECT advisor_staff_id::text FROM enrollment.advisor_assignments
			WHERE student_program_id = $1 AND valid_until IS NULL`, studentProgramID).Scan(&current)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("enrollment: mevcut danışman okunamadı: %w", err)
		}
		if current != nil && *current == staffID {
			return nil // zaten bu danışman
		}

		// Bitiş ve başlangıç gerçek anla (clock_timestamp) yazılır: now() transaction'ın
		// başlangıç anıdır ve eski atamanın bitişi ile yenisinin başlangıcı aynı ana
		// düşüp dönemler üst üste binerdi.
		if _, err := tx.Exec(ctx, `
			UPDATE enrollment.advisor_assignments SET valid_until = clock_timestamp()
			WHERE student_program_id = $1 AND valid_until IS NULL`, studentProgramID); err != nil {
			return fmt.Errorf("enrollment: önceki danışmanlık sonlandırılamadı: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO enrollment.advisor_assignments (student_program_id, advisor_staff_id, valid_from, assigned_by, reason)
			VALUES ($1, $2, clock_timestamp(), $3, $4)`,
			studentProgramID, staffID, nullable(actorID), reason); err != nil {
			return fmt.Errorf("enrollment: danışman atanamadı: %w", err)
		}

		var before any
		if current != nil {
			before = map[string]any{"advisor_staff_id": *current}
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "advisor.assign", EntityType: "student_program", EntityID: studentProgramID,
			Before: before,
			After:  map[string]any{"advisor_staff_id": staffID, "reason": reason},
		})
	})
}

// Target, program kaydının yetki kapsamı hedefidir.
func (p ProgramRef) Target() authz.Target {
	return authz.Target{FacultyID: p.FacultyID, DepartmentID: p.DepartmentID, ProgramID: p.ID}
}

// CanReadStudent, kullanıcının öğrencinin bilgilerini görebilip göremeyeceğine karar
// verir (politika fonksiyonu). Öğrencinin kendisi, öğrencinin programlarından birini
// kapsayan person:read yetkisi olanlar ve öğrencinin şu anki danışmanı görebilir.
// Danışman rolünün yetkisi bölümü kapsamaz: erişim danışmanlık ilişkisinden gelir.
func (s *Service) CanReadStudent(ctx context.Context, perms *authz.Permissions, userID string, st Student) (bool, error) {
	if st.UserID != "" && st.UserID == userID {
		return true, nil
	}
	for _, p := range st.Programs {
		if perms.Allows("person:read", p.Program.Target()) {
			return true, nil
		}
	}
	if !perms.Has("person:read") {
		return false, nil
	}

	r := NewRepository(s.pool)
	staffID, err := r.StaffByUser(ctx, userID)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return r.IsCurrentAdvisor(ctx, staffID, st.ID)
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
