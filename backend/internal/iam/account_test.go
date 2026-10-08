package iam_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/iam"
	"github.com/CAPELLAX02/agora/backend/internal/iam/password"
)

const newPW = "yeni ve uzun bir parola 42"

func TestChangePassword(t *testing.T) {
	e := newAuthEnv(t)
	ctx := context.Background()
	userID := e.addUser(t, "22290020", pw, e.hasher)
	if _, err := e.pool.Exec(ctx, `UPDATE iam.users SET must_change_password = true WHERE id = $1`, userID); err != nil {
		t.Fatal(err)
	}

	current, _ := e.login("22290020", pw)
	other, _ := e.login("22290020", pw) // başka bir cihaz

	err := e.auth.ChangePassword(ctx, iam.ChangePasswordInput{
		UserID: userID, SessionID: current.SessionID, CurrentPassword: pw, NewPassword: newPW,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := e.login("22290020", pw); !errors.Is(err, iam.ErrInvalidCredentials) {
		t.Errorf("eski parola hâlâ çalışıyor: %v", err)
	}
	if _, err := e.login("22290020", newPW); err != nil {
		t.Errorf("yeni parolayla giriş: %v", err)
	}
	if e.user(t, userID).MustChangePassword {
		t.Error("parola değiştirme zorunluluğu kalkmalıydı")
	}

	// İsteği yapan oturum açık kalır, diğer cihaz kapatılır ve iptal listesine girer.
	if reason := e.sessionRevokeReason(t, current.SessionID); reason != "" {
		t.Errorf("mevcut oturum kapatılmamalı: %q", reason)
	}
	if reason := e.sessionRevokeReason(t, other.SessionID); reason != "PASSWORD_CHANGE" {
		t.Errorf("diğer oturum sebebi = %q, want PASSWORD_CHANGE", reason)
	}
	if !e.revoker.has(other.SessionID) || e.revoker.has(current.SessionID) {
		t.Error("iptal listesi yanlış")
	}
	if got := e.eventCounts(t, userID)[audit.EventPasswordChanged]; got != 1 {
		t.Errorf("PASSWORD_CHANGED olayı %d kez yazıldı", got)
	}
}

func TestChangePasswordRejects(t *testing.T) {
	e := newAuthEnv(t)
	ctx := context.Background()
	userID := e.addUser(t, "22290021", pw, e.hasher)
	tok, _ := e.login("22290021", pw)

	change := func(current, next string) error {
		return e.auth.ChangePassword(ctx, iam.ChangePasswordInput{
			UserID: userID, SessionID: tok.SessionID, CurrentPassword: current, NewPassword: next,
		})
	}

	policyTests := []struct {
		name string
		next string
		want password.Violation
	}{
		{"kısa", "kisa", password.TooShort},
		{"yaygın", "1234567890", password.TooCommon},
		{"kullanıcı adını içeriyor", "benim numaram 22290021", password.ContainsPersonalInfo},
		{"mevcut parolayla aynı", pw, password.SameAsCurrent},
	}
	for _, tt := range policyTests {
		t.Run(tt.name, func(t *testing.T) {
			var pe *iam.PolicyError
			if err := change(pw, tt.next); !errors.As(err, &pe) || !slices.Contains(pe.Violations, tt.want) {
				t.Errorf("err = %v, %s ihlali bekleniyordu", err, tt.want)
			}
		})
	}

	t.Run("yanlış mevcut parola girişle aynı sayaçla kilitler", func(t *testing.T) {
		for i := 1; i <= 4; i++ {
			if err := change("yanlış mevcut parola", newPW); !errors.Is(err, iam.ErrInvalidCurrentPassword) {
				t.Fatalf("%d. deneme: err = %v", i, err)
			}
		}
		if err := change("yanlış mevcut parola", newPW); !errors.Is(err, iam.ErrAccountLocked) {
			t.Fatalf("5. deneme kilitlemeliydi: %v", err)
		}
		// Kilitliyken doğru parola da işe yaramaz, girişte de aynı kilit geçerli.
		if err := change(pw, newPW); !errors.Is(err, iam.ErrAccountLocked) {
			t.Errorf("kilitliyken: %v", err)
		}
		if _, err := e.login("22290021", pw); !errors.Is(err, iam.ErrAccountLocked) {
			t.Errorf("kilit girişte de geçerli olmalı: %v", err)
		}
	})
}
