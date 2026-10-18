package announce

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/notify"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

const columns = `an.id, an.title, an.summary, an.body_html, an.announcement_type, an.publish_from, an.publish_until,
	an.is_pinned, an.status, an.author_id, coalesce(p.first_name || ' ' || p.last_name, ''), an.notified_at,
	an.created_at, an.updated_at, an.version`

const joins = `FROM communication.announcements an
	LEFT JOIN iam.users au ON au.id = an.author_id
	LEFT JOIN people.persons p ON p.id = au.person_id`

func scan(row pgx.Row, extra ...any) (Announcement, error) {
	var a Announcement
	err := row.Scan(append([]any{&a.ID, &a.Title, &a.Summary, &a.Body, &a.Type, &a.PublishFrom, &a.PublishUntil,
		&a.Pinned, &a.Status, &a.AuthorID, &a.AuthorName, &a.NotifiedAt, &a.CreatedAt, &a.UpdatedAt, &a.Version}, extra...)...)
	return a, err
}

// audiencesOf, duyuruların hedeflerini birim adlarıyla okur.
func audiencesOf(ctx context.Context, q db.Querier, ids []string) (map[string][]Audience, error) {
	rows, err := q.Query(ctx, `
		SELECT a.announcement_id, a.scope_type, coalesce(a.scope_id::text, ''),
		       coalesce(f.name_tr, d.name_tr, pr.name_tr, ''), a.audience
		FROM communication.announcement_audiences a
		LEFT JOIN org.faculties f ON a.scope_type = 'FACULTY' AND f.id = a.scope_id
		LEFT JOIN org.departments d ON a.scope_type = 'DEPARTMENT' AND d.id = a.scope_id
		LEFT JOIN org.programs pr ON a.scope_type = 'PROGRAM' AND pr.id = a.scope_id
		WHERE a.announcement_id = ANY ($1::uuid[])
		ORDER BY a.announcement_id,
		         array_position(ARRAY['UNIVERSITY', 'FACULTY', 'DEPARTMENT', 'PROGRAM'], a.scope_type), 4, a.audience`, ids)
	if err != nil {
		return nil, fmt.Errorf("announce: hedefler okunamadı: %w", err)
	}
	out := map[string][]Audience{}
	var (
		id string
		a  Audience
	)
	_, err = pgx.ForEachRow(rows, []any{&id, &a.ScopeType, &a.ScopeID, &a.ScopeName, &a.People}, func() error {
		out[id] = append(out[id], a)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("announce: hedefler okunamadı: %w", err)
	}
	return out, nil
}

func withAudiences(ctx context.Context, q db.Querier, list []Announcement) error {
	ids := make([]string, len(list))
	for i, a := range list {
		ids[i] = a.ID
	}
	byID, err := audiencesOf(ctx, q, ids)
	if err != nil {
		return err
	}
	for i := range list {
		list[i].Audiences = byID[list[i].ID]
	}
	return nil
}

// Get, duyuruyu hedefleriyle okur.
func (s *Service) Get(ctx context.Context, id string) (Announcement, error) {
	a, err := scan(s.pool.QueryRow(ctx, `SELECT `+columns+` `+joins+` WHERE an.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return a, ErrNotFound
	}
	if err != nil {
		return a, fmt.Errorf("announce: duyuru okunamadı: %w", err)
	}
	list := []Announcement{a}
	if err := withAudiences(ctx, s.pool, list); err != nil {
		return a, err
	}
	return list[0], nil
}

func writeAudiences(ctx context.Context, tx pgx.Tx, id string, audiences []Audience) error {
	if _, err := tx.Exec(ctx, `DELETE FROM communication.announcement_audiences WHERE announcement_id = $1`, id); err != nil {
		return fmt.Errorf("announce: hedefler silinemedi: %w", err)
	}
	for _, a := range audiences {
		var scopeID any
		if a.ScopeID != "" {
			scopeID = a.ScopeID
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO communication.announcement_audiences (announcement_id, scope_type, scope_id, audience)
			VALUES ($1, $2, $3, $4)`, id, a.ScopeType, scopeID, a.People); err != nil {
			return fmt.Errorf("announce: hedef yazılamadı: %w", err)
		}
	}
	return nil
}

// Create, duyuruyu taslak olarak oluşturur; publish ise hemen yayımlar.
func (s *Service) Create(ctx context.Context, actorID string, in Input, publish bool) (string, error) {
	if err := in.validate(); err != nil {
		return "", err
	}
	from := s.now()
	if in.PublishFrom != nil {
		from = *in.PublishFrom
	}
	if in.PublishUntil != nil && !in.PublishUntil.After(from) {
		return "", &ValidationError{"publish_until", "Yayın bitişi başlangıçtan sonra olmalı."}
	}
	var id string
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			INSERT INTO communication.announcements (title, summary, body_html, announcement_type, publish_from, publish_until,
			                                         is_pinned, author_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
			in.Title, in.Summary, in.Body, in.Type, from, in.PublishUntil, in.Pinned, actorID).Scan(&id); err != nil {
			return fmt.Errorf("announce: duyuru yazılamadı: %w", err)
		}
		if err := writeAudiences(ctx, tx, id, in.Audiences); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Entry{ActorUserID: actorID, Action: "announcement.create", EntityType: "announcement",
			EntityID: id, After: map[string]any{"title": in.Title, "type": in.Type, "audiences": len(in.Audiences)}}); err != nil {
			return err
		}
		if publish {
			return s.publish(ctx, tx, actorID, id)
		}
		return nil
	})
	return id, err
}

