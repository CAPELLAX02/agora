package lms_test

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

	"github.com/CAPELLAX02/agora/backend/internal/files"
	"github.com/CAPELLAX02/agora/backend/internal/lms"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
	"github.com/CAPELLAX02/agora/backend/internal/platform/s3test"
)

type grants map[string][]authz.Grant

func (g grants) Permissions(_ context.Context, userID string) (*authz.Permissions, error) {
	return authz.NewPermissions(userID, g[userID]), nil
}

type fixture struct {
	section, department                         string
	primary, assistant, other, student, dropped string
	head                                        string
	stranger                                    string
}

// seed, bir şube kurar: sorumlu ve asistan öğretim elemanı, aynı bölümde başka bir öğretim
// elemanı, kesin kayıtlı bir öğrenci, dersi bırakmış bir öğrenci ve bölüm başkanı.
func seed(t *testing.T, pool *pgxpool.Pool) fixture {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		INSERT INTO org.faculties (code, name_tr, name_en, unit_type) VALUES ('MUH', 'Mühendislik Fakültesi', 'Faculty of Engineering', 'FACULTY');
		INSERT INTO org.departments (faculty_id, code, name_tr, name_en)
			SELECT id, 'BIL', 'Bilgisayar Mühendisliği', 'Computer Engineering' FROM org.faculties;
		INSERT INTO org.programs (department_id, code, name_tr, name_en, degree_level, language, education_type,
		                          duration_semesters, max_duration_years, total_ects_required)
			SELECT id, 'BIL-L', 'Bilgisayar Mühendisliği', 'Computer Engineering', 'BACHELOR', 'EN', 'DAYTIME', 8, 7, 240 FROM org.departments;
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
	if err := pool.QueryRow(ctx, `SELECT s.id, d.id FROM offering.sections s, org.departments d`).Scan(&f.section, &f.department); err != nil {
		t.Fatal(err)
	}
	staff := func(no, first, last, role string) string {
		t.Helper()
		var staffID, userID string
		err := pool.QueryRow(ctx, `
			WITH p AS (INSERT INTO people.persons (first_name, last_name) VALUES ($2, $3) RETURNING id),
			s AS (INSERT INTO people.staff (person_id, staff_no, staff_type, academic_title_code)
			      SELECT id, $1::text, 'ACADEMIC', 'ASSOC_PROF' FROM p RETURNING id),
			u AS (INSERT INTO iam.users (person_id, username, email, password_hash)
			      SELECT id, $1::text, $1::text || '@agora.test', 'x' FROM p RETURNING id)
			SELECT (SELECT id FROM s), (SELECT id FROM u)`, no, first, last).Scan(&staffID, &userID)
		if err != nil {
			t.Fatal(err)
		}
		if role != "" {
			if _, err := pool.Exec(ctx, `INSERT INTO offering.section_instructors (section_id, staff_id, role) VALUES ($1, $2, $3)`,
				f.section, staffID, role); err != nil {
				t.Fatal(err)
			}
		}
		return userID
	}
	f.primary = staff("P1", "Ayşe", "Yılmaz", "PRIMARY")
	f.assistant = staff("P2", "Zehra", "Kurt", "ASSISTANT")
	f.other = staff("P3", "Mehmet", "Demir", "")
	f.head = staff("P4", "Selin", "Koç", "")
	student := func(no, status string) string {
		t.Helper()
		var userID string
		err := pool.QueryRow(ctx, `
			WITH p AS (INSERT INTO people.persons (first_name, last_name) VALUES ('Öğrenci', $1) RETURNING id),
			st AS (INSERT INTO people.students (person_id, student_no) SELECT id, $1 FROM p RETURNING id),
			sp AS (INSERT INTO enrollment.student_programs (student_id, program_id, admission_type, admission_year, admitted_on)
			       SELECT st.id, pr.id, 'OSYS', 2026, '2026-09-15' FROM st, org.programs pr RETURNING id),
			e AS (INSERT INTO enrollment.enrollments (student_program_id, section_id, term_id, course_id, attempt_no, ects_snapshot, credit_snapshot, status)
			      SELECT sp.id, s.id, o.term_id, o.course_id, 1, 6, 5, $2
			      FROM sp, offering.sections s JOIN offering.course_offerings o ON o.id = s.offering_id RETURNING id)
			INSERT INTO iam.users (person_id, username, email, password_hash)
			SELECT id, $1, $1 || '@ogrenci.test', 'x' FROM p WHERE EXISTS (SELECT 1 FROM e) RETURNING id`, no, status).Scan(&userID)
		if err != nil {
			t.Fatal(err)
		}
		return userID
	}
	f.student = student("26290101", "ENROLLED")
	f.dropped = student("26290102", "DROPPED")
	f.stranger = staff("P9", "Yabancı", "Kişi", "")
	return f
}

