// Package notify, kullanıcı bildirimlerini yazar ve okur: bildirim kataloğu, alıcıya yazma
// (çağıranın transaction'ında), bildirim merkezi (liste, okunmamış sayısı, okundu, arşiv) ve
// bildirim metinleri.
//
// Kayıtta metin değil şablon verisi tutulur; başlık ve metin okunurken türün Türkçe ve
// İngilizce şablonlarından üretilir (text.go).
package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/mail"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// Type, bir bildirim türüdür (communication.notification_types).
type Type string

// Bildirim türleri.
const (
	PasswordChanged Type = "security.password_changed"
	MFAEnabled      Type = "security.mfa_enabled"
	MFADisabled     Type = "security.mfa_disabled"

	RegistrationSubmitted Type = "registration.submitted"
	RegistrationDecision  Type = "registration.decision"
	AddDropPending        Type = "registration.add_drop_pending"
	AddDropDecision       Type = "registration.add_drop_decision"

	ComponentPublished Type = "grades.component_published"
	LettersPublished   Type = "grades.letters_published"
	ChangeRequested    Type = "grades.change_requested"
	ChangeDecided      Type = "grades.change_decided"

	AssignmentCreated  Type = "lms.assignment_created"
	AssignmentReleased Type = "lms.grades_released"
	ForumAnnouncement  Type = "forum.announcement"
	ForumReply         Type = "forum.reply"
	ForumThread        Type = "forum.thread"

	AnnouncementPublished Type = "announcement.published"
	AnnouncementUrgent    Type = "announcement.urgent"

	MessageReceived Type = "message.received"

	WindowOpening Type = "reminder.window_opening"
	WindowClosing Type = "reminder.window_closing"
	AssignmentDue Type = "reminder.assignment_due"
)

// ErrNotFound: bildirim yok ya da başka bir kullanıcının.
var ErrNotFound = errors.New("notify: bildirim bulunamadı")

// Notification, alıcılara yazılacak bir bildirimdir.
type Notification struct {
	Type    Type
	UserIDs []string
	Data    map[string]any // şablon verisi; JSON'a çevrilebilir olmalı
	Link    string         // uygulama içi adres ("/" ile başlar); boş olabilir
}

// Send, bildirimi her alıcıya bir kez, verilen sorgulayıcıyla (çoğunlukla çağıranın
// transaction'ı) yazar: bildirim, iş verisiyle birlikte commit edilir ya da hiç yazılmaz.
//
// Kanallar alıcının tercihinden (yoksa türün varsayılanından) gelir; zorunlu türlerde tercih
// yok sayılır. Uygulama içi kanal kapalıysa kayıt yazılmaz; e-posta açıksa ve hesap etkinse
// e-posta aynı transaction'da kuyruğa girer.
func Send(ctx context.Context, q db.Querier, n Notification) error {
	if n.Link != "" && !strings.HasPrefix(n.Link, "/") {
		return fmt.Errorf("notify: bağlantı uygulama içi olmalı: %q", n.Link)
	}
	data := n.Data
	if data == nil {
		data = map[string]any{}
	}
	// Şablon verisi gönderirken denetlenir: eksik alan bir programlama hatasıdır.
	text, err := render(n.Type, data)
	if err != nil {
		return err
	}
	if len(n.UserIDs) == 0 {
		return nil
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("notify: şablon verisi: %w", err)
	}
	rows, err := q.Query(ctx, `
		WITH r AS (SELECT DISTINCT unnest($1::uuid[]) AS user_id),
		     t AS (SELECT * FROM communication.notification_types WHERE code = $2)
		SELECT r.user_id, u.email, p.first_name, u.status = 'ACTIVE', t.is_mandatory,
		       CASE WHEN t.is_mandatory THEN 'IN_APP' = ANY (t.default_channels)
		            ELSE coalesce(pa.enabled, 'IN_APP' = ANY (t.default_channels)) END,
		       CASE WHEN t.is_mandatory THEN 'EMAIL' = ANY (t.default_channels)
		            ELSE coalesce(pe.enabled, 'EMAIL' = ANY (t.default_channels)) END
		FROM r
		JOIN iam.users u ON u.id = r.user_id
		JOIN people.persons p ON p.id = u.person_id
		CROSS JOIN t
		LEFT JOIN communication.notification_preferences pa ON pa.user_id = r.user_id AND pa.type_code = t.code AND pa.channel = 'IN_APP'
		LEFT JOIN communication.notification_preferences pe ON pe.user_id = r.user_id AND pe.type_code = t.code AND pe.channel = 'EMAIL'`,
		n.UserIDs, string(n.Type))
	if err != nil {
		return fmt.Errorf("notify: alıcılar okunamadı: %w", err)
	}
	type recipient struct {
		id, email, name   string
		active, mandatory bool
		inApp, byEmail    bool
	}
	var list []recipient
	var r recipient
	if _, err := pgx.ForEachRow(rows, []any{&r.id, &r.email, &r.name, &r.active, &r.mandatory, &r.inApp, &r.byEmail}, func() error {
		list = append(list, r)
		return nil
	}); err != nil {
		return fmt.Errorf("notify: alıcılar okunamadı: %w", err)
	}

	var (
		inApp  []string
		emails []mail.Email
	)
	for _, r := range list {
		if r.inApp {
			inApp = append(inApp, r.id)
		}
		if r.byEmail && r.active {
			emails = append(emails, mail.Email{To: r.email, Template: mail.TemplateNotification, Data: map[string]any{
				"name": r.name, "title": text.TitleTR, "body": text.BodyTR, "link": n.Link, "mandatory": r.mandatory,
			}})
		}
	}
	if err := mail.EnqueueBatch(ctx, q, emails); err != nil {
		return fmt.Errorf("notify: %w", err)
	}
	if len(inApp) == 0 {
		return nil
	}
	if _, err := q.Exec(ctx, `
		INSERT INTO communication.notifications (user_id, type_code, payload, link_path)
		SELECT u, $2::text, $3::jsonb, nullif($4::text, '') FROM unnest($1::uuid[]) AS u`,
		inApp, string(n.Type), payload, n.Link); err != nil {
		return fmt.Errorf("notify: bildirim yazılamadı: %w", err)
	}
	// Canlı akışlara duyuru: PostgreSQL bunu transaction commit edilince iletir.
	if _, err := q.Exec(ctx, `SELECT pg_notify('`+channel+`', u) FROM unnest($1::text[]) AS u`, inApp); err != nil {
		return fmt.Errorf("notify: bildirim duyurulamadı: %w", err)
	}
	return nil
}

// EnsurePartitions, içinde bulunulan ay ve sonraki monthsAhead ay için partition açar.
func EnsurePartitions(ctx context.Context, q db.Querier, monthsAhead int) error {
	if _, err := q.Exec(ctx, `SELECT communication.ensure_notification_partitions($1)`, monthsAhead); err != nil {
		return fmt.Errorf("notify: partition'lar açılamadı: %w", err)
	}
	return nil
}

// DropExpired, saklama süresini (gün) bütünüyle aşan aylık partition'ları düşürür ve
// düşürülen partition sayısını döndürür.
func DropExpired(ctx context.Context, q db.Querier, keepDays int) (int, error) {
	var n int
	if err := q.QueryRow(ctx, `SELECT communication.drop_notification_partitions(make_interval(days => $1))`,
		keepDays).Scan(&n); err != nil {
		return 0, fmt.Errorf("notify: eski partition'lar düşürülemedi: %w", err)
	}
	return n, nil
}
