package metrics

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

// PoolCollector, pgx bağlantı havuzunun istatistiklerini Prometheus metriği olarak yayınlar.
//
// Sayaçları her istekte güncellemek yerine, Prometheus her veri topladığında (scrape)
// havuzun anlık durumunu okur. Bunun için prometheus.Collector interface'ini
// (Describe + Collect) kendimiz gerçekliyoruz.
type PoolCollector struct {
	pool *pgxpool.Pool

	acquiredConns     *prometheus.Desc
	idleConns         *prometheus.Desc
	totalConns        *prometheus.Desc
	maxConns          *prometheus.Desc
	acquireTotal      *prometheus.Desc
	emptyAcquireTotal *prometheus.Desc
	canceledAcquire   *prometheus.Desc
	acquireSeconds    *prometheus.Desc
	emptyAcquireWait  *prometheus.Desc
}

var _ prometheus.Collector = (*PoolCollector)(nil)

// NewPoolCollector, verilen havuz için bir collector oluşturur. Registry'ye
// reg.MustRegister(metrics.NewPoolCollector(pool)) ile kaydedilir.
func NewPoolCollector(pool *pgxpool.Pool) *PoolCollector {
	desc := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc(prometheus.BuildFQName(namespace, "db_pool", name), help, nil, nil)
	}

	return &PoolCollector{
		pool:              pool,
		acquiredConns:     desc("acquired_conns", "Şu anda kullanımda olan bağlantı sayısı."),
		idleConns:         desc("idle_conns", "Havuzda boşta bekleyen bağlantı sayısı."),
		totalConns:        desc("total_conns", "Havuzdaki toplam bağlantı sayısı."),
		maxConns:          desc("max_conns", "Havuzun izin verdiği en fazla bağlantı sayısı."),
		acquireTotal:      desc("acquire_total", "Havuzdan alınan bağlantıların toplam sayısı."),
		emptyAcquireTotal: desc("empty_acquire_total", "Boş bağlantı olmadığı için beklemek zorunda kalınan alımlar."),
		canceledAcquire:   desc("canceled_acquire_total", "Context iptal edildiği için yarıda kalan alımlar."),
		acquireSeconds:    desc("acquire_duration_seconds_total", "Bağlantı alırken geçen toplam süre."),
		emptyAcquireWait:  desc("empty_acquire_wait_seconds_total", "Boş bağlantı beklerken geçen toplam süre."),
	}
}

// Describe, collector'ın yayınlayacağı metriklerin tanımlarını bildirir.
func (c *PoolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.acquiredConns
	ch <- c.idleConns
	ch <- c.totalConns
	ch <- c.maxConns
	ch <- c.acquireTotal
	ch <- c.emptyAcquireTotal
	ch <- c.canceledAcquire
	ch <- c.acquireSeconds
	ch <- c.emptyAcquireWait
}

// Collect, havuzun anlık istatistiklerini okur ve metrik olarak gönderir.
func (c *PoolCollector) Collect(ch chan<- prometheus.Metric) {
	s := c.pool.Stat()

	gauge := func(d *prometheus.Desc, v float64) {
		ch <- prometheus.MustNewConstMetric(d, prometheus.GaugeValue, v)
	}
	counter := func(d *prometheus.Desc, v float64) {
		ch <- prometheus.MustNewConstMetric(d, prometheus.CounterValue, v)
	}

	gauge(c.acquiredConns, float64(s.AcquiredConns()))
	gauge(c.idleConns, float64(s.IdleConns()))
	gauge(c.totalConns, float64(s.TotalConns()))
	gauge(c.maxConns, float64(s.MaxConns()))
	counter(c.acquireTotal, float64(s.AcquireCount()))
	counter(c.emptyAcquireTotal, float64(s.EmptyAcquireCount()))
	counter(c.canceledAcquire, float64(s.CanceledAcquireCount()))
	counter(c.acquireSeconds, s.AcquireDuration().Seconds())
	counter(c.emptyAcquireWait, s.EmptyAcquireWaitTime().Seconds())
}
