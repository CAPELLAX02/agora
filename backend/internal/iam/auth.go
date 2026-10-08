package iam

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/iam/password"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
	"github.com/CAPELLAX02/agora/backend/internal/platform/jwt"
)

// Kimlik doğrulama hataları. HTTP katmanı bunları istemciye aynı ayrıntı
// düzeyinde yansıtmaz: örneğin "kullanıcı yok" ile "parola yanlış" ayırt edilmez.
var (
	ErrInvalidCredentials  = errors.New("iam: kullanıcı adı veya parola hatalı")
	ErrAccountLocked       = errors.New("iam: hesap geçici olarak kilitli")
	ErrAccountDisabled     = errors.New("iam: hesap aktif değil")
	ErrInvalidRefreshToken = errors.New("iam: refresh token geçersiz")
	ErrRefreshTokenReused  = errors.New("iam: refresh token yeniden kullanıldı")
)

// LockedError, hesabın ne zamana kadar kilitli olduğunu taşıyan hatadır.
// errors.Is(err, ErrAccountLocked) ile tanınır, errors.As ile kilit bitişi okunur.
type LockedError struct {
	Until time.Time
}

func (e *LockedError) Error() string {
	return fmt.Sprintf("%v: %s'e kadar", ErrAccountLocked, e.Until.Format(time.RFC3339))
}

// Is, errors.Is'in bu hatayı ErrAccountLocked ile eşleştirmesini sağlar.
func (e *LockedError) Is(target error) bool {
	return target == ErrAccountLocked
}

// AuthConfig, kimlik doğrulama servisinin ayarlarıdır.
type AuthConfig struct {
	Issuer            string
	Audience          string
	AccessTokenTTL    time.Duration // access token ömrü (ör. 15 dk)
	IdleTimeout       time.Duration // bu süre refresh yapılmazsa oturum düşer (ör. 2 saat)
	AbsoluteTimeout   time.Duration // oturumun refresh'lerle bile uzayamayacağı üst sınır (ör. 30 gün)
	ReuseGracePeriod  time.Duration // eşzamanlı sekmeler için kullanılmış token toleransı (ör. 10 sn)
	MaxFailedAttempts int           // bu kadar başarısız denemeden sonra hesap kilitlenir
	LockoutBase       time.Duration // ilk kilit süresi
	LockoutMax        time.Duration // en uzun kilit süresi
}

// Hasher, parola hash'leme işlemleridir. *password.Hasher bunu sağlar.
type Hasher interface {
	Hash(ctx context.Context, password string) (string, error)
	Verify(ctx context.Context, password, encoded string) error
	NeedsRehash(encoded string) bool
}

// TokenSigner, access token imzalayan bileşendir. *jwt.Signer bunu sağlar.
type TokenSigner interface {
	Sign(c jwt.Claims) (string, error)
}

// LoginInput, giriş isteğinin bilgileridir.
type LoginInput struct {
	Username  string
	Password  string
	Client    ClientType
	IP        string
	UserAgent string
}

// Tokens, başarılı giriş veya refresh sonucunda istemciye verilen token'lardır.
type Tokens struct {
	AccessToken        string
	AccessExpiresAt    time.Time
	RefreshToken       string // sadece bir kez, üretildiği anda bilinir. Sunucu hash'ini saklar
	RefreshExpiresAt   time.Time
	SessionID          string
	UserID             string
	MustChangePassword bool
}

// Auth, giriş, token yenileme ve çıkış iş kurallarını uygular.
type Auth struct {
	pool      *pgxpool.Pool
	hasher    Hasher
	signer    TokenSigner
	cfg       AuthConfig
	now       func() time.Time
	dummyHash string
}

// NewAuth, bir kimlik doğrulama servisi oluşturur. now, zamanı veren fonksiyondur:
// production'da time.Now, testlerde kontrol edilen sahte bir saat.
func NewAuth(ctx context.Context, pool *pgxpool.Pool, hasher Hasher, signer TokenSigner, cfg AuthConfig, now func() time.Time) (*Auth, error) {
	// Var olmayan kullanıcı adlarında da gerçek bir parola doğrulaması yapılsın diye
	// bir kez sahte bir hash üretiyoruz. Aksi halde "kullanıcı yok" yanıtı, argon2id'yi
	// atladığı için ~50 ms daha hızlı döner ve geçerli kullanıcı adları zamanlamadan
	// tespit edilebilirdi (user enumeration).
	dummy, err := hasher.Hash(ctx, "agora-dummy-password-for-timing")
	if err != nil {
		return nil, fmt.Errorf("iam: sahte hash üretilemedi: %w", err)
	}
	return &Auth{pool: pool, hasher: hasher, signer: signer, cfg: cfg, now: now, dummyHash: dummy}, nil
}

