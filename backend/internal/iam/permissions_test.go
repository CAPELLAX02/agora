package iam_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/CAPELLAX02/agora/backend/internal/iam"
	"github.com/CAPELLAX02/agora/backend/internal/iam/password"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
	"github.com/CAPELLAX02/agora/backend/internal/platform/redistest"
)

type resolverEnv struct {
	pool     *pgxpool.Pool
	rdb      *redis.Client
	resolver *iam.PermissionResolver
	logs     *bytes.Buffer
}

func newResolverEnv(t *testing.T) *resolverEnv {
	t.Helper()
	pool := dbtest.New(t)
	rdb := redistest.New(t)
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(logs, nil))
	return &resolverEnv{
		pool:     pool,
		rdb:      rdb,
		resolver: iam.NewPermissionResolver(pool, rdb, time.Hour, logger),
		logs:     logs,
	}
}

func (e *resolverEnv) addUser(t *testing.T, username string) string {
	t.Helper()
	return (&authEnv{pool: e.pool}).addUser(t, username, pw, password.NewHasher(cheapParams, 1))
}

// assign, rolü verilen zaman aralığıyla atar ve yetki sürümünü artırır.
// validFrom ve validUntil, veritabanı saatine göre interval ifadeleridir.
func (e *resolverEnv) assign(t *testing.T, userID, role string, scope iam.ScopeType, scopeID, validFrom, validUntil string) {
	t.Helper()
	until := "NULL"
	if validUntil != "" {
		until = fmt.Sprintf("now() + interval '%s'", validUntil)
	}
	_, err := e.pool.Exec(context.Background(), fmt.Sprintf(`
		WITH a AS (
			INSERT INTO iam.role_assignments (user_id, role_id, scope_type, scope_id, valid_from, valid_until)
			SELECT $1, id, $3, $4, now() + interval '%s', %s FROM iam.roles WHERE code = $2
			RETURNING user_id
		)
		UPDATE iam.users SET perm_version = perm_version + 1 WHERE id = (SELECT user_id FROM a)`,
		validFrom, until),
		userID, role, string(scope), nullableArg(scopeID))
	if err != nil {
		t.Fatal(err)
	}
}

