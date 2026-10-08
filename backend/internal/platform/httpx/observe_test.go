package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// recordingObserver, Observe middleware'inin bildirdiklerini kaydeden sahte observer'dır.
type recordingObserver struct {
	started  int
	method   string
	route    string
	status   int
	duration time.Duration
}

func (o *recordingObserver) RequestStarted() { o.started++ }

func (o *recordingObserver) RequestFinished(method, route string, status int, d time.Duration) {
	o.method, o.route, o.status, o.duration = method, route, status, d
}

func TestObserve(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/programs/{id}", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Millisecond)
		w.WriteHeader(http.StatusTeapot)
	})

	tests := []struct {
		name       string
		method     string
		path       string
		wantRoute  string
		wantStatus int
	}{
		{
			name:       "eşleşen istekte route etiketi gerçek yol değil, desendir",
			method:     http.MethodGet,
			path:       "/api/v1/programs/01a11b2d-57fe-7520-8e98-b6dcc256425b",
			wantRoute:  "/api/v1/programs/{id}",
			wantStatus: http.StatusTeapot,
		},
		{
			name:       "eşleşmeyen istek tek bir etikette toplanır",
			method:     http.MethodGet,
			path:       "/olmayan/01a11b2d",
			wantRoute:  "unmatched",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "yanlış metot da eşleşmemiş sayılır",
			method:     http.MethodDelete,
			path:       "/api/v1/programs/x",
			wantRoute:  "unmatched",
			wantStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obs := &recordingObserver{}
			// Gerçek zincirdeki gibi: RequestID isteği kopyalar, Observe ondan sonra gelir.
			h := Chain(WithProblemFallback(mux), RequestID, Observe(obs))
			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(tt.method, tt.path, nil))

			if obs.started != 1 {
				t.Errorf("RequestStarted %d kez çağrıldı, want 1", obs.started)
			}
			if obs.route != tt.wantRoute {
				t.Errorf("route = %q, want %q", obs.route, tt.wantRoute)
			}
			if obs.status != tt.wantStatus {
				t.Errorf("status = %d, want %d", obs.status, tt.wantStatus)
			}
			if obs.method != tt.method {
				t.Errorf("method = %q, want %q", obs.method, tt.method)
			}
			if tt.wantStatus == http.StatusTeapot && obs.duration < 2*time.Millisecond {
				t.Errorf("süre = %v, en az 2ms olmalı", obs.duration)
			}
		})
	}
}
