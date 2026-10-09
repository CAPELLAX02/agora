package grading_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/grading"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
)

type fixture struct {
	bil, section         string
	ayseUser, mehmetUser string // ayşe şubenin sorumlusu, mehmet aynı bölümde ama bu şubede değil
}

func seed(t *testing.T, pool *pgxpool.Pool) fixture {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		INSERT INTO org.faculties (code, name_tr, name_en, unit_type) VALUES ('MUH', 'Mühendislik Fakültesi', 'Faculty of Engineering', 'FACULTY');
		INSERT INTO org.departments (faculty_id, code, name_tr, name_en)
			SELECT id, 'BIL', 'Bilgisayar Mühendisliği', 'Computer Engineering' FROM org.faculties;
		INSERT INTO academic.academic_years (start_year, starts_on, ends_on) VALUES (2026, '2026-09-01', '2027-08-31');
		INSERT INTO academic.terms (academic_year_id, term_type, code, starts_on, ends_on, status, is_current)
			SELECT id, 'FALL', '2026-FALL', '2026-09-21', '2027-01-31', 'ACTIVE', true FROM academic.academic_years;
		INSERT INTO curriculum.courses (code, owner_department_id, name_tr, name_en, theory_hours, national_credit, ects, language)
			SELECT 'COM1001', id, 'Bilgisayar Programlama I', 'Computer Programming I', 4, 5, 6, 'EN' FROM org.departments;
		INSERT INTO offering.course_offerings (term_id, course_id, department_id)
			SELECT t.id, c.id, d.id FROM academic.terms t, curriculum.courses c, org.departments d;
		INSERT INTO offering.sections (offering_id, section_code, capacity, language)
			SELECT id, '1', 60, 'EN' FROM offering.course_offerings;`)
	if err != nil {
		t.Fatal(err)
	}
	var f fixture
	if err := pool.QueryRow(ctx, `SELECT d.id, s.id FROM org.departments d, offering.sections s`).Scan(&f.bil, &f.section); err != nil {
		t.Fatal(err)
	}
	staff := func(no, first, last string) (staffID, userID string) {
		t.Helper()
		err := pool.QueryRow(ctx, `
			WITH p AS (INSERT INTO people.persons (first_name, last_name) VALUES ($2, $3) RETURNING id),
			s AS (
				INSERT INTO people.staff (person_id, staff_no, staff_type, academic_title_code)
				SELECT id, $1::text, 'ACADEMIC', 'ASSOC_PROF' FROM p RETURNING id
			), u AS (
				INSERT INTO iam.users (person_id, username, email, password_hash)
				SELECT id, $1::text, $1::text || '@personel.test', 'x' FROM p RETURNING id
			)
			SELECT (SELECT id FROM s), (SELECT id FROM u)`, no, first, last).Scan(&staffID, &userID)
		if err != nil {
			t.Fatal(err)
		}
		return staffID, userID
	}
	ayse, ayseUser := staff("P1001", "Ayşe", "Yılmaz")
	_, f.mehmetUser = staff("P1002", "Mehmet", "Demir")
	f.ayseUser = ayseUser
	if _, err := pool.Exec(ctx, `INSERT INTO offering.section_instructors (section_id, staff_id, role) VALUES ($1, $2, 'PRIMARY')`, f.section, ayse); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestPlanLifecycle(t *testing.T) {
	pool := dbtest.New(t)
	fx := seed(t, pool)
	repo := grading.NewRepository(pool)
	ctx := context.Background()

	p, err := repo.Plan(ctx, fx.section, fx.ayseUser)
	if err != nil || len(p.Components) != 0 || p.InstructorRole != "PRIMARY" || p.CourseCode != "COM1001" {
		t.Fatalf("boş plan = %+v, %v", p, err)
	}
	if err := repo.Lock(ctx, "", fx.section); !errors.Is(err, grading.ErrPlanIncomplete) {
		t.Errorf("boş planı kilitleme: %v", err)
	}

	items := []grading.ComponentInput{
		{TypeCode: "MIDTERM", SequenceNo: 1, Weight: 20},
		{TypeCode: "MIDTERM", SequenceNo: 2, Weight: 20},
		{TypeCode: "HOMEWORK", SequenceNo: 1, Weight: 10},
		{TypeCode: "FINAL", SequenceNo: 1, Weight: 50},
	}
	if err := repo.SetPlan(ctx, "", fx.section, 1, items); err != nil {
		t.Fatal(err)
	}
	p, _ = repo.Plan(ctx, fx.section, "")
	if len(p.Components) != 5 || p.Version != 2 {
		t.Fatalf("plan = %d bileşen, sürüm %d", len(p.Components), p.Version)
	}
	last := p.Components[4]
	if last.Type.Code != "MAKEUP" || last.Weight != 50 || p.Components[3].Type.Code != "FINAL" {
		t.Errorf("bütünleme finalden türetilmeli: %+v", p.Components)
	}
	if label := p.Components[0].Label(false, true); label != "Ara sınav 1" {
		t.Errorf("etiket = %q", label)
	}
	midterm1 := p.Components[0].ID

	// Yeniden yazma: aynı tür ve sıra numaralı bileşenin kimliği korunur, eksik olan silinir.
	items = []grading.ComponentInput{
		{TypeCode: "MIDTERM", SequenceNo: 1, Weight: 30, NameTR: "Vize"},
		{TypeCode: "FINAL", SequenceNo: 1, Weight: 70},
	}
	if err := repo.SetPlan(ctx, "", fx.section, 1, items); !errors.Is(err, grading.ErrVersionMismatch) {
		t.Errorf("eski sürüm: %v", err)
	}
	if err := repo.SetPlan(ctx, "", fx.section, 2, items); err != nil {
		t.Fatal(err)
	}
	p, _ = repo.Plan(ctx, fx.section, "")
	if len(p.Components) != 3 || p.Components[0].ID != midterm1 || p.Components[0].Label(false, false) != "Vize" || p.Components[2].Weight != 70 {
		t.Errorf("güncel plan = %+v", p.Components)
	}

	// Kilit: plan donar; kilidi bölüm açar.
	if err := repo.Lock(ctx, fx.ayseUser, fx.section); err != nil {
		t.Fatal(err)
	}
	p, _ = repo.Plan(ctx, fx.section, "")
	if p.LockedAt == nil || p.LockedByName != "Ayşe Yılmaz" {
		t.Errorf("kilit = %v %q", p.LockedAt, p.LockedByName)
	}
	if err := repo.SetPlan(ctx, "", fx.section, p.Version, items); !errors.Is(err, grading.ErrPlanLocked) {
		t.Errorf("kilitli plan: %v", err)
	}
	if err := repo.Unlock(ctx, "", fx.section, "Ara sınav tarihi değişti"); err != nil {
		t.Fatal(err)
	}
	p, _ = repo.Plan(ctx, fx.section, "")
	if p.LockedAt != nil {
		t.Errorf("kilit açılmalı")
	}
}

// --- HTTP ----------------------------------------------------------------------

type grants map[string][]authz.Grant

func (g grants) Permissions(ctx context.Context, userID string) (*authz.Permissions, error) {
	return authz.NewPermissions(userID, g[userID]), nil
}

const (
	head    = "01a11b7f-0000-7000-8000-0000000000d1" // bölüm başkanı
	student = "01a11b7f-0000-7000-8000-0000000000d2"
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
	instructor := []authz.Grant{read, {Permission: "assessment_plan:manage", ScopeType: authz.ScopeDepartment, ScopeID: fx.bil}}
	resolver := grants{
		fx.ayseUser:   instructor,
		fx.mehmetUser: instructor,
		head:          {read, {Permission: "section:manage", ScopeType: authz.ScopeDepartment, ScopeID: fx.bil}},
		student:       {read},
	}
	mux := http.NewServeMux()
	grading.NewHandler(grading.NewRepository(pool), logger).Register(authz.NewRouter(mux, authenticate, resolver, logger))
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

func TestHTTPAssessmentPlan(t *testing.T) {
	s := newServer(t)
	path := "/api/v1/sections/" + s.fx.section + "/assessment-plan"
	plan := func(parts ...any) map[string]any { return map[string]any{"components": parts} }
	c := func(typ string, weight float64) map[string]any { return map[string]any{"type": typ, "weight": weight} }

	res := s.do("GET", path, s.fx.ayseUser, nil, nil)
	if res.status != 200 || res.body["editable"] != true || res.body["is_complete"] != false || res.header.Get("ETag") != `"1"` {
		t.Fatalf("boş plan: %d %s", res.status, res.raw)
	}
	if res := s.do("GET", path, student, nil, nil); res.body["editable"] != false {
		t.Errorf("öğrenci düzenleyemez: %s", res.raw)
	}
	if res := s.do("GET", "/api/v1/assessment-types", student, nil, nil); len(res.body["items"].([]any)) != 9 {
		t.Errorf("türler: %s", res.raw)
	}

	good := plan(c("MIDTERM", 30), c("QUIZ", 10), c("FINAL", 60))
	ifMatch := map[string]string{"If-Match": `"1"`}
	if res := s.do("PUT", path, s.fx.mehmetUser, ifMatch, good); res.status != 403 {
		t.Errorf("şubede olmayan öğretim elemanı: %d", res.status)
	}
	if res := s.do("PUT", path, student, ifMatch, good); res.status != 403 {
		t.Errorf("öğrenci: %d", res.status)
	}
	for name, bad := range map[string]map[string]any{
		"toplam 90":          plan(c("MIDTERM", 30), c("FINAL", 60)),
		"finalsiz":           plan(c("MIDTERM", 40), c("HOMEWORK", 60)),
		"iki final":          plan(c("FINAL", 50), c("FINAL", 50)),
		"bütünleme verilmiş": plan(c("MIDTERM", 40), c("FINAL", 60), c("MAKEUP", 60)),
		"üç ondalık":         plan(c("MIDTERM", 39.999), c("FINAL", 60.001)),
		"tanımsız tür":       plan(c("ESSAY", 40), c("FINAL", 60)),
	} {
		if res := s.do("PUT", path, s.fx.ayseUser, ifMatch, bad); res.status != 400 {
			t.Errorf("%s: %d %s", name, res.status, res.raw)
		}
	}
	if res := s.do("PUT", path, s.fx.ayseUser, nil, good); res.status != 428 {
		t.Errorf("If-Match olmadan: %d", res.status)
	}
	res = s.do("PUT", path, s.fx.ayseUser, ifMatch, good)
	if res.status != 200 || res.body["is_complete"] != true || res.body["in_term_weight"] != 40.0 || len(res.body["components"].([]any)) != 4 {
		t.Fatalf("plan: %d %s", res.status, res.raw)
	}
	makeup := res.body["components"].([]any)[3].(map[string]any)
	if makeup["type"].(map[string]any)["code"] != "MAKEUP" || makeup["weight"] != 60.0 || makeup["label_tr"] != "Bütünleme" {
		t.Errorf("bütünleme: %v", makeup)
	}
	if res := s.do("PUT", path, s.fx.ayseUser, ifMatch, good); res.status != 412 {
		t.Errorf("eski sürüm: %d", res.status)
	}

	// Kilit: öğretim elemanı kilitler, sadece bölüm açar.
	res = s.do("POST", path+"/lock", s.fx.ayseUser, nil, nil)
	if res.status != 200 || res.body["locked_at"] == nil || res.body["editable"] != false || res.body["can_unlock"] != false {
		t.Fatalf("kilit: %d %s", res.status, res.raw)
	}
	etag := res.header.Get("ETag")
	if res := s.do("PUT", path, s.fx.ayseUser, map[string]string{"If-Match": etag}, good); res.status != 409 || res.code() != "ASSESSMENT_PLAN_LOCKED" {
		t.Errorf("kilitli plan: %d %s", res.status, res.raw)
	}
	if res := s.do("POST", path+"/unlock", s.fx.ayseUser, nil, map[string]any{"reason": "Hata yaptım"}); res.status != 403 {
		t.Errorf("öğretim elemanı kilidi açamaz: %d", res.status)
	}
	if res := s.do("POST", path+"/unlock", head, nil, map[string]any{"reason": "x"}); res.status != 400 {
		t.Errorf("gerekçesiz: %d", res.status)
	}
	res = s.do("POST", path+"/unlock", head, nil, map[string]any{"reason": "Ara sınav tarihi değişti"})
	if res.status != 200 || res.body["locked_at"] != nil || res.body["editable"] != true {
		t.Errorf("kilit açma: %d %s", res.status, res.raw)
	}
}