// lock, duyuruyu sürümüyle kilitler ve durumunu döndürür.
func lock(ctx context.Context, tx pgx.Tx, id string, version int) (string, error) {
	var status string
	var current int
	err := tx.QueryRow(ctx, `SELECT status, version FROM communication.announcements WHERE id = $1 FOR UPDATE`, id).Scan(&status, &current)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("announce: duyuru okunamadı: %w", err)
	}
	if current != version {
		return "", ErrVersionMismatch
	}
	return status, nil
}

// Update, duyuruyu düzenler (taslak ya da yayında). Bildirilmiş bir duyuru yeniden bildirilmez.
func (s *Service) Update(ctx context.Context, actorID, id string, version int, in Input) error {
	if err := in.validate(); err != nil {
		return err
	}
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		status, err := lock(ctx, tx, id, version)
		if err != nil {
			return err
		}
		if status == StatusArchived {
			return ErrInvalidState
		}
		from := s.now()
		if in.PublishFrom != nil {
			from = *in.PublishFrom
		}
		if in.PublishUntil != nil && !in.PublishUntil.After(from) {
			return &ValidationError{"publish_until", "Yayın bitişi başlangıçtan sonra olmalı."}
		}
		if _, err := tx.Exec(ctx, `
			UPDATE communication.announcements
			SET title = $2, summary = $3, body_html = $4, announcement_type = $5, publish_from = $6, publish_until = $7,
			    is_pinned = $8, version = version + 1, updated_at = now()
			WHERE id = $1`, id, in.Title, in.Summary, in.Body, in.Type, from, in.PublishUntil, in.Pinned); err != nil {
			return fmt.Errorf("announce: duyuru güncellenemedi: %w", err)
		}
		if err := writeAudiences(ctx, tx, id, in.Audiences); err != nil {
			return err
		}
		if err := audit.Record(ctx, tx, audit.Entry{ActorUserID: actorID, Action: "announcement.update", EntityType: "announcement",
			EntityID: id, After: map[string]any{"title": in.Title, "type": in.Type, "audiences": len(in.Audiences)}}); err != nil {
			return err
		}
		if status == StatusPublished {
			return s.notifyIfDue(ctx, tx, id)
		}
		return nil
	})
}

// Publish, taslağı yayımlar; yayın başlamışsa hedef kitleye hemen bildirilir.
func (s *Service) Publish(ctx context.Context, actorID, id string, version int) error {
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		status, err := lock(ctx, tx, id, version)
		if err != nil {
			return err
		}
		if status != StatusDraft {
			return ErrInvalidState
		}
		return s.publish(ctx, tx, actorID, id)
	})
}

