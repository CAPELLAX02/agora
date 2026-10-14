package grading_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/academic"
	"github.com/CAPELLAX02/agora/backend/internal/grading"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
)

// fakeCalendar, açık pencerelerin kümesidir (olay türü kodu → açık).
type fakeCalendar struct {
	mu   sync.Mutex
	open map[string]bool
}

func (f *fakeCalendar) set(code string, open bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.open[code] = open
}

func (f *fakeCalendar) Window(_ context.Context, _, code string, _ academic.WindowTarget, at time.Time) (academic.Window, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.open[code] {
		ev := academic.Event{StartsAt: at.Add(-time.Hour), EndsAt: at.Add(24 * time.Hour)}
		return academic.Window{Open: true, Current: &ev}, nil
	}
	next := academic.Event{StartsAt: at.Add(72 * time.Hour), EndsAt: at.Add(96 * time.Hour)}
	return academic.Window{Next: &next}, nil
}

// fakeMetrics, sayılan olaylardır: "scores/csv", "published/final", "changes/applied" → toplam.
type fakeMetrics struct {
	mu sync.Mutex
	n  map[string]int
}

func (f *fakeMetrics) add(key string, n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.n[key] += n
}

func (f *fakeMetrics) get(key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.n[key]
}

func (f *fakeMetrics) ScoresEntered(source string, n int)  { f.add("scores/"+source, n) }
func (f *fakeMetrics) GradesPublished(phase string, n int) { f.add("published/"+phase, n) }
func (f *fakeMetrics) GradeChange(outcome string)          { f.add("changes/"+outcome, 1) }

type gradebookFixture struct {
	fixture
	enrollments []string // Ali Yıldız, Ece Arslan, Can Çelik (soyadına göre: Arslan, Çelik, Yıldız)
	components  map[string]string
}

// seedClass, şubeye üç öğrencinin kesin kaydını ve bir değerlendirme planını ekler.
func seedClass(t *testing.T, pool *pgxpool.Pool) gradebookFixture {
	t.Helper()
	ctx := context.Background()
	fx := gradebookFixture{fixture: seed(t, pool), components: map[string]string{}}
	_, err := pool.Exec(ctx, `
		INSERT INTO org.programs (department_id, code, name_tr, name_en, degree_level, language, education_type,
		                          duration_semesters, max_duration_years, total_ects_required)
		SELECT id, 'BIL-L', 'Bilgisayar Mühendisliği', 'Computer Engineering', 'BACHELOR', 'EN', 'DAYTIME', 8, 7, 240
		FROM org.departments`)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []struct{ no, first, last string }{
		{"22290101", "Ali", "Yıldız"}, {"22290102", "Ece", "Arslan"}, {"22290103", "Can", "Çelik"},
	} {
		var id string
		err := pool.QueryRow(ctx, `
			WITH p AS (INSERT INTO people.persons (first_name, last_name) VALUES ($2, $3) RETURNING id),
			st AS (INSERT INTO people.students (person_id, student_no) SELECT id, $1 FROM p RETURNING id),
			sp AS (
				INSERT INTO enrollment.student_programs (student_id, program_id, admission_type, admission_year, admitted_on)
				SELECT st.id, pr.id, 'OSYS', 2022, '2022-09-15' FROM st, org.programs pr RETURNING id
			)
			INSERT INTO enrollment.enrollments (student_program_id, section_id, term_id, course_id, attempt_no, ects_snapshot, credit_snapshot)
			SELECT sp.id, s.id, o.term_id, o.course_id, 1, 6, 5
			FROM sp, offering.sections s JOIN offering.course_offerings o ON o.id = s.offering_id
			RETURNING id`, s.no, s.first, s.last).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		fx.enrollments = append(fx.enrollments, id)
	}
	repo := grading.NewRepository(pool)
	if err := repo.SetPlan(ctx, "", fx.section, 1, []grading.ComponentInput{
		{TypeCode: "MIDTERM", SequenceNo: 1, Weight: 30},
		{TypeCode: "HOMEWORK", SequenceNo: 1, Weight: 10},
		{TypeCode: "FINAL", SequenceNo: 1, Weight: 60},
	}); err != nil {
		t.Fatal(err)
	}
	p, err := repo.Plan(ctx, fx.section, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range p.Components {
		fx.components[c.Type.Code] = c.ID
	}
	return fx
}

type gradebookServer struct {
	*server
	pool     *pgxpool.Pool
	calendar *fakeCalendar
	grants   grants
	fx       gradebookFixture
	metrics  *fakeMetrics
}

// addGrant, kullanıcıya test sırasında bir yetki daha verir.
func (s *gradebookServer) addGrant(userID string, g authz.Grant) {
	s.grants[userID] = append(s.grants[userID], g)
}

func newGradebookServer(t *testing.T) *gradebookServer {
	t.Helper()
	pool := dbtest.New(t)
	fx := seedClass(t, pool)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	authenticate := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := authn.Principal{UserID: r.Header.Get("X-Test-User")}
			if r.Header.Get("X-Test-MFA") != "" { // iki adımlı doğrulamayla açılmış oturum
				p.AMR = []string{authn.MethodOTP}
			}
			next.ServeHTTP(w, r.WithContext(authn.WithPrincipal(r.Context(), p)))
		})
	}
	read := authz.Grant{Permission: "course:read", ScopeType: authz.ScopeNone}
	instructor := []authz.Grant{read,
		{Permission: "assessment_plan:manage", ScopeType: authz.ScopeDepartment, ScopeID: fx.bil},
		{Permission: "score:enter", ScopeType: authz.ScopeDepartment, ScopeID: fx.bil, RequiresMFA: true},
	}
	resolver := grants{
		fx.ayseUser:   instructor,
		fx.mehmetUser: instructor,
		head: {read,
			{Permission: "section:manage", ScopeType: authz.ScopeDepartment, ScopeID: fx.bil},
			{Permission: "grade:read_scoped", ScopeType: authz.ScopeDepartment, ScopeID: fx.bil},
		},
		student: {read},
	}
	calendar := &fakeCalendar{open: map[string]bool{grading.WindowMidtermEntry: true}}
	mux := http.NewServeMux()
	rt := authz.NewRouter(mux, authenticate, resolver, logger)
	grading.NewHandler(grading.NewRepository(pool), logger).Register(rt)
	svc := grading.NewService(pool, calendar, time.Now)
	m := &fakeMetrics{n: map[string]int{}}
	svc.SetMetrics(m)
	grading.NewGradebookHandler(svc, logger).Register(rt)
	return &gradebookServer{server: &server{t: t, h: mux, fx: fx.fixture}, pool: pool, calendar: calendar, grants: resolver, fx: fx, metrics: m}
}

