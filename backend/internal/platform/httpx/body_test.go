package httpx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type loginBody struct {
	Username string `json:"username"`
	Remember bool   `json:"remember"`
}

func TestReadJSON(t *testing.T) {
	const jsonType = "application/json"

	tests := []struct {
		name        string
		contentType string
		body        string
		wantStatus  int    // 0: hata beklenmiyor
		wantDetail  string // hata mesajında geçmeli
	}{
		{"geçerli", jsonType, `{"username":"ali","remember":true}`, 0, ""},
		{"charset parametresi kabul edilir", "application/json; charset=utf-8", `{"username":"ali"}`, 0, ""},
		{"sondaki boşluk sorun değil", jsonType, "{\"username\":\"ali\"}\n  ", 0, ""},
		{"Content-Type yok", "", `{"username":"ali"}`, 415, "application/json olmalı"},
		{"form gövdesi", "application/x-www-form-urlencoded", "username=ali", 415, "application/json olmalı"},
		{"text/plain", "text/plain", `{"username":"ali"}`, 415, "application/json olmalı"},
		{"boş gövde", jsonType, "", 400, "boş"},
		{"yarım JSON", jsonType, `{"username":`, 400, "yarım kalmış"},
		{"sözdizimi hatası", jsonType, `{"username" "ali"}`, 400, "Geçersiz JSON (13. bayt)"},
		{"yanlış tür", jsonType, `{"username":42}`, 400, `"username" alanının türü yanlış: string`},
		{"kök değer yanlış tür", jsonType, `["ali"]`, 400, "JSON türü yanlış"},
		{"bilinmeyen alan", jsonType, `{"username":"ali","is_admin":true}`, 400, `Bilinmeyen alan: "is_admin"`},
		{"iki JSON değeri", jsonType, `{"username":"ali"}{"username":"veli"}`, 400, "tek bir JSON değeri"},
		{"çok büyük", jsonType, `{"username":"` + strings.Repeat("a", MaxBodyBytes) + `"}`, 413, "en fazla 1048576 bayt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}

			var dst loginBody
			err := ReadJSON(httptest.NewRecorder(), req, &dst)

			if tt.wantStatus == 0 {
				if err != nil {
					t.Fatalf("beklenmeyen hata: %v", err)
				}
				if dst.Username != "ali" {
					t.Errorf("username = %q", dst.Username)
				}
				return
			}

			var be *BodyError
			if !errors.As(err, &be) {
				t.Fatalf("err = %v, *BodyError bekleniyordu", err)
			}
			if be.Status != tt.wantStatus || !strings.Contains(be.Detail, tt.wantDetail) {
				t.Errorf("hata = %d %q, want %d ve %q içermeli", be.Status, be.Detail, tt.wantStatus, tt.wantDetail)
			}
		})
	}
}

func TestReadJSONPanicsOnNonPointer(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("pointer olmayan hedef panic'e yol açmalıydı: bu programlama hatasıdır")
		}
	}()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	var dst loginBody
	_ = ReadJSON(httptest.NewRecorder(), req, dst)
}

func TestInvalidBody(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"413", &BodyError{Status: 413, Code: "BODY_TOO_LARGE", Detail: "büyük"}, 413, "BODY_TOO_LARGE"},
		{"415", &BodyError{Status: 415, Code: "UNSUPPORTED_MEDIA_TYPE", Detail: "tür"}, 415, "UNSUPPORTED_MEDIA_TYPE"},
		{"BodyError olmayan hata", errors.New("başka"), 400, "INVALID_BODY"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			InvalidBody(rec, httptest.NewRequest(http.MethodPost, "/x", nil), tt.err)

			p := decodeProblem(t, rec)
			if rec.Code != tt.wantStatus || p.Code != tt.wantCode {
				t.Errorf("yanıt = %d %s, want %d %s", rec.Code, p.Code, tt.wantStatus, tt.wantCode)
			}
		})
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		remoteAddr string
		want       string
	}{
		{"203.0.113.7:51234", "203.0.113.7"},
		{"[2001:db8::1]:443", "2001:db8::1"},
		{"[::ffff:192.0.2.1]:8080", "192.0.2.1"}, // IPv6'ya gömülü IPv4 sade haline döner
		{"bozuk", ""},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = tt.remoteAddr
		// X-Forwarded-For'a (henüz) güvenilmez: istemci bunu istediği gibi yazabilir.
		req.Header.Set("X-Forwarded-For", "10.0.0.1")
		if got := ClientIP(req); got != tt.want {
			t.Errorf("ClientIP(%q) = %q, want %q", tt.remoteAddr, got, tt.want)
		}
	}
}
