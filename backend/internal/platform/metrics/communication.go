package metrics

import "github.com/prometheus/client_golang/prometheus"

// Communication, iletişimin iş metrikleridir: türe ve kanala göre bildirilen kişiler (hatırlatmalar
// da bildirim türüdür), konuşma türüne göre gönderilen iletiler, yayımlanan duyurular ve duyuruların
// ilk okunuşları. Açık canlı akışlar agora_notification_streams göstergesindedir.
//
// notify.Metrics, messaging.Metrics ve announce.Metrics interface'lerini örtük olarak sağlar.
// Bildirimler hem API'de hem worker'da (ileri tarihli duyurular, hatırlatmalar) gönderilir; iki
// sürecin sayaçları toplanarak okunur.
type Communication struct {
	notifications *prometheus.CounterVec
	messages      *prometheus.CounterVec
	published     prometheus.Counter
	reads         prometheus.Counter
}

// NewCommunication, iletişim metriklerini oluşturur ve registry'ye kaydeder. Bilinen bildirim
// türleri (types) ve konuşma türleri sıfırla başlar.
func NewCommunication(reg prometheus.Registerer, types []string) *Communication {
	m := &Communication{
		notifications: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "notifications_total",
			Help: "Bildirilen kişiler, bildirim türüne ve kanala göre (in_app, email).",
		}, []string{"type", "channel"}),
		messages: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "messages_total",
			Help: "Gönderilen iletiler, konuşma türüne göre (direct, group, section).",
		}, []string{"kind"}),
		published: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace, Name: "announcements_published_total",
			Help: "Yayımlanan duyurular (ileri tarihliler yayımlandıkları anda).",
		}),
		reads: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace, Name: "announcement_reads_total",
			Help: "Duyuruların ilk okunuşları (kişi başına bir kez).",
		}),
	}
	for _, t := range types {
		for _, c := range []string{"in_app", "email"} {
			m.notifications.WithLabelValues(t, c)
		}
	}
	for _, v := range []string{"direct", "group", "section"} {
		m.messages.WithLabelValues(v)
	}
	reg.MustRegister(m.notifications, m.messages, m.published, m.reads)
	return m
}

// Delivered, bir kanaldan bildirilen n kişiyi sayar.
func (m *Communication) Delivered(typ, channel string, n int) {
	m.notifications.WithLabelValues(typ, channel).Add(float64(n))
}

// MessageSent, gönderilen bir iletiyi sayar.
func (m *Communication) MessageSent(kind string) {
	m.messages.WithLabelValues(kind).Inc()
}

// AnnouncementPublished, yayımlanan bir duyuruyu sayar.
func (m *Communication) AnnouncementPublished() { m.published.Inc() }

// AnnouncementRead, bir duyurunun ilk okunuşunu sayar.
func (m *Communication) AnnouncementRead() { m.reads.Inc() }
