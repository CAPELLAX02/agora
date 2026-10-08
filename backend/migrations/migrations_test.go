package migrations

import (
	"fmt"
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// fileName, migration dosya adı biçimidir: 5 haneli sıra numarası + snake_case ad.
var fileName = regexp.MustCompile(`^(\d{5})_[a-z0-9]+(_[a-z0-9]+)*\.sql$`)

// TestMigrationFiles, gömülü migration dosyalarının adlandırma, sıra ve
// goose işaretleme kurallarına uyduğunu doğrular.
func TestMigrationFiles(t *testing.T) {
	entries, err := fs.ReadDir(FS, ".")
	if err != nil {
		t.Fatalf("migration klasörü okunamadı: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("hiç migration dosyası gömülmemiş")
	}

	for i, e := range entries {
		name := e.Name()

		m := fileName.FindStringSubmatch(name)
		if m == nil {
			t.Errorf("%s: dosya adı NNNNN_snake_case_ad.sql biçiminde olmalı", name)
			continue
		}

		// fs.ReadDir sonuçları ada göre sıralıdır, bu yüzden sıra numaraları
		// 1'den başlayıp boşluksuz artmalıdır.
		if want := fmt.Sprintf("%05d", i+1); m[1] != want {
			t.Errorf("%s: sıra numarası %s olmalı (eksik ya da tekrarlanan numara var)", name, want)
		}

		body, err := fs.ReadFile(FS, name)
		if err != nil {
			t.Fatalf("%s okunamadı: %v", name, err)
		}
		content := string(body)

		up := strings.Index(content, "-- +goose Up")
		down := strings.Index(content, "-- +goose Down")
		switch {
		case up == -1:
			t.Errorf("%s: '-- +goose Up' bölümü yok", name)
		case down == -1:
			t.Errorf("%s: '-- +goose Down' bölümü yok, migration geri alınamaz", name)
		case down < up:
			t.Errorf("%s: Down bölümü Up bölümünden önce gelmiş", name)
		}
	}
}
