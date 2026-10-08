package jwt

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
)

// RFC 8037 Ek A.3: Ed25519 açık anahtarının JWK parmak izi.
func TestKeyIDRFC8037Vector(t *testing.T) {
	x, _ := b64.DecodeString("11qYAYKxCrfVS_7TyWQHOg7hcvPapiMlrwIaaPcHURo")

	const want = "kPrK_qmxVWaYVA9wwBF6Iuo3vVzz7TxHCTwXBygrS4k"
	if got := KeyID(ed25519.PublicKey(x)); got != want {
		t.Errorf("KeyID = %s, want %s", got, want)
	}
}

func pemEncode(t *testing.T, typ string, key any) []byte {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der})
}

func TestParsePrivateKeyPEM(t *testing.T) {
	key := newKey(t)

	got, err := ParsePrivateKeyPEM(pemEncode(t, "PRIVATE KEY", key))
	if err != nil {
		t.Fatalf("geçerli anahtar reddedildi: %v", err)
	}
	if !got.Equal(key) {
		t.Error("çözülen anahtar orijinaliyle aynı değil")
	}
}

// "openssl genpkey -algorithm ed25519" çıktısıyla aynı biçimde, RFC 8037'deki örnek
// anahtarı içeren bir dosya. "openssl pkey -in ... -text" ile de doğrulandı.
func TestParsePrivateKeyPEMOpenSSLFormat(t *testing.T) {
	const openssl = `-----BEGIN PRIVATE KEY-----
MC4CAQAwBQYDK2VwBCIEIJ1hsZ3v/VpguoRK9JLsLMREScVpezJpGXA7rAMcrn9g
-----END PRIVATE KEY-----
`
	got, err := ParsePrivateKeyPEM([]byte(openssl))
	if err != nil {
		t.Fatal(err)
	}
	if pub := b64.EncodeToString(got.Public().(ed25519.PublicKey)); pub != "11qYAYKxCrfVS_7TyWQHOg7hcvPapiMlrwIaaPcHURo" {
		t.Errorf("açık anahtar = %s", pub)
	}
}

func TestParsePrivateKeyPEMRejects(t *testing.T) {
	ec, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		data []byte
		want string
	}{
		{"PEM değil", []byte("merhaba"), "PEM bloğu bulunamadı"},
		{"yanlış blok türü", pemEncode(t, "EC PRIVATE KEY", newKey(t)), "PEM bloğu bulunamadı"},
		{"bozuk içerik", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte{1, 2, 3}}), "çözülemedi"},
		{"Ed25519 olmayan anahtar", pemEncode(t, "PRIVATE KEY", ec), "Ed25519 değil: *ecdsa.PrivateKey"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParsePrivateKeyPEM(tt.data)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, %q içermeli", err, tt.want)
			}
		})
	}
}
