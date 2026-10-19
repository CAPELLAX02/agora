// Package attendance, yoklamadır (§5 #21): şubenin haftalık programından üretilen oturumlar,
// öğretim elemanının oturum oturum aldığı yoklama ve yönetmelikteki sınıra göre devamsızlık.
// Yetki şubeyle ilişkiden gelir: şubenin öğretim elemanları (asistan dahil) yoklama alır,
// öğrenci kendi devamsızlığını görür.
package attendance

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// Oturum türleri (şube programındaki gibi), oturum çeşitleri ve durumları.
const (
	TypeTheory   = "THEORY"
	TypePractice = "PRACTICE"
	TypeLab      = "LAB"

	KindRegular = "REGULAR"
	KindMakeup  = "MAKEUP"

	StatusScheduled = "SCHEDULED"
	StatusHeld      = "HELD"
	StatusCancelled = "CANCELLED"
)

// Yoklama durumları. Devamsızlık sadece ABSENT'tir: geç kalan gelmiş, raporlu (EXCUSED)
// devamsız sayılmaz.
const (
	Present = "PRESENT"
	Absent  = "ABSENT"
	Late    = "LATE"
	Excused = "EXCUSED"
)

// Hatalar.
var (
	ErrNotFound        = errors.New("attendance: bulunamadı")
	ErrForbidden       = errors.New("attendance: şubenin öğretim elemanı değil")
	ErrVersionMismatch = errors.New("attendance: sürüm uyuşmuyor")
	ErrCancelled       = errors.New("attendance: oturum iptal edilmiş")
	ErrHeld            = errors.New("attendance: oturumun yoklaması alınmış")
)

