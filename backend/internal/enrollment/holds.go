package enrollment

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// --- Kayıt engelleri: okuma ------------------------------------------------------

const holdSelect = `
	SELECT h.id, sp.id, s.id, s.student_no, pe.first_name, pe.last_name,
	       p.id, p.code, p.name_tr, d.id, d.faculty_id,
	       h.hold_type, h.blocks, h.reason,
	       coalesce(pb.first_name || ' ' || pb.last_name, ''), h.placed_at,
	       coalesce(rb.first_name || ' ' || rb.last_name, ''), h.released_at, coalesce(h.release_reason, '')
	FROM enrollment.holds h
	JOIN enrollment.student_programs sp ON sp.id = h.student_program_id
	JOIN people.students s  ON s.id = sp.student_id
	JOIN people.persons pe  ON pe.id = s.person_id
	JOIN org.programs p     ON p.id = sp.program_id
	JOIN org.departments d  ON d.id = p.department_id
	LEFT JOIN iam.users pu      ON pu.id = h.placed_by
	LEFT JOIN people.persons pb ON pb.id = pu.person_id
	LEFT JOIN iam.users ru      ON ru.id = h.released_by
	LEFT JOIN people.persons rb ON rb.id = ru.person_id`

func scanHold(row pgx.CollectableRow) (Hold, error) {
	var (
		h            Hold
		typ          string
		blocks       []string
		placedAt     time.Time
		releasedAt   *time.Time
		placedBy, rb string
	)
	err := row.Scan(&h.ID, &h.StudentProgramID, &h.StudentID, &h.StudentNo, &h.FirstName, &h.LastName,
		&h.Program.ID, &h.Program.Code, &h.Program.NameTR, &h.Program.DepartmentID, &h.Program.FacultyID,
		&typ, &blocks, &h.Reason, &placedBy, &placedAt, &rb, &releasedAt, &h.ReleaseReason)
	if err != nil {
		return Hold{}, err
	}
	h.Type, h.PlacedBy, h.PlacedAt, h.ReleasedBy, h.ReleasedAt = HoldType(typ), placedBy, placedAt, rb, releasedAt
	for _, b := range blocks {
		h.Blocks = append(h.Blocks, HoldBlock(b))
	}
	return h, nil
}

// Holds, program kaydının engellerini döndürür: aktifler önce, sonra kaldırılmış olanlar
// (en yeni önce).
func (r *Repository) Holds(ctx context.Context, studentProgramID string) ([]Hold, error) {
	rows, err := r.db.Query(ctx, holdSelect+`
		WHERE h.student_program_id = $1
		ORDER BY h.released_at IS NOT NULL, h.placed_at DESC, h.id DESC`, studentProgramID)
	if err != nil {
		return nil, fmt.Errorf("enrollment: engeller okunamadı: %w", err)
	}
	list, err := pgx.CollectRows(rows, scanHold)
	if err != nil {
		return nil, fmt.Errorf("enrollment: engeller okunamadı: %w", err)
	}
	return list, nil
}

// Hold, engeli döndürür. Yoksa ErrNotFound döner.
func (r *Repository) Hold(ctx context.Context, id string) (Hold, error) {
	rows, err := r.db.Query(ctx, holdSelect+"\n\tWHERE h.id = $1", id)
	if err != nil {
		return Hold{}, fmt.Errorf("enrollment: engel okunamadı: %w", err)
	}
	h, err := pgx.CollectExactlyOneRow(rows, scanHold)
	if errors.Is(err, pgx.ErrNoRows) {
		return Hold{}, ErrNotFound
	}
	if err != nil {
		return Hold{}, fmt.Errorf("enrollment: engel okunamadı: %w", err)
	}
	return h, nil
}

// HoldCursor, engel listesinde bir sonraki sayfanın başlangıcıdır.
type HoldCursor struct {
	PlacedAt time.Time `json:"t"`
	ID       string    `json:"i"`
}

// HoldFilter, engel listesinin filtreleridir.
type HoldFilter struct {
	Scope        authz.ScopeSet // sadece bu kapsamdaki programların öğrencileri
	FacultyID    string
	DepartmentID string
	ProgramID    string
	Type         HoldType
	Active       bool // true: sadece aktifler, false: sadece kaldırılmışlar
	Query        string
	After        *HoldCursor
	Limit        int
}

