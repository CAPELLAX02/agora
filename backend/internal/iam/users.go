package iam

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/people"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// Hesap yönetimi hataları.
var (
	ErrSelfAction             = errors.New("iam: kişi bu işlemi kendi hesabında yapamaz")
	ErrInvalidStatusChange    = errors.New("iam: bu durum değişikliği yapılamaz")
	ErrUnknownDepartment      = errors.New("iam: bölüm bulunamadı")
	ErrUnknownAcademicTitle   = errors.New("iam: akademik unvan bulunamadı")
	ErrAccountNotPending      = errors.New("iam: hesap aktivasyon beklemiyor")
	ErrAccountNotActive       = errors.New("iam: hesap aktif değil")
	ErrLastSystemAdmin        = errors.New("iam: son sistem yöneticisinin rolü kaldırılamaz")
	ErrRoleAssignmentNotFound = errors.New("iam: rol ataması bulunamadı")
)

// Hesap türleri.
const (
	KindStudent = "STUDENT"
	KindStaff   = "STAFF"
)

// NewAccount, yönetici tarafından oluşturulacak hesabın bilgileridir.
type NewAccount struct {
	Kind          string // KindStudent ya da KindStaff
	Number        string // öğrenci ya da personel numarası; kullanıcı adı olur
	FirstName     string
	LastName      string
	Email         string
	StaffType     people.StaffType // sadece personelde
	AcademicTitle string           // sadece akademik personelde
	DepartmentID  string           // personelin bağlı olduğu bölüm (isteğe bağlı)
}

// CreateAccount, kişiyi, öğrenci ya da personel kaydını ve hesabı oluşturur ve
// aktivasyon e-postasını kuyruğa alır. Hesap PENDING başlar: kullanıcı e-postadaki
// bağlantıyla kendi parolasını belirleyince etkinleşir. Yönetici hiçbir zaman
// parola görmez ya da belirlemez.
//
// Kullanıcı adı ya da e-posta başka bir hesapta varsa ErrConflict döner.
func (a *Auth) CreateAccount(ctx context.Context, actorID string, in NewAccount) (string, error) {
	// Kimsenin bilmediği bir parola: hesap aktivasyondan önce girişe kapalıdır.
	placeholder, err := a.hasher.Hash(ctx, rand.Text()+rand.Text())
	if err != nil {
		return "", err
	}

	var userID string
	err = db.InTx(ctx, a.pool, func(tx pgx.Tx) error {
		if err := checkReferences(ctx, tx, in); err != nil {
			return err
		}

		pr := people.NewRepository(tx)
		personID, err := pr.CreatePerson(ctx, people.NewPerson{FirstName: in.FirstName, LastName: in.LastName})
		if err != nil {
			return err
		}
		if in.Kind == KindStudent {
			err = pr.CreateStudent(ctx, personID, in.Number)
		} else {
			err = pr.CreateStaff(ctx, people.NewStaff{
				PersonID: personID, StaffNo: in.Number, Type: in.StaffType,
				AcademicTitle: in.AcademicTitle, DepartmentID: in.DepartmentID,
			})
		}
		if errors.Is(err, people.ErrConflict) {
			return ErrConflict
		}
		if err != nil {
			return err
		}

		r := NewRepository(tx)
		userID, err = r.CreateUser(ctx, NewUser{
			PersonID: personID, Username: in.Number, Email: in.Email,
			PasswordHash: placeholder, Status: StatusPending,
		})
		if err != nil {
			return err
		}
		user, err := r.UserByID(ctx, userID)
		if err != nil {
			return err
		}
		if err := a.sendTokenTx(ctx, tx, user, PurposeActivation, a.cfg.ActivationTokenTTL); err != nil {
			return err
		}

		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "user.create", EntityType: "user", EntityID: userID,
			After: map[string]any{
				"username": in.Number, "email": in.Email, "kind": in.Kind, "status": StatusPending,
				"first_name": in.FirstName, "last_name": in.LastName,
			},
		})
	})
	if err != nil {
		return "", err
	}
	return userID, nil
}

