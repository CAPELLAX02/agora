package main

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
)

// TestFall2026, elle kurulan 2026 güz programının gerçek SQL seed'leri üzerinde bütün
// çakışma ve kapasite denetimlerinden geçtiğini ve seed'in tekrar çalıştırılabildiğini
// doğrular: programdaki bir değişiklik derslik, öğretim elemanı ya da şube çakışması
// oluşturursa bu test kırılır.
func TestFall2026(t *testing.T) {
	pool := dbtest.New(t)
	loadSQLSeeds(t, pool)
	ctx := context.Background()

	for _, u := range users {
		if err := db.InTx(ctx, pool, func(tx pgx.Tx) error { return createUser(ctx, tx, u, "$argon2id$sahte") }); err != nil {
			t.Fatalf("%s: %v", u.username, err)
		}
	}
	if err := enrollDemoStudents(ctx, pool); err != nil {
		t.Fatal(err)
	}
	for range 2 { // ikinci çalıştırma bir şey değiştirmemeli
		if err := seedFall2026(ctx, pool); err != nil {
			t.Fatal(err)
		}
		if err := linkCurricula(ctx, pool); err != nil {
			t.Fatal(err)
		}
	}

	if n := count(t, pool, `SELECT count(*) FROM offering.course_offerings WHERE status = 'OPEN'`); n != len(fall2026) {
		t.Errorf("%d açık ders, %d olmalı", n, len(fall2026))
	}
	// Her şubenin tamamlanmış bir değerlendirme planı var (dönem içi + final = 100, bütünleme).
	if n := count(t, pool, `
		SELECT count(*) FROM offering.sections s
		WHERE (SELECT coalesce(sum(weight) FILTER (WHERE type_code <> 'MAKEUP'), 0) FROM grading.assessment_components WHERE section_id = s.id) <> 100
		   OR NOT EXISTS (SELECT 1 FROM grading.assessment_components WHERE section_id = s.id AND type_code = 'MAKEUP')`); n != 0 {
		t.Errorf("%d şubenin planı eksik", n)
	}
	// Teorik oturumların derslikleri şube kontenjanını alıyor.
	if n := count(t, pool, `
		SELECT count(*) FROM offering.schedule_slots sl
		JOIN offering.sections s ON s.id = sl.section_id JOIN org.classrooms c ON c.id = sl.classroom_id
		WHERE sl.session_type = 'THEORY' AND c.capacity < s.capacity`); n != 0 {
		t.Errorf("%d teorik oturumun dersliği küçük", n)
	}
	// Demo öğrencileri (2022 girişli) 2022 ders planına bağlı.
	if n := count(t, pool, `
		SELECT count(*) FROM enrollment.student_programs sp JOIN curriculum.curricula c ON c.id = sp.curriculum_id
		WHERE c.effective_from_year = 2018`); n != 2 {
		t.Errorf("2022 planına bağlı %d öğrenci, 2 olmalı", n)
	}
	if n := count(t, pool, `SELECT count(*) FROM curriculum.curricula WHERE status = 'ACTIVE'`); n != 3 {
		t.Errorf("%d yürürlükte müfredat, 3 olmalı", n)
	}
}
