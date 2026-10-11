// Package outbox, alan olaylarının transactional outbox'ıdır. İş kodu olayı Publish ile
// kendi transaction'ında yazar; Dispatcher (worker) olayları tüketicilere dağıtır.
//
// Teslim en az bir kezdir (at-least-once): worker bir olayı işlerken çökerse olay kira
// süresi dolunca yeniden dağıtılır. Tüketiciler bu yüzden idempotent olmalıdır; Dispatcher
// bunu kolaylaştırır: her tüketici olayı bir transaction içinde işler ve aynı transaction'da
// "işlendi" kaydı yazılır. Kayıt varsa olay o tüketiciye bir daha verilmez.
package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// Event, yayımlanan bir alan olayıdır.
type Event struct {
	ID            int64
	Type          string // ör. "registration.approved"
	AggregateType string // ör. "registration"
	AggregateID   string
	Payload       json.RawMessage
	OccurredAt    time.Time
	Attempts      int
}

// Decode, olayın verisini v'ye çözer.
func (e Event) Decode(v any) error {
	return json.Unmarshal(e.Payload, v)
}

// Publish, olayı outbox'a yazar. Çağıranın transaction'ı verilmelidir: olay iş verisiyle
// birlikte commit edilir ya da hiç yazılmaz.
func Publish(ctx context.Context, tx pgx.Tx, typ, aggregateType, aggregateID string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("outbox: olay verisi kodlanamadı: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO platform.outbox_events (event_type, aggregate_type, aggregate_id, payload)
		VALUES ($1, $2, $3, $4)`, typ, aggregateType, aggregateID, body); err != nil {
		return fmt.Errorf("outbox: olay yazılamadı: %w", err)
	}
	return nil
}

// Consumer, bir olay türünün tüketicisidir. Handle, olayı verilen transaction içinde işler;
// hata dönerse transaction geri alınır ve olay daha sonra yeniden denenir.
type Consumer interface {
	Name() string // tüketicinin kalıcı adı: "işlendi" kayıtlarında kullanılır
	Handles(eventType string) bool
	Handle(ctx context.Context, tx pgx.Tx, e Event) error
}

// Config, dağıtıcının ayarlarıdır.
type Config struct {
	BatchSize   int
	Lease       time.Duration // bir olayın işlenmesi için ayrılan süre
	MaxAttempts int
	BackoffBase time.Duration
	BackoffMax  time.Duration
}

// DefaultConfig, production için başlangıç değerleridir.
var DefaultConfig = Config{
	BatchSize:   50,
	Lease:       time.Minute,
	MaxAttempts: 10,
	BackoffBase: 10 * time.Second,
	BackoffMax:  30 * time.Minute,
}

// Sonuçlar (metrikler için).
const (
	ResultDone   = "done"
	ResultRetry  = "retry"
	ResultFailed = "failed"
)

// Dispatcher, bekleyen olayları tüketicilere dağıtır. Birden fazla worker aynı anda
// çalışabilir: olaylar SKIP LOCKED ile paylaştırılır.
type Dispatcher struct {
	pool      *pgxpool.Pool
	consumers []Consumer
	cfg       Config
	logger    *slog.Logger
	observe   func(result string)
}

// NewDispatcher, bir Dispatcher oluşturur. observe nil olabilir.
func NewDispatcher(pool *pgxpool.Pool, consumers []Consumer, cfg Config, logger *slog.Logger, observe func(string)) *Dispatcher {
	if observe == nil {
		observe = func(string) {}
	}
	return &Dispatcher{pool: pool, consumers: consumers, cfg: cfg, logger: logger, observe: observe}
}

// Run, ctx iptal edilene kadar her interval'de bekleyen olayları işler.
func (d *Dispatcher) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if _, err := d.ProcessBatch(ctx); err != nil && ctx.Err() == nil {
			d.logger.Error("outbox olayları işlenemedi", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// ProcessBatch, zamanı gelmiş en fazla BatchSize olayı işler ve işlenen sayısını döndürür.
func (d *Dispatcher) ProcessBatch(ctx context.Context) (int, error) {
	events, err := d.claim(ctx)
	if err != nil {
		return 0, err
	}
	for _, e := range events {
		d.process(ctx, e)
	}
	return len(events), nil
}

// claim, zamanı gelmiş olayları kiralar: başka worker'lar kira süresince onlara dokunmaz.
func (d *Dispatcher) claim(ctx context.Context) ([]Event, error) {
	rows, err := d.pool.Query(ctx, `
		UPDATE platform.outbox_events SET status = 'PROCESSING', locked_until = now() + make_interval(secs => $2),
			attempts = attempts + 1
		WHERE id IN (
			SELECT id FROM platform.outbox_events
			WHERE (status = 'PENDING' AND next_attempt_at <= now())
			   OR (status = 'PROCESSING' AND locked_until < now())
			ORDER BY id
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, event_type, aggregate_type, aggregate_id, payload, occurred_at, attempts`,
		d.cfg.BatchSize, d.cfg.Lease.Seconds())
	if err != nil {
		return nil, fmt.Errorf("outbox: olaylar kiralanamadı: %w", err)
	}
	events, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Event, error) {
		var e Event
		return e, row.Scan(&e.ID, &e.Type, &e.AggregateType, &e.AggregateID, &e.Payload, &e.OccurredAt, &e.Attempts)
	})
	if err != nil {
		return nil, fmt.Errorf("outbox: olaylar okunamadı: %w", err)
	}
	// Olaylar oluşma sırasıyla işlenir (aynı kaydın onayı ve geri alınması gibi).
	return events, nil
}

func (d *Dispatcher) process(ctx context.Context, e Event) {
	var failure error
	for _, c := range d.consumers {
		if !c.Handles(e.Type) {
			continue
		}
		err := db.InTx(ctx, d.pool, func(tx pgx.Tx) error {
			tag, err := tx.Exec(ctx, `
				INSERT INTO platform.outbox_consumptions (event_id, consumer) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
				e.ID, c.Name())
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				return nil // bu tüketici olayı daha önce işledi
			}
			return c.Handle(ctx, tx, e)
		})
		if err != nil {
			failure = errors.Join(failure, fmt.Errorf("%s: %w", c.Name(), err))
		}
	}

	if failure == nil {
		if _, err := d.pool.Exec(ctx, `
			UPDATE platform.outbox_events SET status = 'DONE', locked_until = NULL, processed_at = now(), last_error = NULL
			WHERE id = $1`, e.ID); err != nil {
			d.logger.Error("outbox olayı işlendi olarak işaretlenemedi", "event_id", e.ID, "err", err)
			return
		}
		d.observe(ResultDone)
		return
	}

	if e.Attempts >= d.cfg.MaxAttempts {
		d.logger.Error("outbox olayı kalıcı olarak başarısız", "event_id", e.ID, "type", e.Type, "err", failure)
		if _, err := d.pool.Exec(ctx, `
			UPDATE platform.outbox_events SET status = 'FAILED', locked_until = NULL, last_error = $2 WHERE id = $1`,
			e.ID, failure.Error()); err != nil {
			d.logger.Error("outbox olayı başarısız olarak işaretlenemedi", "event_id", e.ID, "err", err)
		}
		d.observe(ResultFailed)
		return
	}
	wait := d.backoff(e.Attempts)
	d.logger.Warn("outbox olayı yeniden denenecek", "event_id", e.ID, "type", e.Type, "attempt", e.Attempts, "in", wait, "err", failure)
	// Kira süresi dolmadan yeniden kuyruğa: süre saniye olarak verilir (Duration.String()
	// "µs" gibi PostgreSQL'in tanımadığı birimler üretebilir).
	if _, err := d.pool.Exec(ctx, `
		UPDATE platform.outbox_events SET status = 'PENDING', locked_until = NULL, last_error = $2,
			next_attempt_at = now() + make_interval(secs => $3)
		WHERE id = $1`, e.ID, failure.Error(), wait.Seconds()); err != nil {
		d.logger.Error("outbox olayı yeniden kuyruğa alınamadı", "event_id", e.ID, "err", err)
	}
	d.observe(ResultRetry)
}

// backoff, n. denemeden sonraki bekleme süresidir: üstel artış, üst sınır ve rastgele sapma.
func (d *Dispatcher) backoff(attempt int) time.Duration {
	wait := d.cfg.BackoffBase << min(attempt-1, 20)
	if wait <= 0 || wait > d.cfg.BackoffMax {
		wait = d.cfg.BackoffMax
	}
	return wait/2 + rand.N(wait/2+1)
}

// Lag, işlenmeyi bekleyen en eski olayın bekleme süresidir (alarm için).
func Lag(ctx context.Context, q db.Querier) (time.Duration, error) {
	var seconds float64
	err := q.QueryRow(ctx, `
		SELECT coalesce(extract(epoch FROM now() - min(occurred_at)), 0)::float8
		FROM platform.outbox_events WHERE status IN ('PENDING', 'PROCESSING')`).Scan(&seconds)
	if err != nil {
		return 0, fmt.Errorf("outbox: gecikme ölçülemedi: %w", err)
	}
	return time.Duration(seconds * float64(time.Second)), nil
}
