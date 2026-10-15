package lms_test

import (
	"testing"
	"time"

	"github.com/CAPELLAX02/agora/backend/internal/grading"
	"github.com/CAPELLAX02/agora/backend/internal/lms"
)

func TestLatePenalty(t *testing.T) {
	due := time.Date(2026, 11, 2, 23, 59, 0, 0, time.UTC)
	st := lms.AssignmentSettings{DueAt: &due, LatePolicy: lms.LatePenalty, PenaltyPerDay: 1000}
	at := func(d time.Duration) *time.Time { v := due.Add(d); return &v }
	for _, c := range []struct {
		name string
		at   *time.Time
		want grading.Points
	}{
		{"zamanında", at(-time.Minute), 0},
		{"tam son anda", at(0), 0},
		{"bir dakika geç: başlamış gün", at(time.Minute), 1000},
		{"tam bir gün", at(24 * time.Hour), 1000},
		{"bir gün bir saniye", at(24*time.Hour + time.Second), 2000},
		{"en çok yüzde yüz", at(30 * 24 * time.Hour), 10_000},
		{"teslim yok", nil, 0},
	} {
		if got := st.Penalty(c.at); got != c.want {
			t.Errorf("%s: %d, beklenen %d", c.name, got, c.want)
		}
	}
	if got := lms.ApplyPenalty(4550, 1000); got != 4095 {
		t.Errorf("45,50 - %%10 = %d", got)
	}
	if got := lms.ApplyPenalty(3333, 1500); got != 2833 { // 28,3305 → 28,33
		t.Errorf("yuvarlama: %d", got)
	}
	allow := lms.AssignmentSettings{DueAt: &due, LatePolicy: lms.LateAllow, PenaltyPerDay: 1000}
	if got := allow.Penalty(at(48 * time.Hour)); got != 0 {
		t.Errorf("ALLOW politikasında kesinti olmaz: %d", got)
	}
}

func TestSubmissionWindow(t *testing.T) {
	opens := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	due := opens.Add(7 * 24 * time.Hour)
	cutoff := due.Add(48 * time.Hour)
	allow := lms.AssignmentSettings{OpensAt: &opens, DueAt: &due, CutoffAt: &cutoff, LatePolicy: lms.LateAllow}
	deny := lms.AssignmentSettings{OpensAt: &opens, DueAt: &due, LatePolicy: lms.LateDeny}
	for _, c := range []struct {
		st   lms.AssignmentSettings
		at   time.Time
		want lms.SubmissionState
	}{
		{allow, opens.Add(-time.Second), lms.SubmissionNotOpen},
		{allow, opens, lms.SubmissionOpen},
		{allow, due.Add(time.Hour), lms.SubmissionOpen}, // geç ama kabul
		{allow, cutoff, lms.SubmissionOpen},
		{allow, cutoff.Add(time.Second), lms.SubmissionClosed},
		{deny, due, lms.SubmissionOpen},
		{deny, due.Add(time.Second), lms.SubmissionClosed},
	} {
		if got := c.st.StateAt(c.at); got != c.want {
			t.Errorf("%s %s: %s, beklenen %s", c.st.LatePolicy, c.at, got, c.want)
		}
	}
	if allow.LateAt(due) || !allow.LateAt(due.Add(time.Second)) {
		t.Error("son tarihten sonrası geçtir")
	}
}

func submissions(res result) []map[string]any {
	var out []map[string]any
	for _, a := range res.body["attempts"].([]any) {
		out = append(out, a.(map[string]any))
	}
	return out
}

