package iam

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// Rol yönetimi hataları.
var (
	ErrUnknownRole   = errors.New("iam: rol bulunamadı")
	ErrInvalidScope  = errors.New("iam: kapsam rolün kapsam türüne uymuyor")
	ErrUnknownScope  = errors.New("iam: kapsamdaki birim bulunamadı")
	ErrInvalidPeriod = errors.New("iam: bitiş başlangıçtan sonra olmalı")
)

// Atama durumları.
const (
	AssignmentActive   = "ACTIVE"
	AssignmentUpcoming = "UPCOMING"
	AssignmentEnded    = "ENDED"
)

// RoleDef, rol kataloğundaki bir roldür.
type RoleDef struct {
	Code        string
	NameTR      string
	NameEN      string
	ScopeType   ScopeType
	Description string
	Permissions []string
}

// Roles, rol kataloğunu yetkileriyle birlikte döndürür.
func (r *Repository) Roles(ctx context.Context) ([]RoleDef, error) {
	rows, err := r.db.Query(ctx, `
		SELECT ro.code, ro.name_tr, ro.name_en, ro.scope_type, coalesce(ro.description, ''),
		       coalesce(array_agg(p.code ORDER BY p.code) FILTER (WHERE p.code IS NOT NULL), '{}')
		FROM iam.roles ro
		LEFT JOIN iam.role_permissions rp ON rp.role_id = ro.id
		LEFT JOIN iam.permissions p ON p.id = rp.permission_id
		GROUP BY ro.id
		ORDER BY ro.code`)
	if err != nil {
		return nil, fmt.Errorf("iam: roller okunamadı: %w", err)
	}
	roles, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (RoleDef, error) {
		var (
			d     RoleDef
			scope string
		)
		err := row.Scan(&d.Code, &d.NameTR, &d.NameEN, &scope, &d.Description, &d.Permissions)
		d.ScopeType = ScopeType(scope)
		return d, err
	})
	if err != nil {
		return nil, fmt.Errorf("iam: roller okunamadı: %w", err)
	}
	return roles, nil
}

// Assignment, bir kullanıcının (geçmiş, şimdiki ya da ileri tarihli) rol atamasıdır.
type Assignment struct {
	ID         string
	Role       string
	RoleName   string
	ScopeType  ScopeType
	ScopeID    string
	ScopeName  string
	ValidFrom  time.Time
	ValidUntil *time.Time
	State      string // AssignmentActive, AssignmentUpcoming ya da AssignmentEnded
	AssignedBy string // atamayı yapan kullanıcı; seed ve sistem atamalarında boş
	Reason     string
	CreatedAt  time.Time
}

const assignmentSelect = `
	SELECT ra.id, ro.code, ro.name_tr, ra.scope_type, coalesce(ra.scope_id::text, ''),
	       coalesce(f.name_tr, d.name_tr, pr.name_tr, ''), ra.valid_from, ra.valid_until,
	       CASE WHEN ra.valid_from > now() THEN 'UPCOMING'
	            WHEN ra.valid_until IS NOT NULL AND ra.valid_until <= now() THEN 'ENDED'
	            ELSE 'ACTIVE' END,
	       coalesce(ra.assigned_by::text, ''), coalesce(ra.reason, ''), ra.created_at
	FROM iam.role_assignments ra
	JOIN iam.roles ro ON ro.id = ra.role_id
	LEFT JOIN org.faculties f   ON ra.scope_type = 'FACULTY'    AND f.id = ra.scope_id
	LEFT JOIN org.departments d ON ra.scope_type = 'DEPARTMENT' AND d.id = ra.scope_id
	LEFT JOIN org.programs pr   ON ra.scope_type = 'PROGRAM'    AND pr.id = ra.scope_id`

func scanAssignment(row pgx.CollectableRow) (Assignment, error) {
	var (
		a     Assignment
		scope string
	)
	err := row.Scan(&a.ID, &a.Role, &a.RoleName, &scope, &a.ScopeID, &a.ScopeName, &a.ValidFrom,
		&a.ValidUntil, &a.State, &a.AssignedBy, &a.Reason, &a.CreatedAt)
	a.ScopeType = ScopeType(scope)
	return a, err
}

