// Package config, uygulama yapılandırmasını ortam değişkenlerinden okur ve doğrular.
package config

import (
	"errors"
	"fmt"
	"os"
	"time"
)

// Config, API sürecinin çalışma zamanı yapılandırmasıdır.
type Config struct {
	Env             string // development, test, production
	HTTPAddr        string
	LogLevel        string // debug, info, warn, error
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
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

	cfg := Config{
		Env:             lookup("AGORA_ENV", "development"),
		HTTPAddr:        lookup("AGORA_HTTP_ADDR", ":8080"),
		LogLevel:        lookup("AGORA_LOG_LEVEL", "info"),
		ReadTimeout:     duration("AGORA_HTTP_READ_TIMEOUT", 5*time.Second),
		WriteTimeout:    duration("AGORA_HTTP_WRITE_TIMEOUT", 10*time.Second),
		IdleTimeout:     duration("AGORA_HTTP_IDLE_TIMEOUT", 60*time.Second),
		ShutdownTimeout: duration("AGORA_SHUTDOWN_TIMEOUT", 15*time.Second),
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
