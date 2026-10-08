// Package migrations, veritabanı şema migrasyonlarını ikili dosyanın içerisine gömer.
package migrations

import "embed"

// FS, bu klasördeki tüm .sql migration dosyalarını içerir.
//
//go:embed *.sql
var FS embed.FS
