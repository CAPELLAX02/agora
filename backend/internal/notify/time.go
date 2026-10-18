package notify

import (
	"fmt"
	"time"
)

// turkey, Türkiye saatidir: 2016'dan beri yaz saati uygulanmıyor, sabit UTC+3.
var turkey = time.FixedZone("TRT", 3*60*60)

var monthsTR = [...]string{"Ocak", "Şubat", "Mart", "Nisan", "Mayıs", "Haziran", "Temmuz", "Ağustos", "Eylül", "Ekim", "Kasım", "Aralık"}

// FormatTime, bildirim metinleri için Türkiye saatiyle iki dilde tarih ve saat yazar:
// "21 Ekim 2026 23:59" ve "21 October 2026, 23:59".
func FormatTime(t time.Time) (tr, en string) {
	t = t.In(turkey)
	tr = fmt.Sprintf("%d %s %d %s", t.Day(), monthsTR[t.Month()-1], t.Year(), t.Format("15:04"))
	en = t.Format("2 January 2006, 15:04")
	return tr, en
}
