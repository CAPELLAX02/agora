package lms_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// notes, kullanıcının bildirim türleridir (eskiden yeniye).
func (s *server) notes(userID string) []string {
	s.t.Helper()
	rows, err := s.pool.Query(context.Background(), `
		SELECT type_code FROM communication.notifications WHERE user_id = $1 ORDER BY id`, userID)
	if err != nil {
		s.t.Fatal(err)
	}
	out, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		s.t.Fatal(err)
	}
	return out
}

func TestSpaceNotifications(t *testing.T) {
	s := newServer(t)
	fx := s.fx
	space := "/api/v1/spaces/" + fx.section
	res := s.do("POST", space+"/topics", fx.primary, nil, map[string]any{"title": "Hafta 1"})
	news := activities(topics(res)[0])[0]["id"].(string) // Genel bölümündeki Duyurular
	acts := "/api/v1/topics/" + topics(res)[1]["id"].(string) + "/activities"
	assignment := func(title string, visible bool) string {
		t.Helper()
		res := s.do("POST", acts, fx.primary, nil, map[string]any{"type": "ASSIGNMENT", "title": title, "is_visible": visible,
			"assignment": map[string]any{"due_at": time.Date(2026, time.October, 21, 20, 59, 0, 0, time.UTC), "submission_types": []string{"ONLINE_TEXT"}}})
		if res.status != 201 {
			t.Fatalf("ödev: %d %s", res.status, res.raw)
		}
		return res.body["id"].(string)
	}

	// Görünür ödev öğrencilere bildirilir (dersi bırakan ve oluşturan hariç), son tarihiyle.
	a1 := assignment("Ödev 1", true)
	if got := s.notes(fx.student); !slices.Equal(got, []string{"lms.assignment_created"}) {
		t.Fatalf("öğrenci: %v", got)
	}
	if got := append(s.notes(fx.dropped), s.notes(fx.primary)...); len(got) != 0 {
		t.Errorf("bırakan ve oluşturan: %v", got)
	}
	var body, link string
	if err := s.pool.QueryRow(context.Background(), `
		SELECT payload->>'due_tr', link_path FROM communication.notifications WHERE user_id = $1`, fx.student).Scan(&body, &link); err != nil {
		t.Fatal(err)
	}
	if body != "21 Ekim 2026 23:59" || link != "/derslerim/"+fx.section+"/etkinlik/"+a1 {
		t.Errorf("veri: %q %q", body, link)
	}
	// Gizli ödev açılınca bildirilir.
	hidden := assignment("Ödev 2", false)
	if n := len(s.notes(fx.student)); n != 1 {
		t.Errorf("gizli ödev bildirildi")
	}
	if res := s.do("PUT", "/api/v1/activities/"+hidden, fx.primary, map[string]string{"If-Match": `"1"`}, map[string]any{
		"type": "ASSIGNMENT", "title": "Ödev 2", "is_visible": true, "assignment": map[string]any{"submission_types": []string{"ONLINE_TEXT"}}}); res.status != 200 {
		t.Fatalf("görünür yap: %d %s", res.status, res.raw)
	}
	if got := s.notes(fx.student); len(got) != 2 {
		t.Errorf("açılan ödev: %v", got)
	}

	// Puanlar yayımlanınca puanı olan öğrenciye.
	base := "/api/v1/activities/" + a1
	if res := s.do("PUT", base+"/submissions/"+fx.student+"/grade", fx.primary, nil, map[string]any{"score": 80}); res.status != 200 {
		t.Fatalf("puan: %d %s", res.status, res.raw)
	}
	if res := s.do("POST", base+"/grades/release", fx.assistant, nil, nil); res.status != 200 {
		t.Fatalf("yayım: %d %s", res.status, res.raw)
	}
	if got := s.notes(fx.student); got[len(got)-1] != "lms.grades_released" {
		t.Errorf("puan bildirimi: %v", got)
	}

	// Duyuru bütün üyelere (yazan hariç), metniyle.
	if res := s.do("POST", "/api/v1/activities/"+news+"/threads", fx.primary, nil, map[string]any{
		"title": "Ara sınav tarihi", "body_html": "<p>Ara sınav <strong>12 Kasım</strong>'da.</p><p>Başarılar.</p>"}); res.status != 201 {
		t.Fatalf("duyuru: %d %s", res.status, res.raw)
	}
	for _, u := range []string{fx.student, fx.assistant} {
		if got := s.notes(u); got[len(got)-1] != "forum.announcement" {
			t.Errorf("duyuru bildirimi: %v", got)
		}
	}
	if got := s.notes(fx.primary); len(got) != 0 {
		t.Errorf("duyuruyu yazan: %v", got)
	}
	var excerpt string
	if err := s.pool.QueryRow(context.Background(), `
		SELECT payload->>'excerpt' FROM communication.notifications WHERE user_id = $1 AND type_code = 'forum.announcement'`, fx.student).Scan(&excerpt); err != nil {
		t.Fatal(err)
	}
	if excerpt != "Ara sınav 12 Kasım'da. Başarılar." {
		t.Errorf("duyuru metni: %q", excerpt)
	}

	// Genel forum: abone olan yeni konuyu, konuya yazan yanıtı alır.
	res = s.do("POST", acts, fx.primary, nil, map[string]any{"type": "FORUM", "title": "Soru-cevap"})
	qa := "/api/v1/activities/" + res.body["id"].(string)
	if res := s.do("PUT", "/api/v1/activities/"+news+"/subscription", fx.student, nil, nil); res.status != 409 {
		t.Errorf("duyurulara abonelik: %d", res.status)
	}
	if res := s.do("PUT", qa+"/subscription", fx.dropped, nil, nil); res.status != 404 {
		t.Errorf("bırakan öğrencinin aboneliği: %d", res.status)
	}
	for range 2 { // tekrarı zararsız
		if res := s.do("PUT", qa+"/subscription", fx.student, nil, nil); res.status != 204 {
			t.Fatalf("abonelik: %d %s", res.status, res.raw)
		}
	}
	if res := s.do("GET", qa+"/threads", fx.student, nil, nil); res.body["subscribed"] != true || res.body["can_subscribe"] != true {
		t.Errorf("abonelik durumu: %s", res.raw)
	}
	res = s.do("POST", qa+"/threads", fx.primary, nil, map[string]any{"title": "Proje konuları", "body_html": "<p>Liste ekte.</p>"})
	thread := "/api/v1/threads/" + res.body["id"].(string)
	if got := s.notes(fx.student); got[len(got)-1] != "forum.thread" {
		t.Errorf("yeni konu bildirimi: %v", got)
	}
	if got := s.notes(fx.assistant); slices.Contains(got, "forum.thread") {
		t.Errorf("abone olmayana yeni konu: %v", got)
	}
	if res := s.do("POST", thread+"/posts", fx.student, nil, map[string]any{"body_html": "<p>Kendi konumuzu önerebilir miyiz?</p>"}); res.status != 201 {
		t.Fatalf("yanıt: %d %s", res.status, res.raw)
	}
	if got := s.notes(fx.primary); !slices.Equal(got, []string{"forum.reply"}) {
		t.Errorf("konuyu açanın bildirimleri: %v", got)
	}
	if got := s.notes(fx.assistant); slices.Contains(got, "forum.reply") {
		t.Errorf("konuya yazmamış olana yanıt: %v", got)
	}
	if res := s.do("DELETE", qa+"/subscription", fx.student, nil, nil); res.status != 204 {
		t.Errorf("abonelikten çık: %d", res.status)
	}
	if res := s.do("GET", qa+"/threads", fx.student, nil, nil); res.body["subscribed"] != false || !strings.Contains(res.raw, `"can_subscribe":true`) {
		t.Errorf("abonelikten sonra: %s", res.raw)
	}
}
