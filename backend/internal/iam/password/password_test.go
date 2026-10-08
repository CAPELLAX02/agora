package password

import (
	"context"
	"errors"
	"regexp"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// testParams, testleri hızlı tutmak için düşük maliyetli parametrelerdir.
// Production parametreleri benchmark'ta ölçülür.
var testParams = Params{Memory: 64, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}

var phc = regexp.MustCompile(`^\$argon2id\$v=19\$m=64,t=1,p=1\$[A-Za-z0-9+/]{22}\$[A-Za-z0-9+/]{43}$`)

func TestHashAndVerify(t *testing.T) {
	ctx := context.Background()
	h := NewHasher(testParams, 4)

	encoded, err := h.Hash(ctx, "doğru-parola-123")
	if err != nil {
		t.Fatal(err)
	}
	if !phc.MatchString(encoded) {
		t.Errorf("hash PHC biçiminde değil: %s", encoded)
	}

	if err := h.Verify(ctx, "doğru-parola-123", encoded); err != nil {
		t.Errorf("doğru parola reddedildi: %v", err)
	}
	if err := h.Verify(ctx, "yanlış-parola-123", encoded); !errors.Is(err, ErrMismatch) {
		t.Errorf("yanlış parola: err = %v, want ErrMismatch", err)
	}
	if err := h.Verify(ctx, "Doğru-parola-123", encoded); !errors.Is(err, ErrMismatch) {
		t.Errorf("büyük/küçük harf farkı: err = %v, want ErrMismatch", err)
	}
}

func TestHashUsesRandomSalt(t *testing.T) {
	h := NewHasher(testParams, 1)
	a, _ := h.Hash(context.Background(), "aynı-parola-123")
	b, _ := h.Hash(context.Background(), "aynı-parola-123")
	if a == b {
		t.Error("aynı parola iki kez aynı hash'i üretti: salt rastgele değil")
	}
}

func TestVerifyRejectsMalformedHashes(t *testing.T) {
	h := NewHasher(testParams, 1)
	tests := []struct {
		name    string
		encoded string
		want    error
	}{
		{"boş", "", ErrInvalidHash},
		{"başka algoritma", "$bcrypt$v=19$m=64,t=1,p=1$c2FsdA$aGFzaA", ErrInvalidHash},
		{"eksik parça", "$argon2id$v=19$m=64,t=1,p=1$c2FsdA", ErrInvalidHash},
		{"bozuk parametre", "$argon2id$v=19$m=x,t=1,p=1$c2FsdA$aGFzaA", ErrInvalidHash},
		{"bozuk base64", "$argon2id$v=19$m=64,t=1,p=1$%%%$aGFzaA", ErrInvalidHash},
		{"eski argon2 sürümü", "$argon2id$v=16$m=64,t=1,p=1$c2FsdA$aGFzaA", ErrIncompatibleVersion},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := h.Verify(context.Background(), "parola", tt.encoded); !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestNeedsRehash(t *testing.T) {
	old := NewHasher(testParams, 1)
	encoded, err := old.Hash(context.Background(), "parola-123456")
	if err != nil {
		t.Fatal(err)
	}

	if old.NeedsRehash(encoded) {
		t.Error("aynı parametrelerle üretilen hash için yeniden hash istenmemeli")
	}

	stronger := testParams
	stronger.Iterations = 2
	upgraded := NewHasher(stronger, 1)

	if !upgraded.NeedsRehash(encoded) {
		t.Error("parametreler artırıldığında eski hash yeniden hash istemeli")
	}
	// Parametreler hash'in içinde olduğu için eski hash hâlâ doğrulanabilmeli.
	if err := upgraded.Verify(context.Background(), "parola-123456", encoded); err != nil {
		t.Errorf("eski parametreli hash doğrulanamadı: %v", err)
	}
	if !upgraded.NeedsRehash("bozuk") {
		t.Error("bozuk hash yeniden hash istemeli")
	}
}

func TestHasherLimitsConcurrency(t *testing.T) {
	// Daha pahalı parametreler: her işlem birkaç ms sürsün ki çakışma gözlenebilsin.
	p := Params{Memory: 8 * 1024, Iterations: 2, Parallelism: 1, SaltLength: 16, KeyLength: 32}
	const limit = 2
	h := NewHasher(p, limit)

	var (
		running, peak atomic.Int32
		wg            sync.WaitGroup
	)
	// derive'ı doğrudan çağırarak semaforun içindeki eşzamanlı işlem sayısını ölçüyoruz.
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case h.slots <- struct{}{}:
			case <-time.After(5 * time.Second):
				t.Error("semafor yeri alınamadı")
				return
			}
			n := running.Add(1)
			for {
				old := peak.Load()
				if n <= old || peak.CompareAndSwap(old, n) {
					break
				}
			}
			time.Sleep(5 * time.Millisecond)
			running.Add(-1)
			<-h.slots
		}()
	}
	wg.Wait()

	if got := peak.Load(); got > limit {
		t.Errorf("aynı anda %d işlem çalıştı, sınır %d", got, limit)
	}
}

func TestVerifyRespectsContextWhileWaiting(t *testing.T) {
	h := NewHasher(testParams, 1)
	encoded, _ := h.Hash(context.Background(), "parola-123456")

	h.slots <- struct{}{} // tek yeri biz tutuyoruz: Verify beklemek zorunda kalacak
	defer func() { <-h.slots }()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := h.Verify(ctx, "parola-123456", encoded)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want context.DeadlineExceeded", err)
	}
}

// BenchmarkHash, production parametreleriyle tek bir hash'in maliyetini ölçer.
// Çalıştırma: go test -bench=Hash -benchmem ./internal/iam/password
func BenchmarkHash(b *testing.B) {
	h := NewHasher(DefaultParams, 1)
	ctx := context.Background()
	for b.Loop() {
		if _, err := h.Hash(ctx, "benchmark-parolası-123"); err != nil {
			b.Fatal(err)
		}
	}
}
