// Package db, PostgreSQL bağlantı havuzunu kurar.
package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Options, bağlantı havuzu ayarlarıdır.
type Options struct {
	URL             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

// Open, bağlantı havuzunu oluşturur ve veritabanına ulaşılabildiğini doğrular.
// Çağıran taraf işi bitince pool.Close() çağırmalıdır.
func Open(ctx context.Context, opts Options) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(opts.URL)
	if err != nil {
		return nil, fmt.Errorf("db: bağlantı adresi çözümlenemedi: %w", err)
	}

	cfg.MaxConns = opts.MaxConns
	cfg.MinConns = opts.MinConns
	cfg.MaxConnLifetime = opts.MaxConnLifetime
	cfg.MaxConnIdleTime = opts.MaxConnIdleTime

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("db: havuz oluşturulamadı: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: veritabanına ulaşılamadı: %w", err)
	}

	return pool, nil
}
