package lms

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// Forum türleri: Duyurular (her ders alanında bir tane; konuyu ve yanıtı öğretim elemanı yazar)
// ve genel forum (öğretim elemanı ekler; öğrencilerin konu açması ayarlanabilir).
const (
	ForumAnnouncements = "ANNOUNCEMENTS"
	ForumGeneral       = "GENERAL"
)

// ForumSettings, forum etkinliğinin ayarlarıdır.
type ForumSettings struct {
	Kind             string
	StudentsCanStart bool
	Threads          int        // silinmemiş konu sayısı (salt okunur)
	LastPostAt       *time.Time // son gönderi (salt okunur)
}

// writeForum, forumun ayarlarını yazar. Duyurular forumunun türü korunur, öğrenciler konu açamaz.
func writeForum(ctx context.Context, tx pgx.Tx, id string, f ForumSettings) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO lms.forums (activity_id, forum_kind, students_can_start_threads) VALUES ($1, 'GENERAL', $2)
		ON CONFLICT (activity_id) DO UPDATE
		SET students_can_start_threads = excluded.students_can_start_threads AND lms.forums.forum_kind <> 'ANNOUNCEMENTS'`,
		id, f.StudentsCanStart)
	if err != nil {
		return fmt.Errorf("lms: forum yazılamadı: %w", err)
	}
	return nil
}

// forums, etkinliklerin forum ayarları, konu sayıları ve son gönderileridir.
func forums(ctx context.Context, q db.Querier, ids []string) (map[string]ForumSettings, error) {
	rows, err := q.Query(ctx, `
		SELECT f.activity_id, f.forum_kind, f.students_can_start_threads,
		       count(t.id), max(t.last_post_at)
		FROM lms.forums f LEFT JOIN lms.forum_threads t ON t.activity_id = f.activity_id AND t.deleted_at IS NULL
		WHERE f.activity_id = ANY($1)
		GROUP BY f.activity_id`, ids)
	if err != nil {
		return nil, fmt.Errorf("lms: forumlar okunamadı: %w", err)
	}
	out := map[string]ForumSettings{}
	var (
		id string
		f  ForumSettings
	)
	_, err = pgx.ForEachRow(rows, []any{&id, &f.Kind, &f.StudentsCanStart, &f.Threads, &f.LastPostAt}, func() error {
		out[id] = f
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("lms: forumlar okunamadı: %w", err)
	}
	return out, nil
}

// CanStartThread, rolün forumda konu açabildiğini söyler.
func (f ForumSettings) CanStartThread(role Role) bool {
	return role.CanEdit() || (role == RoleStudent && f.Kind == ForumGeneral && f.StudentsCanStart)
}

// CanReply, rolün konuya yanıt yazabildiğini söyler: öğretim elemanı her zaman; öğrenci
// duyurulara yanıt yazamaz, kilitli konuya da.
func (f ForumSettings) CanReply(role Role, locked bool) bool {
	return role.CanEdit() || (role == RoleStudent && f.Kind == ForumGeneral && !locked)
}

// Author, bir gönderinin ya da konunun yazarıdır.
type Author struct {
	ID         string
	Name       string
	Instructor bool // şubenin öğretim elemanı
}

// ThreadSummary, konu listesindeki bir konudur.
type ThreadSummary struct {
	ID         string
	Title      string
	Author     Author
	Pinned     bool
	Locked     bool
	PostCount  int
	LastPostAt time.Time
	CreatedAt  time.Time
}

// Post, bir gönderidir. Silinen gönderinin metni boştur.
type Post struct {
	ID        string
	ParentID  *string
	Author    Author
	Body      string
	CreatedAt time.Time
	EditedAt  *time.Time
	Deleted   bool
}

// Thread, bir konu ve gönderileridir (eskiden yeniye; ilki konunun açılış gönderisi).
type Thread struct {
	ThreadSummary
	ActivityID string
	SectionID  string
	Posts      []Post
}

// authorColumns, yazarın adı ve şubenin öğretim elemanı olup olmadığıdır ($section şube kimliği).
func authorColumns(userCol, sectionExpr string) string {
	return `p.first_name || ' ' || p.last_name,
		EXISTS (SELECT 1 FROM offering.section_instructors si JOIN people.staff st ON st.id = si.staff_id
		        JOIN iam.users iu ON iu.person_id = st.person_id WHERE si.section_id = ` + sectionExpr + ` AND iu.id = ` + userCol + `)`
}

// Threads, forumun konularıdır: sabitlenenler önce, sonra son gönderiye göre.
func (s *Service) Threads(ctx context.Context, sectionID, activityID string) ([]ThreadSummary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT t.id, t.title, t.author_id, `+authorColumns("t.author_id", "$2")+`,
		       t.is_pinned, t.is_locked, t.post_count, t.last_post_at, t.created_at
		FROM lms.forum_threads t
		JOIN iam.users u ON u.id = t.author_id JOIN people.persons p ON p.id = u.person_id
		WHERE t.activity_id = $1 AND t.deleted_at IS NULL
		ORDER BY t.is_pinned DESC, t.last_post_at DESC`, activityID, sectionID)
	if err != nil {
		return nil, fmt.Errorf("lms: konular okunamadı: %w", err)
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (ThreadSummary, error) {
		var t ThreadSummary
		err := row.Scan(&t.ID, &t.Title, &t.Author.ID, &t.Author.Name, &t.Author.Instructor,
			&t.Pinned, &t.Locked, &t.PostCount, &t.LastPostAt, &t.CreatedAt)
		return t, err
	})
	if err != nil {
		return nil, fmt.Errorf("lms: konular okunamadı: %w", err)
	}
	return list, nil
}

