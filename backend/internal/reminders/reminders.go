// Package reminders, worker'da zamanlanmış hatırlatmalardır: işlem pencerelerinin (ders seçme,
// ekle-bırak, not girişi) açılışından ve kapanışından 24 saat önce ilgililere, gönderilmemiş
// ödevin son tarihinden 24 saat önce öğrenciye bildirim. Aynı hatırlatma bir kez gider
// (communication.reminders_sent); zaman karşılaştırmaları veritabanı saatiyle yapılır.
package reminders

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/notify"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// Lead, hatırlatmanın ne kadar önce gittiğidir.
const Lead = 24 * time.Hour

// leadSQL, Lead'in PostgreSQL aralığı olarak yazılışıdır.
var leadSQL = fmt.Sprintf("%d hours", int(Lead.Hours()))

// Öğrencilere ve öğretim elemanlarına hatırlatılan pencere türleri.
var (
	studentWindows    = []string{"COURSE_REGISTRATION", "ADD_DROP_STUDENT"}
	instructorWindows = []string{"MIDTERM_GRADE_ENTRY", "FINAL_GRADE_ENTRY", "MAKEUP_GRADE_ENTRY"}
)

// Result, bir çalıştırmada gönderilen hatırlatmaların kişi sayılarıdır.
type Result struct {
	Opening, Closing, AssignmentDue int
}

// Run, zamanı gelmiş hatırlatmaları gönderir.
func Run(ctx context.Context, pool *pgxpool.Pool) (Result, error) {
	var res Result
	var err error
	if res.Opening, err = windows(ctx, pool, "window_opening"); err != nil {
		return res, err
	}
	if res.Closing, err = windows(ctx, pool, "window_closing"); err != nil {
		return res, err
	}
	res.AssignmentDue, err = assignments(ctx, pool)
	return res, err
}

// window, hatırlatılacak bir takvim olayıdır.
type window struct {
	id, code, nameTR, nameEN string
	at                       time.Time
}

// studentsSQL, $1 penceresinin kapsamındaki etkin öğrencilerdir; aynı dönemde aynı türden daha
// dar kapsamlı (fakültenin ya da programın kendi) penceresi olanlar o pencereye bağlıdır.
const studentsSQL = `
	WITH e AS (SELECT * FROM academic.calendar_events WHERE id = $1)
	SELECT DISTINCT u.id FROM e, enrollment.student_programs sp
	JOIN people.students st ON st.id = sp.student_id
	JOIN iam.users u ON u.person_id = st.person_id AND u.status = 'ACTIVE'
	JOIN org.programs p ON p.id = sp.program_id
	JOIN org.departments d ON d.id = p.department_id
	WHERE sp.status IN ('ACTIVE', 'PREP')
	  AND (e.scope_type = 'UNIVERSITY' OR (e.scope_type = 'FACULTY' AND e.scope_id = d.faculty_id)
	       OR (e.scope_type = 'PROGRAM' AND e.scope_id = p.id))
	  AND NOT EXISTS (
		SELECT 1 FROM academic.calendar_events n
		WHERE n.term_id = e.term_id AND n.event_type_code = e.event_type_code AND n.is_published AND n.id <> e.id
		  AND ((e.scope_type = 'UNIVERSITY' AND ((n.scope_type = 'FACULTY' AND n.scope_id = d.faculty_id)
		                                       OR (n.scope_type = 'PROGRAM' AND n.scope_id = p.id)))
		       OR (e.scope_type = 'FACULTY' AND n.scope_type = 'PROGRAM' AND n.scope_id = p.id)))`

// instructorsSQL, $1 penceresinin döneminde, dersi açan bölümü kapsamdaki şubelerin öğretim
// elemanlarıdır; fakültenin kendi penceresi üniversitenin penceresinin yerine geçer.
const instructorsSQL = `
	WITH e AS (SELECT * FROM academic.calendar_events WHERE id = $1)
	SELECT DISTINCT u.id FROM e, offering.sections s
	JOIN offering.course_offerings o ON o.id = s.offering_id
	JOIN org.departments d ON d.id = o.department_id
	JOIN offering.section_instructors si ON si.section_id = s.id
	JOIN people.staff st ON st.id = si.staff_id
	JOIN iam.users u ON u.person_id = st.person_id AND u.status = 'ACTIVE'
	WHERE o.term_id = e.term_id AND s.status <> 'CANCELLED'
	  AND (e.scope_type = 'UNIVERSITY' OR (e.scope_type = 'FACULTY' AND e.scope_id = d.faculty_id))
	  AND NOT EXISTS (
		SELECT 1 FROM academic.calendar_events n
		WHERE n.term_id = e.term_id AND n.event_type_code = e.event_type_code AND n.is_published AND n.id <> e.id
		  AND e.scope_type = 'UNIVERSITY' AND n.scope_type = 'FACULTY' AND n.scope_id = d.faculty_id)`

