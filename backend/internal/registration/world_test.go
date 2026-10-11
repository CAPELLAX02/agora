package registration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/enrollment"
	"github.com/CAPELLAX02/agora/backend/internal/iam"
	"github.com/CAPELLAX02/agora/backend/internal/people"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
	"github.com/CAPELLAX02/agora/backend/internal/platform/idempotency"
	"github.com/CAPELLAX02/agora/backend/internal/platform/redistest"
	"github.com/CAPELLAX02/agora/backend/internal/registration"
)

// world, ders seçme testlerinin dünyasıdır. Mühendislik Fakültesi'nin Bilgisayar
// Mühendisliği programı, üç yarıyıllık küçük bir müfredat, 2026 güzünde açılan şubeler ve
// ders seçme ile danışman onayı pencereleri:
//
//  1. yarıyıl: MAT101 (yerini MAT111 almış), PRG101, FIZ101
//  2. yarıyıl: PRG102, MAT102
//  3. yarıyıl: VY201 (ön koşulu PRG102'ye devam), ALG201, DB201, bir teknik seçmeli (TE)
//  5. yarıyıl: OS301, iki teknik seçmeli
//
// ali (3. yarıyıl): MAT101'den kaldı, PRG102'ye devamsızlıktan kaldı, GANO 2.40 (40 AKTS).
// berk (3. yarıyıl): hepsini geçti, GANO 3.20 (45 AKTS).
type world struct {
	t    *testing.T
	pool *pgxpool.Pool
	h    http.Handler
	now  time.Time

	program, otherProgram, term string
	courses                     map[string]string // kod → kimlik
	sections                    map[string]string // ders kodu → 2026 güz şubesi

	advisor, head, registrar string // kullanıcılar
	ali, berk                string // öğrenci kullanıcıları
	aliSP, berkSP            string // program kayıtları
}