// Login, kullanıcı adı ve parolayı doğrular, yeni bir oturum açar ve token'ları döndürür.
func (a *Auth) Login(ctx context.Context, in LoginInput) (Tokens, error) {
	now := a.now()
	repo := NewRepository(a.pool)

	user, err := repo.UserByUsername(ctx, strings.TrimSpace(in.Username))
	if errors.Is(err, ErrNotFound) {
		_ = a.hasher.Verify(ctx, in.Password, a.dummyHash) // zamanlamayı eşitlemek için
		return Tokens{}, ErrInvalidCredentials
	}
	if err != nil {
		return Tokens{}, err
	}

	if user.LockedUntil != nil && now.Before(*user.LockedUntil) {
		return Tokens{}, &LockedError{Until: *user.LockedUntil}
	}

	if err := a.hasher.Verify(ctx, in.Password, user.PasswordHash); err != nil {
		if !errors.Is(err, password.ErrMismatch) {
			return Tokens{}, err
		}
		lockedUntil, err := repo.RecordLoginFailure(ctx, user.ID, now,
			a.cfg.MaxFailedAttempts, a.cfg.LockoutBase, a.cfg.LockoutMax)
		if err != nil {
			return Tokens{}, err
		}
		if lockedUntil != nil && now.Before(*lockedUntil) {
			return Tokens{}, &LockedError{Until: *lockedUntil}
		}
		return Tokens{}, ErrInvalidCredentials
	}

	// Hesap durumu parola doğrulandıktan SONRA kontrol edilir: yanlış parolayla
	// gelen biri, hesabın askıda olup olmadığını öğrenemez.
	if user.Status != StatusActive {
		return Tokens{}, ErrAccountDisabled
	}

	newHash := ""
	if a.hasher.NeedsRehash(user.PasswordHash) {
		if newHash, err = a.hasher.Hash(ctx, in.Password); err != nil {
			return Tokens{}, err
		}
	}

	var tokens Tokens
	err = db.InTx(ctx, a.pool, func(tx pgx.Tx) error {
		r := NewRepository(tx)

		if err := r.RecordLoginSuccess(ctx, user.ID, now, newHash); err != nil {
			return err
		}

		amr := []string{"pwd"}
		absolute := now.Add(a.cfg.AbsoluteTimeout)
		sessionID, err := r.CreateSession(ctx, NewSession{
			UserID: user.ID, ClientType: in.Client, IP: in.IP, UserAgent: in.UserAgent,
			AMR: amr, CreatedAt: now, ExpiresAt: absolute,
		})
		if err != nil {
			return err
		}

		tokens, _, err = a.issue(ctx, r, user, sessionID, "", now, absolute, amr)
		return err
	})
	if err != nil {
		return Tokens{}, err
	}
	return tokens, nil
}

