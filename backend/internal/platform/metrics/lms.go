package metrics

import "github.com/prometheus/client_golang/prometheus"

// LMS, ders alanının iş metrikleridir: gönderilen ödev teslimleri (zamanında ya da geç),
// verilen ödev puanları (tek tek ya da dosyayla), forum gönderileri (konu ya da yanıt) ve
// tamamlanan etkinlikler. Teslim tarihlerinden önceki yoğunluk ve forumun canlılığı buradan
// izlenir; yüklenen dosyalar Files'tadır.
//
// lms.Metrics interface'ini örtük olarak sağlar.
type LMS struct {
	submissions *prometheus.CounterVec
	grades      *prometheus.CounterVec
	posts       *prometheus.CounterVec
	completions *prometheus.CounterVec
}

// NewLMS, ders alanı metriklerini oluşturur ve registry'ye kaydeder. Bilinen etiket değerleri
// sıfırla başlar.
func NewLMS(reg prometheus.Registerer) *LMS {
	m := &LMS{
		submissions: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "lms_submissions_total",
			Help: "Gönderilen ödev teslimleri, zamanlamaya göre (on_time, late).",
		}, []string{"timing"}),
		grades: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "lms_grades_total",
			Help: "Verilen (değişen) ödev puanları, kaynağa göre (manual, csv, xlsx).",
		}, []string{"source"}),
		posts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "lms_forum_posts_total",
			Help: "Forum gönderileri, türe göre (thread: yeni konu, reply: yanıt).",
		}, []string{"kind"}),
		completions: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "lms_activity_completions_total",
			Help: "Tamamlanan etkinlikler, etkinlik türüne göre (resource, folder, url, page, assignment, forum).",
		}, []string{"type"}),
	}
	for _, v := range []string{"on_time", "late"} {
		m.submissions.WithLabelValues(v)
	}
	for _, v := range []string{"manual", "csv", "xlsx"} {
		m.grades.WithLabelValues(v)
	}
	for _, v := range []string{"thread", "reply"} {
		m.posts.WithLabelValues(v)
	}
	for _, v := range []string{"resource", "folder", "url", "page", "assignment", "forum"} {
		m.completions.WithLabelValues(v)
	}
	reg.MustRegister(m.submissions, m.grades, m.posts, m.completions)
	return m
}

// Submitted, gönderilen bir teslimi sayar.
func (m *LMS) Submitted(late bool) {
	timing := "on_time"
	if late {
		timing = "late"
	}
	m.submissions.WithLabelValues(timing).Inc()
}

// Graded, verilen n ödev puanını sayar.
func (m *LMS) Graded(source string, n int) {
	m.grades.WithLabelValues(source).Add(float64(n))
}

// Posted, bir forum gönderisini sayar.
func (m *LMS) Posted(kind string) {
	m.posts.WithLabelValues(kind).Inc()
}

// Completed, tamamlanan bir etkinliği sayar.
func (m *LMS) Completed(activityType string) {
	m.completions.WithLabelValues(activityType).Inc()
}
