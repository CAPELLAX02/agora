package announce_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/announce"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
)

type grants map[string][]authz.Grant

func (g grants) Permissions(_ context.Context, userID string) (*authz.Permissions, error) {
	return authz.NewPermissions(userID, append(g[userID], authz.Grant{Permission: "announcement:read", ScopeType: authz.ScopeNone})), nil
}

type fixture struct {
	muh, bil, eem, bilL, eemL      string
	ali, ece, bilStaff, adminStaff string // hedef kitle adayları
	head, dean, central, other     string // yayımlayanlar (yetkileri sahte)
}

func seed(t *testing.T, pool *pgxpool.Pool) fixture {
	t.Helper()
	ctx := context.Background()
	var f fixture
	err := pool.QueryRow(ctx, `
		WITH fac AS (INSERT INTO org.faculties (code, name_tr, name_en, unit_type) VALUES ('MUH', 'Mühendislik Fakültesi', 'Engineering', 'FACULTY') RETURNING id),
		bil AS (INSERT INTO org.departments (faculty_id, code, name_tr, name_en) SELECT id, 'BIL', 'Bilgisayar Mühendisliği', 'CE' FROM fac RETURNING id),
		eem AS (INSERT INTO org.departments (faculty_id, code, name_tr, name_en) SELECT id, 'EEM', 'Elektrik Mühendisliği', 'EE' FROM fac RETURNING id)
		SELECT (SELECT id FROM fac), (SELECT id FROM bil), (SELECT id FROM eem)`).Scan(&f.muh, &f.bil, &f.eem)
	if err != nil {
		t.Fatal(err)
	}
	program := func(dept, code string) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, `
			INSERT INTO org.programs (department_id, code, name_tr, name_en, degree_level, language, education_type,
			                          duration_semesters, max_duration_years, total_ects_required)
			VALUES ($1, $2, $2, $2, 'BACHELOR', 'EN', 'DAYTIME', 8, 7, 240) RETURNING id`, dept, code).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	f.bilL, f.eemL = program(f.bil, "BIL-L"), program(f.eem, "EEM-L")
	user := func(username string) (person, id string) {
		t.Helper()
		if err := pool.QueryRow(ctx, `
			WITH p AS (INSERT INTO people.persons (first_name, last_name) VALUES ('Kişi', $1) RETURNING id)
			INSERT INTO iam.users (person_id, username, email, password_hash) SELECT id, $1, $1 || '@agora.test', 'x' FROM p
			RETURNING person_id, id`, username).Scan(&person, &id); err != nil {
			t.Fatal(err)
		}
		return person, id
	}
	student := func(no, programID string) string {
		t.Helper()
		person, id := user(no)
		if _, err := pool.Exec(ctx, `
			WITH st AS (INSERT INTO people.students (person_id, student_no) VALUES ($1, $2) RETURNING id)
			INSERT INTO enrollment.student_programs (student_id, program_id, admission_type, admission_year, admitted_on)
			SELECT id, $3, 'OSYS', 2024, '2024-09-15' FROM st`, person, no, programID); err != nil {
			t.Fatal(err)
		}
		return id
	}
	staff := func(no string, dept any) string {
		t.Helper()
		person, id := user(no)
		if _, err := pool.Exec(ctx, `
			INSERT INTO people.staff (person_id, staff_no, staff_type, primary_department_id) VALUES ($1, $2, 'ADMINISTRATIVE', $3)`,
			person, no, dept); err != nil {
			t.Fatal(err)
		}
		return id
	}
	f.ali, f.ece = student("24000001", f.bilL), student("24000002", f.eemL)
	f.bilStaff, f.adminStaff = staff("P9001", f.bil), staff("P9002", nil)
	_, f.head = user("P9101")
	_, f.dean = user("P9102")
	_, f.central = user("P9103")
	_, f.other = user("P9104")
	return f
}

type server struct {
	t    *testing.T
	pool *pgxpool.Pool
	svc  *announce.Service
	h    http.Handler
	fx   fixture
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
	publish := func(scope, id string) []authz.Grant {
		return []authz.Grant{{Permission: "announcement:publish", ScopeType: scope, ScopeID: id}}
	}
	resolver := grants{
		fx.head:    publish(authz.ScopeDepartment, fx.bil),
		fx.dean:    publish(authz.ScopeFaculty, fx.muh),
		fx.central: publish(authz.ScopeUniversity, ""),
	}
	mux := http.NewServeMux()
	svc := announce.NewService(pool, time.Now)
	announce.NewHandler(svc, logger).Register(authz.NewRouter(mux, authenticate, resolver, logger))
	return &server{t: t, pool: pool, svc: svc, h: mux, fx: fx}
}