// checkReferences, veritabanı kısıtına takılmadan önce başvuruların varlığını denetler:
// yabancı anahtar hatası yerine alanı belli, anlaşılır bir hata döner.
func checkReferences(ctx context.Context, q db.Querier, in NewAccount) error {
	if in.DepartmentID != "" {
		var ok bool
		if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM org.departments WHERE id = $1)`, in.DepartmentID).Scan(&ok); err != nil {
			return fmt.Errorf("iam: bölüm denetlenemedi: %w", err)
		}
		if !ok {
			return ErrUnknownDepartment
		}
	}
	if in.AcademicTitle != "" {
		var ok bool
		if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM people.academic_titles WHERE code = $1)`, in.AcademicTitle).Scan(&ok); err != nil {
			return fmt.Errorf("iam: unvan denetlenemedi: %w", err)
		}
		if !ok {
			return ErrUnknownAcademicTitle
		}
	}
	return nil
}

// SetStatus, hesabın durumunu değiştirir (askıya alma, kapatma, yeniden etkinleştirme).
// Askıya alınan ya da kapatılan hesabın bütün oturumları hemen sonlanır. Kişi kendi
// hesabının durumunu değiştiremez. Aktivasyonu tamamlanmamış (PENDING) bir hesap
// elle etkinleştirilemez: kullanıcı parolasını belirlemelidir.
func (a *Auth) SetStatus(ctx context.Context, actorID, userID string, status UserStatus, reason string) error {
	if actorID == userID {
		return ErrSelfAction
	}
	switch status {
	case StatusActive, StatusSuspended, StatusDisabled:
	default:
		return ErrInvalidStatusChange
	}

	now := a.now()
	var revoked []string
	err := db.InTx(ctx, a.pool, func(tx pgx.Tx) error {
		var current string
		err := tx.QueryRow(ctx, `SELECT status FROM iam.users WHERE id = $1 FOR UPDATE`, userID).Scan(&current)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("iam: hesap okunamadı: %w", err)
		}
		if UserStatus(current) == status {
			return nil
		}
		if UserStatus(current) == StatusPending && status == StatusActive {
			return ErrInvalidStatusChange
		}

		if _, err := tx.Exec(ctx, `UPDATE iam.users SET status = $2, updated_at = $3 WHERE id = $1`,
			userID, string(status), now); err != nil {
			return fmt.Errorf("iam: hesap durumu değiştirilemedi: %w", err)
		}
		if status != StatusActive {
			if revoked, err = NewRepository(tx).RevokeOtherSessions(ctx, userID, "", now, RevokeAdmin); err != nil {
				return err
			}
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "user.status_change", EntityType: "user", EntityID: userID,
			Before: map[string]any{"status": current},
			After:  map[string]any{"status": status, "reason": reason, "revoked_sessions": len(revoked)},
		})
	})
	if err != nil {
		return err
	}
	return a.revokeAll(ctx, revoked)
}

// ResendActivation, aktivasyonu tamamlanmamış hesaba yeni bir aktivasyon e-postası
// gönderir. Önceki bağlantılar geçersiz olur.
func (a *Auth) ResendActivation(ctx context.Context, actorID, userID string) error {
	return a.sendForUser(ctx, actorID, userID, StatusPending, PurposeActivation, a.cfg.ActivationTokenTTL,
		"user.activation_email", ErrAccountNotPending)
}

// SendPasswordReset, yöneticinin isteğiyle kullanıcıya parola sıfırlama bağlantısı
// gönderir (ör. yardım masası). Yönetici bağlantıyı görmez, e-posta kullanıcıya gider.
func (a *Auth) SendPasswordReset(ctx context.Context, actorID, userID string) error {
	return a.sendForUser(ctx, actorID, userID, StatusActive, PurposeReset, a.cfg.ResetTokenTTL,
		"user.password_reset_email", ErrAccountNotActive)
}

