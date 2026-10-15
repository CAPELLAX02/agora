package lms_test

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/CAPELLAX02/agora/backend/internal/platform/sheet"
)

func (s *server) raw(method, path, user string, headers map[string]string, contentType string, body []byte) result {
	s.t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("X-Test-User", user)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	s.h.ServeHTTP(rec, req)
	res := result{status: rec.Code, header: rec.Header(), raw: rec.Body.String()}
	_ = json.Unmarshal(rec.Body.Bytes(), &res.body)
	return res
}

// newAssignment, sorumlu öğretim elemanının ilk bölüme eklediği bir ödevdir (en yüksek puan 50).
func newAssignment(t *testing.T, s *server, title string) string {
	t.Helper()
	res := s.do("POST", "/api/v1/spaces/"+s.fx.section+"/topics", s.fx.primary, nil, map[string]any{"title": "Hafta"})
	tp := topics(res)
	acts := "/api/v1/topics/" + tp[len(tp)-1]["id"].(string) + "/activities"
	res = s.do("POST", acts, s.fx.primary, nil, map[string]any{"type": "ASSIGNMENT", "title": title, "assignment": map[string]any{
		"due_at": s.now.Add(48 * time.Hour).UTC(), "max_score": 50, "submission_types": []string{"FILE", "ONLINE_TEXT"}}})
	if res.status != 201 {
		t.Fatalf("ödev: %d %s", res.status, res.raw)
	}
	return res.body["id"].(string)
}

func TestGradeSheetCSVImport(t *testing.T) {
	s := newServer(t)
	fx := s.fx
	id := newAssignment(t, s, "Ödev 1")
	base := "/api/v1/activities/" + id
	csv := []byte("Öğrenci No;Puan;Geri Bildirim\n26290101;45,5;İyi iş\n26290102;10;\n\n99999999;;\n26290101;;\n")

	if res := s.raw("POST", base+"/grades/import/preview", fx.student, nil, sheet.MediaCSV, csv); res.status != 403 {
		t.Errorf("öğrenci içe aktaramaz: %d", res.status)
	}
	if res := s.raw("POST", base+"/grades/import/preview", fx.head, nil, sheet.MediaCSV, csv); res.status != 403 {
		t.Errorf("izleyici içe aktaramaz: %d", res.status)
	}
	res := s.raw("POST", base+"/grades/import/preview", fx.assistant, nil, sheet.MediaCSV, csv)
	if res.status != 200 {
		t.Fatalf("önizleme: %d %s", res.status, res.raw)
	}
	sum := res.body["summary"].(map[string]any)
	rows := res.body["rows"].([]any)
	if sum["changed"] != 1.0 || sum["errors"] != 1.0 || sum["empty"] != 2.0 || res.body["version"] != 0.0 ||
		rows[1].(map[string]any)["error_code"] != "NOT_IN_SECTION" || rows[0].(map[string]any)["next_score"] != 45.5 ||
		rows[0].(map[string]any)["feedback_changed"] != true || len(res.body["missing"].([]any)) != 0 {
		t.Fatalf("önizleme: %s", res.raw)
	}
	// Bırakan öğrencinin satırı şubede değil; hatalı satır varken hiçbir puan yazılmaz.
	if res := s.raw("POST", base+"/grades/import", fx.assistant, map[string]string{"If-Match": `"0"`}, sheet.MediaCSV, csv); res.status != 400 ||
		!strings.Contains(res.raw, "rows.3") {
		t.Errorf("hatalı satır: %d %s", res.status, res.raw)
	}
	good := []byte("Öğrenci No;Puan;Geri Bildirim\n26290101;45,5;İyi iş\nikinci satır\n")
	good = []byte(strings.Replace(string(good), "İyi iş\nikinci satır", "\"İyi iş\nikinci satır\"", 1))
	if res := s.raw("POST", base+"/grades/import", fx.assistant, nil, sheet.MediaCSV, good); res.status != 428 {
		t.Errorf("sürüm gerekli: %d", res.status)
	}
	res = s.raw("POST", base+"/grades/import", fx.assistant, map[string]string{"If-Match": `"0"`}, sheet.MediaCSV, good)
	if res.status != 200 {
		t.Fatalf("içe aktar: %d %s", res.status, res.raw)
	}
	g := res.body["items"].([]any)[0].(map[string]any)["grade"].(map[string]any)
	if g["score"] != 45.5 || g["feedback_html"] != "<p>İyi iş</p><p>ikinci satır</p>" {
		t.Errorf("puan: %s", res.raw)
	}
	// Aynı dosya: değişiklik yok; eski sürümle yükleme reddedilir.
	res = s.raw("POST", base+"/grades/import/preview", fx.primary, nil, sheet.MediaCSV, good)
	if res.body["summary"].(map[string]any)["unchanged"] != 1.0 || res.body["version"] != 1.0 {
		t.Errorf("değişmeyen: %s", res.raw)
	}
	changed := []byte("Öğrenci No;Puan\n26290101;40\n")
	if res := s.raw("POST", base+"/grades/import", fx.primary, map[string]string{"If-Match": `"0"`}, sheet.MediaCSV, changed); res.status != 412 {
		t.Errorf("eski sürüm: %d %s", res.status, res.raw)
	}
	// Puan sütunu boş, geri bildirim yok: sadece puan değişir, geri bildirim korunur.
	res = s.raw("POST", base+"/grades/import", fx.primary, map[string]string{"If-Match": `"1"`}, sheet.MediaCSV, changed)
	g = res.body["items"].([]any)[0].(map[string]any)["grade"].(map[string]any)
	if res.status != 200 || g["score"] != 40.0 || g["feedback_html"] != "<p>İyi iş</p><p>ikinci satır</p>" {
		t.Errorf("puan güncelleme: %d %s", res.status, res.raw)
	}
	if res := s.raw("POST", base+"/grades/import/preview", fx.primary, nil, sheet.MediaCSV, []byte("Öğrenci No;Puan\n26290101;50,5\n")); res.body["rows"].([]any)[0].(map[string]any)["error_code"] != "INVALID_SCORE" {
		t.Errorf("en yüksek puanın üstü: %s", res.raw)
	}
}

