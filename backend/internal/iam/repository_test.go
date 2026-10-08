package iam_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/iam"
	"github.com/CAPELLAX02/agora/backend/internal/people"
	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
)

func TestRepository(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	repo := iam.NewRepository(pool)
	deptID, facultyID := seedOrg(t, pool)

	userID := createUser(t, pool, "P10001", "ayse@agora.test")

	t.Run("kullanıcı adı büyük/küçük harf duyarsız bulunur", func(t *testing.T) {
		u, err := repo.UserByUsername(ctx, "p10001")
		if err != nil {
			t.Fatal(err)
		}
		if u.ID != userID || u.Status != iam.StatusActive || u.PermVersion != 1 || u.LastLoginAt != nil {
			t.Errorf("kullanıcı = %+v", u)
		}
	})

	t.Run("olmayan kullanıcı ErrNotFound", func(t *testing.T) {
		if _, err := repo.UserByUsername(ctx, "yok"); !errors.Is(err, iam.ErrNotFound) {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("aynı kullanıcı adı ErrConflict", func(t *testing.T) {
		personID, _ := people.NewRepository(pool).CreatePerson(ctx, people.NewPerson{FirstName: "X", LastName: "Y"})
		_, err := repo.CreateUser(ctx, iam.NewUser{PersonID: personID, Username: "p10001", Email: "baska@agora.test", PasswordHash: "h"})
		if !errors.Is(err, iam.ErrConflict) {
			t.Errorf("err = %v, want ErrConflict", err)
		}
	})

	t.Run("rol ataması yetkileri getirir ve yetki sürümünü artırır", func(t *testing.T) {
		if err := repo.AssignRole(ctx, userID, "INSTRUCTOR", iam.ScopeDepartment, deptID); err != nil {
			t.Fatal(err)
		}
		if err := repo.AssignRole(ctx, userID, "FACULTY_REGISTRAR", iam.ScopeFaculty, facultyID); err != nil {
			t.Fatal(err)
		}

		u, _ := repo.UserByID(ctx, userID)
		if u.PermVersion != 3 {
			t.Errorf("perm_version = %d, want 3 (iki atama sonrası)", u.PermVersion)
		}

		grants, err := repo.Grants(ctx, userID, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		has := func(perm, role, scopeID string) bool {
			return slices.ContainsFunc(grants, func(g iam.Grant) bool {
				return g.Permission == perm && g.Role == role && g.ScopeID == scopeID
			})
		}
		if !has("score:enter", "INSTRUCTOR", deptID) {
			t.Error("INSTRUCTOR rolünden score:enter (bölüm kapsamında) gelmedi")
		}
		if !has("registration:override", "FACULTY_REGISTRAR", facultyID) {
			t.Error("FACULTY_REGISTRAR rolünden registration:override (fakülte kapsamında) gelmedi")
		}
		if !has("profile:read_own", "INSTRUCTOR", deptID) {
			t.Error("ortak yetki profile:read_own gelmedi")
		}
		if has("user:manage", "INSTRUCTOR", deptID) {
			t.Error("INSTRUCTOR'a ait olmayan user:manage geldi")
		}
	})

	t.Run("aynı rol aynı kapsamda tekrar atanamaz (EXCLUDE kısıtı)", func(t *testing.T) {
		err := repo.AssignRole(ctx, userID, "INSTRUCTOR", iam.ScopeDepartment, deptID)
		if !errors.Is(err, iam.ErrConflict) {
			t.Errorf("err = %v, want ErrConflict", err)
		}
	})

	t.Run("rol yanlış kapsam türüyle atanamaz", func(t *testing.T) {
		err := repo.AssignRole(ctx, userID, "DEPARTMENT_HEAD", iam.ScopeFaculty, facultyID)
		if !errors.Is(err, iam.ErrInvalidRoleScope) {
			t.Errorf("err = %v, want ErrInvalidRoleScope", err)
		}
		if err := repo.AssignRole(ctx, userID, "OLMAYAN_ROL", iam.ScopeNone, ""); !errors.Is(err, iam.ErrInvalidRoleScope) {
			t.Errorf("olmayan rol: err = %v", err)
		}
	})

	t.Run("süresi dolmuş ve henüz başlamamış atamalar yetki vermez", func(t *testing.T) {
		otherID := createUser(t, pool, "P10002", "mehmet@agora.test")
		_, err := pool.Exec(ctx, `
			INSERT INTO iam.role_assignments (user_id, role_id, scope_type, scope_id, valid_from, valid_until)
			SELECT $1::uuid, id, 'DEPARTMENT', $2::uuid, now() - interval '2 years', now() - interval '1 year'
			FROM iam.roles WHERE code = 'DEPARTMENT_HEAD'
			UNION ALL
			SELECT $1::uuid, id, 'DEPARTMENT', $2::uuid, now() + interval '1 month', NULL
			FROM iam.roles WHERE code = 'INSTRUCTOR'`, otherID, deptID)
		if err != nil {
			t.Fatal(err)
		}

		grants, err := repo.Grants(ctx, otherID, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if len(grants) != 0 {
			t.Errorf("geçerli olmayan atamalardan %d yetki geldi", len(grants))
		}

		// Bir yıldan biraz daha önce bölüm başkanıydı.
		past, _ := repo.Grants(ctx, otherID, time.Now().AddDate(-1, 0, -1))
		if !slices.ContainsFunc(past, func(g iam.Grant) bool { return g.Permission == "offering:manage" }) {
			t.Error("geçmiş tarihte bölüm başkanlığı yetkisi görünmeliydi")
		}
	})
}

func TestRoleCatalog(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()

	var roles, perms, orphan int
	err := pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM iam.roles),
		       (SELECT count(*) FROM iam.permissions),
		       (SELECT count(*) FROM iam.permissions p
		        WHERE NOT EXISTS (SELECT 1 FROM iam.role_permissions rp WHERE rp.permission_id = p.id))`,
	).Scan(&roles, &perms, &orphan)
	if err != nil {
		t.Fatal(err)
	}
	if roles != 11 || perms < 60 {
		t.Errorf("katalog: %d rol, %d yetki", roles, perms)
	}
	if orphan != 0 {
		t.Errorf("hiçbir role verilmemiş %d yetki var", orphan)
	}

	// Görevler ayrılığı: sistem yöneticisi akademik veriyi değiştiremez.
	var adminAcademic int
	err = pool.QueryRow(ctx, `
		SELECT count(*) FROM iam.role_permissions rp
		JOIN iam.roles r ON r.id = rp.role_id
		JOIN iam.permissions p ON p.id = rp.permission_id
		WHERE r.code = 'SYSTEM_ADMIN'
		  AND p.code IN ('score:enter', 'grade:finalize', 'grade_change:approve', 'registration:override')`,
	).Scan(&adminAcademic)
	if err != nil {
		t.Fatal(err)
	}
	if adminAcademic != 0 {
		t.Errorf("SYSTEM_ADMIN'de %d akademik yazma yetkisi var, olmamalı", adminAcademic)
	}
}

func seedOrg(t *testing.T, pool *pgxpool.Pool) (deptID, facultyID string) {
	t.Helper()
	err := pool.QueryRow(context.Background(), `
		WITH f AS (
			INSERT INTO org.faculties (code, name_tr, name_en, unit_type)
			VALUES ('MUH', 'Mühendislik Fakültesi', 'Faculty of Engineering', 'FACULTY')
			RETURNING id
		), d AS (
			INSERT INTO org.departments (faculty_id, code, name_tr, name_en)
			SELECT id, 'BIL', 'Bilgisayar Mühendisliği', 'Computer Engineering' FROM f
			RETURNING id, faculty_id
		)
		SELECT id, faculty_id FROM d`).Scan(&deptID, &facultyID)
	if err != nil {
		t.Fatal(err)
	}
	return deptID, facultyID
}

func createUser(t *testing.T, pool *pgxpool.Pool, username, email string) string {
	t.Helper()
	ctx := context.Background()
	personID, err := people.NewRepository(pool).CreatePerson(ctx, people.NewPerson{FirstName: "Test", LastName: username})
	if err != nil {
		t.Fatal(err)
	}
	id, err := iam.NewRepository(pool).CreateUser(ctx, iam.NewUser{
		PersonID: personID, Username: username, Email: email, PasswordHash: "$argon2id$test",
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}
