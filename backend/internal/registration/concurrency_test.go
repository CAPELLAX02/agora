package registration_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CAPELLAX02/agora/backend/internal/enrollment"
	"github.com/CAPELLAX02/agora/backend/internal/registration"
)

// TestNoOverbooking, ders seçme açılışındaki yarışı canlandırır: kontenjanı 40 olan bir
// şubeye 150 öğrenci aynı anda ders ekler. Koşullu atomik UPDATE yüzünden tam 40 öğrenci
// yer bulur, diğerleri "kontenjan dolu" alır; şube mevcudu sepet satırlarıyla birebir tutar.
// make check bu testi race detector açıkken çalıştırır.
func TestNoOverbooking(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	const students, capacity = 150, 40
	w.exec(fmt.Sprintf(`UPDATE offering.sections SET capacity = %d WHERE id = '%s'`, capacity, w.sections["ALG201"]))
	w.exec(fmt.Sprintf(`
		WITH p AS (
			INSERT INTO people.persons (first_name, last_name)
			SELECT 'Yük', 'Öğrenci ' || g FROM generate_series(1, %d) g RETURNING id
		), s AS (
			INSERT INTO people.students (person_id, student_no)
			SELECT id, '2590' || lpad((row_number() OVER ())::text, 4, '0') FROM p RETURNING id
		)
		INSERT INTO enrollment.student_programs (student_id, program_id, admission_type, admission_year, admitted_on,
		                                         class_level, current_semester, curriculum_id, gpa_cache)
		SELECT id, '%s', 'OSYS', 2025, '2025-09-15', 2, 3, (SELECT id FROM curriculum.curricula), 2.50 FROM s`,
		students, w.program))

	repo := enrollment.NewRepository(w.pool)
	rows, err := w.pool.Query(ctx, `
		SELECT sp.id FROM enrollment.student_programs sp JOIN people.students s ON s.id = sp.student_id
		WHERE s.student_no LIKE '2590%'`)
	if err != nil {
		t.Fatal(err)
	}
	var sps []enrollment.StudentProgram
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		sp, err := repo.StudentProgram(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		sps = append(sps, sp)
	}
	rows.Close()

	svc := registration.NewService(w.pool, func() time.Time { return w.now }, nil)
	var ok, full atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, students)
	for _, sp := range sps {
		wg.Go(func() {
			<-start // hepsi aynı anda
			_, err := svc.AddItem(ctx, "", sp, w.sections["ALG201"])
			switch {
			case err == nil:
				ok.Add(1)
			case errors.Is(err, registration.ErrSectionFull):
				full.Add(1)
			default:
				errs <- err
			}
		})
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("beklenmeyen hata: %v", err)
	}

	if ok.Load() != capacity || full.Load() != students-capacity {
		t.Errorf("%d öğrenci yer buldu, %d dolu aldı; %d/%d olmalı", ok.Load(), full.Load(), capacity, students-capacity)
	}
	var enrolled, items int
	_ = w.pool.QueryRow(ctx, `SELECT enrolled_count FROM offering.sections WHERE id = $1`, w.sections["ALG201"]).Scan(&enrolled)
	_ = w.pool.QueryRow(ctx, `SELECT count(*) FROM enrollment.registration_items WHERE section_id = $1 AND status = 'ACTIVE'`,
		w.sections["ALG201"]).Scan(&items)
	if enrolled != capacity || items != capacity {
		t.Errorf("şube mevcudu %d, sepet satırı %d; ikisi de %d olmalı", enrolled, items, capacity)
	}
}

// TestECTSLimitRace, aynı öğrencinin iki sekmeden aynı anda eklediği derslerin AKTS
// sınırını birlikte aşamadığını doğrular: öğrenci başına advisory lock işlemleri sıraya
// koyar, her ekleme bir öncekinin sonucunu görerek denetlenir.
func TestECTSLimitRace(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	// GANO'su 2.00'ın altında olana 10 AKTS.
	w.exec(`UPDATE curriculum.regulation_parameters
		SET value = '[{"max_gpa": 1.99, "ects": 10}, {"max_gpa": 4.00, "ects": 45}]' WHERE key = 'ects_limit_by_gpa'`)
	w.exec(`UPDATE enrollment.student_programs SET gpa_cache = 1.50 WHERE id = '` + w.berkSP + `'`)
	sp, err := enrollment.NewRepository(w.pool).StudentProgram(ctx, w.berkSP)
	if err != nil {
		t.Fatal(err)
	}
	svc := registration.NewService(w.pool, func() time.Time { return w.now }, nil)

	for range 5 {
		var wg sync.WaitGroup
		start := make(chan struct{})
		for _, course := range []string{"MAT111", "ALG201", "OS301"} { // 6 + 5 + 5 AKTS
			wg.Go(func() {
				<-start
				_, _ = svc.AddItem(ctx, "", sp, w.sections[course])
			})
		}
		close(start)
		wg.Wait()

		var total float64
		_ = w.pool.QueryRow(ctx, `
			SELECT coalesce(sum(i.ects), 0)::float8 FROM enrollment.registration_items i
			JOIN enrollment.registrations r ON r.id = i.registration_id WHERE r.student_program_id = $1`, w.berkSP).Scan(&total)
		if total > 10 {
			t.Fatalf("sepet %v AKTS, sınır 10", total)
		}
		w.exec(`DELETE FROM enrollment.registration_items; UPDATE offering.sections SET enrolled_count = 0;
			UPDATE enrollment.registrations SET total_ects = 0`)
	}
}
