package notify_test

import (
	"bufio"
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

	"github.com/CAPELLAX02/agora/backend/internal/notify"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
)

type event struct {
	id, name string
	data     map[string]any
}

// openStream, kullanıcının canlı akışını açar; olaylar kanaldan okunur. Akış bağlanınca döner.
func openStream(t *testing.T, ctx context.Context, url, user, lastID string) (*http.Response, <-chan event) {
	t.Helper()
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("X-Test-User", user)
	if lastID != "" {
		req.Header.Set("Last-Event-ID", lastID)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan event, 16)
	if res.StatusCode != 200 {
		close(events)
		return res, events
	}
	if ct := res.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("içerik türü: %s", ct)
	}
	sc := bufio.NewScanner(res.Body)
	if !sc.Scan() || sc.Text() != "retry: 5000" {
		t.Fatalf("akış başlangıcı: %q", sc.Text())
	}
	go func() {
		defer close(events)
		var e event
		for sc.Scan() {
			line := sc.Text()
			switch {
			case line == "":
				if e.name != "" {
					events <- e
				}
				e = event{}
			case strings.HasPrefix(line, "id: "):
				e.id = strings.TrimPrefix(line, "id: ")
			case strings.HasPrefix(line, "event: "):
				e.name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				_ = json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &e.data)
			}
		}
	}()
	return res, events
}

func next(t *testing.T, events <-chan event) event {
	t.Helper()
	select {
	case e, ok := <-events:
		if !ok {
			t.Fatal("akış kapandı")
		}
		return e
	case <-time.After(10 * time.Second):
		t.Fatal("olay gelmedi")
	}
	return event{}
}

func TestStream(t *testing.T) {
	pool := dbtest.New(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ada, can := user(t, pool, "ada"), user(t, pool, "can")
	hub := notify.NewHub(pool, logger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	authenticate := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(authn.WithPrincipal(r.Context(), authn.Principal{UserID: r.Header.Get("X-Test-User")})))
		})
	}
	mux := http.NewServeMux()
	notify.NewHandler(notify.NewCenter(pool, time.Now), hub, logger).Register(authz.NewRouter(mux, authenticate, noGrants{}, logger))
	srv := httptest.NewServer(mux)
	defer srv.Close()
	url := srv.URL + "/api/v1/me/notifications/stream"
	send := func(users ...string) {
		t.Helper()
		if err := notify.Send(context.Background(), pool, notify.Notification{Type: notify.PasswordChanged, UserIDs: users, Link: "/guvenlik"}); err != nil {
			t.Fatal(err)
		}
	}

	// Bağlanmadan önceki bildirim gönderilmez (listeden okunur); sonrakiler canlı gelir.
	send(ada)
	streamCtx, closeStream := context.WithCancel(ctx)
	_, events := openStream(t, streamCtx, url, ada, "")
	send(ada, can)
	e := next(t, events)
	if e.name != "notification" || e.data["type"] != "security.password_changed" || e.data["title_tr"] != "Parolanız değiştirildi" ||
		e.data["link"] != "/guvenlik" || e.id == "" {
		t.Fatalf("olay: %+v", e)
	}
	if got := id(e.data["id"].(float64)); got != e.id {
		t.Errorf("olay kimliği %s, bildirim kimliği %s", e.id, got)
	}
	first := e.id

	// Bağlantı kopukken gelen bildirim yeniden bağlanınca Last-Event-ID ile gelir.
	closeStream()
	send(ada)
	_, events = openStream(t, ctx, url, ada, first)
	if e := next(t, events); e.name != "notification" || atoi(e.id) <= atoi(first) {
		t.Errorf("kaçırılan bildirim: %+v", e)
	}

	// Kullanıcı başına akış sınırı.
	for range notify.MaxStreamsPerUser {
		openStream(t, ctx, url, can, "")
	}
	if res, _ := openStream(t, ctx, url, can, ""); res.StatusCode != http.StatusTooManyRequests {
		t.Errorf("sınır aşımı: %d", res.StatusCode)
	}
	if n := hub.Streams(); n != 1+notify.MaxStreamsPerUser {
		t.Errorf("açık akış: %d", n)
	}

	// Kapanışta açık akışlar biter.
	hub.Close()
	select {
	case _, ok := <-events:
		for ok {
			_, ok = <-events
		}
	case <-time.After(10 * time.Second):
		t.Fatal("kapanışta akış bitmedi")
	}
}

func atoi(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}
