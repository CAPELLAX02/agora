package mail

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// Email, kuyruğa alınacak bir e-postadır.
type Email struct {
	To       string
	Template string
	Locale   string // boşsa "tr"
	Data     map[string]any
}

// Enqueue, e-postayı outbox'a yazar ve kaydın kimliğini döndürür. Çağıranın
// transaction'ı verilirse e-posta, iş verisiyle birlikte commit edilir.
func Enqueue(ctx context.Context, q db.Querier, e Email) (string, error) {
	if e.Locale == "" {
		e.Locale = locales[0]
	}
	payload, err := json.Marshal(e.Data)
	if err != nil {
		return "", fmt.Errorf("mail: şablon verisi kodlanamadı: %w", err)
	}

	var id string
	err = q.QueryRow(ctx, `
		INSERT INTO communication.email_outbox (to_address, template_code, locale, payload)
		VALUES ($1, $2, $3, $4)
		RETURNING id`,
		e.To, e.Template, e.Locale, payload,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("mail: e-posta kuyruğa alınamadı: %w", err)
	}
	return id, nil
}

// Sender, e-posta gönderen bileşendir. *SMTPSender bunu sağlar.
type Sender interface {
	Send(ctx context.Context, m Message) error
}

// Gönderim sonuçları (metrikler için).
const (
	ResultSent   = "sent"
	ResultRetry  = "retry"
	ResultFailed = "failed"
)

// DispatcherConfig, outbox işleyicisinin ayarlarıdır.
type DispatcherConfig struct {
	BatchSize   int
	Lease       time.Duration // bir kaydın gönderimi için ayrılan süre
	SendTimeout time.Duration // tek bir SMTP gönderiminin süresi; Lease'ten kısa olmalı
	MaxAttempts int
	BackoffBase time.Duration // ilk yeniden denemeden önceki bekleme
	BackoffMax  time.Duration
}

// DefaultDispatcherConfig, production için başlangıç değerleridir: 8 denemede
// (30 sn, 1 dk, 2 dk ... en çok 1 saat arayla) toplamda ~2 saat denenir.
var DefaultDispatcherConfig = DispatcherConfig{
	BatchSize:   20,
	Lease:       2 * time.Minute,
	SendTimeout: 30 * time.Second,
	MaxAttempts: 8,
	BackoffBase: 30 * time.Second,
	BackoffMax:  time.Hour,
}

// Dispatcher, outbox'taki e-postaları gönderir. Birden fazla worker aynı anda
// çalışabilir: kayıtlar FOR UPDATE SKIP LOCKED ile alındığı için iki worker aynı
// e-postayı almaz. Gönderirken çöken bir worker'ın aldığı kayıt, kira (lease)
// süresi dolunca başka bir worker tarafından yeniden denenir.
//
// Zaman karşılaştırmaları veritabanı saatiyle yapılır: kayıtların zamanı da
// veritabanında yazılıyor.
type Dispatcher struct {
	db       db.Querier
	renderer *Renderer
	sender   Sender
	cfg      DispatcherConfig
	logger   *slog.Logger
	observe  func(result string)
}

// NewDispatcher, bir Dispatcher oluşturur. observe, her gönderim sonucunda
// çağrılır (metrikler için). nil olabilir.
func NewDispatcher(q db.Querier, renderer *Renderer, sender Sender, cfg DispatcherConfig, logger *slog.Logger, observe func(string)) *Dispatcher {
	if observe == nil {
		observe = func(string) {}
	}
	return &Dispatcher{db: q, renderer: renderer, sender: sender, cfg: cfg, logger: logger, observe: observe}
}

type job struct {
	id       string
	to       string
	template string
	locale   string
	data     map[string]any
	attempts int
}

// RunOnce, gönderim zamanı gelmiş en fazla BatchSize e-postayı işler ve işlenen
// kayıt sayısını döndürür.
func (d *Dispatcher) RunOnce(ctx context.Context) (int, error) {
	jobs, err := d.claim(ctx)
	if err != nil {
		return 0, err
	}

	for _, j := range jobs {
		msg, err := d.renderer.Render(j.template, j.locale, j.to, j.data)
		if err != nil {
			// Şablon hatası kalıcıdır: yeniden denemek aynı sonucu verir.
			d.fail(ctx, j, err, true)
			continue
		}

		sendCtx, cancel := context.WithTimeout(ctx, d.cfg.SendTimeout)
		err = d.sender.Send(sendCtx, msg)
		cancel()
		if err != nil {
			d.fail(ctx, j, err, false)
			continue
		}
		d.markSent(ctx, j)
	}
	return len(jobs), nil
}

// Run, ctx iptal edilene kadar outbox'ı işler. Parti doluysa hemen devam eder,
// değilse interval kadar bekler.
func (d *Dispatcher) Run(ctx context.Context, interval time.Duration) {
	for {
		n, err := d.RunOnce(ctx)
		if err != nil && ctx.Err() == nil {
			d.logger.Error("e-posta outbox'ı işlenemedi", "err", err)
		}
		if n == d.cfg.BatchSize && err == nil {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
		}
	}
}

