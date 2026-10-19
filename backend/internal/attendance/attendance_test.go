package attendance_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/attendance"
	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
)

var trt = time.FixedZone("TRT", 3*3600)

type fixture struct {
	term, section, teacher, stranger string
	ali, ece, can, dropped           string // kesin kayıtlar; can 2. sınıf, dropped bırakmış
	from, to, holiday                time.Time
}

// day, bugünden n gün sonraki günün başlangıcıdır (Türkiye saati).
func day(n int) time.Time {
	y, m, d := time.Now().In(trt).Date()
	return time.Date(y, m, d+n, 0, 0, 0, 0, trt)
}

// seed: derslerin sürdüğü dönem üç hafta önce başlamış, beş hafta sonra biter; şubenin programı
// pazartesi teori (09:00-11:50) ve çarşamba laboratuvar (13:00-14:50). Ders döneminin ikinci
// haftasının pazartesisi tatildir.
func seed(t *testing.T, pool *pgxpool.Pool) fixture {
	t.Helper()
	ctx := context.Background()
	f := fixture{from: day(-21), to: day(35)}
	f.holiday = f.from.AddDate(0, 0, 7)
	for f.holiday.Weekday() != time.Monday {
		f.holiday = f.holiday.AddDate(0, 0, 1)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO org.faculties (code, name_tr, name_en, unit_type) VALUES ('MUH', 'Mühendislik Fakültesi', 'Engineering', 'FACULTY');
		INSERT INTO org.departments (faculty_id, code, name_tr, name_en) SELECT id, 'BIL', 'Bilgisayar Mühendisliği', 'CE' FROM org.faculties;
		INSERT INTO org.programs (department_id, code, name_tr, name_en, degree_level, language, education_type,
		                          duration_semesters, max_duration_years, total_ects_required)
			SELECT id, 'BIL-L', 'BIL', 'CE', 'BACHELOR', 'EN', 'DAYTIME', 8, 7, 240 FROM org.departments;
		INSERT INTO org.campuses (code, name) VALUES ('MERKEZ', 'Merkez');
		INSERT INTO org.buildings (campus_id, code, name) SELECT id, 'MUH-A', 'A Blok' FROM org.campuses;
		INSERT INTO org.classrooms (building_id, code, name, capacity, exam_capacity, room_type)
			SELECT id, 'A101', 'A101', 60, 30, 'LECTURE' FROM org.buildings;
		INSERT INTO academic.academic_years (start_year, starts_on, ends_on) VALUES (2026, '2026-09-01', '2027-08-31');
		INSERT INTO curriculum.courses (code, owner_department_id, name_tr, name_en, theory_hours, national_credit, ects, language)
			SELECT 'BIL1001', id, 'Programlama', 'Programming', 3, 3, 5, 'EN' FROM org.departments;`); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO academic.terms (academic_year_id, term_type, code, starts_on, ends_on, status, is_current)
		SELECT id, 'FALL', '2026-FALL', $1::date - 7, $2::date + 30, 'ACTIVE', true FROM academic.academic_years RETURNING id`,
		f.from, f.to).Scan(&f.term); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO academic.calendar_events (term_id, event_type_code, period, scope_type) VALUES
			($1, 'CLASSES', tstzrange($2::timestamptz, $3::timestamptz), 'UNIVERSITY'),
			($1, 'HOLIDAY', tstzrange($4::timestamptz, $4::timestamptz + interval '1 day'), 'UNIVERSITY')`, f.term, f.from, f.to, f.holiday); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		WITH o AS (INSERT INTO offering.course_offerings (term_id, course_id, department_id)
		           SELECT $1, c.id, c.owner_department_id FROM curriculum.courses c RETURNING id),
		s AS (INSERT INTO offering.sections (offering_id, section_code, capacity, language) SELECT id, '1', 60, 'EN' FROM o RETURNING id),
		sl AS (INSERT INTO offering.schedule_slots (section_id, term_id, day_of_week, time_span, classroom_id, session_type)
		       SELECT s.id, $1, v.d, v.span::offering.timerange, (SELECT id FROM org.classrooms), v.kind
		       FROM s, (VALUES (1, '[09:00,11:50)', 'THEORY'), (3, '[13:00,14:50)', 'LAB')) AS v(d, span, kind) RETURNING 1)
		SELECT id FROM s`, f.term).Scan(&f.section); err != nil {
		t.Fatal(err)
	}
	staff := func(no string, teaches bool) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, `
			WITH p AS (INSERT INTO people.persons (first_name, last_name) VALUES ('Hoca', $1) RETURNING id),
			s AS (INSERT INTO people.staff (person_id, staff_no, staff_type) SELECT id, $1::text, 'ACADEMIC' FROM p RETURNING id),
			si AS (INSERT INTO offering.section_instructors (section_id, staff_id, role) SELECT $2, s.id, 'PRIMARY' FROM s WHERE $3 RETURNING 1)
			INSERT INTO iam.users (person_id, username, email, password_hash) SELECT id, $1::text, $1::text || '@p.test', 'x' FROM p
			RETURNING id`, no, f.section, teaches).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	f.teacher, f.stranger = staff("P1", true), staff("P2", false)
	student := func(no, first, status string) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, `
			WITH p AS (INSERT INTO people.persons (first_name, last_name) VALUES ($2, 'Öğrenci') RETURNING id),
			st AS (INSERT INTO people.students (person_id, student_no) SELECT id, $1 FROM p RETURNING id),
			sp AS (INSERT INTO enrollment.student_programs (student_id, program_id, admission_type, admission_year, admitted_on, status)
			       SELECT st.id, pr.id, 'OSYS', 2025, '2025-09-15', 'ACTIVE' FROM st, org.programs pr RETURNING id),
			u AS (INSERT INTO iam.users (person_id, username, email, password_hash) SELECT id, $1, $1 || '@ogr.test', 'x' FROM p)
			INSERT INTO enrollment.enrollments (student_program_id, section_id, term_id, course_id, attempt_no, ects_snapshot, credit_snapshot, status)
			SELECT sp.id, $3, $4, (SELECT id FROM curriculum.courses), 1, 5, 3, $5 FROM sp RETURNING id`,
			no, first, f.section, f.term, status).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	f.ali, f.ece, f.can = student("25000001", "Ali", "ENROLLED"), student("25000002", "Ece", "ENROLLED"), student("25000003", "Can", "ENROLLED")
	f.dropped = student("25000004", "Deniz", "DROPPED")
	return f
}