// UserAssignments, kullanıcının bütün rol atamalarını (geçmiş dahil) en yeniden
// eskiye döndürür. Durum veritabanı saatiyle hesaplanır.
func (r *Repository) UserAssignments(ctx context.Context, userID string) ([]Assignment, error) {
	rows, err := r.db.Query(ctx, assignmentSelect+`
		WHERE ra.user_id = $1
		ORDER BY ra.valid_from DESC, ra.id DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("iam: rol atamaları okunamadı: %w", err)
	}
	list, err := pgx.CollectRows(rows, scanAssignment)
	if err != nil {
		return nil, fmt.Errorf("iam: rol atamaları okunamadı: %w", err)
	}
	return list, nil
}

func (r *Repository) assignment(ctx context.Context, userID, assignmentID string) (Assignment, error) {
	rows, err := r.db.Query(ctx, assignmentSelect+`
		WHERE ra.user_id = $1 AND ra.id = $2`, userID, assignmentID)
	if err != nil {
		return Assignment{}, fmt.Errorf("iam: rol ataması okunamadı: %w", err)
	}
	a, err := pgx.CollectExactlyOneRow(rows, scanAssignment)
	if errors.Is(err, pgx.ErrNoRows) {
		return Assignment{}, ErrRoleAssignmentNotFound
	}
	if err != nil {
		return Assignment{}, fmt.Errorf("iam: rol ataması okunamadı: %w", err)
	}
	return a, nil
}

// AssignInput, rol atama isteğidir.
type AssignInput struct {
	Role       string
	ScopeID    string     // rolün kapsam türü UNIVERSITY ya da NONE ise boş
	ValidFrom  *time.Time // boşsa hemen
	ValidUntil *time.Time // boşsa süresiz
	Reason     string
}

// AssignRole, kullanıcıya bir rol atar ve kullanıcının yetki sürümünü artırır:
// değişiklik bir sonraki istekte etkili olur.
//
// Kişi kendine rol atayamaz (görevler ayrılığı). Aynı rol aynı kapsamda çakışan bir
// dönemde zaten atanmışsa ErrConflict döner.
func (a *Auth) AssignRole(ctx context.Context, actorID, userID string, in AssignInput) (string, error) {
	if actorID == userID {
		return "", ErrSelfAction
	}

	var id string
	err := db.InTx(ctx, a.pool, func(tx pgx.Tx) error {
		if _, err := NewRepository(tx).UserByID(ctx, userID); err != nil {
			return err
		}

		var scope string
		err := tx.QueryRow(ctx, `SELECT scope_type FROM iam.roles WHERE code = $1`, in.Role).Scan(&scope)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUnknownRole
		}
		if err != nil {
			return fmt.Errorf("iam: rol okunamadı: %w", err)
		}
		if err := checkScope(ctx, tx, ScopeType(scope), in.ScopeID); err != nil {
			return err
		}

		err = tx.QueryRow(ctx, `
			INSERT INTO iam.role_assignments (user_id, role_id, scope_type, scope_id, valid_from, valid_until, assigned_by, reason)
			SELECT $1, r.id, r.scope_type, $3, coalesce($4, now()), $5, $6, $7
			FROM iam.roles r WHERE r.code = $2
			RETURNING id`,
			userID, in.Role, nullable(in.ScopeID), in.ValidFrom, in.ValidUntil, nullable(actorID), in.Reason,
		).Scan(&id)
		switch {
		case db.IsConflict(err):
			return ErrConflict
		case db.IsCheckViolation(err):
			return ErrInvalidPeriod
		case err != nil:
			return fmt.Errorf("iam: rol atanamadı: %w", err)
		}

		if err := bumpPermVersion(ctx, tx, userID); err != nil {
			return err
		}
		assigned, err := NewRepository(tx).assignment(ctx, userID, id)
		if err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "role.assign", EntityType: "user", EntityID: userID,
			After: assignmentAudit(assigned),
		})
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

// EndAssignment, bir rol atamasını sonlandırır. Atama başlamışsa bitiş zamanı
// "şimdi" yapılır (geçmiş korunur), henüz başlamamışsa iptal edilip silinir: hiç
// yürürlüğe girmemiş bir atamanın saklanacak geçmişi yoktur, iz denetim kaydında
// kalır. Zaten bitmiş bir atama için bir şey yapılmaz.
//
// Son aktif sistem yöneticisinin rolü sonlandırılamaz: sistem yönetimsiz kalırdı.
func (a *Auth) EndAssignment(ctx context.Context, actorID, userID, assignmentID, reason string) error {
	if actorID == userID {
		return ErrSelfAction
	}

	return db.InTx(ctx, a.pool, func(tx pgx.Tx) error {
		r := NewRepository(tx)
		before, err := r.assignment(ctx, userID, assignmentID)
		if err != nil {
			return err
		}
		if before.State == AssignmentEnded {
			return nil
		}

		if before.Role == "SYSTEM_ADMIN" && before.State == AssignmentActive {
			if err := ensureAnotherAdmin(ctx, tx, assignmentID); err != nil {
				return err
			}
		}

		action := "role.end"
		if before.State == AssignmentUpcoming {
			action = "role.cancel"
			_, err = tx.Exec(ctx, `DELETE FROM iam.role_assignments WHERE id = $1`, assignmentID)
		} else {
			// now() transaction'ın başlangıç anıdır. Eşzamanlı bir transaction'ın
			// (ensureAnotherAdmin) bu bitişi doğru görmesi için gerçek an yazılır.
			_, err = tx.Exec(ctx, `UPDATE iam.role_assignments SET valid_until = clock_timestamp() WHERE id = $1`, assignmentID)
		}
		if err != nil {
			return fmt.Errorf("iam: rol ataması sonlandırılamadı: %w", err)
		}

		if err := bumpPermVersion(ctx, tx, userID); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: action, EntityType: "user", EntityID: userID,
			Before: assignmentAudit(before),
			After:  map[string]any{"assignment_id": assignmentID, "reason": reason},
		})
	})
}

// ensureAnotherAdmin, sonlandırılacak atama dışında en az bir aktif sistem yöneticisi
// ataması olduğunu doğrular. İki yöneticinin aynı anda birbirinin rolünü kaldırması
// (her biri "diğeri hâlâ var" görüp ikisinin de silinmesi) advisory lock ile önlenir.
//
// Sayım clock_timestamp() ile yapılır, now() ile değil: now() transaction'ın başladığı
// anı döndürür. Kilidi bekleyen transaction, önceki transaction'dan daha erken
// başlamışsa onun az önce yazdığı bitişi "henüz gelmemiş" görür ve kilit işe yaramazdı.
func ensureAnotherAdmin(ctx context.Context, tx pgx.Tx, exceptAssignmentID string) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('iam.system_admin_assignments'))`); err != nil {
		return fmt.Errorf("iam: kilit alınamadı: %w", err)
	}
	var others int
	err := tx.QueryRow(ctx, `
		SELECT count(*) FROM iam.role_assignments ra
		JOIN iam.roles r ON r.id = ra.role_id
		JOIN iam.users u ON u.id = ra.user_id
		WHERE r.code = 'SYSTEM_ADMIN' AND ra.id <> $1 AND u.status = 'ACTIVE'
		  AND ra.valid_from <= clock_timestamp()
		  AND (ra.valid_until IS NULL OR ra.valid_until > clock_timestamp())`,
		exceptAssignmentID).Scan(&others)
	if err != nil {
		return fmt.Errorf("iam: sistem yöneticileri sayılamadı: %w", err)
	}
	if others == 0 {
		return ErrLastSystemAdmin
	}
	return nil
}

