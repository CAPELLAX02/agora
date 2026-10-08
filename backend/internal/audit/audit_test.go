package audit_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

// requestContext, bir HTTP isteğinden geçmiş gibi istek kimliği ve istemci bilgisi
// taşıyan bir context üretir.
func requestContext(t *testing.T) context.Context {
	t.Helper()
	var ctx context.Context
	h := httpx.Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx = r.Context()
	}), httpx.RequestID, httpx.WithClientInfo)

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.RemoteAddr = "203.0.113.9:4321"
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh) Firefox/140.0")
	req.Header.Set("X-Request-Id", "istek-123")
	h.ServeHTTP(httptest.NewRecorder(), req)
	return ctx
}

func TestRecordSecurity(t *testing.T) {
	pool := dbtest.New(t)
	ctx := requestContext(t)
	repo := audit.NewRepository(pool)

	err := audit.RecordSecurity(ctx, pool, audit.SecurityEvent{
		Type:              audit.EventLoginFailed,
		UsernameAttempted: "22290001",
		Details:           map[string]any{"reason": "wrong_password"},
	})
	if err != nil {
		t.Fatal(err)
	}

	events, hasMore, err := repo.ListSecurityEvents(context.Background(), audit.SecurityEventFilter{Limit: 10})
	if err != nil || hasMore || len(events) != 1 {
		t.Fatalf("events = %+v, hasMore = %v, err = %v", events, hasMore, err)
	}
	e := events[0]
	if e.Type != audit.EventLoginFailed || e.UserID != "" || e.UsernameAttempted != "22290001" {
		t.Errorf("olay = %+v", e)
	}
	if e.IP != "203.0.113.9" || !strings.Contains(e.UserAgent, "Firefox") || e.RequestID != "istek-123" {
		t.Errorf("istemci bilgisi context'ten alınmadı: %+v", e)
	}
	var details map[string]string
	if err := json.Unmarshal(e.Details, &details); err != nil || details["reason"] != "wrong_password" {
		t.Errorf("details = %s", e.Details)
	}
}

