package grading_test

import (
	"context"
	"slices"
	"testing"

	"github.com/CAPELLAX02/agora/backend/internal/grading"
	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
)

// TestRecomputeAll: toplu doğrulama bozulmuş dönem sonuçlarını ve önbellekleri bulur ama
// yazmaz; -apply ile sadece farkı olan öğrenciler düzelir, ikinci çalıştırma bir şey bulmaz.
// Notu olmadan GANO önbelleği dolu kayıt sayılır, dokunulmaz.
func TestRecomputeAll(t *testing.T) {
	pool := dbtest.New(t)
	fx := seedClass(t, pool)
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	// Ali A, Ece F2, Can B1: notlar verilmiş, sonuçlar tutarlı.
	for i, letter := range []string{"A", "F2", "B1"} {
		exec(`
			UPDATE enrollment.enrollments SET status = 'COMPLETED', letter_grade = $2,
			       grade_scale_id = (SELECT id FROM curriculum.grade_scales WHERE is_default)
			WHERE id = $1`, fx.enrollments[i], letter)
	}
	var spIDs []string
	rows, err := pool.Query(ctx, `SELECT student_program_id FROM enrollment.enrollments WHERE id = ANY($1)`, fx.enrollments)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		spIDs = append(spIDs, id)
	}
	rows.Close()
	if err := grading.RecomputeTermResults(ctx, pool, spIDs); err != nil {
		t.Fatal(err)
	}
	opts := grading.RecomputeOptions{Workers: 2, Batch: 1}
	r, err := grading.RecomputeAll(ctx, pool, opts)
	if err != nil || r.Students != 3 || r.Results != 3 || r.Drifted != 0 {
		t.Fatalf("tutarlı veri: %+v, %v", r, err)
	}

	// Bozulmalar: Ali'nin YANO'su değişti ve var olmayan bir döneme sonucu eklendi, Ece'nin
	// sonucu silindi, Can'ın GANO önbelleği değişti; sentetik bir öğrencinin geçmişi yok ama
	// önbelleği dolu.
	exec(`
		INSERT INTO academic.terms (academic_year_id, term_type, code, starts_on, ends_on, status)
		SELECT id, 'SPRING', '2026-SPRING', '2027-02-15', '2027-06-30', 'PLANNED' FROM academic.academic_years;
		UPDATE enrollment.term_results SET term_gpa = 1.00
		WHERE student_program_id = (SELECT student_program_id FROM enrollment.enrollments WHERE id = '` + fx.enrollments[0] + `');
		INSERT INTO enrollment.term_results (student_program_id, term_id, attempted_ects, earned_ects, cumulative_ects, standing)
		SELECT e.student_program_id, t.id, 0, 0, 0, 'NORMAL' FROM enrollment.enrollments e, academic.terms t
		WHERE e.id = '` + fx.enrollments[0] + `' AND t.code = '2026-SPRING';
		DELETE FROM enrollment.term_results
		WHERE student_program_id = (SELECT student_program_id FROM enrollment.enrollments WHERE id = '` + fx.enrollments[1] + `');
		UPDATE enrollment.student_programs SET gpa_cache = 2.00
		WHERE id = (SELECT student_program_id FROM enrollment.enrollments WHERE id = '` + fx.enrollments[2] + `');
		WITH p AS (INSERT INTO people.persons (first_name, last_name) VALUES ('Sentetik', 'Öğrenci') RETURNING id),
		st AS (INSERT INTO people.students (person_id, student_no) SELECT id, '22290199' FROM p RETURNING id)
		INSERT INTO enrollment.student_programs (student_id, program_id, admission_type, admission_year, admitted_on, gpa_cache)
		SELECT st.id, pr.id, 'OSYS', 2022, '2022-09-15', 3.10 FROM st, org.programs pr;`)

	r, err = grading.RecomputeAll(ctx, pool, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(r.Examples, []string{"22290101", "22290102", "22290103"}) || r.Students != 3 || r.Results != 3 ||
		r.Drifted != 3 || r.RowsMissing != 1 || r.RowsExtra != 1 || r.RowsChanged != 1 ||
		r.CacheChanged != 1 || r.Orphans != 1 || r.Applied {
		t.Errorf("doğrulama = %+v", r)
	}
	var tampered int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM enrollment.term_results WHERE term_gpa = 1.00`).Scan(&tampered); err != nil || tampered != 1 {
		t.Errorf("doğrulama yazmamalı: %d %v", tampered, err)
	}

	opts.Apply = true
	if r, err = grading.RecomputeAll(ctx, pool, opts); err != nil || r.Drifted != 3 || !r.Applied {
		t.Fatalf("yeniden hesap: %+v, %v", r, err)
	}
	opts.Apply = false
	if r, err = grading.RecomputeAll(ctx, pool, opts); err != nil || r.Drifted != 0 || r.Results != 3 {
		t.Errorf("düzeltmeden sonra: %+v, %v", r, err)
	}
	var orphan string
	if err := pool.QueryRow(ctx, `
		SELECT sp.gpa_cache::text FROM enrollment.student_programs sp JOIN people.students st ON st.id = sp.student_id
		WHERE st.student_no = '22290199'`).Scan(&orphan); err != nil || orphan != "3.10" {
		t.Errorf("geçmişi olmayan öğrencinin önbelleğine dokunulmamalı: %s %v", orphan, err)
	}
}
