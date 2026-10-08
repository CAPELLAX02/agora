// Package secretbox, veritabanında saklanması gereken küçük sırları (ör. TOTP anahtarı)
// AES-256-GCM ile şifreler.
//
// Parolalar gibi doğrulanması yeten değerler hash'lenir. TOTP sırrı ise her doğrulamada
// düz haline ihtiyaç duyulduğu için hash'lenemez: şifrelenir. Böylece sadece veritabanı
// yedeği sızan biri sırları kullanamaz, uygulamanın anahtarına da ihtiyaç duyar.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"fmt"
)

// KeySize, anahtar uzunluğudur (AES-256).
const KeySize = 32

// version, şifreli verinin ilk baytıdır. Anahtar rotasyonunda yeni anahtar yeni bir
// sürüm numarası alır: eski kayıtlar eski anahtarla açılmaya devam eder, veri
// taşımaya gerek kalmadan zamanla yeniden şifrelenir.
const version byte = 1

// ErrOpen, şifreli verinin açılamadığını bildirir: veri bozuk, başka bir anahtarla
// şifrelenmiş ya da başka bir kayda ait (bağlam uyuşmuyor).
var ErrOpen = errors.New("secretbox: şifreli veri açılamadı")

// Box, bir anahtarla şifreleme ve çözme yapar. Eşzamanlı kullanıma uygundur.
type Box struct {
	aead cipher.AEAD
}

// New, 32 baytlık anahtarla bir Box oluşturur.
func New(key []byte) (*Box, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("secretbox: anahtar %d bayt olmalı, %d verildi", KeySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secretbox: %w", err)
	}
	// Nonce her şifrelemede rastgele üretilir ve çıktının başına eklenir. 96 bitlik
	// rastgele nonce, aynı anahtarla 2^32 şifrelemeye kadar güvenlidir: kullanıcı
	// başına bir sır için fazlasıyla yeterli.
	aead, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return nil, fmt.Errorf("secretbox: %w", err)
	}
	return &Box{aead: aead}, nil
}

// Seal, plaintext'i şifreler. binding, şifreli verinin ait olduğu kayıttır (ör.
// kullanıcı kimliği): şifrelenmez ama doğrulanır. Böylece bir kullanıcının şifreli
// sırrı veritabanında başka bir kullanıcının satırına kopyalanırsa açılamaz.
func (b *Box) Seal(plaintext, binding []byte) []byte {
	out := make([]byte, 1, 1+b.aead.Overhead()+len(plaintext))
	out[0] = version
	return b.aead.Seal(out, nil, plaintext, binding)
}

// Open, Seal ile şifrelenmiş veriyi çözer.
func (b *Box) Open(sealed, binding []byte) ([]byte, error) {
	if len(sealed) < 1 || sealed[0] != version {
		return nil, ErrOpen
	}
	plaintext, err := b.aead.Open(nil, nil, sealed[1:], binding)
	if err != nil {
		return nil, ErrOpen
	}
	return plaintext, nil
}