func TestAssignmentLifecycle(t *testing.T) {
	s := newServer(t)
	fx := s.fx
	space := "/api/v1/spaces/" + fx.section
	res := s.do("POST", space+"/topics", fx.primary, nil, map[string]any{"title": "Hafta 1"})
	topicID := topics(res)[1]["id"].(string)
	acts := "/api/v1/topics/" + topicID + "/activities"
	now := *s.now
	due := now.Add(48 * time.Hour).UTC()
	cutoff := now.Add(96 * time.Hour).UTC()
	settings := func(over map[string]any) map[string]any {
		a := map[string]any{
			"due_at": due, "cutoff_at": cutoff, "max_attempts": 2, "submission_types": []string{"FILE", "ONLINE_TEXT"},
			"max_files": 2, "allowed_extensions": []string{"PDF", ".docx"}, "max_score": 50, "late_policy": "PENALTY",
			"late_penalty_pct_per_day": 10,
		}
		for k, v := range over {
			a[k] = v
		}
		return map[string]any{"type": "ASSIGNMENT", "title": "Ödev 1", "description_html": "<p>Döngüler</p>", "assignment": a}
	}

	// Ayarlar doğrulanır.
	for name, over := range map[string]map[string]any{
		"geç teslim kapalıyken kesin kapanış": {"late_policy": "DENY", "late_penalty_pct_per_day": 0},
		"kesin kapanış son tarihten önce":     {"cutoff_at": now.Add(time.Hour).UTC()},
		"izin verilmeyen tür":                 {"allowed_extensions": []string{".exe"}},
		"üç ondalıklı puan":                   {"max_score": 12.345},
		"tanınmayan teslim türü":              {"submission_types": []string{"VIDEO"}},
		"deneme hakkı":                        {"max_attempts": 0},
		"platform sınırından büyük dosya":     {"max_file_size_bytes": 2 << 20},
	} {
		if res := s.do("POST", acts, fx.primary, nil, settings(over)); res.status != 400 {
			t.Errorf("%s: %d %s", name, res.status, res.raw)
		}
	}
	res = s.do("POST", acts, fx.primary, nil, settings(nil))
	if res.status != 201 {
		t.Fatalf("ödev: %d %s", res.status, res.raw)
	}
	asg := res.body["assignment"].(map[string]any)
	if asg["max_score"] != 50.0 || asg["submission_state"] != "OPEN" || asg["max_file_size_bytes"] != float64(1<<20) ||
		len(asg["allowed_extensions"].([]any)) != 2 || asg["allowed_extensions"].([]any)[0] != ".docx" {
		t.Errorf("ödev ayarları: %s", res.raw)
	}
	id := res.body["id"].(string)
	base := "/api/v1/activities/" + id
	my := base + "/my-submission"

	// Ödev öğrencinin ders alanında son tarihiyle görünür.
	res = s.do("GET", space, fx.student, nil, nil)
	if a := activities(topics(res)[1])[0]; a["assignment"].(map[string]any)["due_at"] == nil {
		t.Errorf("öğrenci ödevin son tarihini görür: %s", res.raw)
	}

	// Teslim öğrenciye özeldir.
	if res := s.do("GET", my, fx.primary, nil, nil); res.status != 403 {
		t.Errorf("öğretim elemanının teslimi olmaz: %d", res.status)
	}
	if res := s.do("GET", my, fx.dropped, nil, nil); res.status != 404 {
		t.Errorf("bırakan öğrenci: %d", res.status)
	}
	res = s.do("GET", my, fx.student, nil, nil)
	if res.status != 200 || len(submissions(res)) != 0 || res.body["can_edit"] != true || res.body["can_submit"] != false {
		t.Fatalf("boş teslim: %d %s", res.status, res.raw)
	}

	// Taslak: metin ve dosya; tür, sayı ve sürüm denetlenir.
	pdf := s.upload(fx.student, "odev.pdf", "%PDF-1.4 cevaplar")
	txt := s.upload(fx.student, "notlar.txt", "taslak")
	if res := s.do("PUT", my, fx.student, nil, map[string]any{"file_ids": []string{txt}}); res.status != 400 {
		t.Errorf("izin verilmeyen tür: %d %s", res.status, res.raw)
	}
	others := s.upload(fx.primary, "baska.pdf", "%PDF-1.4 x")
	if res := s.do("PUT", my, fx.student, nil, map[string]any{"file_ids": []string{others}}); res.status != 400 {
		t.Errorf("başkasının dosyası: %d", res.status)
	}
	res = s.do("PUT", my, fx.student, nil, map[string]any{"online_text_html": "<p>Cevap <script>x</script></p>", "file_ids": []string{pdf}})
	if res.status != 200 {
		t.Fatalf("taslak: %d %s", res.status, res.raw)
	}
	draft := submissions(res)[0]
	if draft["status"] != "DRAFT" || draft["version"] != 1.0 || len(draft["files"].([]any)) != 1 ||
		draft["online_text_html"] != "<p>Cevap </p>" || res.body["can_submit"] != true {
		t.Errorf("taslak: %s", res.raw)
	}
	if res := s.do("PUT", my, fx.student, nil, map[string]any{"online_text_html": "<p>x</p>"}); res.status != 428 {
		t.Errorf("var olan taslak sürümsüz yazılamaz: %d", res.status)
	}
	if res := s.do("PUT", my, fx.student, map[string]string{"If-Match": `"9"`}, map[string]any{"online_text_html": "<p>x</p>"}); res.status != 412 {
		t.Errorf("eski sürüm: %d", res.status)
	}
	extra := []string{pdf, s.upload(fx.student, "ek1.pdf", "%PDF-1.4 a"), s.upload(fx.student, "ek2.pdf", "%PDF-1.4 b")}
	if res := s.do("PUT", my, fx.student, map[string]string{"If-Match": `"1"`}, map[string]any{"file_ids": extra}); res.status != 400 {
		t.Errorf("en çok iki dosya: %d %s", res.status, res.raw)
	}

	// Gönderme: deneme kilitlenir.
	if res := s.do("POST", my+"/submit", fx.student, nil, nil); res.status != 428 {
		t.Errorf("gönderme sürüm ister: %d", res.status)
	}
	res = s.do("POST", my+"/submit", fx.student, map[string]string{"If-Match": `"1"`}, nil)
	if res.status != 200 || submissions(res)[0]["status"] != "SUBMITTED" || submissions(res)[0]["is_late"] != false ||
		res.body["can_edit"] != false || res.body["can_new_attempt"] != true {
		t.Fatalf("gönder: %d %s", res.status, res.raw)
	}
	if res := s.do("POST", my+"/submit", fx.student, map[string]string{"If-Match": `"2"`}, nil); res.status != 409 || res.body["code"] != "ALREADY_SUBMITTED" {
		t.Errorf("ikinci gönderme: %d %s", res.status, res.raw)
	}
	if res := s.do("PUT", my, fx.student, map[string]string{"If-Match": `"2"`}, map[string]any{"online_text_html": "<p>y</p>"}); res.status != 409 {
		t.Errorf("gönderilen deneme değişmez: %d", res.status)
	}

	// Teslim listesi: bırakan öğrenci yok; puanlanmayı bekleyen bir teslim.
	if res := s.do("GET", base+"/submissions", fx.student, nil, nil); res.status != 403 {
		t.Errorf("öğrenci listeyi göremez: %d", res.status)
	}
	if res := s.do("GET", base+"/submissions", fx.head, nil, nil); res.status != 200 || res.body["can_grade"] != false {
		t.Errorf("izleyici salt okunur görür: %d %s", res.status, res.raw)
	}
	res = s.do("GET", base+"/submissions", fx.assistant, nil, nil)
	sum := res.body["summary"].(map[string]any)
	if res.status != 200 || sum["students"] != 1.0 || sum["submitted"] != 1.0 || sum["needs_grading"] != 1.0 {
		t.Fatalf("liste: %d %s", res.status, res.raw)
	}
	student := base + "/submissions/" + fx.student
	if res := s.do("GET", student+"/files/"+pdf+"/download-url", fx.assistant, nil, nil); res.status != 200 || res.body["url"] == nil {
		t.Errorf("öğretim elemanı teslim dosyasını indirir: %d %s", res.status, res.raw)
	}
	if res := s.do("GET", base+"/submissions/"+fx.dropped, fx.assistant, nil, nil); res.status != 404 {
		t.Errorf("şubenin öğrencisi olmayan: %d", res.status)
	}

	// Puanlama: sınır, sürüm; yayımlanmadan öğrenci görmez.
	if res := s.do("PUT", student+"/grade", fx.head, nil, map[string]any{"score": 40}); res.status != 403 {
		t.Errorf("izleyici puanlayamaz: %d", res.status)
	}
	if res := s.do("PUT", student+"/grade", fx.primary, nil, map[string]any{"score": 60}); res.status != 400 {
		t.Errorf("en yüksek puanın üstü: %d", res.status)
	}
	res = s.do("PUT", student+"/grade", fx.primary, nil, map[string]any{"score": 40, "feedback_html": "<p>İyi</p>"})
	if res.status != 200 || res.body["grade"].(map[string]any)["score"] != 40.0 || res.body["grade"].(map[string]any)["attempt_no"] != 1.0 {
		t.Fatalf("puan: %d %s", res.status, res.raw)
	}
	if res := s.do("PUT", student+"/grade", fx.primary, nil, map[string]any{"score": 41}); res.status != 428 {
		t.Errorf("var olan puan sürümsüz yazılamaz: %d", res.status)
	}
	res = s.do("PUT", student+"/grade", fx.primary, map[string]string{"If-Match": `"1"`}, map[string]any{"score": 45.5, "feedback_html": "<p>Çok iyi</p>"})
	if res.status != 200 || res.body["grade"].(map[string]any)["version"] != 2.0 {
		t.Fatalf("puan güncelleme: %d %s", res.status, res.raw)
	}
	if res := s.do("GET", my, fx.student, nil, nil); res.body["grade"] != nil {
		t.Errorf("yayımlanmamış puan görünmez: %s", res.raw)
	}
	if res := s.do("POST", base+"/grades/release", fx.assistant, nil, nil); res.status != 200 {
		t.Fatalf("yayımla: %d %s", res.status, res.raw)
	}
	if res := s.do("POST", base+"/grades/release", fx.assistant, nil, nil); res.status != 409 {
		t.Errorf("ikinci yayımlama: %d", res.status)
	}
	res = s.do("GET", my, fx.student, nil, nil)
	if g := res.body["grade"].(map[string]any); g["score"] != 45.5 || g["final_score"] != 45.5 || g["feedback_html"] != "<p>Çok iyi</p>" {
		t.Errorf("öğrenci puanı görür: %s", res.raw)
	}

	// Yeni deneme öncekinin içeriğiyle başlar; geç gönderilen deneme kesintiye uğrar.
	res = s.do("POST", my+"/attempts", fx.student, nil, nil)
	if res.status != 201 || len(submissions(res)) != 2 || submissions(res)[1]["status"] != "DRAFT" ||
		len(submissions(res)[1]["files"].([]any)) != 1 || submissions(res)[1]["online_text_html"] != "<p>Cevap </p>" {
		t.Fatalf("yeni deneme: %d %s", res.status, res.raw)
	}
	*s.now = due.Add(12 * time.Hour) // yarım gün geç
	res = s.do("PUT", my, fx.student, map[string]string{"If-Match": `"1"`}, map[string]any{"online_text_html": "<p>Düzeltilmiş</p>", "file_ids": []string{}})
	if res.status != 200 {
		t.Fatalf("ikinci taslak: %d %s", res.status, res.raw)
	}
	var attached bool
	if err := s.pool.QueryRow(t.Context(), `SELECT attached_at IS NOT NULL FROM platform.files WHERE id = $1`, pdf).Scan(&attached); err != nil || !attached {
		t.Errorf("ilk denemede kalan dosya serbest kalmaz: %v %v", attached, err)
	}
	res = s.do("POST", my+"/submit", fx.student, map[string]string{"If-Match": `"2"`}, nil)
	if res.status != 200 || submissions(res)[1]["is_late"] != true || res.body["can_new_attempt"] != false {
		t.Fatalf("geç gönderme: %d %s", res.status, res.raw)
	}
	if res := s.do("POST", my+"/attempts", fx.student, nil, nil); res.status != 409 || res.body["code"] != "NO_ATTEMPTS_LEFT" {
		t.Errorf("deneme hakkı bitti: %d %s", res.status, res.raw)
	}
	res = s.do("GET", base+"/submissions", fx.primary, nil, nil)
	if sum := res.body["summary"].(map[string]any); sum["needs_grading"] != 1.0 || sum["late"] != 1.0 {
		t.Errorf("yeni deneme puanlanmayı bekler: %s", res.raw)
	}
	res = s.do("PUT", student+"/grade", fx.primary, map[string]string{"If-Match": `"2"`}, map[string]any{"score": 45, "feedback_html": "<p>Geç</p>"})
	if g := res.body["grade"].(map[string]any); g["late_penalty_pct"] != 10.0 || g["final_score"] != 40.5 || g["attempt_no"] != 2.0 {
		t.Errorf("geç kesintisi: %s", res.raw)
	}

	// Gönderim yorumları: öğrenci ve öğretim elemanı.
	if res := s.do("POST", my+"/comments", fx.student, nil, map[string]any{"body": "  "}); res.status != 400 {
		t.Errorf("boş yorum: %d", res.status)
	}
	s.do("POST", my+"/comments", fx.student, nil, map[string]any{"body": "Geç teslim için özür dilerim."})
	if res := s.do("POST", student+"/comments", fx.assistant, nil, map[string]any{"body": "Sorun değil."}); res.status != 201 {
		t.Fatalf("öğretim elemanı yorumu: %d %s", res.status, res.raw)
	}
	res = s.do("GET", my, fx.student, nil, nil)
	if c := res.body["comments"].([]any); len(c) != 2 || c[1].(map[string]any)["author_name"] != "Zehra Kurt" {
		t.Errorf("yorumlar: %s", res.raw)
	}

	// Kesin kapanıştan sonra teslim kapalı, okuma açık.
	*s.now = cutoff.Add(time.Minute)
	res = s.do("GET", my, fx.student, nil, nil)
	if res.status != 200 || res.body["assignment"].(map[string]any)["submission_state"] != "CLOSED" || res.body["can_edit"] != false {
		t.Errorf("kapanmış ödev okunur: %d %s", res.status, res.raw)
	}
	if res := s.do("GET", my+"/files/"+pdf+"/download-url", fx.student, nil, nil); res.status != 200 {
		t.Errorf("öğrenci kendi dosyasını indirir: %d", res.status)
	}
	if res := s.do("GET", my+"/files/"+others+"/download-url", fx.student, nil, nil); res.status != 404 {
		t.Errorf("başkasının dosyası indirilemez: %d", res.status)
	}
}