// windows, açılışı (window_opening) ya da kapanışı (window_closing) önümüzdeki 24 saatte olan
// pencereleri hatırlatır ve bildirim alan kişi sayısını döndürür. Kapanış hatırlatması açılmış
// pencereler içindir.
func windows(ctx context.Context, pool *pgxpool.Pool, kind string) (int, error) {
	cond := `lower(e.period) > now() AND lower(e.period) <= now() + $2::interval`
	at := `lower(e.period)`
	if kind == "window_closing" {
		cond = `upper(e.period) > now() AND upper(e.period) <= now() + $2::interval AND lower(e.period) <= now()`
		at = `upper(e.period)`
	}
	rows, err := pool.Query(ctx, `
		SELECT e.id, e.event_type_code, coalesce(e.title_tr, t.name_tr), coalesce(e.title_en, t.name_en), `+at+`
		FROM academic.calendar_events e JOIN academic.calendar_event_types t ON t.code = e.event_type_code
		WHERE e.is_published AND e.event_type_code = ANY ($1::text[]) AND `+cond,
		append(append([]string{}, studentWindows...), instructorWindows...), leadSQL)
	if err != nil {
		return 0, fmt.Errorf("reminders: pencereler okunamadı: %w", err)
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (window, error) {
		var w window
		return w, row.Scan(&w.id, &w.code, &w.nameTR, &w.nameEN, &w.at)
	})
	if err != nil {
		return 0, fmt.Errorf("reminders: pencereler okunamadı: %w", err)
	}
	typ := notify.WindowOpening
	if kind == "window_closing" {
		typ = notify.WindowClosing
	}
	total := 0
	for _, w := range list {
		source, link := studentsSQL, "/ders-secimi"
		for _, c := range instructorWindows {
			if w.code == c {
				source, link = instructorsSQL, "/verdigim-dersler"
			}
		}
		atTR, atEN := notify.FormatTime(w.at)
		n, err := remind(ctx, pool, kind, source, []any{w.id}, notify.Notification{Type: typ, Link: link,
			Data: map[string]any{"window_tr": w.nameTR, "window_en": w.nameEN, "at_tr": atTR, "at_en": atEN}})
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

// assignments, son tarihi önümüzdeki 24 saatte olan görünür ödevleri, göndermemiş öğrencilere
// hatırlatır.
func assignments(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	rows, err := pool.Query(ctx, `
		SELECT a.id, a.section_id, a.title, asg.due_at, c.code
		FROM lms.activities a
		JOIN lms.assignments asg ON asg.activity_id = a.id
		JOIN lms.topics tp ON tp.id = a.topic_id
		JOIN offering.sections s ON s.id = a.section_id
		JOIN offering.course_offerings o ON o.id = s.offering_id
		JOIN curriculum.courses c ON c.id = o.course_id
		WHERE a.deleted_at IS NULL AND a.is_visible AND tp.is_visible AND tp.deleted_at IS NULL
		  AND (a.available_from IS NULL OR a.available_from <= now())
		  AND asg.due_at > now() AND asg.due_at <= now() + $1::interval`, leadSQL)
	if err != nil {
		return 0, fmt.Errorf("reminders: ödevler okunamadı: %w", err)
	}
	type due struct {
		id, section, title, code string
		at                       time.Time
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (due, error) {
		var d due
		return d, row.Scan(&d.id, &d.section, &d.title, &d.at, &d.code)
	})
	if err != nil {
		return 0, fmt.Errorf("reminders: ödevler okunamadı: %w", err)
	}
	total := 0
	for _, d := range list {
		atTR, atEN := notify.FormatTime(d.at)
		n, err := remind(ctx, pool, "assignment_due", `
			SELECT DISTINCT u.id FROM enrollment.enrollments e
			JOIN enrollment.student_programs sp ON sp.id = e.student_program_id
			JOIN people.students st ON st.id = sp.student_id
			JOIN iam.users u ON u.person_id = st.person_id AND u.status = 'ACTIVE'
			WHERE e.section_id = $2 AND e.status <> 'DROPPED'
			  AND NOT EXISTS (SELECT 1 FROM lms.assignment_submissions sub
			                  WHERE sub.activity_id = $1 AND sub.user_id = u.id AND sub.status = 'SUBMITTED')`,
			[]any{d.id, d.section}, notify.Notification{Type: notify.AssignmentDue, Link: "/derslerim/" + d.section + "/etkinlik/" + d.id,
				Data: map[string]any{"course_code": d.code, "title": d.title, "due_tr": atTR, "due_en": atEN}})
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

// remind, hatırlatmayı kaynaktaki kişilerden henüz almamış olanlara aynı transaction'da kaydedip
// bildirir ve kişi sayısını döndürür. Kaynağın ilk parametresi ($1) hatırlatılan kayıttır.
func remind(ctx context.Context, pool *pgxpool.Pool, kind, source string, args []any, n notify.Notification) (int, error) {
	count := 0
	err := db.InTx(ctx, pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			INSERT INTO communication.reminders_sent (kind, ref_id, user_id)
			SELECT $`+fmt.Sprint(len(args)+1)+`, $1, r.id FROM (`+source+`) r(id)
			ON CONFLICT DO NOTHING RETURNING user_id::text`, append(args, kind)...)
		if err != nil {
			return fmt.Errorf("reminders: hatırlatma kaydedilemedi: %w", err)
		}
		users, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return fmt.Errorf("reminders: hatırlatma kaydedilemedi: %w", err)
		}
		count = len(users)
		n.UserIDs = users
		return notify.Send(ctx, tx, n)
	})
	return count, err
}
