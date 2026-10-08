package httpx

import (
	"net/http"
	"net/netip"
	"regexp"
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
