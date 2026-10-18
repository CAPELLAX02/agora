package lms_test

import (
	"testing"
	"time"
)

func progress(res result) (float64, float64) {
	p, ok := res.body["progress"].(map[string]any)
	if !ok {
		return -1, -1
	}
	return p["completed"].(float64), p["total"].(float64)
}

func TestCompletionAndProgress(t *testing.T) {
	s := newServer(t)
	fx := s.fx
	space := "/api/v1/spaces/" + fx.section
	res := s.do("POST", space+"/topics", fx.primary, nil, map[string]any{"title": "Hafta 1"})
	acts := "/api/v1/topics/" + topics(res)[1]["id"].(string) + "/activities"
	create := func(body map[string]any) string {
		t.Helper()
		res := s.do("POST", acts, fx.primary, nil, body)
		if res.status != 201 {
			t.Fatalf("%v: %d %s", body["type"], res.status, res.raw)
		}
		return res.body["id"].(string)
	}
	pdf := s.upload(fx.primary, "ders.pdf", "%PDF-1.4 x")
	resource := create(map[string]any{"type": "RESOURCE", "title": "Ders notu", "file_ids": []string{pdf}})
	page := create(map[string]any{"type": "PAGE", "title": "Okuma", "body_html": "<p>Metin</p>"})
	link := create(map[string]any{"type": "URL", "title": "Go turu", "url": "https://go.dev/tour"})
	create(map[string]any{"type": "LABEL", "title": "Not", "description_html": "<p>Etiket</p>"})
	create(map[string]any{"type": "PAGE", "title": "Gizli sayfa", "is_visible": false})
	assignment := create(map[string]any{"type": "ASSIGNMENT", "title": "Ödev", "assignment": map[string]any{
		"due_at": s.now.Add(48 * time.Hour).UTC(), "submission_types": []string{"ONLINE_TEXT"}}})

	// Duyurular forumu + dosya + sayfa + bağlantı + ödev; etiket ve gizli sayfa sayılmaz.
	if c, total := progress(s.do("GET", space, fx.student, nil, nil)); c != 0 || total != 5 {
		t.Fatalf("başlangıç: %v/%v", c, total)
	}
	if c, _ := progress(s.do("GET", space, fx.primary, nil, nil)); c != -1 {
		t.Errorf("öğretim elemanının ilerlemesi olmaz")
	}

	s.do("GET", "/api/v1/activities/"+page, fx.student, nil, nil)
	if res := s.do("GET", "/api/v1/activities/"+resource+"/files/"+pdf+"/download-url", fx.student, nil, nil); res.status != 200 {
		t.Fatalf("indir: %d", res.status)
	}
	if res := s.do("POST", "/api/v1/activities/"+link+"/views", fx.student, nil, nil); res.status != 204 {
		t.Fatalf("bağlantı: %d %s", res.status, res.raw)
	}
	s.do("POST", "/api/v1/activities/"+link+"/views", fx.student, nil, nil) // tekrarı zararsız
	// Ödevin sayfasını açmak tamamlamaz; göndermek tamamlar.
	s.do("GET", "/api/v1/activities/"+assignment, fx.student, nil, nil)
	if c, _ := progress(s.do("GET", space, fx.student, nil, nil)); c != 3 {
		t.Errorf("görüntüleme ve indirme: %v", c)
	}
	my := "/api/v1/activities/" + assignment + "/my-submission"
	s.do("PUT", my, fx.student, nil, map[string]any{"online_text_html": "<p>Cevap</p>"})
	s.do("POST", my+"/submit", fx.student, map[string]string{"If-Match": `"1"`}, nil)

	res = s.do("GET", space, fx.student, nil, nil)
	if c, total := progress(res); c != 4 || total != 5 {
		t.Errorf("gönderince: %v/%v", c, total)
	}
	done := map[string]bool{}
	for _, tp := range topics(res) {
		for _, a := range activities(tp) {
			done[a["title"].(string)] = a["completed"].(bool)
		}
	}
	if !done["Ders notu"] || !done["Okuma"] || !done["Go turu"] || !done["Ödev"] || done["Duyurular"] {
		t.Errorf("tamamlananlar: %v", done)
	}
	res = s.do("GET", "/api/v1/me/spaces", fx.student, nil, nil)
	if p := res.body["items"].([]any)[0].(map[string]any)["progress"].(map[string]any); p["completed"] != 4.0 || p["total"] != 5.0 {
		t.Errorf("derslerim ilerlemesi: %s", res.raw)
	}
	if res := s.do("POST", "/api/v1/activities/"+link+"/views", fx.dropped, nil, nil); res.status != 404 {
		t.Errorf("bırakan öğrenci: %d", res.status)
	}
}
