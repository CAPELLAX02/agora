// Komut migrate, veritabanı şema migrasyonlarını yönetir.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/CAPELLAX02/agora/backend/internal/platform/config"
	"github.com/CAPELLAX02/agora/backend/migrations"
)

const usage = `kullanım: migrate <komut>

komutlar:
  up       bekleyen tüm migration'ları uygular
  down     son uygulanan migration'ı geri alır
  status   tüm migration'ların durumunu listeler
  version  veritabanının mevcut şema sürümünü yazdırır`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "agora-migrate: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("tek bir komut bekleniyordu\n\n%s", usage)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("veritabanı açılamadı: %w", err)
	}
	defer db.Close()

	provider, err := migrations.NewProvider(db)
	if err != nil {
		return err
	}

	switch args[0] {
	case "up":
		results, err := provider.Up(ctx)
		for _, r := range results {
			fmt.Printf("UYGULANDI %-28s %v\n", r.Source.Path, r.Duration.Round(time.Millisecond))
		}
		if err != nil {
			return fmt.Errorf("up: %w", err)
		}
		if len(results) == 0 {
			fmt.Println("veritabanı güncel, bekleyen migration yok")
		}

	case "down":
		r, err := provider.Down(ctx)
		if err != nil {
			return fmt.Errorf("down: %w", err)
		}
		fmt.Printf("GERİ ALINDI %-28s %v\n", r.Source.Path, r.Duration.Round(time.Millisecond))

	case "status":
		statuses, err := provider.Status(ctx)
		if err != nil {
			return fmt.Errorf("status: %w", err)
		}
		for _, s := range statuses {
			applied := "-"
			if !s.AppliedAt.IsZero() {
				applied = s.AppliedAt.Local().Format("2006-01-02 15:04:05")
			}
			fmt.Printf("%-8s %-20s %s\n", s.State, applied, s.Source.Path)
		}

	case "version":
		v, err := provider.GetDBVersion(ctx)
		if err != nil {
			return fmt.Errorf("version: %w", err)
		}
		fmt.Println(v)

	default:
		return fmt.Errorf("bilinmeyen komut %q\n\n%s", args[0], usage)
	}

	return nil
}
