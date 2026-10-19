package messaging

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/notify"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// messageRef, iletinin konuşması, yazarı ve silinip silinmediğidir.
func messageRef(ctx context.Context, q db.Querier, messageID string) (conversationID, senderID string, deleted, hasFiles bool, err error) {
	err = q.QueryRow(ctx, `
		SELECT conversation_id, sender_id, deleted_at IS NOT NULL,
		       EXISTS (SELECT 1 FROM communication.message_attachments a WHERE a.message_id = m.id)
		FROM communication.messages m WHERE id = $1 FOR UPDATE`, messageID).Scan(&conversationID, &senderID, &deleted, &hasFiles)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, false, ErrNotFound
	}
	if err != nil {
		err = fmt.Errorf("messaging: ileti okunamadı: %w", err)
	}
	return conversationID, senderID, deleted, hasFiles, err
}

// announce, konuşmanın değiştiğini (düzenlenen, silinen ileti) üyelerin açık akışlarına duyurur.
func announce(ctx context.Context, tx pgx.Tx, conversationID, actorID string) error {
	var kind, sectionID string
	if err := tx.QueryRow(ctx, `
		SELECT conversation_type, coalesce(section_id::text, '') FROM communication.conversations WHERE id = $1`,
		conversationID).Scan(&kind, &sectionID); err != nil {
		return fmt.Errorf("messaging: konuşma okunamadı: %w", err)
	}
	to, err := recipients(ctx, tx, conversationID, kind, sectionID, actorID)
	if err != nil {
		return err
	}
	return notify.AnnounceMessages(ctx, tx, to, conversationID)
}

// EditMessage, iletinin metnini değiştirir: sadece yazarı, silinmemiş iletide.
func (s *Service) EditMessage(ctx context.Context, actorID, messageID, body string) error {
	body = strings.TrimSpace(body)
	if utf8.RuneCountInString(body) > MaxBody {
		return &ValidationError{"body", fmt.Sprintf("En çok %d karakter olmalı.", MaxBody)}
	}
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		conv, sender, deleted, hasFiles, err := messageRef(ctx, tx, messageID)
		if err != nil {
			return err
		}
		if _, _, err := access(ctx, tx, conv, actorID); err != nil {
			return err
		}
		if sender != actorID || deleted {
			return ErrNotAllowed
		}
		if body == "" && !hasFiles {
			return &ValidationError{"body", "İleti metni ya da eki olmalı."}
		}
		if _, err := tx.Exec(ctx, `UPDATE communication.messages SET body = $2, edited_at = now() WHERE id = $1`, messageID, body); err != nil {
			return fmt.Errorf("messaging: ileti güncellenemedi: %w", err)
		}
		return announce(ctx, tx, conv, actorID)
	})
}

// DeleteMessage, iletiyi siler (yerinde "silindi" olarak kalır): yazarı ya da konuşmanın
// yöneticisi (grubu kuran, şubenin öğretim elemanı).
func (s *Service) DeleteMessage(ctx context.Context, actorID, messageID string) error {
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		conv, sender, deleted, _, err := messageRef(ctx, tx, messageID)
		if err != nil {
			return err
		}
		_, moderator, err := access(ctx, tx, conv, actorID)
		if err != nil {
			return err
		}
		if sender != actorID && !moderator {
			return ErrNotAllowed
		}
		if deleted {
			return nil
		}
		if _, err := tx.Exec(ctx, `UPDATE communication.messages SET deleted_at = now() WHERE id = $1`, messageID); err != nil {
			return fmt.Errorf("messaging: ileti silinemedi: %w", err)
		}
		return announce(ctx, tx, conv, actorID)
	})
}

// State, kullanıcının konuşmadaki kişisel durumudur.
type State struct {
	Starred    bool
	MutedUntil *time.Time // boşsa sessiz değil
}

