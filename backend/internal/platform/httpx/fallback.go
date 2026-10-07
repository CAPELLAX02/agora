package httpx

import "net/http"

// WithProblemFallback, ServeMux'un düz metin 404 ve 405 yanıtlarını RFC 9457 problem yanıtlarına çevirir.
func WithProblemFallback(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			h, pattern := mux.Handler(r)
			if pattern != "" {
				mux.ServeHTTP(w, r)
				return
			}

			// Eşleşen route yok. 404 mü 405 mi olduğunu mux'un kendi yanıtını istemciye göndermeden "prova ederek" öğreniyoruz.
			probe := &probeWriter{header: make(http.Header)}
			h.ServeHTTP(probe, r)

			if probe.status == http.StatusMethodNotAllowed {
				w.Header().Set("Allow", probe.header.Get("Allow"))
				MethodNotAllowed(w, r)
				return
			}

			NotFound(w, r)
		})
}

// probeWriter, hiçbir şey göndermeden sadece durum kodunu ve başlıkları kaydeden bir ResponseWriter'dır.
type probeWriter struct {
	header http.Header
	status int
}

var _ http.ResponseWriter = (*probeWriter)(nil)

func (p *probeWriter) Header() http.Header {
	return p.header
}

func (p *probeWriter) Write(b []byte) (int, error) {
	return len(b), nil
}

func (p *probeWriter) WriteHeader(code int) {
	p.status = code
}
