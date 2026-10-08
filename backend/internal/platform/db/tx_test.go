package db_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
)

func TestInTx(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, `CREATE TABLE tx_test (code text PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	insert := func(code string) func(tx pgx.Tx) error {
		return func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `INSERT INTO tx_test (code) VALUES ($1)`, code)
			return err
		}
	}

	t.Run("başarılı fonksiyon commit edilir", func(t *testing.T) {
		if err := db.InTx(ctx, pool, insert("a")); err != nil {
			t.Fatal(err)
		}
		if !exists(t, pool, "a") {
			t.Error("commit edilen satır yok")
		}
	})

	t.Run("hata dönerse geri alınır", func(t *testing.T) {
		boom := errors.New("bilerek")
		err := db.InTx(ctx, pool, func(tx pgx.Tx) error {
			if err := insert("b")(tx); err != nil {
				return err
			}
			return boom
		})
		if !errors.Is(err, boom) {
			t.Fatalf("err = %v, want %v", err, boom)
		}
		if exists(t, pool, "b") {
			t.Error("hata dönen transaction'ın satırı kalmış")
		}
	})

	t.Run("panic olursa geri alınır ve panic yeniden fırlatılır", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Error("panic yutuldu, yeniden fırlatılmalıydı")
			}
			if exists(t, pool, "c") {
				t.Error("panic olan transaction'ın satırı kalmış")
			}
		}()
		_ = db.InTx(ctx, pool, func(tx pgx.Tx) error {
			_ = insert("c")(tx)
			panic("bilerek")
		})
	})

	t.Run("benzersizlik ihlali IsConflict ile tanınır", func(t *testing.T) {
		err := db.InTx(ctx, pool, insert("a"))
		if !db.IsConflict(err) {
			t.Errorf("IsConflict(%v) = false, want true", err)
		}
		if db.IsConflict(errors.New("başka hata")) {
			t.Error("rastgele hata çakışma sayıldı")
		}
	})
}

func exists(t *testing.T, pool *pgxpool.Pool, code string) bool {
	t.Helper()
	var ok bool
	if err := pool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM tx_test WHERE code = $1)`, code).Scan(&ok); err != nil {
		t.Fatal(err)
	}
	return ok
}

// TestTimestamptzIsUTC, veritabanından okunan zamanların sürecin saat diliminden
// bağımsız olarak UTC geldiğini doğrular.
func TestTimestamptzIsUTC(t *testing.T) {
	pool := dbtest.New(t)

	var now time.Time
	if err := pool.QueryRow(context.Background(), `SELECT now()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	if now.Location() != time.UTC {
		t.Errorf("zaman %s saat diliminde okundu, UTC olmalı", now.Location())
	}
}
