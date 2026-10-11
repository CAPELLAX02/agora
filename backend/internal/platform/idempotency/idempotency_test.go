package idempotency_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
	"github.com/CAPELLAX02/agora/backend/internal/platform/idempotency"
)

func TestMiddleware(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	var userID string
	if err := pool.QueryRow(ctx, `
		WITH p AS (INSERT INTO people.persons (first_name, last_name) VALUES ('Ece', 'Kaya') RETURNING id)
		INSERT INTO iam.users (person_id, username, email, password_hash) SELECT id, '22290002', 'ece@agora.test', 'x' FROM p
		RETURNING id`).Scan(&userID); err != nil {
		t.Fatal(err)
	}

	var calls atomic.Int32
	status := http.StatusCreated
	h := idempotency.Middleware(pool, slog.New(slog.NewTextHandler(io.Discard, nil)))(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			n := calls.Add(1)
			body, _ := io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("ETag", `"7"`)
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"call":` + string(rune('0'+n)) + `,"body":` + string(body) + `}`))
		}))
	do := func(key, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", "/api/v1/student-programs/x/registration/items", strings.NewReader(body))
		if key != "" {
			req.Header.Set(idempotency.Header, key)
		}
		req = req.WithContext(authn.WithPrincipal(req.Context(), authn.Principal{UserID: userID}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	first := do("anahtar-0001", `{"s":1}`)
	again := do("anahtar-0001", `{"s":1}`)
	if calls.Load() != 1 {
		t.Fatalf("handler %d kez çalıştı, 1 olmalı", calls.Load())
	}
	if again.Code != http.StatusCreated || again.Body.String() != first.Body.String() ||
		again.Header().Get("ETag") != `"7"` || again.Header().Get(idempotency.ReplayedHeader) != "true" {
		t.Errorf("tekrar = %d %q %v", again.Code, again.Body.String(), again.Header())
	}
	if r := do("anahtar-0001", `{"s":2}`); r.Code != http.StatusUnprocessableEntity || !strings.Contains(r.Body.String(), "IDEMPOTENCY_KEY_REUSED") {
		t.Errorf("başka gövdeyle aynı anahtar: %d %s", r.Code, r.Body.String())
	}
	if r := do("kisa", `{}`); r.Code != http.StatusBadRequest {
		t.Errorf("kısa anahtar: %d", r.Code)
	}
	if do("", `{}`); calls.Load() != 2 {
		t.Errorf("anahtarsız istek her zaman çalışmalı: %d", calls.Load())
	}

	// Sunucu hatasıyla biten isteğin anahtarı silinir: aynı anahtarla yeniden denenebilir.
	status = http.StatusInternalServerError
	do("anahtar-0002", `{}`)
	status = http.StatusCreated
	if r := do("anahtar-0002", `{}`); r.Code != http.StatusCreated || calls.Load() != 4 {
		t.Errorf("5xx sonrası yeniden deneme: %d, %d çağrı", r.Code, calls.Load())
	}
	if n, err := idempotency.Purge(ctx, pool, 0); err != nil || n != 2 {
		t.Errorf("temizlik = %d, %v", n, err)
	}
}
