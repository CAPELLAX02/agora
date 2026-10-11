package enrollment_test

import (
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/CAPELLAX02/agora/backend/internal/platform/redistest"
)

// world, testin organizasyonu ve kişileridir:
//
//	Mühendislik (MUH): Bilgisayar (BIL) ve Elektrik (EEM) bölümleri, birer lisans programı
//	Fen (FEN): Fizik (FIZ) bölümü ve programı
type world struct {
	t    *testing.T
	pool *pgxpool.Pool
	h    http.Handler

	muh, bil, eem, fen                 string
	bilProgram, eemProgram, fizProgram string

	head, facultyRegistrar, advisor, notAdvisor, otherAdvisor string // kullanıcı kimlikleri
	advisorStaff, notAdvisorStaff, otherAdvisorStaff          string // personel kimlikleri
	student1, student2                                        string // öğrenci kimlikleri
	student1User, student2User                                string
}

func newWorld(t *testing.T) *world {
	t.Helper()
	ctx := context.Background()
	w := &world{t: t, pool: dbtest.New(t)}

	err := w.pool.QueryRow(ctx, `
		WITH m AS (INSERT INTO org.faculties (code, name_tr, name_en, unit_type) VALUES ('MUH', 'Mühendislik Fakültesi', 'Engineering', 'FACULTY') RETURNING id),
		     f AS (INSERT INTO org.faculties (code, name_tr, name_en, unit_type) VALUES ('FEN', 'Fen Fakültesi', 'Science', 'FACULTY') RETURNING id),
		     b AS (INSERT INTO org.departments (faculty_id, code, name_tr, name_en) SELECT id, 'BIL', 'Bilgisayar Mühendisliği', 'CE' FROM m RETURNING id),
		     e AS (INSERT INTO org.departments (faculty_id, code, name_tr, name_en) SELECT id, 'EEM', 'Elektrik-Elektronik Mühendisliği', 'EEE' FROM m RETURNING id),
		     z AS (INSERT INTO org.departments (faculty_id, code, name_tr, name_en) SELECT id, 'FIZ', 'Fizik', 'Physics' FROM f RETURNING id)
		SELECT (SELECT id FROM m), (SELECT id FROM b), (SELECT id FROM e), (SELECT id FROM f)`,
	).Scan(&w.muh, &w.bil, &w.eem, &w.fen)
	if err != nil {
		t.Fatal(err)
	}
	w.bilProgram = w.program("BIL-L", w.bil)
	w.eemProgram = w.program("EEM-L", w.eem)
	var fiz string
	_ = w.pool.QueryRow(ctx, `SELECT id FROM org.departments WHERE code = 'FIZ'`).Scan(&fiz)
	w.fizProgram = w.program("FIZ-L", fiz)

	w.head, _ = w.staff("P10001", "DEPARTMENT_HEAD", iam.ScopeDepartment, w.bil)
	w.facultyRegistrar, _ = w.staff("P20001", "FACULTY_REGISTRAR", iam.ScopeFaculty, w.muh)
	w.advisor, w.advisorStaff = w.staff("P10002", "ADVISOR", iam.ScopeDepartment, w.bil)
	w.otherAdvisor, w.otherAdvisorStaff = w.staff("P10003", "ADVISOR", iam.ScopeDepartment, w.bil)
	w.notAdvisor, w.notAdvisorStaff = w.staff("P10004", "INSTRUCTOR", iam.ScopeDepartment, w.bil)

	w.student1, w.student1User = w.newStudent("22290001")
	w.student2, w.student2User = w.newStudent("22290002")

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	resolver := iam.NewPermissionResolver(w.pool, redistest.New(t), time.Hour, logger)
	authenticate := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(rw, r.WithContext(authn.WithPrincipal(r.Context(), authn.Principal{UserID: r.Header.Get("X-Test-User")})))
		})
	}
	mux := http.NewServeMux()
	enrollment.NewHandler(enrollment.NewRepository(w.pool), enrollment.NewService(w.pool), logger).
		Register(authz.NewRouter(mux, authenticate, resolver, logger))
	w.h = mux
	return w
}

