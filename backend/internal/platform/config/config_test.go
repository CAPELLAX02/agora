package config

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// clearAgoraEnv, testin geliştiricinin kabuğundaki AGORA_* değişkenlerinden etkilenmemesini sağlar.
func clearAgoraEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"AGORA_ENV", "AGORA_HTTP_ADDR", "AGORA_LOG_LEVEL",
		"AGORA_HTTP_READ_TIMEOUT", "AGORA_HTTP_WRITE_TIMEOUT",
		"AGORA_HTTP_IDLE_TIMEOUT", "AGORA_SHUTDOWN_TIMEOUT",
		"AGORA_DATABASE_URL", "AGORA_DB_MAX_CONNS", "AGORA_DB_MIN_CONNS",
		"AGORA_DB_MAX_CONN_LIFETIME", "AGORA_DB_MAX_CONN_IDLE_TIME",
	} {
		t.Setenv(key, "")
	}
}

// defaultsWith, varsayılan yapılandırmayı döndürür. modify verilirse önce onu uygular.
func defaultsWith(modify func(c *Config)) Config {
	c := Config{
		Env:             "development",
		HTTPAddr:        ":8080",
		LogLevel:        "info",
		ReadTimeout:     5 * time.Second,
		WriteTimeout:    10 * time.Second,
		IdleTimeout:     60 * time.Second,
		ShutdownTimeout: 15 * time.Second,

		DatabaseURL:       devDatabaseURL,
		DBMaxConns:        10,
		DBMinConns:        2,
		DBMaxConnLifetime: time.Hour,
		DBMaxConnIdleTime: 30 * time.Minute,
	}
	if modify != nil {
		modify(&c)
	}
	return c
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    Config   // sadece hata beklenmiyorsa karşılaştırılır
		wantErr []string // doluysa hata beklenir ve mesaj bu parçaların hepsini içermeli
	}{
		{
			name: "varsayılanlar",
			want: defaultsWith(nil),
		},
		{
			name: "özel değerler",
			env: map[string]string{
				"AGORA_ENV":              "production",
				"AGORA_HTTP_ADDR":        ":9090",
				"AGORA_LOG_LEVEL":        "debug",
				"AGORA_SHUTDOWN_TIMEOUT": "30s",
				"AGORA_DATABASE_URL":     "postgres://u:p@db:5432/agora",
				"AGORA_DB_MAX_CONNS":     "25",
			},
			want: defaultsWith(func(c *Config) {
				c.Env = "production"
				c.HTTPAddr = ":9090"
				c.LogLevel = "debug"
				c.ShutdownTimeout = 30 * time.Second
				c.DatabaseURL = "postgres://u:p@db:5432/agora"
				c.DBMaxConns = 25
			}),
		},
		{
			name:    "production'da veritabanı adresi zorunlu",
			env:     map[string]string{"AGORA_ENV": "production"},
			wantErr: []string{"AGORA_DATABASE_URL zorunlu"},
		},
		{
			name:    "geçersiz tamsayı",
			env:     map[string]string{"AGORA_DB_MAX_CONNS": "on"},
			wantErr: []string{"AGORA_DB_MAX_CONNS geçersiz tamsayı"},
		},
		{
			name:    "min bağlantı max'tan büyük olamaz",
			env:     map[string]string{"AGORA_DB_MAX_CONNS": "5", "AGORA_DB_MIN_CONNS": "8"},
			wantErr: []string{"AGORA_DB_MIN_CONNS 0 ile AGORA_DB_MAX_CONNS (5) arasında olmalı"},
		},
		{
			name:    "geçersiz ortam",
			env:     map[string]string{"AGORA_ENV": "prod"},
			wantErr: []string{`AGORA_ENV geçersiz: "prod"`},
		},
		{
			name:    "geçersiz süre",
			env:     map[string]string{"AGORA_HTTP_READ_TIMEOUT": "abc"},
			wantErr: []string{"AGORA_HTTP_READ_TIMEOUT geçersiz süre"},
		},
		{
			name: "tüm hatalar birlikte raporlanır",
			env: map[string]string{
				"AGORA_ENV":              "prod",
				"AGORA_LOG_LEVEL":        "verbose",
				"AGORA_SHUTDOWN_TIMEOUT": "abc",
			},
			wantErr: []string{"AGORA_ENV geçersiz", "AGORA_LOG_LEVEL geçersiz", "AGORA_SHUTDOWN_TIMEOUT geçersiz süre"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearAgoraEnv(t)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			got, err := Load()

			if len(tt.wantErr) > 0 {
				if err == nil {
					t.Fatalf("Load() hata döndürmedi, şunları içeren bir hata bekleniyordu: %q", tt.wantErr)
				}
				for _, part := range tt.wantErr {
					if !strings.Contains(err.Error(), part) {
						t.Errorf("Load() hatası %q içermiyor:\n%v", part, err)
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("Load() beklenmeyen hata: %v", err)
			}
			if got != tt.want {
				t.Errorf("Load() =\n  %+v\nwant\n  %+v", got, tt.want)
			}
		})
	}
}

func TestConfigLogValueRedactsSecrets(t *testing.T) {
	cfg := defaultsWith(func(c *Config) {
		c.DatabaseURL = "postgres://agora:s3cr3t-parola@db:5432/agora"
	})

	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, nil)).Info("yapılandırma", "config", cfg)
	out := buf.String()

	if strings.Contains(out, "s3cr3t-parola") {
		t.Errorf("parola log'a sızdı:\n%s", out)
	}
	if !strings.Contains(out, "config.database_url=postgres://agora:xxxxx@db:5432/agora") {
		t.Errorf("maskelenmiş adres log'da yok:\n%s", out)
	}
	if !strings.Contains(out, "config.env=development") {
		t.Errorf("diğer alanlar log'da yok:\n%s", out)
	}
}
