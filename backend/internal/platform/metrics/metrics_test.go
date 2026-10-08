package metrics

import (
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
