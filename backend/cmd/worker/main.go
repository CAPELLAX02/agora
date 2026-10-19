// Komut worker, API'den bağımsız çalışan arka plan işlerini yürütür: e-posta outbox'ı, alan
// olaylarının dağıtımı (outbox), ders seçme penceresi biten taslakların serbest bırakılması,
// ileri tarihli duyuruların bildirimi, pencere ve ödev hatırlatmaları, denetim ve bildirim
// partition'larının bakımı, süresi dolmuş kayıtların temizliği ve dönem sonuçlarının günlük
// doğrulaması.
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

	"github.com/CAPELLAX02/agora/backend/internal/announce"
	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/enrollment"
	"github.com/CAPELLAX02/agora/backend/internal/files"
	"github.com/CAPELLAX02/agora/backend/internal/grading"
	"github.com/CAPELLAX02/agora/backend/internal/mail"
	"github.com/CAPELLAX02/agora/backend/internal/notify"
	"github.com/CAPELLAX02/agora/backend/internal/platform/config"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
	"github.com/CAPELLAX02/agora/backend/internal/platform/idempotency"
	"github.com/CAPELLAX02/agora/backend/internal/platform/logging"
	"github.com/CAPELLAX02/agora/backend/internal/platform/metrics"
	"github.com/CAPELLAX02/agora/backend/internal/platform/outbox"
	"github.com/CAPELLAX02/agora/backend/internal/registration"
	"github.com/CAPELLAX02/agora/backend/internal/reminders"
)

