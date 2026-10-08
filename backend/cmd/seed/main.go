// Komut seed, geliştirme ortamı için örnek kullanıcılar oluşturur.
//
// Organizasyon verisi (fakülte, bölüm) SQL seed'inden gelir, bu yüzden önce
// "make seed" ile o yüklenmiş olmalıdır. Komut tekrar çalıştırılabilir: var olan
// kullanıcılar atlanır. Production ortamında çalışmayı reddeder.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/iam"
	"github.com/CAPELLAX02/agora/backend/internal/iam/password"
	"github.com/CAPELLAX02/agora/backend/internal/people"
	"github.com/CAPELLAX02/agora/backend/internal/platform/config"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// role, bir seed kullanıcısına atanacak rol ve kapsamıdır. scopeCode, kapsam
// türüne göre fakülte ya da bölüm kodudur (ör. "MUH", "BIL").
type role struct {
	code      string
	scope     iam.ScopeType
	scopeCode string
}

type seedUser struct {
	username  string // öğrenci için öğrenci no, personel için personel no
	firstName string
	lastName  string
	email     string
	student   bool
	staffType people.StaffType
	title     string // akademik unvan kodu
	deptCode  string // personelin bağlı olduğu bölüm
	roles     []role
}

// Kurgusal kişiler. Gerçek kişilerle ilgisi yoktur.
var users = []seedUser{
	{
		username: "22290001", firstName: "Deniz", lastName: "Aksoy",
		email: "deniz.aksoy@ogrenci.agora.test", student: true,
		roles: []role{{code: "STUDENT", scope: iam.ScopeNone}},
	},
	{
		username: "22290002", firstName: "Ece", lastName: "Kaya",
		email: "ece.kaya@ogrenci.agora.test", student: true,
		roles: []role{{code: "STUDENT", scope: iam.ScopeNone}},
	},
	{
		username: "P10001", firstName: "Ayşe", lastName: "Demir",
		email: "ayse.demir@agora.test", staffType: people.StaffAcademic, title: "PROF", deptCode: "BIL",
		roles: []role{
			{code: "INSTRUCTOR", scope: iam.ScopeDepartment, scopeCode: "BIL"},
			{code: "ADVISOR", scope: iam.ScopeDepartment, scopeCode: "BIL"},
		},
	},
	{
		username: "P10002", firstName: "Mehmet", lastName: "Yıldız",
		email: "mehmet.yildiz@agora.test", staffType: people.StaffAcademic, title: "ASSOC_PROF", deptCode: "BIL",
		roles: []role{
			{code: "INSTRUCTOR", scope: iam.ScopeDepartment, scopeCode: "BIL"},
			{code: "DEPARTMENT_HEAD", scope: iam.ScopeDepartment, scopeCode: "BIL"},
		},
	},
	{
		username: "P20001", firstName: "Fatma", lastName: "Çelik",
		email: "fatma.celik@agora.test", staffType: people.StaffAdministrative,
		roles: []role{{code: "FACULTY_REGISTRAR", scope: iam.ScopeFaculty, scopeCode: "MUH"}},
	},
	{
		username: "P20002", firstName: "Hasan", lastName: "Öztürk",
		email: "hasan.ozturk@agora.test", staffType: people.StaffAdministrative,
		roles: []role{{code: "CENTRAL_REGISTRAR", scope: iam.ScopeUniversity}},
	},
	{
		username: "P90001", firstName: "Sistem", lastName: "Yöneticisi",
		email: "admin@agora.test", staffType: people.StaffAdministrative,
		roles: []role{{code: "SYSTEM_ADMIN", scope: iam.ScopeUniversity}},
	},
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "agora-seed: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.IsProduction() {
		return errors.New("production ortamında seed çalıştırılamaz")
	}

	plain := os.Getenv("AGORA_SEED_PASSWORD")
	if plain == "" {
		plain = "agora-dev-parola"
	}
	if v := password.Validate(plain); len(v) > 0 {
		return fmt.Errorf("seed parolası politikaya uymuyor: %v", v)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, db.Options{
		URL:             cfg.DatabaseURL,
		MaxConns:        2,
		MaxConnLifetime: cfg.DBMaxConnLifetime,
		MaxConnIdleTime: cfg.DBMaxConnIdleTime,
	})
	if err != nil {
		return err
	}
	defer pool.Close()

	hasher := password.NewHasher(password.DefaultParams, runtime.NumCPU())

	for _, u := range users {
		if _, err := iam.NewRepository(pool).UserByUsername(ctx, u.username); err == nil {
			fmt.Printf("ATLANDI    %-9s %s %s (zaten var)\n", u.username, u.firstName, u.lastName)
			continue
		} else if !errors.Is(err, iam.ErrNotFound) {
			return err
		}

		// Her kullanıcının parolası ayrı hash'lenir: salt rastgele olduğu için
		// aynı parola bile farklı hash üretir.
		hash, err := hasher.Hash(ctx, plain)
		if err != nil {
			return err
		}

		err = db.InTx(ctx, pool, func(tx pgx.Tx) error {
			return createUser(ctx, tx, u, hash)
		})
		if err != nil {
			return fmt.Errorf("%s: %w", u.username, err)
		}

		fmt.Printf("OLUŞTURULDU %-9s %s %s %v\n", u.username, u.firstName, u.lastName, roleCodes(u.roles))
	}

	fmt.Printf("\nTüm seed kullanıcılarının parolası: %s\n", plain)
	return nil
}

