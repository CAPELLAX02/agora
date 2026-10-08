// Package redistest, entegrasyon testleri için gerçek ve geçici bir Redis sağlar.
package redistest

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	tclog "github.com/testcontainers/testcontainers-go/log"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

// Image, testlerde kullanılan Redis imajıdır. compose.yaml ile aynı sürüm olmalı.
const Image = "redis:8-alpine"

// New, Docker'da geçici bir Redis başlatır ve ona bağlı bir istemci döndürür.
// Konteyner ve istemci test bitince otomatik kapatılır. "go test -short" ile
// çalıştırıldığında test atlanır.
func New(t *testing.T) *redis.Client {
	t.Helper()

	if testing.Short() {
		t.Skip("entegrasyon testi: -short modunda atlanıyor")
	}

	ctx := context.Background()

	ctr, err := tcredis.Run(ctx, Image, testcontainers.WithLogger(tclog.TestLogger(t)))
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatalf("redistest: Redis konteyneri başlatılamadı: %v", err)
	}

	url, err := ctr.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("redistest: bağlantı adresi alınamadı: %v", err)
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		t.Fatalf("redistest: %v", err)
	}

	rdb := redis.NewClient(opts)
	t.Cleanup(func() { _ = rdb.Close() })

	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatalf("redistest: Redis'e ulaşılamadı: %v", err)
	}
	return rdb
}
