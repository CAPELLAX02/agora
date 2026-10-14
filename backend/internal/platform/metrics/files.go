package metrics

import "github.com/prometheus/client_golang/prometheus"

// Files, dosya yüklemelerinin metrikleridir: yüklenen dosya ve bayt sayısı, reddedilen
// yüklemeler (tür, içerik, boyut). Teslim tarihlerinden önceki yük artışı buradan izlenir.
//
// files.Metrics interface'ini örtük olarak sağlar.
type Files struct {
	uploaded prometheus.Counter
	bytes    prometheus.Counter
	rejected *prometheus.CounterVec
}

// NewFiles, dosya metriklerini oluşturur ve registry'ye kaydeder.
func NewFiles(reg prometheus.Registerer) *Files {
	m := &Files{
		uploaded: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace, Name: "files_uploaded_total",
			Help: "Yüklenen dosyalar.",
		}),
		bytes: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace, Name: "file_upload_bytes_total",
			Help: "Yüklenen dosyaların toplam boyutu (bayt).",
		}),
		rejected: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "file_uploads_rejected_total",
			Help: "Reddedilen yüklemeler, nedene göre (FILE_TYPE_NOT_ALLOWED, FILE_CONTENT_MISMATCH, FILE_TOO_LARGE, FILE_EMPTY).",
		}, []string{"reason"}),
	}
	reg.MustRegister(m.uploaded, m.bytes, m.rejected)
	return m
}

// Uploaded, yüklenen bir dosyayı sayar.
func (m *Files) Uploaded(bytes int64) {
	m.uploaded.Inc()
	m.bytes.Add(float64(bytes))
}

// Rejected, reddedilen bir yüklemeyi sayar.
func (m *Files) Rejected(code string) { m.rejected.WithLabelValues(code).Inc() }
