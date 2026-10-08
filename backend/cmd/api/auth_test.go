package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CAPELLAX02/agora/backend/internal/platform/config"
)

func TestLoadSigningKey(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("dosyadan", func(t *testing.T) {
		_, want, _ := ed25519.GenerateKey(rand.Reader)
		der, err := x509.MarshalPKCS8PrivateKey(want)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "jwt.pem")
		if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600); err != nil {
			t.Fatal(err)
		}

		got, err := loadSigningKey(config.Config{JWTPrivateKeyFile: path}, logger)
		if err != nil {
			t.Fatal(err)
		}
		if !got.Equal(want) {
			t.Error("dosyadaki anahtar okunmadı")
		}
	})

	t.Run("dosya yok", func(t *testing.T) {
		_, err := loadSigningKey(config.Config{JWTPrivateKeyFile: filepath.Join(t.TempDir(), "yok.pem")}, logger)
		if err == nil || !strings.Contains(err.Error(), "okunamadı") {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("dosya verilmemişse her seferinde yeni geçici anahtar", func(t *testing.T) {
		a, err := loadSigningKey(config.Config{}, logger)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := loadSigningKey(config.Config{}, logger)
		if len(a) != ed25519.PrivateKeySize || a.Equal(b) {
			t.Error("geçici anahtarlar rastgele ve geçerli uzunlukta olmalı")
		}
	})
}
