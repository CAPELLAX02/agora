package main

import (
	"context"
	"testing"
	"time"

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
	for range 2 { // ikinci çalıştırma bir şey değiştirmemeli
		if err := enrollDemoStudents(ctx, pool); err != nil {
			t.Fatal(err)
		}
		if err := seedFall2026(ctx, pool); err != nil {
			t.Fatal(err)
		}
		if err := linkCurricula(ctx, pool); err != nil {
			t.Fatal(err)
		}
		if err := seedHistory(ctx, pool, 20261008); err != nil {
			t.Fatal(err)
		}
		if err := seedGradeStats(ctx, pool); err != nil {
			t.Fatal(err)
		}
		if err := seedTermResults(ctx, pool); err != nil {
			t.Fatal(err)
		}
		if err := openDemoWindows(ctx, pool, time.Now()); err != nil {
			t.Fatal(err)
		}
		if err := seedRegistrations(ctx, pool); err != nil {
			t.Fatal(err)
		}
		if err := seedDemoScores(ctx, pool, 20261008); err != nil {
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
	// 2025 bahar yönetmeliğin 2023 değişikliğinden sonra: devamsızlık harfi F4.
	if got := letters("25290011", "COM1002"); len(got) != 1 || got[0] != "F4" {
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
		JOIN curriculum.grade_scale_items gi ON gi.grade_scale_id = e.grade_scale_id AND gi.letter = e.letter_grade
		WHERE s.student_no IN ('22290001', '23290013', '25290016') AND NOT gi.is_passing`); n != 0 {
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
	// Son not geçerli: notu verilmiş denemelerden sadece sonuncusu bir sonrakine bağlı değil.
	if n := count(t, pool, `
		SELECT count(*) FROM (
			SELECT student_program_id, course_id FROM enrollment.enrollments
			WHERE superseded_by_id IS NULL AND status = 'COMPLETED' GROUP BY 1, 2 HAVING count(*) <> 1) x`); n != 0 {
		t.Errorf("%d derste birden fazla geçerli deneme var", n)
	}
	// Geçmiş dönem şubelerinin mevcudu aktarılan kayıtlar kadardır (içinde bulunulan dönemde sepetler).
	if n := count(t, pool, `
		SELECT count(*) FROM offering.sections s JOIN offering.course_offerings o ON o.id = s.offering_id
		JOIN academic.terms t ON t.id = o.term_id
		WHERE NOT t.is_current
		  AND s.enrolled_count <> (SELECT count(*) FROM enrollment.enrollments e WHERE e.section_id = s.id AND e.status <> 'DROPPED')`); n != 0 {
		t.Errorf("%d şubenin mevcudu kayıtlarla tutmuyor", n)
	}
	// Her dönemin notu o dönemin ölçeğiyle: 2023-2024 akademik yılından itibaren AU-LISANS-2023.
	if n := count(t, pool, `
		SELECT count(*) FROM enrollment.enrollments e
		JOIN academic.terms t ON t.id = e.term_id JOIN academic.academic_years ay ON ay.id = t.academic_year_id
		JOIN curriculum.grade_scales g ON g.id = e.grade_scale_id
		WHERE e.source = 'IMPORT' AND (g.code = 'AU-LISANS-2023') <> (ay.start_year >= 2023)`); n != 0 {
		t.Errorf("%d aktarılan not yanlış ölçekte", n)
	}
	// Notu verilmiş her şubenin sınıf istatistiği vardır ve kayıtlarla tutar.
	if n := count(t, pool, `
		SELECT count(*) FROM (SELECT DISTINCT section_id FROM enrollment.enrollments WHERE status = 'COMPLETED') e
		LEFT JOIN grading.section_grade_stats g ON g.section_id = e.section_id
		WHERE g.section_id IS NULL
		   OR g.student_count <> (SELECT count(*) FROM enrollment.enrollments x WHERE x.section_id = e.section_id AND x.status = 'COMPLETED')`); n != 0 {
		t.Errorf("%d şubenin sınıf istatistiği eksik ya da tutmuyor", n)
	}
	// Dönem sonuçları: 7. yarıyıldaki öğrencinin altı dönemi var; önbellek son dönemin GANO'su.
	if n := count(t, pool, `
		SELECT count(*) FROM enrollment.term_results r
		JOIN enrollment.student_programs sp ON sp.id = r.student_program_id JOIN people.students s ON s.id = sp.student_id
		WHERE s.student_no = '23290013'`); n != 6 {
		t.Errorf("7. yarıyıldaki öğrencinin %d dönem sonucu var, 6 olmalı", n)
	}
	if n := count(t, pool, `
		SELECT count(*) FROM enrollment.student_programs sp
		WHERE EXISTS (SELECT 1 FROM enrollment.enrollments e WHERE e.student_program_id = sp.id AND e.status = 'COMPLETED')
		  AND NOT EXISTS (
		        SELECT 1 FROM (SELECT r.cumulative_gpa, r.cumulative_ects FROM enrollment.term_results r
		                       JOIN academic.terms t ON t.id = r.term_id
		                       WHERE r.student_program_id = sp.id ORDER BY t.starts_on DESC LIMIT 1) last
		        WHERE last.cumulative_gpa IS NOT DISTINCT FROM sp.gpa_cache AND last.cumulative_ects = sp.earned_ects_cache)`); n != 0 {
		t.Errorf("%d öğrencinin GANO önbelleği son dönem sonucuyla tutmuyor", n)
	}
	if n := count(t, pool, `SELECT count(*) FROM enrollment.student_programs WHERE current_semester = 7 AND class_level = 4`); n != 4 {
		t.Errorf("7. yarıyıldaki demo öğrencisi sayısı = %d, 4 olmalı", n)
	}

	// Demo pencereleri ve P10001'in kuyruğu.
	if n := count(t, pool, `SELECT count(*) FROM academic.calendar_events WHERE scope_type = 'PROGRAM' AND note LIKE 'Geliştirme ortamı demo%'`); n != 5 {
		t.Errorf("%d demo penceresi, 5 olmalı", n)
	}
	if n := count(t, pool, `
		SELECT count(*) FROM academic.calendar_events WHERE scope_type = 'FACULTY' AND note LIKE 'Geliştirme ortamı demo%'
		  AND event_type_code LIKE '%GRADE%' AND period @> now()`); n != 4 {
		t.Errorf("%d açık not girişi demo penceresi, 4 olmalı", n)
	}
	for no, want := range map[string]string{
		"23290013": "SUBMITTED", "25290016": "SUBMITTED", "26290017": "SUBMITTED", "23290015": "APPROVED", "24290012": "DRAFT",
	} {
		var status string
		if err := pool.QueryRow(ctx, `
			SELECT r.status FROM enrollment.registrations r JOIN enrollment.student_programs sp ON sp.id = r.student_program_id
			JOIN people.students s ON s.id = sp.student_id WHERE s.student_no = $1`, no).Scan(&status); err != nil || status != want {
			t.Errorf("%s kaydı = %q (%v), %s olmalı", no, status, err, want)
		}
	}
	// 1. sınıf öğrencisi dönemin bütün zorunlu derslerini aldı; düşük GANO'lu öğrenci 30 AKTS'yi aşmadı
	// ve alttan dersini aldı.
	if n := count(t, pool, `
		SELECT count(*) FROM enrollment.registration_items i JOIN enrollment.registrations r ON r.id = i.registration_id
		JOIN enrollment.student_programs sp ON sp.id = r.student_program_id JOIN people.students s ON s.id = sp.student_id
		WHERE s.student_no = '26290017'`); n != 9 {
		t.Errorf("1. sınıf sepeti %d ders, 9 olmalı", n)
	}
	var total float64
	var hasRepeat bool
	if err := pool.QueryRow(ctx, `
		SELECT r.total_ects::float8, EXISTS (SELECT 1 FROM enrollment.registration_items i JOIN curriculum.courses c ON c.id = i.course_id
		                                    WHERE i.registration_id = r.id AND c.code = 'COM2067')
		FROM enrollment.registrations r JOIN enrollment.student_programs sp ON sp.id = r.student_program_id
		JOIN people.students s ON s.id = sp.student_id WHERE s.student_no = '24290012'`).Scan(&total, &hasRepeat); err != nil {
		t.Fatal(err)
	}
	if total > 30 || !hasRepeat {
		t.Errorf("24290012 sepeti %v AKTS, alttan ders: %v", total, hasRepeat)
	}
	// Onaylanan kaydın kesin kayıtları oluştu; şube mevcutları sepetlerle tutarlı.
	if n := count(t, pool, `
		SELECT count(*) FROM enrollment.enrollments e JOIN enrollment.student_programs sp ON sp.id = e.student_program_id
		JOIN people.students s ON s.id = sp.student_id WHERE s.student_no = '23290015' AND e.status = 'ENROLLED'`); n == 0 {
		t.Error("onaylanan kaydın kesin kayıtları yok")
	}
	if n := count(t, pool, `
		SELECT count(*) FROM offering.sections s JOIN offering.course_offerings o ON o.id = s.offering_id
		JOIN academic.terms t ON t.id = o.term_id
		WHERE t.is_current AND s.enrolled_count <> (SELECT count(*) FROM enrollment.registration_items i
		                                            WHERE i.section_id = s.id AND i.status = 'ACTIVE')`); n != 0 {
		t.Errorf("%d şubenin mevcudu sepetlerle tutmuyor", n)
	}
	// Not girişi demosu: 1. sınıf grubu danışmanının onayıyla demo sınıfında; ara sınav notları
	// girilmiş ve yayımlanmış (biri girmedi), laboratuvarda iki not eksik, final girilmemiş.
	if n := count(t, pool, `
		SELECT count(*) FROM enrollment.enrollments e
		JOIN offering.sections s ON s.id = e.section_id JOIN offering.course_offerings o ON o.id = s.offering_id
		JOIN curriculum.courses c ON c.id = o.course_id
		JOIN enrollment.student_programs sp ON sp.id = e.student_program_id JOIN people.students st ON st.id = sp.student_id
		WHERE c.code = 'COM1007' AND s.section_code = '1' AND st.student_no LIKE '262901%' AND e.status = 'ENROLLED'`); n != len(cohort) {
		t.Errorf("demo sınıfında %d grup öğrencisi, %d olmalı", n, len(cohort))
	}
	type componentState struct {
		scored, absent int
		published      bool
	}
	got := map[string]componentState{}
	rows, err := pool.Query(ctx, `
		SELECT ac.type_code, count(sc.id) FILTER (WHERE sc.status = 'SCORED'), count(sc.id) FILTER (WHERE sc.status = 'ABSENT'),
		       ac.published_at IS NOT NULL
		FROM grading.assessment_components ac
		JOIN offering.sections s ON s.id = ac.section_id JOIN offering.course_offerings o ON o.id = s.offering_id
		JOIN curriculum.courses c ON c.id = o.course_id
		LEFT JOIN grading.assessment_scores sc ON sc.component_id = ac.id
		WHERE c.code = 'COM1007' AND s.section_code = '1'
		GROUP BY ac.id`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var code string
		var st componentState
		if err := rows.Scan(&code, &st.scored, &st.absent, &st.published); err != nil {
			t.Fatal(err)
		}
		got[code] = st
	}
	students := len(cohort)
	for code, want := range map[string]componentState{
		"MIDTERM": {students - 1, 1, true}, "LAB": {students - 2, 0, false}, "FINAL": {}, "MAKEUP": {},
	} {
		if got[code] != want {
			t.Errorf("%s: %+v, beklenen %+v", code, got[code], want)
		}
	}
	if n := count(t, pool, `
		SELECT count(*) FROM enrollment.advisor_assignments aa JOIN people.staff st ON st.id = aa.advisor_staff_id
		WHERE st.staff_no = 'P10001' AND aa.valid_until IS NULL`); n != 10 {
		t.Errorf("P10001'in %d danışmanlığı var, 10 olmalı (grup başka danışmanda)", n)
	}
}
