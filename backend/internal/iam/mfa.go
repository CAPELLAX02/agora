package iam

import (
	"context"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/iam/totp"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
	"github.com/CAPELLAX02/agora/backend/internal/platform/secretbox"
)

// İki adımlı doğrulama hataları.
var (
	ErrInvalidMFAToken   = errors.New("iam: iki adımlı doğrulama oturumu geçersiz")
	ErrInvalidMFACode    = errors.New("iam: doğrulama kodu hatalı")
	ErrMFAAlreadyEnabled = errors.New("iam: iki adımlı doğrulama zaten açık")
	ErrMFANotEnabled     = errors.New("iam: iki adımlı doğrulama açık değil")
	ErrMFASetupRequired  = errors.New("iam: iki adımlı doğrulama kurulumu başlatılmamış")
)

// MFARequiredError, parolanın doğru olduğunu ama girişin ikinci adımla tamamlanması
// gerektiğini bildirir. Token, ikinci adımda gönderilecek kısa ömürlü değerdir.
type MFARequiredError struct {
	Token     string
	ExpiresAt time.Time
}

func (e *MFARequiredError) Error() string {
	return "iam: giriş için iki adımlı doğrulama gerekli"
}

const (
	mfaMaxAttempts    = 5  // bir giriş denemesinde en fazla kaç kod denenebilir
	recoveryCodeCount = 10 // kurulumda ve yenilemede üretilen kurtarma kodu sayısı

	// recoveryAlphabet, kurtarma kodlarının harfleridir: karışan karakterler (0/o, 1/l)
	// yok. 32 harf, her karakter 5 bit: 10 karakterlik kod 50 bit rastgelelik taşır.
	recoveryAlphabet = "abcdefghijkmnpqrstuvwxyz23456789"
	recoveryLength   = 10

	// Kimlik doğrulama yöntemleri (AMR, RFC 8176) ve güvenlik olaylarındaki adları.
	methodPassword     = "pwd"
	mfaMethodTOTP      = "totp"
	mfaMethodRecovery  = "recovery_code"
	mfaFailTOTP        = "wrong_otp"
	mfaFailRecovery    = "wrong_recovery_code"
	mfaFailUnavailable = "mfa_unavailable"
)

// mfaKeys, ana anahtardan türetilen alt anahtarlardır. Her amaç için ayrı anahtar
// kullanılır: biri sızarsa ya da kötüye kullanılırsa diğeri etkilenmez.
type mfaKeys struct {
	box      *secretbox.Box // TOTP sırlarını şifreler
	recovery []byte         // kurtarma kodlarının HMAC anahtarı
}

func newMFAKeys(master []byte) (mfaKeys, error) {
	encKey, err := hkdf.Key(sha256.New, master, nil, "agora mfa secret encryption", secretbox.KeySize)
	if err != nil {
		return mfaKeys{}, fmt.Errorf("iam: MFA anahtarı türetilemedi: %w", err)
	}
	box, err := secretbox.New(encKey)
	if err != nil {
		return mfaKeys{}, err
	}
	recovery, err := hkdf.Key(sha256.New, master, nil, "agora mfa recovery codes", 32)
	if err != nil {
		return mfaKeys{}, fmt.Errorf("iam: MFA anahtarı türetilemedi: %w", err)
	}
	return mfaKeys{box: box, recovery: recovery}, nil
}

// recoveryHash, kurtarma kodunun saklanan halidir. Anahtarlı hash (HMAC) kullanılır:
// 50 bitlik kodlar sadece veritabanı sızan biri tarafından çevrimdışı denenemez.
func (k mfaKeys) recoveryHash(code string) []byte {
	mac := hmac.New(sha256.New, k.recovery)
	mac.Write([]byte(normalizeRecoveryCode(code)))
	return mac.Sum(nil)
}

// normalizeRecoveryCode, kullanıcının yazdığı kodu karşılaştırılabilir hale getirir:
// büyük harf, tire ve boşluklar önemsizdir.
func normalizeRecoveryCode(code string) string {
	return strings.Map(func(r rune) rune {
		if r == '-' || r == ' ' {
			return -1
		}
		return r
	}, strings.ToLower(strings.TrimSpace(code)))
}

