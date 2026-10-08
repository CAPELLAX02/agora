// Package migrations, veritabanı şema migrasyonlarını ikili dosyanın içerisine gömer.
package migrations

import (
	"database/sql"
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

// FS, bu klasördeki tüm .sql migration dosyalarını içerir.
//
//go:embed *.sql
var FS embed.FS

// NewProvider, gömülü migration'ları verilen veritabanına uygulayacak bir goose Provider'ı oluşturur.
// Eşzamanlı çalıştırmalara karşı PostgreSQL advisory lock kullanır.
func NewProvider(db *sql.DB) (*goose.Provider, error) {
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return nil, fmt.Errorf("migrations: kilit oluşturulamadı: %w", err)
	}

	provider, err := goose.NewProvider(goose.DialectPostgres, db, FS, goose.WithSessionLocker(locker))
	if err != nil {
		return nil, fmt.Errorf("migrations: goose başlatılamadı: %w", err)
	}

	return provider, nil
}
