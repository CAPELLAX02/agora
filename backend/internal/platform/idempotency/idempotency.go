// Package idempotency, Idempotency-Key başlığıyla gelen değiştirici isteklerin tekrarında
// işlemi yeniden yapmadan ilk yanıtı döndüren bir middleware'dir.
//
// İstemci (ör. ders seçme ekranı) her değiştirici istek için yeni bir anahtar üretir ve
// isteği yeniden denerken aynı anahtarı gönderir. İlk istek işlenirken gelen tekrar 409,
// başka bir gövdeyle kullanılan anahtar 422 alır; tamamlanmış isteğin tekrarı ilk yanıtı
// (durum, gövde, ETag, Location) "Idempotent-Replayed: true" başlığıyla alır. Sunucu
// hatasıyla (5xx) biten isteğin anahtarı silinir: istemci aynı anahtarla yeniden deneyebilir.
package idempotency

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

// Header, istemcinin anahtarı gönderdiği başlıktır.
const Header = "Idempotency-Key"

// ReplayedHeader, yanıtın kayıttan döndüğünü söyleyen başlıktır.
const ReplayedHeader = "Idempotent-Replayed"

// maxBody, kaydedilecek en büyük istek gövdesidir.
const maxBody = 1 << 20

// Middleware, kimliği doğrulanmış route'larda Idempotency-Key başlığını uygular. Başlık
// yoksa istek olduğu gibi geçer.
func Middleware(pool *pgxpool.Pool, logger *slog.Logger) httpx.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get(Header)
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}
			principal, ok := authn.PrincipalFrom(r.Context())
			if !ok || principal.UserID == "" || len(key) < 8 || len(key) > 255 {
				httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: Header, Message: "8-255 karakterlik bir anahtar olmalı."}})
				return
			}
			body, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
			if err != nil || len(body) > maxBody {
				httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "body", Message: "İstek gövdesi okunamadı ya da çok büyük."}})
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			sum := sha256.Sum256(append([]byte(r.Method+" "+r.URL.RequestURI()+"\n"), body...))

			tag, err := pool.Exec(r.Context(), `
				INSERT INTO platform.idempotency_keys (user_id, key, fingerprint) VALUES ($1, $2, $3)
				ON CONFLICT DO NOTHING`, principal.UserID, key, sum[:])
			if err != nil {
				logger.Error("idempotentlik anahtarı yazılamadı", "err", err)
				httpx.InternalServerError(w, r)
				return
			}
			if tag.RowsAffected() == 0 {
				replay(w, r, pool, logger, principal.UserID, key, sum[:])
				return
			}

			rec := &recorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			// Yanıt istemciye gitti; kaydı istemcinin bağlamı iptal olsa da tamamla.
			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
			defer cancel()
			if rec.status >= 500 {
				_, err = pool.Exec(ctx, `DELETE FROM platform.idempotency_keys WHERE user_id = $1 AND key = $2`, principal.UserID, key)
			} else {
				h := rec.Header()
				_, err = pool.Exec(ctx, `
					UPDATE platform.idempotency_keys SET status_code = $3, response_body = $4, content_type = $5,
						etag = $6, location = $7, completed_at = now()
					WHERE user_id = $1 AND key = $2`, principal.UserID, key, rec.status, rec.body.Bytes(),
					h.Get("Content-Type"), nullable(h.Get("ETag")), nullable(h.Get("Location")))
			}
			if err != nil {
				logger.Error("idempotentlik kaydı tamamlanamadı", "err", err)
			}
		})
	}
}

func replay(w http.ResponseWriter, r *http.Request, pool *pgxpool.Pool, logger *slog.Logger, userID, key string, sum []byte) {
	var (
		fingerprint                 []byte
		status                      *int16
		body                        []byte
		contentType, etag, location *string
	)
	err := pool.QueryRow(r.Context(), `
		SELECT fingerprint, status_code, response_body, content_type, etag, location
		FROM platform.idempotency_keys WHERE user_id = $1 AND key = $2`, userID, key,
	).Scan(&fingerprint, &status, &body, &contentType, &etag, &location)
	if errors.Is(err, pgx.ErrNoRows) {
		// İlk istek sunucu hatasıyla bitti ve anahtar silindi: istemci yeniden denesin.
		problem(w, r, http.StatusConflict, "IDEMPOTENCY_RETRY", "İstek tamamlanamadı; aynı anahtarla yeniden deneyin.")
		return
	}
	if err != nil {
		logger.Error("idempotentlik anahtarı okunamadı", "err", err)
		httpx.InternalServerError(w, r)
		return
	}
	if !bytes.Equal(fingerprint, sum) {
		problem(w, r, http.StatusUnprocessableEntity, "IDEMPOTENCY_KEY_REUSED", "Bu anahtar başka bir istek için kullanılmış.")
		return
	}
	if status == nil {
		problem(w, r, http.StatusConflict, "IDEMPOTENCY_IN_PROGRESS", "Aynı istek hâlâ işleniyor.")
		return
	}
	h := w.Header()
	for name, v := range map[string]*string{"Content-Type": contentType, "ETag": etag, "Location": location} {
		if v != nil && *v != "" {
			h.Set(name, *v)
		}
	}
	h.Set(ReplayedHeader, "true")
	w.WriteHeader(int(*status))
	_, _ = w.Write(body)
}

func problem(w http.ResponseWriter, r *http.Request, status int, code, detail string) {
	_ = httpx.WriteProblem(w, r, httpx.Problem{Status: status, Code: code, Detail: detail})
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// recorder, yanıtı istemciye yazarken bir kopyasını tutar.
type recorder struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
	wrote  bool
}

func (r *recorder) WriteHeader(code int) {
	if !r.wrote {
		r.status, r.wrote = code, true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *recorder) Write(b []byte) (int, error) {
	r.wrote = true
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// Purge, olderThan'dan eski anahtarları siler ve silinen sayıyı döndürür.
func Purge(ctx context.Context, q db.Querier, olderThan time.Duration) (int64, error) {
	tag, err := q.Exec(ctx, `DELETE FROM platform.idempotency_keys WHERE created_at < now() - make_interval(secs => $1)`,
		olderThan.Seconds())
	if err != nil {
		return 0, fmt.Errorf("idempotency: eski anahtarlar silinemedi: %w", err)
	}
	return tag.RowsAffected(), nil
}
