package main

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"math/rand/v2"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	academicrepo "github.com/CAPELLAX02/agora/backend/internal/academic"
	"github.com/CAPELLAX02/agora/backend/internal/grading"
)

// Not girişi demosu. Gerçek takvimde 2026 güz not girişi pencereleri kasımda açılır; not
// ekranlarının her gün denenebilmesi için Mühendislik Fakültesine, seed'in çalıştığı andan
// itibaren üç hafta açık kalan fakülte kapsamlı pencereler eklenir (ders seçme demosu gibi,
// yol haritası §5 #13). Not girişi pencereleri şubenin fakültesinde çözüldüğü için sadece
// Mühendislik etkilenir; bütün aşamalar (ara sınav, final, elle harf notu, bütünleme) aynı
// anda açıktır ki ilan ve bütünleme de denenebilsin.
//
// Demo sınıfı: P10001'in 1. sınıf dersi COM1007-1. 20 kişilik kurgusal 1. sınıf grubu ders
// seçme servisiyle sepetini kurar, danışmanı (P10008) onaylar; kesin kayıtlar gerçek akıştan
// gelir. Ara sınav notları girilip yayımlanmıştır (öğrenci "Notlarım"da görür), laboratuvar
// notları girilmiş ama yayımlanmamıştır, final girilmemiştir: öğretim elemanı finali girip
// harf notlarını ilan edebilir.

var demoGradeWindowTypes = []demoWindowType{
	{"MIDTERM_GRADE_ENTRY", "Ara sınav not girişleri (demo)", "Midterm grade entry (demo)"},
	{"FINAL_GRADE_ENTRY", "Final sınavı not girişleri (demo)", "Final grade entry (demo)"},
	{"MANUAL_LETTER_GRADE", "Elle harf notu atama (demo)", "Manual letter grade assignment (demo)"},
	{"MAKEUP_GRADE_ENTRY", "Bütünleme not girişleri (demo)", "Make-up grade entry (demo)"},
}

// Kurgusal 1. sınıf öğrencileri. Gerçek kişilerle ilgisi yoktur.
var cohort = []struct{ no, first, last string }{
	{"26290101", "Ada", "Aydemir"}, {"26290102", "Berk", "Karaca"}, {"26290103", "Ceren", "Doğan"},
	{"26290104", "Doruk", "Ekinci"}, {"26290105", "Ela", "Gündoğdu"}, {"26290106", "Efe", "Halıcı"},
	{"26290107", "Ezgi", "Işık"}, {"26290108", "Furkan", "Kalkan"}, {"26290109", "Gökçe", "Kartal"},
	{"26290110", "Hakan", "Korkmaz"}, {"26290111", "Ilgın", "Mutlu"}, {"26290112", "Kerem", "Oral"},
	{"26290113", "Lale", "Özkan"}, {"26290114", "Mete", "Sarı"}, {"26290115", "Nehir", "Soylu"},
	{"26290116", "Ozan", "Taş"}, {"26290117", "Pelin", "Turan"}, {"26290118", "Rüzgar", "Uslu"},
	{"26290119", "Selin", "Vural"}, {"26290120", "Tuna", "Yalçın"},
}

func cohortUsers() []seedUser {
	out := make([]seedUser, 0, len(cohort))
	for _, c := range cohort {
		out = append(out, student(c.no, c.first, c.last))
	}
	return out
}

func cohortStudents() []demoStudent {
	out := make([]demoStudent, 0, len(cohort))
	for _, c := range cohort {
		out = append(out, demoStudent{no: c.no, admission: 2026, semester: 1, classLevel: 1, advisor: "P10008"})
	}
	return out
}

func cohortRegistrations() []demoRegistration {
	out := make([]demoRegistration, 0, len(cohort))
	for _, c := range cohort {
		out = append(out, demoRegistration{no: c.no, submit: true, approve: true})
	}
	return out
}

// Demo sınıfı ve notları giren öğretim elemanı.
const (
	demoClassCourse  = "COM1007"
	demoClassSection = "1"
	demoInstructor   = "P10001"
)

