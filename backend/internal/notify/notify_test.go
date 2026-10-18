package notify_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/notify"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
)

type noGrants struct{}

func (noGrants) Permissions(_ context.Context, userID string) (*authz.Permissions, error) {
	return authz.NewPermissions(userID, nil), nil
}

func user(t *testing.T, pool *pgxpool.Pool, username string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), `
		WITH p AS (INSERT INTO people.persons (first_name, last_name) VALUES ('Ada', $1) RETURNING id)
		INSERT INTO iam.users (person_id, username, email, password_hash)
		SELECT id, $1, $1 || '@agora.test', 'x' FROM p RETURNING id`, username).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

type server struct {
	t    *testing.T
	pool *pgxpool.Pool
	h    http.Handler
}

func newServer(t *testing.T) *server {
	t.Helper()
	pool := dbtest.New(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	authenticate := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(authn.WithPrincipal(r.Context(), authn.Principal{UserID: r.Header.Get("X-Test-User")})))
		})
	}
	mux := http.NewServeMux()
	rt := authz.NewRouter(mux, authenticate, noGrants{}, logger)
	notify.NewHandler(notify.NewCenter(pool, time.Now), nil, logger).Register(rt)
	return &server{t: t, pool: pool, h: mux}
}

type result struct {
	status int
	body   map[string]any
	raw    string
}

func (s *server) do(method, path, user string) result {
	s.t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("X-Test-User", user)
	rec := httptest.NewRecorder()
	s.h.ServeHTTP(rec, req)
	res := result{status: rec.Code, raw: rec.Body.String()}
	_ = json.Unmarshal(rec.Body.Bytes(), &res.body)
	return res
}

func (s *server) send(n notify.Notification) {
	s.t.Helper()
	if err := notify.Send(context.Background(), s.pool, n); err != nil {
		s.t.Fatal(err)
	}
}

func items(res result) []map[string]any {
	out := []map[string]any{}
	for _, it := range res.body["items"].([]any) {
		out = append(out, it.(map[string]any))
	}
	return out
}

func (s *server) unread(user string) float64 {
	s.t.Helper()
	return s.do("GET", "/api/v1/me/notifications/unread-count", user).body["count"].(float64)
}

