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

	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// Type, bir bildirim türüdür (communication.notification_types).
type Type string

// Bildirim türleri.
const (
	PasswordChanged Type = "security.password_changed"
	MFAEnabled      Type = "security.mfa_enabled"
	MFADisabled     Type = "security.mfa_disabled"
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
func Send(ctx context.Context, q db.Querier, n Notification) error {
	if _, ok := texts[n.Type]; !ok {
		return fmt.Errorf("notify: bilinmeyen bildirim türü %q", n.Type)
	}
	if n.Link != "" && !strings.HasPrefix(n.Link, "/") {
		return fmt.Errorf("notify: bağlantı uygulama içi olmalı: %q", n.Link)
	}
	if len(n.UserIDs) == 0 {
		return nil
	}
	data := n.Data
	if data == nil {
		data = map[string]any{}
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("notify: şablon verisi: %w", err)
	}
	if _, err := q.Exec(ctx, `
		INSERT INTO communication.notifications (user_id, type_code, payload, link_path)
		SELECT DISTINCT u, $2::text, $3::jsonb, nullif($4::text, '') FROM unnest($1::uuid[]) AS u`,
		n.UserIDs, string(n.Type), payload, n.Link); err != nil {
		return fmt.Errorf("notify: bildirim yazılamadı: %w", err)
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
