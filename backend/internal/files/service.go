// Package files, yüklenen dosyalardır: akışla yükleme (tür ve boyut denetimiyle), üst veri,
// imzalı indirme URL'si, bir varlığa bağlama ve bağlanmayanların temizliği (yol haritası
// §5 #20). Bir dosyayı kimin indirebileceğine dosyayı kullanan varlık karar verir; burası
// sadece yükleyenin kendi dosyasına erişimini bilir.
package files

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// Hatalar.
var (
	ErrNotFound = errors.New("files: dosya bulunamadı")
	ErrAttached = errors.New("files: dosya bir kayda bağlı")
)

// RejectError, yüklemenin reddidir; Code ve Detail istemciye döner.
type RejectError struct {
	Code   string
	Detail string
}

func (e *RejectError) Error() string { return "files: " + e.Detail }

// StorageError, nesne deposuna ulaşılamadığını söyler.
type StorageError struct{ Err error }

func (e *StorageError) Error() string { return "files: depo: " + e.Err.Error() }
func (e *StorageError) Unwrap() error { return e.Err }

// File, yüklenmiş bir dosyanın üst verisidir.
type File struct {
	ID         string
	Name       string
	MimeType   string
	Size       int64
	SHA256     []byte
	UploadedBy string
	UploadedAt time.Time
	AttachedAt *time.Time
	key        string
}

// Metrics, dosya işlemlerinin sayaçlarıdır.
type Metrics interface {
	Uploaded(bytes int64)
	Rejected(code string)
}

type noopMetrics struct{}

func (noopMetrics) Uploaded(int64)  {}
func (noopMetrics) Rejected(string) {}

// Service, dosya işlemleridir.
type Service struct {
	pool     *pgxpool.Pool
	store    Store
	maxBytes int64
	now      func() time.Time
	metrics  Metrics
}

// NewService, bir Service oluşturur. maxBytes bir dosyanın en büyük boyutudur.
func NewService(pool *pgxpool.Pool, store Store, maxBytes int64, now func() time.Time) *Service {
	return &Service{pool: pool, store: store, maxBytes: maxBytes, now: now, metrics: noopMetrics{}}
}

// SetMetrics, sayaçları bağlar.
func (s *Service) SetMetrics(m Metrics) { s.metrics = m }

// MaxBytes, bir dosyanın en büyük boyutudur.
func (s *Service) MaxBytes() int64 { return s.maxBytes }

var errTooLarge = errors.New("files: dosya çok büyük")

// limitReader, n bayttan fazlası okunursa errTooLarge döner (io.LimitReader sessizce keser).
type limitReader struct {
	r     io.Reader
	n     int64
	total int64
}

func (l *limitReader) Read(p []byte) (int, error) {
	n, err := l.r.Read(p)
	l.total += int64(n)
	if l.total > l.n {
		return n, errTooLarge
	}
	return n, err
}

func (s *Service) reject(code, detail string) error {
	s.metrics.Rejected(code)
	return &RejectError{Code: code, Detail: detail}
}

// Upload, dosyayı akışla depoya yazar ve üst verisini kaydeder: bellekte sadece ilk 512 bayt
// (tür tespiti) tutulur, SHA-256 ve boyut akarken hesaplanır. Tür uzantıdan seçilir ve
// içerikle uyuşmalıdır; boyut sınırı aşılırsa depoya bir şey kalmaz.
func (s *Service) Upload(ctx context.Context, userID, name string, body io.Reader) (File, error) {
	name, ext := cleanName(name)
	if name == "" || ext == "" {
		return File{}, s.reject("FILE_TYPE_NOT_ALLOWED", "Dosya adında bir uzantı olmalı.")
	}
	k, ok := kinds[ext]
	if !ok {
		return File{}, s.reject("FILE_TYPE_NOT_ALLOWED", fmt.Sprintf("%s dosyaları yüklenemez.", ext))
	}
	head := make([]byte, 512)
	n, err := io.ReadFull(body, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return File{}, fmt.Errorf("files: dosya okunamadı: %w", err)
	}
	head = head[:n]
	if n == 0 {
		return File{}, s.reject("FILE_EMPTY", "Dosya boş.")
	}
	if !k.matches(head) {
		return File{}, s.reject("FILE_CONTENT_MISMATCH", fmt.Sprintf("Dosyanın içeriği %s uzantısıyla uyuşmuyor.", ext))
	}

	hash := sha256.New()
	limited := &limitReader{r: io.MultiReader(bytes.NewReader(head), body), n: s.maxBytes}
	random := make([]byte, 16)
	_, _ = rand.Read(random)
	key := s.now().UTC().Format("2006/01/") + hex.EncodeToString(random)
	if err := s.store.Put(ctx, key, io.TeeReader(limited, hash), -1, k.mime); err != nil {
		if errors.Is(err, errTooLarge) || limited.total > s.maxBytes {
			return File{}, s.reject("FILE_TOO_LARGE", fmt.Sprintf("Dosya en fazla %d MB olabilir.", s.maxBytes>>20))
		}
		if ctx.Err() != nil {
			return File{}, ctx.Err()
		}
		return File{}, &StorageError{Err: err}
	}

	f := File{Name: name, MimeType: k.mime, Size: limited.total, SHA256: hash.Sum(nil), UploadedBy: userID, key: key}
	err = s.pool.QueryRow(ctx, `
		INSERT INTO platform.files (storage_key, original_name, mime_type, size_bytes, sha256, uploaded_by, uploaded_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id, uploaded_at`,
		key, f.Name, f.MimeType, f.Size, f.SHA256, userID, s.now()).Scan(&f.ID, &f.UploadedAt)
	if err != nil {
		_ = s.store.Remove(context.WithoutCancel(ctx), key)
		return File{}, fmt.Errorf("files: dosya kaydedilemedi: %w", err)
	}
	s.metrics.Uploaded(f.Size)
	return f, nil
}

