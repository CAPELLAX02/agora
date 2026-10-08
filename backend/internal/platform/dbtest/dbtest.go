// Package dbtest, entegrasyon testleri için gerçek ve geçici bir PostgreSQL veritabanı sağlar.
package dbtest

import (
	"context"
	"database/sql"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	tclog "github.com/testcontainers/testcontainers-go/log"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/CAPELLAX02/agora/backend/migrations"
)

// Image, testlerde kullanılan PostgreSQL imajıdır. compose.yaml ile aynı sürüm olmalı.
const Image = "postgres:18-alpine"

// New, Docker'da geçici bir PostgreSQL başlatır, tüm migration'ları uygular ve
// bağlantı havuzunu döndürür. Konteyner ve havuz test bitince otomatik kapatılır.
// "go test -short" ile çalıştırıldığında test atlanır.
func New(t *testing.T) *pgxpool.Pool {
	t.Helper()

	if testing.Short() {
		t.Skip("entegrasyon testi: -short modunda atlanıyor")
	}

	ctx := context.Background()

	ctr, err := postgres.Run(ctx, Image,
		postgres.WithDatabase("agora_test"),
		postgres.WithUsername("agora"),
		postgres.WithPassword("agora_test_password"),
		postgres.BasicWaitStrategies(),
		// Konteyner log'ları t.Log'a gider: sadece -v ile ya da test başarısız olunca görünür.
		testcontainers.WithLogger(tclog.TestLogger(t)),
	)
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatalf("dbtest: PostgreSQL konteyneri başlatılamadı: %v", err)
	}

	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("dbtest: bağlantı adresi alınamadı: %v", err)
	}

	migrate(t, ctx, dsn)

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("dbtest: bağlantı havuzu oluşturulamadı: %v", err)
	}
	t.Cleanup(pool.Close)

	return pool
}

func migrate(t *testing.T, ctx context.Context, dsn string) {
	t.Helper()

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("dbtest: veritabanı açılamadı: %v", err)
	}
	defer db.Close()

	provider, err := migrations.NewProvider(db)
	if err != nil {
		t.Fatalf("dbtest: %v", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("dbtest: migration'lar uygulanamadı: %v", err)
	}
}
