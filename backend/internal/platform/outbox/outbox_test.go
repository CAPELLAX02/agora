package outbox_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
	"github.com/CAPELLAX02/agora/backend/internal/platform/outbox"
)

type consumer struct {
	name     string
	calls    atomic.Int32
	failures int32 // ilk bu kadar çağrı hata verir
}

func (c *consumer) Name() string          { return c.name }
func (c *consumer) Handles(t string) bool { return t == "test.happened" }
func (c *consumer) Handle(ctx context.Context, tx pgx.Tx, e outbox.Event) error {
	n := c.calls.Add(1)
	var p struct{ N int }
	if err := e.Decode(&p); err != nil || p.N != 42 {
		return errors.New("veri çözülemedi")
	}
	if n <= c.failures {
		return errors.New("geçici hata")
	}
	return nil
}

func TestDispatch(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	publish := func() {
		err := db.InTx(ctx, pool, func(tx pgx.Tx) error {
			return outbox.Publish(ctx, tx, "test.happened", "test", "01928e3c-0000-7000-8000-000000000001", map[string]int{"n": 42})
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	// Transaction geri alınırsa olay da yazılmaz.
	_ = db.InTx(ctx, pool, func(tx pgx.Tx) error {
		_ = outbox.Publish(ctx, tx, "test.happened", "test", "01928e3c-0000-7000-8000-000000000001", nil)
		return errors.New("iş başarısız")
	})
	publish()

	ok, flaky := &consumer{name: "ok"}, &consumer{name: "flaky", failures: 1}
	cfg := outbox.Config{BatchSize: 10, Lease: time.Minute, MaxAttempts: 3, BackoffBase: time.Millisecond, BackoffMax: time.Millisecond}
	var results []string
	d := outbox.NewDispatcher(pool, []outbox.Consumer{ok, flaky}, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)),
		func(r string) { results = append(results, r) })

	if n, err := d.ProcessBatch(ctx); err != nil || n != 1 {
		t.Fatalf("ilk tur: %d, %v (geri alınan olay yazılmamalı)", n, err)
	}
	time.Sleep(20 * time.Millisecond) // yeniden deneme zamanı
	if n, err := d.ProcessBatch(ctx); err != nil || n != 1 {
		t.Fatalf("ikinci tur: %d, %v", n, err)
	}
	// Başarılı tüketici olayı ikinci turda yeniden işlemez.
	if ok.calls.Load() != 1 || flaky.calls.Load() != 2 {
		t.Errorf("çağrılar: ok=%d flaky=%d, 1 ve 2 olmalı", ok.calls.Load(), flaky.calls.Load())
	}
	if len(results) != 2 || results[0] != outbox.ResultRetry || results[1] != outbox.ResultDone {
		t.Errorf("sonuçlar = %v", results)
	}
	var status string
	_ = pool.QueryRow(ctx, `SELECT status FROM platform.outbox_events`).Scan(&status)
	if status != "DONE" {
		t.Errorf("durum = %s", status)
	}

	// Hep hata veren tüketici: deneme hakkı bitince kalıcı olarak başarısız.
	publish()
	broken := &consumer{name: "broken", failures: 100}
	d = outbox.NewDispatcher(pool, []outbox.Consumer{broken}, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	for range 3 {
		if _, err := d.ProcessBatch(ctx); err != nil {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = pool.QueryRow(ctx, `SELECT status FROM platform.outbox_events ORDER BY id DESC LIMIT 1`).Scan(&status)
	if status != "FAILED" || broken.calls.Load() != 3 {
		t.Errorf("kalıcı hata: durum %s, %d çağrı", status, broken.calls.Load())
	}
	if lag, err := outbox.Lag(ctx, pool); err != nil || lag != 0 {
		t.Errorf("bekleyen olay yokken gecikme = %v, %v", lag, err)
	}
}
