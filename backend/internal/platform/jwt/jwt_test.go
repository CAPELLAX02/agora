package jwt

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestRFC8037Vector, RFC 8037 Ek A.4'teki resmi Ed25519 JWS test vektörünü doğrular:
// aynı anahtar ve aynı girdiyle byte byte aynı imza üretilmeli.
func TestRFC8037Vector(t *testing.T) {
	d, _ := b64.DecodeString("nWGxne_9WmC6hEr0kuwsxERJxWl7MmkZcDusAxyuf2A")
	x, _ := b64.DecodeString("11qYAYKxCrfVS_7TyWQHOg7hcvPapiMlrwIaaPcHURo")
	priv := ed25519.NewKeyFromSeed(d)

	if got := b64.EncodeToString(priv.Public().(ed25519.PublicKey)); got != b64.EncodeToString(x) {
		t.Fatalf("açık anahtar = %s, RFC'deki ile aynı değil", got)
	}

	input := "eyJhbGciOiJFZERTQSJ9.RXhhbXBsZSBvZiBFZDI1NTE5IHNpZ25pbmc"
	want := "hgyY0il_MGCjP0JzlnLWG1PPOt7-09PGcvMg3AIbQR6dWbhijcNR4ki4iylGjg5BhVsPt9g7sVvpAr_MuM0KAg"
	if got := b64.EncodeToString(ed25519.Sign(priv, []byte(input))); got != want {
		t.Errorf("imza =\n  %s\nwant\n  %s", got, want)
	}
}

var t0 = time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)

func newKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return priv
}

func validClaims() Claims {
	return Claims{
		Issuer:    "agora",
		Subject:   "01a11b2d-57fe-7520-8e98-b6dcc256425b",
		Audience:  "agora-api",
		IssuedAt:  t0.Unix(),
		NotBefore: t0.Unix(),
		ExpiresAt: t0.Add(15 * time.Minute).Unix(),
		ID:        "jti-1",
		SessionID: "sid-1",
		AMR:       []string{"pwd"},
	}
}

func setup(t *testing.T) (*Signer, *Verifier) {
	t.Helper()
	key := newKey(t)
	s, err := NewSigner("k1", key)
	if err != nil {
		t.Fatal(err)
	}
	v := NewVerifier("agora", "agora-api",
		map[string]ed25519.PublicKey{"k1": key.Public().(ed25519.PublicKey)}, 30*time.Second)
	return s, v
}

func TestSignAndVerify(t *testing.T) {
	s, v := setup(t)

	token, err := s.Sign(validClaims())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(token, ".") != 2 || strings.ContainsAny(token, "+/=") {
		t.Fatalf("token kompakt base64url biçiminde değil: %s", token)
	}

	got, err := v.Verify(token, t0.Add(time.Minute))
	if err != nil {
		t.Fatalf("geçerli token reddedildi: %v", err)
	}
	if got.Subject != validClaims().Subject || got.SessionID != "sid-1" || got.AMR[0] != "pwd" {
		t.Errorf("claim'ler = %+v", got)
	}
}