func TestCenter(t *testing.T) {
	s := newServer(t)
	ada, can := user(t, s.pool, "ada"), user(t, s.pool, "can")
	s.send(notify.Notification{Type: notify.PasswordChanged, UserIDs: []string{ada, ada, can}, Link: "/guvenlik"})
	s.send(notify.Notification{Type: notify.MFADisabled, UserIDs: []string{ada}, Data: map[string]any{"by": "admin"}})

	res := s.do("GET", "/api/v1/me/notifications", ada)
	list := items(res)
	if res.status != 200 || len(list) != 2 {
		t.Fatalf("liste: %d %s", res.status, res.raw)
	}
	// En yeni önce; metin şablondan, veriye göre.
	if list[0]["type"] != "security.mfa_disabled" || list[0]["category"] != "SECURITY" || list[0]["link"] != nil ||
		!strings.Contains(list[0]["body_tr"].(string), "bir yönetici tarafından sıfırlandı") ||
		!strings.Contains(list[0]["body_en"].(string), "An administrator reset") {
		t.Errorf("ilk bildirim: %v", list[0])
	}
	if list[1]["title_tr"] != "Parolanız değiştirildi" || list[1]["link"] != "/guvenlik" || list[1]["read_at"] != nil {
		t.Errorf("ikinci bildirim: %v", list[1])
	}
	if n := s.unread(ada); n != 2 {
		t.Errorf("okunmamış: %v", n)
	}

	// Sayfalama.
	res = s.do("GET", "/api/v1/me/notifications?limit=1", ada)
	if len(items(res)) != 1 || res.body["next_cursor"] == nil {
		t.Fatalf("ilk sayfa: %s", res.raw)
	}
	res = s.do("GET", "/api/v1/me/notifications?limit=1&cursor="+res.body["next_cursor"].(string), ada)
	if page := items(res); len(page) != 1 || page[0]["id"] != list[1]["id"] || res.body["next_cursor"] != nil {
		t.Errorf("ikinci sayfa: %s", res.raw)
	}
	if res := s.do("GET", "/api/v1/me/notifications?cursor=bozuk", ada); res.status != 400 {
		t.Errorf("bozuk cursor: %d", res.status)
	}

	// Başkasının bildirimi yokmuş gibi.
	canID := items(s.do("GET", "/api/v1/me/notifications", can))[0]["id"].(float64)
	if res := s.do("POST", "/api/v1/me/notifications/"+id(canID)+"/read", ada); res.status != 404 {
		t.Errorf("başkasının bildirimi: %d", res.status)
	}
	if res := s.do("POST", "/api/v1/me/notifications/abc/read", ada); res.status != 404 {
		t.Errorf("geçersiz kimlik: %d", res.status)
	}

	first := id(list[0]["id"].(float64))
	if res := s.do("POST", "/api/v1/me/notifications/"+first+"/read", ada); res.status != 204 {
		t.Fatalf("okundu: %d %s", res.status, res.raw)
	}
	if n := s.unread(ada); n != 1 {
		t.Errorf("okundu sonrası: %v", n)
	}
	if res := s.do("GET", "/api/v1/me/notifications?unread=true", ada); len(items(res)) != 1 {
		t.Errorf("okunmamışlar: %s", res.raw)
	}

	// Arşiv: listeden çıkar, okundu sayılır; geri alınabilir.
	second := id(list[1]["id"].(float64))
	if res := s.do("POST", "/api/v1/me/notifications/"+second+"/archive", ada); res.status != 204 {
		t.Fatalf("arşiv: %d", res.status)
	}
	if n := s.unread(ada); n != 0 {
		t.Errorf("arşiv sonrası okunmamış: %v", n)
	}
	if res := s.do("GET", "/api/v1/me/notifications", ada); len(items(res)) != 1 {
		t.Errorf("arşivlenen listede: %s", res.raw)
	}
	res = s.do("GET", "/api/v1/me/notifications?archived=true", ada)
	if archived := items(res); len(archived) != 1 || archived[0]["archived_at"] == nil || archived[0]["read_at"] == nil {
		t.Errorf("arşiv listesi: %s", res.raw)
	}
	if res := s.do("DELETE", "/api/v1/me/notifications/"+second+"/archive", ada); res.status != 204 {
		t.Fatalf("arşivden geri al: %d", res.status)
	}
	if res := s.do("GET", "/api/v1/me/notifications", ada); len(items(res)) != 2 {
		t.Errorf("geri alınan: %s", res.raw)
	}

	if res := s.do("POST", "/api/v1/me/notifications/read-all", can); res.status != 200 || res.body["updated"] != 1.0 {
		t.Errorf("hepsini okundu yap: %d %s", res.status, res.raw)
	}
	if n := s.unread(can); n != 0 {
		t.Errorf("hepsi okundu sonrası: %v", n)
	}
}

// id, JSON'dan okunan bildirim kimliğini adres parçasına çevirir.
func id(v float64) string {
	return strconv.FormatInt(int64(v), 10)
}

func TestSendRejectsBadInput(t *testing.T) {
	pool := dbtest.New(t)
	ada := user(t, pool, "ada")
	ctx := context.Background()
	if err := notify.Send(ctx, pool, notify.Notification{Type: "security.unknown", UserIDs: []string{ada}}); err == nil {
		t.Error("bilinmeyen tür kabul edildi")
	}
	if err := notify.Send(ctx, pool, notify.Notification{Type: notify.PasswordChanged, UserIDs: []string{ada},
		Link: "https://example.com"}); err == nil {
		t.Error("dış bağlantı kabul edildi")
	}
	if err := notify.Send(ctx, pool, notify.Notification{Type: notify.PasswordChanged}); err != nil {
		t.Errorf("alıcısız bildirim: %v", err)
	}
}

