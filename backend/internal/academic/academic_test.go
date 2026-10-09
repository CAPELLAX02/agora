package academic_test

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
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/academic"
	"github.com/CAPELLAX02/agora/backend/internal/org"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
)

type fixture struct {
	muh, fen, program string // mühendislik ve fen fakülteleri, mühendislikte bir program
}

func seedOrg(t *testing.T, pool *pgxpool.Pool) fixture {
	t.Helper()
	var f fixture
	err := pool.QueryRow(context.Background(), `
		WITH m AS (
			INSERT INTO org.faculties (code, name_tr, name_en, unit_type) VALUES ('MUH', 'Mühendislik Fakültesi', 'Faculty of Engineering', 'FACULTY') RETURNING id
		), s AS (
			INSERT INTO org.faculties (code, name_tr, name_en, unit_type) VALUES ('FEN', 'Fen Fakültesi', 'Faculty of Science', 'FACULTY') RETURNING id
		), d AS (
			INSERT INTO org.departments (faculty_id, code, name_tr, name_en) SELECT id, 'BIL', 'Bilgisayar Mühendisliği', 'Computer Engineering' FROM m RETURNING id
		), p AS (
			INSERT INTO org.programs (department_id, code, name_tr, name_en, degree_level, language, education_type,
			                          duration_semesters, max_duration_years, total_ects_required, has_prep_class)
			SELECT id, 'BIL-EN-NO', 'Bilgisayar Mühendisliği (İngilizce)', 'Computer Engineering', 'BACHELOR', 'EN', 'DAYTIME', 8, 7, 240, true FROM d
			RETURNING id
		)
		SELECT (SELECT id FROM m), (SELECT id FROM s), (SELECT id FROM p)`).Scan(&f.muh, &f.fen, &f.program)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func day(s string) time.Time {
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return d
}

// istanbul, gün sınırlarının Türkiye saatine göre olduğu bir zamandır (UTC+3).
func istanbul(s string) time.Time {
	t, err := time.Parse("2006-01-02 15:04", s)
	if err != nil {
		panic(err)
	}
	return t.Add(-3 * time.Hour).UTC()
}

// newCalendar, 2026-2027 akademik yılını ve güz dönemini kurar.
func newCalendar(t *testing.T, repo *academic.Repository) (yearID, fallID string) {
	t.Helper()
	ctx := context.Background()
	yearID, err := repo.CreateAcademicYear(ctx, "", academic.AcademicYear{StartYear: 2026, StartsOn: day("2026-09-01"), EndsOn: day("2027-08-31")})
	if err != nil {
		t.Fatal(err)
	}
	fallID, err = repo.CreateTerm(ctx, "", yearID, academic.TermInput{Type: academic.TermFall, StartsOn: day("2026-09-21"), EndsOn: day("2027-01-31")})
	if err != nil {
		t.Fatal(err)
	}
	return yearID, fallID
}

func TestTerms(t *testing.T) {
	pool := dbtest.New(t)
	repo := academic.NewRepository(pool)
	ctx := context.Background()
	yearID, fallID := newCalendar(t, repo)

	if _, err := repo.CreateAcademicYear(ctx, "", academic.AcademicYear{StartYear: 2026, StartsOn: day("2026-09-01"), EndsOn: day("2027-08-31")}); !errors.Is(err, academic.ErrConflict) {
		t.Errorf("aynı yıl: %v", err)
	}
	if _, err := repo.CreateTerm(ctx, "", yearID, academic.TermInput{Type: academic.TermFall, StartsOn: day("2026-09-21"), EndsOn: day("2027-01-31")}); !errors.Is(err, academic.ErrConflict) {
		t.Errorf("aynı dönem: %v", err)
	}
	if _, err := repo.CreateTerm(ctx, "", yearID, academic.TermInput{Type: academic.TermSummer, StartsOn: day("2027-07-01"), EndsOn: day("2027-09-15")}); !errors.Is(err, academic.ErrOutsideYear) {
		t.Errorf("yıl dışına taşan dönem: %v", err)
	}
	springID, err := repo.CreateTerm(ctx, "", yearID, academic.TermInput{Type: academic.TermSpring, StartsOn: day("2027-02-15"), EndsOn: day("2027-06-30")})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := repo.CurrentTerm(ctx); !errors.Is(err, academic.ErrNoCurrentTerm) {
		t.Errorf("aktif dönem yokken: %v", err)
	}
	if err := repo.MakeCurrent(ctx, "", fallID); err != nil {
		t.Fatal(err)
	}
	if err := repo.MakeCurrent(ctx, "", springID); err != nil {
		t.Fatal(err)
	}
	cur, err := repo.CurrentTerm(ctx)
	if err != nil || cur.Code != "2026-SPRING" || cur.Status != academic.TermActive {
		t.Fatalf("aktif dönem = %+v, %v", cur, err)
	}
	fall, _ := repo.Term(ctx, fallID)
	if fall.IsCurrent || fall.Code != "2026-FALL" {
		t.Errorf("tek aktif dönem olmalı: %+v", fall)
	}

	// Güz dönemi kapanır: kapanmış dönem aktif yapılamaz. Aktif dönem kapanınca aktiflikten çıkar.
	if err := repo.UpdateTerm(ctx, "", fallID, fall.Version, academic.TermInput{StartsOn: fall.StartsOn, EndsOn: fall.EndsOn, Status: academic.TermClosed}); err != nil {
		t.Fatal(err)
	}
	if err := repo.MakeCurrent(ctx, "", fallID); !errors.Is(err, academic.ErrTermClosed) {
		t.Errorf("kapanmış dönem: %v", err)
	}
	if err := repo.UpdateTerm(ctx, "", fallID, fall.Version, academic.TermInput{StartsOn: fall.StartsOn, EndsOn: fall.EndsOn, Status: academic.TermPlanned}); !errors.Is(err, academic.ErrVersionMismatch) {
		t.Errorf("eski sürümle güncelleme: %v", err)
	}
	spring, _ := repo.Term(ctx, springID)
	if err := repo.UpdateTerm(ctx, "", springID, spring.Version, academic.TermInput{StartsOn: spring.StartsOn, EndsOn: spring.EndsOn, Status: academic.TermClosed}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CurrentTerm(ctx); !errors.Is(err, academic.ErrNoCurrentTerm) {
		t.Errorf("kapanan aktif dönem aktiflikten çıkmalı: %v", err)
	}
}

func TestEventsAndWindows(t *testing.T) {
	pool := dbtest.New(t)
	repo := academic.NewRepository(pool)
	ctx := context.Background()
	fx := seedOrg(t, pool)
	_, fallID := newCalendar(t, repo)

	reg := func(scope academic.ScopeType, scopeID, from, to string) (string, error) {
		return repo.CreateEvent(ctx, "", fallID, academic.EventInput{
			TypeCode: "COURSE_REGISTRATION", StartsAt: istanbul(from), EndsAt: istanbul(to),
			ScopeType: scope, ScopeID: scopeID, IsPublished: true,
		})
	}
	if _, err := reg(academic.ScopeUniversity, "", "2026-09-22 00:00", "2026-09-27 00:00"); err != nil {
		t.Fatal(err)
	}
	if _, err := reg(academic.ScopeUniversity, "", "2026-09-26 00:00", "2026-09-28 00:00"); !errors.Is(err, academic.ErrEventOverlap) {
		t.Errorf("aynı kapsamda çakışma: %v", err)
	}
	// Bitişik aralıklar çakışmaz: [22, 27) ile [27, 28).
	if _, err := reg(academic.ScopeUniversity, "", "2026-09-27 00:00", "2026-09-28 00:00"); err != nil {
		t.Errorf("bitişik aralık: %v", err)
	}
	// Mühendislik fakültesi kendi penceresini tanımlar: üniversitenin penceresini geçersiz kılar.
	if _, err := reg(academic.ScopeFaculty, fx.muh, "2026-09-29 00:00", "2026-10-03 00:00"); err != nil {
		t.Fatal(err)
	}
	if _, err := reg(academic.ScopeProgram, "01a11b7f-0000-7000-8000-000000000000", "2026-09-29 00:00", "2026-10-03 00:00"); !errors.Is(err, academic.ErrUnknownScope) {
		t.Errorf("olmayan program: %v", err)
	}
	if _, err := repo.CreateEvent(ctx, "", fallID, academic.EventInput{
		TypeCode: "NO_SUCH_TYPE", StartsAt: istanbul("2026-10-01 00:00"), EndsAt: istanbul("2026-10-02 00:00"), ScopeType: academic.ScopeUniversity,
	}); !errors.Is(err, academic.ErrUnknownEventType) {
		t.Errorf("olmayan tür: %v", err)
	}
	// Yayımlanmamış (taslak) olay pencereyi etkilemez.
	if _, err := repo.CreateEvent(ctx, "", fallID, academic.EventInput{
		TypeCode: "ADVISOR_APPROVAL", StartsAt: istanbul("2026-09-22 00:00"), EndsAt: istanbul("2026-10-10 00:00"),
		ScopeType: academic.ScopeUniversity, IsPublished: false,
	}); err != nil {
		t.Fatal(err)
	}

	at := istanbul("2026-09-24 10:00")
	window := func(target academic.WindowTarget, typ string) academic.Window {
		t.Helper()
		w, err := repo.Window(ctx, fallID, typ, target, at)
		if err != nil {
			t.Fatal(err)
		}
		return w
	}

	fen := window(academic.WindowTarget{FacultyID: fx.fen}, "COURSE_REGISTRATION")
	if !fen.Open || fen.Scope != academic.ScopeUniversity || len(fen.Events) != 2 {
		t.Errorf("fen fakültesi üniversite penceresini kullanır: %+v", fen)
	}
	muh := window(academic.WindowTarget{FacultyID: fx.muh, ProgramID: fx.program}, "COURSE_REGISTRATION")
	if muh.Open || muh.Scope != academic.ScopeFaculty || muh.Next == nil || !muh.Next.StartsAt.Equal(istanbul("2026-09-29 00:00")) {
		t.Errorf("mühendislik kendi penceresini kullanır ve henüz açılmadı: %+v", muh)
	}
	if adv := window(academic.WindowTarget{}, "ADVISOR_APPROVAL"); adv.Open || len(adv.Events) != 0 {
		t.Errorf("taslak olay pencere açmamalı: %+v", adv)
	}

	// Program kendi penceresini tanımlarsa fakülteyi de geçersiz kılar.
	if _, err := reg(academic.ScopeProgram, fx.program, "2026-09-23 00:00", "2026-09-25 00:00"); err != nil {
		t.Fatal(err)
	}
	prog := window(academic.WindowTarget{FacultyID: fx.muh, ProgramID: fx.program}, "COURSE_REGISTRATION")
	if !prog.Open || prog.Scope != academic.ScopeProgram || prog.Current == nil {
		t.Errorf("program penceresi: %+v", prog)
	}

	// Hedefe göre olay listesi: fen fakültesi mühendisliğin ve programın olaylarını görmez.
	events, err := repo.Events(ctx, academic.EventFilter{TermID: fallID, Target: &academic.WindowTarget{FacultyID: fx.fen}})
	if err != nil || len(events) != 2 {
		t.Errorf("fen için olaylar = %d, %v", len(events), err)
	}
	all, _ := repo.Events(ctx, academic.EventFilter{TermID: fallID, IncludeUnpublished: true})
	if len(all) != 5 {
		t.Errorf("bütün olaylar = %d, want 5", len(all))
	}
}

// --- HTTP ----------------------------------------------------------------------

type grants map[string][]authz.Grant

func (g grants) Permissions(ctx context.Context, userID string) (*authz.Permissions, error) {
	return authz.NewPermissions(userID, g[userID]), nil
}

const (
	registrar = "01a11b7f-0000-7000-8000-0000000000a1" // merkezi öğrenci işleri: üniversite genelinde takvim
	facultyMg = "01a11b7f-0000-7000-8000-0000000000a2" // sadece mühendislik fakültesinin takvimi
	student   = "01a11b7f-0000-7000-8000-0000000000a3" // sadece okuma
)

type server struct {
	t    *testing.T
	h    http.Handler
	fx   fixture
	repo *academic.Repository
	now  time.Time
}

func newServer(t *testing.T) *server {
	t.Helper()
	pool := dbtest.New(t)
	fx := seedOrg(t, pool)
	s := &server{t: t, fx: fx, repo: academic.NewRepository(pool), now: istanbul("2026-09-24 10:00")}

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
	read := authz.Grant{Permission: "calendar:read", ScopeType: authz.ScopeUniversity}
	resolver := grants{
		registrar: {read, {Permission: "calendar:manage", ScopeType: authz.ScopeUniversity}},
		facultyMg: {read, {Permission: "calendar:manage", ScopeType: authz.ScopeFaculty, ScopeID: fx.muh}},
		student:   {{Permission: "calendar:read", ScopeType: authz.ScopeNone}},
	}
	mux := http.NewServeMux()
	academic.NewHandler(s.repo, org.NewTargets(pool), logger, func() time.Time { return s.now }).
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

func TestHTTPCalendar(t *testing.T) {
	s := newServer(t)

	if res := s.do("GET", "/api/v1/calendar/windows", student, nil, nil); res.status != 404 || res.code() != "NO_CURRENT_TERM" {
		t.Errorf("aktif dönem yokken pencereler: %d %s", res.status, res.raw)
	}

	// Yıl ve dönem: sadece takvim yöneticisi.
	yearBody := map[string]any{"start_year": 2026, "starts_on": "2026-09-01", "ends_on": "2027-08-31"}
	// Yıllar ve dönemler üniversite genelidir: fakülte kapsamlı takvim yetkisi yetmez.
	if res := s.do("POST", "/api/v1/academic-years", facultyMg, nil, yearBody); res.status != 403 {
		t.Errorf("fakülte yöneticisi yıl oluşturamaz: %d", res.status)
	}
	if res := s.do("POST", "/api/v1/academic-years", registrar, nil, yearBody); res.status != 201 || res.body["label"] != "2026-2027" {
		t.Fatalf("yıl: %d %s", res.status, res.raw)
	}
	if res := s.do("POST", "/api/v1/academic-years", registrar, nil, yearBody); res.status != 409 || res.code() != "ACADEMIC_YEAR_EXISTS" {
		t.Errorf("aynı yıl: %d %s", res.status, res.raw)
	}
	res := s.do("GET", "/api/v1/academic-years", student, nil, nil)
	years, _ := res.body["items"].([]any)
	if res.status != 200 || len(years) != 1 {
		t.Fatalf("yıllar: %d %s", res.status, res.raw)
	}
	yearID := years[0].(map[string]any)["id"].(string)
	if res := s.do("POST", "/api/v1/academic-years", student, nil, yearBody); res.status != 403 {
		t.Errorf("öğrenci yıl oluşturamaz: %d", res.status)
	}

	res = s.do("POST", "/api/v1/academic-years/"+yearID+"/terms", registrar, nil,
		map[string]any{"term_type": "FALL", "starts_on": "2026-09-21", "ends_on": "2027-01-31"})
	if res.status != 201 || res.body["code"] != "2026-FALL" || res.header.Get("ETag") != `"1"` {
		t.Fatalf("dönem: %d %s", res.status, res.raw)
	}
	termID := res.body["id"].(string)
	if res := s.do("POST", "/api/v1/academic-years/"+yearID+"/terms", registrar, nil,
		map[string]any{"term_type": "WINTER", "starts_on": "2026-09-21", "ends_on": "2026-09-01"}); res.status != 400 {
		t.Errorf("geçersiz dönem: %d %s", res.status, res.raw)
	}
	if res := s.do("POST", "/api/v1/terms/"+termID+"/current", registrar, nil, nil); res.status != 200 || res.body["is_current"] != true {
		t.Fatalf("aktif dönem: %d %s", res.status, res.raw)
	}

	// Olaylar: kapsam yetkisi.
	event := func(scope, scopeID, from, to string, published bool) map[string]any {
		return map[string]any{
			"type": "COURSE_REGISTRATION", "scope_type": scope, "scope_id": scopeID,
			"starts_at": istanbul(from).Format(time.RFC3339), "ends_at": istanbul(to).Format(time.RFC3339),
			"is_published": published,
		}
	}
	path := "/api/v1/terms/" + termID + "/events"
	if res := s.do("POST", path, facultyMg, nil, event("UNIVERSITY", "", "2026-09-22 00:00", "2026-09-27 00:00", true)); res.status != 403 {
		t.Errorf("fakülte yöneticisi üniversite olayı oluşturamaz: %d %s", res.status, res.raw)
	}
	if res := s.do("POST", path, facultyMg, nil, event("FACULTY", s.fx.fen, "2026-09-22 00:00", "2026-09-27 00:00", true)); res.status != 403 {
		t.Errorf("başka fakülte: %d", res.status)
	}
	res = s.do("POST", path, registrar, nil, event("UNIVERSITY", "", "2026-09-22 00:00", "2026-09-27 00:00", true))
	if res.status != 201 {
		t.Fatalf("üniversite olayı: %d %s", res.status, res.raw)
	}
	uniEvent := res.body["id"].(string)
	if res := s.do("POST", path, registrar, nil, event("UNIVERSITY", "", "2026-09-23 00:00", "2026-09-24 00:00", true)); res.status != 409 || res.code() != "EVENT_OVERLAP" {
		t.Errorf("çakışan olay: %d %s", res.status, res.raw)
	}
	res = s.do("POST", path, facultyMg, nil, event("FACULTY", s.fx.muh, "2026-09-29 00:00", "2026-10-03 00:00", false))
	if res.status != 201 || res.body["scope"].(map[string]any)["name"] != "Mühendislik Fakültesi" {
		t.Fatalf("fakülte olayı: %d %s", res.status, res.raw)
	}
	draft := res.body["id"].(string)

	// Taslak olayı öğrenci görmez.
	if res := s.do("GET", path, student, nil, nil); len(res.body["items"].([]any)) != 1 {
		t.Errorf("öğrenci sadece yayımlanmış olayı görmeli: %s", res.raw)
	}
	if res := s.do("GET", "/api/v1/calendar-events/"+draft, student, nil, nil); res.status != 404 {
		t.Errorf("taslak olay öğrenciye 404: %d", res.status)
	}
	if res := s.do("GET", path, registrar, nil, nil); len(res.body["items"].([]any)) != 2 {
		t.Errorf("yönetici taslağı da görmeli: %s", res.raw)
	}

	// Taslak yayımlanınca mühendislik programının penceresi fakülteninki olur.
	res = s.do("GET", "/api/v1/calendar-events/"+draft, facultyMg, nil, nil)
	upd := event("FACULTY", s.fx.muh, "2026-09-29 00:00", "2026-10-03 00:00", true)
	delete(upd, "type")
	if res := s.do("PUT", "/api/v1/calendar-events/"+draft, facultyMg, nil, upd); res.status != 428 {
		t.Errorf("If-Match olmadan: %d", res.status)
	}
	if res := s.do("PUT", "/api/v1/calendar-events/"+draft, facultyMg, map[string]string{"If-Match": res.header.Get("ETag")}, upd); res.status != 200 || res.body["is_published"] != true {
		t.Fatalf("yayımlama: %d %s", res.status, res.raw)
	}

	res = s.do("GET", "/api/v1/calendar/windows?program_id="+s.fx.program, student, nil, nil)
	if res.status != 200 {
		t.Fatalf("pencereler: %d %s", res.status, res.raw)
	}
	for _, it := range res.body["items"].([]any) {
		w := it.(map[string]any)
		if w["type"].(map[string]any)["code"] == "COURSE_REGISTRATION" {
			if w["open"] != false || w["scope_type"] != "FACULTY" || w["next"] == nil {
				t.Errorf("mühendislik penceresi: %v", w)
			}
		}
	}
	res = s.do("GET", "/api/v1/calendar/windows?faculty_id="+s.fx.fen, student, nil, nil)
	for _, it := range res.body["items"].([]any) {
		w := it.(map[string]any)
		if w["type"].(map[string]any)["code"] == "COURSE_REGISTRATION" && (w["open"] != true || w["scope_type"] != "UNIVERSITY") {
			t.Errorf("fen penceresi: %v", w)
		}
	}

	if res := s.do("DELETE", "/api/v1/calendar-events/"+uniEvent, facultyMg, nil, nil); res.status != 403 {
		t.Errorf("fakülte yöneticisi üniversite olayını silemez: %d", res.status)
	}
	if res := s.do("DELETE", "/api/v1/calendar-events/"+uniEvent, registrar, nil, nil); res.status != 204 {
		t.Errorf("silme: %d %s", res.status, res.raw)
	}
}