const (
	outboxInterval = 2 * time.Second
	eventsInterval = 2 * time.Second
	expireInterval = 10 * time.Minute
	// İleri tarihli duyurular yayın zamanı gelince bildirilir.
	announcementInterval = 15 * time.Second
	// Pencere ve ödev hatırlatmaları (24 saat kala, bir kez).
	reminderInterval    = 5 * time.Minute
	maintenanceInterval = 24 * time.Hour
	// Bağlanmayan dosyaların temizliği: saatte bir, bir günden eski olanlar.
	fileCleanupInterval     = time.Hour
	unattachedFileRetention = 24 * time.Hour
	sentEmailRetention      = 30 * 24 * time.Hour
	resetTokenRetention     = 7 * 24 * time.Hour
	// İdempotentlik anahtarları: istemcinin yeniden denemelerini kapsayan süre.
	idempotencyRetention = 24 * time.Hour
	// Bildirimler 180 gün saklanır (aylık partition'lar bütünüyle düşürülür).
	notificationRetentionDays = 180
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
	renderer.WebBaseURL = cfg.WebBaseURL
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

	// İleri tarihli duyuruların ve hatırlatmaların bildirimleri; yayımlama ve okuma API'de sayılır.
	notify.SetMetrics(metrics.NewCommunication(reg, notify.Types()))

	dispatcher := mail.NewDispatcher(pool, renderer, sender, mail.DefaultDispatcherConfig, logger,
		func(result string) { emails.WithLabelValues(result).Inc() })

	domainEvents := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "agora_domain_events_total",
		Help: "Outbox'tan dağıtılan alan olayları, sonuca göre (done, retry, failed).",
	}, []string{"result"})
	reg.MustRegister(domainEvents)
	reg.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "agora_domain_events_lag_seconds",
		Help: "İşlenmeyi bekleyen en eski alan olayının bekleme süresi.",
	}, func() float64 {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		lag, err := outbox.Lag(ctx, pool)
		if err != nil {
			logger.Warn("olay gecikmesi ölçülemedi", "err", err)
			return 0
		}
		return lag.Seconds()
	}))
	expiredDrafts := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "agora_registration_drafts_expired_total",
		Help: "Ders seçme penceresi bitince boşaltılan (kontenjanı serbest bırakılan) taslak sepetler.",
	})
	reg.MustRegister(expiredDrafts)
	termDrift := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "agora_term_results_drift",
		Help: "Son günlük doğrulamada dönem sonuçları ya da GANO önbelleği kesin kayıtlarla tutmayan öğrenci sayısı (0 olmalı).",
	})
	reg.MustRegister(termDrift)
	cleanedFiles := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "agora_files_cleaned_total",
		Help: "Bir kayda bağlanmadığı için silinen yüklenmiş dosyalar.",
	})
	reg.MustRegister(cleanedFiles)
	var fileService *files.Service
	if store, err := files.NewS3Store(files.S3Config{
		Endpoint: cfg.S3Endpoint, PublicEndpoint: cfg.S3PublicEndpoint, UseSSL: cfg.S3UseSSL, Region: cfg.S3Region,
		Bucket: cfg.S3Bucket, AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey,
	}); err != nil {
		logger.Error("dosya deposu yapılandırılamadı; temizlik çalışmayacak", "err", err)
	} else {
		fileService = files.NewService(pool, store, cfg.MaxUploadBytes, time.Now)
	}

	// Tüketiciler: dönem geçişinde yarıyıl ilerletmesi, not ilanında dönem sonuçları (YANO/GANO).
	// Bildirimler işlemle aynı transaction'da yazılır (internal/notify); LMS üyeliği kesin
	// kayıttan türetilir.
	events := outbox.NewDispatcher(pool, []outbox.Consumer{
		enrollment.SemesterAdvancer{},
		grading.TermResultsUpdater{},
	}, outbox.DefaultConfig, logger, func(result string) { domainEvents.WithLabelValues(result).Inc() })
	registrations := registration.NewService(pool, time.Now, nil)

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
		logger.Info("alan olayları dağıtılıyor", "interval", eventsInterval.String())
		events.Run(ctx, eventsInterval)
	})
	wg.Go(func() {
		every(ctx, expireInterval, func(ctx context.Context) {
			n, err := registrations.ExpireDrafts(ctx)
			if err != nil {
				logger.Error("süresi dolan taslaklar serbest bırakılamadı", "err", err)
				return
			}
			if n > 0 {
				expiredDrafts.Add(float64(n))
				logger.Info("ders seçme penceresi biten taslaklar boşaltıldı", "count", n)
			}
		})
	})
	wg.Go(func() {
		every(ctx, maintenanceInterval, func(ctx context.Context) {
			maintain(ctx, pool, logger)
		})
	})
	wg.Go(func() {
		every(ctx, reminderInterval, func(ctx context.Context) {
			r, err := reminders.Run(ctx, pool)
			if err != nil {
				if ctx.Err() == nil {
					logger.Error("hatırlatmalar gönderilemedi", "err", err)
				}
				return
			}
			if n := r.Opening + r.Closing + r.AssignmentDue; n > 0 {
				logger.Info("hatırlatmalar gönderildi", "opening", r.Opening, "closing", r.Closing, "assignment_due", r.AssignmentDue)
			}
		})
	})
	announcements := announce.NewService(pool, time.Now)
	wg.Go(func() {
		every(ctx, announcementInterval, func(ctx context.Context) {
			n, err := announcements.NotifyDue(ctx)
			if err != nil {
				if ctx.Err() == nil {
					logger.Error("yayını başlayan duyurular bildirilemedi", "err", err)
				}
				return
			}
			if n > 0 {
				logger.Info("yayını başlayan duyurular bildirildi", "count", n)
			}
		})
	})
	// Yüklenip bir kayda (ödev teslimi, ders içeriği) bağlanmayan dosyalar bir gün sonra silinir.
	if fileService != nil {
		wg.Go(func() {
			every(ctx, fileCleanupInterval, func(ctx context.Context) {
				n, err := fileService.CleanupUnattached(ctx, unattachedFileRetention, 500)
				if err != nil {
					if ctx.Err() == nil {
						logger.Error("bağlanmayan dosyalar temizlenemedi", "err", err)
					}
					return
				}
				if n > 0 {
					cleanedFiles.Add(float64(n))
					logger.Info("bağlanmayan dosyalar silindi", "count", n)
				}
			})
		})
	}
	// Dönem sonuçlarının doğrulaması: türetilmiş veri kesin kayıtlardan yeniden hesaplanır ve
	// karşılaştırılır; yazılmaz. Fark, not ilanı olayının işlenmediğini ya da kesin kayıtların
	// olay dışında değiştiğini gösterir: metrik alarmı tetikler, düzeltme `make term-results-recompute`.
	wg.Go(func() {
		every(ctx, maintenanceInterval, func(ctx context.Context) {
			r, err := grading.RecomputeAll(ctx, pool, grading.RecomputeOptions{Workers: 2, Batch: 500})
			if err != nil {
				if ctx.Err() == nil {
					logger.Error("dönem sonuçları doğrulanamadı", "err", err)
				}
				return
			}
			termDrift.Set(float64(r.Drifted))
			if r.Drifted > 0 {
				logger.Warn("dönem sonuçları kesin kayıtlarla tutmuyor", "students", r.Drifted, "examples", r.Examples)
				return
			}
			logger.Info("dönem sonuçları doğrulandı", "students", r.Students, "duration", r.Duration.String())
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
	if err := notify.EnsurePartitions(ctx, pool, 3); err != nil {
		logger.Error("bildirim partition'ları açılamadı", "err", err)
	}
	if n, err := notify.DropExpired(ctx, pool, notificationRetentionDays); err != nil {
		logger.Error("eski bildirim partition'ları düşürülemedi", "err", err)
	} else if n > 0 {
		logger.Info("eski bildirim partition'ları düşürüldü", "count", n)
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

	// İdempotentlik anahtarları bir gün saklanır: istemci yeniden denemelerini kapsar.
	if n, err := idempotency.Purge(ctx, pool, idempotencyRetention); err != nil {
		logger.Error("eski idempotentlik anahtarları silinemedi", "err", err)
	} else if n > 0 {
		logger.Info("eski idempotentlik anahtarları silindi", "count", n)
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