var mfa = map[string]string{"X-Test-MFA": "1"}

func with(h map[string]string, kv ...string) map[string]string {
	out := map[string]string{}
	for k, v := range h {
		out[k] = v
	}
	for i := 0; i+1 < len(kv); i += 2 {
		out[kv[i]] = kv[i+1]
	}
	return out
}

func component(res result, id string) map[string]any {
	for _, c := range res.body["components"].([]any) {
		if m := c.(map[string]any); m["id"] == id {
			return m
		}
	}
	return nil
}

func studentRow(res result, enrollmentID string) map[string]any {
	for _, s := range res.body["students"].([]any) {
		if m := s.(map[string]any); m["enrollment_id"] == enrollmentID {
			return m
		}
	}
	return nil
}

func TestHTTPGradebook(t *testing.T) {
	s := newGradebookServer(t)
	fx := s.fx
	book := "/api/v1/sections/" + fx.section + "/gradebook"
	scores := func(code string) string { return "/api/v1/assessment-components/" + fx.components[code] + "/scores" }
	midterm := fx.components["MIDTERM"]
	item := func(enrollment string, score any, absent bool) map[string]any {
		return map[string]any{"enrollment_id": enrollment, "score": score, "absent": absent}
	}
	body := func(items ...map[string]any) map[string]any { return map[string]any{"items": items} }

	// Görme yetkisi: şubenin öğretim elemanı ve kapsamlı okuma; başka öğretim elemanı ve öğrenci göremez.
	res := s.do("GET", book, fx.ayseUser, nil, nil)
	if res.status != 200 || res.body["can_enter"] != false || res.body["mfa_required"] != true {
		t.Fatalf("MFA'sız not tablosu: %d %s", res.status, res.raw)
	}
	names := []string{}
	for _, st := range res.body["students"].([]any) {
		names = append(names, st.(map[string]any)["student"].(map[string]any)["last_name"].(string))
	}
	if len(names) != 3 || names[0] != "Arslan" || names[1] != "Çelik" || names[2] != "Yıldız" {
		t.Errorf("öğrenciler Türkçe soyadı sırasıyla gelmeli: %v", names)
	}
	if len(res.body["components"].([]any)) != 4 || res.body["section"].(map[string]any)["plan_locked"] != false {
		t.Errorf("bileşenler: %s", res.raw)
	}
	for user, want := range map[string]int{fx.mehmetUser: 403, student: 403, head: 200} {
		if res := s.do("GET", book, user, mfa, nil); res.status != want {
			t.Errorf("%s: %d, beklenen %d", user, res.status, want)
		}
	}
	if res := s.do("GET", book, head, mfa, nil); res.body["can_enter"] != false {
		t.Errorf("bölüm başkanı not girmez: %s", res.raw)
	}

	// Not girişi MFA ister ve sadece şubenin öğretim elemanı girer; plan kilitli olmalı.
	v1 := map[string]string{"If-Match": `"1"`}
	if res := s.do("PUT", scores("MIDTERM"), fx.ayseUser, v1, body(item(fx.enrollments[0], 70, false))); res.status != 403 || res.code() != "MFA_REQUIRED" {
		t.Errorf("MFA'sız not girişi: %d %s", res.status, res.raw)
	}
	if res := s.do("PUT", scores("MIDTERM"), fx.mehmetUser, with(mfa, "If-Match", `"1"`), body(item(fx.enrollments[0], 70, false))); res.status != 403 || res.code() != "FORBIDDEN" {
		t.Errorf("şubede olmayan öğretim elemanı: %d %s", res.status, res.raw)
	}
	if res := s.do("PUT", scores("MIDTERM"), fx.ayseUser, with(mfa, "If-Match", `"1"`), body(item(fx.enrollments[0], 70, false))); res.status != 409 || res.code() != "ASSESSMENT_PLAN_NOT_LOCKED" {
		t.Errorf("kilitsiz plan: %d %s", res.status, res.raw)
	}
	if res := s.do("POST", "/api/v1/sections/"+fx.section+"/assessment-plan/lock", fx.ayseUser, nil, nil); res.status != 200 {
		t.Fatalf("kilit: %d %s", res.status, res.raw)
	}

	res = s.do("GET", book, fx.ayseUser, mfa, nil)
	if res.body["can_enter"] != true || res.body["mfa_required"] != false {
		t.Errorf("MFA'lı not tablosu: %s", res.raw)
	}
	editable := map[string]bool{}
	for code, id := range fx.components {
		editable[code] = component(res, id)["editable"].(bool)
	}
	makeup := ""
	for _, c := range res.body["components"].([]any) {
		if m := c.(map[string]any); m["type"].(map[string]any)["code"] == "MAKEUP" {
			makeup = m["id"].(string)
			if m["editable"] != false {
				t.Errorf("bütünleme ilandan önce düzenlenemez")
			}
		}
	}
	if !editable["MIDTERM"] || !editable["HOMEWORK"] || editable["FINAL"] {
		t.Errorf("ara sınav penceresinde düzenlenebilir bileşenler: %v", editable)
	}
	if w := component(res, fx.components["FINAL"])["entry_window"].(map[string]any); w["open"] != false || w["next_starts_at"] == nil {
		t.Errorf("final penceresi: %v", w)
	}

	// Doğrulama.
	vm := with(mfa, "If-Match", `"1"`)
	for name, bad := range map[string]map[string]any{
		"100'ü aşan":     body(item(fx.enrollments[0], 100.5, false)),
		"üç ondalık":     body(item(fx.enrollments[0], 70.125, false)),
		"girmedi + puan": body(item(fx.enrollments[0], 70, true)),
		"tekrar":         body(item(fx.enrollments[0], 70, false), item(fx.enrollments[0], 80, false)),
		"şubede yok":     body(item("01a11b7f-0000-7000-8000-0000000000ff", 70, false)),
		"boş":            body(),
	} {
		if res := s.do("PUT", scores("MIDTERM"), fx.ayseUser, vm, bad); res.status != 400 {
			t.Errorf("%s: %d %s", name, res.status, res.raw)
		}
	}
	if res := s.do("PUT", scores("MIDTERM"), fx.ayseUser, mfa, body(item(fx.enrollments[0], 70, false))); res.status != 428 {
		t.Errorf("If-Match olmadan: %d", res.status)
	}
	if res := s.do("PUT", scores("FINAL"), fx.ayseUser, vm, body(item(fx.enrollments[0], 70, false))); res.status != 409 || res.code() != "GRADE_ENTRY_WINDOW_CLOSED" {
		t.Errorf("kapalı pencere: %d %s", res.status, res.raw)
	}
	if res := s.do("PUT", "/api/v1/assessment-components/"+makeup+"/scores", fx.ayseUser, vm, body(item(fx.enrollments[0], 70, false))); res.status != 409 || res.code() != "MAKEUP_NOT_AVAILABLE" {
		t.Errorf("bütünleme: %d %s", res.status, res.raw)
	}

	// Yazma: 75,5 ve 90 puan, bir öğrenci girmedi.
	res = s.do("PUT", scores("MIDTERM"), fx.ayseUser, vm, body(
		item(fx.enrollments[0], 75.5, false), item(fx.enrollments[1], nil, true), item(fx.enrollments[2], 90, false)))
	if res.status != 200 {
		t.Fatalf("not girişi: %d %s", res.status, res.raw)
	}
	c := component(res, midterm)
	if c["version"] != 2.0 || c["scored_count"] != 2.0 || c["absent_count"] != 1.0 || c["average"] != 82.75 {
		t.Errorf("bileşen özeti: %v", c)
	}
	if got := studentRow(res, fx.enrollments[0])["scores"].(map[string]any)[midterm].(map[string]any); got["score"] != 75.5 || got["absent"] != false {
		t.Errorf("puan: %v", got)
	}
	if got := studentRow(res, fx.enrollments[1])["scores"].(map[string]any)[midterm].(map[string]any); got["score"] != nil || got["absent"] != true {
		t.Errorf("girmedi: %v", got)
	}
	if res := s.do("PUT", scores("MIDTERM"), fx.ayseUser, vm, body(item(fx.enrollments[0], 80, false))); res.status != 412 {
		t.Errorf("eski sürüm: %d", res.status)
	}
	// Aynı notların tekrarı sürümü artırmaz; boş not silinir.
	v2 := with(mfa, "If-Match", `"2"`)
	res = s.do("PUT", scores("MIDTERM"), fx.ayseUser, v2, body(item(fx.enrollments[0], 75.5, false)))
	if component(res, midterm)["version"] != 2.0 {
		t.Errorf("değişmeyen not sürümü artırmamalı: %v", component(res, midterm))
	}
	res = s.do("PUT", scores("MIDTERM"), fx.ayseUser, v2, body(item(fx.enrollments[2], nil, false)))
	if c := component(res, midterm); c["version"] != 3.0 || c["scored_count"] != 1.0 || c["average"] != 75.5 {
		t.Errorf("silinen not: %v", c)
	}
	if _, ok := studentRow(res, fx.enrollments[2])["scores"].(map[string]any)[midterm]; ok {
		t.Errorf("silinen not tabloda kalmamalı")
	}

	// Yayımlama: notsuz bileşen yayımlanamaz; ikinci yayım bir şey değiştirmez.
	if res := s.do("POST", "/api/v1/assessment-components/"+fx.components["HOMEWORK"]+"/publish", fx.ayseUser, mfa, nil); res.status != 409 || res.code() != "NO_SCORES" {
		t.Errorf("notsuz yayım: %d %s", res.status, res.raw)
	}
	res = s.do("POST", "/api/v1/assessment-components/"+midterm+"/publish", fx.ayseUser, mfa, nil)
	published := component(res, midterm)["published_at"]
	if res.status != 200 || published == nil {
		t.Fatalf("yayım: %d %s", res.status, res.raw)
	}
	if res := s.do("POST", "/api/v1/assessment-components/"+midterm+"/publish", fx.ayseUser, mfa, nil); component(res, midterm)["published_at"] != published {
		t.Errorf("ikinci yayım tarihi değiştirmemeli")
	}

	// Devam durumu.
	attendance := "/api/v1/sections/" + fx.section + "/attendance"
	if res := s.do("PUT", attendance, fx.ayseUser, mfa, map[string]any{"items": []any{map[string]any{"enrollment_id": fx.enrollments[1], "attendance_status": "ABSENT"}}}); res.status != 400 {
		t.Errorf("geçersiz devam durumu: %d", res.status)
	}
	res = s.do("PUT", attendance, fx.ayseUser, mfa, map[string]any{"items": []any{map[string]any{"enrollment_id": fx.enrollments[1], "attendance_status": "ATTENDANCE_FAIL"}}})
	if res.status != 200 || studentRow(res, fx.enrollments[1])["attendance_status"] != "ATTENDANCE_FAIL" {
		t.Errorf("devamsız: %d %s", res.status, res.raw)
	}

	// Pencereler: ara sınav penceresi kapanınca girilemez; dönem içi bileşenler final
	// penceresinde de girilir.
	s.calendar.set(grading.WindowMidtermEntry, false)
	if res := s.do("PUT", scores("HOMEWORK"), fx.ayseUser, with(mfa, "If-Match", `"1"`), body(item(fx.enrollments[0], 60, false))); res.status != 409 || res.code() != "GRADE_ENTRY_WINDOW_CLOSED" {
		t.Errorf("kapalı ara sınav penceresi: %d %s", res.status, res.raw)
	}
	s.calendar.set(grading.WindowFinalEntry, true)
	res = s.do("PUT", scores("FINAL"), fx.ayseUser, with(mfa, "If-Match", `"1"`), body(item(fx.enrollments[0], 65, false)))
	if res.status != 200 || component(res, fx.components["HOMEWORK"])["editable"] != true || component(res, midterm)["editable"] != true {
		t.Errorf("final penceresi: %d %s", res.status, res.raw)
	}

	// Bırakan öğrenci tablodan ve ortalamadan çıkar.
	if _, err := s.pool.Exec(context.Background(), `UPDATE enrollment.enrollments SET status = 'DROPPED' WHERE id = $1`, fx.enrollments[0]); err != nil {
		t.Fatal(err)
	}
	res = s.do("GET", book, fx.ayseUser, mfa, nil)
	if len(res.body["students"].([]any)) != 2 || component(res, midterm)["scored_count"] != 0.0 || component(res, midterm)["average"] != nil ||
		component(res, fx.components["FINAL"])["scored_count"] != 0.0 {
		t.Errorf("bırakan öğrenci: %s", res.raw)
	}

	// Notu olan bileşen plandan çıkarılamaz.
	if res := s.do("POST", "/api/v1/sections/"+fx.section+"/assessment-plan/unlock", head, nil, map[string]any{"reason": "Ödev kaldırılacak"}); res.status != 200 {
		t.Fatalf("kilit açma: %d %s", res.status, res.raw)
	}
	plan := s.do("GET", "/api/v1/sections/"+fx.section+"/assessment-plan", fx.ayseUser, nil, nil)
	res = s.do("PUT", "/api/v1/sections/"+fx.section+"/assessment-plan", fx.ayseUser, map[string]string{"If-Match": plan.header.Get("ETag")},
		map[string]any{"components": []any{map[string]any{"type": "HOMEWORK", "weight": 40}, map[string]any{"type": "FINAL", "weight": 60}}})
	if res.status != 409 || res.code() != "COMPONENT_HAS_SCORES" {
		t.Errorf("notlu bileşeni çıkarma: %d %s", res.status, res.raw)
	}

	var audits int
	if err := s.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM audit.audit_log WHERE action IN ('assessment_scores.set', 'assessment_scores.publish', 'attendance.set')`).Scan(&audits); err != nil || audits != 5 {
		t.Errorf("denetim kayıtları = %d, %v", audits, err)
	}
}

// TestConcurrentScoreWrites: iki öğretim elemanı aynı bileşenin aynı sürümüne aynı anda
// yazarsa sadece biri başarılı olur, diğeri sürüm uyuşmazlığı alır (kayıp güncelleme yok).
func TestConcurrentScoreWrites(t *testing.T) {
	pool := dbtest.New(t)
	fx := seedClass(t, pool)
	ctx := context.Background()
	if err := grading.NewRepository(pool).Lock(ctx, "", fx.section); err != nil {
		t.Fatal(err)
	}
	calendar := &fakeCalendar{open: map[string]bool{grading.WindowMidtermEntry: true}}
	svc := grading.NewService(pool, calendar, time.Now)
	ref, err := svc.ComponentRef(ctx, fx.components["MIDTERM"], fx.ayseUser)
	if err != nil {
		t.Fatal(err)
	}

	const writers = 8
	var (
		wg             sync.WaitGroup
		mu             sync.Mutex
		ok, mismatches int
	)
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p := grading.Points(5000 + i*100)
			_, err := svc.SetScores(ctx, fx.ayseUser, ref, 1, []grading.ScoreInput{{EnrollmentID: fx.enrollments[0], Points: &p}})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				ok++
			case errors.Is(err, grading.ErrVersionMismatch):
				mismatches++
			default:
				t.Errorf("beklenmeyen hata: %v", err)
			}
		}()
	}
	wg.Wait()
	if ok != 1 || mismatches != writers-1 {
		t.Errorf("başarılı = %d, sürüm uyuşmazlığı = %d", ok, mismatches)
	}
	g, err := svc.Gradebook(ctx, fx.section, fx.ayseUser)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range g.Components {
		if c.ID == ref.ID && (c.Version != 2 || c.Scored != 1) {
			t.Errorf("bileşen = sürüm %d, %d not", c.Version, c.Scored)
		}
	}
}