func TestGradeSheetXLSXRoundTrip(t *testing.T) {
	s := newServer(t)
	fx := s.fx
	id := newAssignment(t, s, "Ödev 1")
	other := newAssignment(t, s, "Ödev 2")
	base := "/api/v1/activities/" + id

	// Biçimli geri bildirim şablonda düz metindir; değiştirilmezse biçimi korunur.
	s.do("PUT", base+"/submissions/"+fx.student+"/grade", fx.primary, nil, map[string]any{"score": 30, "feedback_html": "<p><strong>Güzel</strong> çözüm</p>"})
	if res := s.do("GET", base+"/grades.xlsx", fx.student, nil, nil); res.status != 403 {
		t.Errorf("öğrenci şablon indiremez: %d", res.status)
	}
	res := s.raw("GET", base+"/grades.xlsx?lang=tr", fx.primary, nil, "", nil)
	if res.status != 200 || res.header.Get("Content-Type") != sheet.MediaXLSX || !strings.Contains(res.header.Get("Content-Disposition"), "COM1001-1_Odev_1_puanlar.xlsx") {
		t.Fatalf("şablon: %d %v", res.status, res.header)
	}
	f, err := sheet.Read([]byte(res.raw), sheet.MediaXLSX, 100)
	if err != nil {
		t.Fatal(err)
	}
	if f.Meta["kind"] != "assignment_grades" || f.Meta["activity_id"] != id || f.Meta["score"] != "H" || f.Meta["feedback"] != "I" {
		t.Errorf("kimlik sayfası: %v", f.Meta)
	}
	var studentRow int
	for _, r := range f.Rows {
		if r.Cell(1) == "26290101" {
			studentRow = r.Line
			if r.Cell(4) != "Teslim edilmedi" || r.Cell(7) != "30" || r.Cell(8) != "Güzel çözüm" {
				t.Errorf("öğrenci satırı: %q", r.Cells)
			}
		}
	}
	if studentRow == 0 {
		t.Fatalf("öğrenci satırı yok: %v", f.Rows)
	}

	// Excel'de puan değiştirilip yüklenir.
	book, err := excelize.OpenReader(bytes.NewReader([]byte(res.raw)))
	if err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellValue("Puanlar", "H"+itoa(studentRow), 42.5); err != nil {
		t.Fatal(err)
	}
	var edited bytes.Buffer
	if err := book.Write(&edited); err != nil {
		t.Fatal(err)
	}
	res = s.raw("POST", base+"/grades/import/preview", fx.primary, nil, sheet.MediaXLSX, edited.Bytes())
	row := res.body["rows"].([]any)[0].(map[string]any)
	if res.status != 200 || row["status"] != "CHANGED" || row["next_score"] != 42.5 || row["feedback_changed"] != false {
		t.Fatalf("xlsx önizleme: %d %s", res.status, res.raw)
	}
	res = s.raw("POST", base+"/grades/import", fx.primary, map[string]string{"If-Match": `"1"`}, sheet.MediaXLSX, edited.Bytes())
	g := res.body["items"].([]any)[0].(map[string]any)["grade"].(map[string]any)
	if res.status != 200 || g["score"] != 42.5 || g["feedback_html"] != "<p><strong>Güzel</strong> çözüm</p>" {
		t.Errorf("xlsx içe aktarma: %d %s", res.status, res.raw)
	}
	// Başka ödevin şablonu reddedilir.
	if res := s.raw("POST", "/api/v1/activities/"+other+"/grades/import/preview", fx.primary, nil, sheet.MediaXLSX, edited.Bytes()); res.status != 400 ||
		res.body["code"] != "INVALID_FILE" {
		t.Errorf("başka ödevin şablonu: %d %s", res.status, res.raw)
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
