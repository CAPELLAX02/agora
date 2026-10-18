package metrics

import "github.com/prometheus/client_golang/prometheus"

// RegisterNotificationStreams, açık canlı bildirim akışı (SSE) sayısını gösteren göstergeyi
// kaydeder; streams her kazımada (scrape) okunur. Akışlar açık kaldıkça
// agora_http_requests_in_flight'ta da görünür.
func RegisterNotificationStreams(reg prometheus.Registerer, streams func() int) {
	reg.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Namespace: namespace, Name: "notification_streams",
		Help: "Açık canlı bildirim akışları (Server-Sent Events).",
	}, func() float64 { return float64(streams()) }))
}
