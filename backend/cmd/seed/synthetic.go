package main

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed data/female.txt
var femaleNames string

//go:embed data/male.txt
var maleNames string

//go:embed data/surnames.txt
var surnames string

// syntheticConfig, sentetik veri üretiminin ayarlarıdır.
type syntheticConfig struct {
	Students  int
	Academics int
	Seed      uint64 // aynı tohum her seferinde aynı kişileri üretir
}

// uuid sütunları [16]byte olarak taşınır: COPY'nin ikili biçiminde doğrudan yazılır,
// metne çevirip geri ayrıştırmak gerekmez.
type id = [16]byte

type program struct {
	ID, DepartmentID, FacultyID id
	Code, Degree, Education     string
	Semesters                   int
	PrepClass                   bool
	weight                      float64
}

type academic struct {
	staffID, userID id
	title           string
}

// generator, üretilen satırları COPY için tablolara göre biriktirir.
type generator struct {
	rng  *rand.Rand
	now  time.Time
	hash string // bütün sentetik kullanıcıların parola hash'i

	female, male, last []string

	persons, students, staff, users, roles, studentPrograms, advisors [][]any
	roleIDs                                                           map[string]id
}

// runSynthetic, belirtilen sayıda öğrenci ve akademisyen ile gerçekçi bir veri seti
// üretir: bütün programlara dağılmış öğrenciler (hazırlık, uzatmalı, kayıt dondurmuş),
// unvan dağılımı gerçekçi akademik kadro, bölüm başkanları, dekanlar, fakülte öğrenci
// işleri ve her öğrenciye bir danışman.
//
// Bütün veri tek bir transaction'da COPY ile yazılır: yarıda kesilirse geride hiçbir
// şey kalmaz. Veri zaten yüklüyse hiçbir şey yapmaz.
func runSynthetic(ctx context.Context, pool *pgxpool.Pool, cfg syntheticConfig, passwordHash string) error {
	var existing int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM people.students`).Scan(&existing); err != nil {
		return err
	}
	if existing > 1000 {
		fmt.Printf("ATLANDI    sentetik veri zaten yüklü (%d öğrenci)\n", existing)
		return nil
	}

	g := &generator{
		rng:     newRand(cfg.Seed),
		now:     time.Now().UTC(),
		hash:    passwordHash,
		female:  words(femaleNames),
		male:    words(maleNames),
		last:    words(surnames),
		roleIDs: map[string]id{},
	}

	programs, err := loadPrograms(ctx, pool)
	if err != nil {
		return err
	}
	if len(programs) == 0 {
		return fmt.Errorf("program yok: önce 'make seed' ile SQL seed'lerini yükleyin")
	}
	if err := g.loadRoles(ctx, pool); err != nil {
		return err
	}

	start := time.Now()
	studentCounts := distribute(cfg.Students, programs)
	advisorsByDept := g.generateStaff(programs, studentCounts, cfg.Academics)
	g.generateStudents(programs, studentCounts, advisorsByDept)
	fmt.Printf("ÜRETİLDİ   %d kişi, %d öğrenci kaydı, %d personel, %d rol ataması (%s)\n",
		len(g.persons), len(g.studentPrograms), len(g.staff), len(g.roles), time.Since(start).Round(time.Millisecond))

	start = time.Now()
	if err := g.write(ctx, pool); err != nil {
		return err
	}
	fmt.Printf("YAZILDI    COPY ile tek transaction (%s)\n", time.Since(start).Round(time.Millisecond))
	return nil
}

// newRand, tohumdan belirlenimci bir rastgele sayı üreteci oluşturur.
func newRand(seed uint64) *rand.Rand {
	return rand.New(rand.NewPCG(seed, seed^0x9E3779B97F4A7C15))
}

func words(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, w := range strings.Fields(s) {
		if !seen[w] {
			seen[w] = true
			out = append(out, w)
		}
	}
	return out
}

func loadPrograms(ctx context.Context, pool *pgxpool.Pool) ([]program, error) {
	rows, err := pool.Query(ctx, `
		SELECT p.id, p.code, p.degree_level, p.education_type, d.id, d.faculty_id,
		       p.duration_semesters, p.has_prep_class
		FROM org.programs p JOIN org.departments d ON d.id = p.department_id
		WHERE p.is_active AND p.degree_level IN ('BACHELOR', 'ASSOCIATE')
		ORDER BY p.code`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (program, error) {
		var p program
		err := row.Scan(&p.ID, &p.Code, &p.Degree, &p.Education, &p.DepartmentID, &p.FacultyID, &p.Semesters, &p.PrepClass)
		// Büyük programlar (tıp, hukuk, işletme) daha kalabalık, ikinci öğretim ve ön
		// lisans programları daha küçük.
		p.weight = 1
		switch {
		case p.Degree == "ASSOCIATE":
			p.weight = 0.5
		case p.Education == "EVENING":
			p.weight = 0.6
		case strings.HasPrefix(p.Code, "TIPF") || strings.HasPrefix(p.Code, "HUKF") || strings.HasPrefix(p.Code, "ISL"):
			p.weight = 1.8
		}
		return p, err
	})
}

func (g *generator) loadRoles(ctx context.Context, pool *pgxpool.Pool) error {
	rows, err := pool.Query(ctx, `SELECT code, id FROM iam.roles`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			code   string
			roleID id
		)
		if err := rows.Scan(&code, &roleID); err != nil {
			return err
		}
		g.roleIDs[code] = roleID
	}
	return rows.Err()
}

// distribute, toplam öğrenciyi program ağırlıklarına göre paylaştırır.
func distribute(total int, programs []program) []int {
	var sum float64
	for _, p := range programs {
		sum += p.weight
	}
	counts := make([]int, len(programs))
	assigned := 0
	for i, p := range programs {
		counts[i] = int(float64(total) * p.weight / sum)
		assigned += counts[i]
	}
	for i := 0; assigned < total; i = (i + 1) % len(counts) {
		counts[i]++
		assigned++
	}
	return counts
}

// --- Kişiler ------------------------------------------------------------------

type person struct {
	id               id
	first, last, sex string
}

func (g *generator) newPerson(minAge, maxAge int) person {
	p := person{id: g.uuid(), last: g.pick(g.last)}
	if g.rng.IntN(2) == 0 {
		p.first, p.sex = g.pick(g.female), "FEMALE"
	} else {
		p.first, p.sex = g.pick(g.male), "MALE"
	}
	// Bazı kişilerin iki adı olur: "Ayşe Nur", "Mehmet Ali".
	if g.rng.IntN(6) == 0 {
		if p.sex == "FEMALE" {
			p.first += " " + g.pick(g.female)
		} else {
			p.first += " " + g.pick(g.male)
		}
	}
	age := minAge + g.rng.IntN(maxAge-minAge+1)
	birth := time.Date(g.now.Year()-age, time.Month(1+g.rng.IntN(12)), 1+g.rng.IntN(28), 0, 0, 0, 0, time.UTC)
	g.persons = append(g.persons, []any{p.id, p.first, p.last, birth, p.sex})
	return p
}

func (g *generator) newUser(personID id, username, email string) id {
	id := g.uuid()
	g.users = append(g.users, []any{id, personID, username, email, g.hash})
	return id
}

func (g *generator) assign(userID id, role, scopeType string, scopeID id, since time.Time) {
	g.roles = append(g.roles, []any{userID, g.roleIDs[role], scopeType, scopeID, since, "Sentetik seed"})
}

// --- Personel -----------------------------------------------------------------

// titles, akademik kadronun unvan dağılımıdır (yüzde).
var titles = []struct {
	code    string
	percent int
	advisor bool // danışman olabilir mi
	minAge  int
}{
	{"PROF", 20, true, 45},
	{"ASSOC_PROF", 15, true, 38},
	{"ASSIST_PROF", 20, true, 32},
	{"LECTURER_DR", 7, true, 30},
	{"LECTURER", 10, false, 28},
	{"RESEARCH_ASSISTANT_DR", 6, false, 28},
	{"RESEARCH_ASSISTANT", 22, false, 24},
}

func (g *generator) pickTitle() (code string, advisor bool, minAge int) {
	n := g.rng.IntN(100)
	for _, t := range titles {
		if n < t.percent {
			return t.code, t.advisor, t.minAge
		}
		n -= t.percent
	}
	t := titles[len(titles)-1]
	return t.code, t.advisor, t.minAge
}

// generateStaff, akademisyenleri bölümlere öğrenci sayılarıyla orantılı dağıtır, rolleri
// atar ve her bölümün danışmanlarını döndürür.
func (g *generator) generateStaff(programs []program, studentCounts []int, academics int) map[id][]academic {
	studentsByDept := map[id]int{}
	facultyOf := map[id]id{}
	for i, p := range programs {
		studentsByDept[p.DepartmentID] += studentCounts[i]
		facultyOf[p.DepartmentID] = p.FacultyID
	}
	depts := make([]id, 0, len(studentsByDept))
	for d := range studentsByDept {
		depts = append(depts, d)
	}
	slices.SortFunc(depts, compareID) // map sırası rastgele: tohum aynıyken sonuç aynı olsun
	total := 0
	for _, n := range studentsByDept {
		total += n
	}

	since := g.now.AddDate(-1, 0, 0)
	advisors := map[id][]academic{}
	byFaculty := map[id][]academic{}
	staffNo := 0

	for _, dept := range depts {
		n := max(6, academics*studentsByDept[dept]/max(total, 1))
		var members []academic
		for range n {
			title, canAdvise, minAge := g.pickTitle()
			p := g.newPerson(minAge, min(minAge+25, 67))
			staffNo++
			no := fmt.Sprintf("A%06d", staffNo)
			staffID := g.uuid()
			g.staff = append(g.staff, []any{staffID, p.id, no, "ACADEMIC", title, dept})
			userID := g.newUser(p.id, no, strings.ToLower(no)+"@agora.test")

			a := academic{staffID: staffID, userID: userID, title: title}
			members = append(members, a)
			if !strings.HasPrefix(title, "RESEARCH_ASSISTANT") {
				g.assign(userID, "INSTRUCTOR", "DEPARTMENT", dept, since)
			}
			if canAdvise && g.rng.IntN(10) < 7 {
				g.assign(userID, "ADVISOR", "DEPARTMENT", dept, since)
				advisors[dept] = append(advisors[dept], a)
			}
		}
		// Her bölümün en az bir danışmanı olsun.
		if len(advisors[dept]) == 0 {
			a := members[0]
			g.assign(a.userID, "ADVISOR", "DEPARTMENT", dept, since)
			advisors[dept] = append(advisors[dept], a)
		}
		// Bölüm başkanı: bölümün en kıdemli unvanlı üyesi.
		head := members[0]
		for _, m := range members {
			if rank(m.title) < rank(head.title) {
				head = m
			}
		}
		g.assign(head.userID, "DEPARTMENT_HEAD", "DEPARTMENT", dept, since)
		byFaculty[facultyOf[dept]] = append(byFaculty[facultyOf[dept]], members...)
	}

	// Dekan: fakültenin bölüm başkanı olmayan bir profesörü. Fakülte öğrenci işleri:
	// fakülte başına iki idari personel.
	faculties := make([]id, 0, len(byFaculty))
	for f := range byFaculty {
		faculties = append(faculties, f)
	}
	slices.SortFunc(faculties, compareID)
	adminNo := 0
	for _, f := range faculties {
		for _, m := range byFaculty[f] {
			if m.title == "PROF" {
				g.assign(m.userID, "FACULTY_DEAN", "FACULTY", f, since)
				break
			}
		}
		for range 2 {
			p := g.newPerson(25, 60)
			adminNo++
			no := fmt.Sprintf("I%06d", adminNo)
			g.staff = append(g.staff, []any{g.uuid(), p.id, no, "ADMINISTRATIVE", nil, nil})
			userID := g.newUser(p.id, no, strings.ToLower(no)+"@agora.test")
			g.assign(userID, "FACULTY_REGISTRAR", "FACULTY", f, since)
		}
	}
	return advisors
}

func rank(title string) int {
	for i, t := range titles {
		if t.code == title {
			return i
		}
	}
	return len(titles)
}

// --- Öğrenciler ---------------------------------------------------------------

// generateStudents, öğrencileri giriş yıllarına dağıtır ve her süren kayda bölümün
// danışmanlarından birini sırayla atar (yük dengeli).
func (g *generator) generateStudents(programs []program, counts []int, advisors map[id][]academic) {
	// Güz dönemi Eylül'de başlar: Ekim 2026'da içinde bulunulan akademik yıl 2026.
	ay := g.now.Year()
	if g.now.Month() < time.September {
		ay--
	}
	nextAdvisor := map[id]int{}

	for i, p := range programs {
		years := p.Semesters / 2
		seq := map[int]int{}

		for range counts[i] {
			// %88 normal süre içinde, %12 uzatmalı (azami süreye kadar).
			offset := g.rng.IntN(years)
			if g.rng.IntN(100) < 12 {
				offset = years + g.rng.IntN(3)
			}
			admission := ay - offset

			seq[admission]++
			if seq[admission] > 999 {
				continue // numara biçimi bir program ve yılda en fazla 999 öğrenciye izin verir
			}
			no := fmt.Sprintf("%02d%03d%03d", admission%100, i+1, seq[admission])

			pr := g.newPerson(18+offset, 21+offset)
			studentID := g.uuid()
			g.students = append(g.students, []any{studentID, pr.id, no})
			g.newUser(pr.id, no, no+"@ogrenci.agora.test")

			status, class := "ACTIVE", min(offset+1, years)
			if p.PrepClass && offset == 0 && g.rng.IntN(10) < 4 {
				status, class = "PREP", 0
			} else if r := g.rng.IntN(100); r < 2 {
				status = "FROZEN"
			} else if r < 3 {
				status = "SUSPENDED"
			}
			semester := min(offset*2+1, 20)
			if class == 0 {
				semester = 1
			}

			var gpa any
			ects := 0.0
			if offset > 0 && class > 0 {
				v := math.Round(min(4, max(1.2, g.rng.NormFloat64()*0.55+2.65))*100) / 100
				gpa = v
				ects = math.Round(float64(offset*60)*(0.82+g.rng.Float64()*0.18)*2) / 2
			}

			admissionType := "OSYS"
			switch r := g.rng.IntN(100); {
			case r < 4 && p.Degree == "BACHELOR":
				admissionType = "DGS"
			case r < 7:
				admissionType = "YOS"
			case r < 9:
				admissionType = "TRANSFER_EXTERNAL"
			}
			admitted := time.Date(admission, time.September, 10+g.rng.IntN(15), 0, 0, 0, 0, time.UTC)

			spID := g.uuid()
			g.studentPrograms = append(g.studentPrograms, []any{
				spID, studentID, p.ID, "MAJOR", admissionType, admission, admitted, status, class, semester, gpa, ects,
			})

			if list := advisors[p.DepartmentID]; len(list) > 0 {
				a := list[nextAdvisor[p.DepartmentID]%len(list)]
				nextAdvisor[p.DepartmentID]++
				g.advisors = append(g.advisors, []any{spID, a.staffID, admitted, "Sentetik seed"})
			}
		}
	}
}

// --- Yazma --------------------------------------------------------------------

func (g *generator) write(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tables := []struct {
		name    pgx.Identifier
		columns []string
		rows    [][]any
	}{
		{pgx.Identifier{"people", "persons"}, []string{"id", "first_name", "last_name", "birth_date", "gender"}, g.persons},
		{pgx.Identifier{"people", "students"}, []string{"id", "person_id", "student_no"}, g.students},
		{pgx.Identifier{"people", "staff"}, []string{"id", "person_id", "staff_no", "staff_type", "academic_title_code", "primary_department_id"}, g.staff},
		{pgx.Identifier{"iam", "users"}, []string{"id", "person_id", "username", "email", "password_hash"}, g.users},
		{pgx.Identifier{"iam", "role_assignments"}, []string{"user_id", "role_id", "scope_type", "scope_id", "valid_from", "reason"}, g.roles},
		{pgx.Identifier{"enrollment", "student_programs"}, []string{"id", "student_id", "program_id", "enrollment_kind", "admission_type",
			"admission_year", "admitted_on", "status", "class_level", "current_semester", "gpa_cache", "earned_ects_cache"}, g.studentPrograms},
		{pgx.Identifier{"enrollment", "advisor_assignments"}, []string{"student_program_id", "advisor_staff_id", "valid_from", "reason"}, g.advisors},
	}
	for _, t := range tables {
		n, err := tx.CopyFrom(ctx, t.name, t.columns, pgx.CopyFromRows(t.rows))
		if err != nil {
			return fmt.Errorf("%s yazılamadı: %w", t.name.Sanitize(), err)
		}
		fmt.Printf("           %-36s %7d satır\n", t.name.Sanitize(), n)
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	// Planlayıcı istatistikleri: toplu yüklemeden sonra sorgu planları doğru seçilsin.
	_, err = pool.Exec(ctx, `ANALYZE people.persons, people.students, people.staff, iam.users,
		iam.role_assignments, enrollment.student_programs, enrollment.advisor_assignments`)
	return err
}

// --- Yardımcılar --------------------------------------------------------------

func (g *generator) pick(list []string) string {
	return list[g.rng.IntN(len(list))]
}

// uuid, UUIDv7 üretir (RFC 9562): 48 bit milisaniye zaman damgası + rastgele bitler.
// Veritabanındaki uuidv7() ile aynı biçimdedir, böylece birincil anahtarlar zaman
// sıralı kalır ve B-tree index'leri dağılmaz.
func (g *generator) uuid() id {
	var u id
	ms := uint64(time.Now().UnixMilli())
	for i := range 6 {
		u[i] = byte(ms >> (40 - 8*i))
	}
	r1, r2 := g.rng.Uint64(), g.rng.Uint64()
	for i := range 2 {
		u[6+i] = byte(r1 >> (8 * i))
	}
	for i := range 8 {
		u[8+i] = byte(r2 >> (8 * i))
	}
	u[6] = (u[6] & 0x0f) | 0x70 // sürüm 7
	u[8] = (u[8] & 0x3f) | 0x80 // RFC 9562 varyantı
	return u
}

func compareID(a, b id) int {
	return bytes.Compare(a[:], b[:])
}