// ListHolds, filtreye ve kapsama uyan engelleri en yeniden eskiye döndürür. Kapsam
// boşsa hiçbir kayıt dönmez.
func (r *Repository) ListHolds(ctx context.Context, f HoldFilter) (items []Hold, hasMore bool, err error) {
	if f.Scope.Empty() {
		return []Hold{}, false, nil
	}

	var w db.Where
	if !f.Scope.University {
		w.Add("(d.faculty_id = ANY($1::uuid[]) OR d.id = ANY($2::uuid[]) OR p.id = ANY($3::uuid[]))",
			f.Scope.FacultyIDs, f.Scope.DepartmentIDs, f.Scope.ProgramIDs)
	}
	if f.FacultyID != "" {
		w.Add("d.faculty_id = $1", f.FacultyID)
	}
	if f.DepartmentID != "" {
		w.Add("d.id = $1", f.DepartmentID)
	}
	if f.ProgramID != "" {
		w.Add("p.id = $1", f.ProgramID)
	}
	if f.Type != "" {
		w.Add("h.hold_type = $1", string(f.Type))
	}
	if f.Active {
		w.Add("h.released_at IS NULL")
	} else {
		w.Add("h.released_at IS NOT NULL")
	}
	if f.Query != "" {
		w.Add(`s.id IN (
			SELECT id FROM people.students WHERE student_no ILIKE $1
			UNION
			SELECT s2.id FROM people.persons p2 JOIN people.students s2 ON s2.person_id = p2.id
			WHERE (p2.first_name || ' ' || p2.last_name) ILIKE $1)`, "%"+f.Query+"%")
	}
	if f.After != nil {
		w.Add("(h.placed_at, h.id) < ($1, $2::uuid)", f.After.PlacedAt, f.After.ID)
	}
	limit := w.Arg(f.Limit + 1)

	rows, err := r.db.Query(ctx, holdSelect+"\n\t"+w.SQL()+"\n\tORDER BY h.placed_at DESC, h.id DESC\n\tLIMIT "+limit, w.Args()...)
	if err != nil {
		return nil, false, fmt.Errorf("enrollment: engeller listelenemedi: %w", err)
	}
	items, err = pgx.CollectRows(rows, scanHold)
	if err != nil {
		return nil, false, fmt.Errorf("enrollment: engeller okunamadı: %w", err)
	}
	if len(items) > f.Limit {
		return items[:f.Limit], true, nil
	}
	return items, false, nil
}

// StatusHistory, program kaydının durum değişikliklerini en yeniden eskiye döndürür.
func (r *Repository) StatusHistory(ctx context.Context, studentProgramID string) ([]StatusChange, error) {
	rows, err := r.db.Query(ctx, `
		SELECT h.from_status, h.to_status, coalesce(h.reason, ''), coalesce(h.decision_ref, ''),
		       coalesce(pe.first_name || ' ' || pe.last_name, ''), h.changed_at
		FROM enrollment.student_program_status_history h
		LEFT JOIN iam.users u ON u.id = h.changed_by
		LEFT JOIN people.persons pe ON pe.id = u.person_id
		WHERE h.student_program_id = $1
		ORDER BY h.changed_at DESC, h.id DESC`, studentProgramID)
	if err != nil {
		return nil, fmt.Errorf("enrollment: durum geçmişi okunamadı: %w", err)
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (StatusChange, error) {
		var (
			c        StatusChange
			from     *string
			to       string
			changeAt time.Time
		)
		err := row.Scan(&from, &to, &c.Reason, &c.DecisionRef, &c.ChangedBy, &changeAt)
		if from != nil {
			f := Status(*from)
			c.From = &f
		}
		c.To, c.ChangedAt = Status(to), changeAt
		return c, err
	})
	if err != nil {
		return nil, fmt.Errorf("enrollment: durum geçmişi okunamadı: %w", err)
	}
	return list, nil
}

// --- Kayıt engelleri ve durum: yazma -----------------------------------------------

