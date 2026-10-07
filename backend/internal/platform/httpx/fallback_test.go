package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithProblemFallback(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /terms/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("dönem " + r.PathValue("id")))
	})
	h := WithProblemFallback(mux)

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantCode   string // boşsa problem yanıtı beklenmez, gövde wantBody ile karşılaştırılır
		wantAllow  string
		wantBody   string
	}{
		{
			name:       "eşleşen route ve path parametresi",
			method:     http.MethodGet,
			path:       "/terms/42",
			wantStatus: http.StatusOK,
			wantBody:   "dönem 42",
		},
		{
			name:       "olmayan yol",
			method:     http.MethodGet,
			path:       "/yok",
			wantStatus: http.StatusNotFound,
			wantCode:   "NOT_FOUND",
		},
		{
			name:       "yanlış metot",
			method:     http.MethodDelete,
			path:       "/terms/42",
			wantStatus: http.StatusMethodNotAllowed,
			wantCode:   "METHOD_NOT_ALLOWED",
			wantAllow:  "GET, HEAD",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))

			if rec.Code != tt.wantStatus {
				t.Fatalf("durum = %d, want %d", rec.Code, tt.wantStatus)
			}
			if got := rec.Header().Get("Allow"); got != tt.wantAllow {
				t.Errorf("Allow = %q, want %q", got, tt.wantAllow)
			}

			if tt.wantCode == "" {
				if got := rec.Body.String(); got != tt.wantBody {
					t.Errorf("gövde = %q, want %q", got, tt.wantBody)
				}
				return
			}

			if p := decodeProblem(t, rec); p.Code != tt.wantCode {
				t.Errorf("code = %q, want %q", p.Code, tt.wantCode)
			}
		})
	}
}