// Eski bir kaydın verisi şablonla uyuşmazsa metin yerine türün adı gösterilir.
func TestRenderFallsBackToTypeName(t *testing.T) {
	s := newServer(t)
	ada := user(t, s.pool, "ada")
	if _, err := s.pool.Exec(context.Background(), `
		INSERT INTO communication.notifications (user_id, type_code) VALUES ($1, 'security.mfa_disabled')`, ada); err != nil {
		t.Fatal(err) // "by" eksik
	}
	list := items(s.do("GET", "/api/v1/me/notifications", ada))
	if len(list) != 1 || list[0]["title_tr"] != "İki adımlı doğrulama kapatıldı" || list[0]["body_tr"] != "" ||
		list[0]["title_en"] != "Two-step verification turned off" {
		t.Errorf("yedek metin: %v", list)
	}
}

func TestPartitions(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	ada := user(t, pool, "ada")
	if err := notify.EnsurePartitions(ctx, pool, 2); err != nil {
		t.Fatal(err)
	}
	if err := notify.EnsurePartitions(ctx, pool, 2); err != nil { // tekrar çalıştırılabilir
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		CREATE TABLE communication.notifications_2025_01 PARTITION OF communication.notifications
			FOR VALUES FROM ('2025-01-01 00:00:00+00') TO ('2025-02-01 00:00:00+00')`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO communication.notifications (user_id, type_code, created_at) VALUES
			($1, 'security.password_changed', '2025-01-15 10:00:00+00'),
			($1, 'security.password_changed', now())`, ada); err != nil {
		t.Fatal(err)
	}
	var where string
	if err := pool.QueryRow(ctx, `
		SELECT tableoid::regclass::text FROM communication.notifications WHERE created_at > now() - interval '1 day'`).Scan(&where); err != nil {
		t.Fatal(err)
	}
	if want := "communication.notifications_" + time.Now().UTC().Format("2006_01"); where != want {
		t.Errorf("yeni bildirim %s partition'ında, %s olmalı", where, want)
	}

	n, err := notify.DropExpired(ctx, pool, 180)
	if err != nil || n != 1 {
		t.Fatalf("düşürülen: %d %v", n, err)
	}
	var left int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM communication.notifications`).Scan(&left); err != nil || left != 1 {
		t.Errorf("kalan: %d %v", left, err)
	}
}

func TestPreferencesEndpoints(t *testing.T) {
	s := newServer(t)
	ada := user(t, s.pool, "ada")
	res := s.do("GET", "/api/v1/me/notification-preferences", ada)
	list := items(res)
	if res.status != 200 || len(list) < 3 || list[0]["type"] != "security.password_changed" ||
		list[0]["mandatory"] != true || list[0]["in_app"] != true || list[0]["email"] != true || list[0]["name_tr"] != "Parolanız değiştirildi" {
		t.Fatalf("tercihler: %d %s", res.status, res.raw)
	}
	for body, want := range map[string]int{
		`{"items":[{"type":"security.password_changed","in_app":true,"email":false}]}`: 400,
		`{"items":[{"type":"security.password_changed","in_app":true}]}`:               400,
		`{"items":[{"type":"yok.boyle","in_app":true,"email":true}]}`:                  400,
		`{"items":[{"type":"security.password_changed","in_app":true,"email":true}]}`:  200,
		`{"items":[]}`: 200,
	} {
		if res := s.put("/api/v1/me/notification-preferences", ada, body); res.status != want {
			t.Errorf("%s: %d %s", body, res.status, res.raw)
		}
	}
}

func (s *server) put(path, user, body string) result {
	s.t.Helper()
	req := httptest.NewRequest("PUT", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-User", user)
	rec := httptest.NewRecorder()
	s.h.ServeHTTP(rec, req)
	res := result{status: rec.Code, raw: rec.Body.String()}
	_ = json.Unmarshal(rec.Body.Bytes(), &res.body)
	return res
}