// ValidationError, alan doğrulama hatasıdır.
type ValidationError struct {
	Field, Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

// zone, programdaki saatlerin yorumlandığı saat dilimidir.
const zone = "Europe/Istanbul"

// Session, bir ders oturumudur.
type Session struct {
	ID, SectionID, TermID string
	Type, Kind, Status    string
	StartsAt, EndsAt      time.Time
	Classroom             string // derslik kodu; çevrim içi oturumda boş
	Topic                 string
	TakenAt               *time.Time
	Version               int
	Present, Absent       int // yoklaması alınmışsa: gelen (geç kalan dahil) ve gelmeyen
}

// Section, yoklamanın şubesidir.
type Section struct {
	ID, Code, CourseCode, CourseNameTR, CourseNameEN, TermID string
}

// Service, yoklamayı yönetir.
type Service struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

// NewService, bir Service oluşturur.
func NewService(pool *pgxpool.Pool, now func() time.Time) *Service {
	return &Service{pool: pool, now: now}
}

// instructorSQL, $2 kullanıcısının $1 şubesinin öğretim elemanı olup olmadığıdır.
const instructorSQL = `EXISTS (
	SELECT 1 FROM offering.section_instructors si
	JOIN people.staff st ON st.id = si.staff_id JOIN iam.users u ON u.person_id = st.person_id
	WHERE si.section_id = $1 AND u.id = $2)`

// Teaching, şubeyi ve kullanıcının öğretim elemanı olup olmadığını döndürür. İptal edilen şube
// bulunamaz.
func (s *Service) Teaching(ctx context.Context, q db.Querier, sectionID, userID string) (Section, bool, error) {
	var sec Section
	var teaches bool
	err := q.QueryRow(ctx, `
		SELECT s.id, s.section_code, c.code, c.name_tr, c.name_en, o.term_id, `+instructorSQL+`
		FROM offering.sections s
		JOIN offering.course_offerings o ON o.id = s.offering_id
		JOIN curriculum.courses c ON c.id = o.course_id
		WHERE s.id = $1 AND s.status <> 'CANCELLED' AND o.status <> 'CANCELLED'`, sectionID, userID).
		Scan(&sec.ID, &sec.Code, &sec.CourseCode, &sec.CourseNameTR, &sec.CourseNameEN, &sec.TermID, &teaches)
	if errors.Is(err, pgx.ErrNoRows) {
		return Section{}, false, ErrNotFound
	}
	if err != nil {
		return Section{}, false, fmt.Errorf("attendance: şube okunamadı: %w", err)
	}
	return sec, teaches, nil
}

// plannedSQL, $1 şubesinin programına göre planlanan oturumlarıdır: derslerin sürdüğü dönemde
// (fakültenin kendi ders dönemi üniversitenin yerine geçer) programdaki her blok her hafta bir
// oturumdur; yayımlanmış tatile (üniversitenin ya da fakültenin) değen oturum planlanmaz.
const plannedSQL = `
	WITH sec AS (
		SELECT s.id, o.term_id, d.faculty_id FROM offering.sections s
		JOIN offering.course_offerings o ON o.id = s.offering_id
		JOIN org.departments d ON d.id = o.department_id
		WHERE s.id = $1 AND s.status <> 'CANCELLED' AND o.status <> 'CANCELLED'
	),
	classes AS (
		SELECT e.period FROM academic.calendar_events e, sec
		WHERE e.term_id = sec.term_id AND e.event_type_code = 'CLASSES' AND e.is_published
		  AND (e.scope_type = 'UNIVERSITY' OR (e.scope_type = 'FACULTY' AND e.scope_id = sec.faculty_id))
		ORDER BY e.scope_type = 'FACULTY' DESC LIMIT 1
	),
	slots AS (
		SELECT sl.id AS slot_id, sl.session_type, sl.classroom_id, sec.term_id, sec.faculty_id,
		       ((day.d::date + lower(sl.time_span)) AT TIME ZONE '` + zone + `') AS starts_at,
		       ((day.d::date + upper(sl.time_span)) AT TIME ZONE '` + zone + `') AS ends_at,
		       classes.period
		FROM sec
		JOIN offering.schedule_slots sl ON sl.section_id = sec.id
		CROSS JOIN classes
		CROSS JOIN LATERAL generate_series((lower(classes.period) AT TIME ZONE '` + zone + `')::date,
		                                   (upper(classes.period) AT TIME ZONE '` + zone + `')::date, interval '1 day') AS day(d)
		WHERE extract(isodow FROM day.d) = sl.day_of_week
	),
	planned AS (
		SELECT * FROM slots p
		WHERE tstzrange(p.starts_at, p.ends_at) <@ p.period
		  AND NOT EXISTS (
			SELECT 1 FROM academic.calendar_events h
			WHERE h.term_id = p.term_id AND h.event_type_code = 'HOLIDAY' AND h.is_published
			  AND (h.scope_type = 'UNIVERSITY' OR (h.scope_type = 'FACULTY' AND h.scope_id = p.faculty_id))
			  AND h.period && tstzrange(p.starts_at, p.ends_at))
	)`

// Generate, şubenin oturumlarını programla eşitler: eksik oturumları ekler, programdan kalkmış
// gelecekteki boş oturumları siler ve gelecekteki oturumların saatini ve dersliğini günceller.
// Geçmiş, yoklaması alınmış, iptal edilmiş ve telafi oturumlarına dokunmaz. Tekrarı zararsızdır.
func Generate(ctx context.Context, q db.Querier, sectionID string) error {
	if _, err := q.Exec(ctx, plannedSQL+`
		DELETE FROM attendance.class_sessions c
		WHERE c.section_id = $1 AND c.kind = 'REGULAR' AND c.status = 'SCHEDULED' AND c.starts_at > now()
		  AND NOT EXISTS (SELECT 1 FROM planned p WHERE p.starts_at = c.starts_at)
		  AND NOT EXISTS (SELECT 1 FROM attendance.attendance_records r WHERE r.session_id = c.id)`, sectionID); err != nil {
		return fmt.Errorf("attendance: eski oturumlar silinemedi: %w", err)
	}
	if _, err := q.Exec(ctx, plannedSQL+`
		INSERT INTO attendance.class_sessions (section_id, term_id, schedule_slot_id, session_type, starts_at, ends_at, classroom_id)
		SELECT $1, term_id, slot_id, session_type, starts_at, ends_at, classroom_id FROM planned
		ON CONFLICT (section_id, starts_at) DO UPDATE
		SET schedule_slot_id = excluded.schedule_slot_id, session_type = excluded.session_type, ends_at = excluded.ends_at,
		    classroom_id = excluded.classroom_id, updated_at = now()
		WHERE class_sessions.kind = 'REGULAR' AND class_sessions.status = 'SCHEDULED' AND class_sessions.starts_at > now()
		  AND (class_sessions.schedule_slot_id, class_sessions.session_type, class_sessions.ends_at, class_sessions.classroom_id)
		      IS DISTINCT FROM (excluded.schedule_slot_id, excluded.session_type, excluded.ends_at, excluded.classroom_id)`,
		sectionID); err != nil {
		return fmt.Errorf("attendance: oturumlar üretilemedi: %w", err)
	}
	return nil
}

// GenerateTerm, dönemin bütün şubelerinin oturumlarını programla eşitler (worker ve seed) ve
// eşitlenen şube sayısını döndürür.
func GenerateTerm(ctx context.Context, pool *pgxpool.Pool, termID string) (int, error) {
	rows, err := pool.Query(ctx, `
		SELECT s.id FROM offering.sections s JOIN offering.course_offerings o ON o.id = s.offering_id
		WHERE o.term_id = $1 AND s.status <> 'CANCELLED' AND o.status <> 'CANCELLED'
		  AND EXISTS (SELECT 1 FROM offering.schedule_slots sl WHERE sl.section_id = s.id)`, termID)
	if err != nil {
		return 0, fmt.Errorf("attendance: şubeler okunamadı: %w", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return 0, fmt.Errorf("attendance: şubeler okunamadı: %w", err)
	}
	for _, id := range ids {
		if err := db.InTx(ctx, pool, func(tx pgx.Tx) error { return Generate(ctx, tx, id) }); err != nil {
			return 0, err
		}
	}
	return len(ids), nil
}

// GenerateCurrent, içinde bulunulan dönemin oturumlarını programla eşitler (worker'da günlük);
// dönem yoksa bir şey yapmaz.
func GenerateCurrent(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	var termID string
	err := pool.QueryRow(ctx, `SELECT id FROM academic.terms WHERE is_current`).Scan(&termID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("attendance: dönem okunamadı: %w", err)
	}
	return GenerateTerm(ctx, pool, termID)
}

const sessionColumns = `c.id, c.section_id, c.term_id, c.session_type, c.kind, c.status, c.starts_at, c.ends_at,
	coalesce(cr.code, ''), coalesce(c.topic, ''), c.taken_at, c.version,
	(SELECT count(*) FROM attendance.attendance_records r WHERE r.term_id = c.term_id AND r.session_id = c.id AND r.status IN ('PRESENT', 'LATE')),
	(SELECT count(*) FROM attendance.attendance_records r WHERE r.term_id = c.term_id AND r.session_id = c.id AND r.status = 'ABSENT')`

const sessionJoins = ` FROM attendance.class_sessions c LEFT JOIN org.classrooms cr ON cr.id = c.classroom_id`

func scanSession(row pgx.CollectableRow) (Session, error) {
	var x Session
	return x, row.Scan(&x.ID, &x.SectionID, &x.TermID, &x.Type, &x.Kind, &x.Status, &x.StartsAt, &x.EndsAt,
		&x.Classroom, &x.Topic, &x.TakenAt, &x.Version, &x.Present, &x.Absent)
}

// Sessions, şubenin oturumlarıdır (önce programla eşitlenir), zaman sırasıyla.
func (s *Service) Sessions(ctx context.Context, sectionID string) ([]Session, error) {
	var list []Session
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := Generate(ctx, tx, sectionID); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT `+sessionColumns+sessionJoins+` WHERE c.section_id = $1 ORDER BY c.starts_at`, sectionID)
		if err != nil {
			return fmt.Errorf("attendance: oturumlar okunamadı: %w", err)
		}
		list, err = pgx.CollectRows(rows, scanSession)
		if err != nil {
			return fmt.Errorf("attendance: oturumlar okunamadı: %w", err)
		}
		return nil
	})
	return list, err
}

// Session, bir oturumdur.
func (s *Service) Session(ctx context.Context, q db.Querier, id string) (Session, error) {
	rows, err := q.Query(ctx, `SELECT `+sessionColumns+sessionJoins+` WHERE c.id = $1`, id)
	if err != nil {
		return Session{}, fmt.Errorf("attendance: oturum okunamadı: %w", err)
	}
	x, err := pgx.CollectExactlyOneRow(rows, scanSession)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("attendance: oturum okunamadı: %w", err)
	}
	return x, nil
}

// MakeupInput, telafi oturumudur.
type MakeupInput struct {
	Type             string
	StartsAt, EndsAt time.Time
	Topic            string
}

// AddMakeup, şubeye telafi oturumu ekler ve kimliğini döndürür. Oturum dönemin içinde olmalı ve
// en çok 6 saat sürmeli; şubenin aynı anda başlayan başka oturumu olamaz.
func (s *Service) AddMakeup(ctx context.Context, actorID, sectionID string, in MakeupInput) (string, error) {
	switch {
	case in.Type != TypeTheory && in.Type != TypePractice && in.Type != TypeLab:
		return "", &ValidationError{"session_type", "THEORY, PRACTICE ya da LAB olmalı."}
	case !in.EndsAt.After(in.StartsAt) || in.EndsAt.Sub(in.StartsAt) > 6*time.Hour:
		return "", &ValidationError{"ends_at", "Bitiş başlangıçtan sonra olmalı; oturum en çok 6 saat sürer."}
	case len([]rune(in.Topic)) > 300:
		return "", &ValidationError{"topic", "En çok 300 karakter olmalı."}
	}
	var id string
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var inTerm bool
		if err := tx.QueryRow(ctx, `
			SELECT $2::timestamptz >= (t.starts_on::timestamp AT TIME ZONE '`+zone+`')
			   AND $3::timestamptz <= ((t.ends_on + 1)::timestamp AT TIME ZONE '`+zone+`')
			FROM offering.sections s JOIN offering.course_offerings o ON o.id = s.offering_id JOIN academic.terms t ON t.id = o.term_id
			WHERE s.id = $1`, sectionID, in.StartsAt, in.EndsAt).Scan(&inTerm); err != nil {
			return fmt.Errorf("attendance: dönem okunamadı: %w", err)
		}
		if !inTerm {
			return &ValidationError{"starts_at", "Telafi oturumu dönemin içinde olmalı."}
		}
		err := tx.QueryRow(ctx, `
			INSERT INTO attendance.class_sessions (section_id, term_id, session_type, kind, starts_at, ends_at, topic)
			SELECT s.id, o.term_id, $2, 'MAKEUP', $3, $4, nullif($5, '')
			FROM offering.sections s JOIN offering.course_offerings o ON o.id = s.offering_id WHERE s.id = $1
			ON CONFLICT (section_id, starts_at) DO NOTHING RETURNING id`,
			sectionID, in.Type, in.StartsAt, in.EndsAt, in.Topic).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return &ValidationError{"starts_at", "Şubenin bu saatte başlayan bir oturumu zaten var."}
		}
		if err != nil {
			return fmt.Errorf("attendance: telafi oturumu yazılamadı: %w", err)
		}
		return audit.Record(ctx, tx, audit.Entry{ActorUserID: actorID, Action: "attendance.session_add", EntityType: "class_session",
			EntityID: id, After: map[string]any{"section_id": sectionID, "type": in.Type, "starts_at": in.StartsAt, "ends_at": in.EndsAt}})
	})
	return id, err
}

// SessionUpdate, oturumda değişebilenlerdir: iptal (ya da iptalin geri alınması) ve konu.
type SessionUpdate struct {
	Cancelled bool
	Topic     string
}

// UpdateSession, oturumu iptal eder ya da geri alır ve konusunu yazar (iyimser kilitle).
// Yoklaması alınmış oturum iptal edilemez; telafi oturumu iptal edilince silinmez, sayılmaz.
func (s *Service) UpdateSession(ctx context.Context, actorID, id string, version int, in SessionUpdate) error {
	if len([]rune(in.Topic)) > 300 {
		return &ValidationError{"topic", "En çok 300 karakter olmalı."}
	}
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var status string
		var current int
		err := tx.QueryRow(ctx, `SELECT status, version FROM attendance.class_sessions WHERE id = $1 FOR UPDATE`, id).Scan(&status, &current)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("attendance: oturum okunamadı: %w", err)
		}
		if current != version {
			return ErrVersionMismatch
		}
		next := status
		switch {
		case in.Cancelled && status == StatusHeld:
			return ErrHeld
		case in.Cancelled:
			next = StatusCancelled
		case status == StatusCancelled:
			next = StatusScheduled
		}
		if _, err := tx.Exec(ctx, `
			UPDATE attendance.class_sessions SET status = $2, topic = nullif($3, ''), version = version + 1, updated_at = now()
			WHERE id = $1`, id, next, in.Topic); err != nil {
			return fmt.Errorf("attendance: oturum güncellenemedi: %w", err)
		}
		return audit.Record(ctx, tx, audit.Entry{ActorUserID: actorID, Action: "attendance.session_update", EntityType: "class_session",
			EntityID: id, Before: map[string]any{"status": status}, After: map[string]any{"status": next, "topic": in.Topic}})
	})
}
