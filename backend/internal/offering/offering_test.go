package offering_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/offering"
	"github.com/CAPELLAX02/agora/backend/internal/org"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
)

// fixture, testlerin ortak verisidir: iki bölüm, bir program, bir dönem, dersler,
// derslikler ve öğretim elemanları.
type fixture struct {
	bil, fiz, program, term       string
	prog1, prog2, physics         string // dersler
	amphi, small, lab, closedRoom string // derslikler: 120, 30, 40 (lab), kullanım dışı
	ayse, mehmet, clerk           string // iki akademik personel ve bir idari personel
	ayseUser                      string
}

func seed(t *testing.T, pool *pgxpool.Pool) fixture {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		INSERT INTO org.campuses (code, name) VALUES ('GOLBASI', '50. Yıl Yerleşkesi');
		INSERT INTO org.faculties (code, name_tr, name_en, unit_type) VALUES
			('MUH', 'Mühendislik Fakültesi', 'Faculty of Engineering', 'FACULTY'),
			('FEN', 'Fen Fakültesi', 'Faculty of Science', 'FACULTY');
		INSERT INTO org.departments (faculty_id, code, name_tr, name_en)
			SELECT id, 'BIL', 'Bilgisayar Mühendisliği', 'Computer Engineering' FROM org.faculties WHERE code = 'MUH';
		INSERT INTO org.departments (faculty_id, code, name_tr, name_en)
			SELECT id, 'FIZ', 'Fizik', 'Physics' FROM org.faculties WHERE code = 'FEN';
		INSERT INTO org.programs (department_id, code, name_tr, name_en, degree_level, language, education_type,
		                          duration_semesters, max_duration_years, total_ects_required, has_prep_class)
			SELECT id, 'BIL-EN', 'Bilgisayar Mühendisliği (İngilizce)', 'Computer Engineering', 'BACHELOR', 'EN', 'DAYTIME', 8, 7, 240, true
			FROM org.departments WHERE code = 'BIL';
		INSERT INTO org.buildings (campus_id, code, name) SELECT id, 'MUH-A', 'Mühendislik A Blok' FROM org.campuses;
		INSERT INTO org.classrooms (building_id, code, name, capacity, exam_capacity, room_type, is_active)
		SELECT b.id, v.code, v.name, v.capacity, v.capacity / 2, v.room_type, v.active
		FROM org.buildings b CROSS JOIN (VALUES
			('AMFI1', 'Amfi 1', 120, 'AMPHI', true),
			('101', 'Derslik 101', 30, 'LECTURE', true),
			('LAB1', 'Bilgisayar Lab 1', 40, 'LAB', true),
			('102', 'Derslik 102', 60, 'LECTURE', false)
		) AS v(code, name, capacity, room_type, active);

		INSERT INTO academic.academic_years (start_year, starts_on, ends_on) VALUES (2026, '2026-09-01', '2027-08-31');
		INSERT INTO academic.terms (academic_year_id, term_type, code, starts_on, ends_on, status, is_current)
			SELECT id, 'FALL', '2026-FALL', '2026-09-21', '2027-01-31', 'ACTIVE', true FROM academic.academic_years;

		INSERT INTO curriculum.courses (code, owner_department_id, name_tr, name_en, theory_hours, practice_hours, national_credit, ects, language)
		SELECT v.code, d.id, v.tr, v.en, 3, 2, 4, 6, 'EN' FROM (VALUES
			('COM1001', 'Bilgisayar Programlama I', 'Computer Programming I', 'BIL'),
			('COM1002', 'Bilgisayar Programlama II', 'Computer Programming II', 'BIL'),
			('PHY0101', 'Fizik I', 'Physics I', 'FIZ')
		) AS v(code, tr, en, dept) JOIN org.departments d ON d.code = v.dept;`)
	if err != nil {
		t.Fatal(err)
	}
	var f fixture
	q := func(sql string, dest ...any) {
		t.Helper()
		if err := pool.QueryRow(ctx, sql).Scan(dest...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	q(`SELECT id FROM org.departments WHERE code = 'BIL'`, &f.bil)
	q(`SELECT id FROM org.departments WHERE code = 'FIZ'`, &f.fiz)
	q(`SELECT id FROM org.programs WHERE code = 'BIL-EN'`, &f.program)
	q(`SELECT id FROM academic.terms WHERE code = '2026-FALL'`, &f.term)
	q(`SELECT id FROM curriculum.courses WHERE code = 'COM1001'`, &f.prog1)
	q(`SELECT id FROM curriculum.courses WHERE code = 'COM1002'`, &f.prog2)
	q(`SELECT id FROM curriculum.courses WHERE code = 'PHY0101'`, &f.physics)
	q(`SELECT id FROM org.classrooms WHERE code = 'AMFI1'`, &f.amphi)
	q(`SELECT id FROM org.classrooms WHERE code = '101'`, &f.small)
	q(`SELECT id FROM org.classrooms WHERE code = 'LAB1'`, &f.lab)
	q(`SELECT id FROM org.classrooms WHERE code = '102'`, &f.closedRoom)

	staff := func(no, first, last, staffType, title string) (staffID, userID string) {
		t.Helper()
		err := pool.QueryRow(ctx, `
			WITH p AS (INSERT INTO people.persons (first_name, last_name) VALUES ($2, $3) RETURNING id),
			s AS (
				INSERT INTO people.staff (person_id, staff_no, staff_type, academic_title_code, primary_department_id)
				SELECT p.id, $1::text, $4, nullif($5, ''), d.id FROM p, org.departments d WHERE d.code = 'BIL' RETURNING id
			), u AS (
				INSERT INTO iam.users (person_id, username, email, password_hash)
				SELECT id, $1::text, $1::text || '@personel.test', 'x' FROM p RETURNING id
			)
			SELECT (SELECT id FROM s), (SELECT id FROM u)`, no, first, last, staffType, title).Scan(&staffID, &userID)
		if err != nil {
			t.Fatal(err)
		}
		return staffID, userID
	}
	f.ayse, f.ayseUser = staff("P1001", "Ayşe", "Yılmaz", "ACADEMIC", "ASSOC_PROF")
	f.mehmet, _ = staff("P1002", "Mehmet", "Demir", "ACADEMIC", "ASSIST_PROF")
	f.clerk, _ = staff("P1003", "Zeynep", "Kaya", "ADMINISTRATIVE", "")
	return f
}

func TestOfferingsAndSections(t *testing.T) {
	pool := dbtest.New(t)
	fx := seed(t, pool)
	repo := offering.NewRepository(pool)
	ctx := context.Background()

	oID, err := repo.CreateOffering(ctx, "", fx.term, offering.OfferingInput{CourseID: fx.prog1, DepartmentID: fx.bil, ExternalRef: "760535"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateOffering(ctx, "", fx.term, offering.OfferingInput{CourseID: fx.prog1, DepartmentID: fx.bil}); !errors.Is(err, offering.ErrConflict) {
		t.Errorf("aynı ders iki kez: %v", err)
	}
	section := offering.SectionInput{Code: "1", Capacity: 60, QuotaMode: "RESERVED", InstructionMode: "IN_PERSON", Language: "EN", Status: "ACTIVE"}
	s1, err := repo.CreateSection(ctx, "", oID, section)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateSection(ctx, "", oID, section); !errors.Is(err, offering.ErrConflict) {
		t.Errorf("aynı şube kodu: %v", err)
	}
	section.Code = "2"
	if _, err := repo.CreateSection(ctx, "", oID, section); err != nil {
		t.Fatal(err)
	}

	o, _ := repo.Offering(ctx, oID)
	if o.SectionCount != 2 || o.TotalCapacity != 120 || o.Course.Code != "COM1001" || o.TermCode != "2026-FALL" || o.Status != offering.StatusPlanned {
		t.Errorf("açılan ders = %+v", o)
	}

	// Kontenjanlar: toplam şube kontenjanını aşamaz.
	if err := repo.SetQuotas(ctx, "", s1, []offering.QuotaInput{{ProgramID: fx.program, Quota: 61}}); !errors.Is(err, offering.ErrQuotaExceeds) {
		t.Errorf("kontenjan aşımı: %v", err)
	}
	if err := repo.SetQuotas(ctx, "", s1, []offering.QuotaInput{{ProgramID: fx.program, Quota: 50}}); err != nil {
		t.Fatal(err)
	}
	sec, _ := repo.Section(ctx, s1)
	if len(sec.Quotas) != 1 || sec.Quotas[0].Program.Code != "BIL-EN" || sec.Quotas[0].Quota != 50 {
		t.Errorf("kontenjanlar = %+v", sec.Quotas)
	}
	// Program kontenjanı 50: şube kontenjanı 40'a inemez.
	if err := repo.UpdateSection(ctx, "", s1, sec.Version, offering.SectionInput{Capacity: 40, QuotaMode: "RESERVED", InstructionMode: "IN_PERSON", Language: "EN", Status: "ACTIVE"}); !errors.Is(err, offering.ErrQuotaExceeds) {
		t.Errorf("program kontenjanlarının altına inme: %v", err)
	}

	// Ders seçme (Faz 3) dolduracak sayıları elle veriyoruz.
	if _, err := pool.Exec(ctx, `UPDATE offering.sections SET enrolled_count = 10 WHERE id = $1`, s1); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE offering.section_quotas SET enrolled = 10 WHERE section_id = $1`, s1); err != nil {
		t.Fatal(err)
	}
	sec, _ = repo.Section(ctx, s1)
	if err := repo.UpdateSection(ctx, "", s1, sec.Version, offering.SectionInput{Capacity: 5, QuotaMode: "OPEN", InstructionMode: "IN_PERSON", Language: "EN", Status: "ACTIVE"}); !errors.Is(err, offering.ErrBelowEnrolled) {
		t.Errorf("kayıtlının altına inme: %v", err)
	}
	if err := repo.SetQuotas(ctx, "", s1, []offering.QuotaInput{}); !errors.Is(err, offering.ErrBelowEnrolled) {
		t.Errorf("öğrencisi olan programın kontenjanını kaldırma: %v", err)
	}
	if err := repo.SetQuotas(ctx, "", s1, []offering.QuotaInput{{ProgramID: fx.program, Quota: 20}}); err != nil {
		t.Fatal(err)
	}
	sec, _ = repo.Section(ctx, s1)
	if sec.Quotas[0].Enrolled != 10 {
		t.Errorf("kayıtlı sayısı korunmalı: %+v", sec.Quotas[0])
	}

	// Durum geçişleri: kayıtlı öğrenci varken iptal ya da silme olmaz.
	o, _ = repo.Offering(ctx, oID)
	if err := repo.UpdateOffering(ctx, "", oID, o.Version, offering.OfferingUpdate{Status: offering.StatusClosed}); !errors.Is(err, offering.ErrInvalidTransition) {
		t.Errorf("planlamadan kapanışa: %v", err)
	}
	if err := repo.UpdateOffering(ctx, "", oID, o.Version, offering.OfferingUpdate{Status: offering.StatusOpen}); err != nil {
		t.Fatal(err)
	}
	o, _ = repo.Offering(ctx, oID)
	if err := repo.UpdateOffering(ctx, "", oID, o.Version, offering.OfferingUpdate{Status: offering.StatusCancelled}); !errors.Is(err, offering.ErrHasEnrollments) {
		t.Errorf("öğrencisi olan dersi iptal: %v", err)
	}
	if err := repo.DeleteSection(ctx, "", s1); !errors.Is(err, offering.ErrHasEnrollments) {
		t.Errorf("öğrencisi olan şubeyi silme: %v", err)
	}

	// Kapanmış dönemde ders açılmaz.
	if _, err := pool.Exec(ctx, `UPDATE academic.terms SET status = 'CLOSED', is_current = false WHERE id = $1`, fx.term); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateOffering(ctx, "", fx.term, offering.OfferingInput{CourseID: fx.prog2, DepartmentID: fx.bil}); !errors.Is(err, offering.ErrTermClosed) {
		t.Errorf("kapanmış dönem: %v", err)
	}
}

