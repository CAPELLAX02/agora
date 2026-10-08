package httpx

import (
	"context"
	"net/http"
	"net/netip"
	"regexp"
	"strings"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// ValidUUID, s'nin kanonik biçimde (8-4-4-4-12) bir UUID olup olmadığını söyler.
// Path parametrelerini veritabanına göndermeden önce doğrulamak için kullanılır.
func ValidUUID(s string) bool {
	return uuidPattern.MatchString(s)
}

// ClientIP, isteği yapan istemcinin IP adresidir. Şimdilik doğrudan bağlantının
// adresini kullanıyoruz. API bir ters vekilin (reverse proxy) arkasına geçtiğinde
// X-Forwarded-For sadece güvenilen vekillerden gelirse okunacak: o başlığa
// koşulsuz güvenmek, istemcinin IP'sini istediği gibi yazabilmesi demektir.
func ClientIP(r *http.Request) string {
	ap, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		return ""
	}
	return ap.Addr().Unmap().String()
}

// maxUserAgentLength, saklanan User-Agent'ın en fazla bayt uzunluğudur.
const maxUserAgentLength = 512

// ClientInfo, isteği yapan istemcinin ağ ve tarayıcı bilgisidir.
type ClientInfo struct {
	IP        string
	UserAgent string // en fazla 512 bayt
}

type clientInfoKey struct{}

// WithClientInfo, istemci bilgisini context'e koyar. Böylece servis katmanı (ör.
// denetim kayıtları) http.Request'e bağımlı olmadan bu bilgilere ulaşır.
func WithClientInfo(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info := ClientInfo{IP: ClientIP(r), UserAgent: Truncate(r.UserAgent(), maxUserAgentLength)}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), clientInfoKey{}, info)))
	})
}

// ClientInfoFrom, context'teki istemci bilgisini döndürür. Yoksa sıfır değer döner.
func ClientInfoFrom(ctx context.Context) ClientInfo {
	info, _ := ctx.Value(clientInfoKey{}).(ClientInfo)
	return info
}

// Truncate, s'yi en fazla n bayta kısaltır. Ortadan bölünen çok baytlı bir UTF-8
// karakteri atılır, böylece veritabanına geçersiz UTF-8 gitmez.
func Truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "")
}