// newRecoveryCodes, kullanıcıya gösterilecek kurtarma kodlarını üretir (ör. "k7m2p-x9qaz").
func newRecoveryCodes() []string {
	codes := make([]string, recoveryCodeCount)
	buf := make([]byte, recoveryLength)
	for i := range codes {
		_, _ = rand.Read(buf)
		var b strings.Builder
		for j, c := range buf {
			if j == recoveryLength/2 {
				b.WriteByte('-')
			}
			b.WriteByte(recoveryAlphabet[c&31]) // 256, 32'nin katı: dağılım eşit
		}
		codes[i] = b.String()
	}
	return codes
}

// --- Giriş -------------------------------------------------------------------

// startMFAChallenge, parolası doğrulanmış kullanıcı için ikinci adım bekleyen bir
// giriş oluşturur. Oturum henüz açılmaz. newHash doluysa parola hash'i yükseltilir:
// parolanın düz hali sadece bu adımda bilinir.
func (a *Auth) startMFAChallenge(ctx context.Context, user User, client ClientType, now time.Time, newHash string) error {
	raw, hash, err := newRefreshToken() // aynı üretici: 256 bit rastgele + SHA-256
	if err != nil {
		return err
	}
	expiresAt := now.Add(a.cfg.MFAChallengeTTL)

	err = db.InTx(ctx, a.pool, func(tx pgx.Tx) error {
		r := NewRepository(tx)
		if newHash != "" {
			if err := r.rehashPassword(ctx, user.ID, newHash, now); err != nil {
				return err
			}
		}
		if err := r.createMFAChallenge(ctx, user.ID, hash, client, now, expiresAt); err != nil {
			return err
		}
		return audit.RecordSecurity(ctx, tx, audit.SecurityEvent{
			Type: audit.EventMFAChallengeStarted, UserID: user.ID,
			Details: map[string]any{"client": string(client)},
		})
	})
	if err != nil {
		return err
	}
	return &MFARequiredError{Token: raw, ExpiresAt: expiresAt}
}

// MFAVerifyInput, girişin ikinci adımının bilgileridir. Code (doğrulayıcı uygulamadaki
// 6 haneli kod) ya da RecoveryCode dolu olmalıdır.
type MFAVerifyInput struct {
	Token        string
	Code         string
	RecoveryCode string
	Client       ClientType
	IP           string
	UserAgent    string
}

