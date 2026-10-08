// Package redisx, Redis bağlantısını kurar.
package redisx

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Open, Redis istemcisini oluşturur ve sunucuya ulaşılabildiğini doğrular.
// İstemci kendi bağlantı havuzunu yönetir. Çağıran taraf işi bitince Close çağırmalıdır.
func Open(ctx context.Context, url string) (*redis.Client, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("redisx: bağlantı adresi çözümlenemedi: %w", err)
	}

	rdb := redis.NewClient(opts)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := rdb.Ping(pingCtx).Err(); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("redisx: Redis'e ulaşılamadı: %w", err)
	}
	return rdb, nil
}
