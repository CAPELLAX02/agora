package iam

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Profile, kullanıcının kendisi hakkında görebildiği bilgilerdir.
type Profile struct {
	UserID             string
	Username           string
	Email              string
	FirstName          string
	LastName           string
	Status             UserStatus
	MustChangePassword bool
	MFAEnabled         bool
	LastLoginAt        *time.Time
	Roles              []RoleAssignment
}

// RoleAssignment, kullanıcının bir roldeki geçerli atamasıdır.
type RoleAssignment struct {
	Role       string
	RoleName   string
	ScopeType  ScopeType
	ScopeID    string // UNIVERSITY ve NONE kapsamlarında boş
	ScopeName  string // kapsamdaki birimin adı (ör. "Bilgisayar Mühendisliği")
	ValidUntil *time.Time
}

// Profile, kullanıcının profilini ve verilen anda geçerli rol atamalarını döndürür.
// Kullanıcı yoksa ErrNotFound döner.
func (r *Repository) Profile(ctx context.Context, userID string, at time.Time) (Profile, error) {
	var (
		p      Profile
		status string
	)
	err := r.db.QueryRow(ctx, `
		SELECT u.id, u.username, u.email, u.status, u.must_change_password, u.mfa_enabled, u.last_login_at,
		       pe.first_name, pe.last_name
		FROM iam.users u
		JOIN people.persons pe ON pe.id = u.person_id
		WHERE u.id = $1`,
		userID,
	).Scan(&p.UserID, &p.Username, &p.Email, &status, &p.MustChangePassword, &p.MFAEnabled, &p.LastLoginAt,
		&p.FirstName, &p.LastName)
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	if err != nil {
		return Profile{}, fmt.Errorf("iam: profil okunamadı: %w", err)
	}
	p.Status = UserStatus(status)

	// Kapsamın adı, kapsam türüne göre farklı tablodan gelir. Her LEFT JOIN sadece
	// kendi türündeki satırlarla eşleşir, coalesce dolu olanı seçer.
	rows, err := r.db.Query(ctx, `
		SELECT ro.code, ro.name_tr, ra.scope_type, ra.scope_id,
		       coalesce(f.name_tr, d.name_tr, pr.name_tr), ra.valid_until
		FROM iam.role_assignments ra
		JOIN iam.roles ro ON ro.id = ra.role_id
		LEFT JOIN org.faculties f    ON ra.scope_type = 'FACULTY'    AND f.id = ra.scope_id
		LEFT JOIN org.departments d  ON ra.scope_type = 'DEPARTMENT' AND d.id = ra.scope_id
		LEFT JOIN org.programs pr    ON ra.scope_type = 'PROGRAM'    AND pr.id = ra.scope_id
		WHERE ra.user_id = $1
		  AND ra.valid_from <= $2
		  AND (ra.valid_until IS NULL OR ra.valid_until > $2)
		ORDER BY ro.code, ra.valid_from`,
		userID, at,
	)
	if err != nil {
		return Profile{}, fmt.Errorf("iam: roller okunamadı: %w", err)
	}

	p.Roles, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (RoleAssignment, error) {
		var (
			ra                 RoleAssignment
			scope              string
			scopeID, scopeName *string
		)
		err := row.Scan(&ra.Role, &ra.RoleName, &scope, &scopeID, &scopeName, &ra.ValidUntil)
		ra.ScopeType = ScopeType(scope)
		if scopeID != nil {
			ra.ScopeID = *scopeID
		}
		if scopeName != nil {
			ra.ScopeName = *scopeName
		}
		return ra, err
	})
	if err != nil {
		return Profile{}, fmt.Errorf("iam: roller okunamadı: %w", err)
	}
	return p, nil
}
