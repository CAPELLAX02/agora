package lms

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/files"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// ValidationError, isteğin alan hatalarıdır.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return "lms: " + e.Field + ": " + e.Message }

// ensureSpace, şubenin ders alanını yoksa oluşturur: ilk bölüm "Genel" (e-Kampüs'teki gibi).
func ensureSpace(ctx context.Context, tx pgx.Tx, sectionID string) error {
	tag, err := tx.Exec(ctx, `INSERT INTO lms.course_spaces (section_id) VALUES ($1) ON CONFLICT DO NOTHING`, sectionID)
	if err != nil {
		return fmt.Errorf("lms: ders alanı oluşturulamadı: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `INSERT INTO lms.topics (section_id, title, position) VALUES ($1, 'Genel', 0)`, sectionID); err != nil {
		return fmt.Errorf("lms: ilk bölüm oluşturulamadı: %w", err)
	}
	return nil
}

// lockSpace, ders alanının yapısını değiştiren işlemleri sıraya sokar (sıra numaraları).
func lockSpace(ctx context.Context, tx pgx.Tx, sectionID string) error {
	if err := ensureSpace(ctx, tx, sectionID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `SELECT 1 FROM lms.course_spaces WHERE section_id = $1 FOR UPDATE`, sectionID)
	return err
}

// SetSummary, ders alanının tanıtım metnini yazar.
func (s *Service) SetSummary(ctx context.Context, actorID, sectionID, html string) error {
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := ensureSpace(ctx, tx, sectionID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE lms.course_spaces SET summary_html = $2, updated_at = now() WHERE section_id = $1`,
			sectionID, Sanitize(html)); err != nil {
			return fmt.Errorf("lms: tanıtım yazılamadı: %w", err)
		}
		return audit.Record(ctx, tx, audit.Entry{ActorUserID: actorID, Action: "lms_space.update", EntityType: "section", EntityID: sectionID})
	})
}

// TopicInput, bir bölümün yazılabilir alanlarıdır.
type TopicInput struct {
	Title   string
	Summary string
	Visible bool
	Window  Window
}

func (in TopicInput) validate() error {
	if n := len([]rune(in.Title)); n < 1 || n > 200 {
		return &ValidationError{Field: "title", Message: "1 ile 200 karakter arasında olmalı."}
	}
	return in.Window.validate()
}

func (w Window) validate() error {
	if w.From != nil && w.Until != nil && !w.From.Before(*w.Until) {
		return &ValidationError{Field: "available_until", Message: "Açılıştan sonra olmalı."}
	}
	return nil
}

// CreateTopic, ders alanının sonuna bir bölüm ekler.
func (s *Service) CreateTopic(ctx context.Context, actorID, sectionID string, in TopicInput) (string, error) {
	if err := in.validate(); err != nil {
		return "", err
	}
	var id string
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := lockSpace(ctx, tx, sectionID); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `
			INSERT INTO lms.topics (section_id, title, summary_html, is_visible, available_from, available_until, position)
			VALUES ($1, $2, $3, $4, $5, $6,
			        (SELECT coalesce(max(position), -1) + 1 FROM lms.topics WHERE section_id = $1 AND deleted_at IS NULL))
			RETURNING id`, sectionID, in.Title, Sanitize(in.Summary), in.Visible, in.Window.From, in.Window.Until).Scan(&id)
		if err != nil {
			return fmt.Errorf("lms: bölüm eklenemedi: %w", err)
		}
		return audit.Record(ctx, tx, audit.Entry{ActorUserID: actorID, Action: "lms_topic.create", EntityType: "lms_topic", EntityID: id,
			After: map[string]any{"section_id": sectionID, "title": in.Title}})
	})
	return id, err
}

// TopicSection, bölümün şubesidir.
func (s *Service) TopicSection(ctx context.Context, id string) (string, error) {
	var sectionID string
	err := s.pool.QueryRow(ctx, `SELECT section_id FROM lms.topics WHERE id = $1 AND deleted_at IS NULL`, id).Scan(&sectionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return sectionID, err
}

// UpdateTopic, bölümü günceller (iyimser kilit: version).
func (s *Service) UpdateTopic(ctx context.Context, actorID, id string, version int, in TopicInput) error {
	if err := in.validate(); err != nil {
		return err
	}
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE lms.topics SET title = $3, summary_html = $4, is_visible = $5, available_from = $6, available_until = $7,
			       version = version + 1, updated_at = now()
			WHERE id = $1 AND version = $2 AND deleted_at IS NULL`,
			id, version, in.Title, Sanitize(in.Summary), in.Visible, in.Window.From, in.Window.Until)
		if err != nil {
			return fmt.Errorf("lms: bölüm güncellenemedi: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrVersionMismatch
		}
		return audit.Record(ctx, tx, audit.Entry{ActorUserID: actorID, Action: "lms_topic.update", EntityType: "lms_topic", EntityID: id,
			After: map[string]any{"title": in.Title, "visible": in.Visible}})
	})
}

