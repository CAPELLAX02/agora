package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
)

// loadSQLSeeds, depo kökündeki geliştirme SQL seed'lerini uygular.
func loadSQLSeeds(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	files, err := filepath.Glob("../../../infra/seed/dev/*.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("SQL seed dosyaları bulunamadı: %v", err)
	}
	for _, f := range files {
		body, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(context.Background(), string(body)); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
	}
}

func count(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

func TestSynthetic(t *testing.T) {
	pool := dbtest.New(t)
	loadSQLSeeds(t, pool)
	ctx := context.Background()

	cfg := syntheticConfig{Students: 3000, Academics: 400, Seed: 42}
	if err := runSynthetic(ctx, pool, cfg, "$argon2id$sahte"); err != nil {
		t.Fatal(err)
	}

	if n := count(t, pool, `SELECT count(*) FROM enrollment.student_programs`); n != cfg.Students {
		t.Errorf("%d öğrenci kaydı, %d olmalı", n, cfg.Students)
	}
	// Her kişinin hesabı var.
	if persons, users := count(t, pool, `SELECT count(*) FROM people.persons`), count(t, pool, `SELECT count(*) FROM iam.users`); persons != users {
		t.Errorf("%d kişi, %d hesap", persons, users)
	}

	invariants := map[string]string{
		"aktif danışmanı olmayan süren kayıt": `
			SELECT count(*) FROM enrollment.student_programs sp
			WHERE sp.status IN ('PREP', 'ACTIVE', 'FROZEN', 'SUSPENDED')
			  AND NOT EXISTS (SELECT 1 FROM enrollment.advisor_assignments aa
			                  WHERE aa.student_program_id = sp.id AND aa.valid_until IS NULL)`,
		"bölümünde danışman rolü olmayan danışman": `
			SELECT count(*) FROM enrollment.advisor_assignments aa
			JOIN enrollment.student_programs sp ON sp.id = aa.student_program_id
			JOIN org.programs p ON p.id = sp.program_id
			JOIN people.staff st ON st.id = aa.advisor_staff_id
			JOIN iam.users u ON u.person_id = st.person_id
			WHERE NOT EXISTS (
				SELECT 1 FROM iam.role_assignments ra JOIN iam.roles r ON r.id = ra.role_id
				WHERE ra.user_id = u.id AND r.code = 'ADVISOR' AND ra.scope_id = p.department_id)`,
		"başkanı olmayan bölüm": `
			SELECT count(*) FROM org.departments d
			WHERE EXISTS (SELECT 1 FROM org.programs p WHERE p.department_id = d.id AND p.degree_level IN ('BACHELOR', 'ASSOCIATE'))
			  AND NOT EXISTS (SELECT 1 FROM iam.role_assignments ra JOIN iam.roles r ON r.id = ra.role_id
			                  WHERE r.code = 'DEPARTMENT_HEAD' AND ra.scope_id = d.id)`,
		"fakülte öğrenci işleri olmayan birim": `
			SELECT count(DISTINCT d.faculty_id) - (SELECT count(DISTINCT ra.scope_id) FROM iam.role_assignments ra
			    JOIN iam.roles r ON r.id = ra.role_id WHERE r.code = 'FACULTY_REGISTRAR')
			FROM org.departments d JOIN org.programs p ON p.department_id = d.id`,
		"öğrenci rolü olmayan öğrenci hesabı": `
			SELECT count(*) FROM people.students s
			JOIN iam.users u ON u.person_id = s.person_id
			WHERE NOT EXISTS (SELECT 1 FROM iam.role_assignments ra JOIN iam.roles r ON r.id = ra.role_id
			                  WHERE ra.user_id = u.id AND r.code = 'STUDENT')`,
		"hazırlıkta olup sınıfı 0 olmayan": `
			SELECT count(*) FROM enrollment.student_programs WHERE (status = 'PREP') <> (class_level = 0)`,
		"ilk yılında not ortalaması olan": `
			SELECT count(*) FROM enrollment.student_programs WHERE current_semester = 1 AND gpa_cache IS NOT NULL`,
	}
	for name, q := range invariants {
		if n := count(t, pool, q); n != 0 {
			t.Errorf("%s: %d", name, n)
		}
	}

	rows, err := pool.Query(ctx, `SELECT student_no FROM people.students`)
	if err != nil {
		t.Fatal(err)
	}
	pattern := regexp.MustCompile(`^[0-9]{8}$`)
	for rows.Next() {
		var no string
		_ = rows.Scan(&no)
		if !pattern.MatchString(no) {
			t.Errorf("öğrenci numarası biçimi: %s", no)
		}
	}

	// Tekrar çalıştırmak hiçbir şey eklemez.
	before := count(t, pool, `SELECT count(*) FROM people.persons`)
	if err := runSynthetic(ctx, pool, cfg, "x"); err != nil {
		t.Fatal(err)
	}
	if after := count(t, pool, `SELECT count(*) FROM people.persons`); after != before {
		t.Errorf("ikinci çalıştırma %d kişi ekledi", after-before)
	}
}

// TestSyntheticDeterministic, aynı tohumun aynı kişileri ürettiğini doğrular: hata
// ayıklarken ve yük testlerinde veri setini yeniden üretebilmek için.
func TestSyntheticDeterministic(t *testing.T) {
	names := func(seed uint64) [][]any {
		g := &generator{female: words(femaleNames), male: words(maleNames), last: words(surnames), now: time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC)}
		g.rng = newRand(seed)
		for range 50 {
			g.newPerson(18, 30)
		}
		var out [][]any
		for _, p := range g.persons {
			out = append(out, p[1:]) // kimlik zaman damgası içerir, karşılaştırma dışı
		}
		return out
	}
	if !reflect.DeepEqual(names(7), names(7)) {
		t.Error("aynı tohum farklı veri üretti")
	}
	if reflect.DeepEqual(names(7), names(8)) {
		t.Error("farklı tohumlar aynı veriyi üretti")
	}
}