type result struct {
	status int
	header http.Header
	body   map[string]any
	raw    string
}

func (s *server) do(method, path, user string, body any, headers ...string) result {
	s.t.Helper()
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-User", user)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	s.h.ServeHTTP(rec, req)
	res := result{status: rec.Code, header: rec.Header(), raw: rec.Body.String()}
	_ = json.Unmarshal(rec.Body.Bytes(), &res.body)
	return res
}

func (s *server) notes(userID string) []string {
	s.t.Helper()
	rows, err := s.pool.Query(context.Background(), `
		SELECT type_code FROM communication.notifications WHERE user_id = $1 ORDER BY id`, userID)
	if err != nil {
		s.t.Fatal(err)
	}
	out, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		s.t.Fatal(err)
	}
	return out
}

func (s *server) feed(user string) []map[string]any {
	s.t.Helper()
	res := s.do("GET", "/api/v1/me/announcements", user, nil)
	if res.status != 200 {
		s.t.Fatalf("akış: %d %s", res.status, res.raw)
	}
	var out []map[string]any
	for _, it := range res.body["items"].([]any) {
		out = append(out, it.(map[string]any))
	}
	return out
}

func audience(scopeType string, scopeID any, people string) map[string]any {
	return map[string]any{"scope_type": scopeType, "scope_id": scopeID, "people": people}
}

