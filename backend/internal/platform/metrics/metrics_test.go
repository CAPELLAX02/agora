package metrics

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestHTTP(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewHTTP(reg)

	m.RequestStarted()
	m.RequestStarted()
	if got := testutil.ToFloat64(m.inFlight); got != 2 {
		t.Errorf("in_flight = %v, want 2", got)
	}

	m.RequestFinished("GET", "/api/v1/programs", 200, 30*time.Millisecond)
	m.RequestFinished("BREW", "/api/v1/programs", 405, time.Millisecond)

	if got := testutil.ToFloat64(m.inFlight); got != 0 {
		t.Errorf("in_flight = %v, want 0", got)
	}
	if got := testutil.ToFloat64(m.requests.WithLabelValues("GET", "/api/v1/programs", "200")); got != 1 {
		t.Errorf("GET 200 sayacı = %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.requests.WithLabelValues("OTHER", "/api/v1/programs", "405")); got != 1 {
		t.Errorf("standart dışı metot OTHER etiketine düşmeliydi, sayaç = %v", got)
	}
	if n := testutil.CollectAndCount(m.duration); n != 2 {
		t.Errorf("süre histogramında %d seri var, want 2 (GET ve OTHER)", n)
	}
}

func TestNewRegistryIncludesRuntimeMetrics(t *testing.T) {
	families, err := NewRegistry().Gather()
	if err != nil {
		t.Fatal(err)
	}

	found := map[string]bool{}
	for _, f := range families {
		found[f.GetName()] = true
	}
	for _, name := range []string{"go_goroutines", "process_resident_memory_bytes"} {
		if !found[name] {
			t.Errorf("%s metriği registry'de yok", name)
		}
	}
}

func TestGrading(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewGrading(reg)

	// Bilinen etiketler olaydan önce sıfırla görünür.
	if n := testutil.CollectAndCount(m.changes); n != 4 {
		t.Errorf("düzeltme serileri = %d, want 4", n)
	}
	m.ScoresEntered("csv", 25)
	m.ScoresEntered("manual", 3)
	m.ScoresEntered("csv", 5)
	m.GradesPublished("final", 40)
	m.GradeChange("applied")
	if got := testutil.ToFloat64(m.scores.WithLabelValues("csv")); got != 30 {
		t.Errorf("csv notları = %v, want 30", got)
	}
	if got := testutil.ToFloat64(m.published.WithLabelValues("makeup")); got != 0 {
		t.Errorf("bütünleme = %v, want 0", got)
	}
	if got := testutil.ToFloat64(m.changes.WithLabelValues("applied")); got != 1 {
		t.Errorf("uygulanan düzeltme = %v, want 1", got)
	}
}

func TestLMS(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewLMS(reg)

	if n := testutil.CollectAndCount(m.completions); n != 6 {
		t.Errorf("tamamlama serileri = %d, want 6", n)
	}
	m.Submitted(false)
	m.Submitted(true)
	m.Submitted(true)
	m.Graded("xlsx", 18)
	m.Graded("manual", 1)
	m.Posted("reply")
	m.Completed("page")
	if got := testutil.ToFloat64(m.submissions.WithLabelValues("late")); got != 2 {
		t.Errorf("geç teslim = %v, want 2", got)
	}
	if got := testutil.ToFloat64(m.grades.WithLabelValues("xlsx")); got != 18 {
		t.Errorf("excel puanları = %v, want 18", got)
	}
	if got := testutil.ToFloat64(m.posts.WithLabelValues("thread")); got != 0 {
		t.Errorf("konu = %v, want 0", got)
	}
	if got := testutil.ToFloat64(m.completions.WithLabelValues("page")); got != 1 {
		t.Errorf("sayfa tamamlama = %v, want 1", got)
	}
}

func TestNotificationStreams(t *testing.T) {
	reg := prometheus.NewRegistry()
	open := 3
	RegisterNotificationStreams(reg, func() int { return open })
	want := `
# HELP agora_notification_streams Açık canlı bildirim akışları (Server-Sent Events).
# TYPE agora_notification_streams gauge
agora_notification_streams 3
`
	if err := testutil.GatherAndCompare(reg, strings.NewReader(want), "agora_notification_streams"); err != nil {
		t.Error(err)
	}
}

func TestStreamFinished(t *testing.T) {
	m := NewHTTP(prometheus.NewRegistry())
	m.RequestStarted()
	m.StreamFinished("GET", "/api/v1/me/notifications/stream", 200)
	if got := testutil.ToFloat64(m.inFlight); got != 0 {
		t.Errorf("in-flight = %v", got)
	}
	if n := testutil.CollectAndCount(m.duration); n != 0 {
		t.Errorf("akış süre dağılımına girmemeli: %d seri", n)
	}
}
