// Package ratelimit, Redis üzerinde kayan pencere (sliding window) ile hız sınırlaması yapar.
package ratelimit

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

// slidingWindow, son "window" milisaniyedeki istekleri bir sorted set'te tutar: her
// üye bir istektir, skoru isteğin zamanıdır. Pencerenin dışına düşenleri silip
// kalanları sayar ve limit dolmadıysa yeni isteği ekler.
//
// Betik Redis'te tek parça çalışır, araya başka bir komut giremez. Bu yüzden aynı
// anahtara eşzamanlı gelen istekler "say, sonra ekle" adımları arasında birbirini
// görmeden limiti aşamaz.
//
// Dönüş: {izin (1 ya da 0), kalan hak, yeni hak açılana kadar bekleme (ms)}
var slidingWindow = redis.NewScript(`
local key    = KEYS[1]
local now    = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local limit  = tonumber(ARGV[3])

redis.call('ZREMRANGEBYSCORE', key, '-inf', now - window)

local count = redis.call('ZCARD', key)
if count < limit then
  redis.call('ZADD', key, now, ARGV[4])
  redis.call('PEXPIRE', key, window)
  return {1, limit - count - 1, 0}
end

local oldest = redis.call('ZRANGE', key, 0, 0, 'WITHSCORES')
return {0, 0, tonumber(oldest[2]) + window - now}
`)

// Limiter, her anahtar (ör. IP adresi) için kayan bir pencerede en fazla limit
// kadar isteğe izin verir.
type Limiter struct {
	rdb      redis.Scripter
	name     string
	limit    int
	window   time.Duration
	now      func() time.Time
	onReject func(name string)
}

// Result, bir isteğin sınırlama kararıdır.
type Result struct {
	Allowed    bool
	Remaining  int
	RetryAfter time.Duration // izin verilmediyse yeni bir hakkın açılmasına kalan süre
}

// New, bir Limiter oluşturur. name, Redis anahtarlarını diğer sınırlayıcılardan ayırır.
func New(rdb redis.Scripter, name string, limit int, window time.Duration, now func() time.Time) *Limiter {
	return &Limiter{rdb: rdb, name: name, limit: limit, window: window, now: now}
}

// OnReject, bir istek reddedildiğinde çağrılacak fonksiyonu bağlar (metrikler için).
func (l *Limiter) OnReject(fn func(name string)) {
	l.onReject = fn
}

// Allow, key için bir istek hakkı tüketmeye çalışır.
func (l *Limiter) Allow(ctx context.Context, key string) (Result, error) {
	now := l.now().UnixMilli()
	member := strconv.FormatInt(now, 10) + "-" + rand.Text() // aynı milisaniyedeki istekler ayrı üye olsun

	res, err := slidingWindow.Run(ctx, l.rdb,
		[]string{"agora:ratelimit:" + l.name + ":" + key},
		now, l.window.Milliseconds(), l.limit, member,
	).Int64Slice()
	if err != nil {
		return Result{}, fmt.Errorf("ratelimit: %s: %w", l.name, err)
	}

	return Result{
		Allowed:    res[0] == 1,
		Remaining:  int(res[1]),
		RetryAfter: time.Duration(res[2]) * time.Millisecond,
	}, nil
}

// ByIP, istemci IP'si başına sınırlayan bir middleware döndürür. Limit aşılınca
// 429 ve Retry-After döner.
//
// Redis'e ulaşılamazsa istek geçirilir (fail open). Hız sınırı tek savunma hattı
// değil: girişte hesap kilitleme veritabanında çalışmaya devam eder. Redis
// kesintisinin bütün girişleri durdurması daha büyük bir zarar olurdu.
func (l *Limiter) ByIP(logger *slog.Logger) httpx.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			res, err := l.Allow(r.Context(), ipKey(r))
			if err != nil {
				logger.Error("hız sınırı uygulanamadı, istek geçiriliyor",
					"err", err, "request_id", httpx.RequestIDFrom(r.Context()))
				next.ServeHTTP(w, r)
				return
			}

			if !res.Allowed {
				if l.onReject != nil {
					l.onReject(l.name)
				}
				w.Header().Set("Retry-After", strconv.Itoa(ceilSeconds(res.RetryAfter)))
				_ = httpx.WriteProblem(w, r, httpx.Problem{
					Status: http.StatusTooManyRequests,
					Code:   "RATE_LIMITED",
					Detail: "Çok fazla istek gönderildi. Lütfen biraz sonra tekrar deneyin.",
				})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ipKey, IPv4 için adresin kendisini, IPv6 için adresin /64 ağını döndürür. Bir
// IPv6 kullanıcısının elinde genellikle koca bir /64 blok vardır: tek adrese
// konan sınır, her istekte adres değiştirerek kolayca aşılırdı.
func ipKey(r *http.Request) string {
	addr, err := netip.ParseAddr(httpx.ClientIP(r))
	if err != nil {
		return "unknown"
	}
	if addr.Is6() {
		prefix, _ := addr.Prefix(64)
		return prefix.String()
	}
	return addr.String()
}

func ceilSeconds(d time.Duration) int {
	return int((d + time.Second - 1) / time.Second)
}