// Refresh, refresh token'ı tek kullanımlık olarak tüketir ve yerine yenisini verir
// (rotasyon). Daha önce kullanılmış bir token tekrar gelirse token çalınmış kabul
// edilir: oturum ve ona bağlı tüm token'lar iptal edilir.
func (a *Auth) Refresh(ctx context.Context, rawToken string) (Tokens, error) {
	if rawToken == "" {
		return Tokens{}, ErrInvalidRefreshToken
	}
	now := a.now()

	var (
		tokens        Tokens
		reuseDetected bool
		userDisabled  bool
	)
	err := db.InTx(ctx, a.pool, func(tx pgx.Tx) error {
		r := NewRepository(tx)

		rec, err := r.RefreshTokenForUpdate(ctx, hashRefreshToken(rawToken))
		if errors.Is(err, ErrNotFound) {
			return ErrInvalidRefreshToken
		}
		if err != nil {
			return err
		}

		if rec.SessionRevokedAt != nil || !now.Before(rec.SessionExpiresAt) {
			return ErrInvalidRefreshToken
		}

		if rec.UsedAt != nil {
			// Aynı tarayıcıda iki sekme aynı anda refresh yaparsa ikincisi az önce
			// kullanılmış token'ı gönderir. Bu meşru bir yarıştır: oturumu öldürmeden
			// reddediyoruz, istemci yeni token'la tekrar dener.
			if now.Sub(*rec.UsedAt) <= a.cfg.ReuseGracePeriod {
				return ErrInvalidRefreshToken
			}
			// Tolerans dışında yeniden kullanım: token çalınmış olabilir. Oturumu iptal
			// ediyoruz. Hata DÖNDÜRMÜYORUZ, çünkü hata dönersek InTx geri alır ve iptal
			// de geri alınmış olur. Commit'ten sonra hatayı kendimiz döndüreceğiz.
			reuseDetected = true
			return r.RevokeSession(ctx, rec.SessionID, now, RevokeReuseDetected)
		}

		if !now.Before(rec.ExpiresAt) {
			return ErrInvalidRefreshToken
		}

		user, err := r.UserByID(ctx, rec.UserID)
		if err != nil {
			return err
		}
		if user.Status != StatusActive {
			userDisabled = true
			return r.RevokeSession(ctx, rec.SessionID, now, RevokeAdmin)
		}

		var newTokenID string
		tokens, newTokenID, err = a.issue(ctx, r, user, rec.SessionID, rec.ID, now, rec.SessionExpiresAt, rec.AMR)
		if err != nil {
			return err
		}
		if err := r.MarkRefreshTokenUsed(ctx, rec.ID, now, newTokenID); err != nil {
			return err
		}
		return r.TouchSession(ctx, rec.SessionID, now)
	})

	switch {
	case err != nil:
		return Tokens{}, err
	case reuseDetected:
		return Tokens{}, ErrRefreshTokenReused
	case userDisabled:
		return Tokens{}, ErrAccountDisabled
	}
	return tokens, nil
}

// Logout, refresh token'ın bağlı olduğu oturumu sonlandırır. Bilinmeyen ya da zaten
// geçersiz bir token için hata döndürmez: çıkış her zaman başarılıdır.
func (a *Auth) Logout(ctx context.Context, rawToken string) error {
	if rawToken == "" {
		return nil
	}
	now := a.now()

	return db.InTx(ctx, a.pool, func(tx pgx.Tx) error {
		r := NewRepository(tx)
		rec, err := r.RefreshTokenForUpdate(ctx, hashRefreshToken(rawToken))
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return r.RevokeSession(ctx, rec.SessionID, now, RevokeLogout)
	})
}

// issue, yeni bir refresh token üretip saklar ve imzalı bir access token oluşturur.
// Refresh token'ın geçerliliği boşta kalma süresi kadardır ama oturumun mutlak
// bitişini asla geçmez.
func (a *Auth) issue(ctx context.Context, r *Repository, user User, sessionID, parentID string,
	now, sessionExpiresAt time.Time, amr []string) (Tokens, string, error) {

	raw, hash, err := newRefreshToken()
	if err != nil {
		return Tokens{}, "", err
	}

	refreshExpiresAt := now.Add(a.cfg.IdleTimeout)
	if sessionExpiresAt.Before(refreshExpiresAt) {
		refreshExpiresAt = sessionExpiresAt
	}

	tokenID, err := r.CreateRefreshToken(ctx, NewRefreshToken{
		SessionID: sessionID, Hash: hash, ParentID: parentID,
		IssuedAt: now, ExpiresAt: refreshExpiresAt,
	})
	if err != nil {
		return Tokens{}, "", err
	}

	accessExpiresAt := now.Add(a.cfg.AccessTokenTTL)
	access, err := a.signer.Sign(jwt.Claims{
		Issuer:    a.cfg.Issuer,
		Subject:   user.ID,
		Audience:  a.cfg.Audience,
		ExpiresAt: accessExpiresAt.Unix(),
		NotBefore: now.Unix(),
		IssuedAt:  now.Unix(),
		ID:        rand.Text(),
		SessionID: sessionID,
		AMR:       amr,
	})
	if err != nil {
		return Tokens{}, "", err
	}

	return Tokens{
		AccessToken:        access,
		AccessExpiresAt:    accessExpiresAt,
		RefreshToken:       raw,
		RefreshExpiresAt:   refreshExpiresAt,
		SessionID:          sessionID,
		UserID:             user.ID,
		MustChangePassword: user.MustChangePassword,
	}, tokenID, nil
}

// newRefreshToken, 256 bit rastgele bir refresh token ve onun SHA-256 hash'ini üretir.
// Token zaten tahmin edilemez olduğu için argon2 gibi yavaş bir hash gerekmez.
func newRefreshToken() (raw string, hash []byte, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, fmt.Errorf("iam: refresh token üretilemedi: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	return raw, hashRefreshToken(raw), nil
}

func hashRefreshToken(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}
