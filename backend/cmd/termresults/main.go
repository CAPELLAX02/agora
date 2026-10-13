// Komut termresults, notu olan bütün öğrencilerin dönem sonuçlarını (YANO, GANO, AKTS, onur)
// kesin kayıtlardan yeniden hesaplar ve kayıtlı sonuçlarla, ders seçmenin kullandığı GANO
// önbelleğiyle karşılaştırır. Varsayılan olarak sadece raporlar; fark varsa çıkış kodu 1'dir
// (gece doğrulaması ve CI için). -apply ile farkı olan öğrencilerin sonuçlarını yazar.
//
// Öğrenciler gruplara bölünür ve gruplar paralel işlenir (errgroup); her grup kendi
// transaction'ındadır. Çalışan API ve worker'la birlikte güvenle çalışır: yazılan sonuç, not
// ilanının tüketicisinin yazacağıyla aynıdır.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/CAPELLAX02/agora/backend/internal/grading"
	"github.com/CAPELLAX02/agora/backend/internal/platform/config"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// errDrift, doğrulamada fark bulunduğunu söyler (çıkış kodu 1).
var errDrift = errors.New("dönem sonuçları kesin kayıtlarla tutmuyor")

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "agora-termresults: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	apply := flag.Bool("apply", false, "farkı olan öğrencilerin sonuçlarını yaz (verilmezse sadece raporlar)")
	workers := flag.Int("workers", min(runtime.NumCPU(), 8), "aynı anda işlenen grup sayısı")
	batch := flag.Int("batch", 500, "bir grupta hesaplanan öğrenci sayısı")
	flag.Parse()
	if *workers < 1 || *workers > 64 || *batch < 1 || *batch > 10000 {
		return errors.New("-workers 1-64, -batch 1-10000 arasında olmalı")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, db.Options{
		URL:             cfg.DatabaseURL,
		MaxConns:        int32(*workers + 1),
		MaxConnLifetime: cfg.DBMaxConnLifetime,
		MaxConnIdleTime: cfg.DBMaxConnIdleTime,
	})
	if err != nil {
		return err
	}
	defer pool.Close()

	r, err := grading.RecomputeAll(ctx, pool, grading.RecomputeOptions{Apply: *apply, Workers: *workers, Batch: *batch})
	if err != nil {
		return err
	}
	mode := "doğrulama"
	if r.Applied {
		mode = "yeniden hesap"
	}
	fmt.Printf("%s: %d öğrenci, %d dönem sonucu, %d paralel iş × %d öğrenci, %s (öğrenci başına %s)\n",
		mode, r.Students, r.Results, *workers, *batch, r.Duration.Round(time.Millisecond), perStudent(r))
	fmt.Printf("farklı öğrenci: %d (eksik dönem %d, fazla dönem %d, değişen dönem %d, önbellek %d)\n",
		r.Drifted, r.RowsMissing, r.RowsExtra, r.RowsChanged, r.CacheChanged)
	if len(r.Examples) > 0 {
		fmt.Printf("örnekler: %s\n", strings.Join(r.Examples, ", "))
	}
	if r.Orphans > 0 {
		fmt.Printf("notu olmadan GANO önbelleği dolu: %d (dokunulmadı)\n", r.Orphans)
	}
	switch {
	case r.Drifted == 0:
		fmt.Println("tutarlı")
	case r.Applied:
		fmt.Printf("%d öğrencinin sonuçları yazıldı\n", r.Drifted)
	default:
		fmt.Println("yazmak için: -apply")
		return errDrift
	}
	return nil
}

func perStudent(r grading.RecomputeReport) string {
	if r.Students == 0 {
		return "-"
	}
	return (r.Duration / time.Duration(r.Students)).Round(time.Microsecond).String()
}
