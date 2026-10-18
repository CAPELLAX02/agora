package messaging_test

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

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/messaging"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
)

type noGrants struct{}

func (noGrants) Permissions(_ context.Context, userID string) (*authz.Permissions, error) {
	return authz.NewPermissions(userID, nil), nil
}

type fixture struct {
	section                    string
	teacher, advisor, other    string // personel
	ali, ece, dropped, outside string // öğrenciler
}

func seed(t *testing.T, pool *pgxpool.Pool) fixture {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		INSERT INTO org.faculties (code, name_tr, name_en, unit_type) VALUES ('MUH', 'Mühendislik Fakültesi', 'Engineering', 'FACULTY');
		INSERT INTO org.departments (faculty_id, code, name_tr, name_en) SELECT id, 'BIL', 'Bilgisayar Mühendisliği', 'CE' FROM org.faculties;
		INSERT INTO org.programs (department_id, code, name_tr, name_en, degree_level, language, education_type,
		                          duration_semesters, max_duration_years, total_ects_required)
			SELECT id, 'BIL-L', 'Bilgisayar Mühendisliği', 'CE', 'BACHELOR', 'EN', 'DAYTIME', 8, 7, 240 FROM org.departments;
		INSERT INTO academic.academic_years (start_year, starts_on, ends_on) VALUES (2026, '2026-09-01', '2027-08-31');
		INSERT INTO academic.terms (academic_year_id, term_type, code, starts_on, ends_on, status, is_current)
			SELECT id, 'FALL', '2026-FALL', '2026-09-21', '2027-01-31', 'ACTIVE', true FROM academic.academic_years;
		INSERT INTO curriculum.courses (code, owner_department_id, name_tr, name_en, theory_hours, national_credit, ects, language)
			SELECT 'COM1001', id, 'Programlama', 'Programming', 4, 5, 6, 'EN' FROM org.departments;
		INSERT INTO offering.course_offerings (term_id, course_id, department_id)
			SELECT t.id, c.id, d.id FROM academic.terms t, curriculum.courses c, org.departments d;
		INSERT INTO offering.sections (offering_id, section_code, capacity, language)
			SELECT id, '1', 60, 'EN' FROM offering.course_offerings;`); err != nil {
		t.Fatal(err)
	}
	var f fixture
	if err := pool.QueryRow(ctx, `SELECT id FROM offering.sections`).Scan(&f.section); err != nil {
		t.Fatal(err)
	}
	staff := func(no, first string) (userID, staffID string) {
		t.Helper()
		if err := pool.QueryRow(ctx, `
			WITH p AS (INSERT INTO people.persons (first_name, last_name) VALUES ($2, 'Hoca') RETURNING id),
			s AS (INSERT INTO people.staff (person_id, staff_no, staff_type) SELECT id, $1::text, 'ACADEMIC' FROM p RETURNING id),
			u AS (INSERT INTO iam.users (person_id, username, email, password_hash) SELECT id, $1::text, $1::text || '@agora.test', 'x' FROM p RETURNING id)
			SELECT (SELECT id FROM u), (SELECT id FROM s)`, no, first).Scan(&userID, &staffID); err != nil {
			t.Fatal(err)
		}
		return userID, staffID
	}
	var teacherStaff, advisorStaff string
	f.teacher, teacherStaff = staff("P1", "Ayşe")
	f.advisor, advisorStaff = staff("P2", "Cem")
	f.other, _ = staff("P3", "Selin")
	if _, err := pool.Exec(ctx, `INSERT INTO offering.section_instructors (section_id, staff_id, role) VALUES ($1, $2, 'PRIMARY')`,
		f.section, teacherStaff); err != nil {
		t.Fatal(err)
	}
	student := func(no, first, status string, enrolled bool) string {
		t.Helper()
		var userID, spID string
		if err := pool.QueryRow(ctx, `
			WITH p AS (INSERT INTO people.persons (first_name, last_name) VALUES ($2, 'Öğrenci') RETURNING id),
			st AS (INSERT INTO people.students (person_id, student_no) SELECT id, $1 FROM p RETURNING id),
			sp AS (INSERT INTO enrollment.student_programs (student_id, program_id, admission_type, admission_year, admitted_on)
			       SELECT st.id, pr.id, 'OSYS', 2026, '2026-09-15' FROM st, org.programs pr RETURNING id),
			u AS (INSERT INTO iam.users (person_id, username, email, password_hash) SELECT id, $1, $1 || '@ogrenci.test', 'x' FROM p RETURNING id)
			SELECT (SELECT id FROM u), (SELECT id FROM sp)`, no, first).Scan(&userID, &spID); err != nil {
			t.Fatal(err)
		}
		if enrolled {
			if _, err := pool.Exec(ctx, `
				INSERT INTO enrollment.enrollments (student_program_id, section_id, term_id, course_id, attempt_no, ects_snapshot, credit_snapshot, status)
				SELECT $1, s.id, o.term_id, o.course_id, 1, 6, 5, $2 FROM offering.sections s JOIN offering.course_offerings o ON o.id = s.offering_id`,
				spID, status); err != nil {
				t.Fatal(err)
			}
		}
		if no == "24000001" {
			if _, err := pool.Exec(ctx, `INSERT INTO enrollment.advisor_assignments (student_program_id, advisor_staff_id, valid_from)
				VALUES ($1, $2, '2026-09-01')`, spID, advisorStaff); err != nil {
				t.Fatal(err)
			}
		}
		return userID
	}
	f.ali = student("24000001", "Ali", "ENROLLED", true)
	f.ece = student("24000002", "Ece", "ENROLLED", true)
	f.dropped = student("24000003", "Can", "DROPPED", true)
	f.outside = student("24000004", "Deniz", "", false)
	return f
}

type server struct {
	t    *testing.T
	pool *pgxpool.Pool
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
	mux := http.NewServeMux()
	messaging.NewHandler(messaging.NewService(pool, time.Now), logger).Register(authz.NewRouter(mux, authenticate, noGrants{}, logger))
	return &server{t: t, pool: pool, h: mux, fx: fx}
}

type result struct {
	status int
	body   map[string]any
	raw    string
}

func (s *server) do(method, path, user string, body any) result {
	s.t.Helper()
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-User", user)
	rec := httptest.NewRecorder()
	s.h.ServeHTTP(rec, req)
	res := result{status: rec.Code, raw: rec.Body.String()}
	_ = json.Unmarshal(rec.Body.Bytes(), &res.body)
	return res
}

func items(res result) []map[string]any {
	out := []map[string]any{}
	for _, it := range res.body["items"].([]any) {
		out = append(out, it.(map[string]any))
	}
	return out
}

func (s *server) direct(from, to string) result {
	s.t.Helper()
	return s.do("POST", "/api/v1/conversations", from, map[string]any{"kind": "DIRECT", "user_id": to})
}

func (s *server) send(user, conversation, body string) string {
	s.t.Helper()
	res := s.do("POST", "/api/v1/conversations/"+conversation+"/messages", user, map[string]any{"body": body})
	if res.status != 201 {
		s.t.Fatalf("ileti: %d %s", res.status, res.raw)
	}
	return res.body["id"].(string)
}

func (s *server) unread(user string) float64 {
	s.t.Helper()
	return s.do("GET", "/api/v1/me/conversations/unread-count", user, nil).body["count"].(float64)
}

func names(list []map[string]any) []string {
	var out []string
	for _, p := range list {
		out = append(out, p["name"].(string))
	}
	slices.Sort(out)
	return out
}

func TestPolicyAndContacts(t *testing.T) {
	s := newServer(t)
	fx := s.fx
	// Öğrenci: şubesinin öğretim elemanı ve danışmanı.
	if got := names(items(s.do("GET", "/api/v1/messaging/contacts", fx.ali, nil))); !slices.Equal(got, []string{"Ayşe Hoca", "Cem Hoca"}) {
		t.Errorf("öğrencinin kişileri: %v", got)
	}
	if got := names(items(s.do("GET", "/api/v1/messaging/contacts?q=ayş", fx.ali, nil))); !slices.Equal(got, []string{"Ayşe Hoca"}) {
		t.Errorf("süzülen kişiler: %v", got)
	}
	// Personel: en az iki harfle herkes arasında arar.
	if got := items(s.do("GET", "/api/v1/messaging/contacts?q=a", fx.other, nil)); len(got) != 0 {
		t.Errorf("tek harfle arama: %v", got)
	}
	if got := names(items(s.do("GET", "/api/v1/messaging/contacts?q=öğrenci", fx.other, nil))); len(got) != 4 {
		t.Errorf("personelin araması: %v", got)
	}

	for _, c := range []struct {
		from, to string
		want     int
	}{
		{fx.ali, fx.teacher, 201}, {fx.ali, fx.advisor, 201}, {fx.ali, fx.ece, 403}, {fx.ali, fx.other, 403},
		{fx.other, fx.ali, 201}, {fx.teacher, fx.outside, 201}, {fx.ali, fx.ali, 403},
	} {
		if res := s.direct(c.from, c.to); res.status != c.want {
			t.Errorf("%s → %s: %d %s", c.from, c.to, res.status, res.raw)
		}
	}
	// Aynı iki kişi arasında tek konuşma.
	if a, b := s.direct(fx.ali, fx.teacher), s.direct(fx.teacher, fx.ali); a.body["id"] != b.body["id"] || a.body["peer"].(map[string]any)["name"] != "Ayşe Hoca" {
		t.Errorf("birebir konuşma tek olmalı: %s %s", a.raw, b.raw)
	}

	// Grubu personel kurar.
	if res := s.do("POST", "/api/v1/conversations", fx.ali, map[string]any{"kind": "GROUP", "title": "Proje", "member_ids": []string{fx.ece}}); res.status != 403 {
		t.Errorf("öğrenci grup kuramaz: %d", res.status)
	}
	res := s.do("POST", "/api/v1/conversations", fx.other, map[string]any{"kind": "GROUP", "title": "Kulüp", "member_ids": []string{fx.ali, fx.ece, fx.ali}})
	if res.status != 201 || res.body["is_moderator"] != true || res.body["member_count"] != 3.0 {
		t.Fatalf("grup: %d %s", res.status, res.raw)
	}
	group := res.body["id"].(string)
	if res := s.do("GET", "/api/v1/conversations/"+group, fx.ece, nil); res.status != 200 || res.body["is_moderator"] != false {
		t.Errorf("grup üyesi: %d %s", res.status, res.raw)
	}
	if res := s.do("GET", "/api/v1/conversations/"+group, fx.outside, nil); res.status != 404 {
		t.Errorf("üye olmayan: %d", res.status)
	}
}

func TestMessagesUnreadAndSectionGroup(t *testing.T) {
	s := newServer(t)
	fx := s.fx
	conv := s.direct(fx.ali, fx.teacher).body["id"].(string)
	first := s.send(fx.ali, conv, "Hocam, ödev hakkında bir sorum var.")
	if n := s.unread(fx.teacher); n != 1 {
		t.Errorf("öğretim elemanının okunmamışı: %v", n)
	}
	if n := s.unread(fx.ali); n != 0 {
		t.Errorf("gönderenin okunmamışı: %v", n)
	}
	list := items(s.do("GET", "/api/v1/me/conversations", fx.teacher, nil))
	var direct map[string]any
	for _, c := range list {
		if c["kind"] == "DIRECT" {
			direct = c
		}
	}
	if direct == nil || direct["unread"] != 1.0 || direct["peer"].(map[string]any)["name"] != "Ali Öğrenci" ||
		direct["last_message"].(map[string]any)["body"] != "Hocam, ödev hakkında bir sorum var." {
		t.Fatalf("liste: %v", list)
	}
	// Yanıt ve okundu.
	res := s.do("POST", "/api/v1/conversations/"+conv+"/messages", fx.teacher, map[string]any{"body": "  Buyurun.  ", "reply_to_id": first})
	if res.status != 201 || res.body["body"] != "Buyurun." || res.body["reply_to_id"] != first || res.body["mine"] != true {
		t.Fatalf("yanıt: %d %s", res.status, res.raw)
	}
	if n := s.unread(fx.teacher); n != 0 {
		t.Errorf("yazan konuşmayı okumuş sayılır: %v", n)
	}
	reply := res.body["id"].(string)
	if n := s.unread(fx.ali); n != 1 {
		t.Errorf("öğrencinin okunmamışı: %v", n)
	}
	if res := s.do("POST", "/api/v1/conversations/"+conv+"/read", fx.ali, map[string]any{"message_id": reply}); res.status != 204 {
		t.Fatalf("okundu: %d %s", res.status, res.raw)
	}
	if res := s.do("POST", "/api/v1/conversations/"+conv+"/read", fx.ali, map[string]any{"message_id": first}); res.status != 204 || s.unread(fx.ali) != 0 {
		t.Errorf("okundu imleci geri gitmemeli")
	}

	// Sayfalama: yeniden eskiye.
	for _, b := range []string{"üç", "dört", "beş"} {
		s.send(fx.ali, conv, b)
	}
	page := s.do("GET", "/api/v1/conversations/"+conv+"/messages?limit=2", fx.teacher, nil)
	got := items(page)
	if len(got) != 2 || got[0]["body"] != "beş" || page.body["has_more"] != true {
		t.Fatalf("ilk sayfa: %s", page.raw)
	}
	older := items(s.do("GET", "/api/v1/conversations/"+conv+"/messages?limit=10&before="+got[1]["id"].(string), fx.teacher, nil))
	if len(older) != 3 || older[0]["body"] != "üç" {
		t.Errorf("önceki sayfa: %v", older)
	}
	if res := s.do("GET", "/api/v1/conversations/"+conv+"/messages", fx.ece, nil); res.status != 404 {
		t.Errorf("başkasının konuşması: %d", res.status)
	}

	// Şube grubu: şubenin öğretim elemanı ve kaydı olan öğrenciler görür.
	var section string
	for _, c := range items(s.do("GET", "/api/v1/me/conversations", fx.ali, nil)) {
		if c["kind"] == "SECTION" {
			section = c["id"].(string)
			if c["title"] != "COM1001-1 · Programlama" {
				t.Errorf("şube grubunun adı: %v", c["title"])
			}
		}
	}
	if section == "" {
		t.Fatal("şube grubu listede yok")
	}
	for _, u := range []string{fx.dropped, fx.outside} {
		for _, c := range items(s.do("GET", "/api/v1/me/conversations", u, nil)) {
			if c["kind"] == "SECTION" {
				t.Errorf("şube dışı öğrenci şube grubunu görmemeli")
			}
		}
		if res := s.do("GET", "/api/v1/conversations/"+section, u, nil); res.status != 404 {
			t.Errorf("şube dışı erişim: %d", res.status)
		}
	}
	detail := s.do("GET", "/api/v1/conversations/"+section, fx.teacher, nil)
	if detail.body["is_moderator"] != true || detail.body["member_count"] != 3.0 {
		t.Errorf("şube grubu ayrıntısı: %s", detail.raw)
	}
	s.send(fx.ece, section, "Arkadaşlar, lab saati değişti mi?")
	if n := s.unread(fx.ali); n != 1 {
		t.Errorf("şube grubu okunmamışı: %v", n)
	}
	if res := s.do("POST", "/api/v1/conversations/"+section+"/messages", fx.ali, map[string]any{"body": "Evet", "reply_to_id": first}); res.status != 400 {
		t.Errorf("başka konuşmadaki iletiye yanıt: %d", res.status)
	}
	if res := s.do("POST", "/api/v1/conversations/"+section+"/messages", fx.ali, map[string]any{"body": "   "}); res.status != 400 {
		t.Errorf("boş ileti: %d", res.status)
	}
}

// Mesaj bildirimleri varsayılan kapalıdır; tercihle açılır, sessize alan almaz.
func TestMessageNotifications(t *testing.T) {
	s := newServer(t)
	fx := s.fx
	ctx := context.Background()
	conv := s.direct(fx.ali, fx.teacher).body["id"].(string)
	s.send(fx.ali, conv, "Merhaba")
	count := func(user string) int {
		var n int
		if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM communication.notifications WHERE user_id = $1 AND type_code = 'message.received'`, user).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(fx.teacher); n != 0 {
		t.Errorf("varsayılan kapalı: %d", n)
	}
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO communication.notification_preferences (user_id, type_code, channel, enabled) VALUES ($1, 'message.received', 'IN_APP', true)`,
		fx.teacher); err != nil {
		t.Fatal(err)
	}
	s.send(fx.ali, conv, "Sorum şu")
	if n := count(fx.teacher); n != 1 {
		t.Errorf("tercihle açılınca: %d", n)
	}
	if _, err := s.pool.Exec(ctx, `
		UPDATE communication.conversation_members SET muted_until = now() + interval '1 day' WHERE conversation_id = $1 AND user_id = $2`,
		conv, fx.teacher); err != nil {
		t.Fatal(err)
	}
	s.send(fx.ali, conv, "Acil değil")
	if n := count(fx.teacher); n != 1 {
		t.Errorf("sessize alan bildirim almamalı: %d", n)
	}
	if n := s.unread(fx.teacher); n != 0 {
		t.Errorf("sessize alınan konuşma toplam okunmamışa girmez: %v", n)
	}
}
