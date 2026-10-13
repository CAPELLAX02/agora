package grading_test

import (
	"context"
	"testing"

	"github.com/CAPELLAX02/agora/backend/internal/grading"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
)

// TestStudentGrades: öğrenci bileşen notunu ve sınıf ortalamasını bileşen yayımlanınca görür;
// harf notunu, ilan tarihini ve sınıf istatistiğini ilandan sonra. Bütünleme bileşeni sadece
// bütünleme notu olana listelenir.
func TestStudentGrades(t *testing.T) {
	s := newGradebookServer(t)
	fx := s.fx
	ctx := context.Background()
	s.addGrant(fx.ayseUser, authz.Grant{Permission: "grade:finalize", ScopeType: authz.ScopeDepartment, ScopeID: fx.bil, RequiresMFA: true})
	section := "/api/v1/sections/" + fx.section
	scores := func(code string) string { return "/api/v1/assessment-components/" + fx.components[code] }
	item := func(i int, score any) map[string]any {
		return map[string]any{"enrollment_id": fx.enrollments[i], "score": score, "absent": false}
	}
	put := func(path, version string, items ...map[string]any) {
		t.Helper()
		if res := s.do("PUT", path+"/scores", fx.ayseUser, with(mfa, "If-Match", version), map[string]any{"items": items}); res.status != 200 {
			t.Fatalf("notlar: %d %s", res.status, res.raw)
		}
	}
	publish := func(path string) {
		t.Helper()
		if res := s.do("POST", path+"/publish", fx.ayseUser, mfa, nil); res.status != 200 {
			t.Fatalf("yayımlama: %d %s", res.status, res.raw)
		}
	}
	var aliSP, eceSP string
	for i, sp := range []*string{&aliSP, &eceSP} {
		if err := s.pool.QueryRow(ctx, `SELECT student_program_id FROM enrollment.enrollments WHERE id = $1`, fx.enrollments[i]).Scan(sp); err != nil {
			t.Fatal(err)
		}
	}
	grades := func(sp string) grading.StudentCourse {
		t.Helper()
		g, err := grading.StudentGradesFor(ctx, s.pool, sp, "")
		if err != nil || len(g.Courses) != 1 || len(g.Terms) != 1 || g.Term.Code != "2026-FALL" || !g.Term.Current {
			t.Fatalf("notlar = %+v, %v", g, err)
		}
		return g.Courses[0]
	}
	component := func(c grading.StudentCourse, code string) *grading.StudentComponent {
		for i := range c.Components {
			if c.Components[i].Type.Code == code {
				return &c.Components[i]
			}
		}
		return nil
	}

	if res := s.do("POST", section+"/assessment-plan/lock", fx.ayseUser, nil, nil); res.status != 200 {
		t.Fatalf("kilit: %d %s", res.status, res.raw)
	}
	put(scores("MIDTERM"), `"1"`, item(0, 80), item(1, 60), item(2, 90))
	put(scores("HOMEWORK"), `"1"`, item(0, 100), item(1, 50), item(2, 90))

	// Yayımlanmadan önce bileşenler ağırlıklarıyla görünür, notlar görünmez.
	ali := grades(aliSP)
	if mid := component(ali, "MIDTERM"); mid == nil || mid.Score != nil || mid.Average != nil || mid.Weight != 30 {
		t.Errorf("yayımlanmamış ara sınav = %+v", mid)
	}
	if len(ali.Components) != 3 || component(ali, "MAKEUP") != nil || ali.Status != "ENROLLED" || ali.Letter != "" {
		t.Errorf("ders = %+v", ali)
	}

	// Ara sınav yayımlandı: Ali 80, sınıf ortalaması (80 + 60 + 90) / 3 = 76,67.
	publish(scores("MIDTERM"))
	ali = grades(aliSP)
	if mid := component(ali, "MIDTERM"); mid.Score == nil || *mid.Score != 8000 || mid.Average == nil || *mid.Average != 7667 || mid.PublishedAt == nil {
		t.Errorf("yayımlanan ara sınav = %+v", mid)
	}
	if hw := component(ali, "HOMEWORK"); hw.Score != nil || hw.Average != nil {
		t.Errorf("ödev hâlâ yayımlanmadı: %+v", hw)
	}

	// Final ve harf notu ilanı: Ali B1 (88), Ece F2; sınıf istatistiği ilanla gelir.
	s.calendar.set(grading.WindowFinalEntry, true)
	put(scores("FINAL"), `"1"`, item(0, 90), item(1, 45), item(2, 90))
	if res := s.do("POST", section+"/letter-grades/publish", fx.ayseUser, mfa, nil); res.status != 200 {
		t.Fatalf("ilan: %d %s", res.status, res.raw)
	}
	ali = grades(aliSP)
	if ali.Letter != "B1" || ali.Weighted == nil || *ali.Weighted != 8800 || ali.PublishedAt == nil || ali.Passing == nil || !*ali.Passing ||
		ali.Status != "COMPLETED" || ali.ClassStats == nil || ali.ClassStats.Students != 3 || ali.ClassStats.Mean == nil {
		t.Errorf("ilan sonrası = %+v", ali)
	}

	// Bütünleme: Ece'nin bütünleme notu girilince bileşen ona listelenir, Ali'ye listelenmez.
	s.calendar.set(grading.WindowMakeupEntry, true)
	makeup := ""
	for _, c := range s.do("GET", section+"/gradebook", fx.ayseUser, mfa, nil).body["components"].([]any) {
		if m := c.(map[string]any); m["type"].(map[string]any)["code"] == "MAKEUP" {
			makeup = "/api/v1/assessment-components/" + m["id"].(string)
		}
	}
	put(makeup, `"1"`, item(1, 70))
	if c := component(grades(eceSP), "MAKEUP"); c == nil || c.Score != nil {
		t.Errorf("Ece'nin bütünlemesi (yayımlanmadı) = %+v", c)
	}
	if component(grades(aliSP), "MAKEUP") != nil {
		t.Error("bütünleme notu olmayana bütünleme listelenmemeli")
	}

	if _, err := grading.StudentGradesFor(ctx, s.pool, aliSP, "01a11b7f-0000-7000-8000-0000000000ff"); err != grading.ErrNoTerm {
		t.Errorf("ders alınmayan dönem: %v", err)
	}
}