// DeleteTopic, bölümü ve içindeki etkinlikleri siler (yumuşak); etkinliklerin dosyaları
// serbest kalır ve bir gün sonra temizlenir. Son bölüm silinemez.
func (s *Service) DeleteTopic(ctx context.Context, actorID, id string) error {
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var sectionID string
		if err := tx.QueryRow(ctx, `SELECT section_id FROM lms.topics WHERE id = $1 AND deleted_at IS NULL`, id).Scan(&sectionID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("lms: bölüm okunamadı: %w", err)
		}
		if err := lockSpace(ctx, tx, sectionID); err != nil {
			return err
		}
		var others int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM lms.topics WHERE section_id = $1 AND deleted_at IS NULL AND id <> $2`,
			sectionID, id).Scan(&others); err != nil {
			return fmt.Errorf("lms: bölümler okunamadı: %w", err)
		}
		if others == 0 {
			return &ValidationError{Field: "topic", Message: "Ders alanında en az bir bölüm kalmalı."}
		}
		if err := releaseFiles(ctx, tx, `SELECT af.file_id FROM lms.activity_files af JOIN lms.activities a ON a.id = af.activity_id
			WHERE a.topic_id = $1 AND a.deleted_at IS NULL`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE lms.activities SET deleted_at = now() WHERE topic_id = $1 AND deleted_at IS NULL`, id); err != nil {
			return fmt.Errorf("lms: etkinlikler silinemedi: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE lms.topics SET deleted_at = now() WHERE id = $1`, id); err != nil {
			return fmt.Errorf("lms: bölüm silinemedi: %w", err)
		}
		return audit.Record(ctx, tx, audit.Entry{ActorUserID: actorID, Action: "lms_topic.delete", EntityType: "lms_topic", EntityID: id})
	})
}

// releaseFiles, sorgunun döndürdüğü dosyaları serbest bırakır: bağlanmamış sayılırlar ve
// temizlik işi bir gün sonra siler.
func releaseFiles(ctx context.Context, tx pgx.Tx, query string, args ...any) error {
	if _, err := tx.Exec(ctx, `UPDATE platform.files SET attached_at = NULL WHERE id IN (`+query+`)`, args...); err != nil {
		return fmt.Errorf("lms: dosyalar serbest bırakılamadı: %w", err)
	}
	return nil
}

// Layout, ders alanının sırasıdır: bölümler ve her birinin etkinlikleri, yukarıdan aşağıya.
type Layout []struct {
	TopicID     string
	ActivityIDs []string
}

// Reorder, ders alanını verilen sıraya dizer; etkinlikler bölümler arasında taşınabilir.
// Sıra ders alanının bütün bölümlerini ve etkinliklerini birer kez içermelidir: arada
// başkasının eklediği bir öğe varsa sıra eskimiştir.
func (s *Service) Reorder(ctx context.Context, actorID, sectionID string, layout Layout) error {
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := lockSpace(ctx, tx, sectionID); err != nil {
			return err
		}
		topics, err := s.topics(ctx, tx, sectionID)
		if err != nil {
			return err
		}
		var wantTopics, wantActs, gotTopics, gotActs []string
		for _, t := range topics {
			wantTopics = append(wantTopics, t.ID)
			for _, a := range t.Activities {
				wantActs = append(wantActs, a.ID)
			}
		}
		for _, t := range layout {
			gotTopics = append(gotTopics, t.TopicID)
			gotActs = append(gotActs, t.ActivityIDs...)
		}
		slices.Sort(wantTopics)
		slices.Sort(wantActs)
		slices.Sort(gotTopics)
		slices.Sort(gotActs)
		if !slices.Equal(wantTopics, gotTopics) || !slices.Equal(wantActs, gotActs) {
			return ErrVersionMismatch
		}
		for i, t := range layout {
			if _, err := tx.Exec(ctx, `UPDATE lms.topics SET position = $2 WHERE id = $1 AND position <> $2`, t.TopicID, i); err != nil {
				return fmt.Errorf("lms: sıra yazılamadı: %w", err)
			}
			for j, a := range t.ActivityIDs {
				if _, err := tx.Exec(ctx, `
					UPDATE lms.activities SET topic_id = $2, position = $3
					WHERE id = $1 AND (topic_id <> $2 OR position <> $3)`, a, t.TopicID, j); err != nil {
					return fmt.Errorf("lms: sıra yazılamadı: %w", err)
				}
			}
		}
		return audit.Record(ctx, tx, audit.Entry{ActorUserID: actorID, Action: "lms_space.reorder", EntityType: "section", EntityID: sectionID})
	})
}

