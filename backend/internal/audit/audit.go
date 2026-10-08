// Package audit, denetim izini (kim neyi değiştirdi) ve güvenlik olaylarını (giriş,
// kilitlenme, parola değişikliği...) kaydeder ve sorgular.
//
// Kayıt fonksiyonları bir db.Querier alır: çağıran taraf kendi transaction'ını
// verirse olay iş verisiyle birlikte commit edilir ya da birlikte geri alınır.
// "Rol atandı ama denetim kaydı yazılamadı" gibi bir tutarsızlık oluşamaz.
package audit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

// Güvenlik olayı türleri.
const (
	EventLoginSucceeded         = "LOGIN_SUCCEEDED"
	EventLoginFailed            = "LOGIN_FAILED"
	EventAccountLocked          = "ACCOUNT_LOCKED"
	EventLogout                 = "LOGOUT"
	EventRefreshTokenReused     = "REFRESH_TOKEN_REUSED"
	EventPasswordChanged        = "PASSWORD_CHANGED"
	EventPasswordResetRequested = "PASSWORD_RESET_REQUESTED"
	EventPasswordResetCompleted = "PASSWORD_RESET_COMPLETED"
	EventSessionRevoked         = "SESSION_REVOKED"
	EventOtherSessionsRevoked   = "OTHER_SESSIONS_REVOKED"
	EventMFAEnabled             = "MFA_ENABLED"
	EventMFADisabled            = "MFA_DISABLED"
)

// SecurityEvent, kimlik doğrulamayla ilgili bir olaydır.
type SecurityEvent struct {
	Type              string
	UserID            string // kullanıcı bulunamadıysa boş
	UsernameAttempted string
	Details           map[string]any
}

// Entry, iş verisi üzerindeki bir değişikliğin kaydıdır. Before ve After, değişen
// varlığın önceki ve sonraki halidir (JSON'a çevrilir). Oluşturmada Before,
// silmede After boş bırakılır.
type Entry struct {
	ActorUserID string // sistem işlemlerinde boş
	Action      string // kaynak.eylem, ör. "role.assign"
	EntityType  string
	EntityID    string
	Before      any
	After       any
}

// RecordSecurity, bir güvenlik olayı yazar. İstemci IP'si, tarayıcı bilgisi ve
// istek kimliği context'ten alınır.
func RecordSecurity(ctx context.Context, q db.Querier, e SecurityEvent) error {
	details := []byte("{}")
	if len(e.Details) > 0 {
		b, err := json.Marshal(e.Details)
		if err != nil {
			return fmt.Errorf("audit: olay ayrıntıları kodlanamadı: %w", err)
		}
		details = b
	}

	client := httpx.ClientInfoFrom(ctx)
	_, err := q.Exec(ctx, `
		INSERT INTO audit.security_events
			(event_type, user_id, username_attempted, ip, user_agent, request_id, details)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		e.Type, nullable(e.UserID), nullable(e.UsernameAttempted),
		nullable(client.IP), nullable(client.UserAgent), nullable(httpx.RequestIDFrom(ctx)), details,
	)
	if err != nil {
		return fmt.Errorf("audit: güvenlik olayı yazılamadı: %w", err)
	}
	return nil
}

// Record, bir denetim kaydı yazar.
func Record(ctx context.Context, q db.Querier, e Entry) error {
	before, err := toJSON(e.Before)
	if err != nil {
		return err
	}
	after, err := toJSON(e.After)
	if err != nil {
		return err
	}

	client := httpx.ClientInfoFrom(ctx)
	_, err = q.Exec(ctx, `
		INSERT INTO audit.audit_log
			(actor_user_id, action, entity_type, entity_id, before, after, ip, user_agent, request_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		nullable(e.ActorUserID), e.Action, e.EntityType, nullable(e.EntityID), before, after,
		nullable(client.IP), nullable(client.UserAgent), nullable(httpx.RequestIDFrom(ctx)),
	)
	if err != nil {
		return fmt.Errorf("audit: denetim kaydı yazılamadı: %w", err)
	}
	return nil
}

// EnsurePartitions, içinde bulunulan ay ve sonraki monthsAhead ay için partition açar.
func EnsurePartitions(ctx context.Context, q db.Querier, monthsAhead int) error {
	if _, err := q.Exec(ctx, `SELECT audit.ensure_partitions($1)`, monthsAhead); err != nil {
		return fmt.Errorf("audit: partition'lar açılamadı: %w", err)
	}
	return nil
}

func toJSON(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("audit: kayıt kodlanamadı: %w", err)
	}
	return b, nil
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
