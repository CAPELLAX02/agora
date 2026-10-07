package httpx

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func TestValidRequestID(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"deneme-123", true},
		{"7S5FO7DPSRZDMXM2VMC7A64MR6", true},
		{"a_b-C", true},
		{strings.Repeat("a", 64), true},
		{"", false},
		{strings.Repeat("a", 65), false},
		{"satır\nsonu", false},
		{"boşluk var", false},
		{"ğüş", false},
	}

	for _, tt := range tests {
		if got := validRequestID(tt.id); got != tt.want {
			t.Errorf("validRequestID(%q) = %v, want %v", tt.id, got, tt.want)
		}
	}
}

func TestRequestID(t *testing.T) {
	tests := []struct {
		name     string
		incoming string
		wantSame bool
	}{
		{name: "geçerli kimlik korunur", incoming: "deneme-123", wantSame: true},
		{name: "başlık yoksa yenisi üretilir", incoming: "", wantSame: false},
		{name: "geçersiz kimlik değiştirilir", incoming: "kötü\nkimlik", wantSame: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var seen string
			h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen = RequestIDFrom(r.Context())
			}))

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.incoming != "" {
				req.Header.Set(RequestIDHeader, tt.incoming)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			got := rec.Header().Get(RequestIDHeader)
			if got == "" {
				t.Fatal("yanıtta X-Request-Id başlığı yok")
			}
			if got != seen {
				t.Errorf("handler'ın gördüğü kimlik %q, yanıt başlığı %q, aynı olmalıydı", seen, got)
			}
			if (got == tt.incoming) != tt.wantSame {
				t.Errorf("kimlik = %q, gelen = %q, korunması bekleniyor muydu: %v", got, tt.incoming, tt.wantSame)
			}
			if !validRequestID(got) {
				t.Errorf("yanıttaki kimlik geçersiz: %q", got)
			}
		})
	}
}

func TestChainOrder(t *testing.T) {
	var calls []string

	mark := func(name string) Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls = append(calls, name+":giriş")
				next.ServeHTTP(w, r)
				calls = append(calls, name+":çıkış")
			})
		}
	}
	final := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, "handler")
	})

	h := Chain(final, mark("a"), mark("b"))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	want := []string{"a:giriş", "b:giriş", "handler", "b:çıkış", "a:çıkış"}
	if !slices.Equal(calls, want) {
		t.Errorf("çağrı sırası = %v, want %v", calls, want)
	}
}

func TestRecover(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))

	h := Recover(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("bilerek patlattık")
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/patla", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("durum = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if got := rec.Header().Get("Connection"); got != "close" {
		t.Errorf("Connection = %q, want %q", got, "close")
	}
	if p := decodeProblem(t, rec); p.Code != "INTERNAL_ERROR" {
		t.Errorf("code = %q, want %q", p.Code, "INTERNAL_ERROR")
	}
	if !strings.Contains(logs.String(), "panic yakalandı") {
		t.Errorf("panic loglanmadı, log çıktısı:\n%s", logs.String())
	}
}

func TestAccessLog(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    []string // log satırında geçmesi gereken parçalar
	}{
		{
			name: "4xx yanıt WARN seviyesinde loglanır",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
			want: []string{"level=WARN", "status=404", "method=GET", "path=/kayit"},
		},
		{
			name: "5xx yanıt ERROR seviyesinde loglanır",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			},
			want: []string{"level=ERROR", "status=503"},
		},
		{
			name:    "hiçbir şey yazmayan handler 200 sayılır",
			handler: func(w http.ResponseWriter, r *http.Request) {},
			want:    []string{"level=INFO", "status=200", "bytes=0"},
		},
		{
			name: "yazılan byte sayısı kaydedilir",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("merhaba"))
			},
			want: []string{"level=INFO", "status=200", "bytes=7"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&logs, nil))

			h := AccessLog(logger)(tt.handler)
			h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/kayit", nil))

			out := logs.String()
			for _, part := range tt.want {
				if !strings.Contains(out, part) {
					t.Errorf("log satırında %q yok:\n%s", part, out)
				}
			}
		})
	}
}
