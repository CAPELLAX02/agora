package httpx

import (
	"net/http"
	"strconv"
	"strings"
)

// ETag, bir kaydın sürümünü HTTP ETag başlığı biçimine çevirir: "3".
func ETag(version int) string {
	return `"` + strconv.Itoa(version) + `"`
}

// IfMatchVersion, If-Match başlığındaki sürümü okur. Başlık yoksa ya da bir sürüm
// içermiyorsa ok false döner.
//
// İyimser kilit (optimistic locking): istemci kaydı okurken aldığı ETag'i
// güncellemede geri gönderir. Kayıt bu arada başkası tarafından değiştirildiyse
// sürüm tutmaz ve güncelleme reddedilir: kimsenin değişikliği sessizce ezilmez.
func IfMatchVersion(r *http.Request) (version int, ok bool) {
	v := strings.TrimSpace(r.Header.Get("If-Match"))
	v = strings.TrimPrefix(v, "W/") // zayıf ETag da kabul edilir: sürüm numarası aynıdır
	if len(v) < 3 || v[0] != '"' || v[len(v)-1] != '"' {
		return 0, false
	}
	n, err := strconv.Atoi(v[1 : len(v)-1])
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}

// PreconditionRequired, If-Match başlığı olmadan gelen güncellemeyi reddeder (428).
func PreconditionRequired(w http.ResponseWriter, r *http.Request) {
	_ = WriteProblem(w, r, Problem{
		Status: http.StatusPreconditionRequired,
		Code:   "PRECONDITION_REQUIRED",
		Detail: "Güncelleme için kaydın ETag değeri If-Match başlığında gönderilmeli.",
	})
}

// PreconditionFailed, kaydın bu arada değiştiğini bildirir (412).
func PreconditionFailed(w http.ResponseWriter, r *http.Request) {
	_ = WriteProblem(w, r, Problem{
		Status: http.StatusPreconditionFailed,
		Code:   "VERSION_MISMATCH",
		Detail: "Kayıt siz düzenlerken başka biri tarafından değiştirildi. Güncel hali yükleyip tekrar deneyin.",
	})
}
