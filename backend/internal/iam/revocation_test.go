package iam_test

import (
	"context"
	"testing"
	"time"

	"github.com/CAPELLAX02/agora/backend/internal/iam"
	"github.com/CAPELLAX02/agora/backend/internal/platform/redistest"
)

func TestRevocationList(t *testing.T) {
	rdb := redistest.New(t)
	ctx := context.Background()
	list := iam.NewRevocationList(rdb, 15*time.Minute)

	revoked, err := list.IsRevoked(ctx, "oturum-1")
	if err != nil || revoked {
		t.Fatalf("boş listede: revoked = %v, err = %v", revoked, err)
	}

	if err := list.Revoke(ctx, "oturum-1"); err != nil {
		t.Fatal(err)
	}
	if revoked, _ := list.IsRevoked(ctx, "oturum-1"); !revoked {
		t.Error("iptal edilen oturum listede yok")
	}
	if revoked, _ := list.IsRevoked(ctx, "oturum-2"); revoked {
		t.Error("başka oturum etkilenmemeli")
	}

	// Kayıt sonsuza kadar kalmamalı: access token ömrü kadar yaşar.
	ttl := rdb.TTL(ctx, "agora:revoked_sid:oturum-1").Val()
	if ttl <= 14*time.Minute || ttl > 15*time.Minute {
		t.Errorf("TTL = %v, ~15 dk olmalı", ttl)
	}
}

func TestRevocationListRedisDown(t *testing.T) {
	rdb := redistest.New(t)
	list := iam.NewRevocationList(rdb, time.Minute)
	_ = rdb.Close() // kesintiyi taklit et

	if _, err := list.IsRevoked(context.Background(), "oturum-1"); err == nil {
		t.Error("Redis kapalıyken hata dönmeli: sessizce 'iptal edilmemiş' demek güvenlik açığı olurdu")
	}
	if err := list.Revoke(context.Background(), "oturum-1"); err == nil {
		t.Error("Redis kapalıyken Revoke hata dönmeli")
	}
}
