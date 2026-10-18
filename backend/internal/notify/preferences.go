package notify

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// Preference, bir bildirim türünün kullanıcıdaki kanallarıdır.
type Preference struct {
	Type      Type
	Category  string
	NameTR    string
	NameEN    string
	Mandatory bool
	InApp     bool
	Email     bool
}

// PreferenceInput, bir türün istenen kanallarıdır.
type PreferenceInput struct {
	Type  Type
	InApp bool
	Email bool
}

// ErrMandatory: zorunlu bir türün kanalı kapatılmak istendi.
var ErrMandatory = errors.New("notify: zorunlu bildirim kapatılamaz")

// ErrUnknownType: katalogda olmayan bir tür.
var ErrUnknownType = errors.New("notify: bilinmeyen bildirim türü")

// Preferences, kullanıcının bütün türlerdeki kanallarını katalog sırasıyla döndürür.
func (c *Center) Preferences(ctx context.Context, userID string) ([]Preference, error) {
	return preferences(ctx, c.pool, userID)
}

func preferences(ctx context.Context, q db.Querier, userID string) ([]Preference, error) {
	rows, err := q.Query(ctx, `
		SELECT t.code, t.category, t.name_tr, t.name_en, t.is_mandatory,
		       CASE WHEN t.is_mandatory THEN 'IN_APP' = ANY (t.default_channels)
		            ELSE coalesce(pa.enabled, 'IN_APP' = ANY (t.default_channels)) END,
		       CASE WHEN t.is_mandatory THEN 'EMAIL' = ANY (t.default_channels)
		            ELSE coalesce(pe.enabled, 'EMAIL' = ANY (t.default_channels)) END
		FROM communication.notification_types t
		LEFT JOIN communication.notification_preferences pa ON pa.user_id = $1 AND pa.type_code = t.code AND pa.channel = 'IN_APP'
		LEFT JOIN communication.notification_preferences pe ON pe.user_id = $1 AND pe.type_code = t.code AND pe.channel = 'EMAIL'
		ORDER BY t.sort_order, t.code`, userID)
	if err != nil {
		return nil, fmt.Errorf("notify: tercihler okunamadı: %w", err)
	}
	var (
		out []Preference
		p   Preference
		typ string
	)
	if _, err := pgx.ForEachRow(rows, []any{&typ, &p.Category, &p.NameTR, &p.NameEN, &p.Mandatory, &p.InApp, &p.Email}, func() error {
		p.Type = Type(typ)
		out = append(out, p)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("notify: tercihler okunamadı: %w", err)
	}
	return out, nil
}

// SetPreferences, verilen türlerin kanallarını yazar ve güncel tercihleri döndürür. Zorunlu
// bir türde varsayılandan farklı bir değer ErrMandatory, bilinmeyen tür ErrUnknownType döner;
// o zaman hiçbir değişiklik yazılmaz.
func (c *Center) SetPreferences(ctx context.Context, userID string, in []PreferenceInput) ([]Preference, error) {
	var out []Preference
	err := db.InTx(ctx, c.pool, func(tx pgx.Tx) error {
		current, err := preferences(ctx, tx, userID)
		if err != nil {
			return err
		}
		byType := make(map[Type]Preference, len(current))
		for _, p := range current {
			byType[p.Type] = p
		}
		for _, p := range in {
			cur, ok := byType[p.Type]
			switch {
			case !ok:
				return fmt.Errorf("%w: %s", ErrUnknownType, p.Type)
			case cur.Mandatory && (p.InApp != cur.InApp || p.Email != cur.Email):
				return fmt.Errorf("%w: %s", ErrMandatory, p.Type)
			case cur.Mandatory:
				continue
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO communication.notification_preferences (user_id, type_code, channel, enabled, updated_at)
				VALUES ($1, $2, 'IN_APP', $3, $5), ($1, $2, 'EMAIL', $4, $5)
				ON CONFLICT (user_id, type_code, channel) DO UPDATE SET enabled = excluded.enabled, updated_at = excluded.updated_at`,
				userID, string(p.Type), p.InApp, p.Email, c.now()); err != nil {
				return fmt.Errorf("notify: tercih yazılamadı: %w", err)
			}
		}
		out, err = preferences(ctx, tx, userID)
		return err
	})
	return out, err
}