// postBody, gönderinin temizlenmiş metnidir; boş ya da çok uzunsa doğrulama hatası.
func postBody(html string) (string, error) {
	if len(html) > 100_000 {
		return "", &ValidationError{Field: "body_html", Message: "Metin çok uzun."}
	}
	body := Sanitize(html)
	if emptyHTML(body) {
		return "", &ValidationError{Field: "body_html", Message: "Bir metin yazın."}
	}
	return body, nil
}

// CreateThread, foruma bir konu açar (ilk gönderisiyle).
func (s *Service) CreateThread(ctx context.Context, actorID, activityID, title, html string) (string, error) {
	if n := len([]rune(title)); n < 1 || n > 200 {
		return "", &ValidationError{Field: "title", Message: "1 ile 200 karakter arasında olmalı."}
	}
	body, err := postBody(html)
	if err != nil {
		return "", err
	}
	var id string
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			INSERT INTO lms.forum_threads (activity_id, author_id, title, post_count) VALUES ($1, $2, $3, 1) RETURNING id`,
			activityID, actorID, title).Scan(&id); err != nil {
			return fmt.Errorf("lms: konu açılamadı: %w", err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO lms.forum_posts (thread_id, author_id, body_html) VALUES ($1, $2, $3)`, id, actorID, body); err != nil {
			return fmt.Errorf("lms: gönderi yazılamadı: %w", err)
		}
		return audit.Record(ctx, tx, audit.Entry{ActorUserID: actorID, Action: "lms_thread.create", EntityType: "lms_thread", EntityID: id,
			After: map[string]any{"activity_id": activityID, "title": title}})
	})
	return id, err
}

