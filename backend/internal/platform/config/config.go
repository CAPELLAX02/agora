// Package config, uygulama yapılandırmasını ortam değişkenlerinden okur ve doğrular.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

// devDatabaseURL, sadece development ortamında, AGORA_DATABASE_URL verilmediğinde kullanılır.
// Kökteki compose.yaml ile birebir uyumludur.
const devDatabaseURL = "postgres://agora:agora_dev_password@localhost:5432/agora?sslmode=disable"

// devRedisURL, sadece development ortamında, AGORA_REDIS_URL verilmediğinde kullanılır.
const devRedisURL = "redis://localhost:6379/0"

// devCORSOrigins, development ortamında web geliştirme sunucusunun (Vite) adresidir.
var devCORSOrigins = []string{"http://localhost:5173"}

// Development ortamında e-postalar compose.yaml'daki Mailpit'e gider ve e-postalardaki
// bağlantılar Vite geliştirme sunucusunu gösterir.
const (
	devSMTPAddr   = "localhost:1025"
	devWebBaseURL = "http://localhost:5173"
)

// devMFAKey, sadece development ortamında, AGORA_MFA_ENCRYPTION_KEY verilmediğinde
// kullanılan TOTP sırrı şifreleme anahtarıdır. Herkesin bildiği bir anahtardır: geliştirme
// veritabanındaki sırlar korunmuş sayılmaz. Sabit olmasının sebebi, API yeniden
// başlatılınca daha önce kurulmuş MFA'ların açılamaz hale gelmemesi.
const devMFAKey = "YWdvcmEtZGV2LW1mYS1rZXktMzItYnl0ZXMtbG9uZyE="

// mfaKeySize, TOTP sırrı şifreleme anahtarının uzunluğudur (AES-256).
const mfaKeySize = 32