// VerifyMFA, girişin ikinci adımını tamamlar ve oturumu açar.
//
// Yanlış kod, yanlış parola gibi hesabın başarısız deneme sayacını artırır: parolayı
// ele geçirmiş biri kodu deneme yanılmayla bulamaz. Ayrıca bir giriş denemesi en
// fazla mfaMaxAttempts kez kod kabul eder, sonra kullanıcı parolasını yeniden girer.
func (a *Auth) VerifyMFA(ctx context.Context, in MFAVerifyInput) (Tokens, error) {
	if in.Token == "" {
		return Tokens{}, ErrInvalidMFAToken
	}
	now := a.now()

	var (
		tokens      Tokens
		method      string     // kullanılan ikinci adım
		result      error      // commit'ten sonra döndürülecek hata
		failReason  string     // başarısız denemenin sebebi (metrik)
		lockedUntil *time.Time // bu denemeyle başlayan kilit
	)
	err := db.InTx(ctx, a.pool, func(tx pgx.Tx) error {
		r := NewRepository(tx)

		ch, err := r.mfaChallengeForUpdate(ctx, hashRefreshToken(in.Token))
		if errors.Is(err, ErrNotFound) {
			return ErrInvalidMFAToken
		}
		if err != nil {
			return err
		}
		// Token başka türde bir istemciye taşınamaz: web'de başlayan giriş mobilde bitmez.
		if ch.ConsumedAt != nil || !now.Before(ch.ExpiresAt) || ch.Client != in.Client {
			return ErrInvalidMFAToken
		}

		user, err := r.userForUpdate(ctx, ch.UserID)
		if err != nil {
			return err
		}
		// Bu arada hesap askıya alındıysa ya da yönetici MFA'yı sıfırladıysa giriş
		// baştan yapılmalı.
		if user.Status != StatusActive || !user.MFAEnabled {
			result, failReason = ErrInvalidMFAToken, mfaFailUnavailable
			if user.Status != StatusActive {
				result, failReason = ErrAccountDisabled, "account_inactive"
			}
			if err := r.consumeMFAChallenge(ctx, ch.ID, now); err != nil {
				return err
			}
			return a.recordRejected(ctx, tx, user, failReason)
		}
		if user.LockedUntil != nil && now.Before(*user.LockedUntil) {
			result, failReason = &LockedError{Until: *user.LockedUntil}, "account_locked"
			return a.recordRejected(ctx, tx, user, failReason)
		}

		method, err = a.verifySecondFactor(ctx, r, user, in.Code, in.RecoveryCode, now, true)
		if errors.Is(err, ErrInvalidMFACode) {
			result, failReason = ErrInvalidMFACode, mfaFailTOTP
			if in.Code == "" {
				failReason = mfaFailRecovery
			}
			if err := r.failMFAChallenge(ctx, ch.ID, now, mfaMaxAttempts); err != nil {
				return err
			}
			lockedUntil, err = a.recordFailure(ctx, tx, user, user.Username, now, failReason)
			return err
		}
		if err != nil {
			return err
		}

		if err := r.consumeMFAChallenge(ctx, ch.ID, now); err != nil {
			return err
		}
		if err := r.RecordLoginSuccess(ctx, user.ID, now, ""); err != nil {
			return err
		}

		amr := []string{methodPassword, authn.MethodOTP}
		absolute := now.Add(a.cfg.AbsoluteTimeout)
		sessionID, err := r.CreateSession(ctx, NewSession{
			UserID: user.ID, ClientType: in.Client, IP: in.IP, UserAgent: in.UserAgent,
			AMR: amr, CreatedAt: now, ExpiresAt: absolute,
		})
		if err != nil {
			return err
		}
		if tokens, _, err = a.issue(ctx, r, user, sessionID, "", now, absolute, amr); err != nil {
			return err
		}

		details := map[string]any{"session_id": sessionID, "client": string(in.Client), "mfa_method": method}
		if method == mfaMethodRecovery {
			remaining, err := r.recoveryCodesRemaining(ctx, user.ID)
			if err != nil {
				return err
			}
			details["recovery_codes_remaining"] = remaining
		}
		return audit.RecordSecurity(ctx, tx, audit.SecurityEvent{
			Type: audit.EventLoginSucceeded, UserID: user.ID, Details: details,
		})
	})
	if err != nil {
		return Tokens{}, err
	}
	if result != nil {
		return Tokens{}, a.failed(failReason, lockedUntil, now, result)
	}
	a.metrics.LoginSucceeded(string(in.Client))
	a.countMethod(method)
	return tokens, nil
}

// recordRejected, kod denenmeden reddedilen ikinci adımı kaydeder.
func (a *Auth) recordRejected(ctx context.Context, tx pgx.Tx, user User, reason string) error {
	return audit.RecordSecurity(ctx, tx, audit.SecurityEvent{
		Type: audit.EventLoginFailed, UserID: user.ID, UsernameAttempted: user.Username,
		Details: map[string]any{"reason": reason},
	})
}

// verifySecondFactor, TOTP kodunu ya da (allowRecovery ise) kurtarma kodunu doğrular
// ve tüketir: aynı TOTP kodu ve aynı kurtarma kodu ikinci kez kabul edilmez. Kod
// yanlışsa ErrInvalidMFACode döner. Kullanıcı satırı çağıran tarafından FOR UPDATE ile
// kilitlenmiş olmalı: eşzamanlı iki istek aynı kodu kabul ettiremez.
func (a *Auth) verifySecondFactor(ctx context.Context, r *Repository, user User, code, recoveryCode string,
	now time.Time, allowRecovery bool) (method string, err error) {

	switch {
	case code != "":
		secret, err := a.mfa.box.Open(user.MFASecretEnc, []byte(user.ID))
		if err != nil {
			return "", fmt.Errorf("iam: TOTP sırrı çözülemedi: %w", err)
		}
		step, ok := totp.Validate(secret, code, now, user.MFALastUsedStep)
		if !ok {
			return "", ErrInvalidMFACode
		}
		return mfaMethodTOTP, r.setMFALastUsedStep(ctx, user.ID, step)

	case recoveryCode != "" && allowRecovery:
		used, err := r.useRecoveryCode(ctx, user.ID, a.mfa.recoveryHash(recoveryCode), now)
		if err != nil {
			return "", err
		}
		if !used {
			return "", ErrInvalidMFACode
		}
		return mfaMethodRecovery, nil
	}
	return "", ErrInvalidMFACode
}

