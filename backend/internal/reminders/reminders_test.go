package reminders_test

import (
	"context"
	"io"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/files"
	"github.com/CAPELLAX02/agora/backend/internal/lms"
	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
	"github.com/CAPELLAX02/agora/backend/internal/reminders"
)

type fixture struct {
	term, muh, fen, bilSection   string
	ali, deniz, frozen, ece, can string // öğrenciler: BIL, MAT, BIL dondurulmuş, BIL (ödevi gönderen), MAT
	bilTeacher, matTeacher       string
}

func seed(t *testing.T, pool *pgxpool.Pool) fixture {
	t.Helper()
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`
		INSERT INTO org.faculties (code, name_tr, name_en, unit_type) VALUES
			('MUH', 'Mühendislik Fakültesi', 'Engineering', 'FACULTY'), ('FEN', 'Fen Fakültesi', 'Science', 'FACULTY');
		INSERT INTO org.departments (faculty_id, code, name_tr, name_en)
			SELECT id, CASE code WHEN 'MUH' THEN 'BIL' ELSE 'MAT' END, 'Bölüm ' || code, 'Dept' FROM org.faculties;
		INSERT INTO org.programs (department_id, code, name_tr, name_en, degree_level, language, education_type,
		                          duration_semesters, max_duration_years, total_ects_required)
			SELECT id, code || '-L', code, code, 'BACHELOR', 'EN', 'DAYTIME', 8, 7, 240 FROM org.departments;
		INSERT INTO academic.academic_years (start_year, starts_on, ends_on) VALUES (2026, '2026-09-01', '2027-08-31');
		INSERT INTO academic.terms (academic_year_id, term_type, code, starts_on, ends_on, status, is_current)
			SELECT id, 'FALL', '2026-FALL', '2026-09-21', '2027-01-31', 'ACTIVE', true FROM academic.academic_years;
		INSERT INTO curriculum.courses (code, owner_department_id, name_tr, name_en, theory_hours, national_credit, ects, language)
			SELECT code || '1001', id, 'Ders ' || code, 'Course', 3, 3, 5, 'EN' FROM org.departments;
		INSERT INTO offering.course_offerings (term_id, course_id, department_id)
			SELECT t.id, c.id, c.owner_department_id FROM academic.terms t, curriculum.courses c;
		INSERT INTO offering.sections (offering_id, section_code, capacity, language)
			SELECT id, '1', 60, 'EN' FROM offering.course_offerings;`)
	var f fixture
	if err := pool.QueryRow(ctx, `
		SELECT (SELECT id FROM academic.terms), (SELECT id FROM org.faculties WHERE code = 'MUH'), (SELECT id FROM org.faculties WHERE code = 'FEN'),
		       (SELECT s.id FROM offering.sections s JOIN offering.course_offerings o ON o.id = s.offering_id
		        JOIN curriculum.courses c ON c.id = o.course_id WHERE c.code = 'BIL1001')`).Scan(&f.term, &f.muh, &f.fen, &f.bilSection); err != nil {
		t.Fatal(err)
	}
	student := func(no, program, status string) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, `
			WITH p AS (INSERT INTO people.persons (first_name, last_name) VALUES ('Öğrenci', $1) RETURNING id),
			st AS (INSERT INTO people.students (person_id, student_no) SELECT id, $1 FROM p RETURNING id),
			sp AS (INSERT INTO enrollment.student_programs (student_id, program_id, admission_type, admission_year, admitted_on, status)
			       SELECT st.id, pr.id, 'OSYS', 2024, '2024-09-15', $3 FROM st, org.programs pr WHERE pr.code = $2 RETURNING id, program_id),
			e AS (INSERT INTO enrollment.enrollments (student_program_id, section_id, term_id, course_id, attempt_no, ects_snapshot, credit_snapshot)
			      SELECT sp.id, s.id, o.term_id, o.course_id, 1, 5, 3 FROM sp, offering.sections s
			      JOIN offering.course_offerings o ON o.id = s.offering_id JOIN org.programs pr ON pr.department_id = o.department_id
			      WHERE pr.id = sp.program_id AND $3 = 'ACTIVE' RETURNING id)
			INSERT INTO iam.users (person_id, username, email, password_hash) SELECT id, $1, $1 || '@ogr.test', 'x' FROM p
			RETURNING id`, no, program, status).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	f.ali, f.deniz, f.frozen = student("24000001", "BIL-L", "ACTIVE"), student("24000002", "MAT-L", "ACTIVE"), student("24000003", "BIL-L", "FROZEN")
	f.ece, f.can = student("24000004", "BIL-L", "ACTIVE"), student("24000005", "MAT-L", "ACTIVE")
	teacher := func(no, course string) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, `
			WITH p AS (INSERT INTO people.persons (first_name, last_name) VALUES ('Hoca', $1) RETURNING id),
			s AS (INSERT INTO people.staff (person_id, staff_no, staff_type) SELECT id, $1::text, 'ACADEMIC' FROM p RETURNING id),
			si AS (INSERT INTO offering.section_instructors (section_id, staff_id, role)
			       SELECT sec.id, s.id, 'PRIMARY' FROM s, offering.sections sec JOIN offering.course_offerings o ON o.id = sec.offering_id
			       JOIN curriculum.courses c ON c.id = o.course_id WHERE c.code = $2 RETURNING 1)
			INSERT INTO iam.users (person_id, username, email, password_hash) SELECT id, $1::text, $1::text || '@p.test', 'x' FROM p
			RETURNING id`, no, course).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	f.bilTeacher, f.matTeacher = teacher("P1", "BIL1001"), teacher("P2", "MAT1001")
	return f
}