func (s *Service) publish(ctx context.Context, tx pgx.Tx, actorID, id string) error {
	if _, err := tx.Exec(ctx, `
		UPDATE communication.announcements SET status = 'PUBLISHED', version = version + 1, updated_at = now()
		WHERE id = $1`, id); err != nil {
		return fmt.Errorf("announce: duyuru yayımlanamadı: %w", err)
	}
	if err := audit.Record(ctx, tx, audit.Entry{ActorUserID: actorID, Action: "announcement.publish", EntityType: "announcement",
		EntityID: id}); err != nil {
		return err
	}
	return s.notifyIfDue(ctx, tx, id)
}

// Archive, yayımlanmış duyuruyu yayından kaldırır.
func (s *Service) Archive(ctx context.Context, actorID, id string, version int) error {
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		status, err := lock(ctx, tx, id, version)
		if err != nil {
			return err
		}
		if status != StatusPublished {
			return ErrInvalidState
		}
		if _, err := tx.Exec(ctx, `
			UPDATE communication.announcements SET status = 'ARCHIVED', version = version + 1, updated_at = now()
			WHERE id = $1`, id); err != nil {
			return fmt.Errorf("announce: duyuru arşivlenemedi: %w", err)
		}
		return audit.Record(ctx, tx, audit.Entry{ActorUserID: actorID, Action: "announcement.archive", EntityType: "announcement",
			EntityID: id})
	})
}

// Delete, taslağı siler; yayımlanmış duyuru silinmez (arşivlenir).
func (s *Service) Delete(ctx context.Context, actorID, id string) error {
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM communication.announcements WHERE id = $1 AND status = 'DRAFT'`, id)
		if err != nil {
			return fmt.Errorf("announce: duyuru silinemedi: %w", err)
		}
		if tag.RowsAffected() == 0 {
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM communication.announcements WHERE id = $1)`, id).Scan(&exists); err != nil {
				return fmt.Errorf("announce: duyuru okunamadı: %w", err)
			}
			if exists {
				return ErrInvalidState
			}
			return ErrNotFound
		}
		return audit.Record(ctx, tx, audit.Entry{ActorUserID: actorID, Action: "announcement.delete", EntityType: "announcement",
			EntityID: id})
	})
}

// memberCTE, kullanıcının ($1) hedef kitle üyelikleridir: öğrenci olarak program, bölüm ve
// fakülteleri (su); personel olarak bölümü ve fakültesi (fu) ve personel olup olmadığı (staff).
const memberCTE = `
	WITH su AS (
		SELECT unnest(ARRAY[p.id, d.id, d.faculty_id]) AS unit
		FROM enrollment.student_programs sp
		JOIN people.students st ON st.id = sp.student_id
		JOIN iam.users u ON u.person_id = st.person_id
		JOIN org.programs p ON p.id = sp.program_id
		JOIN org.departments d ON d.id = p.department_id
		WHERE u.id = $1 AND sp.status IN ('PREP', 'ACTIVE', 'FROZEN')
	), fu AS (
		SELECT unnest(ARRAY[d.id, d.faculty_id]) AS unit
		FROM people.staff s
		JOIN iam.users u ON u.person_id = s.person_id
		JOIN org.departments d ON d.id = s.primary_department_id
		WHERE u.id = $1 AND s.employment_status = 'ACTIVE'
	), staff AS (
		SELECT EXISTS (SELECT 1 FROM people.staff s JOIN iam.users u ON u.person_id = s.person_id
		               WHERE u.id = $1 AND s.employment_status = 'ACTIVE') AS v
	)`

