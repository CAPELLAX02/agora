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

// devRedisURL, sadece development ortamında, AGORA_REDIS_URL verilmediğinde kullanılır.
const devRedisURL = "redis://localhost:6379/0"

// Config, API sürecinin çalışma zamanı yapılandırmasıdır.
type Config struct {
	Env               string // development, test, production
	HTTPAddr          string
	MetricsAddr       string // /metrics ve pprof'un sunulduğu iç (admin) adres
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
	RedisURL          string // gizli: parola içerebilir, log'a asla düz yazılmaz

	// Kimlik doğrulama
	JWTPrivateKeyFile      string        // Ed25519 özel anahtarı (PEM). Development'ta boşsa geçici anahtar üretilir
	AccessTokenTTL         time.Duration // access token ömrü
	SessionIdleTimeout     time.Duration // bu süre refresh yapılmazsa oturum düşer
	SessionAbsoluteTimeout time.Duration // oturumun refresh'lerle bile aşamayacağı üst sınır
	PasswordHashWorkers    int           // aynı anda en fazla kaç parola hash'lenir (her biri 64 MiB)
	LoginRateLimit         int           // bir IP'den dakikada en fazla kaç giriş denemesi
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
		Env:                    lookup("AGORA_ENV", "development"),
		HTTPAddr:               lookup("AGORA_HTTP_ADDR", ":8080"),
		MetricsAddr:            lookup("AGORA_METRICS_ADDR", ":9091"),
		LogLevel:               lookup("AGORA_LOG_LEVEL", "info"),
		ReadTimeout:            duration("AGORA_HTTP_READ_TIMEOUT", 5*time.Second),
		WriteTimeout:           duration("AGORA_HTTP_WRITE_TIMEOUT", 10*time.Second),
		IdleTimeout:            duration("AGORA_HTTP_IDLE_TIMEOUT", 60*time.Second),
		ShutdownTimeout:        duration("AGORA_SHUTDOWN_TIMEOUT", 15*time.Second),
		DatabaseURL:            lookup("AGORA_DATABASE_URL", ""),
		DBMaxConns:             integer("AGORA_DB_MAX_CONNS", 10),
		DBMinConns:             integer("AGORA_DB_MIN_CONNS", 2),
		DBMaxConnLifetime:      duration("AGORA_DB_MAX_CONN_LIFETIME", time.Hour),
		DBMaxConnIdleTime:      duration("AGORA_DB_MAX_CONN_IDLE_TIME", 30*time.Minute),
		RedisURL:               lookup("AGORA_REDIS_URL", ""),
		JWTPrivateKeyFile:      lookup("AGORA_JWT_PRIVATE_KEY_FILE", ""),
		AccessTokenTTL:         duration("AGORA_ACCESS_TOKEN_TTL", 15*time.Minute),
		SessionIdleTimeout:     duration("AGORA_SESSION_IDLE_TIMEOUT", 2*time.Hour),
		SessionAbsoluteTimeout: duration("AGORA_SESSION_ABSOLUTE_TIMEOUT", 30*24*time.Hour),
		PasswordHashWorkers:    integer("AGORA_PASSWORD_HASH_WORKERS", 4),
		LoginRateLimit:         integer("AGORA_LOGIN_RATE_LIMIT", 20),
	}

	if cfg.DatabaseURL == "" && cfg.Env == "development" {
		cfg.DatabaseURL = devDatabaseURL
	}
	if cfg.RedisURL == "" && cfg.Env == "development" {
		cfg.RedisURL = devRedisURL
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

// IsDevelopment, uygulamanın yerel geliştirme ortamında çalışıp çalışmadığını söyler.
func (c Config) IsDevelopment() bool {
	return c.Env == "development"
}

// LogValue, Config'in log'a güvenle yazılabilir halini döndürür (slog.LogValuer).
// Sadece burada listelenen alanlar log'a girer, gizli bilgiler maskelenir.
func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("env", c.Env),
		slog.String("http_addr", c.HTTPAddr),
		slog.String("metrics_addr", c.MetricsAddr),
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
		slog.String("redis_url", redactURL(c.RedisURL)),
		slog.String("jwt_private_key_file", c.JWTPrivateKeyFile),
		slog.String("access_token_ttl", c.AccessTokenTTL.String()),
		slog.String("session_idle_timeout", c.SessionIdleTimeout.String()),
		slog.String("session_absolute_timeout", c.SessionAbsoluteTimeout.String()),
		slog.Int("password_hash_workers", c.PasswordHashWorkers),
		slog.Int("login_rate_limit", c.LoginRateLimit),
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

	if c.HTTPAddr == c.MetricsAddr {
		errs = append(errs, fmt.Errorf("AGORA_HTTP_ADDR ile AGORA_METRICS_ADDR aynı olamaz: %q", c.HTTPAddr))
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

	if c.RedisURL == "" {
		errs = append(errs, errors.New("AGORA_REDIS_URL zorunlu"))
	}

	if c.JWTPrivateKeyFile == "" && !c.IsDevelopment() {
		errs = append(errs, errors.New("AGORA_JWT_PRIVATE_KEY_FILE development dışında zorunlu"))
	}

	if c.AccessTokenTTL < time.Minute || c.AccessTokenTTL > time.Hour {
		errs = append(errs, fmt.Errorf("AGORA_ACCESS_TOKEN_TTL 1 dk ile 1 saat arasında olmalı: %s", c.AccessTokenTTL))
	}

	if c.SessionIdleTimeout <= c.AccessTokenTTL {
		errs = append(errs, fmt.Errorf("AGORA_SESSION_IDLE_TIMEOUT (%s) access token ömründen (%s) uzun olmalı",
			c.SessionIdleTimeout, c.AccessTokenTTL))
	}

	if c.SessionAbsoluteTimeout < c.SessionIdleTimeout {
		errs = append(errs, fmt.Errorf("AGORA_SESSION_ABSOLUTE_TIMEOUT (%s) boşta kalma süresinden (%s) kısa olamaz",
			c.SessionAbsoluteTimeout, c.SessionIdleTimeout))
	}

	if c.PasswordHashWorkers < 1 {
		errs = append(errs, fmt.Errorf("AGORA_PASSWORD_HASH_WORKERS en az 1 olmalı: %d", c.PasswordHashWorkers))
	}

	if c.LoginRateLimit < 1 {
		errs = append(errs, fmt.Errorf("AGORA_LOGIN_RATE_LIMIT en az 1 olmalı: %d", c.LoginRateLimit))
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
