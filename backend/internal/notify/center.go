package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Item, bildirim merkezinde gösterilen bir bildirimdir.
type Item struct {
	ID         int64
	CreatedAt  time.Time
	Type       Type
	Category   string
	Text       Text
	Link       string
	ReadAt     *time.Time
	ArchivedAt *time.Time
}

// ListFilter, bildirim listesinin süzgecidir.
type ListFilter struct {
	UserID   string
	Unread   bool  // sadece okunmamışlar
	Archived bool  // arşivdekiler (yoksa arşivlenmemişler)
	Before   int64 // cursor: bu kimlikten eskiler; 0 ise baştan
	Limit    int
}

// Center, kullanıcının bildirim merkezidir.
type Center struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

// NewCenter, bir Center oluşturur.
func NewCenter(pool *pgxpool.Pool, now func() time.Time) *Center {
	return &Center{pool: pool, now: now}
}

// List, kullanıcının bildirimlerini en yeniden eskiye döndürür; sonrası varsa hasMore.
func (c *Center) List(ctx context.Context, f ListFilter) (items []Item, hasMore bool, err error) {
	rows, err := c.pool.Query(ctx, `
		SELECT n.id, n.created_at, n.type_code, t.category, t.name_tr, t.name_en, n.payload,
		       coalesce(n.link_path, ''), n.read_at, n.archived_at
		FROM communication.notifications n
		JOIN communication.notification_types t ON t.code = n.type_code
		WHERE n.user_id = $1
		  AND ($2 = 0 OR n.id < $2)
		  AND (n.archived_at IS NOT NULL) = $3
		  AND (NOT $4 OR n.read_at IS NULL)
		ORDER BY n.id DESC
		LIMIT $5`, f.UserID, f.Before, f.Archived, f.Unread, f.Limit+1)
	if err != nil {
		return nil, false, fmt.Errorf("notify: bildirimler okunamadı: %w", err)
	}
	items, err = scanItems(rows)
	if err != nil {
		return nil, false, err
	}
	if len(items) > f.Limit {
		return items[:f.Limit], true, nil
	}
	return items, false, nil
}

// Since, kullanıcının after kimliğinden sonraki (arşivlenmemiş) bildirimleridir, eskiden yeniye;
// canlı akış kaçırılanları bununla gönderir.
func (c *Center) Since(ctx context.Context, userID string, after int64, limit int) ([]Item, error) {
	rows, err := c.pool.Query(ctx, `
		SELECT n.id, n.created_at, n.type_code, t.category, t.name_tr, t.name_en, n.payload,
		       coalesce(n.link_path, ''), n.read_at, n.archived_at
		FROM communication.notifications n
		JOIN communication.notification_types t ON t.code = n.type_code
		WHERE n.user_id = $1 AND n.id > $2 AND n.archived_at IS NULL
		ORDER BY n.id
		LIMIT $3`, userID, after, limit)
	if err != nil {
		return nil, fmt.Errorf("notify: bildirimler okunamadı: %w", err)
	}
	return scanItems(rows)
}

// LastID, kullanıcının en son bildiriminin kimliğidir (yoksa 0).
func (c *Center) LastID(ctx context.Context, userID string) (int64, error) {
	var id int64
	if err := c.pool.QueryRow(ctx, `
		SELECT coalesce(max(id), 0) FROM communication.notifications WHERE user_id = $1`, userID).Scan(&id); err != nil {
		return 0, fmt.Errorf("notify: son bildirim okunamadı: %w", err)
	}
	return id, nil
}

// UnreadCount, kullanıcının okunmamış (ve arşivlenmemiş) bildirim sayısıdır.
func (c *Center) UnreadCount(ctx context.Context, userID string) (int, error) {
	var n int
	if err := c.pool.QueryRow(ctx, `
		SELECT count(*) FROM communication.notifications
		WHERE user_id = $1 AND read_at IS NULL AND archived_at IS NULL`, userID).Scan(&n); err != nil {
		return 0, fmt.Errorf("notify: okunmamışlar sayılamadı: %w", err)
	}
	return n, nil
}

// MarkRead, bildirimi okundu yapar. Zaten okunmuşsa bir şey değişmez.
func (c *Center) MarkRead(ctx context.Context, userID string, id int64) error {
	return c.update(ctx, userID, id, `read_at = coalesce(read_at, $3)`)
}

// Archive, bildirimi arşive kaldırır (okundu da sayılır).
func (c *Center) Archive(ctx context.Context, userID string, id int64) error {
	return c.update(ctx, userID, id, `read_at = coalesce(read_at, $3), archived_at = coalesce(archived_at, $3)`)
}

// Unarchive, bildirimi arşivden geri alır.
func (c *Center) Unarchive(ctx context.Context, userID string, id int64) error {
	return c.update(ctx, userID, id, `archived_at = NULL, read_at = coalesce(read_at, $3)`)
}

func (c *Center) update(ctx context.Context, userID string, id int64, set string) error {
	tag, err := c.pool.Exec(ctx, `UPDATE communication.notifications SET `+set+`
		WHERE id = $1 AND user_id = $2`, id, userID, c.now())
	if err != nil {
		return fmt.Errorf("notify: bildirim güncellenemedi: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkAllRead, kullanıcının bütün okunmamış bildirimlerini okundu yapar ve sayısını döndürür.
func (c *Center) MarkAllRead(ctx context.Context, userID string) (int, error) {
	tag, err := c.pool.Exec(ctx, `
		UPDATE communication.notifications SET read_at = $2
		WHERE user_id = $1 AND read_at IS NULL`, userID, c.now())
	if err != nil {
		return 0, fmt.Errorf("notify: bildirimler okundu yapılamadı: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// scanItems, bildirim satırlarını okur ve metinlerini şablondan üretir.
func scanItems(rows pgx.Rows) ([]Item, error) {
	var (
		items          []Item
		it             Item
		typ            string
		nameTR, nameEN string
		payload        []byte
	)
	_, err := pgx.ForEachRow(rows, []any{&it.ID, &it.CreatedAt, &typ, &it.Category, &nameTR, &nameEN, &payload,
		&it.Link, &it.ReadAt, &it.ArchivedAt}, func() error {
		it.Type = Type(typ)
		var data map[string]any
		if err := json.Unmarshal(payload, &data); err != nil {
			return fmt.Errorf("notify: bildirim %d verisi: %w", it.ID, err)
		}
		it.Text = Render(it.Type, data, Text{TitleTR: nameTR, TitleEN: nameEN})
		items = append(items, it)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("notify: bildirimler okunamadı: %w", err)
	}
	return items, nil
}
