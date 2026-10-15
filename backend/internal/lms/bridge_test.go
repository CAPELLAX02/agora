package lms_test

import (
	"context"
	"errors"
	"testing"

	"github.com/CAPELLAX02/agora/backend/internal/grading"
	"github.com/CAPELLAX02/agora/backend/internal/lms"
)

func TestComponentScore(t *testing.T) {
	inputs := []lms.ComponentInput{
		{Weight: 100, MaxScore: 5000, Finals: map[string]grading.Points{"a": 4000, "b": 3333}},
		{Weight: 300, MaxScore: 10_000, Finals: map[string]grading.Points{"a": 6000}},
	}
	for user, want := range map[string]grading.Points{
		"a": 6500, // (80 × 1 + 60 × 3) / 4
		"b": 1667, // (66,66 × 1 + 0 × 3) / 4 = 16,665 → 16,67 (yarım yukarı)
		"c": 0,    // puanı olmayan 0
	} {
		if got := lms.ComponentScore(inputs, user); got != want {
			t.Errorf("%s: %d, beklenen %d", user, got, want)
		}
	}
	if got := lms.ComponentScore(nil, "a"); got != 0 {
		t.Errorf("ödev yok: %d", got)
	}
}

// componentScore, öğrencinin bileşendeki notunu ve kaynağını okur; not yoksa -1.
func componentScore(t *testing.T, s *server, componentID string) (grading.Points, string) {
	t.Helper()
	var (
		score  *int64
		source string
	)
	err := s.pool.QueryRow(context.Background(), `
		SELECT (sc.score * 100)::int8, sc.source FROM grading.assessment_scores sc
		JOIN enrollment.enrollments e ON e.id = sc.enrollment_id
		JOIN enrollment.student_programs sp ON sp.id = e.student_program_id
		JOIN people.students st ON st.id = sp.student_id
		JOIN iam.users u ON u.person_id = st.person_id
		WHERE sc.component_id = $1 AND u.id = $2`, componentID, s.fx.student).Scan(&score, &source)
	if err != nil || score == nil {
		return -1, ""
	}
	return grading.Points(*score), source
}