func TestAssignmentDenyAndTextOnly(t *testing.T) {
	s := newServer(t)
	fx := s.fx
	res := s.do("POST", "/api/v1/spaces/"+fx.section+"/topics", fx.primary, nil, map[string]any{"title": "Hafta 1"})
	acts := "/api/v1/topics/" + topics(res)[1]["id"].(string) + "/activities"
	due := s.now.Add(time.Hour).UTC()
	res = s.do("POST", acts, fx.primary, nil, map[string]any{"type": "ASSIGNMENT", "title": "Kısa yazı", "assignment": map[string]any{
		"due_at": due, "submission_types": []string{"ONLINE_TEXT"}, "late_policy": "DENY"}})
	if res.status != 201 {
		t.Fatalf("ödev: %d %s", res.status, res.raw)
	}
	my := "/api/v1/activities/" + res.body["id"].(string) + "/my-submission"
	pdf := s.upload(fx.student, "a.pdf", "%PDF-1.4 a")
	if res := s.do("PUT", my, fx.student, nil, map[string]any{"file_ids": []string{pdf}}); res.status != 400 {
		t.Errorf("sadece metin: %d", res.status)
	}
	res = s.do("PUT", my, fx.student, nil, map[string]any{"online_text_html": "<p></p>"})
	if res.status != 200 || res.body["can_submit"] != false {
		t.Fatalf("boş metin gönderilemez: %d %s", res.status, res.raw)
	}
	if res := s.do("POST", my+"/submit", fx.student, map[string]string{"If-Match": `"1"`}, nil); res.status != 409 || res.body["code"] != "EMPTY_SUBMISSION" {
		t.Errorf("boş gönderme: %d %s", res.status, res.raw)
	}
	*s.now = due.Add(time.Second)
	if res := s.do("PUT", my, fx.student, map[string]string{"If-Match": `"1"`}, map[string]any{"online_text_html": "<p>Geç</p>"}); res.status != 409 ||
		res.body["code"] != "SUBMISSION_CLOSED" {
		t.Errorf("geç teslim kapalı: %d %s", res.status, res.raw)
	}
}