const fileColumns = `id, storage_key, original_name, mime_type, size_bytes, sha256, uploaded_by, uploaded_at, attached_at`

func scanFile(row pgx.Row) (File, error) {
	var f File
	err := row.Scan(&f.ID, &f.key, &f.Name, &f.MimeType, &f.Size, &f.SHA256, &f.UploadedBy, &f.UploadedAt, &f.AttachedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return File{}, ErrNotFound
	}
	if err != nil {
		return File{}, fmt.Errorf("files: dosya okunamadı: %w", err)
	}
	return f, nil
}

// Get, silinmemiş dosyanın üst verisidir.
func (s *Service) Get(ctx context.Context, q db.Querier, id string) (File, error) {
	return scanFile(q.QueryRow(ctx, `SELECT `+fileColumns+` FROM platform.files WHERE id = $1 AND deleted_at IS NULL`, id))
}

// URLTTL, imzalı indirme URL'lerinin ömrüdür.
const URLTTL = 5 * time.Minute

// DownloadURL, dosyayı URLTTL süresince indirtecek imzalı URL'dir. Yetkiyi çağıran denetler.
// İndirme her zaman ek olarak verilir (tarayıcıda açılmaz); Türkçe ad RFC 5987 ile korunur.
func (s *Service) DownloadURL(ctx context.Context, f File) (string, time.Time, error) {
	ascii := strings.Map(func(r rune) rune {
		if r < 0x20 || r > 0x7e || r == '"' || r == '\\' {
			return '_'
		}
		return r
	}, f.Name)
	disposition := fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, ascii, url.PathEscape(f.Name))
	u, err := s.store.PresignGet(ctx, f.key, disposition, URLTTL)
	if err != nil {
		return "", time.Time{}, &StorageError{Err: err}
	}
	return u, s.now().Add(URLTTL), nil
}

// Attach, yükleyenin henüz bağlanmamış dosyasını bir varlığa bağlar (varlığın
// transaction'ında). Başkasının ya da zaten bağlanmış dosya bağlanamaz.
func (s *Service) Attach(ctx context.Context, q db.Querier, id, userID string) (File, error) {
	f, err := scanFile(q.QueryRow(ctx, `
		UPDATE platform.files SET attached_at = now()
		WHERE id = $1 AND uploaded_by = $2 AND attached_at IS NULL AND deleted_at IS NULL
		RETURNING `+fileColumns, id, userID))
	if errors.Is(err, ErrNotFound) {
		if _, gerr := s.Get(ctx, q, id); gerr == nil {
			return File{}, ErrAttached
		}
	}
	return f, err
}

// Delete, yükleyenin henüz bağlanmamış dosyasını siler.
func (s *Service) Delete(ctx context.Context, userID, id string) error {
	var key string
	var attached *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT storage_key, attached_at FROM platform.files WHERE id = $1 AND uploaded_by = $2 AND deleted_at IS NULL`,
		id, userID).Scan(&key, &attached)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("files: dosya okunamadı: %w", err)
	}
	if attached != nil {
		return ErrAttached
	}
	if _, err := s.pool.Exec(ctx, `UPDATE platform.files SET deleted_at = now() WHERE id = $1`, id); err != nil {
		return fmt.Errorf("files: dosya silinemedi: %w", err)
	}
	if err := s.store.Remove(ctx, key); err != nil {
		return &StorageError{Err: err}
	}
	return nil
}

// CleanupUnattached, olderThan'dan eski ve bir varlığa bağlanmamış dosyaları siler (en fazla
// limit kadar). Önce kayıtlar bağlanmamış koşuluyla tek sorguda silinmiş işaretlenir, sonra
// nesneler depodan silinir: aynı anda bağlanan dosya ya bağlanır ya silinir, ikisi birden
// olmaz (Attach silinmiş dosyayı bağlamaz). Silinen sayıyı döndürür.
func (s *Service) CleanupUnattached(ctx context.Context, olderThan time.Duration, limit int) (int, error) {
	rows, err := s.pool.Query(ctx, `
		UPDATE platform.files SET deleted_at = now()
		WHERE id IN (SELECT id FROM platform.files
		             WHERE attached_at IS NULL AND deleted_at IS NULL AND uploaded_at < $1
		             ORDER BY uploaded_at LIMIT $2 FOR UPDATE SKIP LOCKED)
		  AND attached_at IS NULL
		RETURNING storage_key`, s.now().Add(-olderThan), limit)
	if err != nil {
		return 0, fmt.Errorf("files: temizlik: %w", err)
	}
	keys, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return 0, fmt.Errorf("files: temizlik: %w", err)
	}
	for _, key := range keys {
		if err := s.store.Remove(ctx, key); err != nil {
			return len(keys), &StorageError{Err: err}
		}
	}
	return len(keys), nil
}
