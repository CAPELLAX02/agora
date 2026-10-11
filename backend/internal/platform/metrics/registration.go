package metrics

import "github.com/prometheus/client_golang/prometheus"

// Registration, ders seçmenin iş metrikleridir: açılış anında sepete eklenen ders hızı,
// danışmana gönderilen sepetler ve kontenjan dolu yüzünden reddedilen eklemeler.
//
// registration.Metrics interface'ini örtük olarak sağlar.
type Registration struct {
	added     prometheus.Counter
	submitted prometheus.Counter
	full      prometheus.Counter
}

// NewRegistration, ders seçme metriklerini oluşturur ve registry'ye kaydeder.
func NewRegistration(reg prometheus.Registerer) *Registration {
	m := &Registration{
		added: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace, Name: "registration_items_added_total",
			Help: "Sepete eklenen dersler (kural ve kontenjan denetiminden geçenler).",
		}),
		submitted: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace, Name: "registration_submitted_total",
			Help: "Danışman onayına gönderilen sepetler.",
		}),
		full: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace, Name: "section_full_rejections_total",
			Help: "Şube kontenjanı dolu olduğu için reddedilen eklemeler.",
		}),
	}
	reg.MustRegister(m.added, m.submitted, m.full)
	return m
}

// ItemAdded, sepete eklenen bir dersi sayar.
func (m *Registration) ItemAdded() { m.added.Inc() }

// Submitted, danışmana gönderilen bir sepeti sayar.
func (m *Registration) Submitted() { m.submitted.Inc() }

// SectionFull, kontenjan dolu yüzünden reddedilen bir eklemeyi sayar.
func (m *Registration) SectionFull() { m.full.Inc() }