func TestAnnouncementLifecycle(t *testing.T) {
	s := newServer(t)
	fx := s.fx
	body := map[string]any{"title": "Bölüm toplantısı", "summary": "Cuma 14.00'te A101'de.", "body_html": "<p>Gündem <script>x</script>ekte.</p>",
		"type": "GENERAL", "audiences": []any{audience("DEPARTMENT", fx.bil, "ALL")}}

	// Yetki ve doğrulama.
	if res := s.do("POST", "/api/v1/announcements", fx.other, body); res.status != 403 {
		t.Errorf("yetkisiz: %d", res.status)
	}
	if res := s.do("POST", "/api/v1/announcements", fx.head, map[string]any{"title": "x", "type": "GENERAL",
		"audiences": []any{audience("FACULTY", fx.muh, "ALL")}}); res.status != 403 {
		t.Errorf("kapsam dışı hedef: %d %s", res.status, res.raw)
	}
	if res := s.do("POST", "/api/v1/announcements", fx.head, map[string]any{"title": "x", "type": "GENERAL",
		"audiences": []any{audience("PROGRAM", fx.bilL, "STAFF")}}); res.status != 400 {
		t.Errorf("program hedefinde personel: %d", res.status)
	}

	// Taslak: kimse görmez, bildirilmez.
	res := s.do("POST", "/api/v1/announcements", fx.head, body)
	if res.status != 201 || res.body["status"] != "DRAFT" || res.body["body_html"] != "<p>Gündem ekte.</p>" ||
		res.body["audiences"].([]any)[0].(map[string]any)["scope_name"] != "Bilgisayar Mühendisliği" {
		t.Fatalf("taslak: %d %s", res.status, res.raw)
	}
	id := res.body["id"].(string)
	path := "/api/v1/announcements/" + id
	if len(s.feed(fx.ali)) != 0 || len(s.notes(fx.ali)) != 0 {
		t.Errorf("taslak görünmemeli")
	}
	if res := s.do("GET", path, fx.ali, nil); res.status != 404 {
		t.Errorf("taslağı öğrenci okuyamaz: %d", res.status)
	}

	// Yayın: bölümün öğrencisi ve personeli bildirim alır; başka bölüm ve birimsiz personel almaz.
	if res := s.do("POST", path+"/publish", fx.head, nil); res.status != 428 {
		t.Errorf("If-Match'siz yayın: %d", res.status)
	}
	if res := s.do("POST", path+"/publish", fx.head, nil, "If-Match", `"1"`); res.status != 200 || res.body["status"] != "PUBLISHED" || res.body["notified_at"] == nil {
		t.Fatalf("yayın: %d %s", res.status, res.raw)
	}
	for _, u := range []string{fx.ali, fx.bilStaff} {
		if got := s.notes(u); !slices.Equal(got, []string{"announcement.published"}) {
			t.Errorf("hedef kitle bildirimi: %v", got)
		}
	}
	for _, u := range []string{fx.ece, fx.adminStaff, fx.head} {
		if got := s.notes(u); len(got) != 0 {
			t.Errorf("hedef kitle dışı bildirim: %v", got)
		}
	}
	if feed := s.feed(fx.ali); len(feed) != 1 || feed[0]["read_at"] != nil {
		t.Fatalf("öğrencinin akışı: %v", feed)
	}
	if len(s.feed(fx.ece)) != 0 {
		t.Errorf("başka bölümün akışı boş olmalı")
	}
	if res := s.do("GET", path, fx.ece, nil); res.status != 404 {
		t.Errorf("hedef dışı okuma: %d", res.status)
	}
	if res := s.do("GET", path, fx.ali, nil); res.status != 200 || res.body["can_manage"] != false || res.body["stats"] != nil {
		t.Errorf("öğrenci okur: %d %s", res.status, res.raw)
	}
	if feed := s.feed(fx.ali); feed[0]["read_at"] == nil {
		t.Errorf("okundu işaretlenmedi")
	}

	// Yöneten: yazar ve kapsamı hedefleri kapsayan dekan; istatistikle.
	res = s.do("GET", path, fx.dean, nil)
	stats, _ := res.body["stats"].(map[string]any)
	if res.status != 200 || res.body["can_manage"] != true || stats["audience"] != 2.0 || stats["reads"] != 1.0 {
		t.Errorf("dekanın görünümü: %d %s", res.status, res.raw)
	}
	body["title"], body["is_pinned"] = "Bölüm toplantısı (salon değişti)", true
	if res := s.do("PUT", path, fx.dean, body, "If-Match", `"2"`); res.status != 200 || res.body["version"] != 3.0 {
		t.Errorf("dekan düzenler: %d %s", res.status, res.raw)
	}
	if got := s.notes(fx.ali); len(got) != 1 {
		t.Errorf("düzenleme yeniden bildirmemeli: %v", got)
	}
	if res := s.do("PUT", path, fx.head, body, "If-Match", `"2"`); res.status != 412 {
		t.Errorf("eski sürüm: %d", res.status)
	}

	// Yönetim listesi kapsamı izler.
	count := func(user string) int {
		res := s.do("GET", "/api/v1/announcements", user, nil)
		if res.status != 200 {
			t.Fatalf("yönetim listesi: %d %s", res.status, res.raw)
		}
		return len(res.body["items"].([]any))
	}
	if count(fx.head) != 1 || count(fx.dean) != 1 || count(fx.central) != 1 {
		t.Errorf("yönetim listeleri: %d %d %d", count(fx.head), count(fx.dean), count(fx.central))
	}
	if res := s.do("GET", "/api/v1/announcements", fx.other, nil); res.status != 403 {
		t.Errorf("yetkisiz yönetim listesi: %d", res.status)
	}

	// Arşiv akıştan kaldırır; yayımlanmış silinmez.
	if res := s.do("DELETE", path, fx.head, nil); res.status != 409 {
		t.Errorf("yayımlanmış silinmez: %d", res.status)
	}
	if res := s.do("POST", path+"/archive", fx.head, nil, "If-Match", `"3"`); res.status != 200 || res.body["status"] != "ARCHIVED" {
		t.Fatalf("arşiv: %d %s", res.status, res.raw)
	}
	if len(s.feed(fx.ali)) != 0 {
		t.Errorf("arşivlenen akışta kalmamalı")
	}
	draft := s.do("POST", "/api/v1/announcements", fx.head, body)
	if res := s.do("DELETE", "/api/v1/announcements/"+draft.body["id"].(string), fx.head, nil); res.status != 204 {
		t.Errorf("taslak silinir: %d", res.status)
	}
}

