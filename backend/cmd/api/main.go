package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/iam"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/config"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
	"github.com/CAPELLAX02/agora/backend/internal/platform/logging"
	"github.com/CAPELLAX02/agora/backend/internal/platform/metrics"
	"github.com/CAPELLAX02/agora/backend/internal/platform/ratelimit"
	"github.com/CAPELLAX02/agora/backend/internal/platform/redisx"
)

const version = "0.1.0"

type application struct {
	cfg           config.Config
	logger        *slog.Logger
	db            *pgxpool.Pool
	metrics       *metrics.HTTP
	checks        map[string]pinger
	version       string
	startedAt     time.Time
	auth          *iam.Auth
	authenticator *authn.Authenticator
	limits        iam.Limits
	permissions   authz.Resolver
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "agora-api: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := logging.New(os.Stdout, cfg.Env, cfg.LogLevel)
	logger.Debug("yapılandırma yüklendi", "config", cfg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, db.Options{
		URL:             cfg.DatabaseURL,
		MaxConns:        int32(cfg.DBMaxConns),
		MinConns:        int32(cfg.DBMinConns),
		MaxConnLifetime: cfg.DBMaxConnLifetime,
		MaxConnIdleTime: cfg.DBMaxConnIdleTime,
	})
	if err != nil {
		return err
	}
	defer pool.Close()
	logger.Info("veritabanı bağlantı havuzu hazır", "max_conns", cfg.DBMaxConns)

	// Denetim tablolarının önümüzdeki aylar için partition'ları hazır olsun. Worker
	// bunu günlük olarak da yapar. Fonksiyon tekrar çalıştırılabilir.
	if err := audit.EnsurePartitions(ctx, pool, 3); err != nil {
		return err
	}

	rdb, err := redisx.Open(ctx, cfg.RedisURL)
	if err != nil {
		return err
	}
	defer rdb.Close()
	logger.Info("Redis bağlantısı hazır")

	auth, authenticator, err := newAuth(ctx, cfg, pool, rdb, logger)
	if err != nil {
		return err
	}
	loginLimiter := ratelimit.New(rdb, "login", cfg.LoginRateLimit, time.Minute, time.Now)
	resetLimiter := ratelimit.New(rdb, "password_reset", passwordResetLimit, time.Minute, time.Now)

	permissions := iam.NewPermissionResolver(pool, rdb, time.Hour, logger)

	reg := metrics.NewRegistry()
	reg.MustRegister(metrics.NewPoolCollector(pool))
	authMetrics := metrics.NewAuth(reg)
	auth.SetMetrics(authMetrics)
	loginLimiter.OnReject(authMetrics.RateLimited)
	resetLimiter.OnReject(authMetrics.RateLimited)

	app := &application{
		cfg:     cfg,
		logger:  logger,
		db:      pool,
		metrics: metrics.NewHTTP(reg),
		checks: map[string]pinger{
			"postgres": pool,
			"redis":    pingerFunc(func(ctx context.Context) error { return rdb.Ping(ctx).Err() }),
		},
		version:       version,
		startedAt:     time.Now(),
		auth:          auth,
		authenticator: authenticator,
		permissions:   permissions,
		limits: iam.Limits{
			Login:         loginLimiter.ByIP(logger),
			PasswordReset: resetLimiter.ByIP(logger),
		},
	}

	servers := []*http.Server{
		newServer(cfg.HTTPAddr, app.routes(), cfg, cfg.WriteTimeout, logger),
		// pprof'un CPU profili varsayılan olarak 30 saniye sürer, bu yüzden admin
		// sunucusunun yazma zaman aşımı API'ninkinden uzun tutulur.
		newServer(cfg.MetricsAddr, adminRoutes(reg), cfg, 2*time.Minute, logger),
	}

	serverErr := make(chan error, len(servers))
	for _, srv := range servers {
		go func() {
			logger.Info("HTTP sunucusu başlatılıyor",
				"addr", srv.Addr, "env", cfg.Env, "version", version)

			if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				serverErr <- fmt.Errorf("%s: %w", srv.Addr, err)
			}
		}()
	}

	select {
	case err := <-serverErr:
		return fmt.Errorf("http sunucusu: %w", err)
	case <-ctx.Done():
		logger.Info("kapanma sinyali alındı, devam eden istekler tamamlanıyor",
			"timeout", cfg.ShutdownTimeout.String())
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	var errs []error
	for _, srv := range servers {
		if err := srv.Shutdown(shutdownCtx); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", srv.Addr, err))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}

	logger.Info("sunucu düzgün şekilde kapandı")
	return nil
}

// newServer, zaman aşımları yapılandırılmış bir HTTP sunucusu oluşturur.
func newServer(addr string, h http.Handler, cfg config.Config, writeTimeout time.Duration, logger *slog.Logger) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: cfg.ReadTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}
}
