package totp

import (
	"strings"
	"testing"
	"time"
)

// RFC 6238 Ek B: SHA-1 test vektörleri (8 hane). Sır ASCII "12345678901234567890".
func TestRFC6238Vectors(t *testing.T) {
	secret := []byte("12345678901234567890")
	tests := []struct {
		unix int64
		want string
	}{
		{59, "94287082"},
		{1111111109, "07081804"},
		{1111111111, "14050471"},
		{1234567890, "89005924"},
		{2000000000, "69279037"},
		{20000000000, "65353130"},
	}
	for _, tt := range tests {
		if got := Code(secret, Step(time.Unix(tt.unix, 0)), 8); got != tt.want {
			t.Errorf("t=%d: %s, want %s", tt.unix, got, tt.want)
		}
	}
}

// RFC 4226 Ek D: HOTP değerleri (6 hane, sayaç 0-9).
func TestRFC4226Vectors(t *testing.T) {
	secret := []byte("12345678901234567890")
	want := []string{"755224", "287082", "359152", "969429", "338314", "254676", "287922", "162583", "399871", "520489"}
	for counter, w := range want {
		if got := Code(secret, uint64(counter), 6); got != w {
			t.Errorf("sayaç %d: %s, want %s", counter, got, w)
		}
	}
}

func TestValidate(t *testing.T) {
	secret := NewSecret()
	now := time.Date(2026, 10, 8, 12, 0, 15, 0, time.UTC)
	step := Step(now)

	current := Code(secret, step, Digits)
	if s, ok := Validate(secret, current, now, 0); !ok || s != step {
		t.Fatalf("geçerli kod reddedildi")
	}
	if _, ok := Validate(secret, current[:3]+" "+current[3:], now, 0); !ok {
		t.Error("araya boşluk konmuş kod kabul edilmeli")
	}
	// Saat kayması: önceki ve sonraki adım kabul, iki adım öncesi değil.
	if _, ok := Validate(secret, Code(secret, step-1, Digits), now, 0); !ok {
		t.Error("önceki adım kabul edilmeli")
	}
	if _, ok := Validate(secret, Code(secret, step+1, Digits), now, 0); !ok {
		t.Error("sonraki adım kabul edilmeli")
	}
	if _, ok := Validate(secret, Code(secret, step-2, Digits), now, 0); ok {
		t.Error("iki adım öncesi reddedilmeli")
	}
	// Tekrar oynatma: kullanılmış adım ve öncesi reddedilir.
	if _, ok := Validate(secret, current, now, step); ok {
		t.Error("kullanılmış kod ikinci kez kabul edildi")
	}
	if _, ok := Validate(secret, "12345", now, 0); ok {
		t.Error("eksik haneli kod kabul edildi")
	}
}

func TestKeyURI(t *testing.T) {
	uri := KeyURI("Agora", "P90001", []byte("12345678901234567890"))
	for _, part := range []string{"otpauth://totp/Agora:P90001?", "secret=GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ", "issuer=Agora", "digits=6", "period=30"} {
		if !strings.Contains(uri, part) {
			t.Errorf("%s içinde %q yok", uri, part)
		}
	}
}
