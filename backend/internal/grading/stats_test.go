package grading_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/CAPELLAX02/agora/backend/internal/grading"
	"github.com/CAPELLAX02/agora/backend/internal/grading/letter"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
)

func TestComputeStats(t *testing.T) {
	p := func(v grading.Points) *grading.Points { return &v }
	scored := func(l string, v grading.Points, passing bool) grading.Sample {
		return grading.Sample{Letter: l, Weighted: p(v), Passing: passing}
	}
	cases := []struct {
		name                string
		samples             []grading.Sample
		students, n, passed int
		mean, median, sd    *grading.Points
		distribution        map[string]int
	}{
		{name: "boş sınıf", distribution: map[string]int{}},
		{
			name:     "sadece devamsız: puan yok",
			samples:  []grading.Sample{{Letter: "F1"}, {Letter: "F1"}},
			students: 2, distribution: map[string]int{"F1": 2},
		},
		{
			// σ = √(200/3) = 8,1649… → 8,16
			name:     "tek sayıda öğrenci",
			samples:  []grading.Sample{scored("CB", 7000, true), scored("DC", 6000, true), scored("BA", 8000, true)},
			students: 3, n: 3, passed: 3, mean: p(7000), median: p(7000), sd: p(816),
			distribution: map[string]int{"BA": 1, "CB": 1, "DC": 1},
		},
		{
			// Medyan iki ortadakinin ortalaması: 70,005 → 70,01 (yarım yukarı); ortalama 270,01 / 4 = 67,5025 → 67,50.
			name: "çift sayıda öğrenci, harfi belli olmayan sayılmaz",
			samples: []grading.Sample{scored("CB", 7000, true), scored("CB", 7001, true), {Letter: "F1"},
				scored("FF", 3000, false), scored("AA", 10000, true), {Weighted: p(9000)}},
			students: 5, n: 4, passed: 3, mean: p(6750), median: p(7001), sd: p(2487),
			distribution: map[string]int{"AA": 1, "CB": 2, "F1": 1, "FF": 1},
		},
		{
			name:     "herkes aynı puanı aldı",
			samples:  []grading.Sample{scored("BB", 7550, true), scored("BB", 7550, true)},
			students: 2, n: 2, passed: 2, mean: p(7550), median: p(7550), sd: p(0),
			distribution: map[string]int{"BB": 2},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st := grading.ComputeStats(c.samples)
			if st.Students != c.students || st.Scored != c.n || st.Passed != c.passed {
				t.Errorf("sayılar = %d/%d/%d, beklenen %d/%d/%d", st.Students, st.Scored, st.Passed, c.students, c.n, c.passed)
			}
			for name, pair := range map[string][2]*grading.Points{"ortalama": {st.Mean, c.mean}, "medyan": {st.Median, c.median}, "standart sapma": {st.StdDev, c.sd}} {
				if !reflect.DeepEqual(pair[0], pair[1]) {
					t.Errorf("%s = %v, beklenen %v", name, deref(pair[0]), deref(pair[1]))
				}
			}
			if !reflect.DeepEqual(st.Distribution, c.distribution) {
				t.Errorf("dağılım = %v, beklenen %v", st.Distribution, c.distribution)
			}
		})
	}
	if r := grading.ComputeStats(nil).PassRate(); r != nil {
		t.Errorf("boş sınıfın başarı oranı = %v", *r)
	}
	if r := grading.ComputeStats(cases[3].samples).PassRate(); r == nil || *r != 0.6 {
		t.Errorf("başarı oranı = %v", r)
	}
}

func deref(p *grading.Points) any {
	if p == nil {
		return nil
	}
	return *p
}

