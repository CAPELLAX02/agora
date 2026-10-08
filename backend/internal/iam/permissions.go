package iam

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
)

// AccessState, kullanıcının hesap durumunu ve yetki sürümünü döndürür. Her korumalı
// istekte çalışır, bu yüzden sadece birincil anahtarla okunan iki sütundur.
func (r *Repository) AccessState(ctx context.Context, userID string) (UserStatus, int, error) {
	var (
		status  string
		version int
	)
	err := r.db.QueryRow(ctx,
		`SELECT status, perm_version FROM iam.users WHERE id = $1`, userID,
	).Scan(&status, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", 0, ErrNotFound
	}
	if err != nil {
		return "", 0, fmt.Errorf("iam: erişim durumu okunamadı: %w", err)
	}
	return UserStatus(status), version, nil
}

// NextGrantChange, kullanıcının yetkilerinin at'ten sonra zamanla değişeceği ilk anı
// döndürür: süreli bir atamanın bitişi ya da ileri tarihli bir atamanın başlangıcı.
// Böyle bir an yoksa nil döner.
func (r *Repository) NextGrantChange(ctx context.Context, userID string, at time.Time) (*time.Time, error) {
	var next *time.Time
	err := r.db.QueryRow(ctx, `
		SELECT min(t) FROM (
			SELECT valid_until AS t FROM iam.role_assignments WHERE user_id = $1 AND valid_until > $2
			UNION ALL
			SELECT valid_from FROM iam.role_assignments WHERE user_id = $1 AND valid_from > $2
		) changes`,
		userID, at,
	).Scan(&next)
	if err != nil {
		return nil, fmt.Errorf("iam: yetki değişim zamanı okunamadı: %w", err)
	}
	return next, nil
}

// PermissionResolver, kullanıcının güncel yetkilerini çözer ve Redis'te önbellekler.
//
// Önbellek anahtarı kullanıcının yetki sürümünü (perm_version) içerir. Rol ataması
// değişince sürüm artar ve yeni istekler yeni anahtara bakar: eski kayıt silinmeye
// gerek kalmadan kullanılmaz hale gelir ve süresi dolunca Redis'ten düşer.
type PermissionResolver struct {
	pool   *pgxpool.Pool
	rdb    redis.Cmdable
	ttl    time.Duration
	now    func() time.Time
	logger *slog.Logger
}

// NewPermissionResolver, bir PermissionResolver oluşturur. ttl, önbellek kaydının en uzun ömrüdür.
func NewPermissionResolver(pool *pgxpool.Pool, rdb redis.Cmdable, ttl time.Duration, now func() time.Time, logger *slog.Logger) *PermissionResolver {
	return &PermissionResolver{pool: pool, rdb: rdb, ttl: ttl, now: now, logger: logger}
}

func permissionsKey(userID string, version int) string {
	return fmt.Sprintf("agora:perms:%s:%d", userID, version)
}

// Permissions, kullanıcının yetkilerini döndürür. Hesap aktif değilse
// authz.ErrAccountInactive döner.
//
// Önbellek sadece bir hızlandırıcıdır: Redis'e ulaşılamazsa yetkiler veritabanından
// çözülür ve istek devam eder. Doğruluğun kaynağı her zaman veritabanıdır.
func (pr *PermissionResolver) Permissions(ctx context.Context, userID string) (*authz.Permissions, error) {
	repo := NewRepository(pr.pool)

	status, version, err := repo.AccessState(ctx, userID)
	if errors.Is(err, ErrNotFound) {
		return nil, authz.ErrAccountInactive
	}
	if err != nil {
		return nil, err
	}
	if status != StatusActive {
		return nil, authz.ErrAccountInactive
	}

	key := permissionsKey(userID, version)

	data, err := pr.rdb.Get(ctx, key).Bytes()
	switch {
	case err == nil:
		var grants []authz.Grant
		if err := json.Unmarshal(data, &grants); err == nil {
			return authz.NewPermissions(userID, grants), nil
		}
		pr.logger.Warn("yetki önbelleği bozuk, yeniden çözülüyor", "key", key)
	case !errors.Is(err, redis.Nil): // redis.Nil: anahtar yok, sıradan bir önbellek ıskası
		pr.logger.Warn("yetki önbelleği okunamadı, veritabanından çözülüyor", "err", err)
	}

	now := pr.now()
	rows, err := repo.Grants(ctx, userID, now)
	if err != nil {
		return nil, err
	}
	grants := toAuthzGrants(rows)

	// Süreli atamalar zamanla değişir ama perm_version'ı artırmaz. Kayıt, bir sonraki
	// değişim anında kendiliğinden düşsün diye ömrünü o ana kadar kısaltıyoruz.
	ttl := pr.ttl
	next, err := repo.NextGrantChange(ctx, userID, now)
	if err != nil {
		return nil, err
	}
	if next != nil && next.Sub(now) < ttl {
		ttl = next.Sub(now)
	}

	if data, err := json.Marshal(grants); err == nil {
		if err := pr.rdb.Set(ctx, key, data, ttl).Err(); err != nil {
			pr.logger.Warn("yetki önbelleğe yazılamadı", "err", err)
		}
	}
	return authz.NewPermissions(userID, grants), nil
}

// toAuthzGrants, rol bazlı satırları tekilleştirir: iki rol aynı yetkiyi aynı
// kapsamda veriyorsa (ör. öğretim elemanı ve bölüm başkanı) tek kayıt kalır.
func toAuthzGrants(rows []Grant) []authz.Grant {
	type key struct{ perm, scopeType, scopeID string }
	seen := make(map[key]bool, len(rows))
	out := make([]authz.Grant, 0, len(rows))
	for _, g := range rows {
		k := key{g.Permission, string(g.ScopeType), g.ScopeID}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, authz.Grant{Permission: g.Permission, ScopeType: string(g.ScopeType), ScopeID: g.ScopeID})
	}
	return out
}