func newWorld(t *testing.T) *world {
	t.Helper()
	ctx := context.Background()
	w := &world{t: t, pool: dbtest.New(t), courses: map[string]string{}, sections: map[string]string{},
		now: time.Date(2026, time.September, 10, 9, 0, 0, 0, time.UTC)}

	w.exec(`
		INSERT INTO org.faculties (code, name_tr, name_en, unit_type) VALUES ('MUH', 'Mühendislik Fakültesi', 'Engineering', 'FACULTY');
		INSERT INTO org.departments (faculty_id, code, name_tr, name_en) SELECT id, 'BIL', 'Bilgisayar Mühendisliği', 'CE' FROM org.faculties;
		INSERT INTO org.programs (department_id, code, name_tr, name_en, degree_level, language, education_type,
		                          duration_semesters, max_duration_years, total_ects_required)
		SELECT id, p.code, p.code, p.code, 'BACHELOR', 'EN', 'DAYTIME', 8, 7, 240 FROM org.departments,
		       (VALUES ('BIL-L'), ('YZV-L')) AS p(code);

		INSERT INTO academic.academic_years (start_year, starts_on, ends_on)
		VALUES (2025, '2025-09-01', '2026-08-31'), (2026, '2026-09-01', '2027-08-31');
		INSERT INTO academic.terms (academic_year_id, term_type, code, starts_on, ends_on, status, is_current)
		SELECT ay.id, v.typ, v.code, v.s::date, v.e::date, v.status, v.cur
		FROM academic.academic_years ay JOIN (VALUES
			(2025, 'FALL', '2025-FALL', '2025-09-15', '2026-02-05', 'CLOSED', false),
			(2025, 'SPRING', '2025-SPRING', '2026-02-10', '2026-07-03', 'CLOSED', false),
			(2026, 'FALL', '2026-FALL', '2026-09-14', '2027-02-05', 'ACTIVE', true)
		) AS v(y, typ, code, s, e, status, cur) ON v.y = ay.start_year;

		INSERT INTO curriculum.courses (code, owner_department_id, name_tr, name_en, theory_hours, national_credit, ects, language)
		SELECT v.code, d.id, v.name, v.name, 3, 3, v.ects, 'EN' FROM org.departments d, (VALUES
			('MAT101', 'Matematik I', 6), ('MAT111', 'Matematik I (yeni)', 6), ('PRG101', 'Programlama I', 7),
			('FIZ101', 'Fizik I', 5), ('PRG102', 'Programlama II', 7), ('MAT102', 'Matematik II', 6),
			('VY201', 'Veri Yapıları', 6), ('ALG201', 'Algoritmalar', 5), ('DB201', 'Veritabanları', 5),
			('OS301', 'İşletim Sistemleri', 5), ('SEC301', 'Seçmeli A', 4), ('SEC302', 'Seçmeli B', 4), ('SEC303', 'Seçmeli C', 4)
		) AS v(code, name, ects);

		INSERT INTO curriculum.course_equivalences (course_id, equivalent_course_id, is_bidirectional)
		SELECT n.id, o.id, true FROM curriculum.courses n, curriculum.courses o WHERE n.code = 'MAT111' AND o.code = 'MAT101';
		INSERT INTO curriculum.course_prerequisites (course_id, prerequisite_course_id, requirement)
		SELECT c.id, p.id, 'ATTENDED' FROM curriculum.courses c, curriculum.courses p WHERE c.code = 'VY201' AND p.code = 'PRG102';
		INSERT INTO curriculum.elective_groups (code, name_tr, name_en, group_kind) VALUES ('TEKSEC', 'Teknik seçmeli', 'Technical elective', 'TECHNICAL');
		INSERT INTO curriculum.elective_group_courses (elective_group_id, course_id)
		SELECT g.id, c.id FROM curriculum.elective_groups g, curriculum.courses c WHERE c.code IN ('SEC301', 'SEC302', 'SEC303');

		INSERT INTO curriculum.curricula (program_id, name_tr, name_en, effective_from_year, total_ects_required, status, activated_at)
		SELECT id, 'Test planı', 'Test plan', 2020, 240, 'ACTIVE', now() FROM org.programs WHERE code = 'BIL-L';
		INSERT INTO curriculum.curriculum_items (curriculum_id, semester_no, item_type, course_id, is_compulsory, position)
		SELECT cu.id, v.sem, 'COURSE', c.id, true, v.pos FROM curriculum.curricula cu, curriculum.courses c, (VALUES
			('MAT101', 1, 1), ('PRG101', 1, 2), ('FIZ101', 1, 3), ('PRG102', 2, 1), ('MAT102', 2, 2),
			('VY201', 3, 1), ('ALG201', 3, 2), ('DB201', 3, 3), ('OS301', 5, 1)
		) AS v(code, sem, pos) WHERE c.code = v.code;
		INSERT INTO curriculum.curriculum_items (curriculum_id, semester_no, item_type, elective_group_id, slot_theory_hours,
		                                         slot_practice_hours, slot_national_credit, slot_ects, is_compulsory, position, slot_course_count)
		SELECT cu.id, v.sem, 'ELECTIVE_SLOT', g.id, 3, 0, 3, v.ects, false, 9, v.n
		FROM curriculum.curricula cu, curriculum.elective_groups g, (VALUES (3, 4, 1), (5, 8, 2)) AS v(sem, ects, n);`)

	for code, id := range w.idsByCode(`SELECT code, id FROM curriculum.courses`) {
		w.courses[code] = id
	}
	_ = w.pool.QueryRow(ctx, `SELECT id FROM org.programs WHERE code = 'BIL-L'`).Scan(&w.program)
	_ = w.pool.QueryRow(ctx, `SELECT id FROM org.programs WHERE code = 'YZV-L'`).Scan(&w.otherProgram)
	_ = w.pool.QueryRow(ctx, `SELECT id FROM academic.terms WHERE code = '2026-FALL'`).Scan(&w.term)

	// 2026 güz şubeleri (gün, başlangıç, bitiş saati, kontenjan). DB201, MAT111 ile pazartesi;
	// SEC303, ALG201 ile salı çakışır. SEC301'in tek kişilik kontenjanı var.
	for _, s := range []struct {
		code       string
		day        int
		start, end string
		capacity   int
	}{
		{"MAT111", 1, "09:00", "10:50", 40}, {"DB201", 1, "10:00", "11:50", 40}, {"ALG201", 2, "09:00", "10:50", 40},
		{"VY201", 3, "09:00", "10:50", 40}, {"SEC301", 4, "09:00", "10:50", 1}, {"SEC302", 4, "13:00", "14:50", 40},
		{"SEC303", 2, "10:00", "11:50", 40}, {"OS301", 5, "09:00", "10:50", 40},
	} {
		var id string
		err := w.pool.QueryRow(ctx, `
			WITH o AS (
				INSERT INTO offering.course_offerings (term_id, course_id, department_id, status)
				SELECT $1, $2, owner_department_id, 'OPEN' FROM curriculum.courses WHERE id = $2 RETURNING id
			), s AS (
				INSERT INTO offering.sections (offering_id, section_code, capacity, language) SELECT id, '1', $3, 'EN' FROM o RETURNING id
			), sl AS (
				INSERT INTO offering.schedule_slots (section_id, term_id, day_of_week, time_span, session_type)
				SELECT id, $1, $4, offering.timerange($5::time, $6::time, '[)'), 'THEORY' FROM s
			)
			SELECT id FROM s`, w.term, w.courses[s.code], s.capacity, s.day, s.start, s.end).Scan(&id)
		if err != nil {
			t.Fatalf("%s şubesi: %v", s.code, err)
		}
		w.sections[s.code] = id
	}
	// SEC302 sadece diğer programa ayrılmış.
	w.exec(`UPDATE offering.sections SET quota_mode = 'RESERVED' WHERE id = '` + w.sections["SEC302"] + `';
		INSERT INTO offering.section_quotas (section_id, program_id, quota) VALUES ('` + w.sections["SEC302"] + `', '` + w.otherProgram + `', 40);`)

	// Pencereler: ders seçme 7-20 Eylül, danışman onayı 7-22 Eylül.
	w.exec(`
		INSERT INTO academic.calendar_events (term_id, event_type_code, period, scope_type)
		VALUES ('` + w.term + `', 'COURSE_REGISTRATION', tstzrange('2026-09-07 07:00+00', '2026-09-20 21:00+00'), 'UNIVERSITY'),
		       ('` + w.term + `', 'ADVISOR_APPROVAL', tstzrange('2026-09-07 07:00+00', '2026-09-22 21:00+00'), 'UNIVERSITY');`)

	var bil, muh string
	_ = w.pool.QueryRow(ctx, `SELECT id, faculty_id FROM org.departments`).Scan(&bil, &muh)
	var advisorStaff string
	w.advisor, advisorStaff = w.staff("P1001", "ADVISOR", iam.ScopeDepartment, bil)
	w.head, _ = w.staff("P1002", "DEPARTMENT_HEAD", iam.ScopeDepartment, bil)
	w.registrar, _ = w.staff("P2001", "FACULTY_REGISTRAR", iam.ScopeFaculty, muh)

	w.ali, w.aliSP = w.student("24000001", 3, 2.40, advisorStaff)
	w.berk, w.berkSP = w.student("24000002", 3, 3.20, advisorStaff)
	w.history(w.aliSP, map[string]string{"MAT101": "F2", "PRG101": "B1", "FIZ101": "C1"},
		map[string]string{"PRG102": "F1", "MAT102": "B2"})
	w.history(w.berkSP, map[string]string{"MAT101": "B1", "PRG101": "A", "FIZ101": "B2"},
		map[string]string{"PRG102": "B3", "MAT102": "A"})

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	resolver := iam.NewPermissionResolver(w.pool, redistest.New(t), time.Hour, logger)
	authenticate := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			p := authn.Principal{UserID: r.Header.Get("X-Test-User"), AMR: []string{"pwd"}}
			if r.Header.Get("X-Test-MFA") != "" { // iki adımlı doğrulamayla açılmış oturum
				p.AMR = append(p.AMR, authn.MethodOTP)
			}
			next.ServeHTTP(rw, r.WithContext(authn.WithPrincipal(r.Context(), p)))
		})
	}
	mux := http.NewServeMux()
	rt := authz.NewRouter(mux, authenticate, resolver, logger)
	registration.NewHandler(w.pool, logger, func() time.Time { return w.now }, nil).
		WithMutationMiddleware(idempotency.Middleware(w.pool, logger)).Register(rt)
	enrollment.NewHandler(enrollment.NewRepository(w.pool), enrollment.NewService(w.pool), logger).Register(rt)
	w.h = mux
	return w
}