func TestDistributionOrder(t *testing.T) {
	pts := func(v letter.Points) *letter.Points { return &v }
	scale := letter.Scale{
		{Letter: "AA", Min: pts(9000), Max: pts(10000), Passing: true, CountsInGPA: true},
		{Letter: "BA", Min: pts(8500), Max: pts(8999), Passing: true, CountsInGPA: true},
		{Letter: "FF", Min: pts(0), Max: pts(8499), CountsInGPA: true},
		{Letter: "F1", CountsInGPA: true, AttendanceFail: true},
		{Letter: "BŞR", Passing: true},
		{Letter: "BŞZ"},
		{Letter: "MUAF", Passing: true},
	}
	got := grading.Distribution(scale, false, map[string]int{"BA": 2, "MUAF": 1})
	lc := func(l string, n int) grading.LetterCount { return grading.LetterCount{Letter: l, Count: n} }
	want := []grading.LetterCount{lc("AA", 0), lc("BA", 2), lc("FF", 0), lc("F1", 0), lc("MUAF", 1)}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("harf notlu ders = %v", got)
	}
	got = grading.Distribution(scale, true, map[string]int{"BŞZ": 1})
	want = []grading.LetterCount{lc("BŞR", 0), lc("BŞZ", 1)}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("başarılı/başarısız notlu ders = %v", got)
	}
}

