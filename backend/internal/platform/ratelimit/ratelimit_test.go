package ratelimit

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/CAPELLAX02/agora/backend/internal/platform/redistest"
)

// fakeClock, testlerde zamanı elle ilerletmeye yarar.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)}
}

func allow(t *testing.T, l *Limiter, key string) Result {
	t.Helper()
	res, err := l.Allow(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestSlidingWindow(t *testing.T) {
	rdb := redistest.New(t)
	clock := newClock()
	l := New(rdb, "test", 3, time.Minute, clock.Now)

	// 3 hak sırayla tükenir.
	for i, wantRemaining := range []int{2, 1, 0} {
		res := allow(t, l, "203.0.113.7")
		if !res.Allowed || res.Remaining != wantRemaining {
			t.Fatalf("%d. istek = %+v, izin ve %d kalan hak bekleniyordu", i+1, res, wantRemaining)
		}
		clock.Advance(10 * time.Second) // istekler 0, 10, 20. saniyelerde
	}

	// 30. saniyede 4. istek reddedilir. En eski istek 0. saniyede yapıldı, onun
	// pencereden çıkmasına 30 saniye var.
	res := allow(t, l, "203.0.113.7")
	if res.Allowed || res.RetryAfter != 30*time.Second {
		t.Fatalf("limit aşımı = %+v, 30 sn sonra tekrar deneme bekleniyordu", res)
	}

	// Başka bir anahtar etkilenmez.
	if res := allow(t, l, "198.51.100.1"); !res.Allowed {
		t.Error("farklı IP'nin hakkı ayrı olmalı")
	}

	// Kayan pencere: 60. saniyede sadece en eski istek pencereden çıkar, bir hak açılır.
	clock.Advance(30 * time.Second)
	if res := allow(t, l, "203.0.113.7"); !res.Allowed || res.Remaining != 0 {
		t.Fatalf("pencere kayınca bir hak açılmalıydı: %+v", res)
	}
	if res := allow(t, l, "203.0.113.7"); res.Allowed {
		t.Fatal("sadece bir hak açılmalıydı: sabit pencerede olduğu gibi hepsi sıfırlanmamalı")
	}

	// Reddedilen istekler hak tüketmez: aksi halde sürekli deneyen biri kendini
	// sonsuza kadar kilitlerdi.
	if n := rdb.ZCard(context.Background(), "agora:ratelimit:test:203.0.113.7").Val(); n != 3 {
		t.Errorf("pencerede %d istek var, 3 olmalı", n)
	}
}

func TestKeyExpires(t *testing.T) {
	rdb := redistest.New(t)
	l := New(rdb, "test", 5, time.Minute, newClock().Now)
	allow(t, l, "203.0.113.7")

	ttl := rdb.PTTL(context.Background(), "agora:ratelimit:test:203.0.113.7").Val()
	if ttl <= 0 || ttl > time.Minute {
		t.Errorf("anahtarın süresi = %v, pencere kadar olmalı: aksi halde Redis'te birikir", ttl)
	}
}

// TestConcurrentRequests, aynı anahtara eşzamanlı gelen isteklerin limiti aşmadığını
// doğrular. "Say, sonra ekle" iki ayrı komut olsaydı birçok istek aynı sayıyı görüp
// birlikte geçebilirdi. Lua betiği bu iki adımı bölünmez yapıyor.
func TestConcurrentRequests(t *testing.T) {
	rdb := redistest.New(t)
	l := New(rdb, "test", 10, time.Minute, time.Now)

	var (
		wg      sync.WaitGroup
		allowed atomic.Int32
		start   = make(chan struct{})
	)
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			res, err := l.Allow(context.Background(), "203.0.113.7")
			if err != nil {
				t.Error(err)
				return
			}
			if res.Allowed {
				allowed.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()

	if got := allowed.Load(); got != 10 {
		t.Errorf("%d istek geçti, tam olarak 10 olmalıydı", got)
	}
}

func TestByIP(t *testing.T) {
	rdb := redistest.New(t)
	l := New(rdb, "login", 1, time.Minute, newClock().Now)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	var calls int
	h := l.ByIP(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
	}))

	do := func(remoteAddr string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		req.RemoteAddr = remoteAddr
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	if rec := do("203.0.113.7:1000"); rec.Code != http.StatusOK || calls != 1 {
		t.Fatalf("ilk istek geçmeliydi: %d", rec.Code)
	}

	rec := do("203.0.113.7:2000") // aynı IP, farklı port
	if rec.Code != http.StatusTooManyRequests || calls != 1 {
		t.Fatalf("ikinci istek reddedilmeliydi: %d", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "60" {
		t.Errorf("Retry-After = %q, want 60", got)
	}
	if !strings.Contains(rec.Body.String(), `"code":"RATE_LIMITED"`) {
		t.Errorf("gövde = %s", rec.Body.String())
	}

	// Aynı /64 ağındaki IPv6 adresleri tek anahtar sayılır.
	if rec := do("[2001:db8:1:2::1]:1000"); rec.Code != http.StatusOK {
		t.Fatalf("ilk IPv6 isteği geçmeliydi: %d", rec.Code)
	}
	if rec := do("[2001:db8:1:2:ffff::9]:1000"); rec.Code != http.StatusTooManyRequests {
		t.Errorf("aynı /64'ten farklı adres sınırı aşamamalı: %d", rec.Code)
	}
	if rec := do("[2001:db8:1:3::1]:1000"); rec.Code != http.StatusOK {
		t.Errorf("farklı /64 ağı etkilenmemeli: %d", rec.Code)
	}
}

// TestByIPFailsOpen, Redis'e ulaşılamadığında isteğin geçirildiğini ve hatanın
// log'a yazıldığını doğrular.
func TestByIPFailsOpen(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1, DialTimeout: 100 * time.Millisecond})
	defer rdb.Close()

	var logs bytes.Buffer
	l := New(rdb, "login", 1, time.Minute, time.Now)
	h := l.ByIP(slog.New(slog.NewTextHandler(&logs, nil)))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))

	if rec.Code != http.StatusNoContent {
		t.Errorf("Redis yokken istek geçirilmeliydi: %d", rec.Code)
	}
	if !strings.Contains(logs.String(), "hız sınırı uygulanamadı") {
		t.Errorf("hata log'a yazılmadı:\n%s", logs.String())
	}
}
