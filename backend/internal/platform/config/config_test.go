package config

import (
	"bytes"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"
)

// clearAgoraEnv, testin geliştiricinin kabuğundaki AGORA_* değişkenlerinden etkilenmemesini sağlar.
func clearAgoraEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"AGORA_ENV", "AGORA_HTTP_ADDR", "AGORA_METRICS_ADDR", "AGORA_LOG_LEVEL",
		"AGORA_HTTP_READ_TIMEOUT", "AGORA_HTTP_WRITE_TIMEOUT",
		"AGORA_HTTP_IDLE_TIMEOUT", "AGORA_SHUTDOWN_TIMEOUT",
		"AGORA_DATABASE_URL", "AGORA_DB_MAX_CONNS", "AGORA_DB_MIN_CONNS",
		"AGORA_DB_MAX_CONN_LIFETIME", "AGORA_DB_MAX_CONN_IDLE_TIME",
		"AGORA_JWT_PRIVATE_KEY_FILE", "AGORA_ACCESS_TOKEN_TTL", "AGORA_SESSION_IDLE_TIMEOUT",
		"AGORA_SESSION_ABSOLUTE_TIMEOUT", "AGORA_PASSWORD_HASH_WORKERS",
		"AGORA_REDIS_URL", "AGORA_LOGIN_RATE_LIMIT", "AGORA_CORS_ALLOWED_ORIGINS",
		"AGORA_WEB_BASE_URL", "AGORA_SMTP_ADDR", "AGORA_SMTP_FROM", "AGORA_SMTP_USERNAME",
		"AGORA_SMTP_PASSWORD", "AGORA_WORKER_METRICS_ADDR", "AGORA_MFA_ENCRYPTION_KEY",
	} {
		t.Setenv(key, "")
	}
}

