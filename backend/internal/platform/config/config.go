// Package config, uygulama yapılandırmasını ortam değişkenlerinden okur ve doğrular.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"time"
)

// devDatabaseURL, sadece development ortamında, AGORA_DATABASE_URL verilmediğinde kullanılır.
// Kökteki compose.yaml ile birebir uyumludur.
const devDatabaseURL = "postgres://agora:agora_dev_password@localhost:5432/agora?sslmode=disable"

// Config, API sürecinin çalışma zamanı yapılandırmasıdır.
type Config struct {
	Env               string // development, test, production
	HTTPAddr          string
	LogLevel          string // debug, info, warn, error
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	DatabaseURL       string // gizli: parola içerir, log'a asla düz yazılmaz
	DBMaxConns        int
	DBMinConns        int
	DBMaxConnLifetime time.Duration
	DBMaxConnIdleTime time.Duration
}

// Load, ortam değişkenlerini okur, varsayılanları uygular ve sonucu doğrular.
// Tüm hatalar tek seferde raporlanır.
func Load() (Config, error) {
	var errs []error

	duration := func(key string, def time.Duration) time.Duration {
		d, err := lookupDuration(key, def)
		if err != nil {
			errs = append(errs, err)
		}
		return d
	}

	integer := func(key string, def int) int {
		n, err := lookupInt(key, def)
		if err != nil {
			errs = append(errs, err)
		}
		return n
	}

	cfg := Config{
		Env:               lookup("AGORA_ENV", "development"),
		HTTPAddr:          lookup("AGORA_HTTP_ADDR", ":8080"),
		LogLevel:          lookup("AGORA_LOG_LEVEL", "info"),
		ReadTimeout:       duration("AGORA_HTTP_READ_TIMEOUT", 5*time.Second),
		WriteTimeout:      duration("AGORA_HTTP_WRITE_TIMEOUT", 10*time.Second),
		IdleTimeout:       duration("AGORA_HTTP_IDLE_TIMEOUT", 60*time.Second),
		ShutdownTimeout:   duration("AGORA_SHUTDOWN_TIMEOUT", 15*time.Second),
		DatabaseURL:       lookup("AGORA_DATABASE_URL", ""),
		DBMaxConns:        integer("AGORA_DB_MAX_CONNS", 10),
		DBMinConns:        integer("AGORA_DB_MIN_CONNS", 2),
		DBMaxConnLifetime: duration("AGORA_DB_MAX_CONN_LIFETIME", time.Hour),
		DBMaxConnIdleTime: duration("AGORA_DB_MAX_CONN_IDLE_TIME", 30*time.Minute),
	}

	if cfg.DatabaseURL == "" && cfg.Env == "development" {
		cfg.DatabaseURL = devDatabaseURL
	}

	errs = append(errs, cfg.validate()...)

	if err := errors.Join(errs...); err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}

	return cfg, nil
}

// IsProduction, uygulamanın production ortamında çalışıp çalışmadığını söyler.
func (c Config) IsProduction() bool {
	return c.Env == "production"
}

// LogValue, Config'in log'a güvenle yazılabilir halini döndürür (slog.LogValuer).
// Sadece burada listelenen alanlar log'a girer, gizli bilgiler maskelenir.
func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("env", c.Env),
		slog.String("http_addr", c.HTTPAddr),
		slog.String("log_level", c.LogLevel),
		slog.String("read_timeout", c.ReadTimeout.String()),
		slog.String("write_timeout", c.WriteTimeout.String()),
		slog.String("idle_timeout", c.IdleTimeout.String()),
		slog.String("shutdown_timeout", c.ShutdownTimeout.String()),
		slog.String("database_url", redactURL(c.DatabaseURL)),
		slog.Int("db_max_conns", c.DBMaxConns),
		slog.Int("db_min_conns", c.DBMinConns),
		slog.String("db_max_conn_lifetime", c.DBMaxConnLifetime.String()),
		slog.String("db_max_conn_idle_time", c.DBMaxConnIdleTime.String()),
	)
}

func (c Config) validate() []error {
	var errs []error

	switch c.Env {
	case "development", "test", "production":
	default:
		errs = append(errs, fmt.Errorf("AGORA_ENV geçersiz: %q", c.Env))
	}

	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		errs = append(errs, fmt.Errorf("AGORA_LOG_LEVEL geçersiz: %q", c.LogLevel))
	}

	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("AGORA_DATABASE_URL zorunlu"))
	}

	if c.DBMaxConns < 1 {
		errs = append(errs, fmt.Errorf("AGORA_DB_MAX_CONNS en az 1 olmalı: %d", c.DBMaxConns))
	}
	if c.DBMinConns < 0 || c.DBMinConns > c.DBMaxConns {
		errs = append(errs, fmt.Errorf("AGORA_DB_MIN_CONNS 0 ile AGORA_DB_MAX_CONNS (%d) arasında olmalı: %d",
			c.DBMaxConns, c.DBMinConns))
	}

	return errs
}

func lookup(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func lookupDuration(key string, def time.Duration) (time.Duration, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s geçersiz süre %q: %w", key, v, err)
	}
	return d, nil
}

func lookupInt(key string, def int) (int, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s geçersiz tamsayı %q: %w", key, v, err)
	}
	return n, nil
}

// redactURL, bağlantı adresindeki parolayı maskeler.
func redactURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "<çözümlenemeyen-url>"
	}
	return u.Redacted()
}