// ThreadRef, konunun forumu ve şubesidir; silinmiş konu bulunamaz.
func (s *Service) ThreadRef(ctx context.Context, threadID string) (activityID, sectionID string, locked bool, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT t.activity_id, a.section_id, t.is_locked FROM lms.forum_threads t JOIN lms.activities a ON a.id = t.activity_id
		WHERE t.id = $1 AND t.deleted_at IS NULL AND a.deleted_at IS NULL`, threadID).Scan(&activityID, &sectionID, &locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, ErrNotFound
	}
	return activityID, sectionID, locked, err
}

// Thread, konuyu ve gönderilerini döndürür.
func (s *Service) Thread(ctx context.Context, threadID string) (Thread, error) {
	var t Thread
	err := s.pool.QueryRow(ctx, `
		SELECT t.id, t.activity_id, a.section_id, t.title, t.author_id, `+authorColumns("t.author_id", "a.section_id")+`,
		       t.is_pinned, t.is_locked, t.post_count, t.last_post_at, t.created_at
		FROM lms.forum_threads t JOIN lms.activities a ON a.id = t.activity_id
		JOIN iam.users u ON u.id = t.author_id JOIN people.persons p ON p.id = u.person_id
		WHERE t.id = $1 AND t.deleted_at IS NULL`, threadID).
		Scan(&t.ID, &t.ActivityID, &t.SectionID, &t.Title, &t.Author.ID, &t.Author.Name, &t.Author.Instructor,
			&t.Pinned, &t.Locked, &t.PostCount, &t.LastPostAt, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Thread{}, ErrNotFound
	}
	if err != nil {
		return Thread{}, fmt.Errorf("lms: konu okunamadı: %w", err)
	}
	rows, err := s.pool.Query(ctx, `
		SELECT fp.id, fp.parent_post_id, fp.author_id, `+authorColumns("fp.author_id", "$2")+`,
		       CASE WHEN fp.deleted_at IS NULL THEN fp.body_html ELSE '' END, fp.created_at, fp.edited_at, fp.deleted_at IS NOT NULL
		FROM lms.forum_posts fp
		JOIN iam.users u ON u.id = fp.author_id JOIN people.persons p ON p.id = u.person_id
		WHERE fp.thread_id = $1 ORDER BY fp.created_at, fp.id`, threadID, t.SectionID)
	if err != nil {
		return Thread{}, fmt.Errorf("lms: gönderiler okunamadı: %w", err)
	}
	t.Posts, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Post, error) {
		var p Post
		err := row.Scan(&p.ID, &p.ParentID, &p.Author.ID, &p.Author.Name, &p.Author.Instructor, &p.Body, &p.CreatedAt, &p.EditedAt, &p.Deleted)
		return p, err
	})
	if err != nil {
		return Thread{}, fmt.Errorf("lms: gönderiler okunamadı: %w", err)
	}
	return t, nil
}

// Reply, konuya bir yanıt yazar; parentID boş değilse o gönderiye yanıttır.
func (s *Service) Reply(ctx context.Context, actorID, threadID, parentID, html string) (string, error) {
	body, err := postBody(html)
	if err != nil {
		return "", err
	}
	var id string
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT 1 FROM lms.forum_threads WHERE id = $1 FOR UPDATE`, threadID); err != nil {
			return fmt.Errorf("lms: konu kilitlenemedi: %w", err)
		}
		var parent any
		if parentID != "" {
			var ok bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM lms.forum_posts WHERE id = $1 AND thread_id = $2)`, parentID, threadID).
				Scan(&ok); err != nil {
				return fmt.Errorf("lms: gönderi okunamadı: %w", err)
			}
			if !ok {
				return &ValidationError{Field: "parent_post_id", Message: "Yanıtlanan gönderi bu konuda değil."}
			}
			parent = parentID
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO lms.forum_posts (thread_id, parent_post_id, author_id, body_html) VALUES ($1, $2, $3, $4) RETURNING id`,
			threadID, parent, actorID, body).Scan(&id); err != nil {
			return fmt.Errorf("lms: yanıt yazılamadı: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE lms.forum_threads SET post_count = post_count + 1, last_post_at = now() WHERE id = $1`, threadID); err != nil {
			return fmt.Errorf("lms: konu güncellenemedi: %w", err)
		}
		return nil
	})
	return id, err
}

