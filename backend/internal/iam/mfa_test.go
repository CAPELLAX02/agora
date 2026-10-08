package iam_test

import (
	"bytes"
	"context"
	"encoding/base32"
	"errors"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/iam"
	"github.com/CAPELLAX02/agora/backend/internal/iam/totp"
)

func decodeSecret(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// codeAt, sırrın verilen andan offset adım sonraki kodudur.
func codeAt(secret []byte, at time.Time, offset int) string {
	return totp.Code(secret, uint64(int64(totp.Step(at))+int64(offset)), totp.Digits)
}

// enableMFA, kullanıcıya servis üzerinden MFA kurar ve sırrı ile kurtarma kodlarını
// döndürür. Etkinleştirme o anki adımı kullanır: aynı adımın kodu girişte tekrar
// kabul edilmez, bu yüzden saat bir adım ilerletilir.
func (e *authEnv) enableMFA(t *testing.T, userID, sessionID string) ([]byte, []string) {
	t.Helper()
	ctx := context.Background()
	setup, err := e.auth.StartMFASetup(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	secret := decodeSecret(t, setup.Secret)
	codes, err := e.auth.EnableMFA(ctx, iam.EnableMFAInput{
		UserID: userID, SessionID: sessionID, CurrentPassword: pw, Code: codeAt(secret, e.clock.Now(), 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(totp.Period)
	return secret, codes
}

// startMFALogin, parolayla girişi yapar ve ikinci adımın token'ını döndürür.
func (e *authEnv) startMFALogin(t *testing.T, username string) string {
	t.Helper()
	_, err := e.login(username, pw)
	var mfa *iam.MFARequiredError
	if !errors.As(err, &mfa) {
		t.Fatalf("MFA bekleniyordu, hata = %v", err)
	}
	return mfa.Token
}

func (e *authEnv) verifyMFA(token, code, recovery string) (iam.Tokens, error) {
	return e.auth.VerifyMFA(context.Background(), iam.MFAVerifyInput{
		Token: token, Code: code, RecoveryCode: recovery, Client: iam.ClientWeb, IP: "203.0.113.7", UserAgent: "test",
	})
}

func TestMFAEnableAndLogin(t *testing.T) {
	e := newAuthEnv(t)
	ctx := context.Background()
	userID := e.addUser(t, "P90010", pw, e.hasher)
	first, err := e.login("P90010", pw)
	if err != nil {
		t.Fatal(err)
	}
	other, _ := e.login("P90010", pw)

	if _, err := e.auth.EnableMFA(ctx, iam.EnableMFAInput{UserID: userID, SessionID: first.SessionID, CurrentPassword: pw, Code: "123456"}); !errors.Is(err, iam.ErrMFASetupRequired) {
		t.Errorf("kurulumsuz etkinleştirme: %v", err)
	}

	setup, err := e.auth.StartMFASetup(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(setup.URI, "otpauth://totp/Agora:P90010?") || len(setup.Secret) != 32 {
		t.Errorf("kurulum = %+v", setup)
	}
	secret := decodeSecret(t, setup.Secret)

	// Sır veritabanında şifreli durur ve başka bir kullanıcının satırında açılamaz.
	stored := e.user(t, userID).MFASecretEnc
	if len(stored) == 0 || bytes.Contains(stored, secret) {
		t.Fatalf("sır şifreli saklanmalı: %x", stored)
	}

	in := iam.EnableMFAInput{UserID: userID, SessionID: first.SessionID, CurrentPassword: "yanlış parola", Code: codeAt(secret, e.clock.Now(), 0)}
	if _, err := e.auth.EnableMFA(ctx, in); !errors.Is(err, iam.ErrInvalidCurrentPassword) {
		t.Errorf("yanlış parola: %v", err)
	}
	in.CurrentPassword, in.Code = pw, codeAt(secret, e.clock.Now(), 3)
	if _, err := e.auth.EnableMFA(ctx, in); !errors.Is(err, iam.ErrInvalidMFACode) {
		t.Errorf("yanlış kod: %v", err)
	}
	in.Code = codeAt(secret, e.clock.Now(), 0)
	codes, err := e.auth.EnableMFA(ctx, in)
	if err != nil {
		t.Fatal(err)
	}

	pattern := regexp.MustCompile(`^[a-km-np-z2-9]{5}-[a-km-np-z2-9]{5}$`)
	if len(codes) != 10 || len(slices.Compact(slices.Sorted(slices.Values(codes)))) != 10 {
		t.Errorf("10 farklı kurtarma kodu bekleniyordu: %v", codes)
	}
	for _, c := range codes {
		if !pattern.MatchString(c) {
			t.Errorf("kurtarma kodu biçimi: %q", c)
		}
	}
	if _, err := e.auth.StartMFASetup(ctx, userID); !errors.Is(err, iam.ErrMFAAlreadyEnabled) {
		t.Errorf("açıkken yeni kurulum: %v", err)
	}

	// Diğer oturum kapandı, isteği yapan oturum açık kaldı ve MFA'lı sayılıyor.
	if !e.revoker.has(other.SessionID) || e.sessionRevokeReason(t, other.SessionID) != "MFA_CHANGE" {
		t.Error("diğer oturum MFA_CHANGE sebebiyle kapatılmalı")
	}
	refreshed, err := e.auth.Refresh(ctx, first.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	claims, _ := e.verifier.Verify(refreshed.AccessToken, e.clock.Now())
	if !slices.Equal(claims.AMR, []string{"pwd", "otp"}) {
		t.Errorf("etkinleştiren oturumun AMR'si = %v", claims.AMR)
	}

	// Giriş artık iki adımlı. Etkinleştirmede kullanılan kod tekrar kabul edilmez.
	token := e.startMFALogin(t, "P90010")
	if _, err := e.verifyMFA(token, in.Code, ""); !errors.Is(err, iam.ErrInvalidMFACode) {
		t.Errorf("kullanılmış kod: %v", err)
	}
	e.clock.Advance(totp.Period)
	code := codeAt(secret, e.clock.Now(), 0)
	tok, err := e.verifyMFA(token, code, "")
	if err != nil {
		t.Fatal(err)
	}
	claims, _ = e.verifier.Verify(tok.AccessToken, e.clock.Now())
	if claims.Subject != userID || !slices.Equal(claims.AMR, []string{"pwd", "otp"}) {
		t.Errorf("claim'ler = %+v", claims)
	}
	// Başarılı giriş, ikinci adımdaki yanlış denemenin sayacını sıfırladı.
	if u := e.user(t, userID); u.FailedLoginCount != 0 {
		t.Errorf("sayaç = %d", u.FailedLoginCount)
	}

	// Aynı token ve aynı kod bir daha kullanılamaz.
	if _, err := e.verifyMFA(token, code, ""); !errors.Is(err, iam.ErrInvalidMFAToken) {
		t.Errorf("kullanılmış token: %v", err)
	}
	token = e.startMFALogin(t, "P90010")
	if _, err := e.verifyMFA(token, code, ""); !errors.Is(err, iam.ErrInvalidMFACode) {
		t.Errorf("aynı adımın kodu ikinci girişte: %v", err)
	}

	events := e.eventCounts(t, userID)
	if events[audit.EventMFAEnabled] != 1 || events[audit.EventMFAChallengeStarted] != 2 {
		t.Errorf("olaylar = %v", events)
	}
}

func TestMFAChallengeRules(t *testing.T) {
	e := newAuthEnv(t)
	userID := e.addUser(t, "P90011", pw, e.hasher)
	tok, _ := e.login("P90011", pw)
	secret, _ := e.enableMFA(t, userID, tok.SessionID)

	t.Run("süresi dolan token", func(t *testing.T) {
		token := e.startMFALogin(t, "P90011")
		e.clock.Advance(5*time.Minute + time.Second)
		if _, err := e.verifyMFA(token, codeAt(secret, e.clock.Now(), 0), ""); !errors.Is(err, iam.ErrInvalidMFAToken) {
			t.Errorf("hata = %v", err)
		}
	})

	t.Run("başka istemci türü", func(t *testing.T) {
		token := e.startMFALogin(t, "P90011")
		_, err := e.auth.VerifyMFA(context.Background(), iam.MFAVerifyInput{
			Token: token, Code: codeAt(secret, e.clock.Now(), 0), Client: iam.ClientMobile,
		})
		if !errors.Is(err, iam.ErrInvalidMFAToken) {
			t.Errorf("hata = %v", err)
		}
	})

	t.Run("bilinmeyen token", func(t *testing.T) {
		if _, err := e.verifyMFA("uydurma", "123456", ""); !errors.Is(err, iam.ErrInvalidMFAToken) {
			t.Errorf("hata = %v", err)
		}
	})

	t.Run("yanlış kodlar hesabı kilitler", func(t *testing.T) {
		e.clock.Advance(time.Hour)
		token := e.startMFALogin(t, "P90011")
		wrong := codeAt(secret, e.clock.Now(), 5)
		for i := range 4 {
			if _, err := e.verifyMFA(token, wrong, ""); !errors.Is(err, iam.ErrInvalidMFACode) {
				t.Fatalf("deneme %d: %v", i+1, err)
			}
		}
		_, err := e.verifyMFA(token, wrong, "")
		var locked *iam.LockedError
		if !errors.As(err, &locked) {
			t.Fatalf("beşinci yanlış kod kilitlemeli: %v", err)
		}
		// Deneme hakkı biten token, kilit kalksa bile kullanılamaz.
		e.clock.Advance(2 * time.Minute)
		if _, err := e.verifyMFA(token, codeAt(secret, e.clock.Now(), 0), ""); !errors.Is(err, iam.ErrInvalidMFAToken) {
			t.Errorf("hakkı biten token: %v", err)
		}
		if n := e.eventCounts(t, userID)[audit.EventAccountLocked]; n != 1 {
			t.Errorf("ACCOUNT_LOCKED = %d", n)
		}
	})

	t.Run("askıya alınan hesap", func(t *testing.T) {
		e.clock.Advance(time.Hour)
		token := e.startMFALogin(t, "P90011")
		if _, err := e.pool.Exec(context.Background(), `UPDATE iam.users SET status = 'SUSPENDED' WHERE id = $1`, userID); err != nil {
			t.Fatal(err)
		}
		if _, err := e.verifyMFA(token, codeAt(secret, e.clock.Now(), 0), ""); !errors.Is(err, iam.ErrAccountDisabled) {
			t.Errorf("hata = %v", err)
		}
	})
}

func TestMFARecoveryCodes(t *testing.T) {
	e := newAuthEnv(t)
	ctx := context.Background()
	m := &countingMetrics{counts: map[string]int{}}
	e.auth.SetMetrics(m)
	userID := e.addUser(t, "22290011", pw, e.hasher)
	tok, _ := e.login("22290011", pw)
	secret, codes := e.enableMFA(t, userID, tok.SessionID)

	// Büyük harf ve boşluk önemsiz.
	token := e.startMFALogin(t, "22290011")
	typed := strings.ToUpper(strings.ReplaceAll(codes[0], "-", " - "))
	if _, err := e.verifyMFA(token, "", typed); err != nil {
		t.Fatalf("kurtarma koduyla giriş: %v", err)
	}
	token = e.startMFALogin(t, "22290011")
	if _, err := e.verifyMFA(token, "", codes[0]); !errors.Is(err, iam.ErrInvalidMFACode) {
		t.Errorf("kullanılmış kurtarma kodu: %v", err)
	}
	status, err := e.auth.MFAStatus(ctx, userID)
	if err != nil || !status.Enabled || status.RecoveryCodesRemaining != 9 || status.EnabledAt == nil {
		t.Errorf("durum = %+v, %v", status, err)
	}

	// Yenileme TOTP kodu ister, kurtarma kodu yetmez. Eski kodların hepsi geçersiz olur.
	if _, err := e.auth.RegenerateRecoveryCodes(ctx, userID, "000000"); !errors.Is(err, iam.ErrInvalidMFACode) {
		t.Errorf("yanlış kodla yenileme: %v", err)
	}
	fresh, err := e.auth.RegenerateRecoveryCodes(ctx, userID, codeAt(secret, e.clock.Now(), 0))
	if err != nil || len(fresh) != 10 {
		t.Fatalf("yenileme: %v %v", fresh, err)
	}
	if _, err := e.verifyMFA(token, "", codes[1]); !errors.Is(err, iam.ErrInvalidMFACode) {
		t.Errorf("yenilemeden önceki kod: %v", err)
	}
	if _, err := e.verifyMFA(token, "", fresh[0]); err != nil {
		t.Errorf("yeni kod: %v", err)
	}
	if n := e.eventCounts(t, userID)[audit.EventRecoveryCodesRenewed]; n != 1 {
		t.Errorf("yenileme olayı = %d", n)
	}
	// Reddedilen kurtarma kodu sayılmaz, sadece commit edilen kullanımlar sayılır.
	if m.counts["mfa:recovery_code_used"] != 2 || m.counts["mfa:enabled"] != 1 ||
		m.counts["mfa:recovery_codes_renewed"] != 1 || m.counts["fail:wrong_recovery_code"] != 2 {
		t.Errorf("sayaçlar = %v", m.counts)
	}
}

func TestMFADisable(t *testing.T) {
	e := newAuthEnv(t)
	ctx := context.Background()
	userID := e.addUser(t, "P90012", pw, e.hasher)
	tok, _ := e.login("P90012", pw)
	secret, codes := e.enableMFA(t, userID, tok.SessionID)
	other, err := e.verifyMFA(e.startMFALogin(t, "P90012"), codeAt(secret, e.clock.Now(), 0), "")
	if err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(totp.Period)

	in := iam.DisableMFAInput{UserID: userID, SessionID: tok.SessionID, CurrentPassword: pw, Code: "000000"}
	if err := e.auth.DisableMFA(ctx, in); !errors.Is(err, iam.ErrInvalidMFACode) {
		t.Errorf("yanlış kod: %v", err)
	}
	if u := e.user(t, userID); u.FailedLoginCount != 1 || !u.MFAEnabled {
		t.Errorf("yanlış kod sayılmalı ve MFA açık kalmalı: %+v", u)
	}
	in.Code, in.CurrentPassword = codeAt(secret, e.clock.Now(), 0), "yanlış parola"
	if err := e.auth.DisableMFA(ctx, in); !errors.Is(err, iam.ErrInvalidCurrentPassword) {
		t.Errorf("yanlış parola: %v", err)
	}

	in.CurrentPassword, in.Code, in.RecoveryCode = pw, "", codes[3]
	if err := e.auth.DisableMFA(ctx, in); err != nil {
		t.Fatal(err)
	}
	u := e.user(t, userID)
	if u.MFAEnabled || u.MFASecretEnc != nil || u.MFAEnabledAt != nil {
		t.Errorf("MFA bilgileri silinmeli: %+v", u)
	}
	if !e.revoker.has(other.SessionID) {
		t.Error("diğer oturum kapatılmalı")
	}
	refreshed, err := e.auth.Refresh(ctx, tok.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if claims, _ := e.verifier.Verify(refreshed.AccessToken, e.clock.Now()); !slices.Equal(claims.AMR, []string{"pwd"}) {
		t.Errorf("kapatan oturumun AMR'si = %v", claims.AMR)
	}
	if _, err := e.login("P90012", pw); err != nil {
		t.Errorf("MFA kapandıktan sonra tek adımlı giriş: %v", err)
	}
	if err := e.auth.DisableMFA(ctx, in); !errors.Is(err, iam.ErrMFANotEnabled) {
		t.Errorf("ikinci kapatma: %v", err)
	}
	var left int
	_ = e.pool.QueryRow(ctx, `SELECT count(*) FROM iam.mfa_recovery_codes WHERE user_id = $1`, userID).Scan(&left)
	if left != 0 {
		t.Errorf("%d kurtarma kodu kaldı", left)
	}
}

func TestResetMFA(t *testing.T) {
	e := newAuthEnv(t)
	ctx := context.Background()
	adminID := e.addUser(t, "P90001", pw, e.hasher)
	userID := e.addUser(t, "P90013", pw, e.hasher)
	tok, _ := e.login("P90013", pw)

	if err := e.auth.ResetMFA(ctx, adminID, userID, "telefon kayboldu"); !errors.Is(err, iam.ErrMFANotEnabled) {
		t.Errorf("MFA kapalıyken: %v", err)
	}
	secret, _ := e.enableMFA(t, userID, tok.SessionID)
	pending := e.startMFALogin(t, "P90013")

	if err := e.auth.ResetMFA(ctx, userID, userID, "x"); !errors.Is(err, iam.ErrSelfAction) {
		t.Errorf("kendi MFA'sını sıfırlama: %v", err)
	}
	if err := e.auth.ResetMFA(ctx, adminID, "01a11b7f-0000-7000-8000-000000000000", "x"); !errors.Is(err, iam.ErrNotFound) {
		t.Errorf("olmayan kullanıcı: %v", err)
	}
	if err := e.auth.ResetMFA(ctx, adminID, userID, "telefon kayboldu, kimlik kampüste doğrulandı"); err != nil {
		t.Fatal(err)
	}
	if !e.revoker.has(tok.SessionID) || e.sessionRevokeReason(t, tok.SessionID) != "ADMIN" {
		t.Error("kullanıcının oturumları kapatılmalı")
	}
	// Sıfırlamadan önce başlamış ikinci adım tamamlanamaz.
	if _, err := e.verifyMFA(pending, codeAt(secret, e.clock.Now(), 0), ""); !errors.Is(err, iam.ErrInvalidMFAToken) {
		t.Errorf("sıfırlamadan önceki giriş: %v", err)
	}
	if _, err := e.login("P90013", pw); err != nil {
		t.Errorf("sıfırlamadan sonra giriş: %v", err)
	}
	var actor string
	if err := e.pool.QueryRow(ctx, `SELECT actor_user_id FROM audit.audit_log WHERE action = 'user.mfa_reset' AND entity_id = $1`, userID).Scan(&actor); err != nil || actor != adminID {
		t.Errorf("denetim kaydı: %v %s", err, actor)
	}
}

// TestMFASecretBoundToUser, şifreli sırrın başka bir kullanıcının satırına
// kopyalanınca çözülemediğini doğrular.
func TestMFASecretBoundToUser(t *testing.T) {
	e := newAuthEnv(t)
	ctx := context.Background()
	victim := e.addUser(t, "P90014", pw, e.hasher)
	attacker := e.addUser(t, "P90015", pw, e.hasher)
	tok, _ := e.login("P90014", pw)
	secret, _ := e.enableMFA(t, victim, tok.SessionID)

	if _, err := e.pool.Exec(ctx, `
		UPDATE iam.users SET mfa_enabled = true, mfa_enabled_at = now(),
		       mfa_secret_enc = (SELECT mfa_secret_enc FROM iam.users WHERE id = $1)
		WHERE id = $2`, victim, attacker); err != nil {
		t.Fatal(err)
	}
	token := e.startMFALogin(t, "P90015")
	if _, err := e.verifyMFA(token, codeAt(secret, e.clock.Now(), 0), ""); err == nil || errors.Is(err, iam.ErrInvalidMFACode) {
		t.Errorf("kopyalanmış sır çözülmemeli, hata = %v", err)
	}
}

// --- HTTP ----------------------------------------------------------------------

// mfaBearer, kullanıcıya HTTP üzerinden iki adımlı doğrulama kurar ve MFA'lı bir
// oturumun access token'ını döndürür: MFA gerektiren yönetim uçları için.
func (e *httpEnv) mfaBearer(t *testing.T, username string) map[string]string {
	t.Helper()
	res := e.do(t, nil, "POST", "/api/v1/auth/login", mobile, credentials(username))
	var tok tokenBody
	res.json(t, &tok)
	auth := map[string]string{"Authorization": "Bearer " + tok.AccessToken}

	res = e.do(t, nil, "POST", "/api/v1/me/mfa/setup", auth, nil)
	if res.status != http.StatusOK {
		t.Fatalf("kurulum: %d %s", res.status, res.body)
	}
	var setup struct {
		Secret string `json:"secret"`
	}
	res.json(t, &setup)
	code := codeAt(decodeSecret(t, setup.Secret), time.Now(), 0)
	res = e.do(t, nil, "POST", "/api/v1/me/mfa/enable", auth, map[string]string{"current_password": pw, "code": code})
	if res.status != http.StatusOK {
		t.Fatalf("etkinleştirme: %d %s", res.status, res.body)
	}

	// Etkinleştiren oturum MFA'lı sayılır: yenilenen access token "otp" taşır.
	res = e.do(t, nil, "POST", "/api/v1/auth/refresh", mobile, map[string]string{"refresh_token": tok.RefreshToken})
	if res.status != http.StatusOK {
		t.Fatalf("yenileme: %d %s", res.status, res.body)
	}
	res.json(t, &tok)
	return map[string]string{"Authorization": "Bearer " + tok.AccessToken}
}

func TestHTTPMFAFlow(t *testing.T) {
	e := newHTTPEnv(t)
	ctx := context.Background()
	userID := e.addUser(t, "P90020")
	if err := iam.NewRepository(e.pool).AssignRole(ctx, userID, "SYSTEM_ADMIN", iam.ScopeUniversity, ""); err != nil {
		t.Fatal(err)
	}
	plain := e.bearer(t, "P90020")

	// MFA'sız yönetici: kullanıcı okuyabilir ama hesap oluşturamaz, /me bunu bildirir.
	var me struct {
		MFAEnabled  bool `json:"mfa_enabled"`
		MFARequired bool `json:"mfa_required"`
	}
	e.do(t, nil, "GET", "/api/v1/me", plain, nil).json(t, &me)
	if me.MFAEnabled || !me.MFARequired {
		t.Errorf("/me = %+v", me)
	}
	if res := e.do(t, nil, "GET", "/api/v1/users", plain, nil); res.status != http.StatusOK {
		t.Errorf("user:read MFA gerektirmez: %d", res.status)
	}
	if res := e.do(t, nil, "POST", "/api/v1/users", plain, map[string]string{}); res.status != http.StatusForbidden || res.code(t) != "MFA_REQUIRED" {
		t.Errorf("user:manage: %d %s", res.status, res.body)
	}

	// Kurulum.
	res := e.do(t, nil, "POST", "/api/v1/me/mfa/setup", plain, nil)
	if res.status != http.StatusOK || res.header.Get("Cache-Control") != "no-store" {
		t.Fatalf("kurulum: %d %s", res.status, res.body)
	}
	var setup struct {
		Secret     string `json:"secret"`
		OTPAuthURI string `json:"otpauth_uri"`
	}
	res.json(t, &setup)
	secret := decodeSecret(t, setup.Secret)

	for name, body := range map[string]map[string]string{
		"kod yok":    {"current_password": pw},
		"parola yok": {"code": "123456"},
	} {
		if res := e.do(t, nil, "POST", "/api/v1/me/mfa/enable", plain, body); res.status != http.StatusBadRequest {
			t.Errorf("%s: %d", name, res.status)
		}
	}
	res = e.do(t, nil, "POST", "/api/v1/me/mfa/enable", plain, map[string]string{"current_password": pw, "code": "000000"})
	if res.status != http.StatusBadRequest || res.code(t) != "INVALID_MFA_CODE" {
		t.Errorf("yanlış kod: %d %s", res.status, res.body)
	}
	res = e.do(t, nil, "POST", "/api/v1/me/mfa/enable", plain,
		map[string]string{"current_password": pw, "code": codeAt(secret, time.Now(), 0)})
	if res.status != http.StatusOK {
		t.Fatalf("etkinleştirme: %d %s", res.status, res.body)
	}
	var rc struct {
		RecoveryCodes []string `json:"recovery_codes"`
	}
	res.json(t, &rc)
	if len(rc.RecoveryCodes) != 10 {
		t.Errorf("kurtarma kodları: %s", res.body)
	}
	if res := e.do(t, nil, "POST", "/api/v1/me/mfa/setup", plain, nil); res.status != http.StatusConflict || res.code(t) != "MFA_ALREADY_ENABLED" {
		t.Errorf("ikinci kurulum: %d %s", res.status, res.body)
	}

	// Web girişi: parola doğru, çerez yok, ikinci adım bekleniyor.
	client := e.webClient(t)
	res = e.do(t, client, "POST", "/api/v1/auth/login", web, credentials("P90020"))
	var challenge struct {
		MFARequired bool   `json:"mfa_required"`
		MFAToken    string `json:"mfa_token"`
		ExpiresIn   int    `json:"expires_in"`
		AccessToken string `json:"access_token"`
	}
	res.json(t, &challenge)
	if res.status != http.StatusOK || !challenge.MFARequired || challenge.MFAToken == "" || challenge.AccessToken != "" ||
		challenge.ExpiresIn < 295 || challenge.ExpiresIn > 300 || res.cookie(iam.RefreshCookieName) != nil {
		t.Fatalf("giriş yanıtı: %d %s", res.status, res.body)
	}

	verify := func(body map[string]string) response {
		body["mfa_token"] = challenge.MFAToken
		return e.do(t, client, "POST", "/api/v1/auth/mfa/verify", web, body)
	}
	if res := verify(map[string]string{"code": "123456", "recovery_code": rc.RecoveryCodes[0]}); res.status != http.StatusBadRequest {
		t.Errorf("iki kod birden: %d", res.status)
	}
	if res := verify(map[string]string{"code": "000000"}); res.status != http.StatusUnauthorized || res.code(t) != "INVALID_MFA_CODE" {
		t.Errorf("yanlış kod: %d %s", res.status, res.body)
	}
	res = verify(map[string]string{"recovery_code": rc.RecoveryCodes[0]})
	if res.status != http.StatusOK || res.cookie(iam.RefreshCookieName) == nil {
		t.Fatalf("kurtarma koduyla doğrulama: %d %s", res.status, res.body)
	}
	var tok tokenBody
	res.json(t, &tok)
	strong := map[string]string{"Authorization": "Bearer " + tok.AccessToken}

	e.do(t, nil, "GET", "/api/v1/me", strong, nil).json(t, &me)
	if !me.MFAEnabled || me.MFARequired {
		t.Errorf("MFA'lı oturumda /me = %+v", me)
	}
	var status struct {
		Enabled   bool `json:"enabled"`
		Remaining int  `json:"recovery_codes_remaining"`
	}
	e.do(t, nil, "GET", "/api/v1/me/mfa", strong, nil).json(t, &status)
	if !status.Enabled || status.Remaining != 9 {
		t.Errorf("durum = %+v", status)
	}
	if res := e.do(t, nil, "POST", "/api/v1/users", strong, map[string]string{}); res.status != http.StatusBadRequest {
		t.Errorf("MFA'lı oturumda yönetim ucu doğrulamaya kadar gelmeli: %d %s", res.status, res.body)
	}
	// Etkinleştiren oturum açık kalır (diğerleri kapanır).
	if res := e.do(t, nil, "GET", "/api/v1/me", plain, nil); res.status != http.StatusOK {
		t.Errorf("etkinleştiren oturum açık kalmalı: %d", res.status)
	}

	// Token'sız ve geçersiz token'la ikinci adım.
	if res := e.do(t, nil, "POST", "/api/v1/auth/mfa/verify", web, map[string]string{"code": "123456"}); res.status != http.StatusBadRequest {
		t.Errorf("token yok: %d", res.status)
	}
	if res := e.do(t, nil, "POST", "/api/v1/auth/mfa/verify", web, map[string]string{"mfa_token": "x", "code": "123456"}); res.code(t) != "INVALID_MFA_TOKEN" {
		t.Errorf("geçersiz token: %d %s", res.status, res.body)
	}
}

func TestHTTPResetMFA(t *testing.T) {
	e := newHTTPEnv(t)
	ctx := context.Background()
	repo := iam.NewRepository(e.pool)
	adminID := e.addUser(t, "P90001")
	if err := repo.AssignRole(ctx, adminID, "SYSTEM_ADMIN", iam.ScopeUniversity, ""); err != nil {
		t.Fatal(err)
	}
	userID := e.addUser(t, "P90021")
	admin := e.mfaBearer(t, "P90001")
	user := e.mfaBearer(t, "P90021")

	var profile struct {
		MFAEnabled bool `json:"mfa_enabled"`
	}
	e.do(t, nil, "GET", "/api/v1/users/"+userID, admin, nil).json(t, &profile)
	if !profile.MFAEnabled {
		t.Error("kullanıcı detayında mfa_enabled görünmeli")
	}

	path := "/api/v1/users/" + userID + "/mfa/reset"
	if res := e.do(t, nil, "POST", path, admin, map[string]string{}); res.status != http.StatusBadRequest {
		t.Errorf("gerekçesiz: %d", res.status)
	}
	if res := e.do(t, nil, "POST", path, user, map[string]string{"reason": "x"}); res.status != http.StatusForbidden {
		t.Errorf("yetkisiz: %d", res.status)
	}
	if res := e.do(t, nil, "POST", "/api/v1/users/"+adminID+"/mfa/reset", admin, map[string]string{"reason": "x"}); res.code(t) != "SELF_ACTION_FORBIDDEN" {
		t.Errorf("kendi MFA'sı: %d %s", res.status, res.body)
	}
	if res := e.do(t, nil, "POST", path, admin, map[string]string{"reason": "Telefon kayboldu, kimlik görüldü"}); res.status != http.StatusNoContent {
		t.Fatalf("sıfırlama: %d %s", res.status, res.body)
	}
	if res := e.do(t, nil, "GET", "/api/v1/me", user, nil); res.status != http.StatusUnauthorized {
		t.Errorf("kullanıcının oturumu kapanmalı: %d", res.status)
	}
	if res := e.do(t, nil, "POST", path, admin, map[string]string{"reason": "x"}); res.code(t) != "MFA_NOT_ENABLED" {
		t.Errorf("ikinci sıfırlama: %d %s", res.status, res.body)
	}
}
