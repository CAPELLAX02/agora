package jwt

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
)

// ParsePrivateKeyPEM, PEM içindeki PKCS#8 biçimli Ed25519 özel anahtarını çözer.
// "openssl genpkey -algorithm ed25519" anahtarı bu biçimde üretir.
func ParsePrivateKeyPEM(data []byte) (ed25519.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "PRIVATE KEY" {
		return nil, errors.New(`jwt: "PRIVATE KEY" türünde PEM bloğu bulunamadı`)
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("jwt: özel anahtar çözülemedi: %w", err)
	}

	priv, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("jwt: anahtar Ed25519 değil: %T", key)
	}
	return priv, nil
}

// KeyID, açık anahtarın RFC 7638 JWK parmak izini döndürür ve token başlığındaki
// "kid" olarak kullanılır. Kimlik anahtarın kendisinden türetildiği için ayrıca
// yapılandırılması gerekmez ve iki farklı anahtar aynı kimliği alamaz.
func KeyID(pub ed25519.PublicKey) string {
	// RFC 7638: zorunlu alanlar alfabetik sırayla, boşluksuz JSON olarak hash'lenir.
	jwk := `{"crv":"Ed25519","kty":"OKP","x":"` + b64.EncodeToString(pub) + `"}`
	sum := sha256.Sum256([]byte(jwk))
	return b64.EncodeToString(sum[:])
}
