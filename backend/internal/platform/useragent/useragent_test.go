package useragent

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		name string
		ua   string
		want Info
	}{
		{"Chrome macOS",
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36",
			Info{"Chrome 140", "macOS", Desktop}},
		{"Edge Windows",
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36 Edg/140.0.0.0",
			Info{"Edge 140", "Windows", Desktop}},
		{"Firefox Linux",
			"Mozilla/5.0 (X11; Linux x86_64; rv:142.0) Gecko/20100101 Firefox/142.0",
			Info{"Firefox 142", "Linux", Desktop}},
		{"Safari iPhone",
			"Mozilla/5.0 (iPhone; CPU iPhone OS 26_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/26.0 Mobile/15E148 Safari/604.1",
			Info{"Safari 26", "iOS", Mobile}},
		{"Chrome Android telefon",
			"Mozilla/5.0 (Linux; Android 15; Pixel 9) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Mobile Safari/537.36",
			Info{"Chrome 140", "Android", Mobile}},
		{"Android tablet",
			"Mozilla/5.0 (Linux; Android 14; SM-X710) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36",
			Info{"Chrome 139", "Android", Tablet}},
		{"Opera", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/139.0.0.0 Safari/537.36 OPR/124.0.0.0",
			Info{"Opera 124", "Windows", Desktop}},
		{"curl", "curl/8.7.1", Info{"", "", Unknown}},
		{"boş", "", Info{Device: Unknown}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Parse(tt.ua); got != tt.want {
				t.Errorf("Parse = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func FuzzParse(f *testing.F) {
	f.Add("Mozilla/5.0 (Macintosh) Chrome/140.0 Safari/537.36")
	f.Add("Chrome/")
	f.Fuzz(func(t *testing.T, ua string) {
		_ = Parse(ua) // panic olmamalı
	})
}