func TestRecordAndListLog(t *testing.T) {
	pool := dbtest.New(t)
	ctx := requestContext(t)
	repo := audit.NewRepository(pool)

	actor := "01a11b7f-0000-7000-8000-000000000001"
	for i, action := range []string{"user.create", "role.assign", "role.assign", "user.suspend"} {
		var before any
		if action == "user.suspend" {
			before = map[string]string{"status": "ACTIVE"}
		}
		err := audit.Record(ctx, pool, audit.Entry{
			ActorUserID: actor, Action: action, EntityType: "user",
			EntityID: "kullanici-" + string(rune('a'+i%2)),
			Before:   before, After: map[string]any{"step": i},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	// Sistem işlemi: aktör yok.
	if err := audit.Record(context.Background(), pool, audit.Entry{Action: "user.create", EntityType: "user"}); err != nil {
		t.Fatal(err)
	}

	t.Run("filtreler", func(t *testing.T) {
		got, _, err := repo.ListLog(ctx, audit.LogFilter{Action: "role.assign", Limit: 10})
		if err != nil || len(got) != 2 {
			t.Fatalf("role.assign kayıtları = %d, err = %v", len(got), err)
		}
		got, _, _ = repo.ListLog(ctx, audit.LogFilter{EntityType: "user", EntityID: "kullanici-a", Limit: 10})
		if len(got) != 2 {
			t.Errorf("kullanici-a kayıtları = %d, want 2", len(got))
		}
		got, _, _ = repo.ListLog(ctx, audit.LogFilter{ActorUserID: actor, Limit: 10})
		if len(got) != 4 {
			t.Errorf("aktörün kayıtları = %d, want 4", len(got))
		}
	})

	t.Run("işlemi yapanın kullanıcı adı", func(t *testing.T) {
		if _, err := pool.Exec(ctx, `
			WITH p AS (INSERT INTO people.persons (first_name, last_name) VALUES ('Sistem', 'Yöneticisi') RETURNING id)
			INSERT INTO iam.users (id, person_id, username, email, password_hash)
			SELECT $1, id, 'P90001', 'yonetici@agora.test', 'x' FROM p`, actor); err != nil {
			t.Fatal(err)
		}
		got, _, _ := repo.ListLog(ctx, audit.LogFilter{Limit: 10})
		for _, e := range got {
			want := "P90001"
			if e.ActorUserID == "" {
				want = "" // sistem işlemi
			}
			if e.ActorName != want {
				t.Errorf("%s: kullanıcı adı = %q, want %q", e.Action, e.ActorName, want)
			}
		}
	})

	t.Run("en yeniden eskiye ve before/after", func(t *testing.T) {
		got, _, _ := repo.ListLog(ctx, audit.LogFilter{ActorUserID: actor, Limit: 10})
		if got[0].Action != "user.suspend" || got[3].Action != "user.create" {
			t.Errorf("sıra = %s ... %s", got[0].Action, got[3].Action)
		}
		if string(got[0].Before) != `{"status": "ACTIVE"}` {
			t.Errorf("before = %s", got[0].Before)
		}
		if len(got[3].Before) != 0 {
			t.Errorf("oluşturmada before boş (NULL) olmalı: %s", got[3].Before)
		}
	})

	t.Run("keyset sayfalama", func(t *testing.T) {
		var seen []int64
		var after *audit.Cursor
		for page := 0; ; page++ {
			got, hasMore, err := repo.ListLog(ctx, audit.LogFilter{Limit: 2, After: after})
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range got {
				seen = append(seen, e.ID)
			}
			if !hasMore {
				break
			}
			last := got[len(got)-1]
			after = &audit.Cursor{OccurredAt: last.OccurredAt, ID: last.ID}
			if page > 5 {
				t.Fatal("sayfalama bitmedi")
			}
		}
		if len(seen) != 5 {
			t.Fatalf("%d kayıt görüldü, 5 olmalı: %v", len(seen), seen)
		}
		for i := 1; i < len(seen); i++ {
			if seen[i] >= seen[i-1] {
				t.Errorf("tekrar ya da sıra hatası: %v", seen)
			}
		}
	})

	t.Run("zaman aralığı", func(t *testing.T) {
		future := time.Now().Add(time.Hour)
		got, _, _ := repo.ListLog(ctx, audit.LogFilter{From: &future, Limit: 10})
		if len(got) != 0 {
			t.Errorf("gelecekten kayıt dönmemeli: %d", len(got))
		}
	})
}

// TestPartitions, kayıtların aylık partition'a düştüğünü ve varsayılan partition'ın
// boş kaldığını doğrular.
func TestPartitions(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()

	if err := audit.EnsurePartitions(ctx, pool, 6); err != nil {
		t.Fatal(err)
	}
	// İkinci çağrı hata vermemeli (tekrar çalıştırılabilir).
	if err := audit.EnsurePartitions(ctx, pool, 6); err != nil {
		t.Fatalf("tekrar çalıştırma: %v", err)
	}

	var n int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM pg_inherits i
		JOIN pg_class c ON c.oid = i.inhrelid
		WHERE i.inhparent = 'audit.security_events'::regclass AND c.relname <> 'security_events_default'`,
	).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 7 {
		t.Errorf("%d aylık partition var, 7 olmalı (bu ay + 6)", n)
	}

	if err := audit.RecordSecurity(ctx, pool, audit.SecurityEvent{Type: audit.EventLogout}); err != nil {
		t.Fatal(err)
	}
	var partition string
	if err := pool.QueryRow(ctx, `SELECT tableoid::regclass::text FROM audit.security_events`).Scan(&partition); err != nil {
		t.Fatal(err)
	}
	want := "audit.security_events_" + time.Now().UTC().Format("2006_01")
	if partition != want {
		t.Errorf("kayıt %s partition'ına düştü, %s olmalı", partition, want)
	}
}

// fakeResolver, sadece "admin" kullanıcısına audit:read verir.
type fakeResolver struct{}

func (fakeResolver) Permissions(ctx context.Context, userID string) (*authz.Permissions, error) {
	if userID == "admin" {
		return authz.NewPermissions(userID, []authz.Grant{{Permission: "audit:read", ScopeType: authz.ScopeUniversity}}), nil
	}
	return authz.NewPermissions(userID, nil), nil
}

func withUser(ctx context.Context, userID string) context.Context {
	return authn.WithPrincipal(ctx, authn.Principal{UserID: userID})
}

func testServer(t *testing.T, pool *pgxpool.Pool) http.Handler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mux := http.NewServeMux()
	authenticate := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(withUser(r.Context(), r.Header.Get("X-Test-User"))))
		})
	}
	audit.NewHandler(audit.NewRepository(pool), logger).Register(authz.NewRouter(mux, authenticate, fakeResolver{}, logger))
	return mux
}

func TestHandler(t *testing.T) {
	pool := dbtest.New(t)
	ctx := requestContext(t)
	for range 3 {
		if err := audit.RecordSecurity(ctx, pool, audit.SecurityEvent{Type: audit.EventLoginSucceeded}); err != nil {
			t.Fatal(err)
		}
	}
	srv := testServer(t, pool)

	get := func(path, user string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("X-Test-User", user)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		return rec
	}

	if rec := get("/api/v1/audit/security-events", "ogrenci"); rec.Code != http.StatusForbidden {
		t.Errorf("yetkisiz: durum = %d", rec.Code)
	}

	rec := get("/api/v1/audit/security-events?limit=2", "admin")
	if rec.Code != http.StatusOK {
		t.Fatalf("durum = %d\n%s", rec.Code, rec.Body.String())
	}
	var page struct {
		Items []struct {
			Type string  `json:"type"`
			IP   *string `json:"ip"`
		} `json:"items"`
		NextCursor string `json:"next_cursor"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || page.NextCursor == "" || page.Items[0].IP == nil {
		t.Fatalf("ilk sayfa = %s", rec.Body.String())
	}

	rec = get("/api/v1/audit/security-events?limit=2&cursor="+page.NextCursor, "admin")
	page.Items, page.NextCursor = nil, "" // yanıtta olmayan alan eski değerini korumasın
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.NextCursor != "" {
		t.Errorf("ikinci sayfa = %s", rec.Body.String())
	}

	for _, bad := range []string{
		"?from=dun", "?cursor=bozuk", "?limit=1000", "?user_id=123",
		"?from=2026-10-09T00:00:00Z&to=2026-10-08T00:00:00Z",
	} {
		if rec := get("/api/v1/audit/security-events"+bad, "admin"); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: durum = %d, want 400", bad, rec.Code)
		}
	}

	if rec := get("/api/v1/audit/log?entity_type=user", "admin"); rec.Code != http.StatusOK {
		t.Errorf("denetim kayıtları: durum = %d", rec.Code)
	}
}

func TestMySecurityEvents(t *testing.T) {
	pool := dbtest.New(t)
	ctx := requestContext(t)
	alice := "01a11b7f-0000-7000-8000-00000000000a"
	bob := "01a11b7f-0000-7000-8000-00000000000b"
	for _, u := range []string{alice, alice, bob} {
		if err := audit.RecordSecurity(ctx, pool, audit.SecurityEvent{Type: audit.EventLoginSucceeded, UserID: u}); err != nil {
			t.Fatal(err)
		}
	}
	srv := testServer(t, pool)

	// user_id parametresi yok sayılır: kişi sadece kendi olaylarını görür.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/security-events?user_id="+bob, nil)
	req.Header.Set("X-Test-User", alice)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	var page struct {
		Items []struct {
			UserID string `json:"user_id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if rec.Code != 200 || len(page.Items) != 2 {
		t.Fatalf("olaylar = %d %s", rec.Code, rec.Body.String())
	}
	for _, e := range page.Items {
		if e.UserID != alice {
			t.Errorf("başka kullanıcının olayı döndü: %s", e.UserID)
		}
	}
}