// --- Kurulum ve yönetim ------------------------------------------------------

// MFASetup, doğrulayıcı uygulamaya eklenecek sırdır. URI, QR kod olarak gösterilir.
// Secret, QR okutulamazsa elle girilir.
type MFASetup struct {
	Secret string
	URI    string
}

// MFAStatus, kullanıcının iki adımlı doğrulama durumudur.
type MFAStatus struct {
	Enabled                bool
	EnabledAt              *time.Time
	RecoveryCodesRemaining int
}

// MFAStatus, kullanıcının iki adımlı doğrulama durumunu döndürür.
func (a *Auth) MFAStatus(ctx context.Context, userID string) (MFAStatus, error) {
	repo := NewRepository(a.pool)
	user, err := repo.UserByID(ctx, userID)
	if err != nil {
		return MFAStatus{}, err
	}
	s := MFAStatus{Enabled: user.MFAEnabled, EnabledAt: user.MFAEnabledAt}
	if user.MFAEnabled {
		if s.RecoveryCodesRemaining, err = repo.recoveryCodesRemaining(ctx, userID); err != nil {
			return MFAStatus{}, err
		}
	}
	return s, nil
}

// StartMFASetup, yeni bir TOTP sırrı üretir ve şifreli olarak saklar. Kurulum,
// kullanıcı uygulamadaki ilk kodu EnableMFA ile doğrulayınca tamamlanır. Tamamlanmamış
// bir kurulum yeniden başlatılabilir: eski sır geçersiz olur.
func (a *Auth) StartMFASetup(ctx context.Context, userID string) (MFASetup, error) {
	now := a.now()
	secret := totp.NewSecret()

	var user User
	err := db.InTx(ctx, a.pool, func(tx pgx.Tx) error {
		r := NewRepository(tx)
		var err error
		if user, err = r.userForUpdate(ctx, userID); err != nil {
			return err
		}
		if user.MFAEnabled {
			return ErrMFAAlreadyEnabled
		}
		return r.setMFASecret(ctx, userID, a.mfa.box.Seal(secret, []byte(userID)), now)
	})
	if err != nil {
		return MFASetup{}, err
	}
	return MFASetup{
		Secret: totp.EncodeSecret(secret),
		URI:    totp.KeyURI(a.cfg.MFAIssuer, user.Username, secret),
	}, nil
}

// EnableMFAInput, iki adımlı doğrulamayı açma isteğidir.
type EnableMFAInput struct {
	UserID          string
	SessionID       string // isteği yapan oturum: açık kalır ve MFA'lı sayılır
	CurrentPassword string
	Code            string // doğrulayıcı uygulamadaki kod: kurulumun doğru yapıldığını kanıtlar
}