func TestScheduleConflicts(t *testing.T) {
	pool := dbtest.New(t)
	fx := seed(t, pool)
	repo := offering.NewRepository(pool)
	ctx := context.Background()

	newSection := func(courseID, code string, capacity int) string {
		t.Helper()
		var oID string
		if err := pool.QueryRow(ctx, `SELECT id FROM offering.course_offerings WHERE term_id = $1 AND course_id = $2`, fx.term, courseID).Scan(&oID); err != nil {
			if oID, err = repo.CreateOffering(ctx, "", fx.term, offering.OfferingInput{CourseID: courseID, DepartmentID: fx.bil}); err != nil {
				t.Fatal(err)
			}
		}
		id, err := repo.CreateSection(ctx, "", oID, offering.SectionInput{Code: code, Capacity: capacity, QuotaMode: "OPEN", InstructionMode: "IN_PERSON", Language: "EN"})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	a := newSection(fx.prog1, "1", 100) // COM1001-1
	b := newSection(fx.prog2, "1", 25)  // COM1002-1
	c := newSection(fx.physics, "1", 25)

	mon := func(start, end, room, kind string) offering.SlotInput {
		return offering.SlotInput{DayOfWeek: 1, Start: start, End: end, ClassroomID: room, SessionType: kind}
	}
	if _, err := repo.AddSlot(ctx, "", a, mon("09:00", "10:50", fx.amphi, "THEORY")); err != nil {
		t.Fatal(err)
	}
	// Derslik çakışması: okunur mesaj.
	var classroom *offering.ClassroomConflictError
	if _, err := repo.AddSlot(ctx, "", b, mon("10:00", "11:50", fx.amphi, "THEORY")); !errors.As(err, &classroom) ||
		classroom.With.String() != "COM1001-1 (Pazartesi 09:00-10:50)" {
		t.Errorf("derslik çakışması: %v", err)
	}
	// Bitiş hariç: 10:50'de başlayan oturum çakışmaz.
	if _, err := repo.AddSlot(ctx, "", b, mon("10:50", "12:40", fx.amphi, "THEORY")); err != nil {
		t.Errorf("bitişik oturum: %v", err)
	}
	// Şubenin kendi oturumları.
	if _, err := repo.AddSlot(ctx, "", a, mon("10:00", "11:00", "", "PRACTICE")); !errors.Is(err, offering.ErrSectionOverlap) {
		t.Errorf("şube içi çakışma: %v", err)
	}
	// Kapasite: 100 kişilik şubenin teorik oturumu 30 kişilik dersliğe sığmaz; laboratuvar
	// oturumu gruplarla yapılabildiği için sığar. Kullanım dışı derslik verilemez.
	var small *offering.ClassroomTooSmallError
	if _, err := repo.AddSlot(ctx, "", a, mon("13:00", "14:50", fx.small, "THEORY")); !errors.As(err, &small) || small.Needed != 100 {
		t.Errorf("küçük derslik: %v", err)
	}
	if _, err := repo.AddSlot(ctx, "", a, mon("13:00", "14:50", fx.lab, "LAB")); err != nil {
		t.Errorf("laboratuvar: %v", err)
	}
	if _, err := repo.AddSlot(ctx, "", c, mon("15:00", "16:50", fx.closedRoom, "THEORY")); !errors.Is(err, offering.ErrClassroomInactive) {
		t.Errorf("kullanım dışı derslik: %v", err)
	}

	// Öğretim elemanı: akademik ve görevde olmalı, tek sorumlu.
	if err := repo.SetInstructors(ctx, "", a, []offering.InstructorInput{{StaffID: fx.clerk, Role: "PRIMARY"}}); !errors.Is(err, offering.ErrNotInstructor) {
		t.Errorf("idari personel: %v", err)
	}
	if err := repo.SetInstructors(ctx, "", a, []offering.InstructorInput{{StaffID: fx.ayse, Role: "CO_INSTRUCTOR"}}); !errors.Is(err, offering.ErrPrimaryRequired) {
		t.Errorf("sorumlusuz şube: %v", err)
	}
	if err := repo.SetInstructors(ctx, "", a, []offering.InstructorInput{{StaffID: fx.ayse, Role: "PRIMARY"}, {StaffID: fx.mehmet, Role: "ASSISTANT"}}); err != nil {
		t.Fatal(err)
	}
	// Ayşe pazartesi 09:00-10:50'de COM1001-1'de: aynı saatteki şubeye atanamaz.
	if _, err := repo.AddSlot(ctx, "", c, mon("09:30", "10:20", fx.small, "THEORY")); err != nil {
		t.Fatal(err)
	}
	var instructor *offering.InstructorConflictError
	if err := repo.SetInstructors(ctx, "", c, []offering.InstructorInput{{StaffID: fx.ayse, Role: "PRIMARY"}}); !errors.As(err, &instructor) ||
		instructor.StaffName != "Doç. Dr. Ayşe Yılmaz" || instructor.With.CourseCode != "COM1001" {
		t.Errorf("öğretim elemanı çakışması (atama): %v", err)
	}
	// Tersi: Mehmet'in şubesine Ayşe'nin dersiyle çakışan oturum eklenemez.
	if err := repo.SetInstructors(ctx, "", b, []offering.InstructorInput{{StaffID: fx.ayse, Role: "PRIMARY"}}); err != nil {
		t.Fatal(err) // b pazartesi 10:50'de başlıyor, a 10:50'de bitiyor
	}
	if _, err := repo.AddSlot(ctx, "", b, offering.SlotInput{DayOfWeek: 1, Start: "13:30", End: "14:20", SessionType: "PRACTICE"}); !errors.As(err, &instructor) {
		t.Errorf("öğretim elemanı çakışması (oturum): %v", err)
	}

	// Kontenjan artışı teorik oturumun dersliğini aşamaz (amfi 120).
	sec, _ := repo.Section(ctx, a)
	if err := repo.UpdateSection(ctx, "", a, sec.Version, offering.SectionInput{Capacity: 150, QuotaMode: "OPEN", InstructionMode: "IN_PERSON", Language: "EN", Status: "ACTIVE"}); !errors.As(err, &small) {
		t.Errorf("kontenjan artışı: %v", err)
	}
	// İptal edilen şubenin oturumları silinir, dersliği boşalır.
	sec, _ = repo.Section(ctx, b)
	if err := repo.UpdateSection(ctx, "", b, sec.Version, offering.SectionInput{Capacity: 25, QuotaMode: "OPEN", InstructionMode: "IN_PERSON", Language: "EN", Status: "CANCELLED"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.AddSlot(ctx, "", c, mon("11:00", "12:00", fx.amphi, "THEORY")); err != nil {
		t.Errorf("iptalden sonra boşalan derslik: %v", err)
	}

	// Görünümler.
	entries, err := repo.Schedule(ctx, offering.ScheduleFilter{TermID: fx.term, StaffID: fx.ayse})
	if err != nil || len(entries) != 2 || entries[0].Course.Code != "COM1001" || len(entries[0].Instructors) != 2 {
		t.Errorf("Ayşe'nin programı = %+v, %v", entries, err)
	}
	entries, _ = repo.Schedule(ctx, offering.ScheduleFilter{TermID: fx.term, ClassroomID: fx.amphi})
	if len(entries) != 2 || entries[1].Start != "11:00" {
		t.Errorf("amfinin programı = %+v", entries)
	}
	entries, _ = repo.Schedule(ctx, offering.ScheduleFilter{TermID: fx.term, DepartmentID: fx.bil})
	if len(entries) != 4 {
		t.Errorf("bölümün programı = %d", len(entries))
	}
	found, _ := repo.SearchInstructors(ctx, "ayş", "", 10)
	if len(found) != 1 || found[0].Title != "Doç. Dr." {
		t.Errorf("arama = %+v", found)
	}
	if found, _ := repo.SearchInstructors(ctx, "zeynep", "", 10); len(found) != 0 {
		t.Errorf("idari personel aramada çıkmamalı: %+v", found)
	}
}

// --- HTTP ----------------------------------------------------------------------

type grants map[string][]authz.Grant

func (g grants) Permissions(ctx context.Context, userID string) (*authz.Permissions, error) {
	return authz.NewPermissions(userID, g[userID]), nil
}

const (
	head    = "01a11b7f-0000-7000-8000-0000000000c1" // bilgisayar mühendisliği bölüm başkanı
	other   = "01a11b7f-0000-7000-8000-0000000000c2" // fizik bölüm başkanı
	student = "01a11b7f-0000-7000-8000-0000000000c3"
)

type server struct {
	t  *testing.T
	h  http.Handler
	fx fixture
}

func newServer(t *testing.T) *server {
	t.Helper()
	pool := dbtest.New(t)
	fx := seed(t, pool)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	authenticate := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(authn.WithPrincipal(r.Context(), authn.Principal{UserID: r.Header.Get("X-Test-User")})))
		})
	}
	read := authz.Grant{Permission: "course:read", ScopeType: authz.ScopeNone}
	manage := func(dept string) []authz.Grant {
		out := []authz.Grant{read}
		for _, p := range []string{"offering:manage", "section:manage", "quota:manage", "schedule:manage"} {
			out = append(out, authz.Grant{Permission: p, ScopeType: authz.ScopeDepartment, ScopeID: dept})
		}
		return out
	}
	resolver := grants{head: manage(fx.bil), other: manage(fx.fiz), student: {read}, fx.ayseUser: {read}}
	mux := http.NewServeMux()
	offering.NewHandler(offering.NewRepository(pool), org.NewTargets(pool), logger).
		Register(authz.NewRouter(mux, authenticate, resolver, logger))
	return &server{t: t, h: mux, fx: fx}
}

