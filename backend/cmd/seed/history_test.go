package main

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
)

// TestHistory, demo öğrencilerinin geçmişinin demo senaryolarına uyduğunu, kayıtların
// tutarlı olduğunu (son not geçerli, şube mevcutları, GANO) ve üretimin tekrar
// çalıştırıldığında bir şey eklemediğini doğrular.
func TestHistory(t *testing.T) {
	pool := dbtest.New(t)
	loadSQLSeeds(t, pool)
	ctx := context.Background()
	for _, u := range users {
		if err := db.InTx(ctx, pool, func(tx pgx.Tx) error { return createUser(ctx, tx, u, "$argon2id$sahte") }); err != nil {
			t.Fatalf("%s: %v", u.username, err)
		}
	}
	for range 2 {
		if err := enrollDemoStudents(ctx, pool); err != nil {
			t.Fatal(err)
		}
		if err := linkCurricula(ctx, pool); err != nil {
			t.Fatal(err)
		}
		if err := seedHistory(ctx, pool, 20261008); err != nil {
			t.Fatal(err)
		}
	}

	letters := func(no, course string) []string {
		t.Helper()
		rows, err := pool.Query(ctx, `
			SELECT e.letter_grade FROM enrollment.enrollments e
			JOIN enrollment.student_programs sp ON sp.id = e.student_program_id
			JOIN people.students s ON s.id = sp.student_id
			JOIN curriculum.courses c ON c.id = e.course_id
			JOIN academic.terms t ON t.id = e.term_id
			WHERE s.student_no = $1 AND c.code = $2 ORDER BY t.starts_on`, no, course)
		if err != nil {
			t.Fatal(err)
		}
		list, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			t.Fatal(err)
		}
		return list
	}
	gpa := func(no string) float64 {
		t.Helper()
		var v float64
		if err := pool.QueryRow(ctx, `
			SELECT sp.gpa_cache::float8 FROM enrollment.student_programs sp JOIN people.students s ON s.id = sp.student_id
			WHERE s.student_no = $1`, no).Scan(&v); err != nil {
			t.Fatalf("%s GANO: %v", no, err)
		}
		return v
	}

	if got := letters("22290002", "PHY0101"); len(got) != 2 || got[0] != "F2" || got[1] != "F2" {
		t.Errorf("Fizik I iki kez kalınmalı: %v", got)
	}
	if got := letters("22290002", "COM3035"); len(got) != 1 || got[0] != "F2" {
		t.Errorf("İşletim Sistemleri: %v", got)
	}
	if got := letters("25290011", "COM1002"); len(got) != 1 || got[0] != "F1" {
		t.Errorf("Programlama II devamsızlık: %v", got)
	}
	if g := gpa("22290002"); g < 2 || g >= 3 {
		t.Errorf("22290002 GANO = %.2f, 2.00-2.99 (40 AKTS) olmalı", g)
	}
	if g := gpa("24290012"); g >= 2 {
		t.Errorf("24290012 GANO = %.2f, 2.00'ın altında (30 AKTS) olmalı", g)
	}
	if g := gpa("23290013"); g < 3 {
		t.Errorf("23290013 GANO = %.2f, 3.00 ve üstü (45 AKTS) olmalı", g)
	}
	if n := count(t, pool, `
		SELECT count(*) FROM enrollment.enrollments e
		JOIN enrollment.student_programs sp ON sp.id = e.student_program_id JOIN people.students s ON s.id = sp.student_id
		WHERE s.student_no IN ('22290001', '23290013', '25290016') AND e.letter_grade IN ('F1', 'F2', 'BŞZ')`); n != 0 {
		t.Errorf("hiç kalmamış demo öğrencilerinin %d kalınan dersi var", n)
	}
	// 1. yarıyıldakilerin geçmişi yok; 7. yarıyıldakiler altı dönem ders almış.
	if n := count(t, pool, `
		SELECT count(DISTINCT e.term_id) FROM enrollment.enrollments e
		JOIN enrollment.student_programs sp ON sp.id = e.student_program_id JOIN people.students s ON s.id = sp.student_id
		WHERE s.student_no = '23290013'`); n != 6 {
		t.Errorf("7. yarıyıldaki öğrenci %d dönem ders almış, 6 olmalı", n)
	}
	if n := count(t, pool, `
		SELECT count(*) FROM enrollment.enrollments e
		JOIN enrollment.student_programs sp ON sp.id = e.student_program_id JOIN people.students s ON s.id = sp.student_id
		WHERE s.student_no IN ('26290004', '26290017')`); n != 0 {
		t.Errorf("1. yarıyıldaki öğrencilerin %d geçmiş kaydı var", n)
	}
	// Son not geçerli: her dersin sadece son denemesi bir sonrakine bağlı değil.
	if n := count(t, pool, `
		SELECT count(*) FROM (
			SELECT student_program_id, course_id FROM enrollment.enrollments
			WHERE superseded_by_id IS NULL GROUP BY 1, 2 HAVING count(*) <> 1) x`); n != 0 {
		t.Errorf("%d derste birden fazla geçerli deneme var", n)
	}
	if n := count(t, pool, `
		SELECT count(*) FROM offering.sections s
		WHERE s.enrolled_count <> (SELECT count(*) FROM enrollment.enrollments e WHERE e.section_id = s.id AND e.status <> 'DROPPED')`); n != 0 {
		t.Errorf("%d şubenin mevcudu kayıtlarla tutmuyor", n)
	}
	if n := count(t, pool, `SELECT count(*) FROM enrollment.student_programs WHERE current_semester = 7 AND class_level = 4`); n != 4 {
		t.Errorf("7. yarıyıldaki demo öğrencisi sayısı = %d, 4 olmalı", n)
	}
}