// Config, API ve worker süreçlerinin çalışma zamanı yapılandırmasıdır. İki süreç
// aynı ortam değişkenlerini okur: production'da aynı gizli değerlerle çalışırlar.
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
	RedisURL          string   // gizli: parola içerebilir, log'a asla düz yazılmaz
	CORSOrigins       []string // tarayıcıdan istek göndermesine izin verilen origin'ler

	// Kimlik doğrulama
	JWTPrivateKeyFile      string        // Ed25519 özel anahtarı (PEM). Development'ta boşsa geçici anahtar üretilir
	AccessTokenTTL         time.Duration // access token ömrü
	SessionIdleTimeout     time.Duration // bu süre refresh yapılmazsa oturum düşer
	SessionAbsoluteTimeout time.Duration // oturumun refresh'lerle bile aşamayacağı üst sınır
	PasswordHashWorkers    int           // aynı anda en fazla kaç parola hash'lenir (her biri 64 MiB)
	LoginRateLimit         int           // bir IP'den dakikada en fazla kaç giriş denemesi
	MFAKey                 []byte        // gizli: TOTP sırlarını şifreleyen 32 baytlık anahtar

	// E-posta ve worker
	WebBaseURL        string // e-postalardaki bağlantıların kökü (web arayüzünün adresi)
	SMTPAddr          string // host:port
	SMTPFrom          string // "Ad <adres>" biçiminde gönderen
	SMTPUsername      string
	SMTPPassword      string // gizli
	WorkerMetricsAddr string // worker'ın /metrics adresi
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

		WebBaseURL:        lookup("AGORA_WEB_BASE_URL", ""),
		SMTPAddr:          lookup("AGORA_SMTP_ADDR", ""),
		SMTPFrom:          lookup("AGORA_SMTP_FROM", "Agora <no-reply@agora.test>"),
		SMTPUsername:      lookup("AGORA_SMTP_USERNAME", ""),
		SMTPPassword:      lookup("AGORA_SMTP_PASSWORD", ""),
		WorkerMetricsAddr: lookup("AGORA_WORKER_METRICS_ADDR", ":9092"),
	}

	if cfg.DatabaseURL == "" && cfg.Env == "development" {
		cfg.DatabaseURL = devDatabaseURL
	}
	if cfg.RedisURL == "" && cfg.Env == "development" {
		cfg.RedisURL = devRedisURL
	}
	if cfg.Env == "development" {
		if cfg.SMTPAddr == "" {
			cfg.SMTPAddr = devSMTPAddr
		}
		if cfg.WebBaseURL == "" {
			cfg.WebBaseURL = devWebBaseURL
		}
	}
	mfaKey := lookup("AGORA_MFA_ENCRYPTION_KEY", "")
	if mfaKey == "" && cfg.Env == "development" {
		mfaKey = devMFAKey
	}
	if mfaKey != "" {
		key, err := base64.StdEncoding.DecodeString(mfaKey)
		if err != nil || len(key) != mfaKeySize {
			errs = append(errs, fmt.Errorf("AGORA_MFA_ENCRYPTION_KEY %d baytlık bir anahtarın base64 hali olmalı (openssl rand -base64 32)", mfaKeySize))
		} else {
			cfg.MFAKey = key
		}
	} else {
		errs = append(errs, errors.New("AGORA_MFA_ENCRYPTION_KEY development dışında zorunlu"))
	}

	cfg.CORSOrigins = lookupList("AGORA_CORS_ALLOWED_ORIGINS")
	if cfg.CORSOrigins == nil && cfg.Env == "development" {
		cfg.CORSOrigins = devCORSOrigins
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
		slog.String("cors_origins", strings.Join(c.CORSOrigins, ",")),
		slog.String("jwt_private_key_file", c.JWTPrivateKeyFile),
		slog.String("access_token_ttl", c.AccessTokenTTL.String()),
		slog.String("session_idle_timeout", c.SessionIdleTimeout.String()),
		slog.String("session_absolute_timeout", c.SessionAbsoluteTimeout.String()),
		slog.Int("password_hash_workers", c.PasswordHashWorkers),
		slog.Int("login_rate_limit", c.LoginRateLimit),
		slog.Bool("mfa_encryption_key_set", len(c.MFAKey) > 0),
		slog.String("web_base_url", c.WebBaseURL),
		slog.String("smtp_addr", c.SMTPAddr),
		slog.String("smtp_from", c.SMTPFrom),
		slog.String("smtp_username", c.SMTPUsername),
		slog.Bool("smtp_password_set", c.SMTPPassword != ""),
		slog.String("worker_metrics_addr", c.WorkerMetricsAddr),
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

	for _, o := range c.CORSOrigins {
		if !httpx.ValidOrigin(o) {
			errs = append(errs, fmt.Errorf("AGORA_CORS_ALLOWED_ORIGINS geçersiz origin %q: scheme://host[:port] biçiminde olmalı", o))
		}
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

	if c.WebBaseURL == "" {
		errs = append(errs, errors.New("AGORA_WEB_BASE_URL zorunlu"))
	} else if !httpx.ValidOrigin(c.WebBaseURL) {
		errs = append(errs, fmt.Errorf("AGORA_WEB_BASE_URL geçersiz %q: scheme://host[:port] biçiminde olmalı", c.WebBaseURL))
	}
	if c.SMTPAddr == "" {
		errs = append(errs, errors.New("AGORA_SMTP_ADDR zorunlu"))
	}
	if _, err := mail.ParseAddress(c.SMTPFrom); err != nil {
		errs = append(errs, fmt.Errorf("AGORA_SMTP_FROM geçersiz %q: %w", c.SMTPFrom, err))
	}
	if (c.SMTPUsername == "") != (c.SMTPPassword == "") {
		errs = append(errs, errors.New("AGORA_SMTP_USERNAME ve AGORA_SMTP_PASSWORD birlikte verilmeli"))
	}
	if c.WorkerMetricsAddr == c.HTTPAddr || c.WorkerMetricsAddr == c.MetricsAddr {
		errs = append(errs, fmt.Errorf("AGORA_WORKER_METRICS_ADDR diğer adreslerle aynı olamaz: %q", c.WorkerMetricsAddr))
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

// lookupList, virgülle ayrılmış bir listeyi okur. Değişken yoksa ya da boşsa nil döner.
func lookupList(key string) []string {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return nil
	}
	var out []string
	for item := range strings.SplitSeq(v, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
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
