// Package useragent, User-Agent başlığından oturum listesinde gösterilecek kaba cihaz
// bilgisini (tarayıcı, işletim sistemi, cihaz türü) çıkarır.
//
// Amaç kullanıcının "bu oturum benim mi?" sorusunu cevaplayabilmesidir, istatistik
// değil. Bu yüzden sadece yaygın tarayıcı ve sistemler tanınır, gerisi "Bilinmiyor"
// olur. User-Agent istemcinin beyanıdır: güvenlik kararı için kullanılmamalı.
package useragent

import (
	"strings"
)

// Cihaz türleri.
const (
	Desktop = "DESKTOP"
	Mobile  = "MOBILE"
	Tablet  = "TABLET"
	Unknown = "UNKNOWN"
)

// Info, ayrıştırılmış cihaz bilgisidir.
type Info struct {
	Browser string // ör. "Chrome 140", yoksa boş
	OS      string // ör. "macOS", yoksa boş
	Device  string // Desktop, Mobile, Tablet ya da Unknown
}

// browsers, sırası önemli olan tarayıcı imzalarıdır: Edge ve Opera, User-Agent'ta
// "Chrome/" da yazar, Chrome da "Safari/" yazar. Önce daha özel olan aranır.
var browsers = []struct{ token, name string }{
	{"Edg/", "Edge"},
	{"EdgA/", "Edge"},
	{"OPR/", "Opera"},
	{"SamsungBrowser/", "Samsung Internet"},
	{"Firefox/", "Firefox"},
	{"FxiOS/", "Firefox"},
	{"CriOS/", "Chrome"},
	{"Chrome/", "Chrome"},
	{"Version/", "Safari"}, // Safari sürümünü "Version/" ile yazar
}

// Parse, User-Agent'ı ayrıştırır.
func Parse(ua string) Info {
	if strings.TrimSpace(ua) == "" {
		return Info{Device: Unknown}
	}

	info := Info{OS: parseOS(ua), Device: Desktop}
	for _, b := range browsers {
		if v, ok := version(ua, b.token); ok {
			if b.name == "Safari" && !strings.Contains(ua, "Safari/") {
				continue
			}
			info.Browser = strings.TrimSpace(b.name + " " + v)
			break
		}
	}

	switch {
	case strings.Contains(ua, "iPad") || strings.Contains(ua, "Tablet") ||
		(strings.Contains(ua, "Android") && !strings.Contains(ua, "Mobile")):
		info.Device = Tablet
	case strings.Contains(ua, "Mobi") || strings.Contains(ua, "iPhone"):
		info.Device = Mobile
	case info.OS == "" && info.Browser == "":
		info.Device = Unknown
	}
	return info
}

func parseOS(ua string) string {
	switch {
	case strings.Contains(ua, "Windows NT"):
		return "Windows"
	case strings.Contains(ua, "iPhone") || strings.Contains(ua, "iPod"):
		return "iOS"
	case strings.Contains(ua, "iPad"):
		return "iPadOS"
	case strings.Contains(ua, "Android"):
		return "Android"
	case strings.Contains(ua, "CrOS"):
		return "ChromeOS"
	case strings.Contains(ua, "Mac OS X") || strings.Contains(ua, "Macintosh"):
		return "macOS"
	case strings.Contains(ua, "Linux"):
		return "Linux"
	}
	return ""
}

// version, token'dan sonra gelen ana sürüm numarasını döndürür: "Chrome/140.0.1" → "140".
func version(ua, token string) (string, bool) {
	_, rest, ok := strings.Cut(ua, token)
	if !ok {
		return "", false
	}
	end := strings.IndexFunc(rest, func(r rune) bool { return r < '0' || r > '9' })
	if end == -1 {
		end = len(rest)
	}
	return rest[:end], true
}
