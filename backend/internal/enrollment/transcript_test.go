package enrollment_test

import (
	"context"
	"testing"

	"github.com/CAPELLAX02/agora/backend/internal/grading"
	"github.com/CAPELLAX02/agora/backend/internal/iam"
)

// TestTranscript: transkript dönem dönem dersleri, tekrar ve yerini sonraki denemeye bırakmış
// satırları, dönem sonuçlarını ve özeti verir. Öğrenci kendisininkini, danışmanı ve kapsamdaki
// birimler görür; başkası (kayıt var mı yok mu dahil) hiçbir şey göremez.
func TestTranscript(t *testing.T) {
	w := newWorld(t)
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := w.pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	r := w.enroll(w.facultyRegistrar, w.student1, w.bilProgram)
	if r.status != 201 {
		t.Fatalf("kayıt: %d %s", r.status, r.raw)
	}
	sp := r.body["id"].(string)
	if r := w.do("PUT", "/api/v1/student-programs/"+sp+"/advisor", w.head,
		map[string]string{"staff_id": w.advisorStaff, "reason": "Danışman dağılımı"}); r.status != 200 {
		t.Fatalf("danışman: %d %s", r.status, r.raw)
	}
	if r := w.enroll(w.facultyRegistrar, w.student2, w.eemProgram); r.status != 201 {
		t.Fatalf("ikinci kayıt: %d %s", r.status, r.raw)
	}
	for _, u := range []string{w.student1User, w.student2User} {
		if err := iam.NewRepository(w.pool).AssignRole(ctx, u, "STUDENT", iam.ScopeNone, ""); err != nil {
			t.Fatal(err)
		}
	}

	// 2025 güz: COM1001 F2, MTH1001 B1. 2025 bahar: COM1001 tekrar C1, COM1002 A. 2026 güz:
	// COM2001 sürüyor.
	exec(`
		INSERT INTO academic.academic_years (start_year, starts_on, ends_on) VALUES
			(2025, '2025-09-01', '2026-08-31'), (2026, '2026-09-01', '2027-08-31');
		INSERT INTO academic.terms (academic_year_id, term_type, code, starts_on, ends_on, status, is_current)
		SELECT ay.id, v.typ, v.code, v.s::date, v.e::date, v.status, v.cur
		FROM (VALUES (2025, 'FALL', '2025-FALL', '2025-09-22', '2026-01-31', 'CLOSED', false),
		             (2025, 'SPRING', '2025-SPRING', '2026-02-16', '2026-06-30', 'CLOSED', false),
		             (2026, 'FALL', '2026-FALL', '2026-09-21', '2027-01-31', 'ACTIVE', true)) AS v (y, typ, code, s, e, status, cur)
		JOIN academic.academic_years ay ON ay.start_year = v.y;
		INSERT INTO curriculum.courses (code, owner_department_id, name_tr, name_en, national_credit, ects, language)
		SELECT v.code, d.id, v.tr, v.en, v.credit, v.ects, 'TR'
		FROM (VALUES ('COM1001', 'Programlama I', 'Programming I', 5, 6), ('MTH1001', 'Matematik I', 'Calculus I', 4, 5),
		             ('COM1002', 'Programlama II', 'Programming II', 5, 6), ('COM2001', 'Veri Yapıları', 'Data Structures', 5, 6))
		     AS v (code, tr, en, credit, ects), org.departments d WHERE d.code = 'BIL';`)
	take := func(term, course string, attempt int, status, letter string) {
		t.Helper()
		exec(`
			WITH o AS (
				INSERT INTO offering.course_offerings (term_id, course_id, department_id)
				SELECT t.id, c.id, c.owner_department_id FROM academic.terms t, curriculum.courses c WHERE t.code = $2 AND c.code = $3
				ON CONFLICT (term_id, course_id) DO UPDATE SET term_id = excluded.term_id RETURNING id, term_id, course_id
			), s AS (
				INSERT INTO offering.sections (offering_id, section_code, capacity, language) SELECT id, '1', 40, 'TR' FROM o RETURNING id
			)
			INSERT INTO enrollment.enrollments (student_program_id, section_id, term_id, course_id, attempt_no, status,
				grade_scale_id, letter_grade, ects_snapshot, credit_snapshot, source)
			SELECT $1, s.id, o.term_id, o.course_id, $4, $5, (SELECT id FROM curriculum.grade_scales WHERE is_default),
			       nullif($6, ''), c.ects, c.national_credit, 'IMPORT'
			FROM o, s, curriculum.courses c WHERE c.id = o.course_id`, sp, term, course, attempt, status, letter)
	}
	take("2025-FALL", "COM1001", 1, "COMPLETED", "F2")
	take("2025-FALL", "MTH1001", 1, "COMPLETED", "B1")
	take("2025-SPRING", "COM1001", 2, "COMPLETED", "C1")
	take("2025-SPRING", "COM1002", 1, "COMPLETED", "A")
	take("2026-FALL", "COM2001", 1, "ENROLLED", "")
	exec(`
		UPDATE enrollment.enrollments old SET superseded_by_id = new.id
		FROM enrollment.enrollments new
		WHERE old.student_program_id = $1 AND new.student_program_id = $1 AND old.course_id = new.course_id
		  AND old.attempt_no = 1 AND new.attempt_no = 2`, sp)
	if err := grading.RecomputeTermResults(ctx, w.pool, []string{sp}); err != nil {
		t.Fatal(err)
	}

	path := "/api/v1/student-programs/" + sp + "/transcript"
	res := w.do("GET", path, w.student1User, nil)
	if res.status != 200 {
		t.Fatalf("transkript: %d %s", res.status, res.raw)
	}
	obj := func(v any) map[string]any { return v.(map[string]any) }
	terms := res.body["terms"].([]any)
	if len(terms) != 3 || obj(obj(terms[0])["term"])["code"] != "2025-FALL" || obj(obj(terms[2])["term"])["code"] != "2026-FALL" {
		t.Fatalf("dönemler = %v", terms)
	}
	course := func(term int, code string) map[string]any {
		for _, c := range obj(terms[term])["courses"].([]any) {
			if obj(obj(c)["course"])["code"] == code {
				return obj(c)
			}
		}
		t.Fatalf("%d. dönemde %s yok", term, code)
		return nil
	}
	if c := course(0, "COM1001"); c["letter_grade"] != "F2" || c["superseded"] != true || c["is_repeat"] != false || c["is_passing"] != false {
		t.Errorf("kalınan ilk deneme = %v", c)
	}
	if c := course(1, "COM1001"); c["letter_grade"] != "C1" || c["superseded"] != false || c["is_repeat"] != true || c["attempt_no"] != 2.0 {
		t.Errorf("tekrar = %v", c)
	}
	if c := course(0, "MTH1001"); c["coefficient"] != 3.5 || c["ects"] != 5.0 || c["credit"] != 4.0 || c["counts_in_gpa"] != true {
		t.Errorf("B1 = %v", c)
	}
	if c := course(2, "COM2001"); c["status"] != "ENROLLED" || c["letter_grade"] != nil {
		t.Errorf("süren ders = %v", c)
	}
	// Güz: (0 × 6 + 3,50 × 5) / 11 = 1,59. Bahar: YANO (2,75 × 6 + 4 × 6) / 12 = 3,38; GANO
	// F2'siz (3,50 × 5 + 2,75 × 6 + 4 × 6) / 17 = 3,41. 2026 güzün henüz sonucu yok.
	result := func(term int) map[string]any {
		r, _ := obj(terms[term])["result"].(map[string]any)
		return r
	}
	if r := result(0); r["term_gpa"] != 1.59 || r["cumulative_gpa"] != 1.59 || r["attempted_ects"] != 11.0 || r["earned_ects"] != 5.0 {
		t.Errorf("güz = %v", r)
	}
	if r := result(1); r["term_gpa"] != 3.38 || r["cumulative_gpa"] != 3.41 || r["cumulative_ects"] != 17.0 || r["standing"] != "NORMAL" {
		t.Errorf("bahar = %v", r)
	}
	if result(2) != nil {
		t.Errorf("süren dönemin sonucu olmamalı: %v", result(2))
	}
	sum := obj(res.body["summary"])
	if sum["gpa"] != 3.41 || sum["earned_ects"] != 17.0 || sum["required_ects"] != 240.0 || sum["completed_courses"] != 4.0 ||
		sum["in_progress"] != 1.0 || res.body["degree_level"] != "BACHELOR" || obj(res.body["student_program"])["id"] != sp {
		t.Errorf("özet = %v / %s", sum, res.raw)
	}

	for name, c := range map[string]struct {
		user string
		want int
	}{
		"danışmanı":                  {w.advisor, 200},
		"bölüm başkanı":              {w.head, 200},
		"fakülte öğrenci işleri":     {w.facultyRegistrar, 200},
		"başka öğrenci":              {w.student2User, 404},
		"danışmanı olmayan danışman": {w.otherAdvisor, 404},
		"ders veren öğretim elemanı": {w.notAdvisor, 404},
	} {
		if r := w.do("GET", path, c.user, nil); r.status != c.want {
			t.Errorf("%s: %d, beklenen %d", name, r.status, c.want)
		}
	}
	if r := w.do("GET", "/api/v1/student-programs/01a11b7f-0000-7000-8000-0000000000ff/transcript", w.head, nil); r.status != 404 {
		t.Errorf("olmayan kayıt: %d", r.status)
	}
}
