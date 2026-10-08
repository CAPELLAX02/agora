package secretbox

import (
	"bytes"
	"crypto/rand"
	"errors"
	"testing"
)

func newBox(t *testing.T) *Box {
	t.Helper()
	b, err := New(bytes.Repeat([]byte{7}, KeySize))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestSealOpen(t *testing.T) {
	b := newBox(t)
	secret := []byte("12345678901234567890")
	user := []byte("01a11b7f-0000-7000-8000-000000000001")

	sealed := b.Seal(secret, user)
	if bytes.Contains(sealed, secret) {
		t.Fatal("şifreli veri düz metni içeriyor")
	}
	got, err := b.Open(sealed, user)
	if err != nil || !bytes.Equal(got, secret) {
		t.Fatalf("Open = %q, %v", got, err)
	}
	// Aynı veri iki kez şifrelenince farklı çıktı (rastgele nonce).
	if bytes.Equal(sealed, b.Seal(secret, user)) {
		t.Error("iki şifreleme aynı çıktıyı verdi")
	}
}

func TestOpenRejects(t *testing.T) {
	b := newBox(t)
	user := []byte("kullanici-1")
	sealed := b.Seal([]byte("sır"), user)

	other, _ := New(func() []byte { k := make([]byte, KeySize); _, _ = rand.Read(k); return k }())
	tampered := bytes.Clone(sealed)
	tampered[len(tampered)-1] ^= 1

	tests := map[string]struct {
		box     *Box
		sealed  []byte
		binding []byte
	}{
		"başka kayıt":      {b, sealed, []byte("kullanici-2")},
		"başka anahtar":    {other, sealed, user},
		"değiştirilmiş":    {b, tampered, user},
		"bilinmeyen sürüm": {b, append([]byte{9}, sealed[1:]...), user},
		"boş":              {b, nil, user},
		"kısa":             {b, sealed[:5], user},
	}
	for name, tt := range tests {
		if _, err := tt.box.Open(tt.sealed, tt.binding); !errors.Is(err, ErrOpen) {
			t.Errorf("%s: hata = %v, ErrOpen bekleniyordu", name, err)
		}
	}
}

func TestNewKeySize(t *testing.T) {
	if _, err := New(make([]byte, 16)); err == nil {
		t.Error("16 baytlık anahtar kabul edildi")
	}
}
