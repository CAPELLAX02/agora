package notify

import (
	"context"
	"errors"
	"maps"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
)

// sample, sadece testte kataloğa eklenen, zorunlu olmayan bir türdür (varsayılanı uygulama içi).
const sample Type = "test.sample"

func useSampleType(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO communication.notification_types (code, category, name_tr, name_en, default_channels, is_mandatory, sort_order)
		VALUES ($1, 'COURSES', 'Deneme', 'Sample', '{IN_APP}', false, 999)`, string(sample)); err != nil {
		t.Fatal(err)
	}
	text := Text{TitleTR: "{{.course}} duyurusu", TitleEN: "{{.course}} announcement", BodyTR: "Yeni bir duyuru var.", BodyEN: "There is a new announcement."}
	texts[sample], compiled[sample] = text, compile(sample, text)
	t.Cleanup(func() {
		delete(texts, sample)
		delete(compiled, sample)
	})
}

func addUser(t *testing.T, pool *pgxpool.Pool, username, status string) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), `
		WITH p AS (INSERT INTO people.persons (first_name, last_name) VALUES ('Ada', $1) RETURNING id)
		INSERT INTO iam.users (person_id, username, email, password_hash, status)
		SELECT id, $1, $1 || '@agora.test', 'x', $2 FROM p RETURNING id`, username, status).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// delivered, kullanıcıya yazılan bildirim ve kuyruğa giren e-posta sayısıdır.
func delivered(t *testing.T, pool *pgxpool.Pool, userID string) (inApp, emails int) {
	t.Helper()
	err := pool.QueryRow(context.Background(), `
		SELECT (SELECT count(*) FROM communication.notifications WHERE user_id = $1),
		       (SELECT count(*) FROM communication.email_outbox o JOIN iam.users u ON u.email = o.to_address
		        WHERE u.id = $1 AND o.template_code = 'notification')`, userID).Scan(&inApp, &emails)
	if err != nil {
		t.Fatal(err)
	}
	return inApp, emails
}

// counter, kanal başına sayılan bildirimleri tutar.
type counter map[string]int

func (c counter) Delivered(typ, channel string, n int) { c[typ+" "+channel] += n }

func TestChannelsFollowPreferences(t *testing.T) {
	pool := dbtest.New(t)
	useSampleType(t, pool)
	counts := counter{}
	SetMetrics(counts)
	t.Cleanup(func() { SetMetrics(nil) })
	ctx := context.Background()
	c := NewCenter(pool, time.Now)
	ada := addUser(t, pool, "ada", "ACTIVE")
	send := func(typ Type, data map[string]any, users ...string) {
		t.Helper()
		if err := Send(ctx, pool, Notification{Type: typ, UserIDs: users, Data: data, Link: "/derslerim"}); err != nil {
			t.Fatal(err)
		}
	}

	// Varsayılan: sadece uygulama içi.
	send(sample, map[string]any{"course": "COM1007"}, ada)
	if in, mails := delivered(t, pool, ada); in != 1 || mails != 0 {
		t.Fatalf("varsayılan kanallar: %d bildirim, %d e-posta", in, mails)
	}

	// Uygulama içini kapatıp e-postayı açan kullanıcıya kayıt yazılmaz, e-posta gider.
	prefs, err := c.SetPreferences(ctx, ada, []PreferenceInput{{Type: sample, InApp: false, Email: true}})
	if err != nil {
		t.Fatal(err)
	}
	if last := prefs[len(prefs)-1]; last.Type != sample || last.InApp || !last.Email || last.Mandatory {
		t.Errorf("tercihler: %+v", prefs)
	}
	send(sample, map[string]any{"course": "COM1007"}, ada)
	if in, mails := delivered(t, pool, ada); in != 1 || mails != 1 {
		t.Fatalf("tercihten sonra: %d bildirim, %d e-posta", in, mails)
	}
	var title, link, name string
	if err := pool.QueryRow(ctx, `
		SELECT payload->>'title', payload->>'link', payload->>'name' FROM communication.email_outbox
		WHERE template_code = 'notification'`).Scan(&title, &link, &name); err != nil {
		t.Fatal(err)
	}
	if title != "COM1007 duyurusu" || link != "/derslerim" || name != "Ada" {
		t.Errorf("e-posta verisi: %q %q %q", title, link, name)
	}

	// Zorunlu türde tercih yok sayılır; etkin olmayan hesaba e-posta gitmez.
	if _, err := pool.Exec(ctx, `
		INSERT INTO communication.notification_preferences (user_id, type_code, channel, enabled)
		VALUES ($1, 'security.password_changed', 'IN_APP', false), ($1, 'security.password_changed', 'EMAIL', false)`, ada); err != nil {
		t.Fatal(err)
	}
	suspended := addUser(t, pool, "can", "SUSPENDED")
	send(PasswordChanged, nil, ada, suspended)
	if in, mails := delivered(t, pool, ada); in != 2 || mails != 2 {
		t.Errorf("zorunlu bildirim: %d bildirim, %d e-posta", in, mails)
	}
	if in, mails := delivered(t, pool, suspended); in != 1 || mails != 0 {
		t.Errorf("askıdaki hesap: %d bildirim, %d e-posta", in, mails)
	}

	want := counter{"test.sample in_app": 1, "test.sample email": 1, "security.password_changed in_app": 2, "security.password_changed email": 1}
	if !maps.Equal(counts, want) {
		t.Errorf("sayaçlar: %v, want %v", counts, want)
	}

	// Gönderirken şablon verisi denetlenir.
	if err := Send(ctx, pool, Notification{Type: sample, UserIDs: []string{ada}}); err == nil {
		t.Error("eksik şablon verisi kabul edildi")
	}
}

func TestSetPreferencesRejectsMandatoryAndUnknown(t *testing.T) {
	pool := dbtest.New(t)
	useSampleType(t, pool)
	ctx := context.Background()
	c := NewCenter(pool, time.Now)
	ada := addUser(t, pool, "ada", "ACTIVE")

	// Hatalı bir öğe varsa geçerli değişiklik de yazılmaz.
	_, err := c.SetPreferences(ctx, ada, []PreferenceInput{
		{Type: sample, InApp: false, Email: true},
		{Type: PasswordChanged, InApp: true, Email: false},
	})
	if !errors.Is(err, ErrMandatory) {
		t.Errorf("zorunlu tür: %v", err)
	}
	if _, err := c.SetPreferences(ctx, ada, []PreferenceInput{{Type: "test.unknown", InApp: true}}); !errors.Is(err, ErrUnknownType) {
		t.Errorf("bilinmeyen tür: %v", err)
	}
	prefs, err := c.Preferences(ctx, ada)
	if err != nil {
		t.Fatal(err)
	}
	if last := prefs[len(prefs)-1]; !last.InApp || last.Email {
		t.Errorf("geri alınan değişiklik yazılmış: %+v", last)
	}
	// Zorunlu türün varsayılan değeri gönderilebilir (değişiklik yok).
	if _, err := c.SetPreferences(ctx, ada, []PreferenceInput{{Type: PasswordChanged, InApp: true, Email: true}}); err != nil {
		t.Errorf("zorunlu türün varsayılanı: %v", err)
	}
	if prefs[0].Type != PasswordChanged || !prefs[0].Mandatory {
		t.Errorf("sıra ya da zorunluluk: %+v", prefs[0])
	}
}
