// Package totp, zaman tabanlı tek kullanımlık parolaları (TOTP, RFC 6238) üretir ve
// doğrular. Google Authenticator, Microsoft Authenticator gibi uygulamalarla uyumludur:
// SHA-1, 30 saniyelik adım, 6 hane.
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Standart ayarlar. Doğrulayıcı uygulamaların neredeyse hepsi sadece bunları destekler.
const (
	Period     = 30 * time.Second
	Digits     = 6
	SecretSize = 20 // bayt (160 bit), RFC 4226'nın önerdiği uzunluk
)

// b32, sırların biçimidir: doğrulayıcı uygulamalar dolgusuz (padding) base32 bekler.
var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewSecret, rastgele yeni bir sır üretir.
func NewSecret() []byte {
	secret := make([]byte, SecretSize)
	_, _ = rand.Read(secret) // crypto/rand.Read hata döndürmez (Go 1.24+)
	return secret
}

// EncodeSecret, sırrı kullanıcının elle de girebileceği base32 metne çevirir.
func EncodeSecret(secret []byte) string {
	return b32.EncodeToString(secret)
}

// Step, t anının TOTP adım numarasıdır.
func Step(t time.Time) uint64 {
	return uint64(t.Unix()) / uint64(Period/time.Second)
}

// Code, sırrın verilen adımdaki kodunu üretir (RFC 4226 HOTP, sayaç = adım).
func Code(secret []byte, step uint64, digits int) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], step)
	mac := hmac.New(sha1.New, secret)
	mac.Write(msg[:])
	sum := mac.Sum(nil)

	// Dinamik kesme (RFC 4226 §5.3): son baytın alt 4 biti, 31 bitlik değerin başladığı yerdir.
	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff

	mod := uint32(1)
	for range digits {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", digits, value%mod)
}

// Validate, kodun t anında geçerli olup olmadığını denetler ve eşleşen adımı döndürür.
// Saat kayması için bir önceki ve bir sonraki adım da kabul edilir (±30 sn). lastUsed,
// daha önce kabul edilmiş son adımdır: o adım ve öncesi reddedilir. Böylece ekrandan ya
// da omuz üstünden görülen bir kod ikinci kez kullanılamaz (tekrar oynatma).
func Validate(secret []byte, code string, t time.Time, lastUsed uint64) (step uint64, ok bool) {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != Digits {
		return 0, false
	}
	now := Step(t)
	for _, s := range []uint64{now - 1, now, now + 1} {
		if s <= lastUsed {
			continue
		}
		// Sabit zamanlı karşılaştırma: kodun kaç hanesinin tuttuğu süreden anlaşılmasın.
		if subtle.ConstantTimeCompare([]byte(Code(secret, s, Digits)), []byte(code)) == 1 {
			return s, true
		}
	}
	return 0, false
}

// KeyURI, doğrulayıcı uygulamaların QR koddan okuduğu otpauth adresini üretir.
// https://github.com/google/google-authenticator/wiki/Key-Uri-Format
func KeyURI(issuer, account string, secret []byte) string {
	label := url.PathEscape(issuer + ":" + account)
	q := url.Values{}
	q.Set("secret", EncodeSecret(secret))
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprint(Digits))
	q.Set("period", fmt.Sprint(int(Period.Seconds())))
	return "otpauth://totp/" + label + "?" + q.Encode()
}