// EnableMFA, kurulumu tamamlar ve kurtarma kodlarını döndürür. Kodlar sadece bu
// yanıtta görünür, sunucu hash'lerini saklar.
//
// Mevcut parola istenir: çalınmış bir oturumla saldırgan kendi telefonunu ekleyip
// hesabın sahibini dışarıda bırakamaz. Diğer oturumlar kapatılır. İsteği yapan oturum
// parola ve kodla yeniden doğrulandığı için MFA'lı sayılır: bir sonraki token
// yenilemesinde access token'a "otp" eklenir.
func (a *Auth) EnableMFA(ctx context.Context, in EnableMFAInput) ([]string, error) {
	now := a.now()
	user, err := NewRepository(a.pool).UserByID(ctx, in.UserID)
	if err != nil {
		return nil, err
	}
	if err := a.verifyCurrentPassword(ctx, user, in.CurrentPassword, now, "mfa_enable_wrong_password"); err != nil {
		return nil, err
	}

	codes := newRecoveryCodes()
	var revoked []string
	err = db.InTx(ctx, a.pool, func(tx pgx.Tx) error {
		r := NewRepository(tx)
		user, err := r.userForUpdate(ctx, in.UserID)
		if err != nil {
			return err
		}
		if user.MFAEnabled {
			return ErrMFAAlreadyEnabled
		}
		if user.MFASecretEnc == nil {
			return ErrMFASetupRequired
		}
		secret, err := a.mfa.box.Open(user.MFASecretEnc, []byte(user.ID))
		if err != nil {
			return fmt.Errorf("iam: TOTP sırrı çözülemedi: %w", err)
		}
		step, ok := totp.Validate(secret, in.Code, now, 0)
		if !ok {
			return ErrInvalidMFACode
		}

		if err := r.enableMFA(ctx, user.ID, step, now); err != nil {
			return err
		}
		if err := r.replaceRecoveryCodes(ctx, user.ID, a.hashRecoveryCodes(codes), now); err != nil {
			return err
		}
		if revoked, err = r.RevokeOtherSessions(ctx, user.ID, in.SessionID, now, RevokeMFAChange); err != nil {
			return err
		}
		if err := r.setSessionAMR(ctx, in.SessionID, []string{methodPassword, authn.MethodOTP}); err != nil {
			return err
		}
		return audit.RecordSecurity(ctx, tx, audit.SecurityEvent{
			Type: audit.EventMFAEnabled, UserID: user.ID,
			Details: map[string]any{"revoked_sessions": len(revoked)},
		})
	})
	if err != nil {
		return nil, err
	}
	a.metrics.MFAEvent("enabled")
	return codes, a.revokeAll(ctx, revoked)
}

// DisableMFAInput, iki adımlı doğrulamayı kapatma isteğidir. Code ya da RecoveryCode
// dolu olmalıdır.
type DisableMFAInput struct {
	UserID          string
	SessionID       string
	CurrentPassword string
	Code            string
	RecoveryCode    string
}

// DisableMFA, iki adımlı doğrulamayı kapatır. Parola ve ikinci adım birlikte istenir.
// Diğer oturumlar kapatılır, isteği yapan oturum artık MFA'lı sayılmaz.
func (a *Auth) DisableMFA(ctx context.Context, in DisableMFAInput) error {
	now := a.now()
	user, err := NewRepository(a.pool).UserByID(ctx, in.UserID)
	if err != nil {
		return err
	}
	if !user.MFAEnabled {
		return ErrMFANotEnabled
	}
	if err := a.verifyCurrentPassword(ctx, user, in.CurrentPassword, now, "mfa_disable_wrong_password"); err != nil {
		return err
	}

	var (
		revoked    []string
		usedMethod string
	)
	err = a.withSecondFactor(ctx, in.UserID, in.Code, in.RecoveryCode, true, now,
		func(ctx context.Context, tx pgx.Tx, user User, method string) error {
			usedMethod = method
			r := NewRepository(tx)
			if err := r.disableMFA(ctx, user.ID, now); err != nil {
				return err
			}
			var err error
			if revoked, err = r.RevokeOtherSessions(ctx, user.ID, in.SessionID, now, RevokeMFAChange); err != nil {
				return err
			}
			if err := r.setSessionAMR(ctx, in.SessionID, []string{methodPassword}); err != nil {
				return err
			}
			return audit.RecordSecurity(ctx, tx, audit.SecurityEvent{
				Type: audit.EventMFADisabled, UserID: user.ID,
				Details: map[string]any{"by": "user", "mfa_method": method, "revoked_sessions": len(revoked)},
			})
		})
	if err != nil {
		return err
	}
	a.countMethod(usedMethod)
	a.metrics.MFAEvent("disabled")
	return a.revokeAll(ctx, revoked)
}

