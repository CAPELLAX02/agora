package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// LogEntry, okunan bir denetim kaydıdır.
type LogEntry struct {
	ID          int64
	OccurredAt  time.Time
	ActorUserID string
	ActorName   string // işlemi yapanın kullanıcı adı (hesap silinmişse boş)
	Action      string
	EntityType  string
	EntityID    string
	Before      json.RawMessage
	After       json.RawMessage
	IP          string
	UserAgent   string
	RequestID   string
}

// StoredSecurityEvent, okunan bir güvenlik olayıdır.
type StoredSecurityEvent struct {
	ID                int64
	OccurredAt        time.Time
	Type              string
	UserID            string
	Username          string // user_id'nin güncel kullanıcı adı
	UsernameAttempted string
	IP                string
	UserAgent         string
	RequestID         string
	Details           json.RawMessage
}

// Cursor, en yeniden eskiye sıralı listelerde bir sonraki sayfanın başlangıcıdır.
// (occurred_at, id) çifti benzersiz bir konum belirtir.
type Cursor struct {
	OccurredAt time.Time `json:"t"`
	ID         int64     `json:"i"`
}

// LogFilter, denetim kaydı listesinin filtreleridir. Boş alanlar filtre uygulamaz.
type LogFilter struct {
	ActorUserID string
	Action      string
	EntityType  string
	EntityID    string
	From, To    *time.Time // [From, To)
	After       *Cursor
	Limit       int
}

// SecurityEventFilter, güvenlik olayı listesinin filtreleridir.
type SecurityEventFilter struct {
	UserID   string
	Type     string
	From, To *time.Time
	After    *Cursor
	Limit    int
}

// Repository, denetim kayıtlarını okur.
type Repository struct {
	db db.Querier
}

// NewRepository, bir Repository oluşturur.
func NewRepository(q db.Querier) *Repository {
	return &Repository{db: q}
}

// ListLog, filtreye uyan denetim kayıtlarını en yeniden eskiye döndürür. hasMore,
// sonraki sayfa olup olmadığını söyler.
func (r *Repository) ListLog(ctx context.Context, f LogFilter) (entries []LogEntry, hasMore bool, err error) {
	var w db.Where
	if f.ActorUserID != "" {
		w.Add("actor_user_id = $1", f.ActorUserID)
	}
	if f.Action != "" {
		w.Add("action = $1", f.Action)
	}
	if f.EntityType != "" {
		w.Add("entity_type = $1", f.EntityType)
	}
	if f.EntityID != "" {
		w.Add("entity_id = $1", f.EntityID)
	}
	addTimeAndCursor(&w, f.From, f.To, f.After)
	limit := w.Arg(f.Limit + 1)

	rows, err := r.db.Query(ctx, `
		SELECT id, occurred_at, coalesce(actor_user_id::text, ''),
		       coalesce((SELECT u.username::text FROM iam.users u WHERE u.id = audit_log.actor_user_id), ''),
		       action, entity_type,
		       coalesce(entity_id, ''), before, after, coalesce(host(ip), ''),
		       coalesce(user_agent, ''), coalesce(request_id, '')
		FROM audit.audit_log
		`+w.SQL()+`
		ORDER BY occurred_at DESC, id DESC
		LIMIT `+limit, w.Args()...)
	if err != nil {
		return nil, false, fmt.Errorf("audit: denetim kayıtları listelenemedi: %w", err)
	}

	entries, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (LogEntry, error) {
		var e LogEntry
		err := row.Scan(&e.ID, &e.OccurredAt, &e.ActorUserID, &e.ActorName, &e.Action, &e.EntityType,
			&e.EntityID, &e.Before, &e.After, &e.IP, &e.UserAgent, &e.RequestID)
		return e, err
	})
	if err != nil {
		return nil, false, fmt.Errorf("audit: denetim kayıtları okunamadı: %w", err)
	}
	return trim(entries, f.Limit)
}

// ListSecurityEvents, filtreye uyan güvenlik olaylarını en yeniden eskiye döndürür.
func (r *Repository) ListSecurityEvents(ctx context.Context, f SecurityEventFilter) (events []StoredSecurityEvent, hasMore bool, err error) {
	var w db.Where
	if f.UserID != "" {
		w.Add("user_id = $1", f.UserID)
	}
	if f.Type != "" {
		w.Add("event_type = $1", f.Type)
	}
	addTimeAndCursor(&w, f.From, f.To, f.After)
	limit := w.Arg(f.Limit + 1)

	rows, err := r.db.Query(ctx, `
		SELECT id, occurred_at, event_type, coalesce(user_id::text, ''),
		       coalesce((SELECT u.username::text FROM iam.users u WHERE u.id = security_events.user_id), ''),
		       coalesce(username_attempted, ''), coalesce(host(ip), ''),
		       coalesce(user_agent, ''), coalesce(request_id, ''), details
		FROM audit.security_events
		`+w.SQL()+`
		ORDER BY occurred_at DESC, id DESC
		LIMIT `+limit, w.Args()...)
	if err != nil {
		return nil, false, fmt.Errorf("audit: güvenlik olayları listelenemedi: %w", err)
	}

	events, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (StoredSecurityEvent, error) {
		var e StoredSecurityEvent
		err := row.Scan(&e.ID, &e.OccurredAt, &e.Type, &e.UserID, &e.Username, &e.UsernameAttempted,
			&e.IP, &e.UserAgent, &e.RequestID, &e.Details)
		return e, err
	})
	if err != nil {
		return nil, false, fmt.Errorf("audit: güvenlik olayları okunamadı: %w", err)
	}
	return trim(events, f.Limit)
}

// addTimeAndCursor, zaman aralığı ve sayfa konumu koşullarını ekler. Zaman koşulu
// partition budamayı (partition pruning) sağlar: sorgu sadece ilgili ayların
// tablolarına bakar.
func addTimeAndCursor(w *db.Where, from, to *time.Time, after *Cursor) {
	if from != nil {
		w.Add("occurred_at >= $1", *from)
	}
	if to != nil {
		w.Add("occurred_at < $1", *to)
	}
	if after != nil {
		w.Add("(occurred_at, id) < ($1, $2)", after.OccurredAt, after.ID)
	}
}

// trim, istenenden bir fazla okunan listeyi keser ve sonraki sayfa olup olmadığını söyler.
func trim[T any](items []T, limit int) ([]T, bool, error) {
	if len(items) > limit {
		return items[:limit], true, nil
	}
	return items, false, nil
}