// weekdays, [from, to) aralığındaki günü wd olan günlerdir.
func weekdays(from, to time.Time, wd time.Weekday) []time.Time {
	var out []time.Time
	for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {
		if d.Weekday() == wd {
			out = append(out, d)
		}
	}
	return out
}

func TestSessionsFollowSchedule(t *testing.T) {
	pool := dbtest.New(t)
	f := seed(t, pool)
	ctx := context.Background()
	svc := attendance.NewService(pool, time.Now)

	mondays, wednesdays := weekdays(f.from, f.to, time.Monday), weekdays(f.from, f.to, time.Wednesday)
	for range 2 { // tekrar üretmek bir şey değiştirmez
		list, err := svc.Sessions(ctx, f.section)
		if err != nil {
			t.Fatal(err)
		}
		if want := len(mondays) - 1 + len(wednesdays); len(list) != want {
			t.Fatalf("oturum sayısı %d, want %d (tatil hariç)", len(list), want)
		}
	}
	list, _ := svc.Sessions(ctx, f.section)
	first := list[0]
	if first.Classroom != "A101" || first.Status != attendance.StatusScheduled || first.Kind != attendance.KindRegular {
		t.Errorf("ilk oturum: %+v", first)
	}
	for _, x := range list {
		local := x.StartsAt.In(trt)
		switch {
		case local.Year() == f.holiday.Year() && local.YearDay() == f.holiday.YearDay():
			t.Errorf("tatilde oturum: %v", local)
		case x.Type == attendance.TypeTheory && (local.Weekday() != time.Monday || local.Hour() != 9 || x.EndsAt.Sub(x.StartsAt) != 170*time.Minute):
			t.Errorf("teori oturumu: %v – %v", local, x.EndsAt.In(trt))
		case x.Type == attendance.TypeLab && (local.Weekday() != time.Wednesday || local.Hour() != 13):
			t.Errorf("laboratuvar oturumu: %v", local)
		}
	}

	// Laboratuvar programdan kalkınca gelecekteki laboratuvar oturumları silinir, geçmiştekiler kalır.
	if _, err := pool.Exec(ctx, `DELETE FROM offering.schedule_slots WHERE session_type = 'LAB'`); err != nil {
		t.Fatal(err)
	}
	list, err := svc.Sessions(ctx, f.section)
	if err != nil {
		t.Fatal(err)
	}
	pastLabs := 0
	for _, x := range list {
		if x.Type == attendance.TypeLab {
			if x.StartsAt.After(time.Now()) {
				t.Errorf("programdan kalkan gelecek oturum kaldı: %v", x.StartsAt)
			}
			pastLabs++
		}
	}
	if pastLabs == 0 {
		t.Error("geçmiş laboratuvar oturumları silinmemeli")
	}
}

