package iam

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RevocationList, iptal edilen oturumları Redis'te tutar.
//
// Access token doğrulaması veritabanına gitmez, bu yüzden iptal edilen bir oturumun
// elde kalan access token'ı süresi dolana kadar geçerli kalırdı. Kimlik doğrulama
// middleware'i her istekte bu listeye bakar ve iptal anında etkili olur.
//
// Kayıtlar access token ömrü kadar yaşar: bu sürenin sonunda oturumun tüm access
// token'larının süresi zaten dolmuştur, kaydı tutmaya gerek kalmaz.
type RevocationList struct {
	rdb redis.Cmdable
	ttl time.Duration
}

// NewRevocationList, bir iptal listesi oluşturur. ttl en az access token ömrü
// (ve saat kayması toleransı) kadar olmalıdır.
func NewRevocationList(rdb redis.Cmdable, ttl time.Duration) *RevocationList {
	return &RevocationList{rdb: rdb, ttl: ttl}
}

func revokedSessionKey(sessionID string) string {
	return "agora:revoked_sid:" + sessionID
}

// Revoke, oturumu iptal listesine ekler.
func (l *RevocationList) Revoke(ctx context.Context, sessionID string) error {
	if err := l.rdb.Set(ctx, revokedSessionKey(sessionID), 1, l.ttl).Err(); err != nil {
		return fmt.Errorf("iam: oturum iptal listesine eklenemedi: %w", err)
	}
	return nil
}

// IsRevoked, oturumun iptal listesinde olup olmadığını söyler.
func (l *RevocationList) IsRevoked(ctx context.Context, sessionID string) (bool, error) {
	n, err := l.rdb.Exists(ctx, revokedSessionKey(sessionID)).Result()
	if err != nil {
		return false, fmt.Errorf("iam: oturum iptal listesi okunamadı: %w", err)
	}
	return n > 0, nil
}