// checkScope, kapsam kimliğinin rolün kapsam türüne uyduğunu ve birimin var
// olduğunu doğrular.
func checkScope(ctx context.Context, q db.Querier, scope ScopeType, scopeID string) error {
	var table string
	switch scope {
	case ScopeUniversity, ScopeNone:
		if scopeID != "" {
			return ErrInvalidScope
		}
		return nil
	case ScopeFaculty:
		table = "org.faculties"
	case ScopeDepartment:
		table = "org.departments"
	case ScopeProgram:
		table = "org.programs"
	default:
		return ErrInvalidScope
	}
	if scopeID == "" {
		return ErrInvalidScope
	}

	var ok bool
	// table yukarıdaki sabitlerden biridir, kullanıcı girdisi değildir.
	if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM `+table+` WHERE id = $1)`, scopeID).Scan(&ok); err != nil {
		return fmt.Errorf("iam: kapsam denetlenemedi: %w", err)
	}
	if !ok {
		return ErrUnknownScope
	}
	return nil
}

func bumpPermVersion(ctx context.Context, q db.Querier, userID string) error {
	if _, err := q.Exec(ctx,
		`UPDATE iam.users SET perm_version = perm_version + 1, updated_at = now() WHERE id = $1`, userID); err != nil {
		return fmt.Errorf("iam: yetki sürümü artırılamadı: %w", err)
	}
	return nil
}

func assignmentAudit(a Assignment) map[string]any {
	m := map[string]any{
		"assignment_id": a.ID, "role": a.Role, "scope_type": a.ScopeType,
		"valid_from": a.ValidFrom.UTC().Format(time.RFC3339), "reason": a.Reason,
	}
	if a.ScopeID != "" {
		m["scope_id"] = a.ScopeID
	}
	if a.ValidUntil != nil {
		m["valid_until"] = a.ValidUntil.UTC().Format(time.RFC3339)
	}
	return m
}