func (w *world) exec(sql string) {
	w.t.Helper()
	if _, err := w.pool.Exec(context.Background(), sql); err != nil {
		w.t.Fatal(err)
	}
}

func (w *world) idsByCode(sql string) map[string]string {
	w.t.Helper()
	rows, err := w.pool.Query(context.Background(), sql)
	if err != nil {
		w.t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var code, id string
		if err := rows.Scan(&code, &id); err != nil {
			w.t.Fatal(err)
		}
		out[code] = id
	}
	return out
}

func (w *world) account(personID, username string) string {
	w.t.Helper()
	id, err := iam.NewRepository(w.pool).CreateUser(context.Background(), iam.NewUser{
		PersonID: personID, Username: username, Email: strings.ToLower(username) + "@agora.test", PasswordHash: "x",
	})
	if err != nil {
		w.t.Fatal(err)
	}
	return id
}

func (w *world) staff(no, role string, scope iam.ScopeType, scopeID string) (userID, staffID string) {
	w.t.Helper()
	ctx := context.Background()
	pr := people.NewRepository(w.pool)
	personID, err := pr.CreatePerson(ctx, people.NewPerson{FirstName: "Ad" + no, LastName: "Soyad" + no})
	if err != nil {
		w.t.Fatal(err)
	}
	if err := pr.CreateStaff(ctx, people.NewStaff{PersonID: personID, StaffNo: no, Type: people.StaffAcademic, AcademicTitle: "PROF"}); err != nil {
		w.t.Fatal(err)
	}
	userID = w.account(personID, no)
	if err := iam.NewRepository(w.pool).AssignRole(ctx, userID, role, scope, scopeID); err != nil {
		w.t.Fatal(err)
	}
	_ = w.pool.QueryRow(ctx, `SELECT id FROM people.staff WHERE staff_no = $1`, no).Scan(&staffID)
	return userID, staffID
}