// RegenerateRecoveryCodes, kurtarma kodlarını yeniler: eskilerin hepsi geçersiz olur.
// Doğrulayıcı uygulamadaki kod istenir (kurtarma kodu değil): kodları biten kullanıcı
// telefonu elindeyken yenilerini alır.
func (a *Auth) RegenerateRecoveryCodes(ctx context.Context, userID, code string) ([]string, error) {
	now := a.now()
	codes := newRecoveryCodes()
	err := a.withSecondFactor(ctx, userID, code, "", false, now,
		func(ctx context.Context, tx pgx.Tx, user User, _ string) error {
			if err := NewRepository(tx).replaceRecoveryCodes(ctx, user.ID, a.hashRecoveryCodes(codes), now); err != nil {
				return err
			}
			return audit.RecordSecurity(ctx, tx, audit.SecurityEvent{Type: audit.EventRecoveryCodesRenewed, UserID: user.ID})
		})
	if err != nil {
		return nil, err
	}
	a.metrics.MFAEvent("recovery_codes_renewed")
	return codes, nil
}

// withSecondFactor, oturum açmış kullanıcının ikinci adımını doğrular ve doğruysa fn'i
// aynı transaction'da çalıştırır. Yanlış kod, girişteki başarısız denemelerle aynı
// sayacı artırır ve bu sayaç geri alınmasın diye transaction yine de commit edilir.
func (a *Auth) withSecondFactor(ctx context.Context, userID, code, recoveryCode string, allowRecovery bool,
	now time.Time, fn func(ctx context.Context, tx pgx.Tx, user User, method string) error) error {

	var (
		result      error
		failReason  string
		lockedUntil *time.Time
	)
	err := db.InTx(ctx, a.pool, func(tx pgx.Tx) error {
		r := NewRepository(tx)
		user, err := r.userForUpdate(ctx, userID)
		if err != nil {
			return err
		}
		if !user.MFAEnabled {
			return ErrMFANotEnabled
		}
		if user.LockedUntil != nil && now.Before(*user.LockedUntil) {
			return &LockedError{Until: *user.LockedUntil}
		}

		method, err := a.verifySecondFactor(ctx, r, user, code, recoveryCode, now, allowRecovery)
		if errors.Is(err, ErrInvalidMFACode) {
			result, failReason = ErrInvalidMFACode, mfaFailTOTP
			if code == "" {
				failReason = mfaFailRecovery
			}
			lockedUntil, err = a.recordFailure(ctx, tx, user, user.Username, now, failReason)
			return err
		}
		if err != nil {
			return err
		}
		return fn(ctx, tx, user, method)
	})
	if err != nil {
		return err
	}
	if result != nil {
		return a.failed(failReason, lockedUntil, now, result)
	}
	return nil
}

// ResetMFA, yöneticinin isteğiyle kullanıcının iki adımlı doğrulamasını kapatır
// (telefonunu ve kurtarma kodlarını kaybeden kullanıcı için). Kullanıcının bütün
// oturumları kapatılır. Yönetici kendi MFA'sını bu yolla kapatamaz.
func (a *Auth) ResetMFA(ctx context.Context, actorID, userID, reason string) error {
	if actorID == userID {
		return ErrSelfAction
	}
	now := a.now()
	var revoked []string
	err := db.InTx(ctx, a.pool, func(tx pgx.Tx) error {
		r := NewRepository(tx)
		user, err := r.userForUpdate(ctx, userID)
		if err != nil {
			return err
		}
		if !user.MFAEnabled {
			return ErrMFANotEnabled
		}
		if err := r.disableMFA(ctx, userID, now); err != nil {
			return err
		}
		if revoked, err = r.RevokeOtherSessions(ctx, userID, "", now, RevokeAdmin); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "user.mfa_reset", EntityType: "user", EntityID: userID,
			Before: map[string]any{"mfa_enabled": true},
			After:  map[string]any{"mfa_enabled": false, "reason": reason, "revoked_sessions": len(revoked)},
		}); err != nil {
			return err
		}
		return audit.RecordSecurity(ctx, tx, audit.SecurityEvent{
			Type: audit.EventMFADisabled, UserID: userID,
			Details: map[string]any{"by": "admin", "actor_id": actorID, "revoked_sessions": len(revoked)},
		})
	})
	if err != nil {
		return err
	}
	a.metrics.MFAEvent("reset")
	return a.revokeAll(ctx, revoked)
}