// defaultsWith, varsayılan yapılandırmayı döndürür. modify verilirse önce onu uygular.
func defaultsWith(modify func(c *Config)) Config {
	c := Config{
		Env:             "development",
		HTTPAddr:        ":8080",
		MetricsAddr:     ":9091",
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
		RedisURL:          devRedisURL,
		CORSOrigins:       devCORSOrigins,

		AccessTokenTTL:         15 * time.Minute,
		SessionIdleTimeout:     2 * time.Hour,
		SessionAbsoluteTimeout: 30 * 24 * time.Hour,
		PasswordHashWorkers:    4,
		LoginRateLimit:         20,
		MFAKey:                 []byte("agora-dev-mfa-key-32-bytes-long!"),

		WebBaseURL:        devWebBaseURL,
		SMTPAddr:          devSMTPAddr,
		SMTPFrom:          "Agora <no-reply@agora.test>",
		WorkerMetricsAddr: ":9092",
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

				"AGORA_JWT_PRIVATE_KEY_FILE":     "/run/secrets/jwt.pem",
				"AGORA_ACCESS_TOKEN_TTL":         "10m",
				"AGORA_SESSION_IDLE_TIMEOUT":     "1h",
				"AGORA_SESSION_ABSOLUTE_TIMEOUT": "168h",
				"AGORA_PASSWORD_HASH_WORKERS":    "8",
				"AGORA_REDIS_URL":                "redis://:sifre@cache:6379/1",
				"AGORA_LOGIN_RATE_LIMIT":         "60",
				"AGORA_CORS_ALLOWED_ORIGINS":     " https://agora.example.edu.tr , https://yonetim.agora.example.edu.tr:8443,",
				"AGORA_WEB_BASE_URL":             "https://agora.example.edu.tr",
				"AGORA_SMTP_ADDR":                "smtp.example.edu.tr:587",
				"AGORA_SMTP_FROM":                "Agora <agora@example.edu.tr>",
				"AGORA_SMTP_USERNAME":            "agora",
				"AGORA_SMTP_PASSWORD":            "smtp-gizli",
				"AGORA_WORKER_METRICS_ADDR":      ":9100",
				"AGORA_MFA_ENCRYPTION_KEY":       "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=",
			},
			want: defaultsWith(func(c *Config) {
				c.Env = "production"
				c.HTTPAddr = ":9090"
				c.LogLevel = "debug"
				c.ShutdownTimeout = 30 * time.Second
				c.DatabaseURL = "postgres://u:p@db:5432/agora"
				c.DBMaxConns = 25

				c.JWTPrivateKeyFile = "/run/secrets/jwt.pem"
				c.AccessTokenTTL = 10 * time.Minute
				c.SessionIdleTimeout = time.Hour
				c.SessionAbsoluteTimeout = 7 * 24 * time.Hour
				c.PasswordHashWorkers = 8
				c.RedisURL = "redis://:sifre@cache:6379/1"
				c.LoginRateLimit = 60
				c.CORSOrigins = []string{"https://agora.example.edu.tr", "https://yonetim.agora.example.edu.tr:8443"}
				c.WebBaseURL = "https://agora.example.edu.tr"
				c.SMTPAddr = "smtp.example.edu.tr:587"
				c.SMTPFrom = "Agora <agora@example.edu.tr>"
				c.SMTPUsername = "agora"
				c.SMTPPassword = "smtp-gizli"
				c.WorkerMetricsAddr = ":9100"
				c.MFAKey = []byte("0123456789abcdef0123456789abcdef")
			}),
		},
		{
			name:    "development dışında JWT anahtarı zorunlu",
			env:     map[string]string{"AGORA_ENV": "test", "AGORA_DATABASE_URL": "postgres://u:p@db/agora"},
			wantErr: []string{"AGORA_JWT_PRIVATE_KEY_FILE development dışında zorunlu", "AGORA_MFA_ENCRYPTION_KEY development dışında zorunlu"},
		},
		{
			name:    "MFA anahtarı 32 bayt olmalı",
			env:     map[string]string{"AGORA_MFA_ENCRYPTION_KEY": "a2lzYS1hbmFodGFy"},
			wantErr: []string{"AGORA_MFA_ENCRYPTION_KEY 32 baytlık bir anahtarın base64 hali olmalı"},
		},
		{
			name:    "MFA anahtarı base64 olmalı",
			env:     map[string]string{"AGORA_MFA_ENCRYPTION_KEY": "base64 değil!"},
			wantErr: []string{"AGORA_MFA_ENCRYPTION_KEY 32 baytlık"},
		},
		{
			name:    "access token ömrü sınır dışında",
			env:     map[string]string{"AGORA_ACCESS_TOKEN_TTL": "2h", "AGORA_SESSION_IDLE_TIMEOUT": "3h"},
			wantErr: []string{"AGORA_ACCESS_TOKEN_TTL 1 dk ile 1 saat arasında olmalı"},
		},
		{
			name:    "boşta kalma süresi access token ömründen uzun olmalı",
			env:     map[string]string{"AGORA_SESSION_IDLE_TIMEOUT": "15m"},
			wantErr: []string{"AGORA_SESSION_IDLE_TIMEOUT (15m0s) access token ömründen (15m0s) uzun olmalı"},
		},
		{
			name:    "mutlak süre boşta kalma süresinden kısa olamaz",
			env:     map[string]string{"AGORA_SESSION_ABSOLUTE_TIMEOUT": "1h"},
			wantErr: []string{"AGORA_SESSION_ABSOLUTE_TIMEOUT (1h0m0s) boşta kalma süresinden (2h0m0s) kısa olamaz"},
		},
		{
			name:    "geçersiz CORS origin'leri",
			env:     map[string]string{"AGORA_CORS_ALLOWED_ORIGINS": "*,https://agora.test/,agora.test"},
			wantErr: []string{`geçersiz origin "*"`, `geçersiz origin "https://agora.test/"`, `geçersiz origin "agora.test"`},
		},
		{
			name:    "production'da e-posta ayarları zorunlu",
			env:     map[string]string{"AGORA_ENV": "production"},
			wantErr: []string{"AGORA_WEB_BASE_URL zorunlu", "AGORA_SMTP_ADDR zorunlu"},
		},
		{
			name: "geçersiz e-posta ayarları",
			env: map[string]string{
				"AGORA_WEB_BASE_URL":  "agora.test/giris",
				"AGORA_SMTP_FROM":     "adres değil",
				"AGORA_SMTP_USERNAME": "sadece-kullanici",
			},
			wantErr: []string{
				`AGORA_WEB_BASE_URL geçersiz "agora.test/giris"`,
				`AGORA_SMTP_FROM geçersiz "adres değil"`,
				"AGORA_SMTP_USERNAME ve AGORA_SMTP_PASSWORD birlikte verilmeli",
			},
		},
		{
			name:    "worker metrik adresi çakışmamalı",
			env:     map[string]string{"AGORA_WORKER_METRICS_ADDR": ":9091"},
			wantErr: []string{"AGORA_WORKER_METRICS_ADDR diğer adreslerle aynı olamaz"},
		},
		{
			name:    "giriş hız sınırı en az 1",
			env:     map[string]string{"AGORA_LOGIN_RATE_LIMIT": "0"},
			wantErr: []string{"AGORA_LOGIN_RATE_LIMIT en az 1 olmalı"},
		},
		{
			name:    "en az bir hash işçisi",
			env:     map[string]string{"AGORA_PASSWORD_HASH_WORKERS": "0"},
			wantErr: []string{"AGORA_PASSWORD_HASH_WORKERS en az 1 olmalı"},
		},
		{
			name:    "production'da veritabanı adresi zorunlu",
			env:     map[string]string{"AGORA_ENV": "production", "AGORA_JWT_PRIVATE_KEY_FILE": "/run/secrets/jwt.pem"},
			wantErr: []string{"AGORA_DATABASE_URL zorunlu", "AGORA_REDIS_URL zorunlu"},
		},
		{
			name:    "API ve metrik adresi aynı olamaz",
			env:     map[string]string{"AGORA_METRICS_ADDR": ":8080"},
			wantErr: []string{"AGORA_HTTP_ADDR ile AGORA_METRICS_ADDR aynı olamaz"},
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
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Load() =\n  %+v\nwant\n  %+v", got, tt.want)
			}
		})
	}
}

func TestConfigLogValueRedactsSecrets(t *testing.T) {
	cfg := defaultsWith(func(c *Config) {
		c.DatabaseURL = "postgres://agora:s3cr3t-parola@db:5432/agora"
		c.RedisURL = "redis://:r3d1s-parola@cache:6379/0"
		c.SMTPPassword = "smtp-c0k-gizli"
	})

	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, nil)).Info("yapılandırma", "config", cfg)
	out := buf.String()

	if strings.Contains(out, "s3cr3t-parola") || strings.Contains(out, "r3d1s-parola") ||
		strings.Contains(out, "smtp-c0k-gizli") {
		t.Errorf("parola log'a sızdı:\n%s", out)
	}
	if !strings.Contains(out, "config.database_url=postgres://agora:xxxxx@db:5432/agora") {
		t.Errorf("maskelenmiş adres log'da yok:\n%s", out)
	}
	if !strings.Contains(out, "config.env=development") {
		t.Errorf("diğer alanlar log'da yok:\n%s", out)
	}
}
