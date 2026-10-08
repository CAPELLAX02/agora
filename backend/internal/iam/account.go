package iam

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/iam/password"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// ErrInvalidCurrentPassword, parola değiştirirken mevcut parola yanlış girildiğinde döner.
var ErrInvalidCurrentPassword = errors.New("iam: mevcut parola hatalı")

// PolicyError, yeni parolanın politikaya uymadığını bildirir.
type PolicyError struct {
	Violations []password.Violation
}

func (e *PolicyError) Error() string {
	return fmt.Sprintf("iam: parola politikaya uymuyor: %v", e.Violations)
}

// ChangePasswordInput, parola değiştirme isteğinin bilgileridir.
type ChangePasswordInput struct {
	UserID          string
	SessionID       string // isteği yapan oturum: açık kalır, diğerleri kapatılır
	CurrentPassword string
	NewPassword     string
}

// ChangePassword, kullanıcının parolasını değiştirir.
//
// Mevcut parola, çalınmış bir access token'la parolanın değiştirilmesini önler.
// Yanlış girilen mevcut parola, girişteki başarısız denemelerle aynı sayacı artırır:
// token'ı ele geçiren biri bu uçla parola tahmin edemez. Değişiklikten sonra
// kullanıcının diğer bütün oturumları kapatılır.
func (a *Auth) ChangePassword(ctx context.Context, in ChangePasswordInput) error {
	now := a.now()
	repo := NewRepository(a.pool)

	user, err := repo.UserByID(ctx, in.UserID)
	if err != nil {
		return err
	}
	if user.LockedUntil != nil && now.Before(*user.LockedUntil) {
		return &LockedError{Until: *user.LockedUntil}
	}

	if err := a.hasher.Verify(ctx, in.CurrentPassword, user.PasswordHash); err != nil {
		if !errors.Is(err, password.ErrMismatch) {
			return err
		}
		err := a.wrongPassword(ctx, user, user.Username, now, "password_change_wrong_current")
		if errors.Is(err, ErrInvalidCredentials) {
			return ErrInvalidCurrentPassword
		}
		return err
	}

	if err := a.checkPolicy(ctx, repo, user, in.NewPassword, in.CurrentPassword); err != nil {
		return err
	}

	hash, err := a.hasher.Hash(ctx, in.NewPassword)
	if err != nil {
		return err
	}

	var revoked []string
	err = db.InTx(ctx, a.pool, func(tx pgx.Tx) error {
		r := NewRepository(tx)
		if err := r.UpdatePassword(ctx, user.ID, hash, now); err != nil {
			return err
		}
		if revoked, err = r.RevokeOtherSessions(ctx, user.ID, in.SessionID, now, RevokePasswordChange); err != nil {
			return err
		}
		return audit.RecordSecurity(ctx, tx, audit.SecurityEvent{
			Type: audit.EventPasswordChanged, UserID: user.ID,
			Details: map[string]any{"revoked_sessions": len(revoked)},
		})
	})
	if err != nil {
		return err
	}
	return a.revokeAll(ctx, revoked)
}

// checkPolicy, yeni parolayı politikaya ve kişisel bilgilere göre denetler. current
// boş değilse yeni parolanın ondan farklı olması da istenir.
func (a *Auth) checkPolicy(ctx context.Context, repo *Repository, user User, newPassword, current string) error {
	profile, err := repo.Profile(ctx, user.ID, a.now())
	if err != nil {
		return err
	}
	emailLocal, _, _ := strings.Cut(user.Email, "@")

	violations := password.Validate(newPassword, user.Username, emailLocal, profile.FirstName, profile.LastName)
	if current != "" && newPassword == current {
		violations = append(violations, password.SameAsCurrent)
	}
	if len(violations) > 0 {
		return &PolicyError{Violations: violations}
	}
	return nil
}

// revokeAll, sonlandırılan oturumları iptal listesine ekler. Bir oturumda hata olsa
// da diğerleri denenir: bir Redis hatası yüzünden diğer oturumlar açık kalmamalı.
func (a *Auth) revokeAll(ctx context.Context, sessionIDs []string) error {
	var errs []error
	for _, id := range sessionIDs {
		if err := a.revoker.Revoke(ctx, id); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Sessions, kullanıcının aktif oturumlarını döndürür.
func (a *Auth) Sessions(ctx context.Context, userID string) ([]SessionInfo, error) {
	return NewRepository(a.pool).ActiveSessions(ctx, userID, a.now())
}

// EndSession, kullanıcının kendi oturumlarından birini sonlandırır (ör. kaybolan
// telefondaki oturum). Oturum kullanıcıya ait değilse ErrNotFound döner.
func (a *Auth) EndSession(ctx context.Context, userID, sessionID string) error {
	now := a.now()
	var revoked bool
	err := db.InTx(ctx, a.pool, func(tx pgx.Tx) error {
		var err error
		revoked, err = NewRepository(tx).RevokeUserSession(ctx, userID, sessionID, now, RevokeLogout)
		if err != nil || !revoked {
			return err
		}
		return audit.RecordSecurity(ctx, tx, audit.SecurityEvent{
			Type: audit.EventSessionRevoked, UserID: userID,
			Details: map[string]any{"session_id": sessionID, "by": "user"},
		})
	})
	if err != nil {
		return err
	}
	// Zaten sonlanmış olsa da iptal listesine yazmak zararsız ve olası bir eski
	// Redis hatasını onarır.
	return a.revoker.Revoke(ctx, sessionID)
}

// EndOtherSessions, isteği yapan oturum dışındaki bütün oturumları sonlandırır ve
// kaç oturumun kapatıldığını döndürür.
func (a *Auth) EndOtherSessions(ctx context.Context, userID, currentSessionID string) (int, error) {
	now := a.now()
	var revoked []string
	err := db.InTx(ctx, a.pool, func(tx pgx.Tx) error {
		var err error
		revoked, err = NewRepository(tx).RevokeOtherSessions(ctx, userID, currentSessionID, now, RevokeLogoutAll)
		if err != nil || len(revoked) == 0 {
			return err
		}
		return audit.RecordSecurity(ctx, tx, audit.SecurityEvent{
			Type: audit.EventOtherSessionsRevoked, UserID: userID,
			Details: map[string]any{"count": len(revoked)},
		})
	})
	if err != nil {
		return 0, err
	}
	return len(revoked), a.revokeAll(ctx, revoked)
}
