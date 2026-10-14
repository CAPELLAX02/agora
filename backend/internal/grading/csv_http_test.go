package grading_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/CAPELLAX02/agora/backend/internal/grading"
)

func importRow(res result, line int) map[string]any {
	for _, r := range res.body["rows"].([]any) {
		if m := r.(map[string]any); m["line"] == float64(line) {
			return m
		}
	}
	return nil
}

func TestHTTPScoreCSV(t *testing.T) {
	s := newGradebookServer(t)
	fx := s.fx
	ctx := context.Background()
	section := "/api/v1/sections/" + fx.section
	midterm := "/api/v1/assessment-components/" + fx.components["MIDTERM"] + "/scores/import"
	preview := func(path, body string) result {
		return s.do("POST", path+"/preview", fx.ayseUser, mfa, csvBody(body))
	}
	if res := s.do("POST", section+"/assessment-plan/lock", fx.ayseUser, nil, nil); res.status != 200 {
		t.Fatalf("kilit: %d %s", res.status, res.raw)
	}

	// Dışa aktarma: not tablosunu görenler; başka öğretim elemanı ve öğrenci göremez.
	for user, want := range map[string]int{fx.ayseUser: 200, head: 200, fx.mehmetUser: 403, student: 403} {
		if res := s.do("GET", section+"/gradebook.csv", user, nil, nil); res.status != want {
			t.Errorf("dışa aktarma %s: %d, beklenen %d", user, res.status, want)
		}
	}
	if res := s.do("GET", section+"/gradebook.csv?lang=de", fx.ayseUser, nil, nil); res.status != 400 {
		t.Errorf("geçersiz dil: %d", res.status)
	}

	// Önizleme not yazma yetkisi ister: MFA'lı oturum ve şubenin öğretim elemanı.
	file := "Öğrenci No;Ad;Ara sınav\n22290102;Ece;75,5\n22290103;Can;G\n99999999;Yok;50\n22290101;Ali;abc\n22290102;Ece;80\n"
	if res := s.do("POST", midterm+"/preview", fx.ayseUser, nil, csvBody(file)); res.status != 403 || res.code() != "MFA_REQUIRED" {
		t.Errorf("MFA'sız önizleme: %d %s", res.status, res.raw)
	}
	if res := s.do("POST", midterm+"/preview", fx.mehmetUser, mfa, csvBody(file)); res.status != 403 || res.code() != "FORBIDDEN" {
		t.Errorf("şubede olmayan öğretim elemanı: %d %s", res.status, res.raw)
	}
	if res := s.do("POST", midterm+"/preview", fx.ayseUser, mfa, map[string]any{"csv": file}); res.status != 415 {
		t.Errorf("JSON gövde: %d %s", res.status, res.raw)
	}

	// Satır satır doğrulama: tekrar eden öğrencinin iki satırı da hatalı.
	res := preview(midterm, file)
	if res.status != 200 {
		t.Fatalf("önizleme: %d %s", res.status, res.raw)
	}
	if sum := res.body["summary"].(map[string]any); sum["changed"] != 1.0 || sum["errors"] != 4.0 || res.body["column"] != "Ara sınav" {
		t.Errorf("özet: %s", res.raw)
	}
	for line, code := range map[int]any{2: "DUPLICATE_STUDENT", 3: nil, 4: "NOT_IN_SECTION", 5: "INVALID_SCORE", 6: "DUPLICATE_STUDENT"} {
		if r := importRow(res, line); r == nil || r["error_code"] != code {
			t.Errorf("satır %d: %v, beklenen kod %v", line, r, code)
		}
	}
	if r := importRow(res, 3); r["status"] != "CHANGED" || r["current"] != nil ||
		r["next"].(map[string]any)["absent"] != true || r["student"].(map[string]any)["last_name"] != "Çelik" {
		t.Errorf("girmedi satırı: %v", r)
	}
	version := fmt.Sprintf(`"%v"`, res.body["version"])

	// Hatalı satır varsa hiçbir not yazılmaz.
	bad := s.do("POST", midterm, fx.ayseUser, with(mfa, "If-Match", version), csvBody(file))
	if bad.status != 400 || bad.code() != "VALIDATION_FAILED" || len(bad.body["errors"].([]any)) != 4 {
		t.Fatalf("hatalı dosya: %d %s", bad.status, bad.raw)
	}
	if e := bad.body["errors"].([]any)[0].(map[string]any); e["field"] != "rows.2" || e["code"] != "DUPLICATE_STUDENT" {
		t.Errorf("satır hatası: %v", e)
	}
	if n := s.metrics.get("scores/csv"); n != 0 {
		t.Errorf("hatalı dosyada not sayılmamalı: %d", n)
	}
	if res := s.do("POST", midterm, fx.ayseUser, mfa, csvBody(file)); res.status != 428 {
		t.Errorf("If-Match'siz: %d", res.status)
	}

	// Düzeltilmiş dosya: Ali dosyada yok, notu değişmez.
	good := "Öğrenci No;Ad;Ara sınav\n22290102;Ece;75,5\n22290103;Can;G\n"
	res = preview(midterm, good)
	if missing := res.body["missing"].([]any); len(missing) != 1 || missing[0].(map[string]any)["student_no"] != "22290101" {
		t.Errorf("dosyada olmayanlar: %s", res.raw)
	}
	res = s.do("POST", midterm, fx.ayseUser, with(mfa, "If-Match", version), csvBody(good))
	if res.status != 200 {
		t.Fatalf("içe aktarma: %d %s", res.status, res.raw)
	}
	if sc := studentRow(res, fx.enrollments[1])["scores"].(map[string]any)[fx.components["MIDTERM"]].(map[string]any); sc["score"] != 75.5 {
		t.Errorf("Ece'nin notu: %v", sc)
	}
	if sc := studentRow(res, fx.enrollments[2])["scores"].(map[string]any)[fx.components["MIDTERM"]].(map[string]any); sc["absent"] != true {
		t.Errorf("Can girmedi: %v", sc)
	}
	if csv, manual := s.metrics.get("scores/csv"), s.metrics.get("scores/manual"); csv != 2 || manual != 0 {
		t.Errorf("not sayaçları: csv %d, manual %d", csv, manual)
	}
	var sources, action string
	if err := s.pool.QueryRow(ctx, `SELECT string_agg(DISTINCT source, ',') FROM grading.assessment_scores`).Scan(&sources); err != nil || sources != "CSV" {
		t.Errorf("kaynak: %q %v", sources, err)
	}
	if err := s.pool.QueryRow(ctx, `SELECT action FROM audit.audit_log WHERE entity_id = $1 ORDER BY occurred_at DESC LIMIT 1`,
		fx.components["MIDTERM"]).Scan(&action); err != nil || action != "assessment_scores.import" {
		t.Errorf("denetim kaydı: %q %v", action, err)
	}

	// Aynı önizlemeyle ikinci kez uygulanamaz: notlar önizlemeden sonra değişti.
	if res := s.do("POST", midterm, fx.ayseUser, with(mfa, "If-Match", version), csvBody(good)); res.status != 412 {
		t.Errorf("eski önizleme: %d %s", res.status, res.raw)
	}
	// Boş hücre notu silmez; değişmeyen not değişmedi görünür.
	res = preview(midterm, "Öğrenci No;Ara sınav\n22290102;\n22290103;g\n")
	if importRow(res, 2)["status"] != "EMPTY" || importRow(res, 3)["status"] != "UNCHANGED" {
		t.Errorf("boş ve değişmeyen: %s", res.raw)
	}

	// Dışa aktarılan dosya girilen notları içerir ve şablon olarak geri okunur.
	export := s.do("GET", section+"/gradebook.csv", fx.ayseUser, nil, nil)
	if !strings.HasPrefix(export.header.Get("Content-Type"), "text/csv") ||
		!strings.Contains(export.header.Get("Content-Disposition"), `_notlar.csv"`) || export.header.Get("Cache-Control") != "no-store" {
		t.Errorf("başlıklar: %v", export.header)
	}
	if !strings.Contains(export.raw, "22290102;Arslan;Ece;BIL-L;Devam ediyor;75,5;") || !strings.Contains(export.raw, "22290103;Çelik;Can;BIL-L;Devam ediyor;G;") {
		t.Errorf("dışa aktarılan dosya:\n%s", export.raw)
	}
	res = preview(midterm, export.raw)
	if sum := res.body["summary"].(map[string]any); res.status != 200 || sum["unchanged"] != 2.0 || sum["empty"] != 1.0 || sum["errors"] != 0.0 {
		t.Errorf("dışa aktarılanı önizleme: %d %s", res.status, res.raw)
	}

	// Dosya düzeyinde sorunlar ve not girme koşulları.
	if res := preview(midterm, "Ad;Soyad\nEce;Arslan\n"); res.status != 400 || res.code() != "INVALID_CSV" {
		t.Errorf("öğrenci no sütunu yok: %d %s", res.status, res.raw)
	}
	final := "/api/v1/assessment-components/" + fx.components["FINAL"] + "/scores/import"
	if res := preview(final, good); res.status != 409 || res.code() != "GRADE_ENTRY_WINDOW_CLOSED" {
		t.Errorf("kapalı pencere: %d %s", res.status, res.raw)
	}
	s.calendar.set(grading.WindowFinalEntry, true)
	if res := preview(final, good); res.status != 400 || res.code() != "INVALID_CSV" {
		t.Errorf("finalin sütunu yok: %d %s", res.status, res.raw)
	}
	if res := preview(final, "22290102;45\n"); res.status != 200 || res.body["column"] != nil {
		t.Errorf("başlıksız dosya: %d %s", res.status, res.raw)
	}
}
