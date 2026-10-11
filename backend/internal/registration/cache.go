package registration

import (
	"sync"
	"time"
)

// ttlCache, süreç içi kısa ömürlü bir önbellektir. Ders seçme açılışında her ekleme aynı
// müfredatı, eşdeğerlikleri, ön koşulları ve yönetmelik parametrelerini okur; bunlar dönem
// içinde neredeyse hiç değişmez (yürürlükteki müfredat dondurulmuştur). Önbellek bu okumaları
// veritabanından alır; bir değişiklik en geç süre dolunca görünür.
//
// Değer yükleme kilit dışında yapılır: aynı anahtar için eşzamanlı iki yükleme olabilir
// (zararsız), ama bir yükleme diğer anahtarları beklemez.
type ttlCache[T any] struct {
	ttl   time.Duration
	mu    sync.Mutex
	items map[string]ttlEntry[T]
}

type ttlEntry[T any] struct {
	value   T
	expires time.Time
}

func newTTLCache[T any](ttl time.Duration) *ttlCache[T] {
	return &ttlCache[T]{ttl: ttl, items: map[string]ttlEntry[T]{}}
}

// get, anahtarın değerini döndürür; yoksa ya da süresi dolduysa load ile yükler.
func (c *ttlCache[T]) get(key string, load func() (T, error)) (T, error) {
	now := time.Now()
	c.mu.Lock()
	e, ok := c.items[key]
	c.mu.Unlock()
	if ok && now.Before(e.expires) {
		return e.value, nil
	}
	v, err := load()
	if err != nil {
		return v, err
	}
	c.mu.Lock()
	c.items[key] = ttlEntry[T]{value: v, expires: now.Add(c.ttl)}
	c.mu.Unlock()
	return v, nil
}