// ActivityInput, bir etkinliğin yazılabilir alanlarıdır. Türe özgü olanlar sadece o türde
// kullanılır: FileIDs (dosya: 1, klasör: 1-50), URL ve NewTab (bağlantı), Body (sayfa),
// Assignment (ödev).
type ActivityInput struct {
	Type        ActivityType
	Title       string
	Description string
	Visible     bool
	Window      Window
	FileIDs     []string
	URL         string
	NewTab      bool
	Body        string
	Assignment  *AssignmentSettings
}

// validate, girdiyi denetler; maxUpload platformun dosya boyutu sınırıdır (ödevin sınırı onu aşamaz).
func (in ActivityInput) validate(maxUpload int64) error {
	if n := len([]rune(in.Title)); n < 1 || n > 200 {
		return &ValidationError{Field: "title", Message: "1 ile 200 karakter arasında olmalı."}
	}
	switch in.Type {
	case TypeResource:
		if len(in.FileIDs) != 1 {
			return &ValidationError{Field: "file_ids", Message: "Dosya etkinliğinde tek dosya olmalı."}
		}
	case TypeFolder:
		if len(in.FileIDs) < 1 || len(in.FileIDs) > 50 {
			return &ValidationError{Field: "file_ids", Message: "Klasörde 1 ile 50 arasında dosya olmalı."}
		}
	case TypeURL:
		if !validURL(in.URL) {
			return &ValidationError{Field: "url", Message: "http ya da https ile başlayan geçerli bir adres olmalı."}
		}
	case TypeAssignment:
		if in.Assignment == nil {
			return &ValidationError{Field: "assignment", Message: "Ödevin ayarları gerekli."}
		}
		in.Assignment.normalize()
		if err := in.Assignment.validate(maxUpload); err != nil {
			return err
		}
	case TypePage, TypeLabel:
	default:
		return &ValidationError{Field: "type", Message: "RESOURCE, FOLDER, URL, PAGE, LABEL ya da ASSIGNMENT olmalı."}
	}
	if len(in.Body) > 200_000 || len(in.Description) > 50_000 {
		return &ValidationError{Field: "body", Message: "Metin çok uzun."}
	}
	return in.Window.validate()
}

// CreateActivity, bölümün sonuna bir etkinlik ekler; dosyaları etkinliğe bağlar (yükleyen
// işlemi yapan olmalı).
func (s *Service) CreateActivity(ctx context.Context, actorID, topicID string, in ActivityInput) (string, error) {
	if err := in.validate(s.files.MaxBytes()); err != nil {
		return "", err
	}
	var id string
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var sectionID string
		if err := tx.QueryRow(ctx, `SELECT section_id FROM lms.topics WHERE id = $1 AND deleted_at IS NULL`, topicID).Scan(&sectionID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("lms: bölüm okunamadı: %w", err)
		}
		if err := lockSpace(ctx, tx, sectionID); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `
			INSERT INTO lms.activities (section_id, topic_id, activity_type, title, description_html, is_visible,
			                            available_from, available_until, created_by, position)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9,
			        (SELECT coalesce(max(position), -1) + 1 FROM lms.activities WHERE topic_id = $2 AND deleted_at IS NULL))
			RETURNING id`, sectionID, topicID, string(in.Type), in.Title, Sanitize(in.Description), in.Visible,
			in.Window.From, in.Window.Until, actorID).Scan(&id)
		if err != nil {
			return fmt.Errorf("lms: etkinlik eklenemedi: %w", err)
		}
		if err := s.writeDetails(ctx, tx, actorID, id, in, nil); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorUserID: actorID, Action: "lms_activity.create", EntityType: "lms_activity", EntityID: id,
			After: map[string]any{"section_id": sectionID, "type": in.Type, "title": in.Title}})
	})
	return id, err
}