func (a *Auth) sendForUser(ctx context.Context, actorID, userID string, requiredStatus UserStatus,
	purpose string, ttl time.Duration, action string, statusErr error) error {

	return db.InTx(ctx, a.pool, func(tx pgx.Tx) error {
		user, err := NewRepository(tx).UserByID(ctx, userID)
		if err != nil {
			return err
		}
		if user.Status != requiredStatus {
			return statusErr
		}
		if err := a.sendTokenTx(ctx, tx, user, purpose, ttl); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: action, EntityType: "user", EntityID: userID,
		})
	})
}

// UserSummary, kullanıcı listesindeki bir satırdır.
type UserSummary struct {
	ID          string
	Username    string
	Email       string
	FirstName   string
	LastName    string
	Status      UserStatus
	Kind        string // KindStudent, KindStaff ya da boş (sadece hesap)
	LastLoginAt *time.Time
	CreatedAt   time.Time
}

// UserCursor, kullanıcı listesinde bir sonraki sayfanın başlangıcıdır.
type UserCursor struct {
	Username string `json:"u"`
	ID       string `json:"i"`
}

// UserFilter, kullanıcı listesinin filtreleridir.
type UserFilter struct {
	Query  string     // kullanıcı adı, e-posta ya da ad soyadda geçen metin
	Status UserStatus // boşsa hepsi
	Kind   string     // boşsa hepsi
	After  *UserCursor
	Limit  int
}

// ListUsers, filtreye uyan hesapları kullanıcı adına göre sıralı döndürür.
func (r *Repository) ListUsers(ctx context.Context, f UserFilter) (users []UserSummary, hasMore bool, err error) {
	var w db.Where
	if f.Query != "" {
		w.Add(`(u.username ILIKE $1 OR u.email ILIKE $1 OR (pe.first_name || ' ' || pe.last_name) ILIKE $1)`,
			"%"+f.Query+"%")
	}
	if f.Status != "" {
		w.Add("u.status = $1", string(f.Status))
	}
	switch f.Kind {
	case KindStudent:
		w.Add("st.id IS NOT NULL")
	case KindStaff:
		w.Add("sf.id IS NOT NULL")
	}
	if f.After != nil {
		w.Add("(u.username, u.id) > ($1::citext, $2::uuid)", f.After.Username, f.After.ID)
	}
	limit := w.Arg(f.Limit + 1)

	rows, err := r.db.Query(ctx, `
		SELECT u.id, u.username, u.email, pe.first_name, pe.last_name, u.status,
		       CASE WHEN st.id IS NOT NULL THEN 'STUDENT' WHEN sf.id IS NOT NULL THEN 'STAFF' ELSE '' END,
		       u.last_login_at, u.created_at
		FROM iam.users u
		JOIN people.persons pe ON pe.id = u.person_id
		LEFT JOIN people.students st ON st.person_id = u.person_id
		LEFT JOIN people.staff sf ON sf.person_id = u.person_id
		`+w.SQL()+`
		ORDER BY u.username, u.id
		LIMIT `+limit, w.Args()...)
	if err != nil {
		return nil, false, fmt.Errorf("iam: kullanıcılar listelenemedi: %w", err)
	}
	users, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (UserSummary, error) {
		var (
			u      UserSummary
			status string
		)
		err := row.Scan(&u.ID, &u.Username, &u.Email, &u.FirstName, &u.LastName, &status, &u.Kind,
			&u.LastLoginAt, &u.CreatedAt)
		u.Status = UserStatus(status)
		return u, err
	})
	if err != nil {
		return nil, false, fmt.Errorf("iam: kullanıcılar okunamadı: %w", err)
	}
	if len(users) > f.Limit {
		return users[:f.Limit], true, nil
	}
	return users, false, nil
}