// PostRef, gönderinin konusu, yazarı ve konunun ilk gönderisi olup olmadığıdır.
func (s *Service) PostRef(ctx context.Context, postID string) (threadID, authorID string, first bool, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT fp.thread_id, fp.author_id,
		       fp.id = (SELECT f2.id FROM lms.forum_posts f2 WHERE f2.thread_id = fp.thread_id ORDER BY f2.created_at, f2.id LIMIT 1)
		FROM lms.forum_posts fp JOIN lms.forum_threads t ON t.id = fp.thread_id
		WHERE fp.id = $1 AND fp.deleted_at IS NULL AND t.deleted_at IS NULL`, postID).Scan(&threadID, &authorID, &first)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, ErrNotFound
	}
	return threadID, authorID, first, err
}

// EditPost, gönderinin metnini değiştirir (yetkiyi çağıran denetler).
func (s *Service) EditPost(ctx context.Context, actorID, postID, html string) error {
	body, err := postBody(html)
	if err != nil {
		return err
	}
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE lms.forum_posts SET body_html = $2, edited_at = now() WHERE id = $1 AND deleted_at IS NULL`, postID, body)
		if err != nil {
			return fmt.Errorf("lms: gönderi güncellenemedi: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return audit.Record(ctx, tx, audit.Entry{ActorUserID: actorID, Action: "lms_post.update", EntityType: "lms_post", EntityID: postID})
	})
}

// DeletePost, gönderiyi siler: yerinde "silindi" olarak kalır. Konunun ilk gönderisi silinirse
// konu silinir.
func (s *Service) DeletePost(ctx context.Context, actorID, postID string) error {
	threadID, _, first, err := s.PostRef(ctx, postID)
	if err != nil {
		return err
	}
	if first {
		return s.DeleteThread(ctx, actorID, threadID)
	}
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE lms.forum_posts SET deleted_at = now() WHERE id = $1`, postID); err != nil {
			return fmt.Errorf("lms: gönderi silinemedi: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE lms.forum_threads SET post_count = post_count - 1 WHERE id = $1`, threadID); err != nil {
			return fmt.Errorf("lms: konu güncellenemedi: %w", err)
		}
		return audit.Record(ctx, tx, audit.Entry{ActorUserID: actorID, Action: "lms_post.delete", EntityType: "lms_post", EntityID: postID})
	})
}

// DeleteThread, konuyu siler (yumuşak).
func (s *Service) DeleteThread(ctx context.Context, actorID, threadID string) error {
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE lms.forum_threads SET deleted_at = now() WHERE id = $1 AND deleted_at IS NULL`, threadID)
		if err != nil {
			return fmt.Errorf("lms: konu silinemedi: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return audit.Record(ctx, tx, audit.Entry{ActorUserID: actorID, Action: "lms_thread.delete", EntityType: "lms_thread", EntityID: threadID})
	})
}

// ThreadUpdate, konunun değişen alanlarıdır (nil olanlar değişmez).
type ThreadUpdate struct {
	Title  *string
	Pinned *bool
	Locked *bool
}

// UpdateThread, konunun başlığını, sabitlenmesini ya da kilidini değiştirir (yetkiyi çağıran denetler).
func (s *Service) UpdateThread(ctx context.Context, actorID, threadID string, in ThreadUpdate) error {
	if in.Title != nil {
		if n := len([]rune(*in.Title)); n < 1 || n > 200 {
			return &ValidationError{Field: "title", Message: "1 ile 200 karakter arasında olmalı."}
		}
	}
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE lms.forum_threads SET title = coalesce($2, title), is_pinned = coalesce($3, is_pinned), is_locked = coalesce($4, is_locked)
			WHERE id = $1 AND deleted_at IS NULL`, threadID, in.Title, in.Pinned, in.Locked)
		if err != nil {
			return fmt.Errorf("lms: konu güncellenemedi: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return audit.Record(ctx, tx, audit.Entry{ActorUserID: actorID, Action: "lms_thread.update", EntityType: "lms_thread", EntityID: threadID,
			After: map[string]any{"title": in.Title, "pinned": in.Pinned, "locked": in.Locked}})
	})
}