// writeDetails, türe özgü tabloları yazar. current, etkinliğin şimdiki dosyalarıdır: listede
// kalmayanlar serbest bırakılır, yeniler bağlanır.
func (s *Service) writeDetails(ctx context.Context, tx pgx.Tx, actorID, id string, in ActivityInput, current []string) error {
	switch in.Type {
	case TypeURL:
		_, err := tx.Exec(ctx, `
			INSERT INTO lms.urls (activity_id, url, open_in_new_tab) VALUES ($1, $2, $3)
			ON CONFLICT (activity_id) DO UPDATE SET url = excluded.url, open_in_new_tab = excluded.open_in_new_tab`, id, in.URL, in.NewTab)
		if err != nil {
			return fmt.Errorf("lms: bağlantı yazılamadı: %w", err)
		}
	case TypeAssignment:
		return writeAssignment(ctx, tx, id, *in.Assignment)
	case TypePage:
		_, err := tx.Exec(ctx, `
			INSERT INTO lms.pages (activity_id, body_html) VALUES ($1, $2)
			ON CONFLICT (activity_id) DO UPDATE SET body_html = excluded.body_html`, id, Sanitize(in.Body))
		if err != nil {
			return fmt.Errorf("lms: sayfa yazılamadı: %w", err)
		}
	case TypeResource, TypeFolder:
		var removed []string
		for _, f := range current {
			if !slices.Contains(in.FileIDs, f) {
				removed = append(removed, f)
			}
		}
		if len(removed) > 0 {
			if _, err := tx.Exec(ctx, `DELETE FROM lms.activity_files WHERE activity_id = $1 AND file_id = ANY($2)`, id, removed); err != nil {
				return fmt.Errorf("lms: dosyalar çıkarılamadı: %w", err)
			}
			if err := releaseFiles(ctx, tx, `SELECT unnest($1::uuid[])`, removed); err != nil {
				return err
			}
		}
		for i, f := range in.FileIDs {
			if !slices.Contains(current, f) {
				if _, err := s.files.Attach(ctx, tx, f, actorID); err != nil {
					if errors.Is(err, files.ErrNotFound) || errors.Is(err, files.ErrAttached) {
						return &ValidationError{Field: "file_ids", Message: "Dosya bulunamadı ya da başka bir kayda bağlı: " + f}
					}
					return err
				}
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO lms.activity_files (activity_id, file_id, position) VALUES ($1, $2, $3)
				ON CONFLICT (activity_id, file_id) DO UPDATE SET position = excluded.position`, id, f, i); err != nil {
				return fmt.Errorf("lms: dosya bağlanamadı: %w", err)
			}
		}
	}
	return nil
}

// ActivitySection, etkinliğin şubesidir.
func (s *Service) ActivitySection(ctx context.Context, id string) (string, ActivityType, error) {
	var sectionID, typ string
	err := s.pool.QueryRow(ctx, `SELECT section_id, activity_type FROM lms.activities WHERE id = $1 AND deleted_at IS NULL`, id).
		Scan(&sectionID, &typ)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	return sectionID, ActivityType(typ), err
}

// UpdateActivity, etkinliği günceller (türü değişmez; iyimser kilit: version).
func (s *Service) UpdateActivity(ctx context.Context, actorID, id string, version int, in ActivityInput) error {
	if err := in.validate(s.files.MaxBytes()); err != nil {
		return err
	}
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var typ string
		if err := tx.QueryRow(ctx, `SELECT activity_type FROM lms.activities WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, id).Scan(&typ); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return fmt.Errorf("lms: etkinlik okunamadı: %w", err)
		}
		if ActivityType(typ) != in.Type {
			return &ValidationError{Field: "type", Message: "Etkinliğin türü değiştirilemez."}
		}
		tag, err := tx.Exec(ctx, `
			UPDATE lms.activities SET title = $3, description_html = $4, is_visible = $5, available_from = $6,
			       available_until = $7, version = version + 1, updated_at = now()
			WHERE id = $1 AND version = $2`,
			id, version, in.Title, Sanitize(in.Description), in.Visible, in.Window.From, in.Window.Until)
		if err != nil {
			return fmt.Errorf("lms: etkinlik güncellenemedi: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrVersionMismatch
		}
		rows, err := tx.Query(ctx, `SELECT file_id FROM lms.activity_files WHERE activity_id = $1`, id)
		if err != nil {
			return fmt.Errorf("lms: dosyalar okunamadı: %w", err)
		}
		current, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return fmt.Errorf("lms: dosyalar okunamadı: %w", err)
		}
		if err := s.writeDetails(ctx, tx, actorID, id, in, current); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorUserID: actorID, Action: "lms_activity.update", EntityType: "lms_activity", EntityID: id,
			After: map[string]any{"title": in.Title, "visible": in.Visible}})
	})
}

// DeleteActivity, etkinliği siler (yumuşak); dosyaları serbest kalır.
func (s *Service) DeleteActivity(ctx context.Context, actorID, id string) error {
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE lms.activities SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`, id)
		if err != nil {
			return fmt.Errorf("lms: etkinlik silinemedi: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		if err := releaseFiles(ctx, tx, `SELECT file_id FROM lms.activity_files WHERE activity_id = $1`, id); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{ActorUserID: actorID, Action: "lms_activity.delete", EntityType: "lms_activity", EntityID: id})
	})
}

// validURL, http ya da https şemalı, ana bilgisayarı olan bir adrestir.
func validURL(raw string) bool {
	if len(raw) > 2000 {
		return false
	}
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
