package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Querier, hem *pgxpool.Pool'un hem pgx.Tx'in sağladığı sorgu metotlarıdır.
// Repository'ler bu interface'e bağımlı olduğunda aynı kod transaction içinde
// de dışında da çalışabilir.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

var (
	_ Querier = (*pgxpool.Pool)(nil)
	_ Querier = (pgx.Tx)(nil)
)

// InTx, fn'i tek bir transaction içinde çalıştırır. fn hata dönerse ya da panic
// olursa transaction geri alınır, aksi halde commit edilir.
//
// Spring'deki @Transactional'ın açık hali: transaction'ın nerede başlayıp nerede
// bittiği kodda görünür.
func InTx(ctx context.Context, pool *pgxpool.Pool, fn func(tx pgx.Tx) error) (err error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("db: transaction başlatılamadı: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p) // panic'i yutmuyoruz, sadece önce transaction'ı temizliyoruz
		}
		if err != nil {
			if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
				err = errors.Join(err, fmt.Errorf("db: rollback başarısız: %w", rbErr))
			}
		}
	}()

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("db: commit başarısız: %w", err)
	}
	return nil
}

// PostgreSQL hata kodları (SQLSTATE).
const (
	codeUniqueViolation     = "23505"
	codeExclusionViolation  = "23P01"
	codeCheckViolation      = "23514"
	codeForeignKeyViolation = "23503"
)

// IsConflict, hatanın bir benzersizlik (UNIQUE) ya da dışlama (EXCLUDE) kısıtı
// ihlalinden kaynaklanıp kaynaklanmadığını söyler. "Bu kayıt zaten var" durumunu
// 500 yerine 409 Conflict'e çevirmek için kullanılır.
func IsConflict(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == codeUniqueViolation || pgErr.Code == codeExclusionViolation
}

// IsCheckViolation, hatanın bir CHECK kısıtı ihlali olup olmadığını söyler
// (ör. bitiş zamanı başlangıçtan önce).
func IsCheckViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codeCheckViolation
}

// IsForeignKeyViolation, hatanın bir yabancı anahtar ihlali olup olmadığını söyler
// (ör. var olmayan bir kayda başvuru).
func IsForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codeForeignKeyViolation
}