// countMethod, commit edilmiş bir kurtarma kodu kullanımını sayar.
func (a *Auth) countMethod(method string) {
	if method == mfaMethodRecovery {
		a.metrics.MFAEvent("recovery_code_used")
	}
}

func (a *Auth) hashRecoveryCodes(codes []string) [][]byte {
	hashes := make([][]byte, len(codes))
	for i, c := range codes {
		hashes[i] = a.mfa.recoveryHash(c)
	}
	return hashes
}

// --- Veri katmanı ------------------------------------------------------------

type mfaChallenge struct {
	ID         string
	UserID     string
	Client     ClientType
	ExpiresAt  time.Time
	ConsumedAt *time.Time
}

// userForUpdate, hesabı satır kilidiyle okur.
func (r *Repository) userForUpdate(ctx context.Context, id string) (User, error) {
	return r.user(ctx, userSelect+" WHERE id = $1 FOR UPDATE", id)
}

// rehashPassword, parola hash'ini daha güçlü parametrelerle yenisiyle değiştirir.
// Parola değişmediği için password_changed_at'e dokunulmaz.
func (r *Repository) rehashPassword(ctx context.Context, userID, hash string, at time.Time) error {
	if _, err := r.db.Exec(ctx, `UPDATE iam.users SET password_hash = $2, updated_at = $3 WHERE id = $1`,
		userID, hash, at); err != nil {
		return fmt.Errorf("iam: parola hash'i güncellenemedi: %w", err)
	}
	return nil
}

// createMFAChallenge, ikinci adım bekleyen girişi kaydeder. Kullanıcının süresi dolmuş
// ya da kullanılmış eski kayıtları aynı anda silinir: tablo kendiliğinden temiz kalır.
func (r *Repository) createMFAChallenge(ctx context.Context, userID string, hash []byte, client ClientType, at, expiresAt time.Time) error {
	if _, err := r.db.Exec(ctx,
		`DELETE FROM iam.mfa_challenges WHERE user_id = $1 AND (expires_at <= $2 OR consumed_at IS NOT NULL)`,
		userID, at); err != nil {
		return fmt.Errorf("iam: eski MFA girişleri silinemedi: %w", err)
	}
	if _, err := r.db.Exec(ctx,
		`INSERT INTO iam.mfa_challenges (user_id, token_hash, client_type, created_at, expires_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		userID, hash, string(client), at, expiresAt); err != nil {
		return fmt.Errorf("iam: MFA girişi oluşturulamadı: %w", err)
	}
	return nil
}

func (r *Repository) mfaChallengeForUpdate(ctx context.Context, hash []byte) (mfaChallenge, error) {
	var (
		c      mfaChallenge
		client string
	)
	err := r.db.QueryRow(ctx,
		`SELECT id, user_id, client_type, expires_at, consumed_at
		 FROM iam.mfa_challenges WHERE token_hash = $1 FOR UPDATE`, hash,
	).Scan(&c.ID, &c.UserID, &client, &c.ExpiresAt, &c.ConsumedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return mfaChallenge{}, ErrNotFound
	}
	if err != nil {
		return mfaChallenge{}, fmt.Errorf("iam: MFA girişi okunamadı: %w", err)
	}
	c.Client = ClientType(client)
	return c, nil
}

func (r *Repository) consumeMFAChallenge(ctx context.Context, id string, at time.Time) error {
	if _, err := r.db.Exec(ctx, `UPDATE iam.mfa_challenges SET consumed_at = $2 WHERE id = $1`, id, at); err != nil {
		return fmt.Errorf("iam: MFA girişi kapatılamadı: %w", err)
	}
	return nil
}

// failMFAChallenge, yanlış denemeyi sayar. Deneme hakkı biten giriş kapanır.
func (r *Repository) failMFAChallenge(ctx context.Context, id string, at time.Time, maxAttempts int) error {
	_, err := r.db.Exec(ctx, `
		UPDATE iam.mfa_challenges
		SET attempts = attempts + 1,
		    consumed_at = CASE WHEN attempts + 1 >= $3 THEN $2 ELSE consumed_at END
		WHERE id = $1`, id, at, maxAttempts)
	if err != nil {
		return fmt.Errorf("iam: MFA denemesi kaydedilemedi: %w", err)
	}
	return nil
}

func (r *Repository) setMFASecret(ctx context.Context, userID string, enc []byte, at time.Time) error {
	_, err := r.db.Exec(ctx,
		`UPDATE iam.users SET mfa_secret_enc = $2, mfa_last_used_step = 0, updated_at = $3 WHERE id = $1`,
		userID, enc, at)
	if err != nil {
		return fmt.Errorf("iam: MFA sırrı kaydedilemedi: %w", err)
	}
	return nil
}

func (r *Repository) enableMFA(ctx context.Context, userID string, step uint64, at time.Time) error {
	_, err := r.db.Exec(ctx, `
		UPDATE iam.users
		SET mfa_enabled = true, mfa_enabled_at = $3, mfa_last_used_step = $2, updated_at = $3
		WHERE id = $1`, userID, int64(step), at)
	if err != nil {
		return fmt.Errorf("iam: MFA açılamadı: %w", err)
	}
	return nil
}

// disableMFA, sırrı ve kurtarma kodlarını siler.
func (r *Repository) disableMFA(ctx context.Context, userID string, at time.Time) error {
	if _, err := r.db.Exec(ctx, `
		UPDATE iam.users
		SET mfa_enabled = false, mfa_secret_enc = NULL, mfa_enabled_at = NULL, mfa_last_used_step = 0, updated_at = $2
		WHERE id = $1`, userID, at); err != nil {
		return fmt.Errorf("iam: MFA kapatılamadı: %w", err)
	}
	if _, err := r.db.Exec(ctx, `DELETE FROM iam.mfa_recovery_codes WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("iam: kurtarma kodları silinemedi: %w", err)
	}
	return nil
}