type result struct {
	status int
	header http.Header
	body   map[string]any
	raw    string
}

func (r result) code() string {
	c, _ := r.body["code"].(string)
	return c
}

func (r result) items() []any {
	items, _ := r.body["items"].([]any)
	return items
}

func (s *server) do(method, path, user string, headers map[string]string, body any) result {
	s.t.Helper()
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
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	s.h.ServeHTTP(rec, req)
	res := result{status: rec.Code, header: rec.Header(), raw: rec.Body.String()}
	_ = json.Unmarshal(rec.Body.Bytes(), &res.body)
	return res
}

func TestHTTPOfferings(t *testing.T) {
	s := newServer(t)
	offerings := "/api/v1/terms/" + s.fx.term + "/offerings"

	body := map[string]any{"course_id": s.fx.prog1, "department_id": s.fx.bil, "external_ref": "760535"}
	if res := s.do("POST", offerings, other, nil, body); res.status != 403 {
		t.Errorf("başka bölüm: %d", res.status)
	}
	if res := s.do("POST", offerings, student, nil, body); res.status != 403 {
		t.Errorf("öğrenci: %d", res.status)
	}
	res := s.do("POST", offerings, head, nil, body)
	if res.status != 201 || res.body["status"] != "PLANNED" || res.body["course"].(map[string]any)["code"] != "COM1001" {
		t.Fatalf("ders açma: %d %s", res.status, res.raw)
	}
	oID := res.body["id"].(string)
	if res := s.do("POST", offerings, head, nil, body); res.status != 409 || res.code() != "COURSE_ALREADY_OFFERED" {
		t.Errorf("aynı ders: %d %s", res.status, res.raw)
	}

	// Şube: dil varsayılanı dersin dili.
	res = s.do("POST", "/api/v1/offerings/"+oID+"/sections", head, nil, map[string]any{"section_code": "1", "capacity": 100})
	if res.status != 201 || len(res.body["sections"].([]any)) != 1 {
		t.Fatalf("şube: %d %s", res.status, res.raw)
	}
	sec := res.body["sections"].([]any)[0].(map[string]any)
	if sec["language"] != "EN" || sec["quota_mode"] != "OPEN" {
		t.Errorf("şube varsayılanları: %v", sec)
	}
	sID := sec["id"].(string)
	if res := s.do("POST", "/api/v1/offerings/"+oID+"/sections", head, nil, map[string]any{"section_code": "çok uzun"}); res.status != 400 {
		t.Errorf("geçersiz şube: %d", res.status)
	}

	// Öğretim elemanı ve oturumlar.
	res = s.do("PUT", "/api/v1/sections/"+sID+"/instructors", head, nil, map[string]any{"items": []any{map[string]any{"staff_id": s.fx.ayse, "role": "PRIMARY"}}})
	if res.status != 200 || len(res.body["instructors"].([]any)) != 1 {
		t.Fatalf("öğretim elemanı: %d %s", res.status, res.raw)
	}
	slot := map[string]any{"day_of_week": 1, "start_time": "09:00", "end_time": "10:50", "classroom_id": s.fx.amphi}
	res = s.do("POST", "/api/v1/sections/"+sID+"/slots", head, nil, slot)
	if res.status != 201 || len(res.body["slots"].([]any)) != 1 {
		t.Fatalf("oturum: %d %s", res.status, res.raw)
	}
	slotID := res.body["slots"].([]any)[0].(map[string]any)["id"].(string)
	if res := s.do("POST", "/api/v1/sections/"+sID+"/slots", head, nil, map[string]any{"day_of_week": 8, "start_time": "9", "end_time": "08:00"}); res.status != 400 {
		t.Errorf("geçersiz oturum: %d", res.status)
	}

	// İkinci ders: aynı derslik ve saat.
	res = s.do("POST", offerings, head, nil, map[string]any{"course_id": s.fx.prog2, "department_id": s.fx.bil})
	o2 := res.body["id"].(string)
	res = s.do("POST", "/api/v1/offerings/"+o2+"/sections", head, nil, map[string]any{"section_code": "1", "capacity": 25})
	s2 := res.body["sections"].([]any)[0].(map[string]any)["id"].(string)
	res = s.do("POST", "/api/v1/sections/"+s2+"/slots", head, nil, map[string]any{"day_of_week": 1, "start_time": "10:00", "end_time": "11:00", "classroom_id": s.fx.amphi})
	if res.status != 409 || res.code() != "CLASSROOM_CONFLICT" || !strings.Contains(res.body["detail"].(string), "COM1001-1 (Pazartesi 09:00-10:50)") {
		t.Errorf("derslik çakışması: %d %s", res.status, res.raw)
	}
	res = s.do("POST", "/api/v1/sections/"+s2+"/slots", head, nil, map[string]any{"day_of_week": 1, "start_time": "10:00", "end_time": "11:00"})
	if res.status != 201 {
		t.Fatalf("dersliksiz oturum: %d %s", res.status, res.raw)
	}
	res = s.do("PUT", "/api/v1/sections/"+s2+"/instructors", head, nil, map[string]any{"items": []any{map[string]any{"staff_id": s.fx.ayse, "role": "PRIMARY"}}})
	if res.status != 409 || res.code() != "INSTRUCTOR_CONFLICT" || !strings.Contains(res.body["detail"].(string), "Doç. Dr. Ayşe Yılmaz") {
		t.Errorf("öğretim elemanı çakışması: %d %s", res.status, res.raw)
	}
	res = s.do("POST", "/api/v1/sections/"+s2+"/slots", head, nil, map[string]any{"day_of_week": 2, "start_time": "09:00", "end_time": "10:00", "classroom_id": s.fx.small})
	if res.status != 201 {
		t.Fatalf("salı oturumu: %d %s", res.status, res.raw)
	}
	res = s.do("PUT", "/api/v1/sections/"+s2, head, map[string]string{"If-Match": `"1"`}, map[string]any{"capacity": 45})
	if res.status != 409 || res.code() != "CLASSROOM_TOO_SMALL" {
		t.Errorf("kapasite artışı: %d %s", res.status, res.raw)
	}

	// Kontenjan.
	if res := s.do("PUT", "/api/v1/sections/"+sID+"/quotas", head, nil, map[string]any{"items": []any{map[string]any{"program_id": s.fx.program, "quota": 120}}}); res.status != 409 || res.code() != "QUOTA_EXCEEDS_CAPACITY" {
		t.Errorf("kontenjan aşımı: %d %s", res.status, res.raw)
	}
	if res := s.do("PUT", "/api/v1/sections/"+sID+"/quotas", other, nil, map[string]any{"items": []any{}}); res.status != 403 {
		t.Errorf("başka bölümün şubesi: %d", res.status)
	}

	// Görünümler.
	res = s.do("GET", "/api/v1/terms/"+s.fx.term+"/schedule?department_id="+s.fx.bil, student, nil, nil)
	if res.status != 200 || len(res.items()) != 3 {
		t.Errorf("bölüm programı: %d %s", res.status, res.raw)
	}
	if res := s.do("GET", "/api/v1/terms/"+s.fx.term+"/schedule", student, nil, nil); res.status != 400 {
		t.Errorf("süzgeçsiz program: %d", res.status)
	}
	res = s.do("GET", "/api/v1/me/teaching", s.fx.ayseUser, nil, nil)
	if res.status != 200 || len(res.items()) != 1 || res.items()[0].(map[string]any)["course"].(map[string]any)["code"] != "COM1001" {
		t.Errorf("verdiğim dersler: %d %s", res.status, res.raw)
	}
	if res := s.do("GET", "/api/v1/me/teaching", student, nil, nil); res.status != 200 || len(res.items()) != 0 {
		t.Errorf("personel olmayan: %d %s", res.status, res.raw)
	}
	res = s.do("GET", offerings+"?q=com100&limit=1", student, nil, nil)
	if len(res.items()) != 1 || res.body["next_cursor"] == nil {
		t.Errorf("liste: %s", res.raw)
	}
	if res := s.do("GET", "/api/v1/instructors?q=mehmet", head, nil, nil); res.status != 200 || len(res.items()) != 1 {
		t.Errorf("öğretim elemanı arama: %d %s", res.status, res.raw)
	}

	// Oturum silme ve ders açmanın durumu.
	if res := s.do("DELETE", "/api/v1/schedule-slots/"+slotID, other, nil, nil); res.status != 403 {
		t.Errorf("başka bölüm oturum silemez: %d", res.status)
	}
	if res := s.do("DELETE", "/api/v1/schedule-slots/"+slotID, head, nil, nil); res.status != 204 {
		t.Errorf("oturum silme: %d", res.status)
	}
	res = s.do("PUT", "/api/v1/offerings/"+oID, head, map[string]string{"If-Match": `"1"`}, map[string]any{"status": "OPEN", "external_ref": "760535"})
	if res.status != 200 || res.body["status"] != "OPEN" {
		t.Errorf("açma: %d %s", res.status, res.raw)
	}
	if res := s.do("DELETE", "/api/v1/offerings/"+oID, head, nil, nil); res.status != 409 || res.code() != "INVALID_STATUS_TRANSITION" {
		t.Errorf("açık dersi silme: %d %s", res.status, res.raw)
	}
	if res := s.do("DELETE", "/api/v1/offerings/"+o2, head, nil, nil); res.status != 204 {
		t.Errorf("planlanan dersi silme: %d %s", res.status, res.raw)
	}
}