// TestHTTPGradeStats: sınıf istatistikleri harf notu ilanıyla oluşur, bütünleme ilanıyla
// güncellenir. Şubenin öğrencisi, öğretim elemanı ve kapsamdaki birim görür; başka öğrenci
// göremez.
func TestHTTPGradeStats(t *testing.T) {
	s := newGradebookServer(t)
	fx := s.fx
	ctx := context.Background()
	s.addGrant(fx.ayseUser, authz.Grant{Permission: "grade:finalize", ScopeType: authz.ScopeDepartment, ScopeID: fx.bil, RequiresMFA: true})
	var aliUser string
	if err := s.pool.QueryRow(ctx, `
		INSERT INTO iam.users (person_id, username, email, password_hash)
		SELECT person_id, student_no, student_no || '@ogr.test', 'x' FROM people.students WHERE student_no = '22290101'
		RETURNING id`).Scan(&aliUser); err != nil {
		t.Fatal(err)
	}
	s.addGrant(aliUser, authz.Grant{Permission: "course:read", ScopeType: authz.ScopeNone})

	section := "/api/v1/sections/" + fx.section
	stats := section + "/grade-stats"
	item := func(i int, score any) map[string]any {
		return map[string]any{"enrollment_id": fx.enrollments[i], "score": score, "absent": false}
	}
	put := func(component, version string, items ...map[string]any) {
		t.Helper()
		res := s.do("PUT", "/api/v1/assessment-components/"+component+"/scores", fx.ayseUser, with(mfa, "If-Match", version), map[string]any{"items": items})
		if res.status != 200 {
			t.Fatalf("notlar: %d %s", res.status, res.raw)
		}
	}
	if res := s.do("POST", section+"/assessment-plan/lock", fx.ayseUser, nil, nil); res.status != 200 {
		t.Fatalf("kilit: %d %s", res.status, res.raw)
	}
	s.calendar.set(grading.WindowFinalEntry, true)
	// Ali 88 (B1), Ece 50 (final barajı altı → F2), Can devamsız (F4, puanı yok).
	put(fx.components["MIDTERM"], `"1"`, item(0, 80), item(1, 60), item(2, 90))
	put(fx.components["HOMEWORK"], `"1"`, item(0, 100), item(1, 50), item(2, 90))
	put(fx.components["FINAL"], `"1"`, item(0, 90), item(1, 45), item(2, 90))
	if res := s.do("PUT", section+"/attendance", fx.ayseUser, mfa, map[string]any{"items": []any{
		map[string]any{"enrollment_id": fx.enrollments[2], "attendance_status": "ATTENDANCE_FAIL"}}}); res.status != 200 {
		t.Fatalf("devam: %d %s", res.status, res.raw)
	}

	if res := s.do("GET", stats, aliUser, nil, nil); res.status != 404 || res.code() != "GRADES_NOT_PUBLISHED" {
		t.Errorf("ilandan önce: %d %s", res.status, res.raw)
	}
	letters := s.do("POST", section+"/letter-grades/publish", fx.ayseUser, mfa, nil)
	if letters.status != 200 {
		t.Fatalf("ilan: %d %s", letters.status, letters.raw)
	}

	counts := func(res result) map[string]float64 {
		out := map[string]float64{}
		for _, d := range res.body["distribution"].([]any) {
			m := d.(map[string]any)
			out[m["letter"].(string)] = m["count"].(float64)
		}
		return out
	}
	check := func(who string, res result, mean, median, sd float64, passed float64, dist map[string]float64) {
		t.Helper()
		b := res.body
		if res.status != 200 || b["student_count"] != 3.0 || b["scored_count"] != 2.0 || b["passed_count"] != passed ||
			b["mean"] != mean || b["median"] != median || b["stddev"] != sd || b["computed_at"] == nil {
			t.Errorf("%s: %d %s", who, res.status, res.raw)
			return
		}
		got := counts(res)
		for l, n := range dist {
			if got[l] != n {
				t.Errorf("%s: %s = %v, beklenen %v (%v)", who, l, got[l], n, got)
			}
		}
		if _, ok := got["A"]; !ok {
			t.Errorf("%s: dağılım ölçeğin bütün harflerini göstermeli: %v", who, got)
		}
	}
	// (88 + 50) / 2 = 69; σ = 19. Önizlemedeki özet aynı hesaptan gelir.
	check("öğrenci", s.do("GET", stats, aliUser, nil, nil), 69, 69, 19, 1, map[string]float64{"B1": 1, "F2": 1, "F4": 1, "A": 0})
	check("bölüm", s.do("GET", stats, head, nil, nil), 69, 69, 19, 1, nil)
	if sum := letters.body["summary"].(map[string]any); sum["stddev"] != 19.0 || sum["mean"] != 69.0 {
		t.Errorf("önizleme özeti: %v", sum)
	}
	if res := s.do("GET", stats, student, nil, nil); res.status != 403 {
		t.Errorf("şubede olmayan öğrenci: %d %s", res.status, res.raw)
	}
	if res := s.do("GET", "/api/v1/sections/01a11b7f-0000-7000-8000-0000000000ff/grade-stats", aliUser, nil, nil); res.status != 404 {
		t.Errorf("olmayan şube: %d", res.status)
	}

	// Bütünleme: Ece 70 → 18 + 5 + 42 = 65 (C2). (88 + 65) / 2 = 76,5; σ = 11,5.
	s.calendar.set(grading.WindowMakeupEntry, true)
	makeup := ""
	for _, c := range s.do("GET", section+"/gradebook", fx.ayseUser, mfa, nil).body["components"].([]any) {
		if m := c.(map[string]any); m["type"].(map[string]any)["code"] == "MAKEUP" {
			makeup = m["id"].(string)
		}
	}
	put(makeup, `"1"`, item(1, 70))
	if res := s.do("POST", section+"/makeup/publish", fx.ayseUser, mfa, nil); res.status != 200 {
		t.Fatalf("bütünleme ilanı: %d %s", res.status, res.raw)
	}
	check("bütünlemeden sonra", s.do("GET", stats, aliUser, nil, nil), 76.5, 76.5, 11.5, 2, map[string]float64{"B1": 1, "C2": 1, "F2": 0, "F4": 1})

	var stored string
	if err := s.pool.QueryRow(ctx, `
		SELECT student_count || '/' || scored_count || '/' || passed_count || ' ' || mean || ' ' || median || ' ' || stddev
		FROM grading.section_grade_stats WHERE section_id = $1`, fx.section).Scan(&stored); err != nil || stored != "3/2/2 76.50 76.50 11.50" {
		t.Errorf("kayıtlı istatistik = %q, %v", stored, err)
	}
	// Yeniden hesap idempotenttir; ilan edilmiş notu kalmayan şubenin istatistiği silinir.
	if err := grading.RefreshStats(ctx, s.pool, []string{fx.section}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `
		UPDATE enrollment.enrollments SET status = 'DROPPED', letter_grade = NULL, weighted_score = NULL, letter_reason = NULL
		WHERE section_id = $1`, fx.section); err != nil {
		t.Fatal(err)
	}
	if err := grading.RefreshStats(ctx, s.pool, []string{fx.section}); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM grading.section_grade_stats`).Scan(&n); err != nil || n != 0 {
		t.Errorf("istatistik silinmeli: %d %v", n, err)
	}
}
