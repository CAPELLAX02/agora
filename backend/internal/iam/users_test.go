package iam_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/CAPELLAX02/agora/backend/internal/iam"
	"github.com/CAPELLAX02/agora/backend/internal/people"
)

const adminID = "01a11b7f-0000-7000-8000-0000000000ad"

func newStaffAccount(number, email string) iam.NewAccount {
	return iam.NewAccount{
		Kind: iam.KindStaff, Number: number, FirstName: "Zeynep", LastName: "Arslan", Email: email,
		StaffType: people.StaffAcademic, AcademicTitle: "ASSIST_PROF",
	}
}

func (e *authEnv) auditCount(t *testing.T, action, entityID string) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit.audit_log WHERE action = $1 AND entity_id = $2`, action, entityID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCreateAccountAndActivate(t *testing.T) {
	e := newAuthEnv(t)
	ctx := context.Background()

	userID, err := e.auth.CreateAccount(ctx, adminID, newStaffAccount("P30001", "zeynep.arslan@agora.test"))
	if err != nil {
		t.Fatal(err)
	}
	if u := e.user(t, userID); u.Status != iam.StatusPending || u.Username != "P30001" {
		t.Errorf("hesap = %+v", u)
	}
	if e.auditCount(t, "user.create", userID) != 1 {
		t.Error("denetim kaydı yazılmadı")
	}

	link, template := e.lastResetLink(t)
	if template != "account_activation" {
		t.Fatalf("şablon = %s", template)
	}
	info, err := e.auth.VerifyResetToken(ctx, tokenFromLink(t, link))
	if err != nil || info.Purpose != iam.PurposeActivation || info.ExpiresAt.Sub(e.clock.Now()) != 72*time.Hour {
		t.Errorf("aktivasyon bağlantısı = %+v, %v", info, err)
	}

	// Aktivasyondan önce giriş yapılamaz: parola kimsenin bilmediği rastgele bir değer.
	if _, err := e.login("P30001", pw); !errors.Is(err, iam.ErrInvalidCredentials) {
		t.Errorf("aktivasyon öncesi giriş: %v", err)
	}

	if err := e.auth.ResetPassword(ctx, tokenFromLink(t, link), newPW); err != nil {
		t.Fatal(err)
	}
	if _, err := e.login("P30001", newPW); err != nil {
		t.Errorf("aktivasyon sonrası giriş: %v", err)
	}
}

func TestCreateAccountRejects(t *testing.T) {
	e := newAuthEnv(t)
	ctx := context.Background()
	if _, err := e.auth.CreateAccount(ctx, adminID, newStaffAccount("P30002", "a@agora.test")); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		in   iam.NewAccount
		want error
	}{
		{"aynı numara", newStaffAccount("P30002", "b@agora.test"), iam.ErrConflict},
		{"aynı e-posta", newStaffAccount("P30003", "A@agora.test"), iam.ErrConflict},
		{"olmayan bölüm", func() iam.NewAccount {
			in := newStaffAccount("P30004", "c@agora.test")
			in.DepartmentID = "01a11b7f-0000-7000-8000-000000000000"
			return in
		}(), iam.ErrUnknownDepartment},
		{"olmayan unvan", func() iam.NewAccount {
			in := newStaffAccount("P30005", "d@agora.test")
			in.AcademicTitle = "UYDURMA"
			return in
		}(), iam.ErrUnknownAcademicTitle},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := e.auth.CreateAccount(ctx, adminID, tt.in); !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}

	// Başarısız denemeler arkalarında kişi ya da e-posta bırakmamalı (transaction).
	var persons, emails int
	_ = e.pool.QueryRow(ctx, `SELECT count(*) FROM people.persons`).Scan(&persons)
	_ = e.pool.QueryRow(ctx, `SELECT count(*) FROM communication.email_outbox`).Scan(&emails)
	if persons != 1 || emails != 1 {
		t.Errorf("geri alınmayan kayıtlar: %d kişi, %d e-posta", persons, emails)
	}
}

func TestSetStatus(t *testing.T) {
	e := newAuthEnv(t)
	ctx := context.Background()
	userID := e.addUser(t, "22290070", pw, e.hasher)
	session, _ := e.login("22290070", pw)

	if err := e.auth.SetStatus(ctx, adminID, userID, iam.StatusSuspended, "disiplin soruşturması"); err != nil {
		t.Fatal(err)
	}
	if reason := e.sessionRevokeReason(t, session.SessionID); reason != "ADMIN" || !e.revoker.has(session.SessionID) {
		t.Errorf("askıya alınan hesabın oturumu kapanmalı: %q", reason)
	}
	if _, err := e.login("22290070", pw); !errors.Is(err, iam.ErrAccountDisabled) {
		t.Errorf("askıdaki hesap girişi: %v", err)
	}
	if e.auditCount(t, "user.status_change", userID) != 1 {
		t.Error("denetim kaydı yok")
	}

	if err := e.auth.SetStatus(ctx, adminID, userID, iam.StatusActive, "soruşturma kapandı"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.login("22290070", pw); err != nil {
		t.Errorf("yeniden etkinleştirilen hesap: %v", err)
	}

	t.Run("kendi hesabı", func(t *testing.T) {
		if err := e.auth.SetStatus(ctx, userID, userID, iam.StatusDisabled, "x"); !errors.Is(err, iam.ErrSelfAction) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("PENDING elle etkinleştirilemez", func(t *testing.T) {
		pendingID, _ := e.auth.CreateAccount(ctx, adminID, newStaffAccount("P30010", "p@agora.test"))
		if err := e.auth.SetStatus(ctx, adminID, pendingID, iam.StatusActive, "x"); !errors.Is(err, iam.ErrInvalidStatusChange) {
			t.Errorf("err = %v", err)
		}
		if err := e.auth.ResendActivation(ctx, adminID, pendingID); err != nil {
			t.Errorf("aktivasyon e-postası: %v", err)
		}
		if err := e.auth.SendPasswordReset(ctx, adminID, pendingID); !errors.Is(err, iam.ErrAccountNotActive) {
			t.Errorf("PENDING hesaba sıfırlama: %v", err)
		}
	})
	t.Run("olmayan hesap", func(t *testing.T) {
		if err := e.auth.SetStatus(ctx, adminID, "01a11b7f-0000-7000-8000-000000000000", iam.StatusDisabled, "x"); !errors.Is(err, iam.ErrNotFound) {
			t.Errorf("err = %v", err)
		}
	})
}

func TestListUsers(t *testing.T) {
	e := newAuthEnv(t)
	ctx := context.Background()
	for _, n := range []string{"22290080", "22290081", "22290082"} {
		e.addUser(t, n, pw, e.hasher)
	}
	pendingID, _ := e.auth.CreateAccount(ctx, adminID, newStaffAccount("P30020", "zeynep@agora.test"))
	repo := iam.NewRepository(e.pool)

	all, hasMore, err := repo.ListUsers(ctx, iam.UserFilter{Limit: 2})
	if err != nil || len(all) != 2 || !hasMore || all[0].Username != "22290080" {
		t.Fatalf("ilk sayfa = %+v, %v, %v", all, hasMore, err)
	}
	next, hasMore, _ := repo.ListUsers(ctx, iam.UserFilter{Limit: 2, After: &iam.UserCursor{Username: all[1].Username, ID: all[1].ID}})
	if len(next) != 2 || hasMore || next[1].ID != pendingID {
		t.Errorf("ikinci sayfa = %+v", next)
	}

	staff, _, _ := repo.ListUsers(ctx, iam.UserFilter{Kind: iam.KindStaff, Limit: 10})
	if len(staff) != 1 || staff[0].Kind != iam.KindStaff || staff[0].Status != iam.StatusPending {
		t.Errorf("personel = %+v", staff)
	}
	byName, _, _ := repo.ListUsers(ctx, iam.UserFilter{Query: "zeynep arslan", Limit: 10})
	if len(byName) != 1 {
		t.Errorf("ad soyadla arama = %+v", byName)
	}
	pending, _, _ := repo.ListUsers(ctx, iam.UserFilter{Status: iam.StatusPending, Limit: 10})
	if len(pending) != 1 {
		t.Errorf("PENDING filtresi = %+v", pending)
	}
}
