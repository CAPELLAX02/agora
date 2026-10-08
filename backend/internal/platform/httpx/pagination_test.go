package httpx

import (
	"net/url"
	"testing"
)

func TestParseLimit(t *testing.T) {
	tests := []struct {
		raw     string
		want    int
		wantErr bool
	}{
		{raw: "", want: DefaultPageLimit},
		{raw: "1", want: 1},
		{raw: "100", want: MaxPageLimit},
		{raw: "0", wantErr: true},
		{raw: "101", wantErr: true},
		{raw: "-5", wantErr: true},
		{raw: "on", wantErr: true},
	}

	for _, tt := range tests {
		q := url.Values{}
		if tt.raw != "" {
			q.Set("limit", tt.raw)
		}

		got, fieldErr := ParseLimit(q)
		if tt.wantErr {
			if fieldErr == nil || fieldErr.Field != "limit" {
				t.Errorf("ParseLimit(%q) hata = %v, limit alan hatası bekleniyordu", tt.raw, fieldErr)
			}
			continue
		}
		if fieldErr != nil || got != tt.want {
			t.Errorf("ParseLimit(%q) = %d, %v; want %d, nil", tt.raw, got, fieldErr, tt.want)
		}
	}
}

func TestCursorSurvivesURL(t *testing.T) {
	type position struct {
		Name string `json:"n"`
		ID   string `json:"i"`
	}

	// "Ağ Ağ ~" standart base64 alfabesinde '+' üretir. '+' sorgu parametresinde
	// boşluğa dönüştüğü için cursor URL-güvenli kodlanmazsa bu test kırılır.
	for _, in := range []position{
		{Name: "Ağ Ağ ~", ID: "01a11b2d-57fe-7520-8e98-b6dcc256425b"},
		{Name: "İstatistik & Ölçme", ID: "01a11b26-ea81-7ab4-8191-ab2f7c094fa3"},
	} {
		encoded, err := EncodeCursor(in)
		if err != nil {
			t.Fatal(err)
		}

		// İstemci cursor'ı olduğu gibi URL'ye koyar, sunucu sorgu parametresinden okur.
		q, err := url.ParseQuery("cursor=" + encoded)
		if err != nil {
			t.Fatal(err)
		}

		out, err := DecodeCursor[position](q.Get("cursor"))
		if err != nil {
			t.Fatalf("%q: URL'den geçen cursor çözülemedi: %v (cursor=%s)", in.Name, err, encoded)
		}
		if out != in {
			t.Errorf("DecodeCursor = %+v, want %+v", out, in)
		}
	}
}

func TestDecodeCursorRejectsGarbage(t *testing.T) {
	type position struct {
		Name string `json:"n"`
	}
	for _, bad := range []string{"%%%", "bm90LWpzb24"} { // ikincisi: base64("not-json")
		if _, err := DecodeCursor[position](bad); err == nil {
			t.Errorf("DecodeCursor(%q) hata döndürmedi", bad)
		}
	}
}
