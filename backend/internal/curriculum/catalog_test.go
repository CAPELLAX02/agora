package curriculum_test

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

	"github.com/CAPELLAX02/agora/backend/internal/curriculum"
	"github.com/CAPELLAX02/agora/backend/internal/org"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
)

type fixture struct {
	bil, fiz         string // bilgisayar mühendisliği ve fizik bölümleri (farklı fakülteler)
	bilProg, fizProg string // bölümlerin 8 yarıyıllık lisans programları
}

func seedOrg(t *testing.T, pool *pgxpool.Pool) fixture {
	t.Helper()
	var f fixture
	err := pool.QueryRow(context.Background(), `
		WITH m AS (
			INSERT INTO org.faculties (code, name_tr, name_en, unit_type) VALUES ('MUH', 'Mühendislik Fakültesi', 'Faculty of Engineering', 'FACULTY') RETURNING id
		), s AS (
			INSERT INTO org.faculties (code, name_tr, name_en, unit_type) VALUES ('FEN', 'Fen Fakültesi', 'Faculty of Science', 'FACULTY') RETURNING id
		), b AS (
			INSERT INTO org.departments (faculty_id, code, name_tr, name_en) SELECT id, 'BIL', 'Bilgisayar Mühendisliği', 'Computer Engineering' FROM m RETURNING id
		), z AS (
			INSERT INTO org.departments (faculty_id, code, name_tr, name_en) SELECT id, 'FIZ', 'Fizik', 'Physics' FROM s RETURNING id
		), bp AS (
			INSERT INTO org.programs (department_id, code, name_tr, name_en, degree_level, language, education_type,
			                          duration_semesters, max_duration_years, total_ects_required, has_prep_class)
			SELECT id, 'BIL-EN', 'Bilgisayar Mühendisliği (İngilizce)', 'Computer Engineering', 'BACHELOR', 'EN', 'DAYTIME', 8, 7, 240, true FROM b
			RETURNING id
		), zp AS (
			INSERT INTO org.programs (department_id, code, name_tr, name_en, degree_level, language, education_type,
			                          duration_semesters, max_duration_years, total_ects_required, has_prep_class)
			SELECT id, 'FIZ-TR', 'Fizik', 'Physics', 'BACHELOR', 'TR', 'DAYTIME', 8, 7, 240, false FROM z
			RETURNING id
		)
		SELECT (SELECT id FROM b), (SELECT id FROM z), (SELECT id FROM bp), (SELECT id FROM zp)`).Scan(&f.bil, &f.fiz, &f.bilProg, &f.fizProg)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func course(code, dept string, ects float64) curriculum.CourseInput {
	return curriculum.CourseInput{
		Code: code, OwnerDepartmentID: dept, NameTR: code + " dersi", NameEN: code + " course",
		TheoryHours: 3, PracticeHours: 2, NationalCredit: 4, ECTS: ects, Language: "EN",
		Kind: curriculum.KindRegular, GradingMode: curriculum.GradingLetter, IsActive: true,
		LearningOutcomes: []string{"Algoritma tasarlar.", "", "Karmaşıklık analizi yapar."},
	}
}

func mustCreate(t *testing.T, repo *curriculum.Repository, in curriculum.CourseInput) string {
	t.Helper()
	id, err := repo.CreateCourse(context.Background(), "", in)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func auditCount(t *testing.T, pool *pgxpool.Pool, action, entityID string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit.audit_log WHERE action = $1 AND entity_id = $2`, action, entityID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCourses(t *testing.T) {
	pool := dbtest.New(t)
	fx := seedOrg(t, pool)
	repo := curriculum.NewRepository(pool)
	ctx := context.Background()

	algo := mustCreate(t, repo, course("COM2001", fx.bil, 6))
	mustCreate(t, repo, course("COM1001", fx.bil, 5))
	mustCreate(t, repo, course("PHY1001", fx.fiz, 5.5))
	mustCreate(t, repo, course("TDL1001", "", 2)) // üniversite ortak dersi
	if _, err := repo.CreateCourse(ctx, "", course("COM2001", fx.bil, 6)); !errors.Is(err, curriculum.ErrConflict) {
		t.Errorf("aynı kod: %v", err)
	}
	if _, err := repo.CreateCourse(ctx, "", course("COM9001", "01a11b7f-0000-7000-8000-00000000dead", 6)); !errors.Is(err, curriculum.ErrUnknownReference) {
		t.Errorf("olmayan bölüm: %v", err)
	}
	if n := auditCount(t, pool, "course.create", algo); n != 1 {
		t.Errorf("denetim kaydı = %d", n)
	}

	c, err := repo.Course(ctx, algo)
	if err != nil {
		t.Fatal(err)
	}
	if c.OwnerDepartment == nil || c.OwnerDepartment.Code != "BIL" || c.ECTS != 6 || len(c.LearningOutcomes) != 2 {
		t.Errorf("ders = %+v", c)
	}

	// Sayfalama koda göre; arama kodda ya da adda.
	page, more, err := repo.ListCourses(ctx, curriculum.CourseFilter{Limit: 3})
	if err != nil || !more || len(page) != 3 || page[0].Code != "COM1001" || page[2].Code != "PHY1001" {
		t.Fatalf("ilk sayfa = %v %v %v", codes(page), more, err)
	}
	page, more, _ = repo.ListCourses(ctx, curriculum.CourseFilter{Limit: 3, After: &curriculum.CourseCursor{Code: page[2].Code}})
	if more || len(page) != 1 || page[0].Code != "TDL1001" {
		t.Errorf("ikinci sayfa = %v %v", codes(page), more)
	}
	page, _, _ = repo.ListCourses(ctx, curriculum.CourseFilter{Limit: 10, Query: "phy1"})
	if len(page) != 1 || page[0].Code != "PHY1001" {
		t.Errorf("arama = %v", codes(page))
	}
	page, _, _ = repo.ListCourses(ctx, curriculum.CourseFilter{Limit: 10, DepartmentID: fx.bil})
	if len(page) != 2 {
		t.Errorf("bölüm süzgeci = %v", codes(page))
	}

	// Güncelleme: sürüm kontrolü; pasif ders varsayılan listede görünmez.
	in := course("COM2001", fx.bil, 6)
	in.IsActive = false
	if err := repo.UpdateCourse(ctx, "", algo, c.Version+1, in); !errors.Is(err, curriculum.ErrVersionMismatch) {
		t.Errorf("eski sürüm: %v", err)
	}
	if err := repo.UpdateCourse(ctx, "", algo, c.Version, in); err != nil {
		t.Fatal(err)
	}
	page, _, _ = repo.ListCourses(ctx, curriculum.CourseFilter{Limit: 10, DepartmentID: fx.bil})
	if len(page) != 1 {
		t.Errorf("pasif ders listede: %v", codes(page))
	}
	page, _, _ = repo.ListCourses(ctx, curriculum.CourseFilter{Limit: 10, DepartmentID: fx.bil, IncludeInactive: true})
	if len(page) != 2 {
		t.Errorf("pasifler dahil: %v", codes(page))
	}
}

func codes(cs []curriculum.Course) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.Code)
	}
	return out
}

func TestPrerequisites(t *testing.T) {
	pool := dbtest.New(t)
	fx := seedOrg(t, pool)
	repo := curriculum.NewRepository(pool)
	ctx := context.Background()

	prog1 := mustCreate(t, repo, course("COM1001", fx.bil, 6))
	prog2 := mustCreate(t, repo, course("COM1002", fx.bil, 6))
	data := mustCreate(t, repo, course("COM2044", fx.bil, 6))
	mth1 := mustCreate(t, repo, course("MTH1001", fx.bil, 6))
	mth2 := mustCreate(t, repo, course("MTH1002", fx.bil, 6))

	// COM2044: (COM1002) VE (MTH1001 VEYA MTH1002).
	set := []curriculum.PrerequisiteInput{
		{CourseID: prog2, Requirement: curriculum.RequirePassed, GroupNo: 1},
		{CourseID: mth1, Requirement: curriculum.RequireAttended, GroupNo: 2},
		{CourseID: mth2, Requirement: curriculum.RequireAttended, GroupNo: 2},
	}
	if err := repo.SetPrerequisites(ctx, "", data, set); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetPrerequisites(ctx, "", prog2, []curriculum.PrerequisiteInput{{CourseID: prog1, Requirement: curriculum.RequirePassed, GroupNo: 1}}); err != nil {
		t.Fatal(err)
	}

	d, err := repo.CourseDetail(ctx, data)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Prerequisites) != 3 || d.Prerequisites[0].Course.Code != "COM1002" || d.Prerequisites[1].GroupNo != 2 {
		t.Errorf("ön koşullar = %+v", d.Prerequisites)
	}
	d, _ = repo.CourseDetail(ctx, prog2)
	if len(d.RequiredBy) != 1 || d.RequiredBy[0].Code != "COM2044" {
		t.Errorf("bağımlı dersler = %+v", d.RequiredBy)
	}

	// COM1001 → COM2044 dolaylı döngü: COM2044 ← COM1002 ← COM1001.
	err = repo.SetPrerequisites(ctx, "", prog1, []curriculum.PrerequisiteInput{{CourseID: data, Requirement: curriculum.RequirePassed, GroupNo: 1}})
	if !errors.Is(err, curriculum.ErrPrerequisiteCycle) {
		t.Errorf("dolaylı döngü: %v", err)
	}
	d, _ = repo.CourseDetail(ctx, prog1)
	if len(d.Prerequisites) != 0 {
		t.Errorf("başarısız değişiklik geri alınmalı: %+v", d.Prerequisites)
	}
	err = repo.SetPrerequisites(ctx, "", prog1, []curriculum.PrerequisiteInput{{CourseID: "01a11b7f-0000-7000-8000-00000000dead", Requirement: curriculum.RequirePassed, GroupNo: 1}})
	if !errors.Is(err, curriculum.ErrUnknownReference) {
		t.Errorf("olmayan ders: %v", err)
	}

	// Boş küme ön koşulları kaldırır.
	if err := repo.SetPrerequisites(ctx, "", data, nil); err != nil {
		t.Fatal(err)
	}
	d, _ = repo.CourseDetail(ctx, data)
	if len(d.Prerequisites) != 0 {
		t.Errorf("kaldırılan ön koşullar = %+v", d.Prerequisites)
	}
	if n := auditCount(t, pool, "course.set_prerequisites", data); n != 2 {
		t.Errorf("denetim kaydı = %d", n)
	}
}

func TestEquivalences(t *testing.T) {
	pool := dbtest.New(t)
	fx := seedOrg(t, pool)
	repo := curriculum.NewRepository(pool)
	ctx := context.Background()

	oldID := mustCreate(t, repo, course("COM4519", fx.bil, 6))
	newID := mustCreate(t, repo, course("COM4569", fx.bil, 4))
	other := mustCreate(t, repo, course("COM1001", fx.bil, 6))

	year := 2026
	eqID, err := repo.AddEquivalence(ctx, "", newID, curriculum.EquivalenceInput{EquivalentCourseID: oldID, IsBidirectional: true, ValidFromYear: &year, Note: "AKTS 6 → 4"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.AddEquivalence(ctx, "", oldID, curriculum.EquivalenceInput{EquivalentCourseID: newID}); !errors.Is(err, curriculum.ErrDuplicate) {
		t.Errorf("ters yönde aynı çift: %v", err)
	}

	d, _ := repo.CourseDetail(ctx, newID)
	if len(d.Equivalences) != 1 || d.Equivalences[0].Relation != curriculum.Replaces || d.Equivalences[0].Course.Code != "COM4519" ||
		d.Equivalences[0].ValidFromYear == nil || *d.Equivalences[0].ValidFromYear != 2026 {
		t.Errorf("yeni ders eşdeğerlikleri = %+v", d.Equivalences)
	}
	d, _ = repo.CourseDetail(ctx, oldID)
	if len(d.Equivalences) != 1 || d.Equivalences[0].Relation != curriculum.ReplacedBy || d.Equivalences[0].Course.Code != "COM4569" {
		t.Errorf("eski ders eşdeğerlikleri = %+v", d.Equivalences)
	}

	if err := repo.DeleteEquivalence(ctx, "", other, eqID); !errors.Is(err, curriculum.ErrNotFound) {
		t.Errorf("başka dersin eşdeğerliği: %v", err)
	}
	if err := repo.DeleteEquivalence(ctx, "", oldID, eqID); err != nil {
		t.Fatal(err)
	}
	d, _ = repo.CourseDetail(ctx, newID)
	if len(d.Equivalences) != 0 {
		t.Errorf("silinen eşdeğerlik = %+v", d.Equivalences)
	}
}

func TestElectiveGroups(t *testing.T) {
	pool := dbtest.New(t)
	fx := seedOrg(t, pool)
	repo := curriculum.NewRepository(pool)
	ctx := context.Background()

	c1 := mustCreate(t, repo, course("COM3001", fx.bil, 6))
	c2 := mustCreate(t, repo, course("COM3002", fx.bil, 6))

	groupIn := curriculum.GroupInput{Code: "COMTE03", NameTR: "3. sınıf teknik seçmeliler", NameEN: "3rd year technical electives", OwnerDepartmentID: fx.bil, Kind: curriculum.GroupTechnical, IsActive: true}
	gID, err := repo.CreateGroup(ctx, "", groupIn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateGroup(ctx, "", groupIn); !errors.Is(err, curriculum.ErrConflict) {
		t.Errorf("aynı kod: %v", err)
	}
	for _, c := range []string{c1, c2, c1} { // ikinci ekleme sessizce geçer
		if err := repo.AddGroupCourse(ctx, "", gID, c); err != nil {
			t.Fatal(err)
		}
	}
	if n := auditCount(t, pool, "elective_group.add_course", gID); n != 2 {
		t.Errorf("tekrar eklemede denetim kaydı yazılmamalı: %d", n)
	}
	if err := repo.AddGroupCourse(ctx, "", gID, "01a11b7f-0000-7000-8000-00000000dead"); !errors.Is(err, curriculum.ErrUnknownReference) {
		t.Errorf("olmayan ders: %v", err)
	}

	g, err := repo.Group(ctx, gID)
	if err != nil || g.CourseCount != 2 || g.OwnerDepartment.Code != "BIL" {
		t.Fatalf("grup = %+v, %v", g, err)
	}
	d, _ := repo.CourseDetail(ctx, c1)
	if len(d.ElectiveGroups) != 1 || d.ElectiveGroups[0].Code != "COMTE03" {
		t.Errorf("dersin grupları = %+v", d.ElectiveGroups)
	}

	if err := repo.RemoveGroupCourse(ctx, "", gID, c1); err != nil {
		t.Fatal(err)
	}
	if err := repo.RemoveGroupCourse(ctx, "", gID, c1); !errors.Is(err, curriculum.ErrNotFound) {
		t.Errorf("havuzda olmayan ders: %v", err)
	}
	courses, _ := repo.GroupCourses(ctx, gID)
	if len(courses) != 1 || courses[0].Code != "COM3002" {
		t.Errorf("grup dersleri = %v", codes(courses))
	}

	groupIn.NameEN = "Technical electives (3rd year)"
	if err := repo.UpdateGroup(ctx, "", gID, g.Version, groupIn); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateGroup(ctx, "", gID, g.Version, groupIn); !errors.Is(err, curriculum.ErrVersionMismatch) {
		t.Errorf("eski sürüm: %v", err)
	}
	groups, _ := repo.ListGroups(ctx, curriculum.GroupFilter{Kind: curriculum.GroupTechnical})
	if len(groups) != 1 || groups[0].NameEN != "Technical electives (3rd year)" {
		t.Errorf("gruplar = %+v", groups)
	}
}

// --- HTTP ----------------------------------------------------------------------

type grants map[string][]authz.Grant

func (g grants) Permissions(ctx context.Context, userID string) (*authz.Permissions, error) {
	return authz.NewPermissions(userID, g[userID]), nil
}

const (
	registrar = "01a11b7f-0000-7000-8000-0000000000b1" // merkezi öğrenci işleri: bütün katalog
	deptMg    = "01a11b7f-0000-7000-8000-0000000000b2" // sadece bilgisayar mühendisliği bölümünün dersleri ve grupları
	student   = "01a11b7f-0000-7000-8000-0000000000b3" // sadece okuma
)

type server struct {
	t      *testing.T
	h      http.Handler
	fx     fixture
	pool   *pgxpool.Pool
	repo   *curriculum.Repository
	grants grants
}

func newServer(t *testing.T) *server {
	t.Helper()
	pool := dbtest.New(t)
	fx := seedOrg(t, pool)
	s := &server{t: t, fx: fx, pool: pool, repo: curriculum.NewRepository(pool)}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	authenticate := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := r.Header.Get("X-Test-User")
			if user == "" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r.WithContext(authn.WithPrincipal(r.Context(), authn.Principal{UserID: user})))
		})
	}
	read := []authz.Grant{
		{Permission: "course:read", ScopeType: authz.ScopeNone},
		{Permission: "curriculum:read", ScopeType: authz.ScopeNone},
	}
	resolver := grants{
		registrar: append([]authz.Grant{
			{Permission: "course:manage", ScopeType: authz.ScopeUniversity},
			{Permission: "curriculum:manage", ScopeType: authz.ScopeUniversity},
		}, read...),
		deptMg: append([]authz.Grant{
			{Permission: "course:manage", ScopeType: authz.ScopeDepartment, ScopeID: fx.bil},
			{Permission: "curriculum:manage", ScopeType: authz.ScopeDepartment, ScopeID: fx.bil},
		}, read...),
		student: read,
	}
	s.grants = resolver
	mux := http.NewServeMux()
	curriculum.NewHandler(s.repo, org.NewTargets(pool), logger).
		Register(authz.NewRouter(mux, authenticate, resolver, logger))
	s.h = mux
	return s
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

func courseBody(code, dept string) map[string]any {
	return map[string]any{
		"code": code, "owner_department_id": dept, "name_tr": code + " dersi", "name_en": code + " course",
		"theory_hours": 3, "practice_hours": 0, "national_credit": 3, "ects": 5, "language": "EN",
	}
}

func TestHTTPCatalog(t *testing.T) {
	s := newServer(t)

	// Ders oluşturma: kapsam yetkisi.
	if res := s.do("POST", "/api/v1/courses", student, nil, courseBody("COM1001", s.fx.bil)); res.status != 403 {
		t.Errorf("öğrenci ders açamaz: %d", res.status)
	}
	if res := s.do("POST", "/api/v1/courses", deptMg, nil, courseBody("TDL1001", "")); res.status != 403 {
		t.Errorf("bölüm yöneticisi üniversite ortak dersi açamaz: %d %s", res.status, res.raw)
	}
	if res := s.do("POST", "/api/v1/courses", deptMg, nil, courseBody("PHY1001", s.fx.fiz)); res.status != 403 {
		t.Errorf("bölüm yöneticisi başka bölüme ders açamaz: %d", res.status)
	}
	bad := courseBody("com1", s.fx.bil)
	bad["ects"], bad["language"], bad["theory_hours"] = 5.25, "DE", 50
	if res := s.do("POST", "/api/v1/courses", registrar, nil, bad); res.status != 400 || len(res.body["errors"].([]any)) != 4 {
		t.Errorf("geçersiz ders: %d %s", res.status, res.raw)
	}

	res := s.do("POST", "/api/v1/courses", deptMg, nil, courseBody("com1001", s.fx.bil))
	if res.status != 201 || res.body["code"] != "COM1001" || res.body["kind"] != "REGULAR" || res.header.Get("ETag") != `"1"` {
		t.Fatalf("ders: %d %s", res.status, res.raw)
	}
	prog1 := res.body["id"].(string)
	if res := s.do("POST", "/api/v1/courses", registrar, nil, courseBody("COM1001", s.fx.bil)); res.status != 409 || res.code() != "COURSE_CODE_TAKEN" {
		t.Errorf("aynı kod: %d %s", res.status, res.raw)
	}
	prog2 := s.do("POST", "/api/v1/courses", deptMg, nil, courseBody("COM1002", s.fx.bil)).body["id"].(string)
	tdl := s.do("POST", "/api/v1/courses", registrar, nil, courseBody("TDL1001", "")).body["id"].(string)

	// Liste: sayfalama ve arama.
	res = s.do("GET", "/api/v1/courses?limit=2", student, nil, nil)
	if res.status != 200 || len(res.items()) != 2 || res.body["next_cursor"] == nil {
		t.Fatalf("liste: %d %s", res.status, res.raw)
	}
	res = s.do("GET", "/api/v1/courses?limit=2&cursor="+res.body["next_cursor"].(string), student, nil, nil)
	if len(res.items()) != 1 || res.items()[0].(map[string]any)["code"] != "TDL1001" || res.body["next_cursor"] != nil {
		t.Errorf("ikinci sayfa: %s", res.raw)
	}
	if res := s.do("GET", "/api/v1/courses?cursor=bozuk", student, nil, nil); res.status != 400 {
		t.Errorf("bozuk cursor: %d", res.status)
	}

	// Güncelleme: If-Match, kod değişmez, ortak ders sadece üniversite yetkisiyle.
	upd := courseBody("COM1001", s.fx.bil)
	upd["ects"] = 6
	if res := s.do("PUT", "/api/v1/courses/"+prog1, deptMg, nil, upd); res.status != 428 {
		t.Errorf("If-Match olmadan: %d", res.status)
	}
	if res := s.do("PUT", "/api/v1/courses/"+prog1, deptMg, map[string]string{"If-Match": `"7"`}, upd); res.status != 412 {
		t.Errorf("eski sürüm: %d", res.status)
	}
	upd["code"] = "COM1009"
	if res := s.do("PUT", "/api/v1/courses/"+prog1, deptMg, map[string]string{"If-Match": `"1"`}, upd); res.status != 400 {
		t.Errorf("kod değişikliği: %d", res.status)
	}
	upd["code"] = ""
	res = s.do("PUT", "/api/v1/courses/"+prog1, deptMg, map[string]string{"If-Match": `"1"`}, upd)
	if res.status != 200 || res.body["ects"] != 6.0 || res.header.Get("ETag") != `"2"` {
		t.Errorf("güncelleme: %d %s", res.status, res.raw)
	}
	upd["owner_department_id"] = s.fx.fiz
	if res := s.do("PUT", "/api/v1/courses/"+prog1, deptMg, map[string]string{"If-Match": `"2"`}, upd); res.status != 403 {
		t.Errorf("başka bölüme devir: %d", res.status)
	}
	if res := s.do("PUT", "/api/v1/courses/"+tdl, deptMg, map[string]string{"If-Match": `"1"`}, courseBody("TDL1001", "")); res.status != 403 {
		t.Errorf("bölüm yöneticisi ortak dersi değiştiremez: %d", res.status)
	}

	// Ön koşullar ve döngü.
	pre := func(ids ...string) map[string]any {
		items := []map[string]any{}
		for _, id := range ids {
			items = append(items, map[string]any{"course_id": id})
		}
		return map[string]any{"items": items}
	}
	res = s.do("PUT", "/api/v1/courses/"+prog2+"/prerequisites", deptMg, nil, pre(prog1))
	if res.status != 200 || len(res.body["prerequisites"].([]any)) != 1 {
		t.Fatalf("ön koşul: %d %s", res.status, res.raw)
	}
	if p := res.body["prerequisites"].([]any)[0].(map[string]any); p["requirement"] != "PASSED" || p["group_no"] != 1.0 {
		t.Errorf("varsayılanlar: %v", p)
	}
	if res := s.do("PUT", "/api/v1/courses/"+prog1+"/prerequisites", deptMg, nil, pre(prog2)); res.status != 409 || res.code() != "PREREQUISITE_CYCLE" {
		t.Errorf("döngü: %d %s", res.status, res.raw)
	}
	if res := s.do("PUT", "/api/v1/courses/"+prog1+"/prerequisites", deptMg, nil, pre(prog1)); res.status != 400 {
		t.Errorf("kendisi: %d", res.status)
	}
	if res := s.do("PUT", "/api/v1/courses/"+prog1+"/prerequisites", deptMg, nil, map[string]any{}); res.status != 400 {
		t.Errorf("items zorunlu: %d", res.status)
	}
	if res := s.do("GET", "/api/v1/courses/"+prog1, student, nil, nil); len(res.body["required_by"].([]any)) != 1 {
		t.Errorf("bağımlı dersler: %s", res.raw)
	}

	// Eşdeğerlik.
	res = s.do("POST", "/api/v1/courses/"+prog2+"/equivalences", deptMg, nil, map[string]any{"equivalent_course_id": tdl, "valid_from_year": 2026})
	if res.status != 201 || len(res.body["equivalences"].([]any)) != 1 {
		t.Fatalf("eşdeğerlik: %d %s", res.status, res.raw)
	}
	eq := res.body["equivalences"].([]any)[0].(map[string]any)
	if eq["relation"] != "REPLACES" || eq["is_bidirectional"] != true {
		t.Errorf("eşdeğerlik: %v", eq)
	}
	if res := s.do("POST", "/api/v1/courses/"+tdl+"/equivalences", registrar, nil, map[string]any{"equivalent_course_id": prog2}); res.status != 409 || res.code() != "EQUIVALENCE_EXISTS" {
		t.Errorf("aynı çift: %d %s", res.status, res.raw)
	}
	if res := s.do("DELETE", "/api/v1/courses/"+prog2+"/equivalences/"+eq["id"].(string), deptMg, nil, nil); res.status != 204 {
		t.Errorf("eşdeğerlik silme: %d %s", res.status, res.raw)
	}

	// Seçmeli grup ve havuz üyeliği.
	groupBody := map[string]any{"code": "COMTE02", "name_tr": "2. sınıf teknik seçmeliler", "name_en": "2nd year technical electives", "owner_department_id": s.fx.bil, "kind": "TECHNICAL"}
	if res := s.do("POST", "/api/v1/elective-groups", student, nil, groupBody); res.status != 403 {
		t.Errorf("öğrenci grup açamaz: %d", res.status)
	}
	res = s.do("POST", "/api/v1/elective-groups", deptMg, nil, groupBody)
	if res.status != 201 || res.body["course_count"] != 0.0 {
		t.Fatalf("grup: %d %s", res.status, res.raw)
	}
	groupID := res.body["id"].(string)
	if res := s.do("POST", "/api/v1/elective-groups", registrar, nil, groupBody); res.status != 409 || res.code() != "GROUP_CODE_TAKEN" {
		t.Errorf("aynı grup kodu: %d", res.status)
	}
	for range 2 {
		if res := s.do("PUT", "/api/v1/elective-groups/"+groupID+"/courses/"+prog2, deptMg, nil, nil); res.status != 204 {
			t.Errorf("havuza ekleme: %d %s", res.status, res.raw)
		}
	}
	if res := s.do("PUT", "/api/v1/elective-groups/"+groupID+"/courses/01a11b7f-0000-7000-8000-00000000dead", deptMg, nil, nil); res.status != 404 {
		t.Errorf("olmayan ders: %d", res.status)
	}
	res = s.do("GET", "/api/v1/elective-groups/"+groupID, student, nil, nil)
	if res.status != 200 || len(res.body["courses"].([]any)) != 1 || res.body["course_count"] != 1.0 {
		t.Errorf("grup detayı: %d %s", res.status, res.raw)
	}
	if res := s.do("GET", "/api/v1/elective-groups?kind=TECHNICAL", student, nil, nil); len(res.items()) != 1 {
		t.Errorf("grup listesi: %s", res.raw)
	}
	if res := s.do("DELETE", "/api/v1/elective-groups/"+groupID+"/courses/"+prog2, deptMg, nil, nil); res.status != 204 {
		t.Errorf("havuzdan çıkarma: %d", res.status)
	}
	if res := s.do("DELETE", "/api/v1/elective-groups/"+groupID+"/courses/"+prog2, deptMg, nil, nil); res.status != 404 {
		t.Errorf("havuzda olmayan: %d", res.status)
	}
	groupBody["code"], groupBody["is_active"] = "", false
	res = s.do("PUT", "/api/v1/elective-groups/"+groupID, deptMg, map[string]string{"If-Match": `"1"`}, groupBody)
	if res.status != 200 || res.body["is_active"] != false {
		t.Errorf("grup güncelleme: %d %s", res.status, res.raw)
	}
}