// seedDemoScores, demo sınıfının ara sınav notlarını girip yayımlar ve laboratuvar notlarını
// girer. Notlar not girişi servisiyle (pencere, kilitli plan ve iyimser kilit denetimiyle)
// yazılır. Ara sınavına not girilmiş sınıfa dokunmaz.
func seedDemoScores(ctx context.Context, pool *pgxpool.Pool, seed uint64) error {
	var sectionID, userID string
	err := pool.QueryRow(ctx, `
		SELECT s.id, u.id FROM offering.sections s
		JOIN offering.course_offerings o ON o.id = s.offering_id
		JOIN academic.terms t ON t.id = o.term_id AND t.is_current
		JOIN curriculum.courses c ON c.id = o.course_id, iam.users u
		WHERE c.code = $1 AND s.section_code = $2 AND u.username = $3`,
		demoClassCourse, demoClassSection, demoInstructor).Scan(&sectionID, &userID)
	if errors.Is(err, pgx.ErrNoRows) {
		fmt.Println("ATLANDI    not girişi demosu: demo sınıfı yok")
		return nil
	}
	if err != nil {
		return fmt.Errorf("demo sınıfı okunamadı: %w", err)
	}

	svc := grading.NewService(pool, academicrepo.NewRepository(pool), time.Now)
	book, err := svc.Gradebook(ctx, sectionID, userID)
	if err != nil {
		return err
	}
	if len(book.Students) == 0 {
		fmt.Println("ATLANDI    not girişi demosu: demo sınıfında öğrenci yok")
		return nil
	}
	var midterm, lab *grading.GradebookComponent
	for i, c := range book.Components {
		switch c.Type.Code {
		case "MIDTERM":
			midterm = &book.Components[i]
		case "LAB":
			lab = &book.Components[i]
		}
	}
	if midterm == nil || lab == nil {
		return fmt.Errorf("demo sınıfının planında ara sınav ve laboratuvar yok")
	}
	if midterm.Scored+midterm.Absent > 0 {
		return nil
	}

	write := func(c *grading.GradebookComponent, score func(no string, rng *rand.Rand) (*grading.Points, bool)) (int, error) {
		ref, err := svc.ComponentRef(ctx, c.ID, userID)
		if err != nil {
			return 0, err
		}
		var items []grading.ScoreInput
		for _, st := range book.Students {
			h := fnv.New64a()
			_, _ = h.Write([]byte(c.Type.Code + st.StudentNo))
			p, absent := score(st.StudentNo, rand.New(rand.NewPCG(seed, h.Sum64())))
			if p == nil && !absent {
				continue
			}
			items = append(items, grading.ScoreInput{EnrollmentID: st.EnrollmentID, Points: p, Absent: absent})
		}
		return svc.SetScores(ctx, userID, ref, c.Version, items)
	}
	points := func(mean, sd float64, rng *rand.Rand) *grading.Points {
		v := math.Round(math.Max(0, math.Min(100, mean+sd*rng.NormFloat64())))
		p := grading.Points(v * 100)
		return &p
	}

	// Ara sınav: ortalama 62, bir öğrenci sınava girmedi. Yayımlanır.
	n, err := write(midterm, func(no string, rng *rand.Rand) (*grading.Points, bool) {
		if no == "26290108" {
			return nil, true
		}
		return points(62, 17, rng), false
	})
	if err != nil {
		return fmt.Errorf("ara sınav notları: %w", err)
	}
	ref, err := svc.ComponentRef(ctx, midterm.ID, userID)
	if err != nil {
		return err
	}
	if err := svc.PublishComponent(ctx, userID, ref); err != nil {
		return fmt.Errorf("ara sınav notları yayımlanamadı: %w", err)
	}
	// Laboratuvar: iki öğrencinin notu henüz girilmedi; yayımlanmadı.
	m, err := write(lab, func(no string, rng *rand.Rand) (*grading.Points, bool) {
		if no == "26290115" || no == "26290119" {
			return nil, false
		}
		return points(78, 12, rng), false
	})
	if err != nil {
		return fmt.Errorf("laboratuvar notları: %w", err)
	}
	fmt.Printf("NOTLAR     %s-%s: %d ara sınav notu (yayımlandı), %d laboratuvar notu\n",
		demoClassCourse, demoClassSection, n, m)
	return nil
}