func types(t *testing.T, pool *pgxpool.Pool, userID string) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT type_code FROM communication.notifications WHERE user_id = $1 AND type_code LIKE 'reminder.%' ORDER BY id`, userID)
	if err != nil {
		t.Fatal(err)
	}
	out, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRun(t *testing.T) {
	pool := dbtest.New(t)
	fx := seed(t, pool)
	ctx := context.Background()
	event := func(code, scope string, scopeID any, from, to string) {
		t.Helper()
		if _, err := pool.Exec(ctx, `
			INSERT INTO academic.calendar_events (term_id, event_type_code, period, scope_type, scope_id)
			VALUES ($1, $2, tstzrange(now() + $5::interval, now() + $6::interval), $3, $4)`,
			fx.term, code, scope, scopeID, from, to); err != nil {
			t.Fatal(err)
		}
	}
	// Ders seçme 12 saat sonra açılır; Fen Fakültesi'nin kendi penceresi var (iki gün sonra).
	event("COURSE_REGISTRATION", "UNIVERSITY", nil, "12 hours", "5 days")
	event("COURSE_REGISTRATION", "FACULTY", fx.fen, "2 days", "6 days")
	// Final not girişi açık, 6 saat sonra kapanır.
	event("FINAL_GRADE_ENTRY", "UNIVERSITY", nil, "-2 days", "6 hours")
	// Ödev: 10 saat sonra teslim; Ece gönderdi.
	fs := files.NewService(pool, nil, 50<<20, time.Now)
	svc := lms.NewService(pool, fs, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), time.Now)
	topic, err := svc.CreateTopic(ctx, fx.bilTeacher, fx.bilSection, lms.TopicInput{Title: "Hafta 1", Visible: true})
	if err != nil {
		t.Fatal(err)
	}
	due := time.Now().Add(10 * time.Hour)
	activity, err := svc.CreateActivity(ctx, fx.bilTeacher, topic, lms.ActivityInput{Type: lms.TypeAssignment, Title: "Ödev 1", Visible: true,
		Assignment: &lms.AssignmentSettings{DueAt: &due, SubmissionTypes: []string{lms.SubmitText}, MaxFiles: 1, MaxFileSize: 1 << 20,
			MaxAttempts: 1, MaxScore: 10000, LatePolicy: lms.LateAllow}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO lms.assignment_submissions (activity_id, user_id, attempt_no, status, submitted_at) VALUES ($1, $2, 1, 'SUBMITTED', now())`,
		activity, fx.ece); err != nil {
		t.Fatal(err)
	}

	res, err := reminders.Run(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if res.Opening != 2 || res.Closing != 2 || res.AssignmentDue != 1 {
		t.Errorf("sonuç: %+v", res)
	}
	for user, want := range map[string][]string{
		fx.ali:        {"reminder.window_opening", "reminder.assignment_due"},
		fx.ece:        {"reminder.window_opening"},
		fx.deniz:      nil, // fakültesinin kendi penceresi henüz uzak
		fx.frozen:     nil,
		fx.bilTeacher: {"reminder.window_closing"},
		fx.matTeacher: {"reminder.window_closing"},
	} {
		if got := types(t, pool, user); !slices.Equal(got, want) {
			t.Errorf("%s: %v, want %v", user, got, want)
		}
	}
	var body string
	if err := pool.QueryRow(ctx, `
		SELECT payload->>'window_tr' FROM communication.notifications WHERE user_id = $1 AND type_code = 'reminder.window_opening'`, fx.ali).Scan(&body); err != nil || body != "Öğrenci ders seçme işlemleri" {
		t.Errorf("pencere adı: %q %v", body, err)
	}

	// İkinci çalıştırma bir şey göndermez.
	if res, err := reminders.Run(ctx, pool); err != nil || res != (reminders.Result{}) {
		t.Errorf("tekrar: %+v %v", res, err)
	}
}
