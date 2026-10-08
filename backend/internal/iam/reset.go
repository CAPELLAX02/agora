package iam

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/mail"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

// Token amaçları.
const (
	PurposeReset      = "RESET"
	PurposeActivation = "ACTIVATION"
)

// ErrInvalidResetToken, sıfırlama ya da aktivasyon bağlantısı geçersiz, kullanılmış
// veya süresi dolmuşsa döner. Sebepler ayırt edilmez.
var ErrInvalidResetToken = errors.New("iam: bağlantı geçersiz ya da süresi dolmuş")

// resetToken, veritabanındaki bir sıfırlama ya da aktivasyon token'ıdır.
type resetToken struct {
	ID        string
	UserID    string
	Purpose   string
	ExpiresAt time.Time
	UsedAt    *time.Time
}

// UserByEmail, e-postaya göre hesabı döndürür. Karşılaştırma büyük/küçük harf
// duyarsızdır (sütun citext). Bulunamazsa ErrNotFound döner.
func (r *Repository) UserByEmail(ctx context.Context, email string) (User, error) {
	return r.user(ctx, userSelect+" WHERE email = $1", email)
}

// createResetToken, kullanıcının kullanılmamış bütün token'larını geçersiz kılar ve
// yenisini saklar: aynı anda sadece en son istenen bağlantı çalışır.
func (r *Repository) createResetToken(ctx context.Context, userID, purpose string, hash []byte, at, expiresAt time.Time, ip string) error {
	if _, err := r.db.Exec(ctx, `
		UPDATE iam.password_reset_tokens SET used_at = $2
		WHERE user_id = $1 AND used_at IS NULL`, userID, at); err != nil {
		return fmt.Errorf("iam: eski bağlantılar geçersiz kılınamadı: %w", err)
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO iam.password_reset_tokens (user_id, purpose, token_hash, expires_at, requested_ip, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		userID, purpose, hash, expiresAt, nullable(ip), at)
	if err != nil {
		return fmt.Errorf("iam: bağlantı saklanamadı: %w", err)
	}
	return nil
}

// lastResetRequest, kullanıcının en son sıfırlama isteğinin zamanıdır. Hiç yoksa nil.
func (r *Repository) lastResetRequest(ctx context.Context, userID string) (*time.Time, error) {
	var at *time.Time
	err := r.db.QueryRow(ctx,
		`SELECT max(created_at) FROM iam.password_reset_tokens WHERE user_id = $1 AND purpose = 'RESET'`,
		userID).Scan(&at)
	if err != nil {
		return nil, fmt.Errorf("iam: son sıfırlama isteği okunamadı: %w", err)
	}
	return at, nil
}

// resetTokenByHash, token'ı döndürür. forUpdate, satırı transaction sonuna kadar kilitler:
// aynı bağlantıyla eşzamanlı iki istekten sadece biri parolayı değiştirebilir.
func (r *Repository) resetTokenByHash(ctx context.Context, hash []byte, forUpdate bool) (resetToken, error) {
	q := `SELECT id, user_id, purpose, expires_at, used_at FROM iam.password_reset_tokens WHERE token_hash = $1`
	if forUpdate {
		q += " FOR UPDATE"
	}
	var t resetToken
	err := r.db.QueryRow(ctx, q, hash).Scan(&t.ID, &t.UserID, &t.Purpose, &t.ExpiresAt, &t.UsedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return resetToken{}, ErrNotFound
	}
	if err != nil {
		return resetToken{}, fmt.Errorf("iam: bağlantı okunamadı: %w", err)
	}
	return t, nil
}

// activateIfPending, hesap PENDING ise ACTIVE yapar.
func (r *Repository) activateIfPending(ctx context.Context, userID string, at time.Time) error {
	_, err := r.db.Exec(ctx,
		`UPDATE iam.users SET status = 'ACTIVE', updated_at = $2 WHERE id = $1 AND status = 'PENDING'`,
		userID, at)
	if err != nil {
		return fmt.Errorf("iam: hesap etkinleştirilemedi: %w", err)
	}
	return nil
}

// RequestPasswordReset, kullanıcı adına (öğrenci/personel numarası) ya da e-postaya
// göre hesabı bulur ve sıfırlama bağlantısı e-postası kuyruğa alır.
//
// Hesap olsun ya da olmasın hata döndürmez: yanıt, bir kullanıcı adının ya da
// e-postanın sistemde kayıtlı olup olmadığını sızdırmamalıdır (numaralandırma).
// Askıdaki hesaplara bağlantı gönderilmez. Aynı hesaba ResetRequestInterval
// içinde ikinci bir e-posta gönderilmez: başkasının adına e-posta bombardımanı
// yapılamaz.
func (a *Auth) RequestPasswordReset(ctx context.Context, identifier string) error {
	now := a.now()
	repo := NewRepository(a.pool)
	identifier = strings.TrimSpace(identifier)

	var (
		user User
		err  error
	)
	if strings.Contains(identifier, "@") {
		user, err = repo.UserByEmail(ctx, identifier)
	} else {
		user, err = repo.UserByUsername(ctx, identifier)
	}

	ignore := func(userID, reason string) error {
		return audit.RecordSecurity(ctx, a.pool, audit.SecurityEvent{
			Type: audit.EventPasswordResetRequested, UserID: userID, UsernameAttempted: identifier,
			Details: map[string]any{"result": "ignored", "reason": reason},
		})
	}

	switch {
	case errors.Is(err, ErrNotFound):
		return ignore("", "unknown_user")
	case err != nil:
		return err
	case user.Status != StatusActive && user.Status != StatusPending:
		return ignore(user.ID, "account_inactive")
	}

	last, err := repo.lastResetRequest(ctx, user.ID)
	if err != nil {
		return err
	}
	if last != nil && now.Sub(*last) < a.cfg.ResetRequestInterval {
		return ignore(user.ID, "throttled")
	}

	return a.sendToken(ctx, user, PurposeReset, a.cfg.ResetTokenTTL)
}

// sendToken, yeni bir bağlantı token'ı üretir ve e-postasını kuyruğa alır. Token,
// e-posta kaydı ve güvenlik olayı tek transaction'da yazılır.
func (a *Auth) sendToken(ctx context.Context, user User, purpose string, ttl time.Duration) error {
	return db.InTx(ctx, a.pool, func(tx pgx.Tx) error {
		return a.sendTokenTx(ctx, tx, user, purpose, ttl)
	})
}

// sendTokenTx, sendToken'ın çağıranın transaction'ında çalışan halidir (ör. hesap
// oluştururken aktivasyon e-postası, hesapla birlikte commit edilir).
func (a *Auth) sendTokenTx(ctx context.Context, tx pgx.Tx, user User, purpose string, ttl time.Duration) error {
	now := a.now()
	r := NewRepository(tx)

	raw, hash, err := newRefreshToken() // aynı üretici: 256 bit rastgele + SHA-256
	if err != nil {
		return err
	}
	if err := r.createResetToken(ctx, user.ID, purpose, hash, now, now.Add(ttl), httpx.ClientInfoFrom(ctx).IP); err != nil {
		return err
	}

	profile, err := r.Profile(ctx, user.ID, now)
	if err != nil {
		return err
	}
	// Token URL'nin parça (#) kısmında: tarayıcı bu kısmı sunucuya göndermez, böylece
	// token web sunucusu log'larına ve Referer başlığına düşmez.
	link := a.cfg.WebBaseURL + "/sifre-sifirla#token=" + raw

	email := mail.Email{To: user.Email, Data: map[string]any{"name": profile.FirstName + " " + profile.LastName, "link": link}}
	event := audit.EventPasswordResetRequested
	if purpose == PurposeActivation {
		email.Template = mail.TemplateAccountActivation
		email.Data["username"] = user.Username
		email.Data["expires_hours"] = int(ttl.Hours())
		event = "" // aktivasyon, hesabı oluşturan işlemin denetim kaydında görünür
	} else {
		email.Template = mail.TemplatePasswordReset
		email.Data["expires_minutes"] = int(ttl.Minutes())
	}
	if _, err := mail.Enqueue(ctx, tx, email); err != nil {
		return err
	}

	if event == "" {
		return nil
	}
	return audit.RecordSecurity(ctx, tx, audit.SecurityEvent{
		Type: event, UserID: user.ID, Details: map[string]any{"result": "sent"},
	})
}

// ResetTokenInfo, geçerli bir bağlantı hakkında web arayüzünün göstereceği bilgidir.
type ResetTokenInfo struct {
	Purpose   string
	ExpiresAt time.Time
}

// VerifyResetToken, bağlantının kullanılabilir olup olmadığını söyler. Web arayüzü
// formu göstermeden önce çağırır: süresi dolmuş bir bağlantıda kullanıcı boşuna
// parola yazmasın.
func (a *Auth) VerifyResetToken(ctx context.Context, rawToken string) (ResetTokenInfo, error) {
	t, _, err := a.validResetToken(ctx, NewRepository(a.pool), rawToken, false)
	if err != nil {
		return ResetTokenInfo{}, err
	}
	return ResetTokenInfo{Purpose: t.Purpose, ExpiresAt: t.ExpiresAt}, nil
}

// ResetPassword, bağlantıyla yeni parolayı belirler. Aktivasyon bağlantısıysa hesap
// da etkinleşir. Kullanıcının bütün oturumları kapatılır: parolayı sıfırlatan kişi
// hesabı ele geçirilmiş biri olabilir.
func (a *Auth) ResetPassword(ctx context.Context, rawToken, newPassword string) error {
	repo := NewRepository(a.pool)

	// Önce kilitsiz kontrol: geçersiz bir bağlantı için parola politikası ve argon2id
	// maliyetine girilmez.
	t, user, err := a.validResetToken(ctx, repo, rawToken, false)
	if err != nil {
		return err
	}
	if err := a.checkPolicy(ctx, repo, user, newPassword, ""); err != nil {
		return err // bağlantı kullanılmadı: kullanıcı başka bir parolayla tekrar deneyebilir
	}
	hash, err := a.hasher.Hash(ctx, newPassword)
	if err != nil {
		return err
	}

	now := a.now()
	var revoked []string
	err = db.InTx(ctx, a.pool, func(tx pgx.Tx) error {
		r := NewRepository(tx)

		// Kilitle ve tekrar kontrol et: aynı bağlantı eşzamanlı kullanılmış olabilir.
		locked, _, err := a.validResetToken(ctx, r, rawToken, true)
		if err != nil {
			return err
		}
		if locked.ID != t.ID {
			return ErrInvalidResetToken
		}

		if _, err := tx.Exec(ctx,
			`UPDATE iam.password_reset_tokens SET used_at = $2 WHERE user_id = $1 AND used_at IS NULL`,
			user.ID, now); err != nil {
			return fmt.Errorf("iam: bağlantı kullanıldı olarak işaretlenemedi: %w", err)
		}
		if err := r.UpdatePassword(ctx, user.ID, hash, now); err != nil {
			return err
		}
		if err := r.activateIfPending(ctx, user.ID, now); err != nil {
			return err
		}
		if revoked, err = r.RevokeOtherSessions(ctx, user.ID, "", now, RevokePasswordReset); err != nil {
			return err
		}
		return audit.RecordSecurity(ctx, tx, audit.SecurityEvent{
			Type: audit.EventPasswordResetCompleted, UserID: user.ID,
			Details: map[string]any{"purpose": t.Purpose, "revoked_sessions": len(revoked)},
		})
	})
	if err != nil {
		return err
	}
	return a.revokeAll(ctx, revoked)
}

// validResetToken, token'ı ve sahibini döndürür. Token yok, kullanılmış, süresi
// dolmuş ya da hesap askıdaysa ErrInvalidResetToken döner.
func (a *Auth) validResetToken(ctx context.Context, repo *Repository, rawToken string, forUpdate bool) (resetToken, User, error) {
	if rawToken == "" {
		return resetToken{}, User{}, ErrInvalidResetToken
	}
	t, err := repo.resetTokenByHash(ctx, hashRefreshToken(rawToken), forUpdate)
	if errors.Is(err, ErrNotFound) {
		return resetToken{}, User{}, ErrInvalidResetToken
	}
	if err != nil {
		return resetToken{}, User{}, err
	}
	if t.UsedAt != nil || !a.now().Before(t.ExpiresAt) {
		return resetToken{}, User{}, ErrInvalidResetToken
	}

	user, err := repo.UserByID(ctx, t.UserID)
	if err != nil {
		return resetToken{}, User{}, err
	}
	if user.Status != StatusActive && user.Status != StatusPending {
		return resetToken{}, User{}, ErrInvalidResetToken
	}
	return t, user, nil
}
