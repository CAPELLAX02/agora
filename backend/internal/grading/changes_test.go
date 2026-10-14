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

// TestHTTPGradeChanges: ilandan sonra not sadece düzeltme talebiyle değişir. Şubenin öğretim
// elemanı talep eder (bileşen notu ya da harf), bölüm başkanı MFA'lı oturumda onaylar ya da
// gerekçeyle reddeder; talep eden onaylayamaz. Onay notu uygular: bileşen notu, harf, not
// geçmişi, sınıf istatistiği ve (olayla) dönem sonuçları. Talepten sonra değişen not uygulanmaz.
func TestHTTPGradeChanges(t *testing.T) {
	s := newGradebookServer(t)
	fx := s.fx
	ctx := context.Background()
	request := authz.Grant{Permission: "grade_change:request", ScopeType: authz.ScopeDepartment, ScopeID: fx.bil}
	approve := authz.Grant{Permission: "grade_change:approve", ScopeType: authz.ScopeDepartment, ScopeID: fx.bil, RequiresMFA: true}
	s.addGrant(fx.ayseUser, authz.Grant{Permission: "grade:finalize", ScopeType: authz.ScopeDepartment, ScopeID: fx.bil, RequiresMFA: true})
	s.addGrant(fx.ayseUser, request)
	s.addGrant(fx.mehmetUser, request)
	// Onaylayan gerçek bir kullanıcı olmalı (not geçmişi ve talep onaylayanı kaydeder).
	var headUser string
	if err := s.pool.QueryRow(ctx, `
		WITH p AS (INSERT INTO people.persons (first_name, last_name) VALUES ('Selin', 'Koç') RETURNING id)
		INSERT INTO iam.users (person_id, username, email, password_hash) SELECT id, 'P1003', 'p1003@personel.test', 'x' FROM p
		RETURNING id`).Scan(&headUser); err != nil {
		t.Fatal(err)
	}
	s.addGrant(headUser, approve)

	section := "/api/v1/sections/" + fx.section
	scores := func(code string) string { return "/api/v1/assessment-components/" + fx.components[code] + "/scores" }
	item := func(i int, score any) map[string]any {
		return map[string]any{"enrollment_id": fx.enrollments[i], "score": score, "absent": false}
	}
	if res := s.do("POST", section+"/assessment-plan/lock", fx.ayseUser, nil, nil); res.status != 200 {
		t.Fatalf("kilit: %d %s", res.status, res.raw)
	}
	s.calendar.set(grading.WindowFinalEntry, true)
	// Ali 88 (B1), Ece 50 (final 45: baraj altı → F2), Can 90 (A).
	for code, items := range map[string][]map[string]any{
		"MIDTERM": {item(0, 80), item(1, 60), item(2, 90)}, "HOMEWORK": {item(0, 100), item(1, 50), item(2, 90)},
		"FINAL": {item(0, 90), item(1, 45), item(2, 90)},
	} {
		if res := s.do("PUT", scores(code), fx.ayseUser, with(mfa, "If-Match", `"1"`), map[string]any{"items": items}); res.status != 200 {
			t.Fatalf("%s: %d %s", code, res.status, res.raw)
		}
	}
	scoreChange := func(i int, code string, score any, reason string) map[string]any {
		return map[string]any{"enrollment_id": fx.enrollments[i], "component_id": fx.components[code], "score": score, "reason": reason}
	}
	letterChange := func(i int, letter, reason string) map[string]any {
		return map[string]any{"enrollment_id": fx.enrollments[i], "letter": letter, "reason": reason}
	}
	const why = "Final kâğıdında toplama hatası"
	if res := s.do("POST", "/api/v1/grade-change-requests/preview", fx.ayseUser, nil, scoreChange(1, "FINAL", 55, why)); res.status != 409 || res.code() != "GRADES_NOT_PUBLISHED" {
		t.Errorf("ilandan önce: %d %s", res.status, res.raw)
	}
	if res := s.do("POST", section+"/letter-grades/publish", fx.ayseUser, mfa, nil); res.status != 200 {
		t.Fatalf("ilan: %d %s", res.status, res.raw)
	}

	// Önizleme: Ece'nin finali 55 → 18 + 5 + 33 = 56, baraj aşıldı: F1 (50-59). Ali B1 → A elle.
	res := s.do("POST", "/api/v1/grade-change-requests/preview", fx.ayseUser, nil, scoreChange(1, "FINAL", 55, why))
	old, next := func() (map[string]any, map[string]any) {
		o, _ := res.body["old"].(map[string]any)
		n, _ := res.body["new"].(map[string]any)
		return o, n
	}()
	if res.status != 200 || res.body["kind"] != "SCORE" || old["letter"] != "F2" || old["score"] != 45.0 ||
		next["letter"] != "F1" || next["weighted_score"] != 56.0 || next["reason"] != "RANGE" || next["score"] != 55.0 {
		t.Fatalf("önizleme: %d %s", res.status, res.raw)
	}
	res = s.do("POST", "/api/v1/grade-change-requests/preview", fx.ayseUser, nil, letterChange(0, "A", why))
	if n := res.body["new"].(map[string]any); res.status != 200 || res.body["kind"] != "LETTER" || n["letter"] != "A" || n["reason"] != "MANUAL" || n["weighted_score"] != 88.0 {
		t.Errorf("harf önizlemesi: %d %s", res.status, res.raw)
	}
	for name, body := range map[string]map[string]any{
		"not değişmiyor":   scoreChange(1, "FINAL", 45, why),
		"harf değişmiyor":  letterChange(0, "B1", why),
		"verilemeyen harf": letterChange(0, "BŞR", why),
		"puan aralık dışı": scoreChange(1, "FINAL", 101, why),
		"puan ve harf":     {"enrollment_id": fx.enrollments[1], "component_id": fx.components["FINAL"], "score": 55, "letter": "A", "reason": why},
	} {
		if res := s.do("POST", "/api/v1/grade-change-requests/preview", fx.ayseUser, nil, body); res.status != 400 {
			t.Errorf("%s: %d %s", name, res.status, res.raw)
		}
	}
	if res := s.do("POST", "/api/v1/grade-change-requests/preview", fx.mehmetUser, nil, scoreChange(1, "FINAL", 55, why)); res.status != 403 {
		t.Errorf("şubede ders vermeyen öğretim elemanı: %d", res.status)
	}

	// Talep: finali 72 → 18 + 5 + 43,2 = 66,2 → C2.
	res = s.do("POST", "/api/v1/grade-change-requests", fx.ayseUser, nil, scoreChange(1, "FINAL", 72, why))
	if res.status != 201 || res.body["status"] != "PENDING" || res.body["can_withdraw"] != true || res.body["can_approve"] != false ||
		res.body["new"].(map[string]any)["letter"] != "C2" || res.body["requested_by"] != "Ayşe Yılmaz" ||
		res.body["component"].(map[string]any)["label_tr"] != "Final sınavı" {
		t.Fatalf("talep: %d %s", res.status, res.raw)
	}
	eceChange := res.body["id"].(string)
	if res := s.do("POST", "/api/v1/grade-change-requests", fx.ayseUser, nil, scoreChange(1, "MIDTERM", 70, "hata")); res.status != 400 {
		t.Errorf("kısa gerekçeyle talep: %d %s", res.status, res.raw)
	}
	if res := s.do("POST", "/api/v1/grade-change-requests/preview", fx.ayseUser, nil, scoreChange(1, "MIDTERM", 70, "")); res.status != 200 {
		t.Errorf("önizleme gerekçe istemez: %d %s", res.status, res.raw)
	}
	if res := s.do("POST", "/api/v1/grade-change-requests", fx.ayseUser, nil, scoreChange(1, "MIDTERM", 70, why)); res.status != 409 || res.code() != "GRADE_CHANGE_PENDING" {
		t.Errorf("bekleyen talep varken ikincisi: %d %s", res.status, res.raw)
	}
	// Onay kuyruğu MFA'lı oturumda görünür: MFA'sız oturumda MFA isteyen yetki hiç yoktur.
	count := func(user, view string) int {
		t.Helper()
		return len(s.do("GET", "/api/v1/grade-change-requests?view="+view+"&status=PENDING", user, mfa, nil).body["items"].([]any))
	}
	if count(fx.ayseUser, "mine") != 1 || count(headUser, "review") != 1 || count(fx.mehmetUser, "review") != 0 {
		t.Errorf("listeler: %d %d %d", count(fx.ayseUser, "mine"), count(headUser, "review"), count(fx.mehmetUser, "review"))
	}
	if res := s.do("GET", "/api/v1/grade-change-requests/"+eceChange, fx.mehmetUser, nil, nil); res.status != 404 {
		t.Errorf("ilgisiz kullanıcı talebi görmemeli: %d", res.status)
	}

	approvePath := "/api/v1/grade-change-requests/" + eceChange + "/approve"
	if res := s.do("POST", approvePath, headUser, nil, map[string]any{}); res.status != 403 || res.code() != "MFA_REQUIRED" {
		t.Errorf("MFA'sız onay: %d %s", res.status, res.raw)
	}
	s.addGrant(fx.ayseUser, approve)
	if res := s.do("POST", approvePath, fx.ayseUser, mfa, map[string]any{}); res.status != 403 || res.code() != "SELF_APPROVAL" {
		t.Errorf("kendi talebini onaylama: %d %s", res.status, res.raw)
	}
	res = s.do("POST", approvePath, headUser, mfa, map[string]any{"note": "Kâğıt incelendi"})
	if res.status != 200 || res.body["status"] != "APPLIED" || res.body["review_note"] != "Kâğıt incelendi" || res.body["decided_at"] == nil {
		t.Fatalf("onay: %d %s", res.status, res.raw)
	}
	var letter, reason, weighted string
	if err := s.pool.QueryRow(ctx, `SELECT letter_grade, letter_reason, weighted_score::text FROM enrollment.enrollments WHERE id = $1`,
		fx.enrollments[1]).Scan(&letter, &reason, &weighted); err != nil || letter != "C2" || reason != "RANGE" || weighted != "66.20" {
		t.Errorf("kesin kayıt = %s %s %s, %v", letter, reason, weighted, err)
	}
	var final string
	_ = s.pool.QueryRow(ctx, `SELECT score::text FROM grading.assessment_scores WHERE component_id = $1 AND enrollment_id = $2`,
		fx.components["FINAL"], fx.enrollments[1]).Scan(&final)
	var history string
	_ = s.pool.QueryRow(ctx, `SELECT string_agg(kind || ':' || letter, ' ' ORDER BY id) FROM grading.grade_history WHERE enrollment_id = $1`,
		fx.enrollments[1]).Scan(&history)
	var mean string
	_ = s.pool.QueryRow(ctx, `SELECT mean::text FROM grading.section_grade_stats WHERE section_id = $1`, fx.section).Scan(&mean)
	// Ortalama (88 + 66,2 + 90) / 3 = 81,40.
	if final != "72.00" || history != "PUBLISHED:F2 CORRECTION:C2" || mean != "81.40" {
		t.Errorf("bileşen notu %s, not geçmişi %q, sınıf ortalaması %s", final, history, mean)
	}

	// Dönem sonuçları olayla yeniden hesaplanır: Ece'nin GANO'su C2 (2,50).
	dispatcher := outbox.NewDispatcher(s.pool, []outbox.Consumer{grading.TermResultsUpdater{}}, outbox.DefaultConfig,
		slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	if _, err := dispatcher.ProcessBatch(ctx); err != nil {
		t.Fatal(err)
	}
	var gpa string
	if err := s.pool.QueryRow(ctx, `
		SELECT sp.gpa_cache::text FROM enrollment.student_programs sp JOIN enrollment.enrollments e ON e.student_program_id = sp.id
		WHERE e.id = $1`, fx.enrollments[1]).Scan(&gpa); err != nil || gpa != "2.50" {
		t.Errorf("GANO = %s, %v", gpa, err)
	}
	var phases string
	_ = s.pool.QueryRow(ctx, `SELECT string_agg(payload->>'phase', ' ' ORDER BY id) FROM platform.outbox_events WHERE event_type = 'grades.published'`).Scan(&phases)
	if phases != "FINAL CORRECTION" {
		t.Errorf("olaylar = %q", phases)
	}

	// Ret: gerekçe zorunlu.
	res = s.do("POST", "/api/v1/grade-change-requests", fx.ayseUser, nil, letterChange(0, "A", why))
	aliChange := res.body["id"].(string)
	if res := s.do("POST", "/api/v1/grade-change-requests/"+aliChange+"/reject", headUser, mfa, map[string]any{}); res.status != 400 {
		t.Errorf("gerekçesiz ret: %d %s", res.status, res.raw)
	}
	if res := s.do("POST", "/api/v1/grade-change-requests/"+aliChange+"/reject", headUser, mfa, map[string]any{"note": "Kanıt yok"}); res.status != 200 || res.body["status"] != "REJECTED" {
		t.Errorf("ret: %d %s", res.status, res.raw)
	}

	// Geri çekme: sadece talep eden; geri çekilen talep onaylanamaz.
	res = s.do("POST", "/api/v1/grade-change-requests", fx.ayseUser, nil, letterChange(0, "A", why))
	withdrawn := res.body["id"].(string)
	if res := s.do("POST", "/api/v1/grade-change-requests/"+withdrawn+"/withdraw", fx.mehmetUser, nil, nil); res.status != 404 {
		t.Errorf("başkası geri çekemez: %d", res.status)
	}
	if res := s.do("POST", "/api/v1/grade-change-requests/"+withdrawn+"/withdraw", fx.ayseUser, nil, nil); res.status != 200 || res.body["status"] != "WITHDRAWN" {
		t.Errorf("geri çekme: %d %s", res.status, res.raw)
	}
	if res := s.do("POST", "/api/v1/grade-change-requests/"+withdrawn+"/approve", headUser, mfa, map[string]any{}); res.status != 409 || res.code() != "GRADE_CHANGE_DECIDED" {
		t.Errorf("geri çekilmiş talep: %d %s", res.status, res.raw)
	}

	// Bayat talep: talepten sonra not başka yoldan değiştiyse uygulanmaz.
	res = s.do("POST", "/api/v1/grade-change-requests", fx.ayseUser, nil, letterChange(2, "B1", why))
	stale := res.body["id"].(string)
	if _, err := s.pool.Exec(ctx, `UPDATE enrollment.enrollments SET letter_grade = 'B2' WHERE id = $1`, fx.enrollments[2]); err != nil {
		t.Fatal(err)
	}
	if res := s.do("POST", "/api/v1/grade-change-requests/"+stale+"/approve", headUser, mfa, map[string]any{}); res.status != 409 || res.code() != "GRADE_CHANGED" {
		t.Errorf("bayat talep: %d %s", res.status, res.raw)
	}

	var audits string
	_ = s.pool.QueryRow(ctx, `
		SELECT string_agg(DISTINCT action, ' ') FROM audit.audit_log WHERE action LIKE 'grade_change.%'`).Scan(&audits)
	for _, a := range []string{"grade_change.request", "grade_change.apply", "grade_change.reject", "grade_change.withdraw"} {
		if !strings.Contains(audits, a) {
			t.Errorf("denetim izinde %s yok: %s", a, audits)
		}
	}
	// Sayaçlar sadece başarılı işlemleri sayar: reddedilen onay ve bayat talep sayılmaz.
	var requested int
	_ = s.pool.QueryRow(ctx, `SELECT count(*) FROM grading.grade_change_requests`).Scan(&requested)
	for key, want := range map[string]int{"changes/requested": requested, "changes/applied": 1, "changes/rejected": 1, "changes/withdrawn": 1} {
		if got := s.metrics.get(key); got != want {
			t.Errorf("%s = %d, want %d", key, got, want)
		}
	}
}
