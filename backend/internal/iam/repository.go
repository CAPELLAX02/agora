package iam

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// Repository, iam şemasına erişen veri katmanıdır.
type Repository struct {
	db db.Querier
}

// NewRepository, verilen bağlantıyı (havuz veya transaction) kullanan bir Repository oluşturur.
func NewRepository(q db.Querier) *Repository {
	return &Repository{db: q}
}

const userSelect = `
	SELECT id, person_id, username, email, status, password_hash, must_change_password,
	       failed_login_count, locked_until, last_login_at, perm_version, created_at, updated_at
	FROM iam.users`

// UserByUsername, kullanıcı adına göre hesabı döndürür. Karşılaştırma büyük/küçük
// harf duyarsızdır (sütun citext). Bulunamazsa ErrNotFound döner.
func (r *Repository) UserByUsername(ctx context.Context, username string) (User, error) {
	return r.user(ctx, userSelect+" WHERE username = $1", username)
}

// UserByID, kimliğe göre hesabı döndürür. Bulunamazsa ErrNotFound döner.
func (r *Repository) UserByID(ctx context.Context, id string) (User, error) {
	return r.user(ctx, userSelect+" WHERE id = $1", id)
}

func (r *Repository) user(ctx context.Context, query string, arg any) (User, error) {
	var (
		u      User
		status string
	)
	err := r.db.QueryRow(ctx, query, arg).Scan(
		&u.ID, &u.PersonID, &u.Username, &u.Email, &status, &u.PasswordHash, &u.MustChangePassword,
		&u.FailedLoginCount, &u.LockedUntil, &u.LastLoginAt, &u.PermVersion, &u.CreatedAt, &u.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("iam: kullanıcı okunamadı: %w", err)
	}
	u.Status = UserStatus(status)
	return u, nil
}

// CreateUser, yeni bir hesap oluşturur ve kimliğini döndürür. Kullanıcı adı ya da
// e-posta başka bir hesapta varsa ErrConflict döner.
func (r *Repository) CreateUser(ctx context.Context, u NewUser) (string, error) {
	var id string
	err := r.db.QueryRow(ctx,
		`INSERT INTO iam.users (person_id, username, email, password_hash, must_change_password, status)
		 VALUES ($1, $2, $3, $4, $5, coalesce($6, 'ACTIVE'))
		 RETURNING id`,
		u.PersonID, u.Username, u.Email, u.PasswordHash, u.MustChangePassword, nullable(string(u.Status)),
	).Scan(&id)
	if db.IsConflict(err) {
		return "", ErrConflict
	}
	if err != nil {
		return "", fmt.Errorf("iam: kullanıcı oluşturulamadı: %w", err)
	}
	return id, nil
}

// AssignRole, kullanıcıya bir rolü verilen kapsamda atar ve kullanıcının yetki
// sürümünü (perm_version) artırır. İki işlem tek bir SQL ifadesinde yapılır,
// bu yüzden ayrıca transaction gerekmez.
//
// Rol bu kapsam türüne ait değilse ErrInvalidRoleScope, aynı rol aynı kapsamda
// zaten aktifse ErrConflict döner.
func (r *Repository) AssignRole(ctx context.Context, userID, roleCode string, scope ScopeType, scopeID string) error {
	var version int
	err := r.db.QueryRow(ctx, `
		WITH assigned AS (
			INSERT INTO iam.role_assignments (user_id, role_id, scope_type, scope_id)
			SELECT $1, r.id, $3, $4
			FROM iam.roles r
			WHERE r.code = $2 AND r.scope_type = $3
			RETURNING user_id
		)
		UPDATE iam.users
		SET perm_version = perm_version + 1, updated_at = now()
		WHERE id = (SELECT user_id FROM assigned)
		RETURNING perm_version`,
		userID, roleCode, string(scope), nullable(scopeID),
	).Scan(&version)

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ErrInvalidRoleScope // rol yok ya da kapsam türü uymuyor, hiçbir satır eklenmedi
	case db.IsConflict(err):
		return ErrConflict
	case err != nil:
		return fmt.Errorf("iam: rol atanamadı: %w", err)
	}
	return nil
}

// Grants, kullanıcının verilen anda geçerli olan tüm yetkilerini döndürür.
func (r *Repository) Grants(ctx context.Context, userID string, at time.Time) ([]Grant, error) {
	rows, err := r.db.Query(ctx, `
		SELECT p.code, r.code, ra.scope_type, ra.scope_id, r.relationship_scoped
		FROM iam.role_assignments ra
		JOIN iam.roles r            ON r.id = ra.role_id
		JOIN iam.role_permissions rp ON rp.role_id = r.id
		JOIN iam.permissions p      ON p.id = rp.permission_id
		WHERE ra.user_id = $1
		  AND ra.valid_from <= $2
		  AND (ra.valid_until IS NULL OR ra.valid_until > $2)
		ORDER BY p.code, r.code, ra.scope_id`,
		userID, at,
	)
	if err != nil {
		return nil, fmt.Errorf("iam: yetkiler okunamadı: %w", err)
	}
	defer rows.Close()

	grants := []Grant{}
	for rows.Next() {
		var (
			g       Grant
			scope   string
			scopeID *string
		)
		if err := rows.Scan(&g.Permission, &g.Role, &scope, &scopeID, &g.RelationshipScoped); err != nil {
			return nil, fmt.Errorf("iam: yetki okunamadı: %w", err)
		}
		g.ScopeType = ScopeType(scope)
		if scopeID != nil {
			g.ScopeID = *scopeID
		}
		grants = append(grants, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iam: yetkiler okunamadı: %w", err)
	}
	return grants, nil
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
