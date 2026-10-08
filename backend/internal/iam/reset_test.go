package iam_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CAPELLAX02/agora/backend/internal/iam"
)

// lastResetLink, outbox'a yazılan son e-postanın bağlantısını ve şablonunu döndürür.
func (e *authEnv) lastResetLink(t *testing.T) (link, template string) {
	t.Helper()
	err := e.pool.QueryRow(context.Background(), `
		SELECT payload->>'link', template_code FROM communication.email_outbox
		ORDER BY created_at DESC, id DESC LIMIT 1`).Scan(&link, &template)
	if err != nil {
		t.Fatal(err)
	}
	return link, template
}

func tokenFromLink(t *testing.T, link string) string {
	t.Helper()
	_, token, ok := strings.Cut(link, "#token=")
	if !ok || token == "" {
		t.Fatalf("bağlantıda token yok: %s", link)
	}
	return token
}

func (e *authEnv) outboxCount(t *testing.T) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), `SELECT count(*) FROM communication.email_outbox`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (e *authEnv) resetEvents(t *testing.T, result string) int {
	t.Helper()
	var n int
	err := e.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM audit.security_events
		WHERE event_type = 'PASSWORD_RESET_REQUESTED' AND details->>'result' = $1`, result).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRequestPasswordReset(t *testing.T) {
	e := newAuthEnv(t)
	ctx := context.Background()
	e.addUser(t, "22290030", pw, e.hasher) // e-postası 22290030@agora.test

	if err := e.auth.RequestPasswordReset(ctx, " 22290030 "); err != nil {
		t.Fatal(err)
	}
	link, template := e.lastResetLink(t)
	if template != "password_reset" || !strings.HasPrefix(link, "http://web.agora.test/sifre-sifirla#token=") {
		t.Errorf("e-posta = %s %s", template, link)
	}
	info, err := e.auth.VerifyResetToken(ctx, tokenFromLink(t, link))
	if err != nil || info.Purpose != iam.PurposeReset || info.ExpiresAt.Sub(e.clock.Now()) != 30*time.Minute {
		t.Errorf("bağlantı = %+v, %v", info, err)
	}

	t.Run("bilinmeyen kullanıcıda hata yok, e-posta yok", func(t *testing.T) {
		before := e.outboxCount(t)
		if err := e.auth.RequestPasswordReset(ctx, "yok@agora.test"); err != nil {
			t.Fatalf("bilinmeyen kullanıcı hata döndürmemeli: %v", err)
		}
		if e.outboxCount(t) != before || e.resetEvents(t, "ignored") != 1 {
			t.Error("bilinmeyen kullanıcıya e-posta gönderilmemeli ama olay yazılmalı")
		}
	})

	t.Run("kısa sürede ikinci istek e-posta göndermez", func(t *testing.T) {
		before := e.outboxCount(t)
		if err := e.auth.RequestPasswordReset(ctx, "22290030@AGORA.test"); err != nil {
			t.Fatal(err)
		}
		if e.outboxCount(t) != before {
			t.Error("2 dk içinde ikinci e-posta gönderildi")
		}
	})

	t.Run("yeni istek eski bağlantıyı geçersiz kılar", func(t *testing.T) {
		e.clock.Advance(3 * time.Minute)
		if err := e.auth.RequestPasswordReset(ctx, "22290030"); err != nil {
			t.Fatal(err)
		}
		newLink, _ := e.lastResetLink(t)
		if newLink == link {
			t.Fatal("yeni bağlantı üretilmedi")
		}
		if _, err := e.auth.VerifyResetToken(ctx, tokenFromLink(t, link)); !errors.Is(err, iam.ErrInvalidResetToken) {
			t.Errorf("eski bağlantı hâlâ geçerli: %v", err)
		}
		if _, err := e.auth.VerifyResetToken(ctx, tokenFromLink(t, newLink)); err != nil {
			t.Errorf("yeni bağlantı geçersiz: %v", err)
		}
	})

	t.Run("askıdaki hesaba bağlantı gönderilmez", func(t *testing.T) {
		id := e.addUser(t, "22290031", pw, e.hasher)
		if _, err := e.pool.Exec(ctx, `UPDATE iam.users SET status = 'SUSPENDED' WHERE id = $1`, id); err != nil {
			t.Fatal(err)
		}
		before := e.outboxCount(t)
		_ = e.auth.RequestPasswordReset(ctx, "22290031")
		if e.outboxCount(t) != before {
			t.Error("askıdaki hesaba e-posta gönderildi")
		}
	})
}

func TestResetPassword(t *testing.T) {
	e := newAuthEnv(t)
	ctx := context.Background()
	userID := e.addUser(t, "22290032", pw, e.hasher)

	session, _ := e.login("22290032", pw)
	for range 5 { // hesabı kilitle: sıfırlama kilidi de kaldırmalı
		_, _ = e.login("22290032", "yanlış parola 123")
	}

	if err := e.auth.RequestPasswordReset(ctx, "22290032"); err != nil {
		t.Fatal(err)
	}
	link, _ := e.lastResetLink(t)
	token := tokenFromLink(t, link)

	var pe *iam.PolicyError
	if err := e.auth.ResetPassword(ctx, token, "kisa"); !errors.As(err, &pe) {
		t.Fatalf("politika ihlali bekleniyordu: %v", err)
	}
	if _, err := e.auth.VerifyResetToken(ctx, token); err != nil {
		t.Fatalf("politika hatası bağlantıyı tüketmemeli: %v", err)
	}

	if err := e.auth.ResetPassword(ctx, token, newPW); err != nil {
		t.Fatal(err)
	}

	if _, err := e.login("22290032", newPW); err != nil {
		t.Errorf("yeni parolayla giriş (kilit de kalkmalıydı): %v", err)
	}
	if reason := e.sessionRevokeReason(t, session.SessionID); reason != "PASSWORD_RESET" {
		t.Errorf("eski oturum sebebi = %q, want PASSWORD_RESET", reason)
	}
	if !e.revoker.has(session.SessionID) {
		t.Error("eski oturum iptal listesine eklenmedi")
	}
	if err := e.auth.ResetPassword(ctx, token, "başka uzun bir parola 7"); !errors.Is(err, iam.ErrInvalidResetToken) {
		t.Errorf("bağlantı ikinci kez kullanılabildi: %v", err)
	}
	if got := e.eventCounts(t, userID)["PASSWORD_RESET_COMPLETED"]; got != 1 {
		t.Errorf("PASSWORD_RESET_COMPLETED olayı %d kez", got)
	}
}

func TestResetPasswordExpiredAndInvalid(t *testing.T) {
	e := newAuthEnv(t)
	ctx := context.Background()
	e.addUser(t, "22290033", pw, e.hasher)
	_ = e.auth.RequestPasswordReset(ctx, "22290033")
	link, _ := e.lastResetLink(t)

	e.clock.Advance(31 * time.Minute)
	if err := e.auth.ResetPassword(ctx, tokenFromLink(t, link), newPW); !errors.Is(err, iam.ErrInvalidResetToken) {
		t.Errorf("süresi dolmuş bağlantı: %v", err)
	}
	for _, bad := range []string{"", "uydurma-token"} {
		if _, err := e.auth.VerifyResetToken(ctx, bad); !errors.Is(err, iam.ErrInvalidResetToken) {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

// TestResetActivatesPendingAccount, PENDING bir hesabın bağlantıyla parola
// belirleyince etkinleştiğini doğrular.
func TestResetActivatesPendingAccount(t *testing.T) {
	e := newAuthEnv(t)
	ctx := context.Background()
	userID := e.addUser(t, "P10050", pw, e.hasher)
	if _, err := e.pool.Exec(ctx, `UPDATE iam.users SET status = 'PENDING' WHERE id = $1`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.login("P10050", pw); !errors.Is(err, iam.ErrAccountDisabled) {
		t.Fatalf("PENDING hesap giriş yapamamalı: %v", err)
	}

	_ = e.auth.RequestPasswordReset(ctx, "P10050")
	link, _ := e.lastResetLink(t)
	if err := e.auth.ResetPassword(ctx, tokenFromLink(t, link), newPW); err != nil {
		t.Fatal(err)
	}
	if u := e.user(t, userID); u.Status != iam.StatusActive {
		t.Errorf("durum = %s, ACTIVE olmalı", u.Status)
	}
}

// TestConcurrentReset, aynı bağlantıyla eşzamanlı isteklerden sadece birinin
// parolayı değiştirdiğini doğrular.
func TestConcurrentReset(t *testing.T) {
	e := newAuthEnv(t)
	ctx := context.Background()
	e.addUser(t, "22290034", pw, e.hasher)
	_ = e.auth.RequestPasswordReset(ctx, "22290034")
	link, _ := e.lastResetLink(t)
	token := tokenFromLink(t, link)

	var (
		wg        sync.WaitGroup
		successes atomic.Int32
		start     = make(chan struct{})
	)
	for i := range 5 {
		wg.Go(func() {
			<-start
			err := e.auth.ResetPassword(ctx, token, newPW+strings.Repeat("!", i))
			if err == nil {
				successes.Add(1)
			} else if !errors.Is(err, iam.ErrInvalidResetToken) {
				t.Errorf("beklenmeyen hata: %v", err)
			}
		})
	}
	close(start)
	wg.Wait()

	if got := successes.Load(); got != 1 {
		t.Errorf("%d istek başarılı oldu, tam olarak 1 olmalıydı", got)
	}
}
