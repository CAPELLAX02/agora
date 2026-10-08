package metrics

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
)

func TestPoolCollector(t *testing.T) {
	pool := dbtest.New(t)

	// Havuzdan birkaç kez bağlantı alınsın ki sayaçlar sıfırdan farklı olsun.
	for range 3 {
		if err := pool.Ping(context.Background()); err != nil {
			t.Fatal(err)
		}
	}

	c := NewPoolCollector(pool)
	if n := testutil.CollectAndCount(c); n != 9 {
		t.Errorf("collector %d metrik yayınladı, want 9", n)
	}
	if problems, err := testutil.CollectAndLint(c); err != nil || len(problems) > 0 {
		t.Errorf("metrik adlandırma sorunları: %v %v", problems, err)
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(c)
	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}

	values := map[string]float64{}
	for _, f := range families {
		m := f.GetMetric()[0]
		if g := m.GetGauge(); g != nil {
			values[f.GetName()] = g.GetValue()
		}
		if c := m.GetCounter(); c != nil {
			values[f.GetName()] = c.GetValue()
		}
	}

	if values["agora_db_pool_max_conns"] < 1 {
		t.Errorf("max_conns = %v, en az 1 olmalı", values["agora_db_pool_max_conns"])
	}
	if values["agora_db_pool_acquire_total"] < 3 {
		t.Errorf("acquire_total = %v, en az 3 olmalı", values["agora_db_pool_acquire_total"])
	}
}
