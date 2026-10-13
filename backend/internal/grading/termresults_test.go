package grading_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/CAPELLAX02/agora/backend/internal/grading"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/outbox"
)

// TestTermResults: harf notu ve bütünleme ilanlarından sonra grades.published tüketicisi
// dönem sonuçlarını ve GANO önbelleğini üretir. Tekrar alınan ders ve kalınan seçmelinin
// yerine alınan ders önceki denemeyi kapatır; geçmiş dönemin sonucu değişmez. Bütünlemeyle
// geçilen dönemde ve disiplin cezası alınan dönemde onur verilmez.
func TestTermResults(t *testing.T) {
	s := newGradebookServer(t)
	fx := s.fx
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := s.pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	s.addGrant(fx.ayseUser, authz.Grant{Permission: "grade:finalize", ScopeType: authz.ScopeDepartment, ScopeID: fx.bil, RequiresMFA: true})
	// Tek dersle onur denenebilsin: yarıyıl yükü 6 AKTS.
	exec(`UPDATE curriculum.regulation_parameters SET value = '6' WHERE key = 'semester_ects_load'`)

	// 2025 güz: Ali COM1001'den, Ece seçmeli COM1009'dan kaldı.
	exec(`
		INSERT INTO academic.academic_years (start_year, starts_on, ends_on) VALUES (2025, '2025-09-01', '2026-08-31');
		INSERT INTO academic.terms (academic_year_id, term_type, code, starts_on, ends_on, status)
			SELECT id, 'FALL', '2025-FALL', '2025-09-22', '2026-01-31', 'CLOSED' FROM academic.academic_years WHERE start_year = 2025;
		INSERT INTO curriculum.courses (code, owner_department_id, name_tr, name_en, theory_hours, national_credit, ects, language)
			SELECT 'COM1009', id, 'Veri Görselleştirme', 'Data Visualization', 3, 3, 6, 'EN' FROM org.departments;
		INSERT INTO offering.course_offerings (term_id, course_id, department_id)
			SELECT t.id, c.id, d.id FROM academic.terms t, curriculum.courses c, org.departments d
			WHERE t.code = '2025-FALL' AND c.code IN ('COM1001', 'COM1009');
		INSERT INTO offering.sections (offering_id, section_code, capacity, language)
			SELECT o.id, '1', 60, 'EN' FROM offering.course_offerings o JOIN academic.terms t ON t.id = o.term_id WHERE t.code = '2025-FALL';`)
	failed := func(enrollmentID, courseCode string) string {
		t.Helper()
		var id string
		if err := s.pool.QueryRow(ctx, `
			INSERT INTO enrollment.enrollments (student_program_id, section_id, term_id, course_id, attempt_no, status,
				grade_scale_id, letter_grade, ects_snapshot, credit_snapshot, source)
			SELECT e.student_program_id, s.id, o.term_id, o.course_id, 1, 'COMPLETED',
			       (SELECT id FROM curriculum.grade_scales WHERE is_default), 'F2', 6, 5, 'IMPORT'
			FROM enrollment.enrollments e, offering.sections s
			JOIN offering.course_offerings o ON o.id = s.offering_id
			JOIN academic.terms t ON t.id = o.term_id
			JOIN curriculum.courses c ON c.id = o.course_id
			WHERE e.id = $1 AND t.code = '2025-FALL' AND c.code = $2
			RETURNING id`, enrollmentID, courseCode).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	aliOld, eceOld := failed(fx.enrollments[0], "COM1001"), failed(fx.enrollments[1], "COM1009")

	// Ece COM1001'i kaldığı seçmelinin yerine (aynı havuzdan) aldı.
	exec(`
		INSERT INTO curriculum.elective_groups (code, name_tr, name_en, group_kind)
			VALUES ('TEKSEC', 'Teknik seçmeli', 'Technical elective', 'TECHNICAL');
		INSERT INTO curriculum.elective_group_courses (elective_group_id, course_id)
			SELECT g.id, c.id FROM curriculum.elective_groups g, curriculum.courses c WHERE c.code IN ('COM1001', 'COM1009');
		INSERT INTO curriculum.curricula (program_id, name_tr, name_en, effective_from_year, total_ects_required)
			SELECT id, 'Müfredat', 'Curriculum', 2022, 240 FROM org.programs;
		INSERT INTO curriculum.curriculum_items (curriculum_id, semester_no, item_type, elective_group_id,
				slot_theory_hours, slot_practice_hours, slot_national_credit, slot_ects, slot_course_count, is_compulsory)
			SELECT c.id, 5, 'ELECTIVE_SLOT', g.id, 3, 0, 3, 6, 1, false FROM curriculum.curricula c, curriculum.elective_groups g;`)
	exec(`
		WITH r AS (
			INSERT INTO enrollment.registrations (student_program_id, term_id)
			SELECT student_program_id, term_id FROM enrollment.enrollments WHERE id = $1 RETURNING id
		), i AS (
			INSERT INTO enrollment.registration_items (registration_id, section_id, course_id, item_kind, curriculum_item_id, ects)
			SELECT r.id, e.section_id, e.course_id, 'ELECTIVE_REPLACEMENT', ci.id, 6
			FROM r, enrollment.enrollments e, curriculum.curriculum_items ci WHERE e.id = $1 RETURNING id
		)
		UPDATE enrollment.enrollments SET registration_item_id = (SELECT id FROM i) WHERE id = $1`, fx.enrollments[1])

	section := "/api/v1/sections/" + fx.section
	scores := func(code string) string { return "/api/v1/assessment-components/" + fx.components[code] + "/scores" }
	item := func(i int, score any) map[string]any {
		return map[string]any{"enrollment_id": fx.enrollments[i], "score": score, "absent": false}
	}
	put := func(path, version string, items ...map[string]any) {
		t.Helper()
		if res := s.do("PUT", path, fx.ayseUser, with(mfa, "If-Match", version), map[string]any{"items": items}); res.status != 200 {
			t.Fatalf("notlar: %d %s", res.status, res.raw)
		}
	}
	if res := s.do("POST", section+"/assessment-plan/lock", fx.ayseUser, nil, nil); res.status != 200 {
		t.Fatalf("kilit: %d %s", res.status, res.raw)
	}
	s.calendar.set(grading.WindowFinalEntry, true)
	// Ali 88 (B1), Ece 50 (F2), Can devamsız (F4).
	put(scores("MIDTERM"), `"1"`, item(0, 80), item(1, 60), item(2, 90))
	put(scores("HOMEWORK"), `"1"`, item(0, 100), item(1, 50), item(2, 90))
	put(scores("FINAL"), `"1"`, item(0, 90), item(1, 45), item(2, 90))
	if res := s.do("PUT", section+"/attendance", fx.ayseUser, mfa, map[string]any{"items": []any{
		map[string]any{"enrollment_id": fx.enrollments[2], "attendance_status": "ATTENDANCE_FAIL"}}}); res.status != 200 {
		t.Fatalf("devam: %d %s", res.status, res.raw)
	}
	if res := s.do("POST", section+"/letter-grades/publish", fx.ayseUser, mfa, nil); res.status != 200 {
		t.Fatalf("ilan: %d %s", res.status, res.raw)
	}

	for old, by := range map[string]string{aliOld: fx.enrollments[0], eceOld: fx.enrollments[1]} {
		var superseded *string
		if err := s.pool.QueryRow(ctx, `SELECT superseded_by_id FROM enrollment.enrollments WHERE id = $1`, old).Scan(&superseded); err != nil ||
			superseded == nil || *superseded != by {
			t.Errorf("önceki deneme kapanmalı: %v %v", superseded, err)
		}
	}

	dispatcher := outbox.NewDispatcher(s.pool, []outbox.Consumer{grading.TermResultsUpdater{}}, outbox.DefaultConfig,
		slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	process := func() {
		t.Helper()
		if _, err := dispatcher.ProcessBatch(ctx); err != nil {
			t.Fatal(err)
		}
		var pending int
		if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM platform.outbox_events WHERE status <> 'DONE'`).Scan(&pending); err != nil || pending != 0 {
			t.Fatalf("işlenmemiş olay = %d, %v", pending, err)
		}
	}
	// results, öğrencinin dönem sonuçlarıdır: "dönem alınan/kazanılan YANO kümülatif GANO durum".
	results := func(no string) string {
		t.Helper()
		rows, err := s.pool.Query(ctx, `
			SELECT t.code || ' ' || r.attempted_ects || '/' || r.earned_ects || ' ' || coalesce(r.term_gpa::text, '-') || ' ' ||
			       r.cumulative_ects || ' ' || coalesce(r.cumulative_gpa::text, '-') || ' ' || r.standing
			FROM enrollment.term_results r
			JOIN academic.terms t ON t.id = r.term_id
			JOIN enrollment.student_programs sp ON sp.id = r.student_program_id
			JOIN people.students st ON st.id = sp.student_id
			WHERE st.student_no = $1 ORDER BY t.starts_on`, no)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err != nil {
				t.Fatal(err)
			}
			out = append(out, line)
		}
		return strings.Join(out, "; ")
	}
	cache := func(no string) string {
		t.Helper()
		var v string
		if err := s.pool.QueryRow(ctx, `
			SELECT coalesce(sp.gpa_cache::text, '-') || ' ' || sp.earned_ects_cache FROM enrollment.student_programs sp
			JOIN people.students st ON st.id = sp.student_id WHERE st.student_no = $1`, no).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	check := func(no, wantResults, wantCache string) {
		t.Helper()
		if got := results(no); got != wantResults {
			t.Errorf("%s dönem sonuçları:\n got  %s\n want %s", no, got, wantResults)
		}
		if got := cache(no); got != wantCache {
			t.Errorf("%s önbellek = %s, beklenen %s", no, got, wantCache)
		}
	}

	process()
	// Ali: güzün sonucu tekrardan etkilenmez; 2026 güzde F2 kapandı, GANO sadece B1. YANO 3,50:
	// 2022'den beri yüksek onur 3,70 (md. 42), onur.
	check("22290101", "2025-FALL 6.0/0.0 0.00 0.0 0.00 NORMAL; 2026-FALL 6.0/6.0 3.50 6.0 3.50 HONOR", "3.50 6.0")
	// Ece: kalınan seçmeli (COM1009) kapandı, yerine alınan COM1001'den de kaldı.
	check("22290102", "2025-FALL 6.0/0.0 0.00 0.0 0.00 NORMAL; 2026-FALL 6.0/0.0 0.00 0.0 0.00 NORMAL", "0.00 0.0")
	check("22290103", "2026-FALL 6.0/0.0 0.00 0.0 0.00 NORMAL", "0.00 0.0")

	// Bütünleme: Ece 70 → C2. Bütünlemeyle geçilen dönemde onur yok.
	s.calendar.set(grading.WindowMakeupEntry, true)
	makeup := ""
	for _, c := range s.do("GET", section+"/gradebook", fx.ayseUser, mfa, nil).body["components"].([]any) {
		if m := c.(map[string]any); m["type"].(map[string]any)["code"] == "MAKEUP" {
			makeup = "/api/v1/assessment-components/" + m["id"].(string) + "/scores"
		}
	}
	put(makeup, `"1"`, item(1, 70))
	if res := s.do("POST", section+"/makeup/publish", fx.ayseUser, mfa, nil); res.status != 200 {
		t.Fatalf("bütünleme ilanı: %d %s", res.status, res.raw)
	}
	process()
	check("22290102", "2025-FALL 6.0/0.0 0.00 0.0 0.00 NORMAL; 2026-FALL 6.0/6.0 2.50 6.0 2.50 NORMAL", "2.50 6.0")

	// Disiplin cezası (dönem içinde konan uzaklaştırma) onuru kaldırır. Yeniden hesap
	// idempotenttir ve değişmeyen önbelleğin sürümünü artırmaz.
	exec(`
		INSERT INTO enrollment.holds (student_program_id, hold_type, blocks, reason)
		SELECT student_program_id, 'DISCIPLINE_SUSPENSION', ARRAY['REGISTRATION'], 'Uzaklaştırma' FROM enrollment.enrollments WHERE id = $1`,
		fx.enrollments[0])
	var spID string
	var version int
	if err := s.pool.QueryRow(ctx, `
		SELECT sp.id, sp.version FROM enrollment.student_programs sp JOIN enrollment.enrollments e ON e.student_program_id = sp.id
		WHERE e.id = $1`, fx.enrollments[0]).Scan(&spID, &version); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := grading.RecomputeTermResults(ctx, s.pool, []string{spID}); err != nil {
			t.Fatal(err)
		}
	}
	check("22290101", "2025-FALL 6.0/0.0 0.00 0.0 0.00 NORMAL; 2026-FALL 6.0/6.0 3.50 6.0 3.50 NORMAL", "3.50 6.0")
	var after int
	if err := s.pool.QueryRow(ctx, `SELECT version FROM enrollment.student_programs WHERE id = $1`, spID).Scan(&after); err != nil || after != version {
		t.Errorf("değişmeyen önbellek sürümü %d → %d", version, after)
	}
}
