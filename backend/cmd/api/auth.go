package main

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/iam"
	"github.com/CAPELLAX02/agora/backend/internal/iam/password"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/config"
	"github.com/CAPELLAX02/agora/backend/internal/platform/jwt"
)

// Kimlik doğrulama politikası (docs/mimari/03-kimlik-dogrulama-ve-yetkilendirme.md).
const (
	tokenIssuer      = "agora"
	tokenAudience    = "agora-api"
	tokenLeeway      = 30 * time.Second // sunucular arası saat kaymasına tolerans
	reuseGracePeriod = 10 * time.Second
	maxFailedLogins  = 5
	lockoutBase      = time.Minute
	lockoutMax       = time.Hour
)

// newAuth, imza anahtarını yükler, kimlik doğrulama servisini ve korumalı uç
// noktalar için kullanılacak Authenticator'ı kurar.
func newAuth(ctx context.Context, cfg config.Config, pool *pgxpool.Pool, logger *slog.Logger) (*iam.Auth, *authn.Authenticator, error) {
	key, err := loadSigningKey(cfg, logger)
	if err != nil {
		return nil, nil, err
	}
	pub := key.Public().(ed25519.PublicKey)
	kid := jwt.KeyID(pub)

	signer, err := jwt.NewSigner(kid, key)
	if err != nil {
		return nil, nil, err
	}
	verifier := jwt.NewVerifier(tokenIssuer, tokenAudience, map[string]ed25519.PublicKey{kid: pub}, tokenLeeway)

	hasher := password.NewHasher(password.DefaultParams, cfg.PasswordHashWorkers)

	auth, err := iam.NewAuth(ctx, pool, hasher, signer, iam.AuthConfig{
		Issuer:            tokenIssuer,
		Audience:          tokenAudience,
		AccessTokenTTL:    cfg.AccessTokenTTL,
		IdleTimeout:       cfg.SessionIdleTimeout,
		AbsoluteTimeout:   cfg.SessionAbsoluteTimeout,
		ReuseGracePeriod:  reuseGracePeriod,
		MaxFailedAttempts: maxFailedLogins,
		LockoutBase:       lockoutBase,
		LockoutMax:        lockoutMax,
	}, time.Now)
	if err != nil {
		return nil, nil, err
	}

	logger.Info("kimlik doğrulama hazır", "kid", kid, "access_token_ttl", cfg.AccessTokenTTL.String())
	return auth, authn.New(verifier, time.Now), nil
}

// loadSigningKey, JWT imza anahtarını dosyadan okur. Dosya verilmemişse (config bunu
// sadece development'ta kabul eder) her açılışta geçici bir anahtar üretir.
func loadSigningKey(cfg config.Config, logger *slog.Logger) (ed25519.PrivateKey, error) {
	if cfg.JWTPrivateKeyFile == "" {
		_, key, err := ed25519.GenerateKey(nil) // nil: crypto/rand kullanılır
		if err != nil {
			return nil, fmt.Errorf("jwt imza anahtarı üretilemedi: %w", err)
		}
		logger.Warn("geçici JWT imza anahtarı üretildi: API yeniden başlayınca access token'lar " +
			"geçersiz olur, istemciler refresh ile yenisini alır")
		return key, nil
	}

	data, err := os.ReadFile(cfg.JWTPrivateKeyFile)
	if err != nil {
		return nil, fmt.Errorf("jwt imza anahtarı okunamadı: %w", err)
	}
	return jwt.ParsePrivateKeyPEM(data)
}