func TestTakeAttendance(t *testing.T) {
	pool := dbtest.New(t)
	f := seed(t, pool)
	ctx := context.Background()
	svc := attendance.NewService(pool, time.Now)
	list, err := svc.Sessions(ctx, f.section)
	if err != nil {
		t.Fatal(err)
	}
	var past, future attendance.Session
	for _, x := range list {
		if x.StartsAt.Before(time.Now()) {
			past = x
		} else if future.ID == "" && x.StartsAt.After(time.Now().Add(time.Hour)) {
			future = x
		}
	}

	if _, teaches, err := svc.Teaching(ctx, pool, f.section, f.teacher); err != nil || !teaches {
		t.Errorf("öğretim elemanı: %v %v", teaches, err)
	}
	if _, teaches, _ := svc.Teaching(ctx, pool, f.section, f.stranger); teaches {
		t.Error("başka öğretim elemanı şubenin yoklamasını alamaz")
	}

	// Doğrulamalar: bilinmeyen öğrenci, bırakmış öğrenci, geçersiz durum, eski sürüm, başlamamış oturum.
	var verr *attendance.ValidationError
	for name, statuses := range map[string]map[string]string{
		"bırakmış": {f.dropped: attendance.Absent},
		"durum":    {f.ali: "GONE"},
	} {
		if err := svc.Take(ctx, f.teacher, past.ID, past.Version, statuses); !errors.As(err, &verr) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := svc.Take(ctx, f.teacher, past.ID, past.Version+1, nil); !errors.Is(err, attendance.ErrVersionMismatch) {
		t.Errorf("eski sürüm: %v", err)
	}
	if err := svc.Take(ctx, f.teacher, future.ID, future.Version, nil); !errors.As(err, &verr) {
		t.Errorf("başlamamış oturum: %v", err)
	}

	// Yoklama: listede olmayan gelmiş sayılır; bırakmış öğrenci listede yok.
	if err := svc.Take(ctx, f.teacher, past.ID, past.Version, map[string]string{f.ali: attendance.Absent, f.ece: attendance.Late}); err != nil {
		t.Fatal(err)
	}
	x, roll, err := svc.Roll(ctx, past.ID)
	if err != nil {
		t.Fatal(err)
	}
	if x.Status != attendance.StatusHeld || x.TakenAt == nil || x.Present != 2 || x.Absent != 1 || x.Version != past.Version+1 {
		t.Errorf("oturum: %+v", x)
	}
	got := map[string]string{}
	for _, e := range roll {
		got[e.EnrollmentID] = e.Status
	}
	if len(roll) != 3 || got[f.ali] != attendance.Absent || got[f.ece] != attendance.Late || got[f.can] != attendance.Present {
		t.Errorf("yoklama listesi: %+v", roll)
	}
	var partition *string
	if err := pool.QueryRow(ctx, `SELECT to_regclass('attendance.attendance_records_2026_fall')::text`).Scan(&partition); err != nil || partition == nil {
		t.Errorf("dönem partition'ı açılmadı: %v", err)
	}

	// Düzeltme: aynı oturum yeniden yazılır.
	if err := svc.Take(ctx, f.teacher, past.ID, x.Version, map[string]string{f.ali: attendance.Excused}); err != nil {
		t.Fatal(err)
	}
	if x, _, _ := svc.Roll(ctx, past.ID); x.Absent != 0 || x.Present != 2 { // raporlu ne gelmiş ne devamsız
		t.Errorf("düzeltme: %+v", x)
	}

	// Yoklaması alınmış oturum iptal edilemez; iptal edilen oturumun yoklaması alınmaz, geri alınır.
	x, _ = svc.Session(ctx, pool, past.ID)
	if err := svc.UpdateSession(ctx, f.teacher, past.ID, x.Version, attendance.SessionUpdate{Cancelled: true}); !errors.Is(err, attendance.ErrHeld) {
		t.Errorf("yoklaması alınmış oturumun iptali: %v", err)
	}
	if err := svc.UpdateSession(ctx, f.teacher, future.ID, future.Version, attendance.SessionUpdate{Cancelled: true, Topic: "Ara sınav haftası"}); err != nil {
		t.Fatal(err)
	}
	cancelled, _ := svc.Session(ctx, pool, future.ID)
	if cancelled.Status != attendance.StatusCancelled || cancelled.Topic != "Ara sınav haftası" {
		t.Errorf("iptal: %+v", cancelled)
	}
	if err := svc.UpdateSession(ctx, f.teacher, future.ID, cancelled.Version, attendance.SessionUpdate{}); err != nil {
		t.Fatal(err)
	}
	if restored, _ := svc.Session(ctx, pool, future.ID); restored.Status != attendance.StatusScheduled || restored.Topic != "" {
		t.Errorf("iptalin geri alınması: %+v", restored)
	}

	// Telafi oturumu: dönem içinde, aynı anda başlayan oturum yokken.
	start := day(2).Add(17 * time.Hour)
	id, err := svc.AddMakeup(ctx, f.teacher, f.section, attendance.MakeupInput{Type: attendance.TypeTheory, StartsAt: start, EndsAt: start.Add(2 * time.Hour), Topic: "Telafi"})
	if err != nil {
		t.Fatal(err)
	}
	if m, _ := svc.Session(ctx, pool, id); m.Kind != attendance.KindMakeup || m.Classroom != "" {
		t.Errorf("telafi: %+v", m)
	}
	if _, err := svc.AddMakeup(ctx, f.teacher, f.section, attendance.MakeupInput{Type: attendance.TypeTheory, StartsAt: start, EndsAt: start.Add(time.Hour)}); !errors.As(err, &verr) {
		t.Errorf("aynı saatte ikinci oturum: %v", err)
	}
	if _, err := svc.AddMakeup(ctx, f.teacher, f.section, attendance.MakeupInput{Type: attendance.TypeTheory, StartsAt: day(200), EndsAt: day(200).Add(time.Hour)}); !errors.As(err, &verr) {
		t.Errorf("dönem dışı telafi: %v", err)
	}
	// Telafi oturumu programla eşitlemede silinmez.
	if list, _ := svc.Sessions(ctx, f.section); !containsID(list, id) {
		t.Error("telafi oturumu eşitlemede silindi")
	}
}

func containsID(list []attendance.Session, id string) bool {
	for _, x := range list {
		if x.ID == id {
			return true
		}
	}
	return false
}

func TestAbsenceReportAndWarnings(t *testing.T) {
	pool := dbtest.New(t)
	f := seed(t, pool)
	ctx := context.Background()
	svc := attendance.NewService(pool, time.Now)
	list, err := svc.Sessions(ctx, f.section)
	if err != nil {
		t.Fatal(err)
	}
	var labs, theory []attendance.Session
	var futureTheory attendance.Session
	for _, x := range list {
		switch {
		case x.StartsAt.After(time.Now()):
			if x.Type == attendance.TypeTheory && futureTheory.ID == "" {
				futureTheory = x
			}
		case x.Type == attendance.TypeLab:
			labs = append(labs, x)
		default:
			theory = append(theory, x)
		}
	}
	take := func(x attendance.Session, statuses map[string]string) {
		t.Helper()
		cur, err := svc.Session(ctx, pool, x.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err := svc.Take(ctx, f.teacher, x.ID, cur.Version, statuses); err != nil {
			t.Fatal(err)
		}
	}
	warnings := func(enrollment string) []string {
		t.Helper()
		rows, err := pool.Query(ctx, `
			SELECT n.payload->>'level' FROM communication.notifications n
			JOIN iam.users u ON u.id = n.user_id JOIN people.students st ON st.person_id = u.person_id
			JOIN enrollment.student_programs sp ON sp.student_id = st.id JOIN enrollment.enrollments e ON e.student_program_id = sp.id
			WHERE e.id = $1 AND n.type_code = 'attendance.warning' ORDER BY n.id`, enrollment)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatal(err)
			}
			out = append(out, s)
		}
		return out
	}

	// Ders dönemi tam 8 hafta: teori 7 oturum (biri tatil), sınır %30 → 2; laboratuvar 8 oturum,
	// sınır %20 → 1. Ali iki laboratuvara gelmez: birincide sınıra yaklaşır, ikincide aşar.
	take(labs[0], map[string]string{f.ali: attendance.Absent})
	if got := warnings(f.ali); len(got) != 1 || got[0] != "NEAR" {
		t.Fatalf("ilk devamsızlık: %v", got)
	}
	take(labs[1], map[string]string{f.ali: attendance.Absent, f.can: attendance.Late})
	take(labs[1], map[string]string{f.ali: attendance.Absent}) // düzeltme yeni uyarı göndermez
	if got := warnings(f.ali); len(got) != 2 || got[1] != "OVER" {
		t.Fatalf("sınır aşımı: %v", got)
	}
	// Teoride bir devamsızlık sınırın (2) %70'ine ulaşmaz.
	take(theory[0], map[string]string{f.ece: attendance.Absent})
	if got := warnings(f.ece); len(got) != 0 {
		t.Errorf("teoride erken uyarı: %v", got)
	}

	mine, err := svc.Mine(ctx, f.aliUser(t, pool), f.term)
	if err != nil {
		t.Fatal(err)
	}
	if len(mine) != 1 || mine[0].CourseCode != "BIL1001" || len(mine[0].Usage) != 2 || len(mine[0].Sessions) != len(list) {
		t.Fatalf("öğrencinin raporu: %+v", mine)
	}
	th, pr := mine[0].Usage[0], mine[0].Usage[1]
	if th.Group != attendance.GroupTheory || th.Planned != 7 || th.Absences != 0 || *th.Limit != 2 || th.Pct != 30 {
		t.Errorf("teori: %+v", th)
	}
	if pr.Group != attendance.GroupPractice || pr.Planned != 8 || pr.Absences != 2 || *pr.Limit != 1 || !pr.Exceeded() {
		t.Errorf("uygulama: %+v", pr)
	}
	absent := 0
	for _, x := range mine[0].Sessions {
		if x.Record == attendance.Absent {
			absent++
		}
	}
	if absent != 2 {
		t.Errorf("rapordaki devamsız oturumlar: %d", absent)
	}

	// İptal edilen oturum planlanandan düşer: teoride 6 oturum, sınır 1.
	if err := svc.UpdateSession(ctx, f.teacher, futureTheory.ID, futureTheory.Version, attendance.SessionUpdate{Cancelled: true}); err != nil {
		t.Fatal(err)
	}
	summary, err := svc.Summary(ctx, f.section)
	if err != nil {
		t.Fatal(err)
	}
	if len(summary) != 3 || summary[0].StudentNo != "25000001" {
		t.Fatalf("şube özeti: %+v", summary)
	}
	if u := summary[1].Usage[0]; u.Planned != 6 || *u.Limit != 1 || u.Absences != 1 || !u.Near() || u.Exceeded() {
		t.Errorf("Ece teori: %+v", u)
	}
	if u := summary[0].Usage[1]; !u.Exceeded() || summary[0].AttendanceStatus != "CONTINUING" {
		t.Errorf("Ali uygulama: %+v %s", u, summary[0].AttendanceStatus)
	}
}

// aliUser, Ali'nin kullanıcı kimliğidir.
func (f fixture) aliUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), `SELECT id FROM iam.users WHERE username = '25000001'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
