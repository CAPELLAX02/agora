package metrics

import "github.com/prometheus/client_golang/prometheus"

// Grading, değerlendirme ve notun iş metrikleridir: girilen bileşen notları (elle ya da CSV
// ile), ilan edilen harf notları (ilan ve bütünleme) ve not düzeltme talepleri. Not girişi
// pencerelerinin son günlerindeki yoğunluk ve ilandan sonraki düzeltme oranı buradan izlenir.
//
// grading.Metrics interface'ini örtük olarak sağlar.
type Grading struct {
	scores    *prometheus.CounterVec
	published *prometheus.CounterVec
	changes   *prometheus.CounterVec
}

// NewGrading, not metriklerini oluşturur ve registry'ye kaydeder. Bilinen etiket değerleri
// sıfırla başlar: panolar ilk olaydan önce de seriyi görür.
func NewGrading(reg prometheus.Registerer) *Grading {
	m := &Grading{
		scores: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "scores_entered_total",
			Help: "Yazılan (değişen) bileşen notları, kaynağa göre (manual, csv).",
		}, []string{"source"}),
		published: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "grades_published_total",
			Help: "İlan edilen harf notları, aşamaya göre (final: harf notu ilanı, makeup: bütünlemeyle değişen).",
		}, []string{"phase"}),
		changes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "grade_changes_total",
			Help: "Not düzeltme talepleri, sonuca göre (requested, applied, rejected, withdrawn).",
		}, []string{"outcome"}),
	}
	for _, v := range []string{"manual", "csv"} {
		m.scores.WithLabelValues(v)
	}
	for _, v := range []string{"final", "makeup"} {
		m.published.WithLabelValues(v)
	}
	for _, v := range []string{"requested", "applied", "rejected", "withdrawn"} {
		m.changes.WithLabelValues(v)
	}
	reg.MustRegister(m.scores, m.published, m.changes)
	return m
}

// ScoresEntered, yazılan n bileşen notunu sayar.
func (m *Grading) ScoresEntered(source string, n int) {
	m.scores.WithLabelValues(source).Add(float64(n))
}

// GradesPublished, ilan edilen n harf notunu sayar.
func (m *Grading) GradesPublished(phase string, n int) {
	m.published.WithLabelValues(phase).Add(float64(n))
}

// GradeChange, bir not düzeltme talebi olayını sayar.
func (m *Grading) GradeChange(outcome string) {
	m.changes.WithLabelValues(outcome).Inc()
}
