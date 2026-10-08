package migrations_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"

	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
	"github.com/CAPELLAX02/agora/backend/migrations"
)

// TestRoundTrip, bütün migration'ların geri alınıp yeniden uygulanabildiğini
// doğrular. Down bölümü hatalı bir migration, üretimde geri dönüşü imkânsız kılar.
func TestRoundTrip(t *testing.T) {
	pool := dbtest.New(t) // bütün migration'lar uygulanmış olarak gelir
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	provider, err := migrations.NewProvider(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	if _, err := provider.DownTo(ctx, 0); err != nil {
		t.Fatalf("bütün migration'lar geri alınamadı: %v", err)
	}

	var tables int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.tables
		WHERE table_schema NOT IN ('public', 'pg_catalog', 'information_schema')`,
	).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 0 {
		t.Errorf("geri alma sonrası %d tablo kaldı", tables)
	}

	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("yeniden uygulanamadı: %v", err)
	}
}
