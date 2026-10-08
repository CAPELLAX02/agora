package mail_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tclog "github.com/testcontainers/testcontainers-go/log"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/CAPELLAX02/agora/backend/internal/mail"
	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
)

// fakeSender, gönderilen mesajları biriktirir. fail doluysa her gönderim o hatayla
// başarısız olur.
type fakeSender struct {
	mu   sync.Mutex
	sent []mail.Message
	fail error
}

func (f *fakeSender) Send(ctx context.Context, m mail.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail != nil {
		return f.fail
	}
	f.sent = append(f.sent, m)
	return nil
}

func (f *fakeSender) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sent)
}

var testConfig = mail.DispatcherConfig{
	BatchSize:   10,
	Lease:       time.Minute,
	SendTimeout: 5 * time.Second,
	MaxAttempts: 3,
	BackoffBase: 30 * time.Second,
	BackoffMax:  time.Hour,
}

func newDispatcher(t *testing.T, pool *pgxpool.Pool, sender mail.Sender, results *[]string) *mail.Dispatcher {
	t.Helper()
	r, err := mail.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	observe := func(result string) {
		mu.Lock()
		defer mu.Unlock()
		*results = append(*results, result)
	}
	return mail.NewDispatcher(pool, r, sender, testConfig, slog.New(slog.NewTextHandler(io.Discard, nil)), observe)
}