// inAudience, duyurunun (an) hedef kitlesinde kullanıcı olup olmadığıdır (memberCTE ile).
const inAudience = `EXISTS (
	SELECT 1 FROM communication.announcement_audiences a
	WHERE a.announcement_id = an.id AND (
		(a.audience IN ('ALL', 'STUDENTS') AND EXISTS (SELECT 1 FROM su)
		 AND (a.scope_type = 'UNIVERSITY' OR a.scope_id IN (SELECT unit FROM su)))
		OR (a.audience IN ('ALL', 'STAFF') AND (SELECT v FROM staff)
		 AND (a.scope_type = 'UNIVERSITY' OR a.scope_id IN (SELECT unit FROM fu)))))`

// live, yayında olan duyurudur (an).
const live = `an.status = 'PUBLISHED' AND an.publish_from <= now() AND (an.publish_until IS NULL OR an.publish_until > now())`

// FeedCursor, akışın bir sonraki sayfasının başlangıcıdır.
type FeedCursor struct {
	Pinned bool      `json:"p"`
	From   time.Time `json:"t"`
	ID     string    `json:"i"`
}

// Feed, kullanıcının hedef kitlesinde olduğu yayındaki duyurulardır: sabitlenenler önce, sonra
// yeniden eskiye; okuduğu an ReadAt'te.
func (s *Service) Feed(ctx context.Context, userID string, after *FeedCursor, limit int) ([]Announcement, bool, error) {
	args := []any{userID, limit + 1}
	cond := ""
	if after != nil {
		cond = ` AND (an.is_pinned, an.publish_from, an.id) < ($3, $4, $5::uuid)`
		args = append(args, after.Pinned, after.From, after.ID)
	}
	rows, err := s.pool.Query(ctx, memberCTE+`
		SELECT `+columns+`, r.read_at `+joins+`
		LEFT JOIN communication.announcement_reads r ON r.announcement_id = an.id AND r.user_id = $1
		WHERE `+live+` AND `+inAudience+cond+`
		ORDER BY an.is_pinned DESC, an.publish_from DESC, an.id DESC
		LIMIT $2`, args...)
	if err != nil {
		return nil, false, fmt.Errorf("announce: duyurular okunamadı: %w", err)
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Announcement, error) {
		var readAt *time.Time
		a, err := scan(row, &readAt)
		a.ReadAt = readAt
		return a, err
	})
	if err != nil {
		return nil, false, fmt.Errorf("announce: duyurular okunamadı: %w", err)
	}
	more := len(list) > limit
	if more {
		list = list[:limit]
	}
	if err := withAudiences(ctx, s.pool, list); err != nil {
		return nil, false, err
	}
	return list, more, nil
}

// Visible, duyurunun yayında ve kullanıcının hedef kitlesinde olduğunu söyler.
func (s *Service) Visible(ctx context.Context, id, userID string) (bool, error) {
	var ok bool
	if err := s.pool.QueryRow(ctx, memberCTE+`
		SELECT EXISTS (SELECT 1 FROM communication.announcements an WHERE an.id = $2 AND `+live+` AND `+inAudience+`)`,
		userID, id).Scan(&ok); err != nil {
		return false, fmt.Errorf("announce: görünürlük okunamadı: %w", err)
	}
	return ok, nil
}

// MarkRead, duyuruyu kullanıcı için okundu yapar (tekrarı zararsız).
func (s *Service) MarkRead(ctx context.Context, id, userID string) error {
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO communication.announcement_reads (announcement_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		id, userID); err != nil {
		return fmt.Errorf("announce: okundu yazılamadı: %w", err)
	}
	return nil
}

// recipientsSQL, duyurunun ($1) hedef kitlesindeki etkin kullanıcılardır.
const recipientsSQL = `
	WITH a AS (SELECT * FROM communication.announcement_audiences WHERE announcement_id = $1)
	SELECT u.id FROM enrollment.student_programs sp
	JOIN people.students st ON st.id = sp.student_id
	JOIN iam.users u ON u.person_id = st.person_id AND u.status = 'ACTIVE'
	JOIN org.programs p ON p.id = sp.program_id
	JOIN org.departments d ON d.id = p.department_id
	WHERE sp.status IN ('PREP', 'ACTIVE', 'FROZEN') AND EXISTS (
		SELECT 1 FROM a WHERE a.audience IN ('ALL', 'STUDENTS')
		AND (a.scope_type = 'UNIVERSITY' OR a.scope_id IN (p.id, d.id, d.faculty_id)))
	UNION
	SELECT u.id FROM people.staff s
	JOIN iam.users u ON u.person_id = s.person_id AND u.status = 'ACTIVE'
	LEFT JOIN org.departments d ON d.id = s.primary_department_id
	WHERE s.employment_status = 'ACTIVE' AND EXISTS (
		SELECT 1 FROM a WHERE a.audience IN ('ALL', 'STAFF')
		AND (a.scope_type = 'UNIVERSITY' OR a.scope_id IN (d.id, d.faculty_id)))`