// SetState, konuşmayı yıldızlar ya da süreyle sessize alır (şube grubunda üyelik satırını açar).
func (s *Service) SetState(ctx context.Context, userID, conversationID string, st State) error {
	if _, _, err := access(ctx, s.pool, conversationID, userID); err != nil {
		return err
	}
	if st.MutedUntil != nil && !st.MutedUntil.After(s.now()) {
		st.MutedUntil = nil
	}
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO communication.conversation_members (conversation_id, user_id, is_starred, muted_until) VALUES ($1, $2, $3, $4)
		ON CONFLICT (conversation_id, user_id) DO UPDATE SET is_starred = excluded.is_starred, muted_until = excluded.muted_until`,
		conversationID, userID, st.Starred, st.MutedUntil); err != nil {
		return fmt.Errorf("messaging: durum yazılamadı: %w", err)
	}
	return nil
}

// Leave, kullanıcının gruptan ayrılmasıdır; birebir ve şube grubundan ayrılınmaz (sessize alınır).
func (s *Service) Leave(ctx context.Context, userID, conversationID string) error {
	kind, _, err := access(ctx, s.pool, conversationID, userID)
	if err != nil {
		return err
	}
	if kind != KindGroup {
		return ErrNotAllowed
	}
	if _, err := s.pool.Exec(ctx, `
		UPDATE communication.conversation_members SET left_at = now() WHERE conversation_id = $1 AND user_id = $2`,
		conversationID, userID); err != nil {
		return fmt.Errorf("messaging: gruptan ayrılınamadı: %w", err)
	}
	return nil
}

// AddMembers, grubun yöneticisinin gruba kişi eklemesidir (ayrılmış olan yeniden katılır).
func (s *Service) AddMembers(ctx context.Context, actorID, conversationID string, userIDs []string) error {
	kind, moderator, err := access(ctx, s.pool, conversationID, actorID)
	if err != nil {
		return err
	}
	if kind != KindGroup || !moderator {
		return ErrNotAllowed
	}
	users := slices.Compact(slices.Sorted(slices.Values(userIDs)))
	if len(users) == 0 || len(users) > 200 {
		return &ValidationError{"user_ids", "1 ile 200 arasında kişi seçin."}
	}
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var found int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM iam.users WHERE id = ANY ($1::uuid[]) AND status = 'ACTIVE'`, users).Scan(&found); err != nil {
			return fmt.Errorf("messaging: kişiler okunamadı: %w", err)
		}
		if found != len(users) {
			return &ValidationError{"user_ids", "Kişilerden biri bulunamadı."}
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO communication.conversation_members (conversation_id, user_id)
			SELECT $1, u FROM unnest($2::uuid[]) AS u
			ON CONFLICT (conversation_id, user_id) DO UPDATE SET left_at = NULL, joined_at = now()
			WHERE communication.conversation_members.left_at IS NOT NULL`, conversationID, users); err != nil {
			return fmt.Errorf("messaging: üyeler eklenemedi: %w", err)
		}
		return nil
	})
}

// AttachmentURL, iletinin ekinin kısa ömürlü imzalı indirme adresidir; konuşmaya erişen indirir,
// silinmiş iletinin eki indirilmez.
func (s *Service) AttachmentURL(ctx context.Context, userID, messageID, fileID string) (string, time.Time, error) {
	if s.files == nil {
		return "", time.Time{}, ErrNoStorage
	}
	var (
		conv    string
		deleted bool
	)
	err := s.pool.QueryRow(ctx, `
		SELECT m.conversation_id, m.deleted_at IS NOT NULL
		FROM communication.messages m JOIN communication.message_attachments a ON a.message_id = m.id
		WHERE m.id = $1 AND a.file_id = $2`, messageID, fileID).Scan(&conv, &deleted)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", time.Time{}, ErrNotFound
	}
	if err != nil {
		return "", time.Time{}, fmt.Errorf("messaging: ek okunamadı: %w", err)
	}
	if _, _, err := access(ctx, s.pool, conv, userID); err != nil {
		return "", time.Time{}, err
	}
	if deleted {
		return "", time.Time{}, ErrNotFound
	}
	f, err := s.files.Get(ctx, s.pool, fileID)
	if err != nil {
		return "", time.Time{}, err
	}
	return s.files.DownloadURL(ctx, f)
}
