package lms_test

import (
	"testing"
	"time"
)

func timelineKinds(res result) map[string]string {
	out := map[string]string{}
	for _, it := range res.body["items"].([]any) {
		m := it.(map[string]any)
		out[m["title"].(string)] = m["kind"].(string)
	}
	return out
}

func TestTimeline(t *testing.T) {
	s := newServer(t)
	fx := s.fx
	res := s.do("POST", "/api/v1/spaces/"+fx.section+"/topics", fx.primary, nil, map[string]any{"title": "Hafta"})
	acts := "/api/v1/topics/" + topics(res)[1]["id"].(string) + "/activities"
	now := *s.now
	create := func(title string, settings map[string]any, extra map[string]any) string {
		t.Helper()
		body := map[string]any{"type": "ASSIGNMENT", "title": title, "assignment": settings}
		for k, v := range extra {
			body[k] = v
		}
		res := s.do("POST", acts, fx.primary, nil, body)
		if res.status != 201 {
			t.Fatalf("%s: %d %s", title, res.status, res.raw)
		}
		return res.body["id"].(string)
	}
	soon := create("Yakında", map[string]any{"due_at": now.Add(48 * time.Hour).UTC(), "submission_types": []string{"ONLINE_TEXT"}}, nil)
	create("Geç ama açık", map[string]any{"opens_at": now.Add(-96 * time.Hour).UTC(), "due_at": now.Add(-time.Hour).UTC(),
		"submission_types": []string{"ONLINE_TEXT"}}, nil)
	create("Kapandı", map[string]any{"opens_at": now.Add(-96 * time.Hour).UTC(), "due_at": now.Add(-time.Hour).UTC(),
		"late_policy": "DENY", "submission_types": []string{"ONLINE_TEXT"}}, nil)
	create("Gizli", map[string]any{"due_at": now.Add(24 * time.Hour).UTC()}, map[string]any{"is_visible": false})
	create("Çok ileride", map[string]any{"due_at": now.Add(90 * 24 * time.Hour).UTC()}, nil)
	create("Son tarihsiz", map[string]any{}, nil)

	res = s.do("GET", "/api/v1/me/timeline", fx.student, nil, nil)
	items := res.body["items"].([]any)
	if res.status != 200 || len(items) != 2 || items[0].(map[string]any)["title"] != "Geç ama açık" ||
		items[0].(map[string]any)["kind"] != "OVERDUE" || items[1].(map[string]any)["kind"] != "DUE" ||
		items[1].(map[string]any)["course"].(map[string]any)["code"] != "COM1001" {
		t.Fatalf("öğrenci: %d %s", res.status, res.raw)
	}
	if res := s.do("GET", "/api/v1/me/timeline", fx.dropped, nil, nil); len(res.body["items"].([]any)) != 0 {
		t.Errorf("bırakan öğrenci: %s", res.raw)
	}

	// Gönderilen ödev öğrencinin çizelgesinden çıkar, öğretim elemanının çizelgesine girer.
	my := "/api/v1/activities/" + soon + "/my-submission"
	s.do("PUT", my, fx.student, nil, map[string]any{"online_text_html": "<p>Cevap</p>"})
	if res := s.do("POST", my+"/submit", fx.student, map[string]string{"If-Match": `"1"`}, nil); res.status != 200 {
		t.Fatalf("gönder: %d %s", res.status, res.raw)
	}
	if kinds := timelineKinds(s.do("GET", "/api/v1/me/timeline", fx.student, nil, nil)); kinds["Yakında"] != "" {
		t.Errorf("gönderilen ödev: %v", kinds)
	}
	res = s.do("GET", "/api/v1/me/timeline", fx.assistant, nil, nil)
	items = res.body["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["kind"] != "TO_GRADE" || items[0].(map[string]any)["needs_grading"] != 1.0 {
		t.Fatalf("öğretim elemanı: %s", res.raw)
	}
	s.do("PUT", "/api/v1/activities/"+soon+"/submissions/"+fx.student+"/grade", fx.primary, nil, map[string]any{"score": 90})
	if res := s.do("GET", "/api/v1/me/timeline", fx.assistant, nil, nil); len(res.body["items"].([]any)) != 0 {
		t.Errorf("puanlanınca çıkar: %s", res.raw)
	}
}
