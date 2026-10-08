package iam_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/iam"
)

func (e *authEnv) permVersion(t *testing.T, userID string) int {
	t.Helper()
	return e.user(t, userID).PermVersion
}

func (e *authEnv) assignments(t *testing.T, userID string) []iam.Assignment {
	t.Helper()
	list, err := iam.NewRepository(e.pool).UserAssignments(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	return list
}

func TestRolesWithPermissions(t *testing.T) {
	e := newAuthEnv(t)
	roles, err := iam.NewRepository(e.pool).Roles(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(roles, func(r iam.RoleDef) bool { return r.Code == "SYSTEM_ADMIN" })
	if i == -1 || roles[i].ScopeType != iam.ScopeUniversity || !slices.Contains(roles[i].Permissions, "role:assign") {
		t.Errorf("SYSTEM_ADMIN = %+v", roles)
	}
	if !slices.IsSortedFunc(roles, func(a, b iam.RoleDef) int { return compare(a.Code, b.Code) }) {
		t.Error("roller koda göre sıralı değil")
	}
}

func compare(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func TestAssignRole(t *testing.T) {
	e := newAuthEnv(t)
	ctx := context.Background()
	deptID, facultyID := seedOrg(t, e.pool)
	userID := e.addUser(t, "P10060", pw, e.hasher)
	actor := e.addUser(t, "P90060", pw, e.hasher)
	v := e.permVersion(t, userID)

	id, err := e.auth.AssignRole(ctx, actor, userID, iam.AssignInput{
		Role: "DEPARTMENT_HEAD", ScopeID: deptID, Reason: "Yönetim kurulu kararı 2026/14",
	})
	if err != nil {
		t.Fatal(err)
	}
	if e.permVersion(t, userID) != v+1 {
		t.Error("yetki sürümü artmadı: önbellekteki yetkiler eskide kalırdı")
	}
	list := e.assignments(t, userID)
	if len(list) != 1 || list[0].ID != id || list[0].State != iam.AssignmentActive ||
		list[0].AssignedBy != actor || list[0].ScopeName != "Bilgisayar Mühendisliği" {
		t.Errorf("atama = %+v", list)
	}
	if e.auditCount(t, "role.assign", userID) != 1 {
		t.Error("denetim kaydı yok")
	}

	future := time.Now().Add(30 * 24 * time.Hour)
	until := future.Add(-time.Hour)
	tests := []struct {
		name string
		in   iam.AssignInput
		want error
	}{
		{"olmayan rol", iam.AssignInput{Role: "UYDURMA", Reason: "x"}, iam.ErrUnknownRole},
		{"bölüm rolü kapsamsız", iam.AssignInput{Role: "INSTRUCTOR", Reason: "x"}, iam.ErrInvalidScope},
		{"üniversite rolü kapsamlı", iam.AssignInput{Role: "CENTRAL_REGISTRAR", ScopeID: deptID, Reason: "x"}, iam.ErrInvalidScope},
		{"bölüm rolüne fakülte kimliği", iam.AssignInput{Role: "INSTRUCTOR", ScopeID: facultyID, Reason: "x"}, iam.ErrUnknownScope},
		{"çakışan dönem", iam.AssignInput{Role: "DEPARTMENT_HEAD", ScopeID: deptID, Reason: "x"}, iam.ErrConflict},
		{"bitiş başlangıçtan önce", iam.AssignInput{Role: "INSTRUCTOR", ScopeID: deptID, ValidFrom: &future, ValidUntil: &until, Reason: "x"}, iam.ErrInvalidPeriod},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := e.auth.AssignRole(ctx, actor, userID, tt.in); !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}

	t.Run("kendine rol atama", func(t *testing.T) {
		_, err := e.auth.AssignRole(ctx, actor, actor, iam.AssignInput{Role: "SYSTEM_ADMIN", Reason: "x"})
		if !errors.Is(err, iam.ErrSelfAction) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("ileri tarihli atama", func(t *testing.T) {
		if _, err := e.auth.AssignRole(ctx, actor, userID, iam.AssignInput{
			Role: "INSTRUCTOR", ScopeID: deptID, ValidFrom: &future, Reason: "gelecek dönem",
		}); err != nil {
			t.Fatal(err)
		}
		list := e.assignments(t, userID)
		if list[0].State != iam.AssignmentUpcoming {
			t.Errorf("durum = %s, UPCOMING olmalı", list[0].State)
		}
	})
}

func TestEndAssignment(t *testing.T) {
	e := newAuthEnv(t)
	ctx := context.Background()
	deptID, _ := seedOrg(t, e.pool)
	userID := e.addUser(t, "P10061", pw, e.hasher)
	actor := e.addUser(t, "P90061", pw, e.hasher)

	activeID, _ := e.auth.AssignRole(ctx, actor, userID, iam.AssignInput{Role: "INSTRUCTOR", ScopeID: deptID, Reason: "x"})
	future := time.Now().Add(24 * time.Hour)
	upcomingID, _ := e.auth.AssignRole(ctx, actor, userID, iam.AssignInput{Role: "ADVISOR", ScopeID: deptID, ValidFrom: &future, Reason: "x"})

	v := e.permVersion(t, userID)
	if err := e.auth.EndAssignment(ctx, actor, userID, activeID, "emekli oldu"); err != nil {
		t.Fatal(err)
	}
	if err := e.auth.EndAssignment(ctx, actor, userID, upcomingID, "vazgeçildi"); err != nil {
		t.Fatal(err)
	}
	if e.permVersion(t, userID) != v+2 {
		t.Error("her sonlandırmada yetki sürümü artmalı")
	}

	list := e.assignments(t, userID)
	if len(list) != 1 || list[0].ID != activeID || list[0].State != iam.AssignmentEnded {
		t.Errorf("başlamış atama bitmiş olarak kalmalı, başlamamış olan silinmeli: %+v", list)
	}
	if e.auditCount(t, "role.end", userID) != 1 || e.auditCount(t, "role.cancel", userID) != 1 {
		t.Error("denetim kayıtları eksik")
	}

	// Bitmiş atamayı tekrar sonlandırmak bir şey yapmaz.
	if err := e.auth.EndAssignment(ctx, actor, userID, activeID, "x"); err != nil {
		t.Errorf("tekrar sonlandırma: %v", err)
	}
	// Başka kullanıcının ataması bu kullanıcı üzerinden sonlandırılamaz.
	otherID := e.addUser(t, "P10062", pw, e.hasher)
	if err := e.auth.EndAssignment(ctx, actor, otherID, activeID, "x"); !errors.Is(err, iam.ErrRoleAssignmentNotFound) {
		t.Errorf("err = %v", err)
	}
}

// TestLastSystemAdmin, son aktif sistem yöneticisinin rolünün kaldırılamadığını ve
// iki yöneticinin aynı anda birbirinin rolünü kaldıramadığını doğrular.
func TestLastSystemAdmin(t *testing.T) {
	e := newAuthEnv(t)
	ctx := context.Background()
	repo := iam.NewRepository(e.pool)

	a := e.addUser(t, "P90070", pw, e.hasher)
	b := e.addUser(t, "P90071", pw, e.hasher)
	for _, id := range []string{a, b} {
		if err := repo.AssignRole(ctx, id, "SYSTEM_ADMIN", iam.ScopeUniversity, ""); err != nil {
			t.Fatal(err)
		}
	}
	adminAssignment := func(userID string) string {
		return e.assignments(t, userID)[0].ID
	}
	aAssign, bAssign := adminAssignment(a), adminAssignment(b)
	warmPool(t, e)

	var (
		wg        sync.WaitGroup
		successes atomic.Int32
		start     = make(chan struct{})
	)
	for _, pair := range [][3]string{{a, b, bAssign}, {b, a, aAssign}} {
		wg.Go(func() {
			<-start
			err := e.auth.EndAssignment(ctx, pair[0], pair[1], pair[2], "karşılıklı")
			if err == nil {
				successes.Add(1)
			} else if !errors.Is(err, iam.ErrLastSystemAdmin) {
				t.Errorf("beklenmeyen hata: %v", err)
			}
		})
	}
	close(start)
	wg.Wait()

	if got := successes.Load(); got != 1 {
		t.Fatalf("%d sonlandırma başarılı oldu, tam olarak 1 olmalı: sistem yöneticisiz kalabilirdi", got)
	}
}

// warmPool, havuzdaki bütün bağlantıları önceden açar. Soğuk havuzda goroutine'ler
// bağlantı kurmayı beklerken ilki işini bitirir ve test edilmek istenen yarış hiç
// yaşanmaz: test, koruma olmasa bile geçerdi.
func warmPool(t *testing.T, e *authEnv) {
	t.Helper()
	n := int(e.pool.Config().MaxConns)
	conns := make([]*pgxpool.Conn, 0, n)
	for range n {
		c, err := e.pool.Acquire(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		conns = append(conns, c)
	}
	for _, c := range conns {
		c.Release()
	}
}