// createUser, kişiyi, öğrenci veya personel kaydını, hesabı ve rollerini tek bir
// transaction içinde oluşturur: herhangi bir adım başarısız olursa hiçbiri kalmaz.
func createUser(ctx context.Context, tx pgx.Tx, u seedUser, hash string) error {
	peopleRepo := people.NewRepository(tx)
	iamRepo := iam.NewRepository(tx)

	personID, err := peopleRepo.CreatePerson(ctx, people.NewPerson{FirstName: u.firstName, LastName: u.lastName})
	if err != nil {
		return err
	}

	if u.student {
		if err := peopleRepo.CreateStudent(ctx, personID, u.username); err != nil {
			return err
		}
	} else {
		deptID := ""
		if u.deptCode != "" {
			if deptID, err = lookupID(ctx, tx, "org.departments", u.deptCode); err != nil {
				return err
			}
		}
		err := peopleRepo.CreateStaff(ctx, people.NewStaff{
			PersonID: personID, StaffNo: u.username, Type: u.staffType,
			AcademicTitle: u.title, DepartmentID: deptID,
		})
		if err != nil {
			return err
		}
	}

	userID, err := iamRepo.CreateUser(ctx, iam.NewUser{
		PersonID: personID, Username: u.username, Email: u.email, PasswordHash: hash,
	})
	if err != nil {
		return err
	}

	for _, r := range u.roles {
		scopeID, err := scopeIDFor(ctx, tx, r)
		if err != nil {
			return err
		}
		if err := iamRepo.AssignRole(ctx, userID, r.code, r.scope, scopeID); err != nil {
			return fmt.Errorf("%s rolü: %w", r.code, err)
		}
	}
	return nil
}

func scopeIDFor(ctx context.Context, tx pgx.Tx, r role) (string, error) {
	switch r.scope {
	case iam.ScopeFaculty:
		return lookupID(ctx, tx, "org.faculties", r.scopeCode)
	case iam.ScopeDepartment:
		return lookupID(ctx, tx, "org.departments", r.scopeCode)
	default:
		return "", nil
	}
}

// lookupID, org tablolarında koda göre kimlik bulur. table sabit bir değerdir,
// asla kullanıcı girdisinden gelmez (tablo adı parametre olarak gönderilemez).
func lookupID(ctx context.Context, tx pgx.Tx, table, code string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, "SELECT id FROM "+table+" WHERE code = $1", code).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("%s içinde %q kodu yok (önce 'make seed' ile organizasyon verisini yükleyin)", table, code)
	}
	return id, err
}

func roleCodes(rs []role) []string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.code)
	}
	return out
}
