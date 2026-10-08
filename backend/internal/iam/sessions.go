package iam

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ClientType, oturumu açan istemcinin türüdür.
type ClientType string

// İstemci türleri.
const (
	ClientWeb    ClientType = "WEB"
	ClientMobile ClientType = "MOBILE"
)

// RevokeReason, bir oturumun neden sonlandırıldığıdır.
type RevokeReason string

// Oturum sonlandırma sebepleri.
const (
	RevokeLogout        RevokeReason = "LOGOUT"
	RevokeLogoutAll     RevokeReason = "LOGOUT_ALL"
	RevokeReuseDetected RevokeReason = "REUSE_DETECTED"
	RevokePasswordReset RevokeReason = "PASSWORD_RESET"
	RevokeAdmin         RevokeReason = "ADMIN"
)

// NewSession, oluşturulacak oturumun bilgileridir.
type NewSession struct {
	UserID     string
	ClientType ClientType
	IP         string
	UserAgent  string
	AMR        []string
	CreatedAt  time.Time
	ExpiresAt  time.Time // mutlak bitiş: refresh'lerle uzamaz
}

// NewRefreshToken, saklanacak refresh token'ın bilgileridir. Token'ın kendisi değil,
// sadece SHA-256 hash'i saklanır.
type NewRefreshToken struct {
	SessionID string
	Hash      []byte
	ParentID  string // rotasyonda bir önceki token. İlk token'da boş
	IssuedAt  time.Time
	ExpiresAt time.Time
}

// RefreshTokenRecord, bir refresh token'ı ve bağlı olduğu oturumun durumunu birlikte taşır.
type RefreshTokenRecord struct {
	ID               string
	SessionID        string
	UserID           string
	ExpiresAt        time.Time
	UsedAt           *time.Time
	SessionRevokedAt *time.Time
	SessionExpiresAt time.Time
	AMR              []string
}

// CreateSession, yeni bir oturum oluşturur ve kimliğini döndürür.
func (r *Repository) CreateSession(ctx context.Context, s NewSession) (string, error) {
	var id string
	err := r.db.QueryRow(ctx, `
		INSERT INTO iam.sessions (user_id, client_type, ip, user_agent, amr,
		                          created_at, last_seen_at, absolute_expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $6, $7)
		RETURNING id`,
		s.UserID, string(s.ClientType), nullable(s.IP), nullable(s.UserAgent), s.AMR,
		s.CreatedAt, s.ExpiresAt,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("iam: oturum oluşturulamadı: %w", err)
	}
	return id, nil
}

// CreateRefreshToken, bir refresh token'ın hash'ini saklar ve kaydın kimliğini döndürür.
func (r *Repository) CreateRefreshToken(ctx context.Context, t NewRefreshToken) (string, error) {
	var id string
	err := r.db.QueryRow(ctx, `
		INSERT INTO iam.refresh_tokens (session_id, token_hash, parent_id, issued_at, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`,
		t.SessionID, t.Hash, nullable(t.ParentID), t.IssuedAt, t.ExpiresAt,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("iam: refresh token saklanamadı: %w", err)
	}
	return id, nil
}

// RefreshTokenForUpdate, hash'i verilen token'ı oturumuyla birlikte okur ve iki
// satırı da transaction sonuna kadar kilitler (FOR UPDATE). Aynı token'la gelen
// eşzamanlı refresh istekleri böylece sıraya girer: ilki rotasyonu tamamlar,
// sonrakiler token'ı "kullanılmış" görür. Bir transaction içinde çağrılmalıdır.
func (r *Repository) RefreshTokenForUpdate(ctx context.Context, hash []byte) (RefreshTokenRecord, error) {
	var rec RefreshTokenRecord
	err := r.db.QueryRow(ctx, `
		SELECT rt.id, rt.session_id, s.user_id, rt.expires_at, rt.used_at,
		       s.revoked_at, s.absolute_expires_at, s.amr
		FROM iam.refresh_tokens rt
		JOIN iam.sessions s ON s.id = rt.session_id
		WHERE rt.token_hash = $1
		FOR UPDATE OF rt, s`,
		hash,
	).Scan(&rec.ID, &rec.SessionID, &rec.UserID, &rec.ExpiresAt, &rec.UsedAt,
		&rec.SessionRevokedAt, &rec.SessionExpiresAt, &rec.AMR)
	if errors.Is(err, pgx.ErrNoRows) {
		return RefreshTokenRecord{}, ErrNotFound
	}
	if err != nil {
		return RefreshTokenRecord{}, fmt.Errorf("iam: refresh token okunamadı: %w", err)
	}
	return rec, nil
}

