package lms_test

import (
	"sync"
	"testing"
	"time"
)

// recorder, sayılan olayları kaydeder.
type recorder struct {
	mu     sync.Mutex
	events []string
}

func (r *recorder) add(e string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

func (r *recorder) Submitted(late bool) {
	if late {
		r.add("submitted:late")
	} else {
		r.add("submitted:on_time")
	}
}
func (r *recorder) Graded(source string, n int) { r.add("graded:" + source + ":" + itoa(n)) }
func (r *recorder) Posted(kind string)          { r.add("posted:" + kind) }
func (r *recorder) Completed(t string)          { r.add("completed:" + t) }

func TestMetricsEvents(t *testing.T) {
	s := newServer(t)
	rec := &recorder{}
	s.svc.SetMetrics(rec)
	fx := s.fx
	res := s.do("POST", "/api/v1/spaces/"+fx.section+"/topics", fx.primary, nil, map[string]any{"title": "Hafta 1"})
	news := activities(topics(res)[0])[0]["id"].(string)
	acts := "/api/v1/topics/" + topics(res)[1]["id"].(string) + "/activities"
	res = s.do("POST", acts, fx.primary, nil, map[string]any{"type": "ASSIGNMENT", "title": "Ödev", "assignment": map[string]any{
		"due_at": s.now.Add(48 * time.Hour).UTC(), "submission_types": []string{"ONLINE_TEXT"}}})
	assignment := res.body["id"].(string)
	my := "/api/v1/activities/" + assignment + "/my-submission"
	s.do("PUT", my, fx.student, nil, map[string]any{"online_text_html": "<p>Cevap</p>"})
	s.do("POST", my+"/submit", fx.student, map[string]string{"If-Match": `"1"`}, nil)
	s.do("PUT", "/api/v1/activities/"+assignment+"/submissions/"+fx.student+"/grade", fx.primary, nil, map[string]any{"score": 80})
	res = s.do("POST", "/api/v1/activities/"+news+"/threads", fx.primary, nil, map[string]any{"title": "Duyuru", "body_html": "<p>x</p>"})
	s.do("POST", "/api/v1/threads/"+res.body["id"].(string)+"/posts", fx.assistant, nil, map[string]any{"body_html": "<p>y</p>"})
	s.do("GET", "/api/v1/activities/"+news, fx.student, nil, nil)
	s.do("GET", "/api/v1/activities/"+news, fx.student, nil, nil) // ikinci görüntüleme sayılmaz

	want := []string{"submitted:on_time", "completed:assignment", "graded:manual:1", "posted:thread", "posted:reply", "completed:forum"}
	if len(rec.events) != len(want) {
		t.Fatalf("olaylar: %v", rec.events)
	}
	for i := range want {
		if rec.events[i] != want[i] {
			t.Errorf("olay %d: %s, beklenen %s (%v)", i, rec.events[i], want[i], rec.events)
		}
	}
}
