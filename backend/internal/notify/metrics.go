package notify

import (
	"slices"
	"sync/atomic"
)

// Metrics, bildirim sayaçlarıdır: türe ve kanala (in_app, email) göre bildirilen kişi sayısı.
// Bildirim iş verisiyle aynı transaction'da yazıldığından, transaction sonradan geri alınırsa da
// sayılmış olur (nadir).
type Metrics interface {
	Delivered(typ, channel string, n int)
}

var metrics atomic.Pointer[Metrics]

// SetMetrics, Send'in sayaçlarını bağlar (süreç başına bir kez, açılışta). Verilmezse (ya da nil)
// bildirimler sayılmaz.
func SetMetrics(m Metrics) {
	if m == nil {
		metrics.Store(nil)
		return
	}
	metrics.Store(&m)
}

func countDelivered(typ Type, channel string, n int) {
	if m := metrics.Load(); m != nil && n > 0 {
		(*m).Delivered(string(typ), channel, n)
	}
}

// Types, koddaki bildirim türleridir (sıralı); sayaçlar bunlarla sıfırdan başlar.
func Types() []string {
	out := make([]string, 0, len(texts))
	for t := range texts {
		out = append(out, string(t))
	}
	slices.Sort(out)
	return out
}
