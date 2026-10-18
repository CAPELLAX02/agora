package lms

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/notify"
)

// Bildirimler: görünür yeni ödev ve yayımlanan puanlar öğrencilere, Duyurular forumundaki yeni
// duyuru bütün üyelere, genel forumdaki yeni konu abonelere, yanıt konuya yazmış olanlara.
// İşlemle aynı transaction'da yazılır; işlemi yapan kendine bildirim almaz.

// excerptLength, duyuru bildirimindeki metnin en fazla uzunluğudur (karakter).
const excerptLength = 280

// sectionCode, şubenin ders kodudur (ör. COM1007).
func sectionCode(ctx context.Context, tx pgx.Tx, sectionID string) (string, error) {
	var code string
	if err := tx.QueryRow(ctx, `
		SELECT c.code FROM offering.sections s
		JOIN offering.course_offerings o ON o.id = s.offering_id
		JOIN curriculum.courses c ON c.id = o.course_id
		WHERE s.id = $1`, sectionID).Scan(&code); err != nil {
		return "", fmt.Errorf("lms: ders kodu okunamadı: %w", err)
	}
	return code, nil
}

// recipients, sorgunun döndürdüğü kullanıcılardır; except listeden çıkarılır.
func recipients(ctx context.Context, tx pgx.Tx, except, sql string, args ...any) ([]string, error) {
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("lms: bildirim alıcıları okunamadı: %w", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("lms: bildirim alıcıları okunamadı: %w", err)
	}
	return slices.DeleteFunc(ids, func(id string) bool { return id == except }), nil
}

// studentsSQL, şubenin (dersi bırakmamış) öğrencilerinin kullanıcılarıdır; $1 şube.
const studentsSQL = `
	SELECT DISTINCT u.id FROM enrollment.enrollments e
	JOIN enrollment.student_programs sp ON sp.id = e.student_program_id
	JOIN people.students st ON st.id = sp.student_id
	JOIN iam.users u ON u.person_id = st.person_id
	WHERE e.section_id = $1 AND e.status <> 'DROPPED'`

// membersSQL, ders alanının üyeleridir: öğrenciler ve öğretim elemanları; $1 şube.
const membersSQL = studentsSQL + `
	UNION
	SELECT u.id FROM offering.section_instructors si
	JOIN people.staff s ON s.id = si.staff_id
	JOIN iam.users u ON u.person_id = s.person_id
	WHERE si.section_id = $1`

// notifyAssignment, ödevi şubenin öğrencilerine bildirir; ödev ya da bölümü öğrencilerden
// gizliyse bir şey yapmaz.
func notifyAssignment(ctx context.Context, tx pgx.Tx, actorID, activityID string) error {
	var (
		sectionID, title string
		due              *time.Time
		visible          bool
	)
	if err := tx.QueryRow(ctx, `
		SELECT a.section_id, a.title, asg.due_at, a.is_visible AND tp.is_visible
		FROM lms.activities a
		JOIN lms.assignments asg ON asg.activity_id = a.id
		JOIN lms.topics tp ON tp.id = a.topic_id
		WHERE a.id = $1`, activityID).Scan(&sectionID, &title, &due, &visible); err != nil {
		return fmt.Errorf("lms: ödev okunamadı: %w", err)
	}
	if !visible {
		return nil
	}
	code, err := sectionCode(ctx, tx, sectionID)
	if err != nil {
		return err
	}
	users, err := recipients(ctx, tx, actorID, studentsSQL, sectionID)
	if err != nil {
		return err
	}
	data := map[string]any{"course_code": code, "title": title, "due_tr": "", "due_en": ""}
	if due != nil {
		data["due_tr"], data["due_en"] = notify.FormatTime(*due)
	}
	return notify.Send(ctx, tx, notify.Notification{Type: notify.AssignmentCreated, UserIDs: users, Data: data,
		Link: "/derslerim/" + sectionID + "/etkinlik/" + activityID})
}

// notifyReleased, puanları yayımlanan ödevde puanı ya da geri bildirimi olan öğrencilere bildirir.
func notifyReleased(ctx context.Context, tx pgx.Tx, actorID, activityID string) error {
	var sectionID, title string
	if err := tx.QueryRow(ctx, `SELECT section_id, title FROM lms.activities WHERE id = $1`, activityID).Scan(&sectionID, &title); err != nil {
		return fmt.Errorf("lms: ödev okunamadı: %w", err)
	}
	code, err := sectionCode(ctx, tx, sectionID)
	if err != nil {
		return err
	}
	users, err := recipients(ctx, tx, actorID, `
		SELECT user_id FROM lms.assignment_grades
		WHERE activity_id = $1 AND (score IS NOT NULL OR feedback_html <> '')`, activityID)
	if err != nil {
		return err
	}
	return notify.Send(ctx, tx, notify.Notification{Type: notify.AssignmentReleased, UserIDs: users,
		Data: map[string]any{"course_code": code, "title": title}, Link: "/derslerim/" + sectionID + "/etkinlik/" + activityID})
}

