// Package password, parolaları argon2id ile hash'ler ve doğrular.
//
// Hash'ler PHC biçiminde saklanır:
//
//	$argon2id$v=19$m=65536,t=3,p=2$<salt>$<hash>
//
// Parametreler hash'in içinde durduğu için ileride maliyet artırılsa bile eski
// hash'ler doğrulanmaya devam eder. Kullanıcı başarılı giriş yaptığında
// NeedsRehash ile yeni parametrelere yükseltilebilir.
package password

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Hatalar.
var (
	// ErrMismatch, parolanın hash ile eşleşmediğini bildirir.
	ErrMismatch = errors.New("password: parola eşleşmiyor")
	// ErrInvalidHash, saklanan hash'in biçiminin bozuk olduğunu bildirir.
	ErrInvalidHash = errors.New("password: geçersiz hash biçimi")
	// ErrIncompatibleVersion, hash'in desteklenmeyen bir argon2 sürümüyle üretildiğini bildirir.
	ErrIncompatibleVersion = errors.New("password: desteklenmeyen argon2 sürümü")
)

// Params, argon2id maliyet parametreleridir.
type Params struct {
	Memory      uint32 // KiB cinsinden bellek
	Iterations  uint32 // geçiş sayısı (zaman maliyeti)
	Parallelism uint8  // paralel iş parçacığı sayısı
	SaltLength  uint32 // byte
	KeyLength   uint32 // byte
}

// DefaultParams, production için seçilmiş başlangıç parametreleridir:
// 64 MiB bellek, 3 geçiş, 2 iş parçacığı. Tek bir hash modern bir sunucuda
// yaklaşık 50-100 ms sürer. Saldırgan için GPU ile toplu deneme çok pahalı hale gelir.
var DefaultParams = Params{
	Memory:      64 * 1024,
	Iterations:  3,
	Parallelism: 2,
	SaltLength:  16,
	KeyLength:   32,
}

// Hasher, parolaları hash'ler ve doğrular. Aynı anda çalışabilecek hash işlemi
// sayısını sınırlar: her işlem Params.Memory kadar bellek ayırır ve sınırsız
// eşzamanlılık (ör. ders seçme açılışında binlerce giriş) sunucunun belleğini tüketir.
type Hasher struct {
	params Params
	slots  chan struct{} // semafor: kapasitesi kadar işlem aynı anda çalışabilir
}

// NewHasher, en fazla maxConcurrent hash işleminin aynı anda çalışmasına izin
// veren bir Hasher oluşturur.
func NewHasher(p Params, maxConcurrent int) *Hasher {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	return &Hasher{params: p, slots: make(chan struct{}, maxConcurrent)}
}

// Hash, parolayı rastgele bir salt ile hash'ler ve PHC biçiminde döndürür.
func (h *Hasher) Hash(ctx context.Context, password string) (string, error) {
	salt := make([]byte, h.params.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("password: salt üretilemedi: %w", err)
	}

	key, err := h.derive(ctx, password, salt, h.params)
	if err != nil {
		return "", err
	}

	return encode(h.params, salt, key), nil
}

// Verify, parolanın encoded hash ile eşleşip eşleşmediğini kontrol eder.
// Eşleşmiyorsa ErrMismatch döner. Karşılaştırma sabit zamanlıdır.
func (h *Hasher) Verify(ctx context.Context, password, encoded string) error {
	p, salt, want, err := decode(encoded)
	if err != nil {
		return err
	}

	got, err := h.derive(ctx, password, salt, p)
	if err != nil {
		return err
	}

	// subtle.ConstantTimeCompare, ilk farklı byte'ta erken dönmez. Böylece
	// yanıt süresinden hash hakkında bilgi sızdırılamaz (timing attack).
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return ErrMismatch
	}
	return nil
}

// NeedsRehash, hash'in güncel parametrelerden farklı parametrelerle üretilip
// üretilmediğini söyler. Başarılı girişten sonra true ise parola yeniden hash'lenmelidir.
func (h *Hasher) NeedsRehash(encoded string) bool {
	p, _, _, err := decode(encoded)
	if err != nil {
		return true
	}
	return p != h.params
}

// derive, semafordan bir yer alıp argon2id anahtarını hesaplar.
// Yer beklerken context iptal edilirse beklemeyi bırakır.
func (h *Hasher) derive(ctx context.Context, password string, salt []byte, p Params) ([]byte, error) {
	select {
	case h.slots <- struct{}{}:
		defer func() { <-h.slots }()
	case <-ctx.Done():
		return nil, fmt.Errorf("password: hash sırası beklenirken: %w", ctx.Err())
	}

	return argon2.IDKey([]byte(password), salt, p.Iterations, p.Memory, p.Parallelism, p.KeyLength), nil
}

// encode, parametreleri, salt'ı ve anahtarı PHC biçiminde birleştirir.
// PHC standardı padding'siz standart base64 ister. URL'ye girmediği için
// '+' ve '/' karakterleri burada sorun değildir.
func encode(p Params, salt, key []byte) string {
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.Memory, p.Iterations, p.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key))
}

// decode, PHC biçimindeki hash'i parçalarına ayırır.
func decode(encoded string) (Params, []byte, []byte, error) {
	// "$argon2id$v=19$m=65536,t=3,p=2$salt$hash" → ["", "argon2id", "v=19", "m=...", salt, hash]
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return Params{}, nil, nil, ErrInvalidHash
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return Params{}, nil, nil, ErrInvalidHash
	}
	if version != argon2.Version {
		return Params{}, nil, nil, ErrIncompatibleVersion
	}

	var p Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Iterations, &p.Parallelism); err != nil {
		return Params{}, nil, nil, ErrInvalidHash
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) == 0 {
		return Params{}, nil, nil, ErrInvalidHash
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(key) == 0 {
		return Params{}, nil, nil, ErrInvalidHash
	}

	p.SaltLength = uint32(len(salt))
	p.KeyLength = uint32(len(key))
	return p, salt, key, nil
}