type server struct {
	t    *testing.T
	pool *pgxpool.Pool
	h    http.Handler
	fx   fixture
	fs   *files.Service
	now  *time.Time
}

func newServer(t *testing.T) *server {
	t.Helper()
	pool := dbtest.New(t)
	store, _ := s3test.New(t)
	fx := seed(t, pool)
	now := time.Now()
	clock := func() time.Time { return now }
	fs := files.NewService(pool, store, 1<<20, clock)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	authenticate := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(authn.WithPrincipal(r.Context(), authn.Principal{UserID: r.Header.Get("X-Test-User")})))
		})
	}
	resolver := grants{fx.head: {{Permission: "section:manage", ScopeType: authz.ScopeDepartment, ScopeID: fx.department}}}
	mux := http.NewServeMux()
	rt := authz.NewRouter(mux, authenticate, resolver, logger)
	files.NewHandler(fs, logger).Register(rt)
	lms.NewHandler(lms.NewService(pool, fs, clock), logger).Register(rt)
	return &server{t: t, pool: pool, h: mux, fx: fx, fs: fs, now: &now}
}

type result struct {
	status int
	header http.Header
	body   map[string]any
	raw    string
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

func (s *server) upload(user, name, content string) string {
	s.t.Helper()
	f, err := s.fs.Upload(context.Background(), user, name, strings.NewReader(content))
	if err != nil {
		s.t.Fatal(err)
	}
	return f.ID
}

func topics(res result) []map[string]any {
	var out []map[string]any
	for _, t := range res.body["topics"].([]any) {
		out = append(out, t.(map[string]any))
	}
	return out
}

func activities(t map[string]any) []map[string]any {
	var out []map[string]any
	for _, a := range t["activities"].([]any) {
		out = append(out, a.(map[string]any))
	}
	return out
}

func TestSpaceMembershipAndContent(t *testing.T) {
	s := newServer(t)
	fx := s.fx
	space := "/api/v1/spaces/" + fx.section

	// Üyelik türetilir: kesin kayıtlı öğrenci ve öğretim elemanları görür; bırakan, başka
	// öğretim elemanı ve yabancı göremez; bölümü yöneten salt okunur görür.
	for user, want := range map[string]any{fx.primary: "PRIMARY", fx.assistant: "ASSISTANT", fx.student: "STUDENT", fx.head: "VIEWER",
		fx.dropped: 404.0, fx.other: 404.0, fx.stranger: 404.0} {
		res := s.do("GET", space, user, nil, nil)
		if role, ok := want.(string); ok {
			if res.status != 200 || res.body["role"] != role {
				t.Errorf("%s: %d %s", role, res.status, res.raw)
			}
		} else if res.status != 404 {
			t.Errorf("üye olmayan %s: %d", user, res.status)
		}
	}
	res := s.do("GET", "/api/v1/me/spaces", fx.student, nil, nil)
	if items := res.body["items"].([]any); len(items) != 1 || items[0].(map[string]any)["role"] != "STUDENT" ||
		len(items[0].(map[string]any)["instructors"].([]any)) != 2 {
		t.Errorf("derslerim: %s", res.raw)
	}
	if res := s.do("GET", "/api/v1/me/spaces", fx.dropped, nil, nil); len(res.body["items"].([]any)) != 0 {
		t.Errorf("bırakan öğrencinin dersleri: %s", res.raw)
	}

	// Düzenleme öğretim elemanlarınındır; ilk içerikle "Genel" bölümü oluşur.
	if res := s.do("POST", space+"/topics", fx.student, nil, map[string]any{"title": "Hafta 1"}); res.status != 403 {
		t.Errorf("öğrenci bölüm ekleyemez: %d", res.status)
	}
	if res := s.do("POST", space+"/topics", fx.head, nil, map[string]any{"title": "Hafta 1"}); res.status != 403 {
		t.Errorf("izleyici bölüm ekleyemez: %d", res.status)
	}
	res = s.do("POST", space+"/topics", fx.assistant, nil, map[string]any{"title": "Hafta 1", "summary_html": `<p>Giriş <script>alert(1)</script><a href="https://go.dev">Go</a></p>`})
	if res.status != 201 || len(topics(res)) != 2 || topics(res)[0]["title"] != "Genel" {
		t.Fatalf("bölüm: %d %s", res.status, res.raw)
	}
	week1 := topics(res)[1]
	if sum := week1["summary_html"].(string); strings.Contains(sum, "script") || !strings.Contains(sum, "noopener") ||
		!strings.Contains(sum, `target="_blank"`) {
		t.Errorf("HTML temizlenmeli: %s", sum)
	}
	week1ID := week1["id"].(string)
	hidden := s.do("POST", space+"/topics", fx.primary, nil, map[string]any{"title": "Taslak hafta", "is_visible": false})
	future := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
	later := s.do("POST", space+"/topics", fx.primary, nil, map[string]any{"title": "Hafta 2", "available_from": future})
	if hidden.status != 201 || later.status != 201 {
		t.Fatalf("bölümler: %s %s", hidden.raw, later.raw)
	}
	week2ID := topics(later)[3]["id"].(string)

	// Etkinlikler: dosya (yükleyenin dosyası bağlanır), klasör, bağlantı, sayfa, etiket.
	acts := "/api/v1/topics/" + week1ID + "/activities"
	pdfID := s.upload(fx.primary, "ders1.pdf", "%PDF-1.4 ders notu")
	if res := s.do("POST", acts, fx.assistant, nil, map[string]any{"type": "RESOURCE", "title": "Ders notu", "file_ids": []string{pdfID}}); res.status != 400 {
		t.Errorf("başkasının dosyası bağlanamaz: %d %s", res.status, res.raw)
	}
	res = s.do("POST", acts, fx.primary, nil, map[string]any{"type": "RESOURCE", "title": "Ders notu", "file_ids": []string{pdfID}})
	if res.status != 201 || res.body["files"].([]any)[0].(map[string]any)["name"] != "ders1.pdf" || res.header.Get("ETag") != `"1"` {
		t.Fatalf("dosya etkinliği: %d %s", res.status, res.raw)
	}
	resource := res.body["id"].(string)
	if res := s.do("POST", acts, fx.primary, nil, map[string]any{"type": "RESOURCE", "title": "Tekrar", "file_ids": []string{pdfID}}); res.status != 400 {
		t.Errorf("bağlı dosya ikinci kez bağlanamaz: %d", res.status)
	}
	f1, f2 := s.upload(fx.assistant, "örnek1.py", "print(1)\n"), s.upload(fx.assistant, "örnek2.py", "print(2)\n")
	folder := s.do("POST", acts, fx.assistant, nil, map[string]any{"type": "FOLDER", "title": "Örnek kodlar", "file_ids": []string{f1, f2}})
	link := s.do("POST", acts, fx.primary, nil, map[string]any{"type": "URL", "title": "Go Tour", "url": "https://go.dev/tour", "open_in_new_tab": false})
	page := s.do("POST", acts, fx.primary, nil, map[string]any{"type": "PAGE", "title": "Ders kuralları", "body_html": `<h2>Kurallar</h2><img src=x onerror=alert(1)>`})
	label := s.do("POST", acts, fx.primary, nil, map[string]any{"type": "LABEL", "title": "Duyuru", "description_html": "<b>Ara sınav 9 Kasım'da</b>"})
	hiddenAct := s.do("POST", acts, fx.primary, nil, map[string]any{"type": "LABEL", "title": "Gizli not", "is_visible": false})
	for name, r := range map[string]result{"klasör": folder, "bağlantı": link, "sayfa": page, "etiket": label, "gizli": hiddenAct} {
		if r.status != 201 {
			t.Fatalf("%s: %d %s", name, r.status, r.raw)
		}
	}
	if body := page.body["body_html"].(string); strings.Contains(body, "onerror") || !strings.Contains(body, "<h2>Kurallar</h2>") {
		t.Errorf("sayfa temizlenmeli: %s", body)
	}
	for name, body := range map[string]map[string]any{
		"tür":        {"type": "QUIZ", "title": "x"},
		"bağlantı":   {"type": "URL", "title": "x", "url": "javascript:alert(1)"},
		"klasörsüz":  {"type": "FOLDER", "title": "x", "file_ids": []string{}},
		"başlıksız":  {"type": "LABEL", "title": ""},
		"tarih sıra": {"type": "LABEL", "title": "x", "available_from": future, "available_until": time.Now().UTC().Format(time.RFC3339)},
	} {
		if res := s.do("POST", acts, fx.primary, nil, body); res.status != 400 {
			t.Errorf("geçersiz %s: %d %s", name, res.status, res.raw)
		}
	}

	// Öğrenci: gizli bölüm ve etkinlik görünmez; açılmamış bölümün sadece adı.
	res = s.do("GET", space, fx.student, nil, nil)
	ts := topics(res)
	if len(ts) != 3 || ts[1]["title"] != "Hafta 1" || ts[2]["availability"] != "NOT_YET" || len(activities(ts[2])) != 0 {
		t.Fatalf("öğrencinin görünümü: %s", res.raw)
	}
	if as := activities(ts[1]); len(as) != 5 || as[2]["url"] != "https://go.dev/tour" || as[2]["open_in_new_tab"] != false {
		t.Errorf("öğrencinin etkinlikleri: %v", as)
	}
	editor := topics(s.do("GET", space, fx.assistant, nil, nil))
	if len(editor) != 4 || len(activities(editor[1])) != 6 {
		t.Errorf("öğretim elemanı her şeyi görür: %v", editor)
	}

	// Dosya indirme: üye ve açık etkinlik; gizli etkinliğe 404.
	fileURL := fmt.Sprintf("/api/v1/activities/%s/files/%s/download-url", resource, pdfID)
	res = s.do("GET", fileURL, fx.student, nil, nil)
	if res.status != 200 {
		t.Fatalf("öğrenci indirir: %d %s", res.status, res.raw)
	}
	got, err := http.Get(res.body["url"].(string))
	if err != nil || got.StatusCode != 200 {
		t.Fatalf("indirme: %v", err)
	}
	if res := s.do("GET", fileURL, fx.dropped, nil, nil); res.status != 404 {
		t.Errorf("bırakan öğrenci indiremez: %d", res.status)
	}
	if res := s.do("GET", "/api/v1/activities/"+hiddenAct.body["id"].(string), fx.student, nil, nil); res.status != 404 {
		t.Errorf("gizli etkinlik: %d", res.status)
	}
	if res := s.do("GET", "/api/v1/activities/"+page.body["id"].(string), fx.student, nil, nil); res.status != 200 || res.body["body_html"] == nil {
		t.Errorf("sayfa ayrıntısı: %d %s", res.status, res.raw)
	}

	// Güncelleme: iyimser kilit; klasörden çıkarılan dosya serbest kalır, yenisi bağlanır.
	folderID := folder.body["id"].(string)
	f3 := s.upload(fx.primary, "örnek3.py", "print(3)\n")
	update := map[string]any{"type": "FOLDER", "title": "Örnek kodlar (güncel)", "file_ids": []string{f2, f3}}
	if res := s.do("PUT", "/api/v1/activities/"+folderID, fx.primary, nil, update); res.status != 428 {
		t.Errorf("If-Match'siz: %d", res.status)
	}
	res = s.do("PUT", "/api/v1/activities/"+folderID, fx.primary, map[string]string{"If-Match": `"1"`}, update)
	if res.status != 200 || len(res.body["files"].([]any)) != 2 || res.body["version"] != 2.0 {
		t.Fatalf("klasör güncelleme: %d %s", res.status, res.raw)
	}
	if res := s.do("PUT", "/api/v1/activities/"+folderID, fx.primary, map[string]string{"If-Match": `"1"`}, update); res.status != 412 {
		t.Errorf("eski sürüm: %d", res.status)
	}
	if f, err := s.fs.Get(context.Background(), s.pool, f1); err != nil || f.AttachedAt != nil {
		t.Error("çıkarılan dosya serbest kalmalı")
	}

	// Sıralama: bütün bölüm ve etkinlikler birer kez; etkinlik bölümler arası taşınır.
	editor = topics(s.do("GET", space, fx.primary, nil, nil))
	var layout []map[string]any
	for i := len(editor) - 1; i >= 0; i-- {
		var ids []string
		for _, a := range activities(editor[i]) {
			ids = append(ids, a["id"].(string))
		}
		layout = append(layout, map[string]any{"id": editor[i]["id"], "activity_ids": ids})
	}
	// Etiketi Hafta 2'ye taşı.
	moved := label.body["id"].(string)
	for _, l := range layout {
		ids := l["activity_ids"].([]string)
		for j, id := range ids {
			if id == moved {
				l["activity_ids"] = append(ids[:j], ids[j+1:]...)
				break
			}
		}
		if l["id"] == week2ID {
			l["activity_ids"] = append(ids, moved)
		}
	}
	res = s.do("PUT", space+"/layout", fx.primary, nil, map[string]any{"topics": layout})
	if res.status != 200 || topics(res)[0]["id"] != week2ID || activities(topics(res)[0])[0]["id"] != moved {
		t.Fatalf("sıralama: %d %s", res.status, res.raw)
	}
	if res := s.do("PUT", space+"/layout", fx.primary, nil, map[string]any{"topics": layout[:1]}); res.status != 412 {
		t.Errorf("eksik sıralama: %d", res.status)
	}

	// Silme: etkinlik ve bölüm; dosyaları serbest kalır. Son bölüm silinemez.
	if res := s.do("DELETE", "/api/v1/activities/"+resource, fx.student, nil, nil); res.status != 403 {
		t.Errorf("öğrenci silemez: %d", res.status)
	}
	if res := s.do("DELETE", "/api/v1/activities/"+resource, fx.primary, nil, nil); res.status != 204 {
		t.Errorf("etkinlik silme: %d %s", res.status, res.raw)
	}
	if res := s.do("GET", fileURL, fx.primary, nil, nil); res.status != 404 {
		t.Errorf("silinen etkinliğin dosyası: %d", res.status)
	}
	for _, tp := range topics(s.do("GET", space, fx.primary, nil, nil)) {
		res := s.do("DELETE", "/api/v1/topics/"+tp["id"].(string), fx.primary, nil, nil)
		if tp["title"] == "Genel" {
			continue
		}
		if res.status != 204 {
			t.Errorf("bölüm silme %s: %d %s", tp["title"], res.status, res.raw)
		}
	}
	remaining := topics(s.do("GET", space, fx.primary, nil, nil))
	if len(remaining) != 1 {
		t.Fatalf("kalan bölüm: %v", remaining)
	}
	if res := s.do("DELETE", "/api/v1/topics/"+remaining[0]["id"].(string), fx.primary, nil, nil); res.status != 400 {
		t.Errorf("son bölüm silinemez: %d %s", res.status, res.raw)
	}
}
