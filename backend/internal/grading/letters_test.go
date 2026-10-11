package grading_test

import (
	"context"
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CAPELLAX02/agora/backend/internal/grading"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
)

func letterRow(res result, enrollmentID string) map[string]any {
	for _, s := range res.body["students"].([]any) {
		if m := s.(map[string]any); m["enrollment_id"] == enrollmentID {
			return m
		}
	}
	return nil
}

func TestHTTPLetterGradesAndPublish(t *testing.T) {
	s := newGradebookServer(t)
	fx := s.fx
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := s.pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	// Mehmet şubenin ortak öğretim elemanı: not girer ama ilan edemez. İkisi de not
	// kesinleştirme yetkisine (MFA) sahip.
	exec(`INSERT INTO offering.section_instructors (section_id, staff_id, role)
		SELECT $1, st.id, 'CO_INSTRUCTOR' FROM people.staff st WHERE st.staff_no = 'P1002'`, fx.section)
	finalize := authz.Grant{Permission: "grade:finalize", ScopeType: authz.ScopeDepartment, ScopeID: fx.bil, RequiresMFA: true}
	s.addGrant(fx.ayseUser, finalize)
	s.addGrant(fx.mehmetUser, finalize)

	// Önceki denemeler: Ali eşdeğer dersten (COM1007), Ece aynı dersten kalmış.
	exec(`
		INSERT INTO academic.academic_years (start_year, starts_on, ends_on) VALUES (2025, '2025-09-01', '2026-08-31');
		INSERT INTO academic.terms (academic_year_id, term_type, code, starts_on, ends_on, status)
			SELECT id, 'FALL', '2025-FALL', '2025-09-22', '2026-01-31', 'CLOSED' FROM academic.academic_years WHERE start_year = 2025;
		INSERT INTO curriculum.courses (code, owner_department_id, name_tr, name_en, theory_hours, national_credit, ects, language)
			SELECT 'COM1007', id, 'Bilgisayar Programlama I (eski)', 'Computer Programming I (old)', 4, 5, 6, 'EN' FROM org.departments;
		INSERT INTO curriculum.course_equivalences (course_id, equivalent_course_id, note)
			SELECT n.id, o.id, 'Test' FROM curriculum.courses n, curriculum.courses o WHERE n.code = 'COM1001' AND o.code = 'COM1007';
		INSERT INTO offering.course_offerings (term_id, course_id, department_id)
			SELECT t.id, c.id, d.id FROM academic.terms t, curriculum.courses c, org.departments d
			WHERE t.code = '2025-FALL' AND c.code IN ('COM1001', 'COM1007');
		INSERT INTO offering.sections (offering_id, section_code, capacity, language)
			SELECT o.id, '1', 60, 'EN' FROM offering.course_offerings o JOIN academic.terms t ON t.id = o.term_id WHERE t.code = '2025-FALL';`)
	old := func(enrollmentID, courseCode string) string {
		t.Helper()
		var id string
		err := s.pool.QueryRow(ctx, `
			INSERT INTO enrollment.enrollments (student_program_id, section_id, term_id, course_id, attempt_no, status,
				grade_scale_id, letter_grade, ects_snapshot, credit_snapshot, source)
			SELECT e.student_program_id, s.id, o.term_id, o.course_id, 1, 'COMPLETED',
			       (SELECT id FROM curriculum.grade_scales WHERE is_default), 'F2', 6, 5, 'IMPORT'
			FROM enrollment.enrollments e, offering.sections s
			JOIN offering.course_offerings o ON o.id = s.offering_id
			JOIN academic.terms t ON t.id = o.term_id
			JOIN curriculum.courses c ON c.id = o.course_id
			WHERE e.id = $1 AND t.code = '2025-FALL' AND c.code = $2
			RETURNING id`, enrollmentID, courseCode).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	aliOld, eceOld := old(fx.enrollments[0], "COM1007"), old(fx.enrollments[1], "COM1001")

	letters := "/api/v1/sections/" + fx.section + "/letter-grades"
	publish := letters + "/publish"
	overrides := "/api/v1/sections/" + fx.section + "/letter-overrides"
	scores := func(code string) string { return "/api/v1/assessment-components/" + fx.components[code] + "/scores" }
	put := func(code string, version int, items ...map[string]any) {
		t.Helper()
		res := s.do("PUT", scores(code), fx.ayseUser, with(mfa, "If-Match", `"`+string(rune('0'+version))+`"`), map[string]any{"items": items})
		if res.status != 200 {
			t.Fatalf("%s notları: %d %s", code, res.status, res.raw)
		}
	}
	item := func(i int, score any, absent bool) map[string]any {
		return map[string]any{"enrollment_id": fx.enrollments[i], "score": score, "absent": absent}
	}
	if res := s.do("POST", "/api/v1/sections/"+fx.section+"/assessment-plan/lock", fx.ayseUser, nil, nil); res.status != 200 {
		t.Fatalf("kilit: %d %s", res.status, res.raw)
	}
	put("MIDTERM", 1, item(0, 80, false), item(1, nil, true), item(2, 90, false))
	put("HOMEWORK", 1, item(0, 100, false), item(1, 50, false), item(2, 0, false))

	// Final girilmeden harf notu hesaplanmaz; final penceresi kapalıyken ilan edilemez.
	res := s.do("GET", letters, fx.ayseUser, mfa, nil)
	if res.status != 200 || res.body["can_publish"] != false || res.body["published_at"] != nil {
		t.Fatalf("önizleme: %d %s", res.status, res.raw)
	}
	if r := letterRow(res, fx.enrollments[0]); r["complete"] != false || r["letter"] != nil ||
		len(r["missing_components"].([]any)) != 1 || r["missing_components"].([]any)[0] != fx.components["FINAL"] {
		t.Errorf("eksik final: %v", r)
	}
	if res := s.do("POST", publish, fx.ayseUser, mfa, nil); res.status != 409 || res.code() != "PUBLISH_WINDOW_CLOSED" {
		t.Errorf("kapalı ilan penceresi: %d %s", res.status, res.raw)
	}

	// Final: Ali 90 (88 → B1), Ece 45 (baraj altı → F2), Can girilmedi.
	s.calendar.set(grading.WindowFinalEntry, true)
	put("FINAL", 1, item(0, 90, false), item(1, 45, false))
	res = s.do("GET", letters, fx.ayseUser, mfa, nil)
	if r := letterRow(res, fx.enrollments[0]); r["letter"] != "B1" || r["weighted_score"] != 88.0 || r["rounded_score"] != 88.0 || r["reason"] != "RANGE" {
		t.Errorf("Ali: %v", r)
	}
	if r := letterRow(res, fx.enrollments[1]); r["letter"] != "F2" || r["reason"] != "FINAL_BELOW_MIN" || r["passing"] != false {
		t.Errorf("Ece: %v", r)
	}
	if res.body["can_publish"] != false || res.body["summary"].(map[string]any)["complete"] != 2.0 {
		t.Errorf("eksik notla ilan edilemez: %s", res.raw)
	}
	res = s.do("POST", publish, fx.ayseUser, mfa, nil)
	if res.status != 409 || res.code() != "GRADES_INCOMPLETE" || !strings.Contains(res.raw, "Can Çelik (22290103)") || strings.Contains(res.raw, "Ali Yıldız") {
		t.Errorf("eksik not: %d %s", res.status, res.raw)
	}

	// Elle harf notu: sadece kendi penceresinde, ölçekteki uygun harflerle, gerekçeyle.
	override := func(letter any, reason string) map[string]any {
		return map[string]any{"items": []any{map[string]any{"enrollment_id": fx.enrollments[2], "letter": letter, "reason": reason}}}
	}
	if res := s.do("PUT", overrides, fx.ayseUser, mfa, override("C3", "Final mazeret sınavı")); res.status != 409 || res.code() != "MANUAL_LETTER_WINDOW_CLOSED" {
		t.Errorf("kapalı elle not penceresi: %d %s", res.status, res.raw)
	}
	s.calendar.set(grading.WindowManualLetter, true)
	for name, body := range map[string]map[string]any{
		"BŞR harf notlu derste": override("BŞR", "Final mazeret sınavı"),
		"muafiyet":              override("MUAF", "Final mazeret sınavı"),
		"ölçekte yok":           override("D", "Final mazeret sınavı"),
		"gerekçesiz":            override("C3", "yok"),
	} {
		if res := s.do("PUT", overrides, fx.ayseUser, mfa, body); res.status != 400 {
			t.Errorf("%s: %d %s", name, res.status, res.raw)
		}
	}
	if res := s.do("PUT", overrides, fx.mehmetUser, mfa, override("C3", "Final mazeret sınavı")); res.status != 403 {
		t.Errorf("ortak öğretim elemanı elle not veremez: %d", res.status)
	}
	res = s.do("PUT", overrides, fx.ayseUser, mfa, override("C3", "Final mazeret sınavı"))
	if r := letterRow(res, fx.enrollments[2]); res.status != 200 || r["letter"] != "C3" || r["reason"] != "MANUAL" ||
		r["override"].(map[string]any)["set_by"] != "Ayşe Yılmaz" || r["computed_letter"] != nil {
		t.Fatalf("elle harf notu: %d %s", res.status, res.raw)
	}
	if res.body["can_publish"] != true || res.body["can_override"] != true {
		t.Errorf("artık ilan edilebilir: %s", res.raw)
	}

	// İlan: sadece sorumlu öğretim elemanı, MFA'lı oturumda.
	if res := s.do("POST", publish, fx.ayseUser, nil, nil); res.status != 403 || res.code() != "MFA_REQUIRED" {
		t.Errorf("MFA'sız ilan: %d %s", res.status, res.raw)
	}
	if res := s.do("POST", publish, fx.mehmetUser, mfa, nil); res.status != 403 {
		t.Errorf("ortak öğretim elemanı ilan edemez: %d", res.status)
	}
	res = s.do("POST", publish, fx.ayseUser, mfa, nil)
	if res.status != 200 || res.body["published_at"] == nil || res.body["published_by"] != "Ayşe Yılmaz" || res.body["can_publish"] != false {
		t.Fatalf("ilan: %d %s", res.status, res.raw)
	}
	sum := res.body["summary"].(map[string]any)
	if sum["mean"] != 60.0 || sum["median"] != 60.0 || sum["complete"] != 3.0 || sum["pass_rate"].(float64) < 0.66 || sum["pass_rate"].(float64) > 0.67 {
		t.Errorf("özet: %v", sum)
	}

	type graded struct{ status, letter, reason string }
	for i, want := range []graded{{"COMPLETED", "B1", "RANGE"}, {"COMPLETED", "F2", "FINAL_BELOW_MIN"}, {"COMPLETED", "C3", "MANUAL"}} {
		var got graded
		var published bool
		if err := s.pool.QueryRow(ctx, `
			SELECT status, letter_grade, letter_reason, grade_published_at IS NOT NULL FROM enrollment.enrollments WHERE id = $1`,
			fx.enrollments[i]).Scan(&got.status, &got.letter, &got.reason, &published); err != nil || got != want || !published {
			t.Errorf("kesin kayıt %d = %+v (%v), beklenen %+v", i, got, err, want)
		}
	}
	for old, by := range map[string]string{aliOld: fx.enrollments[0], eceOld: fx.enrollments[1]} {
		var superseded *string
		if err := s.pool.QueryRow(ctx, `SELECT superseded_by_id FROM enrollment.enrollments WHERE id = $1`, old).Scan(&superseded); err != nil ||
			superseded == nil || *superseded != by {
			t.Errorf("önceki deneme kapanmalı (son not geçerli): %v %v", superseded, err)
		}
	}
	var events, audits int
	_ = s.pool.QueryRow(ctx, `SELECT count(*) FROM platform.outbox_events WHERE event_type = 'grades.published' AND aggregate_id = $1`, fx.section).Scan(&events)
	_ = s.pool.QueryRow(ctx, `SELECT count(*) FROM audit.audit_log WHERE action IN ('grades.publish', 'letter_override.set')`).Scan(&audits)
	if events != 1 || audits != 2 {
		t.Errorf("olay = %d, denetim = %d", events, audits)
	}

	// İlandan sonra notlar, devam durumu, elle notlar ve plan değişmez; ikinci ilan reddedilir.
	if res := s.do("PUT", scores("FINAL"), fx.ayseUser, with(mfa, "If-Match", `"2"`), map[string]any{"items": []any{item(2, 70, false)}}); res.status != 409 || res.code() != "GRADES_PUBLISHED" {
		t.Errorf("ilan sonrası not: %d %s", res.status, res.raw)
	}
	if res := s.do("PUT", "/api/v1/sections/"+fx.section+"/attendance", fx.ayseUser, mfa,
		map[string]any{"items": []any{map[string]any{"enrollment_id": fx.enrollments[0], "attendance_status": "ATTENDANCE_FAIL"}}}); res.status != 409 {
		t.Errorf("ilan sonrası devam: %d %s", res.status, res.raw)
	}
	if res := s.do("PUT", overrides, fx.ayseUser, mfa, override(nil, "")); res.status != 409 {
		t.Errorf("ilan sonrası elle not: %d %s", res.status, res.raw)
	}
	if res := s.do("POST", publish, fx.ayseUser, mfa, nil); res.status != 409 || res.code() != "GRADES_PUBLISHED" {
		t.Errorf("ikinci ilan: %d %s", res.status, res.raw)
	}
	if res := s.do("POST", "/api/v1/sections/"+fx.section+"/assessment-plan/unlock", head, nil, map[string]any{"reason": "Ağırlık değişti"}); res.status != 409 || res.code() != "GRADES_PUBLISHED" {
		t.Errorf("ilan sonrası plan kilidi: %d %s", res.status, res.raw)
	}
	book := s.do("GET", "/api/v1/sections/"+fx.section+"/gradebook", fx.ayseUser, mfa, nil)
	if book.body["section"].(map[string]any)["grades_published_at"] == nil || component(book, fx.components["MIDTERM"])["editable"] != false {
		t.Errorf("not tablosu ilanı göstermeli: %s", book.raw)
	}
	// İlan edilmiş notlar önizlemede kalıcıdır: hesaplanan değil, kesin kayıttaki harf.
	if r := letterRow(s.do("GET", letters, head, nil, nil), fx.enrollments[2]); r["letter"] != "C3" || r["reason"] != "MANUAL" {
		t.Errorf("ilan edilmiş not: %v", r)
	}
}

// TestPublishRacesScoreWrites: ilan ile not yazımı aynı anda çalışır. Şube kilidi sayesinde
// her not yazımı ya ilandan önce biter (ilan onu görür) ya da ilanı görüp reddedilir: ilan
// anından sonra yazılmış bir not ve ilan edilen notla çelişen bir bileşen notu olamaz.
func TestPublishRacesScoreWrites(t *testing.T) {
	pool := dbtest.New(t)
	fx := seedClass(t, pool)
	ctx := context.Background()
	if err := grading.NewRepository(pool).Lock(ctx, "", fx.section); err != nil {
		t.Fatal(err)
	}
	calendar := &fakeCalendar{open: map[string]bool{grading.WindowMidtermEntry: true, grading.WindowFinalEntry: true}}
	svc := grading.NewService(pool, calendar, time.Now)
	write := func(code string, points grading.Points) error {
		ref, err := svc.ComponentRef(ctx, fx.components[code], fx.ayseUser)
		if err != nil {
			return err
		}
		g, err := svc.Gradebook(ctx, fx.section, fx.ayseUser)
		if err != nil {
			return err
		}
		version := 0
		for _, c := range g.Components {
			if c.ID == ref.ID {
				version = c.Version
			}
		}
		items := make([]grading.ScoreInput, 0, len(fx.enrollments))
		for _, id := range fx.enrollments {
			p := points
			items = append(items, grading.ScoreInput{EnrollmentID: id, Points: &p})
		}
		_, err = svc.SetScores(ctx, fx.ayseUser, ref, version, items)
		return err
	}
	for code, p := range map[string]grading.Points{"MIDTERM": 7000, "HOMEWORK": 6000, "FINAL": 8000} {
		if err := write(code, p); err != nil {
			t.Fatal(err)
		}
	}
	sc, err := svc.ComponentRef(ctx, fx.components["FINAL"], fx.ayseUser)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for i := range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := write("HOMEWORK", grading.Points(5000+i*400))
			if err != nil && !errors.Is(err, grading.ErrGradesPublished) && !errors.Is(err, grading.ErrVersionMismatch) {
				errs <- err
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		if _, err := svc.PublishGrades(ctx, fx.ayseUser, sc.Section); err != nil {
			errs <- err
		}
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("beklenmeyen hata: %v", err)
	}

	var late int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM grading.assessment_scores sc
		JOIN grading.assessment_components ac ON ac.id = sc.component_id
		JOIN offering.sections s ON s.id = ac.section_id
		WHERE s.id = $1 AND sc.entered_at > s.grades_published_at`, fx.section).Scan(&late); err != nil || late != 0 {
		t.Errorf("ilandan sonra yazılmış not = %d, %v", late, err)
	}
	// İlan edilen puan, ilan anındaki bileşen notlarından hesaplananla aynı olmalı.
	rows, err := pool.Query(ctx, `
		SELECT e.weighted_score::float8,
		       sum(sc.score * ac.weight / 100)::float8
		FROM enrollment.enrollments e
		JOIN grading.assessment_scores sc ON sc.enrollment_id = e.id
		JOIN grading.assessment_components ac ON ac.id = sc.component_id AND ac.type_code <> 'MAKEUP'
		WHERE e.section_id = $1 GROUP BY e.id, e.weighted_score`, fx.section)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var published, current float64
		if err := rows.Scan(&published, &current); err != nil {
			t.Fatal(err)
		}
		if math.Abs(published-current) > 0.005 {
			t.Errorf("ilan edilen puan %.2f, bileşen notlarından %.2f", published, current)
		}
	}
}