func (w *world) program(code, departmentID string) string {
	w.t.Helper()
	var id string
	err := w.pool.QueryRow(context.Background(), `
		INSERT INTO org.programs (department_id, code, name_tr, name_en, degree_level, language, education_type,
		                          duration_semesters, max_duration_years, total_ects_required)
		VALUES ($1, $2, $2 || ' Lisans', $2, 'BACHELOR', 'TR', 'DAYTIME', 8, 7, 240) RETURNING id`,
		departmentID, code).Scan(&id)
	if err != nil {
		w.t.Fatal(err)
	}
	return id
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

// staff, akademik personel ve hesabı oluşturur, rolü atar.
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

func (w *world) newStudent(no string) (studentID, userID string) {
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
	_ = w.pool.QueryRow(ctx, `SELECT id FROM people.students WHERE student_no = $1`, no).Scan(&studentID)
	return studentID, userID
}

type res struct {
	status int
	body   map[string]any
	raw    string
}

func (w *world) do(method, path, user string, body any) res {
	w.t.Helper()
	return w.doWith(method, path, user, body, nil)
}

func (w *world) doWith(method, path, user string, body any, headers map[string]string) res {
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
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	w.h.ServeHTTP(rec, req)
	out := res{status: rec.Code, raw: rec.Body.String()}
	_ = json.Unmarshal(rec.Body.Bytes(), &out.body)
	return out
}

func (r res) items() []any {
	items, _ := r.body["items"].([]any)
	return items
}

func (w *world) enroll(user, studentID, programID string) res {
	w.t.Helper()
	return w.do("POST", "/api/v1/students/"+studentID+"/programs", user, map[string]any{
		"program_id": programID, "admission_type": "OSYS", "admission_year": 2022, "admitted_on": "2022-09-15",
	})
}

func TestEnrollmentAndAdvisors(t *testing.T) {
	w := newWorld(t)

	// --- Program kaydı: fakülte öğrenci işleri sadece kendi fakültesinde.
	r := w.enroll(w.facultyRegistrar, w.student1, w.bilProgram)
	if r.status != http.StatusCreated {
		t.Fatalf("kayıt: %d %s", r.status, r.raw)
	}
	sp1 := r.body["id"].(string)
	if r := w.enroll(w.facultyRegistrar, w.student2, w.eemProgram); r.status != http.StatusCreated {
		t.Fatalf("ikinci kayıt: %d %s", r.status, r.raw)
	}
	if r := w.enroll(w.facultyRegistrar, w.student1, w.fizProgram); r.status != http.StatusForbidden {
		t.Errorf("başka fakültenin programı: %d", r.status)
	}
	if r := w.enroll(w.facultyRegistrar, w.student1, w.bilProgram); r.status != 409 {
		t.Errorf("aynı programa ikinci kayıt: %d", r.status)
	}
	if r := w.enroll(w.head, w.student1, w.eemProgram); r.status != http.StatusForbidden {
		t.Errorf("bölüm başkanının kayıt yetkisi yok: %d", r.status)
	}

	// --- Liste: herkes sadece kendi kapsamını görür.
	if n := len(w.do("GET", "/api/v1/students", w.facultyRegistrar, nil).items()); n != 2 {
		t.Errorf("fakülte öğrenci işleri %d kayıt gördü, 2 olmalı", n)
	}
	if items := w.do("GET", "/api/v1/students", w.head, nil).items(); len(items) != 1 {
		t.Errorf("bölüm başkanı %d kayıt gördü, sadece kendi bölümü (1) olmalı", len(items))
	}
	if n := len(w.do("GET", "/api/v1/students", w.advisor, nil).items()); n != 0 {
		t.Errorf("danışman rolü bölümdeki bütün öğrencileri görmemeli, %d gördü", n)
	}

	// --- Danışman atama
	if r := w.do("PUT", "/api/v1/student-programs/"+sp1+"/advisor", w.head,
		map[string]string{"staff_id": w.notAdvisorStaff, "reason": "x"}); r.status != 400 {
		t.Errorf("danışman rolü olmayan biri atanamamalı: %d %s", r.status, r.raw)
	}
	r = w.do("PUT", "/api/v1/student-programs/"+sp1+"/advisor", w.head,
		map[string]string{"staff_id": w.advisorStaff, "reason": "Birinci sınıf danışman dağılımı"})
	if r.status != 200 {
		t.Fatalf("danışman atama: %d %s", r.status, r.raw)
	}
	if adv := r.body["advisor"].(map[string]any); adv["staff_id"] != w.advisorStaff || adv["title"] != "Prof. Dr." {
		t.Errorf("danışman = %v", adv)
	}
	var sp2 string
	_ = w.pool.QueryRow(context.Background(), `SELECT id FROM enrollment.student_programs WHERE student_id = $1`, w.student2).Scan(&sp2)
	if r := w.do("PUT", "/api/v1/student-programs/"+sp2+"/advisor", w.head,
		map[string]string{"staff_id": w.advisorStaff, "reason": "x"}); r.status != http.StatusForbidden {
		t.Errorf("başka bölümün öğrencisine atama: %d", r.status)
	}

	// --- Erişim: öğrenci kendini, danışman danışmanı olduğu öğrenciyi görür.
	for name, c := range map[string]struct {
		user, student string
		want          int
	}{
		"danışman kendi öğrencisi":   {w.advisor, w.student1, 200},
		"danışman başka öğrenci":     {w.advisor, w.student2, 404},
		"öğrenci kendisi":            {w.student1User, w.student1, 200},
		"öğrenci başka öğrenci":      {w.student1User, w.student2, 404},
		"bölüm başkanı kendi bölümü": {w.head, w.student1, 200},
		"bölüm başkanı başka bölüm":  {w.head, w.student2, 404},
		"diğer danışman (atanmamış)": {w.otherAdvisor, w.student1, 404},
	} {
		if r := w.do("GET", "/api/v1/students/"+c.student, c.user, nil); r.status != c.want {
			t.Errorf("%s: %d, want %d", name, r.status, c.want)
		}
	}

	if items := w.do("GET", "/api/v1/me/advisees", w.advisor, nil).items(); len(items) != 1 {
		t.Errorf("danışmanlıklarım = %d", len(items))
	}
	mine := w.do("GET", "/api/v1/me/programs", w.student1User, nil).items()
	if len(mine) != 1 || mine[0].(map[string]any)["advisor"] == nil {
		t.Errorf("programlarım = %v", mine)
	}
	if items := w.do("GET", "/api/v1/me/programs", w.advisor, nil).items(); len(items) != 0 {
		t.Errorf("öğrenci olmayanın programları boş olmalı: %d", len(items))
	}

	// --- Danışman değişikliği geçmişte kalır.
	if r := w.do("PUT", "/api/v1/student-programs/"+sp1+"/advisor", w.head,
		map[string]string{"staff_id": w.otherAdvisorStaff, "reason": "Danışman izinde"}); r.status != 200 {
		t.Fatalf("danışman değişikliği: %d %s", r.status, r.raw)
	}
	history := w.do("GET", "/api/v1/student-programs/"+sp1+"/advisors", w.head, nil).items()
	if len(history) != 2 || history[0].(map[string]any)["until"] != nil || history[1].(map[string]any)["until"] == nil {
		t.Errorf("danışman geçmişi = %v", history)
	}
	if r := w.do("GET", "/api/v1/students/"+w.student1, w.advisor, nil); r.status != 404 {
		t.Errorf("eski danışman artık öğrenciyi görmemeli: %d", r.status)
	}

	eligible := w.do("GET", "/api/v1/departments/"+w.bil+"/advisors", w.head, nil).items()
	if len(eligible) != 2 {
		t.Errorf("bölümün danışmanları = %v", eligible)
	}
	for _, e := range eligible {
		e := e.(map[string]any)
		if e["staff_id"] == w.otherAdvisorStaff && e["active_count"].(float64) != 1 {
			t.Errorf("danışmanlık sayısı = %v", e["active_count"])
		}
	}

	// --- Mezun olmuş kayda danışman atanmaz.
	if _, err := w.pool.Exec(context.Background(),
		`UPDATE enrollment.student_programs SET status = 'GRADUATED', graduated_on = '2026-07-01' WHERE id = $1`, sp1); err != nil {
		t.Fatal(err)
	}
	if r := w.do("PUT", "/api/v1/student-programs/"+sp1+"/advisor", w.head,
		map[string]string{"staff_id": w.advisorStaff, "reason": "x"}); r.status != 409 {
		t.Errorf("mezun kayıt: %d %s", r.status, r.raw)
	}
}

func TestCreateProgramValidation(t *testing.T) {
	w := newWorld(t)
	r := w.do("POST", "/api/v1/students/"+w.student1+"/programs", w.facultyRegistrar, map[string]any{
		"program_id": w.bilProgram, "kind": "UYDURMA", "admission_type": "X", "admission_year": 1900,
		"admitted_on": "15.09.2022", "status": "GRADUATED", "class_level": 9,
	})
	if r.status != 400 {
		t.Fatalf("durum = %d", r.status)
	}
	for _, f := range []string{"kind", "admission_type", "admission_year", "admitted_on", "status", "class_level"} {
		if !strings.Contains(r.raw, `"field":"`+f+`"`) {
			t.Errorf("%s alan hatası yok: %s", f, r.raw)
		}
	}
}

func TestHoldsAndStatus(t *testing.T) {
	w := newWorld(t)
	r := w.enroll(w.facultyRegistrar, w.student1, w.bilProgram)
	if r.status != http.StatusCreated {
		t.Fatalf("kayıt: %d %s", r.status, r.raw)
	}
	sp1 := r.body["id"].(string)
	if r.body["version"].(float64) != 1 || r.body["normal_semesters"].(float64) != 8 || r.body["max_semesters"].(float64) != 14 {
		t.Errorf("süre alanları = %v", r.body)
	}
	if r := w.do("PUT", "/api/v1/student-programs/"+sp1+"/advisor", w.head,
		map[string]string{"staff_id": w.advisorStaff, "reason": "Dağılım"}); r.status != 200 {
		t.Fatalf("danışman: %d %s", r.status, r.raw)
	}
	holds := "/api/v1/student-programs/" + sp1 + "/holds"

	// --- Engel koyma: sadece hold:manage kapsamı.
	if r := w.do("POST", holds, w.head, map[string]any{"hold_type": "TUITION_DEBT", "reason": "x"}); r.status != http.StatusForbidden {
		t.Errorf("bölüm başkanı engel koyamaz: %d", r.status)
	}
	r = w.do("POST", holds, w.facultyRegistrar, map[string]any{"hold_type": "TUITION_DEBT", "reason": "2026 güz katkı payı ödenmedi"})
	if r.status != http.StatusCreated {
		t.Fatalf("engel: %d %s", r.status, r.raw)
	}
	holdID := r.body["id"].(string)
	if b := r.body["blocks"].([]any); len(b) != 2 || b[0] != "REGISTRATION" || b[1] != "DOCUMENT" {
		t.Errorf("varsayılan engellenen işlemler = %v", b)
	}
	if r.body["placed_by"] != "AdP20001 SoyadP20001" || r.body["active"] != true {
		t.Errorf("engel = %v", r.body)
	}
	if r := w.do("POST", holds, w.facultyRegistrar, map[string]any{"hold_type": "TUITION_DEBT", "reason": "y"}); r.status != 409 || r.body["code"] != "HOLD_EXISTS" {
		t.Errorf("aynı türde ikinci engel: %d %s", r.status, r.raw)
	}
	if r := w.do("POST", holds, w.facultyRegistrar, map[string]any{"hold_type": "LIBRARY_DEBT", "blocks": []string{"X"}, "reason": "y"}); r.status != 400 {
		t.Errorf("geçersiz işlem: %d", r.status)
	}
	if r := w.do("POST", holds, w.facultyRegistrar, map[string]any{"hold_type": "LIBRARY_DEBT", "blocks": []string{"GRADUATION"}, "reason": "Kitap iade edilmedi"}); r.status != 201 {
		t.Errorf("ikinci tür: %d %s", r.status, r.raw)
	}

	// --- Okuma: öğrenci kendini, danışman öğrencisini, mali işler kapsamını görür.
	if n := len(w.do("GET", holds, w.student1User, nil).items()); n != 2 {
		t.Errorf("öğrenci kendi engelleri = %d", n)
	}
	if n := len(w.do("GET", holds, w.advisor, nil).items()); n != 2 {
		t.Errorf("danışman = %d", n)
	}
	if r := w.do("GET", holds, w.student2User, nil); r.status != 404 {
		t.Errorf("başka öğrenci: %d", r.status)
	}
	if n := len(w.do("GET", "/api/v1/holds", w.facultyRegistrar, nil).items()); n != 2 {
		t.Errorf("kapsamdaki engeller = %d", n)
	}
	if n := len(w.do("GET", "/api/v1/holds?type=TUITION_DEBT&q=2229", w.facultyRegistrar, nil).items()); n != 1 {
		t.Errorf("filtreli engeller = %d", n)
	}
	if r := w.do("GET", "/api/v1/holds", w.head, nil); r.status != http.StatusForbidden {
		t.Errorf("yetkisiz liste: %d", r.status)
	}

	// --- Kaldırma
	release := "/api/v1/holds/" + holdID + "/release"
	if r := w.do("POST", release, w.facultyRegistrar, map[string]string{"reason": " "}); r.status != 400 {
		t.Errorf("gerekçesiz kaldırma: %d", r.status)
	}
	r = w.do("POST", release, w.facultyRegistrar, map[string]string{"reason": "Ödeme dekontu geldi"})
	if r.status != 200 || r.body["active"] != false || r.body["release_reason"] != "Ödeme dekontu geldi" {
		t.Fatalf("kaldırma: %d %s", r.status, r.raw)
	}
	if r := w.do("POST", release, w.facultyRegistrar, map[string]string{"reason": "tekrar"}); r.status != 409 {
		t.Errorf("ikinci kaldırma: %d", r.status)
	}
	if n := len(w.do("GET", "/api/v1/holds?active=false", w.facultyRegistrar, nil).items()); n != 1 {
		t.Errorf("kaldırılmış engeller = %d", n)
	}
	if items := w.do("GET", holds, w.student1User, nil).items(); items[0].(map[string]any)["active"] != true {
		t.Errorf("aktif engel önce gelmeli: %v", items)
	}

	// --- Durum geçişleri
	status := "/api/v1/student-programs/" + sp1 + "/status"
	frozen := map[string]any{"status": "FROZEN", "reason": "Sağlık raporu", "decision_ref": "YK 2026/41"}
	if r := w.do("POST", status, w.facultyRegistrar, frozen); r.status != 428 {
		t.Errorf("If-Match olmadan: %d", r.status)
	}
	if r := w.doWith("POST", status, w.facultyRegistrar, frozen, map[string]string{"If-Match": `"7"`}); r.status != 412 {
		t.Errorf("eski sürüm: %d", r.status)
	}
	if r := w.doWith("POST", status, w.head, frozen, map[string]string{"If-Match": `"1"`}); r.status != 403 {
		t.Errorf("bölüm başkanı durum değiştiremez: %d", r.status)
	}
	if r := w.doWith("POST", status, w.facultyRegistrar, map[string]any{"status": "GRADUATED", "reason": "x"},
		map[string]string{"If-Match": `"1"`}); r.status != 400 || !strings.Contains(r.raw, "graduated_on") {
		t.Errorf("mezuniyet tarihi olmadan: %d %s", r.status, r.raw)
	}
	r = w.doWith("POST", status, w.facultyRegistrar, frozen, map[string]string{"If-Match": `"1"`})
	if r.status != 200 || r.body["status"] != "FROZEN" || r.body["version"].(float64) != 2 {
		t.Fatalf("dondurma: %d %s", r.status, r.raw)
	}
	if r := w.doWith("POST", status, w.facultyRegistrar, map[string]any{"status": "GRADUATED", "reason": "x", "graduated_on": "2026-07-01"},
		map[string]string{"If-Match": `"2"`}); r.status != 409 || r.body["code"] != "INVALID_STATUS_TRANSITION" {
		t.Errorf("dondurulmuş kayıt mezun olamaz: %d %s", r.status, r.raw)
	}
	r = w.doWith("POST", status, w.facultyRegistrar, map[string]any{"status": "DISMISSED", "reason": "Azami süre doldu"},
		map[string]string{"If-Match": `"2"`})
	if r.status != 200 || r.body["advisor"] != nil {
		t.Fatalf("ilişik kesme danışmanlığı sonlandırmalı: %d %s", r.status, r.raw)
	}
	history := w.do("GET", "/api/v1/student-programs/"+sp1+"/status-history", w.student1User, nil).items()
	if len(history) != 3 {
		t.Fatalf("durum geçmişi = %v", history)
	}
	if h := history[1].(map[string]any); h["from_status"] != "ACTIVE" || h["to_status"] != "FROZEN" ||
		h["decision_ref"] != "YK 2026/41" || h["changed_by"] != "AdP20001 SoyadP20001" {
		t.Errorf("dondurma kaydı = %v", h)
	}
	if r := w.do("POST", holds, w.facultyRegistrar, map[string]any{"hold_type": "MISSING_DOCUMENT", "reason": "x"}); r.status != 409 {
		t.Errorf("ilişiği kesilmiş kayda engel: %d", r.status)
	}
}

func TestStatusTransitions(t *testing.T) {
	cases := []struct {
		from, to enrollment.Status
		ok       bool
	}{
		{enrollment.StatusPrep, enrollment.StatusActive, true},
		{enrollment.StatusPrep, enrollment.StatusFrozen, false},
		{enrollment.StatusActive, enrollment.StatusGraduated, true},
		{enrollment.StatusActive, enrollment.StatusPrep, false},
		{enrollment.StatusFrozen, enrollment.StatusActive, true},
		{enrollment.StatusSuspended, enrollment.StatusGraduated, false},
		{enrollment.StatusGraduated, enrollment.StatusActive, false},
		{enrollment.StatusDismissed, enrollment.StatusActive, false},
		{enrollment.StatusActive, enrollment.StatusActive, false},
	}
	for _, c := range cases {
		if got := c.from.CanBecome(c.to); got != c.ok {
			t.Errorf("%s → %s = %v, want %v", c.from, c.to, got, c.ok)
		}
	}
}