// Stats, duyurunun hedef kitle büyüklüğü ve okuyan sayısıdır.
func (s *Service) Stats(ctx context.Context, id string) (audience, reads int, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM (`+recipientsSQL+`) r),
		       (SELECT count(*) FROM communication.announcement_reads WHERE announcement_id = $1)`, id).Scan(&audience, &reads)
	if err != nil {
		err = fmt.Errorf("announce: istatistik okunamadı: %w", err)
	}
	return audience, reads, err
}

// notifyIfDue, yayını başlamış ve henüz bildirilmemiş duyuruyu hedef kitleye bildirir.
func (s *Service) notifyIfDue(ctx context.Context, tx pgx.Tx, id string) error {
	var (
		title, summary, typ, author string
		due                         bool
	)
	err := tx.QueryRow(ctx, `
		SELECT an.title, an.summary, an.announcement_type, coalesce(p.first_name || ' ' || p.last_name, ''),
		       an.status = 'PUBLISHED' AND an.notified_at IS NULL AND an.publish_from <= now()
		       AND (an.publish_until IS NULL OR an.publish_until > now())
		`+joins+` WHERE an.id = $1`, id).Scan(&title, &summary, &typ, &author, &due)
	if err != nil {
		return fmt.Errorf("announce: duyuru okunamadı: %w", err)
	}
	if !due {
		return nil
	}
	rows, err := tx.Query(ctx, recipientsSQL, id)
	if err != nil {
		return fmt.Errorf("announce: hedef kitle okunamadı: %w", err)
	}
	users, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return fmt.Errorf("announce: hedef kitle okunamadı: %w", err)
	}
	kind := notify.AnnouncementPublished
	if typ == TypeUrgent {
		kind = notify.AnnouncementUrgent
	}
	if err := notify.Send(ctx, tx, notify.Notification{Type: kind, UserIDs: users, Link: "/duyurular/" + id,
		Data: map[string]any{"title": title, "summary": summary, "author": author}}); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE communication.announcements SET notified_at = now() WHERE id = $1`, id); err != nil {
		return fmt.Errorf("announce: bildirim kaydedilemedi: %w", err)
	}
	return nil
}