func TestScheduledUrgentAndFeedOrder(t *testing.T) {
	s := newServer(t)
	fx := s.fx
	ctx := context.Background()
	students := []any{audience("UNIVERSITY", nil, "STUDENTS")}
	if res := s.do("POST", "/api/v1/announcements", fx.dean, map[string]any{"title": "x", "type": "GENERAL", "audiences": students}); res.status != 403 {
		t.Errorf("üniversite hedefi üniversite kapsamı ister: %d", res.status)
	}
	// İleri tarihli acil duyuru: yayımlanır ama zamanı gelince bildirilir (e-postayla da).
	res := s.do("POST", "/api/v1/announcements", fx.central, map[string]any{"title": "Kampüs kapalı", "type": "URGENT",
		"publish_from": time.Now().Add(time.Hour), "audiences": students, "publish": true})
	if res.status != 201 || res.body["status"] != "PUBLISHED" || res.body["notified_at"] != nil {
		t.Fatalf("ileri tarihli: %d %s", res.status, res.raw)
	}
	urgent := res.body["id"].(string)
	if n, err := s.svc.NotifyDue(ctx); err != nil || n != 0 || len(s.feed(fx.ali)) != 0 {
		t.Fatalf("zamanı gelmeden: %d %v", n, err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE communication.announcements SET publish_from = now() - interval '1 minute' WHERE id = $1`, urgent); err != nil {
		t.Fatal(err)
	}
	if n, err := s.svc.NotifyDue(ctx); err != nil || n != 1 {
		t.Fatalf("zamanı gelince: %d %v", n, err)
	}
	if n, _ := s.svc.NotifyDue(ctx); n != 0 {
		t.Errorf("ikinci kez bildirilmemeli")
	}
	for _, u := range []string{fx.ali, fx.ece} {
		if got := s.notes(u); !slices.Equal(got, []string{"announcement.urgent"}) {
			t.Errorf("acil duyuru: %v", got)
		}
	}
	if got := s.notes(fx.bilStaff); len(got) != 0 {
		t.Errorf("öğrenci duyurusu personele: %v", got)
	}
	var emails int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM communication.email_outbox WHERE template_code = 'notification'`).Scan(&emails); err != nil || emails != 2 {
		t.Errorf("acil duyuru e-postaları: %d %v", emails, err)
	}

	// Akış: sabitlenen önce, sonra yeniden eskiye; sayfa sayfa.
	for i, title := range []string{"Eski", "Sabit", "Yeni"} {
		res := s.do("POST", "/api/v1/announcements", fx.head, map[string]any{"title": title, "type": "ACADEMIC", "is_pinned": title == "Sabit",
			"publish_from": time.Now().Add(time.Duration(i-10) * time.Minute), "publish": true,
			"audiences": []any{audience("PROGRAM", fx.bilL, "STUDENTS")}})
		if res.status != 201 {
			t.Fatalf("%s: %d %s", title, res.status, res.raw)
		}
	}
	var titles []string
	cursor := ""
	for range 10 {
		path := "/api/v1/me/announcements?limit=2"
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		res := s.do("GET", path, fx.ali, nil)
		for _, it := range res.body["items"].([]any) {
			titles = append(titles, it.(map[string]any)["title"].(string))
		}
		next, _ := res.body["next_cursor"].(string)
		if next == "" {
			break
		}
		cursor = next
	}
	if want := []string{"Sabit", "Kampüs kapalı", "Yeni", "Eski"}; !slices.Equal(titles, want) {
		t.Errorf("akış sırası: %v", titles)
	}
}

func TestTargets(t *testing.T) {
	s := newServer(t)
	fx := s.fx
	names := func(res result, key string) int { return len(res.body[key].([]any)) }
	res := s.do("GET", "/api/v1/announcement-targets", fx.head, nil)
	if res.status != 200 || res.body["university"] != false || names(res, "faculties") != 0 || names(res, "departments") != 1 || names(res, "programs") != 1 {
		t.Errorf("bölüm başkanı: %s", res.raw)
	}
	res = s.do("GET", "/api/v1/announcement-targets", fx.dean, nil)
	if names(res, "faculties") != 1 || names(res, "departments") != 2 || names(res, "programs") != 2 {
		t.Errorf("dekan: %s", res.raw)
	}
	if res := s.do("GET", "/api/v1/announcement-targets", fx.central, nil); res.body["university"] != true {
		t.Errorf("merkez: %s", res.raw)
	}
}