// PlaceHold, program kaydına engel koyar. Aynı türde aktif engel varsa ErrHoldExists,
// kayıt süren bir kayıt değilse ErrNotActive döner.
func (s *Service) PlaceHold(ctx context.Context, actorID, studentProgramID string, t HoldType, blocks []HoldBlock, reason string) (string, error) {
	var id string
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var status string
		err := tx.QueryRow(ctx, `SELECT status FROM enrollment.student_programs WHERE id = $1`, studentProgramID).Scan(&status)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("enrollment: program kaydı okunamadı: %w", err)
		}
		if !Status(status).Ongoing() {
			return ErrNotActive
		}

		list := make([]string, len(blocks))
		for i, b := range blocks {
			list[i] = string(b)
		}
		err = tx.QueryRow(ctx, `
			INSERT INTO enrollment.holds (student_program_id, hold_type, blocks, reason, placed_by)
			VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			studentProgramID, string(t), list, reason, nullable(actorID)).Scan(&id)
		if db.IsConflict(err) {
			return ErrHoldExists
		}
		if err != nil {
			return fmt.Errorf("enrollment: engel konulamadı: %w", err)
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "hold.place", EntityType: "student_program", EntityID: studentProgramID,
			After: map[string]any{"hold_id": id, "hold_type": t, "blocks": list, "reason": reason},
		})
	})
	return id, err
}

// ReleaseHold, engeli gerekçeyle kaldırır. Zaten kaldırılmışsa ErrHoldReleased döner.
func (s *Service) ReleaseHold(ctx context.Context, actorID, holdID, reason string) error {
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var (
			studentProgramID string
			releasedAt       *time.Time
		)
		err := tx.QueryRow(ctx, `
			SELECT student_program_id, released_at FROM enrollment.holds WHERE id = $1 FOR UPDATE`, holdID,
		).Scan(&studentProgramID, &releasedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("enrollment: engel okunamadı: %w", err)
		}
		if releasedAt != nil {
			return ErrHoldReleased
		}
		if _, err := tx.Exec(ctx, `
			UPDATE enrollment.holds SET released_at = now(), released_by = $2, release_reason = $3 WHERE id = $1`,
			holdID, nullable(actorID), reason); err != nil {
			return fmt.Errorf("enrollment: engel kaldırılamadı: %w", err)
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "hold.release", EntityType: "student_program", EntityID: studentProgramID,
			After: map[string]any{"hold_id": holdID, "reason": reason},
		})
	})
}

// StatusChangeInput, program kaydının durum değişikliğidir.
type StatusChangeInput struct {
	StudentProgramID string
	Version          int // iyimser kilit
	To               Status
	Reason           string
	DecisionRef      string
	GraduatedOn      *time.Time // sadece mezuniyette
}

// ChangeStatus, program kaydının durumunu değiştirir ve geçmişe yazar. Geçiş geçersizse
// ErrInvalidTransition, sürüm tutmazsa ErrVersionMismatch döner. Kesin bir duruma
// (mezuniyet, ayrılma, ilişik kesme) geçen kaydın aktif danışmanlığı o anda sona erer.
func (s *Service) ChangeStatus(ctx context.Context, actorID string, in StatusChangeInput) error {
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var (
			from    string
			version int
		)
		err := tx.QueryRow(ctx, `
			SELECT status, version FROM enrollment.student_programs WHERE id = $1 FOR UPDATE`, in.StudentProgramID,
		).Scan(&from, &version)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("enrollment: program kaydı okunamadı: %w", err)
		}
		if version != in.Version {
			return ErrVersionMismatch
		}
		if !Status(from).CanBecome(in.To) {
			return ErrInvalidTransition
		}

		var graduatedOn any
		if in.To == StatusGraduated {
			graduatedOn = in.GraduatedOn
		}
		// Hazırlıktan çıkan öğrenci 1. sınıfın 1. yarıyılından başlar.
		if _, err := tx.Exec(ctx, `
			UPDATE enrollment.student_programs SET
				status = $2, status_changed_at = now(), graduated_on = $3,
				class_level      = CASE WHEN status = 'PREP' THEN 1 ELSE class_level END,
				current_semester = CASE WHEN status = 'PREP' THEN 1 ELSE current_semester END,
				version = version + 1, updated_at = now()
			WHERE id = $1`, in.StudentProgramID, string(in.To), graduatedOn); err != nil {
			return fmt.Errorf("enrollment: durum değiştirilemedi: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO enrollment.student_program_status_history
				(student_program_id, from_status, to_status, reason, decision_ref, changed_by)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			in.StudentProgramID, from, string(in.To), in.Reason, nullable(in.DecisionRef), nullable(actorID)); err != nil {
			return fmt.Errorf("enrollment: durum geçmişi yazılamadı: %w", err)
		}
		if !in.To.Ongoing() {
			if _, err := tx.Exec(ctx, `
				UPDATE enrollment.advisor_assignments SET valid_until = clock_timestamp()
				WHERE student_program_id = $1 AND valid_until IS NULL`, in.StudentProgramID); err != nil {
				return fmt.Errorf("enrollment: danışmanlık sonlandırılamadı: %w", err)
			}
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "student_program.status", EntityType: "student_program", EntityID: in.StudentProgramID,
			Before: map[string]any{"status": from},
			After:  map[string]any{"status": in.To, "reason": in.Reason, "decision_ref": in.DecisionRef, "graduated_on": graduatedOn},
		})
	})
}
