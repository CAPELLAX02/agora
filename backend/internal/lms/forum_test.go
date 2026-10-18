package lms_test

import (
	"strings"
	"testing"
)

func TestForums(t *testing.T) {
	s := newServer(t)
	fx := s.fx
	space := "/api/v1/spaces/" + fx.section

	// Ders alanı ilk içerikle oluşur; Genel bölümünde Duyurular forumu vardır, silinemez.
	res := s.do("POST", space+"/topics", fx.primary, nil, map[string]any{"title": "Hafta 1"})
	general := topics(res)[0]
	acts := activities(general)
	if len(acts) != 1 || acts[0]["type"] != "FORUM" || acts[0]["title"] != "Duyurular" ||
		acts[0]["forum"].(map[string]any)["kind"] != "ANNOUNCEMENTS" {
		t.Fatalf("duyurular forumu: %s", res.raw)
	}
	news := acts[0]["id"].(string)
	if res := s.do("DELETE", "/api/v1/activities/"+news, fx.primary, nil, nil); res.status != 400 {
		t.Errorf("duyurular silinemez: %d", res.status)
	}

	// Duyurularda konuyu ve yanıtı öğretim elemanı yazar.
	newsThreads := "/api/v1/activities/" + news + "/threads"
	if res := s.do("POST", newsThreads, fx.student, nil, map[string]any{"title": "Soru", "body_html": "<p>x</p>"}); res.status != 403 {
		t.Errorf("öğrenci duyuru açamaz: %d", res.status)
	}
	res = s.do("POST", newsThreads, fx.assistant, nil, map[string]any{"title": "Ara sınav tarihi", "body_html": "<p>12 Kasım <script>x</script></p>"})
	if res.status != 201 || res.body["posts"].([]any)[0].(map[string]any)["body_html"] != "<p>12 Kasım </p>" ||
		res.body["author"].(map[string]any)["is_instructor"] != true {
		t.Fatalf("duyuru: %d %s", res.status, res.raw)
	}
	announcement := "/api/v1/threads/" + res.body["id"].(string)
	res = s.do("GET", newsThreads, fx.student, nil, nil)
	if res.status != 200 || len(res.body["items"].([]any)) != 1 || res.body["can_start"] != false {
		t.Errorf("öğrenci duyuruları görür: %d %s", res.status, res.raw)
	}
	if res := s.do("GET", announcement, fx.student, nil, nil); res.body["can_reply"] != false {
		t.Errorf("öğrenci duyuruya yanıt yazamaz: %s", res.raw)
	}
	if res := s.do("POST", announcement+"/posts", fx.student, nil, map[string]any{"body_html": "<p>Teşekkürler</p>"}); res.status != 403 {
		t.Errorf("duyuruya yanıt: %d", res.status)
	}
	if res := s.do("GET", newsThreads, fx.dropped, nil, nil); res.status != 404 {
		t.Errorf("bırakan öğrenci: %d", res.status)
	}

	// Genel forum: öğrenci konu açar, üyeler yanıtlar.
	week1 := "/api/v1/topics/" + topics(res0(s, space, fx.primary))[1]["id"].(string) + "/activities"
	res = s.do("POST", week1, fx.primary, nil, map[string]any{"type": "FORUM", "title": "Soru-cevap"})
	if res.status != 201 || res.body["forum"].(map[string]any)["kind"] != "GENERAL" || res.body["forum"].(map[string]any)["students_can_start_threads"] != true {
		t.Fatalf("genel forum: %d %s", res.status, res.raw)
	}
	qa := "/api/v1/activities/" + res.body["id"].(string) + "/threads"
	if res := s.do("POST", qa, fx.student, nil, map[string]any{"title": "", "body_html": "<p>x</p>"}); res.status != 400 {
		t.Errorf("başlıksız konu: %d", res.status)
	}
	if res := s.do("POST", qa, fx.student, nil, map[string]any{"title": "Boş", "body_html": "<p> </p>"}); res.status != 400 {
		t.Errorf("boş gönderi: %d", res.status)
	}
	res = s.do("POST", qa, fx.student, nil, map[string]any{"title": "Döngü sorusu", "body_html": "<p>for ile while farkı?</p>"})
	if res.status != 201 {
		t.Fatalf("öğrenci konu açar: %d %s", res.status, res.raw)
	}
	thread := "/api/v1/threads/" + res.body["id"].(string)
	first := res.body["posts"].([]any)[0].(map[string]any)["id"].(string)
	res = s.do("POST", thread+"/posts", fx.assistant, nil, map[string]any{"body_html": "<p>while koşula bakar.</p>", "parent_post_id": first})
	posts := res.body["posts"].([]any)
	if res.status != 201 || len(posts) != 2 || posts[1].(map[string]any)["parent_post_id"] != first || res.body["post_count"] != 2.0 {
		t.Fatalf("yanıt: %d %s", res.status, res.raw)
	}
	answer := posts[1].(map[string]any)["id"].(string)

	// Gönderiyi yazarı ya da öğretim elemanı düzenler.
	if res := s.do("PUT", "/api/v1/posts/"+answer, fx.student, nil, map[string]any{"body_html": "<p>değiştir</p>"}); res.status != 403 {
		t.Errorf("başkasının gönderisi: %d", res.status)
	}
	res = s.do("PUT", "/api/v1/posts/"+first, fx.student, nil, map[string]any{"body_html": "<p>for ile while arasındaki fark nedir?</p>"})
	if res.status != 200 || res.body["posts"].([]any)[0].(map[string]any)["edited_at"] == nil {
		t.Errorf("kendi gönderisi: %d %s", res.status, res.raw)
	}
	if res := s.do("PUT", "/api/v1/posts/"+first, fx.primary, nil, map[string]any{"body_html": "<p>Düzenlendi</p>"}); res.status != 200 {
		t.Errorf("öğretim elemanı düzenler: %d", res.status)
	}

	// Sabitleme ve kilitleme öğretim elemanınındır; kilitli konuya öğrenci yazamaz.
	if res := s.do("PUT", thread, fx.student, nil, map[string]any{"is_locked": true}); res.status != 403 {
		t.Errorf("öğrenci kilitleyemez: %d", res.status)
	}
	res = s.do("PUT", thread, fx.primary, nil, map[string]any{"is_locked": true, "is_pinned": true})
	if res.status != 200 || res.body["is_locked"] != true {
		t.Fatalf("kilitle: %d %s", res.status, res.raw)
	}
	if res := s.do("POST", thread+"/posts", fx.student, nil, map[string]any{"body_html": "<p>bir şey daha</p>"}); res.status != 403 ||
		!strings.Contains(res.raw, "kilitli") {
		t.Errorf("kilitli konu: %d %s", res.status, res.raw)
	}
	if res := s.do("POST", thread+"/posts", fx.primary, nil, map[string]any{"body_html": "<p>Kapatıyorum.</p>"}); res.status != 201 {
		t.Errorf("öğretim elemanı kilitli konuya yazar: %d", res.status)
	}
	s.do("POST", qa, fx.student, nil, map[string]any{"title": "İkinci konu", "body_html": "<p>y</p>"})
	if res := s.do("GET", qa, fx.student, nil, nil); res.body["items"].([]any)[0].(map[string]any)["title"] != "Döngü sorusu" {
		t.Errorf("sabitlenen konu önce: %s", res.raw)
	}

	// İzleyici okur, yazamaz.
	if res := s.do("GET", thread, fx.head, nil, nil); res.status != 200 || res.body["can_reply"] != false || res.body["can_moderate"] != false {
		t.Errorf("izleyici: %d %s", res.status, res.raw)
	}
	if res := s.do("POST", qa, fx.head, nil, map[string]any{"title": "x", "body_html": "<p>x</p>"}); res.status != 403 {
		t.Errorf("izleyici konu açamaz: %d", res.status)
	}

	// Yanıt silinince yerinde "silindi" kalır; ilk gönderi silinince konu silinir.
	if res := s.do("DELETE", "/api/v1/posts/"+answer, fx.assistant, nil, nil); res.status != 204 {
		t.Fatalf("yanıtı sil: %d %s", res.status, res.raw)
	}
	res = s.do("GET", thread, fx.student, nil, nil)
	if p := res.body["posts"].([]any)[1].(map[string]any); p["deleted"] != true || p["body_html"] != "" || res.body["post_count"] != 2.0 {
		t.Errorf("silinen yanıt: %s", res.raw)
	}
	if res := s.do("DELETE", "/api/v1/posts/"+first, fx.student, nil, nil); res.status != 204 {
		t.Fatalf("ilk gönderiyi sil: %d %s", res.status, res.raw)
	}
	if res := s.do("GET", thread, fx.student, nil, nil); res.status != 404 {
		t.Errorf("konu silinir: %d", res.status)
	}

	// Öğrencilerin konu açmadığı forum.
	res = s.do("POST", week1, fx.primary, nil, map[string]any{"type": "FORUM", "title": "Tartışma", "forum": map[string]any{"students_can_start_threads": false}})
	closed := "/api/v1/activities/" + res.body["id"].(string) + "/threads"
	if res := s.do("POST", closed, fx.student, nil, map[string]any{"title": "x", "body_html": "<p>x</p>"}); res.status != 403 {
		t.Errorf("öğrenci konu açamaz: %d", res.status)
	}
}

// res0, ders alanının güncel hâlidir.
func res0(s *server, space, user string) result {
	return s.do("GET", space, user, nil, nil)
}
