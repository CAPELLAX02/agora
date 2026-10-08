// Package jwt, Ed25519 (EdDSA) ile imzalanmış JSON Web Token'ları üretir ve doğrular.
//
// Sadece Agora'nın ihtiyacı olan alt küme gerçeklenmiştir: tek algoritma (EdDSA),
// kompakt JWS biçimi ve kayıtlı claim'lerin doğrulanması. Kapsamın dar tutulması
// bilinçlidir: JWT kütüphanelerindeki tarihi açıkların çoğu ("alg":"none",
// RS256/HS256 karışıklığı) gereğinden fazla esneklikten doğmuştur.
//
// Biçim (RFC 7519, RFC 7515, RFC 8037):
//
//	base64url(header) "." base64url(claims) "." base64url(Ed25519(header "." claims))
package jwt

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Algorithm, desteklenen tek imza algoritmasıdır (RFC 8037).
const Algorithm = "EdDSA"

// Doğrulama hataları. Çağıranlar errors.Is ile ayırt edebilir, ama istemciye
// hangisi olduğu söylenmez: hepsi aynı 401 yanıtına dönüşür.
var (
	ErrMalformed        = errors.New("jwt: biçim bozuk")
	ErrUnsupportedAlg   = errors.New("jwt: desteklenmeyen algoritma")
	ErrUnknownKey       = errors.New("jwt: bilinmeyen anahtar kimliği")
	ErrInvalidSignature = errors.New("jwt: imza geçersiz")
	ErrExpired          = errors.New("jwt: süresi dolmuş")
	ErrNotYetValid      = errors.New("jwt: henüz geçerli değil")
	ErrInvalidClaims    = errors.New("jwt: claim'ler geçersiz")
)

// b64, JWT'nin kullandığı padding'siz base64url kodlamasıdır. Strict, son
// karakterdeki kullanılmayan bitlerin sıfır olmasını şart koşar. Aksi halde aynı
// imzanın 16 farklı yazılışı geçerli sayılırdı (token malleability).
var b64 = base64.RawURLEncoding.Strict()

// Claims, Agora access token'larının taşıdığı alanlardır. Zamanlar Unix saniyesidir.
type Claims struct {
	Issuer    string   `json:"iss"`
	Subject   string   `json:"sub"` // kullanıcı kimliği
	Audience  string   `json:"aud"`
	ExpiresAt int64    `json:"exp"`
	NotBefore int64    `json:"nbf"`
	IssuedAt  int64    `json:"iat"`
	ID        string   `json:"jti"`           // token'ın benzersiz kimliği
	SessionID string   `json:"sid"`           // oturum kimliği: iptal edilmiş oturumlar için
	AMR       []string `json:"amr,omitempty"` // kimlik doğrulama yöntemleri: ["pwd"], ["pwd","otp"]
}

type header struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
	Kid string `json:"kid"`
}

// Signer, token'ları tek bir Ed25519 özel anahtarıyla imzalar.
type Signer struct {
	keyID string
	key   ed25519.PrivateKey
}

// NewSigner, keyID ile tanımlanan özel anahtarı kullanan bir Signer oluşturur.
// keyID token başlığına (kid) yazılır, doğrulayan taraf hangi açık anahtarı
// kullanacağını buradan bilir. Böylece anahtar rotasyonu sırasında eski ve yeni
// anahtarla imzalanmış token'lar bir süre birlikte geçerli olabilir.
func NewSigner(keyID string, key ed25519.PrivateKey) (*Signer, error) {
	if keyID == "" {
		return nil, errors.New("jwt: anahtar kimliği boş olamaz")
	}
	if len(key) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("jwt: geçersiz Ed25519 özel anahtar uzunluğu: %d", len(key))
	}
	return &Signer{keyID: keyID, key: key}, nil
}

// Sign, claim'leri imzalar ve kompakt JWT metnini döndürür.
func (s *Signer) Sign(c Claims) (string, error) {
	h, err := json.Marshal(header{Alg: Algorithm, Typ: "JWT", Kid: s.keyID})
	if err != nil {
		return "", fmt.Errorf("jwt: başlık kodlanamadı: %w", err)
	}
	p, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("jwt: claim'ler kodlanamadı: %w", err)
	}

	signingInput := b64.EncodeToString(h) + "." + b64.EncodeToString(p)
	sig := ed25519.Sign(s.key, []byte(signingInput))

	return signingInput + "." + b64.EncodeToString(sig), nil
}

// Verifier, token'ları bilinen açık anahtarlarla ve beklenen issuer/audience'a göre doğrular.
type Verifier struct {
	keys     map[string]ed25519.PublicKey
	issuer   string
	audience string
	leeway   time.Duration
}

// NewVerifier, verilen açık anahtarları (kid → anahtar) tanıyan bir Verifier oluşturur.
// leeway, sunucular arasındaki küçük saat farklarını tolere etmek için exp ve nbf
// kontrollerine eklenen paydır (ör. 30 saniye).
func NewVerifier(issuer, audience string, keys map[string]ed25519.PublicKey, leeway time.Duration) *Verifier {
	return &Verifier{keys: keys, issuer: issuer, audience: audience, leeway: leeway}
}

// Verify, token'ı doğrular ve claim'lerini döndürür. Zaman parametre olarak
// alınır: kod time.Now'a gizlice bağlı olmaz ve testlerde zaman kontrol edilebilir.
//
// Sıralama önemlidir: claim'ler imza doğrulanmadan önce asla yorumlanmaz.
func (v *Verifier) Verify(token string, now time.Time) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, ErrMalformed
	}

	var h header
	if err := decodeSegment(parts[0], &h); err != nil {
		return Claims{}, ErrMalformed
	}
	// "alg" saldırganın kontrolündedir. Ne yazdığına bakmadan sadece bizim
	// algoritmamızı kabul ediyoruz: "none" ve diğer algoritmalar reddedilir.
	if h.Alg != Algorithm {
		return Claims{}, ErrUnsupportedAlg
	}
	key, ok := v.keys[h.Kid]
	if !ok {
		return Claims{}, ErrUnknownKey
	}

	sig, err := b64.DecodeString(parts[2])
	if err != nil {
		return Claims{}, ErrMalformed
	}
	signingInput := parts[0] + "." + parts[1]
	if !ed25519.Verify(key, []byte(signingInput), sig) {
		return Claims{}, ErrInvalidSignature
	}

	// Buradan sonrası imzalı, yani bizim ürettiğimiz veri.
	var c Claims
	if err := decodeSegment(parts[1], &c); err != nil {
		return Claims{}, ErrMalformed
	}

	if c.Issuer != v.issuer || c.Audience != v.audience || c.Subject == "" {
		return Claims{}, ErrInvalidClaims
	}
	if c.ExpiresAt == 0 || !now.Before(time.Unix(c.ExpiresAt, 0).Add(v.leeway)) {
		return Claims{}, ErrExpired
	}
	if now.Add(v.leeway).Before(time.Unix(c.NotBefore, 0)) {
		return Claims{}, ErrNotYetValid
	}

	return c, nil
}

func decodeSegment(seg string, v any) error {
	b, err := b64.DecodeString(seg)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