// MarkRefreshTokenUsed, rotasyonda eski token'ı kullanılmış olarak işaretler ve
// yerine gelen token'a bağlar.
func (r *Repository) MarkRefreshTokenUsed(ctx context.Context, id string, at time.Time, replacedBy string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE iam.refresh_tokens SET used_at = $2, replaced_by_id = $3 WHERE id = $1`,
		id, at, replacedBy,
	)
	if err != nil {
		return fmt.Errorf("iam: refresh token işaretlenemedi: %w", err)
	}
	return nil
}

// TouchSession, oturumun son görülme zamanını günceller.
func (r *Repository) TouchSession(ctx context.Context, sessionID string, at time.Time) error {
	_, err := r.db.Exec(ctx, `UPDATE iam.sessions SET last_seen_at = $2 WHERE id = $1`, sessionID, at)
	if err != nil {
		return fmt.Errorf("iam: oturum güncellenemedi: %w", err)
	}
	return nil
}

// RevokeSession, oturumu sonlandırır. Zaten sonlandırılmışsa hiçbir şey yapmaz,
// böylece ilk sonlandırma sebebi kaybolmaz.
func (r *Repository) RevokeSession(ctx context.Context, sessionID string, at time.Time, reason RevokeReason) error {
	_, err := r.db.Exec(ctx,
		`UPDATE iam.sessions SET revoked_at = $2, revoke_reason = $3
		 WHERE id = $1 AND revoked_at IS NULL`,
		sessionID, at, string(reason),
	)
	if err != nil {
		return fmt.Errorf("iam: oturum sonlandırılamadı: %w", err)
	}
	return nil
}

// RecordLoginFailure, başarısız giriş sayacını atomik olarak artırır. Sayaç maxAttempts'e
// ulaştığında hesabı kilitler: kilit süresi base'den başlar, her yeni başarısız
// denemede ikiye katlanır ve maxLock'u geçmez (1 dk, 2 dk, 4 dk ... en fazla 1 saat).
//
// Hesaplama SQL içinde yapılır: aynı hesaba eşzamanlı gelen başarısız denemeler
// sayacı kaybetmeden sırayla artırır.
func (r *Repository) RecordLoginFailure(ctx context.Context, userID string, now time.Time, maxAttempts int, base, maxLock time.Duration) (lockedUntil *time.Time, err error) {
	err = r.db.QueryRow(ctx, `
		UPDATE iam.users
		SET failed_login_count = failed_login_count + 1,
		    locked_until = CASE
		        WHEN failed_login_count + 1 >= $3 THEN
		            $2::timestamptz + make_interval(secs => least(
		                $4::float8 * power(2, failed_login_count + 1 - $3),
		                $5::float8))
		        ELSE locked_until
		    END,
		    updated_at = $2
		WHERE id = $1
		RETURNING locked_until`,
		userID, now, maxAttempts, base.Seconds(), maxLock.Seconds(),
	).Scan(&lockedUntil)
	if err != nil {
		return nil, fmt.Errorf("iam: başarısız giriş kaydedilemedi: %w", err)
	}
	return lockedUntil, nil
}

// RecordLoginSuccess, başarılı girişte sayacı ve kilidi sıfırlar, son giriş zamanını
// yazar. newPasswordHash doluysa parola hash'i de güncellenir (parametre yükseltme).
func (r *Repository) RecordLoginSuccess(ctx context.Context, userID string, at time.Time, newPasswordHash string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE iam.users
		SET failed_login_count = 0,
		    locked_until = NULL,
		    last_login_at = $2,
		    password_hash = coalesce($3, password_hash),
		    updated_at = $2
		WHERE id = $1`,
		userID, at, nullable(newPasswordHash),
	)
	if err != nil {
		return fmt.Errorf("iam: başarılı giriş kaydedilemedi: %w", err)
	}
	return nil
}
