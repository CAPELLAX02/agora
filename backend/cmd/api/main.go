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

	"github.com/CAPELLAX02/agora/backend/internal/platform/config"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
	"github.com/CAPELLAX02/agora/backend/internal/platform/logging"
	"github.com/jackc/pgx/v5/pgxpool"
)

const version = "0.1.0"

type application struct {
	cfg       config.Config
	logger    *slog.Logger
	db        *pgxpool.Pool
	checks    map[string]pinger
	version   string
	startedAt time.Time
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

	pool, err := db.Open(
		ctx,
		db.Options{
			URL:             cfg.DatabaseURL,
			MaxConns:        int32(cfg.DBMaxConns),
			MinConns:        int32(cfg.DBMinConns),
			MaxConnLifetime: cfg.DBMaxConnLifetime,
			MaxConnIdleTime: cfg.DBMaxConnIdleTime,
		},
	)
	if err != nil {
		return err
	}
	defer pool.Close()
	logger.Info("veritabanı bağlantı havuzu hazır", "max_conns", cfg.DBMaxConns)

	app := &application{
		cfg:       cfg,
		logger:    logger,
		db:        pool,
		checks:    map[string]pinger{"postgres": pool},
		version:   version,
		startedAt: time.Now(),
	}

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           app.routes(),
		ReadHeaderTimeout: cfg.ReadTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	serverErr := make(chan error, 1)

	go func() {
		logger.Info("HTTP sunucusu başlatılıyor",
			"addr", cfg.HTTPAddr, "env", cfg.Env, "version", version)

		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		return fmt.Errorf("http sunucusu: %w", err)
	case <-ctx.Done():
		logger.Info("kapanma sinyali alındı, devam eden istekler tamamlanıyor",
			"timeout", cfg.ShutdownTimeout.String())
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}

	logger.Info("sunucu düzgün şekilde kapandı")
	return nil
}