// student, 2024 girişli, verilen yarıyılda ve GANO'da bir öğrenci oluşturur, test
// müfredatına bağlar ve danışmanını atar.
func (w *world) student(no string, semester int, gpa float64, advisorStaff string) (userID, spID string) {
	w.t.Helper()
	ctx := context.Background()
	pr := people.NewRepository(w.pool)
	personID, err := pr.CreatePerson(ctx, people.NewPerson{FirstName: "Öğrenci", LastName: no})
	if err != nil {
		w.t.Fatal(err)
	}
	if err := pr.CreateStudent(ctx, personID, no); err != nil {
		w.t.Fatal(err)
	}
	userID = w.account(personID, no)
	if err := iam.NewRepository(w.pool).AssignRole(ctx, userID, "STUDENT", iam.ScopeNone, ""); err != nil {
		w.t.Fatal(err)
	}
	var studentID string
	_ = w.pool.QueryRow(ctx, `SELECT id FROM people.students WHERE student_no = $1`, no).Scan(&studentID)
	svc := enrollment.NewService(w.pool)
	spID, err = svc.CreateStudentProgram(ctx, "", enrollment.NewStudentProgram{
		StudentID: studentID, ProgramID: w.program, Kind: enrollment.KindMajor, AdmissionType: enrollment.AdmissionOSYS,
		AdmissionYear: 2024, AdmittedOn: time.Date(2024, 9, 15, 0, 0, 0, 0, time.UTC), Status: enrollment.StatusActive,
		ClassLevel: (semester + 1) / 2, CurrentSemester: semester,
	})
	if err != nil {
		w.t.Fatal(err)
	}
	if err := svc.AssignAdvisor(ctx, "", spID, advisorStaff, "Test"); err != nil {
		w.t.Fatal(err)
	}
	w.exec(fmt.Sprintf(`UPDATE enrollment.student_programs SET gpa_cache = %.2f,
		curriculum_id = (SELECT id FROM curriculum.curricula) WHERE id = '%s'`, gpa, spID))
	return userID, spID
}

// history, öğrencinin 2025 güz ve bahar dönemlerinde aldığı dersleri ve notlarını yazar.
func (w *world) history(spID string, fall, spring map[string]string) {
	w.t.Helper()
	for term, grades := range map[string]map[string]string{"2025-FALL": fall, "2025-SPRING": spring} {
		for code, letter := range grades {
			attendance := "CONTINUING"
			if letter == "F1" {
				attendance = "ATTENDANCE_FAIL"
			}
			_, err := w.pool.Exec(context.Background(), `
				WITH o AS (
					INSERT INTO offering.course_offerings (term_id, course_id, department_id, status)
					SELECT t.id, c.id, c.owner_department_id, 'CLOSED' FROM academic.terms t, curriculum.courses c
					WHERE t.code = $1 AND c.code = $2
					ON CONFLICT (term_id, course_id) DO UPDATE SET status = 'CLOSED' RETURNING id, term_id, course_id
				), s AS (
					INSERT INTO offering.sections (offering_id, section_code, capacity, enrolled_count, language)
					SELECT id, '1', 100, 0, 'EN' FROM o
					ON CONFLICT (offering_id, section_code) DO UPDATE SET section_code = '1' RETURNING id
				)
				INSERT INTO enrollment.enrollments (student_program_id, section_id, term_id, course_id, attempt_no, status,
					attendance_status, grade_scale_id, letter_grade, ects_snapshot, credit_snapshot, source)
				SELECT $3, s.id, o.term_id, o.course_id, 1, 'COMPLETED', $5,
				       (SELECT id FROM curriculum.grade_scales WHERE is_default), $4, c.ects, c.national_credit, 'IMPORT'
				FROM s, o JOIN curriculum.courses c ON c.id = o.course_id`, term, code, spID, letter, attendance)
			if err != nil {
				w.t.Fatalf("%s %s: %v", term, code, err)
			}
		}
	}
}

type res struct {
	status int
	header http.Header
	body   map[string]any
	raw    string
}

func (w *world) do(method, path, user string, body any, headers ...string) res {
	w.t.Helper()
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-Test-User", user)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	w.h.ServeHTTP(rec, req)
	out := res{status: rec.Code, header: rec.Header(), raw: rec.Body.String()}
	_ = json.Unmarshal(rec.Body.Bytes(), &out.body)
	return out
}

func (r res) items() []any {
	items, _ := r.body["items"].([]any)
	return items
}

func obj(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}