func TestAssignmentComponentBridge(t *testing.T) {
	s := newGradingServer(t)
	fx := s.fx
	ctx := context.Background()
	var lab, midterm, final string
	err := s.pool.QueryRow(ctx, `
		WITH c AS (INSERT INTO grading.assessment_components (section_id, type_code, weight, position) VALUES
			($1, 'LAB', 20, 1), ($1, 'MIDTERM', 20, 2), ($1, 'FINAL', 60, 3) RETURNING id, type_code)
		SELECT (SELECT id FROM c WHERE type_code = 'LAB'), (SELECT id FROM c WHERE type_code = 'MIDTERM'),
		       (SELECT id FROM c WHERE type_code = 'FINAL')`, fx.section).Scan(&lab, &midterm, &final)
	if err != nil {
		t.Fatal(err)
	}
	a1 := newAssignment(t, s, "Ödev 1") // en yüksek 50
	a2 := newAssignment(t, s, "Ödev 2")
	if res := s.do("PUT", "/api/v1/activities/"+a2, fx.primary, map[string]string{"If-Match": `"1"`}, map[string]any{
		"type": "ASSIGNMENT", "title": "Ödev 2", "assignment": map[string]any{"max_score": 100, "submission_types": []string{"FILE"}}}); res.status != 200 {
		t.Fatalf("ödev 2: %d %s", res.status, res.raw)
	}

	// Bağlanabilir bileşenler dönem içi olanlardır.
	res := s.do("GET", "/api/v1/activities/"+a1+"/link", fx.assistant, nil, nil)
	if res.status != 200 || len(res.body["components"].([]any)) != 2 || res.body["link"] != nil || res.body["plan_locked"] != false {
		t.Fatalf("bağlantı: %d %s", res.status, res.raw)
	}
	if res := s.do("PUT", "/api/v1/activities/"+a1+"/link", fx.primary, nil, map[string]any{"component_id": final}); res.status != 400 {
		t.Errorf("final bağlanamaz: %d", res.status)
	}
	if res := s.do("PUT", "/api/v1/activities/"+a1+"/link", fx.head, nil, map[string]any{"component_id": lab}); res.status != 403 {
		t.Errorf("izleyici bağlayamaz: %d", res.status)
	}
	// Plan kilitlenmeden notlar akmaz.
	res = s.do("PUT", "/api/v1/activities/"+a1+"/link", fx.primary, nil, map[string]any{"component_id": lab, "weight": 1})
	if res.status != 200 || res.body["sync_state"] != "PLAN_NOT_LOCKED" || res.body["link"].(map[string]any)["component_id"] != lab {
		t.Fatalf("bağla: %d %s", res.status, res.raw)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE offering.sections SET assessment_plan_locked_at = now() WHERE id = $1`, fx.section); err != nil {
		t.Fatal(err)
	}

	// Yayımlanmamış puan bileşene girmez; yayımlanınca 100'lük olarak akar.
	grade := func(activity string, score float64, version string) {
		t.Helper()
		headers := map[string]string{}
		if version != "" {
			headers["If-Match"] = version
		}
		if res := s.do("PUT", "/api/v1/activities/"+activity+"/submissions/"+fx.student+"/grade", fx.primary, headers,
			map[string]any{"score": score}); res.status != 200 {
			t.Fatalf("puan: %d %s", res.status, res.raw)
		}
	}
	grade(a1, 40, "")
	if got, _ := componentScore(t, s, lab); got != -1 {
		t.Errorf("yayımlanmamış ödev: %d", got)
	}
	s.do("POST", "/api/v1/activities/"+a1+"/grades/release", fx.primary, nil, nil)
	if got, src := componentScore(t, s, lab); got != 8000 || src != "LMS" {
		t.Errorf("yayımlanınca: %d %s", got, src)
	}

	// İkinci ödev ağırlık 3 ile; yayımlanınca ağırlıklı ortalama.
	if res := s.do("PUT", "/api/v1/activities/"+a2+"/link", fx.primary, nil, map[string]any{"component_id": lab, "weight": 3}); res.body["sync_state"] != "SYNCED" {
		t.Fatalf("ikinci bağlantı: %s", res.raw)
	}
	grade(a2, 60, "")
	s.do("POST", "/api/v1/activities/"+a2+"/grades/release", fx.primary, nil, nil)
	if got, _ := componentScore(t, s, lab); got != 6500 {
		t.Errorf("ağırlıklı ortalama: %d", got)
	}

	// Bileşene elle not girilmez; not tablosu bileşeni LMS'ten besleniyor olarak gösterir.
	ref, err := s.grading.ComponentRef(ctx, lab, fx.primary)
	if err != nil {
		t.Fatal(err)
	}
	p := grading.Points(5000)
	if _, err := s.grading.SetScores(ctx, fx.primary, ref, 1, []grading.ScoreInput{{EnrollmentID: "x", Points: &p}}); !errors.Is(err, grading.ErrFedByLMS) {
		t.Errorf("elle giriş: %v", err)
	}
	book, err := s.grading.Gradebook(ctx, fx.section, fx.primary)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range book.Components {
		if (c.ID == lab) != c.LMSFed {
			t.Errorf("%s LMS işareti: %v", c.Type.Code, c.LMSFed)
		}
	}

	// Yayımlandıktan sonra değişen puan bileşene yansır; bağlantı kalkınca bileşen yeniden hesaplanır.
	grade(a1, 50, `"1"`)
	if got, _ := componentScore(t, s, lab); got != 7000 {
		t.Errorf("puan değişince: %d", got)
	}
	if res := s.do("PUT", "/api/v1/activities/"+a2+"/link", fx.primary, nil, map[string]any{"component_id": nil}); res.status != 200 || res.body["link"] != nil {
		t.Fatalf("bağlantıyı kaldır: %s", res.raw)
	}
	if got, _ := componentScore(t, s, lab); got != 10_000 {
		t.Errorf("bağlantı kalkınca: %d", got)
	}

	// Harf notları ilan edilince notlar donar.
	if _, err := s.pool.Exec(ctx, `UPDATE offering.sections SET grades_published_at = now() WHERE id = $1`, fx.section); err != nil {
		t.Fatal(err)
	}
	grade(a1, 10, `"2"`)
	if got, _ := componentScore(t, s, lab); got != 10_000 {
		t.Errorf("ilandan sonra donar: %d", got)
	}
	if res := s.do("PUT", "/api/v1/activities/"+a1+"/link", fx.primary, nil, map[string]any{"component_id": lab, "weight": 2}); res.body["sync_state"] != "FROZEN" ||
		res.body["grades_published"] != true {
		t.Errorf("donmuş: %s", res.raw)
	}
}