// forumOf, forumun şubesi, adı ve türüdür.
func forumOf(ctx context.Context, tx pgx.Tx, activityID string) (sectionID, title, kind string, err error) {
	err = tx.QueryRow(ctx, `
		SELECT a.section_id, a.title, f.forum_kind FROM lms.activities a JOIN lms.forums f ON f.activity_id = a.id
		WHERE a.id = $1`, activityID).Scan(&sectionID, &title, &kind)
	if err != nil {
		err = fmt.Errorf("lms: forum okunamadı: %w", err)
	}
	return sectionID, title, kind, err
}

func authorName(ctx context.Context, tx pgx.Tx, userID string) (string, error) {
	var name string
	if err := tx.QueryRow(ctx, `
		SELECT p.first_name || ' ' || p.last_name FROM iam.users u JOIN people.persons p ON p.id = u.person_id
		WHERE u.id = $1`, userID).Scan(&name); err != nil {
		return "", fmt.Errorf("lms: yazar okunamadı: %w", err)
	}
	return name, nil
}

// notifyThread, yeni konuyu bildirir: Duyurular forumunda bütün üyelere (metniyle), genel
// forumda abonelere.
func notifyThread(ctx context.Context, tx pgx.Tx, actorID, activityID, threadID, title, body string) error {
	sectionID, forum, kind, err := forumOf(ctx, tx, activityID)
	if err != nil {
		return err
	}
	code, err := sectionCode(ctx, tx, sectionID)
	if err != nil {
		return err
	}
	author, err := authorName(ctx, tx, actorID)
	if err != nil {
		return err
	}
	link := "/derslerim/" + sectionID + "/etkinlik/" + activityID + "/konu/" + threadID
	if kind == ForumAnnouncements {
		users, err := recipients(ctx, tx, actorID, membersSQL, sectionID)
		if err != nil {
			return err
		}
		excerpt := []rune(strings.Join(strings.Fields(plainText(body)), " "))
		if len(excerpt) > excerptLength {
			excerpt = append(excerpt[:excerptLength-1], '…')
		}
		return notify.Send(ctx, tx, notify.Notification{Type: notify.ForumAnnouncement, UserIDs: users, Link: link,
			Data: map[string]any{"course_code": code, "title": title, "author": author, "excerpt": string(excerpt)}})
	}
	users, err := recipients(ctx, tx, actorID, `SELECT user_id FROM lms.forum_subscriptions WHERE activity_id = $1`, activityID)
	if err != nil {
		return err
	}
	return notify.Send(ctx, tx, notify.Notification{Type: notify.ForumThread, UserIDs: users, Link: link,
		Data: map[string]any{"course_code": code, "forum": forum, "thread": title, "author": author}})
}

// notifyReply, yanıtı konuya (silinmemiş gönderiyle) yazmış olanlara bildirir.
func notifyReply(ctx context.Context, tx pgx.Tx, actorID, threadID string) error {
	var activityID, sectionID, thread string
	if err := tx.QueryRow(ctx, `
		SELECT t.activity_id, a.section_id, t.title FROM lms.forum_threads t JOIN lms.activities a ON a.id = t.activity_id
		WHERE t.id = $1`, threadID).Scan(&activityID, &sectionID, &thread); err != nil {
		return fmt.Errorf("lms: konu okunamadı: %w", err)
	}
	code, err := sectionCode(ctx, tx, sectionID)
	if err != nil {
		return err
	}
	author, err := authorName(ctx, tx, actorID)
	if err != nil {
		return err
	}
	users, err := recipients(ctx, tx, actorID, `
		SELECT DISTINCT author_id FROM lms.forum_posts WHERE thread_id = $1 AND deleted_at IS NULL`, threadID)
	if err != nil {
		return err
	}
	return notify.Send(ctx, tx, notify.Notification{Type: notify.ForumReply, UserIDs: users,
		Link: "/derslerim/" + sectionID + "/etkinlik/" + activityID + "/konu/" + threadID,
		Data: map[string]any{"course_code": code, "thread": thread, "author": author}})
}