func (r *Repository) setMFALastUsedStep(ctx context.Context, userID string, step uint64) error {
	if _, err := r.db.Exec(ctx, `UPDATE iam.users SET mfa_last_used_step = $2 WHERE id = $1`, userID, int64(step)); err != nil {
		return fmt.Errorf("iam: TOTP adımı kaydedilemedi: %w", err)
	}
	return nil
}

func (r *Repository) replaceRecoveryCodes(ctx context.Context, userID string, hashes [][]byte, at time.Time) error {
	if _, err := r.db.Exec(ctx, `DELETE FROM iam.mfa_recovery_codes WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("iam: kurtarma kodları silinemedi: %w", err)
	}
	if _, err := r.db.Exec(ctx, `
		INSERT INTO iam.mfa_recovery_codes (user_id, code_hash, created_at)
		SELECT $1, h, $3 FROM unnest($2::bytea[]) AS h`, userID, hashes, at); err != nil {
		return fmt.Errorf("iam: kurtarma kodları kaydedilemedi: %w", err)
	}
	return nil
}

// useRecoveryCode, kullanılmamış kodu tüketir. Kod yoksa ya da kullanılmışsa false döner.
func (r *Repository) useRecoveryCode(ctx context.Context, userID string, hash []byte, at time.Time) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE iam.mfa_recovery_codes SET used_at = $3
		WHERE user_id = $1 AND code_hash = $2 AND used_at IS NULL`, userID, hash, at)
	if err != nil {
		return false, fmt.Errorf("iam: kurtarma kodu kullanılamadı: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (r *Repository) recoveryCodesRemaining(ctx context.Context, userID string) (int, error) {
	var n int
	err := r.db.QueryRow(ctx,
		`SELECT count(*) FROM iam.mfa_recovery_codes WHERE user_id = $1 AND used_at IS NULL`, userID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("iam: kurtarma kodları sayılamadı: %w", err)
	}
	return n, nil
}

// setSessionAMR, oturumun kimlik doğrulama yöntemlerini değiştirir. Access token'lar
// yenilenirken AMR oturumdan kopyalanır: değişiklik bir sonraki yenilemede etkili olur.
func (r *Repository) setSessionAMR(ctx context.Context, sessionID string, amr []string) error {
	if _, err := r.db.Exec(ctx, `UPDATE iam.sessions SET amr = $2 WHERE id = $1`, sessionID, amr); err != nil {
		return fmt.Errorf("iam: oturum güncellenemedi: %w", err)
	}
	return nil
}