func enqueueReset(t *testing.T, pool *pgxpool.Pool, to string) string {
	t.Helper()
	id, err := mail.Enqueue(context.Background(), pool, mail.Email{
		To: to, Template: mail.TemplatePasswordReset,
		Data: map[string]any{"name": "Ayşe", "link": "http://localhost:5173/sifre-sifirla#token=gizli", "expires_minutes": 30},
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

type outboxRow struct {
	Status    string
	Attempts  int
	Payload   string
	LastError *string
	Wait      time.Duration // next_attempt_at - now()
}

func row(t *testing.T, pool *pgxpool.Pool, id string) outboxRow {
	t.Helper()
	var r outboxRow
	var waitSec float64
	err := pool.QueryRow(context.Background(), `
		SELECT status, attempts, payload::text, last_error, extract(epoch FROM next_attempt_at - now())
		FROM communication.email_outbox WHERE id = $1`, id,
	).Scan(&r.Status, &r.Attempts, &r.Payload, &r.LastError, &waitSec)
	if err != nil {
		t.Fatal(err)
	}
	r.Wait = time.Duration(waitSec * float64(time.Second))
	return r
}

func TestDispatcherSends(t *testing.T) {
	pool := dbtest.New(t)
	sender := &fakeSender{}
	var results []string
	d := newDispatcher(t, pool, sender, &results)

	id := enqueueReset(t, pool, "ayse@agora.test")

	n, err := d.RunOnce(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("n = %d, err = %v", n, err)
	}
	if sender.count() != 1 || !strings.Contains(sender.sent[0].Text, "#token=gizli") {
		t.Fatalf("gönderilen = %+v", sender.sent)
	}

	r := row(t, pool, id)
	if r.Status != "SENT" || r.Attempts != 1 {
		t.Errorf("kayıt = %+v", r)
	}
	if r.Payload != "{}" {
		t.Errorf("gönderildikten sonra gizli veri silinmeli: %s", r.Payload)
	}
	if len(results) != 1 || results[0] != mail.ResultSent {
		t.Errorf("metrik sonuçları = %v", results)
	}

	// Tekrar çalıştırmak aynı e-postayı yeniden göndermez.
	if n, _ := d.RunOnce(context.Background()); n != 0 {
		t.Errorf("gönderilmiş e-posta tekrar alındı")
	}
}

func TestDispatcherRetriesThenFails(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	sender := &fakeSender{fail: errors.New("smtp: bağlantı reddedildi")}
	var results []string
	d := newDispatcher(t, pool, sender, &results)
	id := enqueueReset(t, pool, "ayse@agora.test")

	for attempt := 1; attempt <= testConfig.MaxAttempts; attempt++ {
		if n, err := d.RunOnce(ctx); err != nil || n != 1 {
			t.Fatalf("%d. deneme: n = %d, err = %v", attempt, n, err)
		}
		r := row(t, pool, id)

		if attempt < testConfig.MaxAttempts {
			if r.Status != "PENDING" || r.Attempts != attempt || r.LastError == nil {
				t.Fatalf("%d. deneme sonrası = %+v", attempt, r)
			}
			// Üstel geri çekilme: 30 sn, 1 dk ... (en çok %20 sapma).
			want := testConfig.BackoffBase << (attempt - 1)
			if r.Wait < want-time.Second || r.Wait > want+want/5+time.Second {
				t.Errorf("%d. deneme sonrası bekleme = %v, ~%v olmalı", attempt, r.Wait, want)
			}
			// Bekleme süresi dolmadan tekrar alınmamalı.
			if n, _ := d.RunOnce(ctx); n != 0 {
				t.Fatal("bekleme süresi dolmadan yeniden denendi")
			}
			if _, err := pool.Exec(ctx, `UPDATE communication.email_outbox SET next_attempt_at = now() WHERE id = $1`, id); err != nil {
				t.Fatal(err)
			}
			continue
		}

		if r.Status != "FAILED" || r.Payload != "{}" {
			t.Errorf("son denemeden sonra kalıcı başarısızlık ve boş gövde bekleniyordu: %+v", r)
		}
	}

	want := []string{mail.ResultRetry, mail.ResultRetry, mail.ResultFailed}
	if fmt.Sprint(results) != fmt.Sprint(want) {
		t.Errorf("sonuçlar = %v, want %v", results, want)
	}
}

func TestDispatcherTemplateErrorIsPermanent(t *testing.T) {
	pool := dbtest.New(t)
	var results []string
	d := newDispatcher(t, pool, &fakeSender{}, &results)

	id, err := mail.Enqueue(context.Background(), pool, mail.Email{
		To: "a@agora.test", Template: mail.TemplatePasswordReset, Data: map[string]any{"name": "eksik veri"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = d.RunOnce(context.Background())

	if r := row(t, pool, id); r.Status != "FAILED" || r.Attempts != 1 {
		t.Errorf("şablon hatası yeniden denenmemeli: %+v", r)
	}
}

// TestDispatcherReclaimsExpiredLease, gönderirken çöken bir worker'ın aldığı kaydın
// kira süresi dolunca yeniden işlendiğini doğrular.
func TestDispatcherReclaimsExpiredLease(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	sender := &fakeSender{}
	var results []string
	d := newDispatcher(t, pool, sender, &results)
	id := enqueueReset(t, pool, "ayse@agora.test")

	// Başka bir worker kaydı almış ve çökmüş: SENDING, kira 1 dk daha geçerli.
	if _, err := pool.Exec(ctx, `
		UPDATE communication.email_outbox
		SET status = 'SENDING', attempts = 1, locked_until = now() + interval '1 minute' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if n, _ := d.RunOnce(ctx); n != 0 {
		t.Fatal("kirası süren kayıt başka worker tarafından alınmamalı")
	}

	if _, err := pool.Exec(ctx, `UPDATE communication.email_outbox SET locked_until = now() - interval '1 second' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if n, _ := d.RunOnce(ctx); n != 1 || sender.count() != 1 {
		t.Fatal("kirası dolan kayıt yeniden işlenmeli")
	}
	if r := row(t, pool, id); r.Status != "SENT" || r.Attempts != 2 {
		t.Errorf("kayıt = %+v", r)
	}
}

// TestConcurrentDispatchers, aynı anda çalışan worker'ların her e-postayı tam bir kez
// gönderdiğini doğrular (FOR UPDATE SKIP LOCKED).
func TestConcurrentDispatchers(t *testing.T) {
	pool := dbtest.New(t)
	const n = 60
	for i := range n {
		enqueueReset(t, pool, fmt.Sprintf("ogrenci%d@agora.test", i))
	}

	sender := &fakeSender{}
	var processed atomic.Int32
	var wg sync.WaitGroup
	for range 4 {
		var results []string
		d := newDispatcher(t, pool, sender, &results)
		wg.Go(func() {
			for {
				k, err := d.RunOnce(context.Background())
				if err != nil {
					t.Error(err)
					return
				}
				if k == 0 {
					return
				}
				processed.Add(int32(k))
			}
		})
	}
	wg.Wait()

	seen := map[string]int{}
	for _, m := range sender.sent {
		seen[m.To]++
	}
	if len(seen) != n || processed.Load() != n {
		t.Fatalf("%d farklı alıcıya, toplam %d gönderim yapıldı; %d olmalı", len(seen), processed.Load(), n)
	}
	for to, k := range seen {
		if k != 1 {
			t.Errorf("%s adresine %d kez gönderildi", to, k)
		}
	}
}

func TestLag(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()

	if lag, err := mail.Lag(ctx, pool); err != nil || lag != 0 {
		t.Fatalf("boş outbox: lag = %v, err = %v", lag, err)
	}
	id := enqueueReset(t, pool, "a@agora.test")
	if _, err := pool.Exec(ctx, `UPDATE communication.email_outbox SET next_attempt_at = now() - interval '90 seconds' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if lag, _ := mail.Lag(ctx, pool); lag < 89*time.Second || lag > 95*time.Second {
		t.Errorf("lag = %v, ~90 sn olmalı", lag)
	}
}

// TestSMTPSenderWithMailpit, e-postanın gerçek bir SMTP sunucusuna (Mailpit) iletildiğini
// ve içeriğinin bozulmadan ulaştığını doğrular.
func TestSMTPSenderWithMailpit(t *testing.T) {
	if testing.Short() {
		t.Skip("entegrasyon testi")
	}
	ctx := context.Background()
	ctr, err := testcontainers.Run(ctx, "axllent/mailpit:v1.31.2",
		testcontainers.WithExposedPorts("1025/tcp", "8025/tcp"),
		testcontainers.WithWaitStrategy(wait.ForHTTP("/livez").WithPort("8025/tcp")),
		testcontainers.WithLogger(tclog.TestLogger(t)),
	)
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatal(err)
	}
	smtpAddr, err := ctr.PortEndpoint(ctx, "1025/tcp", "")
	if err != nil {
		t.Fatal(err)
	}
	api, err := ctr.PortEndpoint(ctx, "8025/tcp", "http")
	if err != nil {
		t.Fatal(err)
	}

	r, err := mail.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	m, err := r.Render(mail.TemplatePasswordReset, "tr", "Ayşe Yılmaz <ayse@agora.test>", map[string]any{
		"name": "Ayşe Yılmaz", "link": "http://localhost:5173/sifre-sifirla#token=abc", "expires_minutes": 30,
	})
	if err != nil {
		t.Fatal(err)
	}

	sender := &mail.SMTPSender{Addr: smtpAddr, From: "Agora <no-reply@agora.test>", Timeout: 10 * time.Second}
	if err := sender.Send(ctx, m); err != nil {
		t.Fatalf("gönderilemedi: %v", err)
	}

	resp, err := http.Get(api + "/api/v1/messages")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var list struct {
		Messages []struct {
			ID      string `json:"ID"`
			Subject string `json:"Subject"`
			To      []struct {
				Address string `json:"Address"`
			} `json:"To"`
		} `json:"messages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list.Messages) != 1 || list.Messages[0].Subject != "Agora parola sıfırlama" ||
		list.Messages[0].To[0].Address != "ayse@agora.test" {
		t.Fatalf("Mailpit'teki mesajlar = %+v", list.Messages)
	}

	resp, err = http.Get(api + "/api/v1/message/" + list.Messages[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var full struct {
		Text string `json:"Text"`
		HTML string `json:"HTML"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&full); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(full.Text, "Ayşe Yılmaz") || !strings.Contains(full.HTML, "#token=abc") {
		t.Errorf("içerik bozuk ulaştı: %q", full.Text)
	}

	t.Run("ulaşılamayan sunucu zaman aşımına uğrar", func(t *testing.T) {
		s := &mail.SMTPSender{Addr: "10.255.255.1:25", From: "Agora <a@agora.test>", Timeout: 300 * time.Millisecond}
		start := time.Now()
		if err := s.Send(ctx, m); err == nil {
			t.Fatal("hata bekleniyordu")
		}
		if time.Since(start) > 3*time.Second {
			t.Errorf("zaman aşımı uygulanmadı: %v", time.Since(start))
		}
	})
}