func nullableArg(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (e *resolverEnv) permissions(t *testing.T, userID string) *authz.Permissions {
	t.Helper()
	p, err := e.resolver.Permissions(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func (e *resolverEnv) cacheTTL(t *testing.T, userID string) time.Duration {
	t.Helper()
	keys := e.rdb.Keys(context.Background(), "agora:perms:"+userID+":*").Val()
	if len(keys) != 1 {
		t.Fatalf("önbellekte %d kayıt var, 1 bekleniyordu: %v", len(keys), keys)
	}
	return e.rdb.TTL(context.Background(), keys[0]).Val()
}

func TestPermissionResolver(t *testing.T) {
	e := newResolverEnv(t)
	deptID, facultyID := seedOrg(t, e.pool)
	userID := e.addUser(t, "P10002")
	e.assign(t, userID, "INSTRUCTOR", iam.ScopeDepartment, deptID, "-1 hour", "")
	e.assign(t, userID, "DEPARTMENT_HEAD", iam.ScopeDepartment, deptID, "-1 hour", "")

	p := e.permissions(t, userID)
	bil := authz.Target{FacultyID: facultyID, DepartmentID: deptID}

	if !p.Allows("quota:manage", bil) || !p.Allows("score:enter", bil) {
		t.Error("bölüm başkanı ve öğretim elemanı yetkileri bölümünde geçerli olmalı")
	}
	if p.Allows("quota:manage", authz.Target{FacultyID: facultyID, DepartmentID: "baska-bolum"}) {
		t.Error("yetki başka bölüme taşmamalı")
	}
	if p.Has("role:assign") {
		t.Error("bölüm başkanının rol atama yetkisi olmamalı")
	}

	// İki rol de analitik okuma veriyor: aynı kapsamda tek kayıt kalmalı.
	var n int
	for _, g := range p.Grants() {
		if g.Permission == "analytics:read_scoped" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("analytics:read_scoped %d kez var, tekilleştirilmeliydi", n)
	}

	if ttl := e.cacheTTL(t, userID); ttl <= 59*time.Minute {
		t.Errorf("süresiz atamalarda önbellek ömrü 1 saat olmalı: %v", ttl)
	}
}

// TestPermissionResolverCacheInvalidation, önbelleğin gerçekten kullanıldığını ve
// yetki sürümü artınca devre dışı kaldığını doğrular.
func TestPermissionResolverCacheInvalidation(t *testing.T) {
	e := newResolverEnv(t)
	ctx := context.Background()
	userID := e.addUser(t, "P90001")
	e.assign(t, userID, "SYSTEM_ADMIN", iam.ScopeUniversity, "", "-1 hour", "")

	if !e.permissions(t, userID).Has("role:assign") {
		t.Fatal("yönetici rol atayabilmeli")
	}

	// Atamayı sürüm artırmadan sil: önbellek hâlâ eski yetkileri vermeli. Bu, sonucun
	// gerçekten önbellekten geldiğinin kanıtı (ve sürümü artırmanın neden şart olduğunun).
	if _, err := e.pool.Exec(ctx, `DELETE FROM iam.role_assignments WHERE user_id = $1`, userID); err != nil {
		t.Fatal(err)
	}
	if !e.permissions(t, userID).Has("role:assign") {
		t.Fatal("sürüm değişmeden önbellek kullanılmalıydı")
	}

	// Sürüm artınca yeni anahtara bakılır: değişiklik anında etkili.
	if _, err := e.pool.Exec(ctx, `UPDATE iam.users SET perm_version = perm_version + 1 WHERE id = $1`, userID); err != nil {
		t.Fatal(err)
	}
	if e.permissions(t, userID).Has("role:assign") {
		t.Error("sürüm arttıktan sonra eski yetkiler dönmemeli")
	}
}

// TestPermissionResolverTimeBounded, süreli atamaların önbellekte süresinden fazla
// yaşamadığını doğrular. Zamanla değişen bir atama perm_version'ı artırmaz.
func TestPermissionResolverTimeBounded(t *testing.T) {
	e := newResolverEnv(t)
	deptID, _ := seedOrg(t, e.pool)

	t.Run("biten atama", func(t *testing.T) {
		userID := e.addUser(t, "P10003")
		e.assign(t, userID, "DEPARTMENT_HEAD", iam.ScopeDepartment, deptID, "-1 year", "5 minutes")

		if !e.permissions(t, userID).Has("quota:manage") {
			t.Fatal("görev süresi içinde yetki olmalı")
		}
		if ttl := e.cacheTTL(t, userID); ttl > 5*time.Minute {
			t.Errorf("önbellek ömrü %v, görevin bitişini (5 dk) geçmemeli", ttl)
		}
	})

	t.Run("ileri tarihli atama", func(t *testing.T) {
		userID := e.addUser(t, "P10004")
		e.assign(t, userID, "DEPARTMENT_HEAD", iam.ScopeDepartment, deptID, "2 minutes", "")

		if e.permissions(t, userID).Has("quota:manage") {
			t.Fatal("başlamamış görevin yetkisi olmamalı")
		}
		if ttl := e.cacheTTL(t, userID); ttl > 2*time.Minute {
			t.Errorf("önbellek ömrü %v, görevin başlangıcını (2 dk) geçmemeli", ttl)
		}
	})
}

func TestPermissionResolverInactive(t *testing.T) {
	e := newResolverEnv(t)
	userID := e.addUser(t, "22290001")
	if _, err := e.pool.Exec(context.Background(), `UPDATE iam.users SET status = 'SUSPENDED' WHERE id = $1`, userID); err != nil {
		t.Fatal(err)
	}

	if _, err := e.resolver.Permissions(context.Background(), userID); !errors.Is(err, authz.ErrAccountInactive) {
		t.Errorf("askıdaki hesap: err = %v", err)
	}
	if _, err := e.resolver.Permissions(context.Background(), "01a11b7f-0000-7000-8000-000000000000"); !errors.Is(err, authz.ErrAccountInactive) {
		t.Errorf("olmayan hesap: err = %v", err)
	}
}

// TestPermissionResolverWithoutRedis, önbelleğe ulaşılamadığında yetkilerin
// veritabanından çözüldüğünü doğrular: önbellek bir güvenlik kontrolü değil,
// hızlandırıcıdır.
func TestPermissionResolverWithoutRedis(t *testing.T) {
	e := newResolverEnv(t)
	userID := e.addUser(t, "P90001")
	e.assign(t, userID, "SYSTEM_ADMIN", iam.ScopeUniversity, "", "-1 hour", "")
	_ = e.rdb.Close()

	if !e.permissions(t, userID).Has("user:read") {
		t.Error("Redis yokken de yetkiler çözülmeli")
	}
	if !strings.Contains(e.logs.String(), "yetki önbelleği okunamadı") {
		t.Errorf("uyarı log'a yazılmadı:\n%s", e.logs.String())
	}
}

func TestPermissionResolverCorruptCache(t *testing.T) {
	e := newResolverEnv(t)
	ctx := context.Background()
	userID := e.addUser(t, "P90001")
	e.assign(t, userID, "SYSTEM_ADMIN", iam.ScopeUniversity, "", "-1 hour", "")

	state, err := iam.NewRepository(e.pool).AccessState(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	key := fmt.Sprintf("agora:perms:%s:%d", userID, state.PermVersion)
	e.rdb.Set(ctx, key, "bozuk{", time.Hour)

	if !e.permissions(t, userID).Has("user:read") {
		t.Error("bozuk önbellek kaydı yok sayılıp yeniden çözülmeli")
	}
	if got := e.rdb.Get(ctx, key).Val(); !strings.HasPrefix(got, "[") {
		t.Errorf("bozuk kayıt düzeltilmedi: %q", got)
	}
}
