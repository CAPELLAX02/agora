// Komut worker, API'den bağımsız çalışan arka plan işlerini yürütür: e-posta outbox'ı,
// denetim partition'larının bakımı ve süresi dolmuş kayıtların temizliği.
//
// API yatayda çoğaltıldığında zamanlanmış işler her kopyada çalışmasın diye ayrı bir
// süreçtir. Birden fazla worker da güvenle çalışabilir: outbox kayıtları
// SKIP LOCKED ile paylaştırılır, bakım işleri tekrar çalıştırılabilir.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/mail"
	"github.com/CAPELLAX02/agora/backend/internal/platform/config"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
	"github.com/CAPELLAX02/agora/backend/internal/platform/logging"
	"github.com/CAPELLAX02/agora/backend/internal/platform/metrics"
)

const (
	outboxInterval      = 2 * time.Second
	maintenanceInterval = 24 * time.Hour
	sentEmailRetention  = 30 * 24 * time.Hour
	resetTokenRetention = 7 * 24 * time.Hour
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "agora-worker: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger := logging.New(os.Stdout, cfg.Env, cfg.LogLevel).With("process", "worker")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, db.Options{
		URL:             cfg.DatabaseURL,
		MaxConns:        4,
		MaxConnLifetime: cfg.DBMaxConnLifetime,
		MaxConnIdleTime: cfg.DBMaxConnIdleTime,
	})
	if err != nil {
		return err
	}
	defer pool.Close()

	renderer, err := mail.NewRenderer()
	if err != nil {
		return err
	}
	sender := &mail.SMTPSender{
		Addr:     cfg.SMTPAddr,
		From:     cfg.SMTPFrom,
		Username: cfg.SMTPUsername,
		Password: cfg.SMTPPassword,
		Timeout:  30 * time.Second,
	}

	reg := metrics.NewRegistry()
	reg.MustRegister(metrics.NewPoolCollector(pool))
	emails := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "agora_outbox_emails_total",
		Help: "Outbox'tan işlenen e-postalar, sonuca göre (sent, retry, failed).",
	}, []string{"result"})
	reg.MustRegister(emails)
	reg.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "agora_outbox_lag_seconds",
		Help: "Gönderim zamanı gelmiş en eski bekleyen e-postanın bekleme süresi.",
	}, func() float64 {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		lag, err := mail.Lag(ctx, pool)
		if err != nil {
			logger.Warn("outbox gecikmesi ölçülemedi", "err", err)
			return 0
		}
		return lag.Seconds()
	}))

	dispatcher := mail.NewDispatcher(pool, renderer, sender, mail.DefaultDispatcherConfig, logger,
		func(result string) { emails.WithLabelValues(result).Inc() })

	admin := &http.Server{
		Addr:              cfg.WorkerMetricsAddr,
		Handler:           adminRoutes(reg),
		ReadHeaderTimeout: 5 * time.Second,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	var wg sync.WaitGroup
	serverErr := make(chan error, 1)

	wg.Go(func() {
		logger.Info("metrik sunucusu başlatılıyor", "addr", admin.Addr)
		if err := admin.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	})
	wg.Go(func() {
		logger.Info("e-posta outbox'ı işleniyor", "smtp", cfg.SMTPAddr, "interval", outboxInterval.String())
		dispatcher.Run(ctx, outboxInterval)
	})
	wg.Go(func() {
		every(ctx, maintenanceInterval, func(ctx context.Context) {
			maintain(ctx, pool, logger)
		})
	})

	select {
	case err = <-serverErr:
		err = fmt.Errorf("metrik sunucusu: %w", err)
		stop()
	case <-ctx.Done():
		logger.Info("kapanma sinyali alındı, işler tamamlanıyor")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if shutdownErr := admin.Shutdown(shutdownCtx); shutdownErr != nil {
		err = errors.Join(err, shutdownErr)
	}
	wg.Wait()

	if err == nil {
		logger.Info("worker düzgün şekilde kapandı")
	}
	return err
}

// every, fn'i hemen ve sonra her interval'de bir çalıştırır. ctx iptal edilince döner.
func every(ctx context.Context, interval time.Duration, fn func(context.Context)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		fn(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// maintain, günlük bakım işleridir. Bir adımın hatası diğerlerini durdurmaz.
func maintain(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) {
	if err := audit.EnsurePartitions(ctx, pool, 3); err != nil {
		logger.Error("denetim partition'ları açılamadı", "err", err)
	}

	// Gönderilmiş e-postaların gövdesi zaten boşaltılmış durumda. Sadece "gönderildi"
	// kaydı kalır ve bir ay sonra silinir.
	tag, err := pool.Exec(ctx, `
		DELETE FROM communication.email_outbox
		WHERE status = 'SENT' AND sent_at < now() - make_interval(secs => $1)`,
		sentEmailRetention.Seconds())
	if err != nil {
		logger.Error("eski e-posta kayıtları silinemedi", "err", err)
	} else if n := tag.RowsAffected(); n > 0 {
		logger.Info("eski e-posta kayıtları silindi", "count", n)
	}

	// Kullanılmış ya da süresi dolmuş bağlantılar bir hafta sonra silinir. Olayın
	// kendisi güvenlik olaylarında kalır.
	tag, err = pool.Exec(ctx, `
		DELETE FROM iam.password_reset_tokens
		WHERE created_at < now() - make_interval(secs => $1)`,
		resetTokenRetention.Seconds())
	if err != nil {
		logger.Error("eski sıfırlama bağlantıları silinemedi", "err", err)
	} else if n := tag.RowsAffected(); n > 0 {
		logger.Info("eski sıfırlama bağlantıları silindi", "count", n)
	}
}