// NotifyDue, yayın zamanı gelmiş ve bildirilmemiş (ileri tarihli) duyuruları bildirir ve
// bildirilen duyuru sayısını döndürür (worker). Birden çok worker güvenle çalışır.
func (s *Service) NotifyDue(ctx context.Context) (int, error) {
	n := 0
	for {
		var id string
		err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
			err := tx.QueryRow(ctx, `
				SELECT id FROM communication.announcements
				WHERE status = 'PUBLISHED' AND notified_at IS NULL AND publish_from <= now()
				ORDER BY publish_from LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&id)
			if err != nil {
				return err
			}
			// Yayın penceresi kapanmış duyuru bildirilmez, işaretlenir.
			if _, err := tx.Exec(ctx, `
				UPDATE communication.announcements SET notified_at = now()
				WHERE id = $1 AND publish_until IS NOT NULL AND publish_until <= now()`, id); err != nil {
				return fmt.Errorf("announce: duyuru işaretlenemedi: %w", err)
			}
			return s.notifyIfDue(ctx, tx, id)
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return n, nil
		}
		if err != nil {
			return n, err
		}
		n++
	}
}

// Managed, yayımlayanın yönettiği duyurulardır: yazdıkları ve hedeflerinden biri yetki kapsamındaki
// birimlerde (alt birimler dahil) olanlar; en yeni önce, okuyan sayısıyla.
func (s *Service) Managed(ctx context.Context, actorID string, scope authz.ScopeSet) ([]Announcement, map[string]int, error) {
	rows, err := s.pool.Query(ctx, `
		WITH units AS (
			SELECT id FROM org.faculties WHERE id = ANY ($3::uuid[])
			UNION SELECT id FROM org.departments WHERE id = ANY ($4::uuid[]) OR faculty_id = ANY ($3::uuid[])
			UNION SELECT p.id FROM org.programs p JOIN org.departments d ON d.id = p.department_id
			      WHERE p.id = ANY ($5::uuid[]) OR d.id = ANY ($4::uuid[]) OR d.faculty_id = ANY ($3::uuid[])
		)
		SELECT `+columns+`, (SELECT count(*) FROM communication.announcement_reads r WHERE r.announcement_id = an.id)
		`+joins+`
		WHERE an.author_id = $1 OR $2 OR EXISTS (
			SELECT 1 FROM communication.announcement_audiences a WHERE a.announcement_id = an.id AND a.scope_id IN (SELECT id FROM units))
		ORDER BY an.created_at DESC
		LIMIT 200`, actorID, scope.University, scope.FacultyIDs, scope.DepartmentIDs, scope.ProgramIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("announce: duyurular okunamadı: %w", err)
	}
	reads := map[string]int{}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Announcement, error) {
		var n int
		a, err := scan(row, &n)
		reads[a.ID] = n
		return a, err
	})
	if err != nil {
		return nil, nil, fmt.Errorf("announce: duyurular okunamadı: %w", err)
	}
	if err := withAudiences(ctx, s.pool, list); err != nil {
		return nil, nil, err
	}
	return list, reads, nil
}

// Unit, hedef seçicide gösterilen bir birimdir.
type Unit struct {
	ID, Name, ParentID string
}

// TargetUnits, yetki kapsamındaki hedeflenebilir birimlerdir: fakülteler, bölümler, programlar
// (kapsamdaki birimin alt birimleri dahil).
func (s *Service) TargetUnits(ctx context.Context, scope authz.ScopeSet) (faculties, departments, programs []Unit, err error) {
	read := func(sql string, args ...any) ([]Unit, error) {
		rows, err := s.pool.Query(ctx, sql, args...)
		if err != nil {
			return nil, fmt.Errorf("announce: birimler okunamadı: %w", err)
		}
		list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Unit, error) {
			var u Unit
			return u, row.Scan(&u.ID, &u.Name, &u.ParentID)
		})
		if err != nil {
			return nil, fmt.Errorf("announce: birimler okunamadı: %w", err)
		}
		return list, nil
	}
	if faculties, err = read(`
		SELECT id, name_tr, '' FROM org.faculties WHERE $1 OR id = ANY ($2::uuid[]) ORDER BY name_tr COLLATE "tr-x-icu"`,
		scope.University, scope.FacultyIDs); err != nil {
		return
	}
	if departments, err = read(`
		SELECT id, name_tr, faculty_id::text FROM org.departments
		WHERE $1 OR faculty_id = ANY ($2::uuid[]) OR id = ANY ($3::uuid[]) ORDER BY name_tr COLLATE "tr-x-icu"`,
		scope.University, scope.FacultyIDs, scope.DepartmentIDs); err != nil {
		return
	}
	programs, err = read(`
		SELECT p.id, p.name_tr || ' (' || p.code || ')', p.department_id::text FROM org.programs p
		JOIN org.departments d ON d.id = p.department_id
		WHERE $1 OR d.faculty_id = ANY ($2::uuid[]) OR d.id = ANY ($3::uuid[]) OR p.id = ANY ($4::uuid[])
		ORDER BY p.name_tr COLLATE "tr-x-icu", p.code`, scope.University, scope.FacultyIDs, scope.DepartmentIDs, scope.ProgramIDs)
	return
}