func (d *Dispatcher) claim(ctx context.Context) ([]job, error) {
	rows, err := d.db.Query(ctx, `
		WITH due AS (
			SELECT id FROM communication.email_outbox
			WHERE (status = 'PENDING' AND next_attempt_at <= now())
			   OR (status = 'SENDING' AND locked_until < now())
			ORDER BY next_attempt_at
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE communication.email_outbox o
		SET status = 'SENDING',
		    attempts = o.attempts + 1,
		    locked_until = now() + make_interval(secs => $2)
		FROM due
		WHERE o.id = due.id
		RETURNING o.id, o.to_address, o.template_code, o.locale, o.payload, o.attempts`,
		d.cfg.BatchSize, d.cfg.Lease.Seconds(),
	)
	if err != nil {
		return nil, fmt.Errorf("mail: outbox kayıtları alınamadı: %w", err)
	}
	jobs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (job, error) {
		var (
			j       job
			payload []byte
		)
		if err := row.Scan(&j.id, &j.to, &j.template, &j.locale, &payload, &j.attempts); err != nil {
			return j, err
		}
		return j, json.Unmarshal(payload, &j.data)
	})
	if err != nil {
		return nil, fmt.Errorf("mail: outbox kayıtları okunamadı: %w", err)
	}
	return jobs, nil
}

func (d *Dispatcher) markSent(ctx context.Context, j job) {
	_, err := d.db.Exec(ctx, `
		UPDATE communication.email_outbox
		SET status = 'SENT', sent_at = now(), locked_until = NULL, last_error = NULL, payload = '{}'
		WHERE id = $1`, j.id)
	if err != nil {
		// E-posta gitti ama işaretlenemedi: kira dolunca tekrar gönderilebilir
		// (en az bir kez teslim). Bunu log'da görmek önemli.
		d.logger.Error("gönderilen e-posta işaretlenemedi", "err", err, "outbox_id", j.id)
		return
	}
	d.observe(ResultSent)
}

func (d *Dispatcher) fail(ctx context.Context, j job, sendErr error, permanent bool) {
	msg := sendErr.Error()
	if len(msg) > 1000 {
		msg = msg[:1000]
	}

	if permanent || j.attempts >= d.cfg.MaxAttempts {
		_, err := d.db.Exec(ctx, `
			UPDATE communication.email_outbox
			SET status = 'FAILED', locked_until = NULL, last_error = $2, payload = '{}'
			WHERE id = $1`, j.id, msg)
		if err != nil {
			d.logger.Error("başarısız e-posta işaretlenemedi", "err", err, "outbox_id", j.id)
			return
		}
		d.logger.Error("e-posta kalıcı olarak gönderilemedi",
			"err", sendErr, "outbox_id", j.id, "template", j.template, "attempts", j.attempts)
		d.observe(ResultFailed)
		return
	}

	wait := backoff(j.attempts, d.cfg.BackoffBase, d.cfg.BackoffMax)
	_, err := d.db.Exec(ctx, `
		UPDATE communication.email_outbox
		SET status = 'PENDING', locked_until = NULL, last_error = $2,
		    next_attempt_at = now() + make_interval(secs => $3)
		WHERE id = $1`, j.id, msg, wait.Seconds())
	if err != nil {
		d.logger.Error("e-posta yeniden denemeye alınamadı", "err", err, "outbox_id", j.id)
		return
	}
	d.logger.Warn("e-posta gönderilemedi, yeniden denenecek",
		"err", sendErr, "outbox_id", j.id, "attempts", j.attempts, "retry_in", wait.String())
	d.observe(ResultRetry)
}

// backoff, attempt'inci başarısızlıktan sonraki bekleme süresidir: base, 2×base,
// 4×base ... en çok max. %20'ye kadar rastgele sapma (jitter) eklenir: aynı anda
// başarısız olan çok sayıda e-posta, SMTP sunucusuna aynı anda tekrar yüklenmesin.
func backoff(attempt int, base, max time.Duration) time.Duration {
	d := base
	for i := 1; i < attempt && d < max; i++ {
		d *= 2
	}
	d = min(d, max)
	return d + time.Duration(rand.Int64N(int64(d)/5+1))
}

// Lag, gönderim zamanı gelmiş en eski bekleyen e-postanın ne kadar beklediğidir.
// Bekleyen yoksa sıfır döner. Outbox'ın tıkandığını gösteren ana metriktir.
func Lag(ctx context.Context, q db.Querier) (time.Duration, error) {
	var seconds float64
	err := q.QueryRow(ctx, `
		SELECT coalesce(extract(epoch FROM now() - min(next_attempt_at)), 0)
		FROM communication.email_outbox
		WHERE status = 'PENDING' AND next_attempt_at <= now()`,
	).Scan(&seconds)
	if err != nil {
		return 0, fmt.Errorf("mail: outbox gecikmesi okunamadı: %w", err)
	}
	return time.Duration(seconds * float64(time.Second)), nil
}