func TestVerifyTimeChecks(t *testing.T) {
	s, v := setup(t)
	token, _ := s.Sign(validClaims())
	exp := t0.Add(15 * time.Minute)

	tests := []struct {
		name string
		now  time.Time
		want error
	}{
		{"süre dolmadan hemen önce", exp.Add(-time.Second), nil},
		{"süre doldu ama tolerans içinde", exp.Add(20 * time.Second), nil},
		{"tolerans da geçti", exp.Add(31 * time.Second), ErrExpired},
		{"nbf'den önce ama tolerans içinde (saat kayması)", t0.Add(-20 * time.Second), nil},
		{"nbf'den çok önce", t0.Add(-time.Minute), ErrNotYetValid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := v.Verify(token, tt.now)
			if !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestVerifyClaimChecks(t *testing.T) {
	s, v := setup(t)

	tests := []struct {
		name   string
		modify func(c *Claims)
		want   error
	}{
		{"başka issuer", func(c *Claims) { c.Issuer = "baska-sistem" }, ErrInvalidClaims},
		{"başka audience", func(c *Claims) { c.Audience = "agora-admin" }, ErrInvalidClaims},
		{"boş subject", func(c *Claims) { c.Subject = "" }, ErrInvalidClaims},
		{"exp yok", func(c *Claims) { c.ExpiresAt = 0 }, ErrExpired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validClaims()
			tt.modify(&c)
			token, _ := s.Sign(c)
			if _, err := v.Verify(token, t0.Add(time.Minute)); !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

// TestVerifyRejectsAttacks, bilinen JWT saldırılarına karşı davranışı doğrular.
func TestVerifyRejectsAttacks(t *testing.T) {
	s, v := setup(t)
	token, _ := s.Sign(validClaims())
	parts := strings.Split(token, ".")
	enc := func(s string) string { return b64.EncodeToString([]byte(s)) }

	// Saldırgan payload'u değiştirip kendini başka kullanıcı yapmaya çalışıyor.
	forged := validClaims()
	forged.Subject = "admin"
	forgedToken, _ := (&Signer{keyID: "k1", key: newKey(t)}).Sign(forged)
	forgedPayload := strings.Split(forgedToken, ".")[1]

	tests := []struct {
		name  string
		token string
		want  error
	}{
		{
			name:  `"alg":"none" ile imzasız token`,
			token: enc(`{"alg":"none","typ":"JWT","kid":"k1"}`) + "." + parts[1] + ".",
			want:  ErrUnsupportedAlg,
		},
		{
			name:  "HS256'ya geçirilmiş başlık (algoritma karışıklığı)",
			token: enc(`{"alg":"HS256","typ":"JWT","kid":"k1"}`) + "." + parts[1] + "." + parts[2],
			want:  ErrUnsupportedAlg,
		},
		{
			name:  "payload değiştirilmiş, imza eski",
			token: parts[0] + "." + forgedPayload + "." + parts[2],
			want:  ErrInvalidSignature,
		},
		{
			name:  "başka bir anahtarla imzalanmış",
			token: forgedToken,
			want:  ErrInvalidSignature,
		},
		{
			name:  "bilinmeyen kid",
			token: enc(`{"alg":"EdDSA","typ":"JWT","kid":"k9"}`) + "." + parts[1] + "." + parts[2],
			want:  ErrUnknownKey,
		},
		{name: "iki parça", token: parts[0] + "." + parts[1], want: ErrMalformed},
		{name: "dört parça", token: token + ".x", want: ErrMalformed},
		{name: "bozuk base64", token: "%%%." + parts[1] + "." + parts[2], want: ErrMalformed},
		{name: "boş", token: "", want: ErrMalformed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := v.Verify(tt.token, t0.Add(time.Minute)); !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

// TestVerifyRejectsNonCanonicalEncoding, aynı imzanın farklı base64 yazılışlarının
// (son karakterin kullanılmayan bitleri değiştirilerek) reddedildiğini doğrular.
func TestVerifyRejectsNonCanonicalEncoding(t *testing.T) {
	s, v := setup(t)
	token, _ := s.Sign(validClaims())

	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	last := token[len(token)-1]
	idx := strings.IndexByte(alphabet, last)
	// 64 byte'lık imzanın son karakteri sadece 2 anlamlı bit taşır. En alttaki biti
	// çevirmek byte'ları değiştirmez, sadece yazılışı değiştirir.
	variant := token[:len(token)-1] + string(alphabet[idx^1])

	if _, err := v.Verify(variant, t0.Add(time.Minute)); !errors.Is(err, ErrMalformed) {
		t.Errorf("kanonik olmayan imza yazılışı: err = %v, want ErrMalformed", err)
	}
}

func TestKeyRotation(t *testing.T) {
	oldKey, newKeyPriv := newKey(t), newKey(t)
	oldSigner, _ := NewSigner("2026-09", oldKey)
	newSigner, _ := NewSigner("2026-10", newKeyPriv)

	// Rotasyon döneminde doğrulayıcı iki açık anahtarı da tanır.
	v := NewVerifier("agora", "agora-api", map[string]ed25519.PublicKey{
		"2026-09": oldKey.Public().(ed25519.PublicKey),
		"2026-10": newKeyPriv.Public().(ed25519.PublicKey),
	}, 0)

	for _, s := range []*Signer{oldSigner, newSigner} {
		token, _ := s.Sign(validClaims())
		if _, err := v.Verify(token, t0.Add(time.Minute)); err != nil {
			t.Errorf("%s anahtarıyla imzalanan token reddedildi: %v", s.keyID, err)
		}
	}
}

func TestNewSignerValidatesInput(t *testing.T) {
	if _, err := NewSigner("", newKey(t)); err == nil {
		t.Error("boş kid kabul edildi")
	}
	if _, err := NewSigner("k1", ed25519.PrivateKey("kısa")); err == nil {
		t.Error("geçersiz uzunluktaki anahtar kabul edildi")
	}
}

// FuzzVerify, Verify'ın rastgele girdilerde asla panic olmadığını ve hiçbir
// rastgele girdiyi geçerli saymadığını sınar.
// Çalıştırma: go test -fuzz=FuzzVerify -fuzztime=30s ./internal/platform/jwt
func FuzzVerify(f *testing.F) {
	key := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	s, _ := NewSigner("k1", key)
	v := NewVerifier("agora", "agora-api", map[string]ed25519.PublicKey{"k1": key.Public().(ed25519.PublicKey)}, 0)

	valid, _ := s.Sign(validClaims())
	f.Add(valid)
	f.Add("")
	f.Add("a.b.c")
	f.Add(base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`)) + "..")

	f.Fuzz(func(t *testing.T, token string) {
		_, err := v.Verify(token, t0.Add(time.Minute))
		if err == nil && token != valid {
			t.Errorf("rastgele bir girdi geçerli sayıldı: %q", token)
		}
	})
}
