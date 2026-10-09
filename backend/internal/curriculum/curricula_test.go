package curriculum_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/curriculum"
	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
)

// seedStudent, programa verilen yılda girmiş bir öğrenci ve kullanıcı hesabı oluşturur.
func seedStudent(t *testing.T, pool *pgxpool.Pool, programID, studentNo string, admissionYear int) (userID, studentProgramID string) {
	t.Helper()
	err := pool.QueryRow(context.Background(), `
		WITH p AS (
			INSERT INTO people.persons (first_name, last_name) VALUES ('Deneme', 'Öğrenci') RETURNING id
		), u AS (
			INSERT INTO iam.users (person_id, username, email, password_hash)
			SELECT id, $1::text, $1::text || '@ogrenci.test', 'x' FROM p RETURNING id
		), s AS (
			INSERT INTO people.students (person_id, student_no) SELECT id, $1::text FROM p RETURNING id
		), sp AS (
			INSERT INTO enrollment.student_programs (student_id, program_id, admission_type, admission_year, admitted_on)
			SELECT id, $2, 'OSYS', $3::int, make_date($3::int, 9, 1) FROM s RETURNING id
		)
		SELECT (SELECT id FROM u), (SELECT id FROM sp)`, studentNo, programID, admissionYear).Scan(&userID, &studentProgramID)
	if err != nil {
		t.Fatal(err)
	}
	return userID, studentProgramID
}

func studentCurriculum(t *testing.T, pool *pgxpool.Pool, studentProgramID string) string {
	t.Helper()
	var id *string
	if err := pool.QueryRow(context.Background(), `SELECT curriculum_id::text FROM enrollment.student_programs WHERE id = $1`, studentProgramID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if id == nil {
		return ""
	}
	return *id
}

func intp(v int) *int { return &v }

func TestCurriculumLifecycle(t *testing.T) {
	pool := dbtest.New(t)
	fx := seedOrg(t, pool)
	repo := curriculum.NewRepository(pool)
	ctx := context.Background()

	c1 := mustCreate(t, repo, course("COM1001", fx.bil, 6))
	c2 := mustCreate(t, repo, course("COM1002", fx.bil, 6))
	group, err := repo.CreateGroup(ctx, "", curriculum.GroupInput{Code: "COMTE02", NameTR: "Teknik seçmeli", NameEN: "Technical elective", OwnerDepartmentID: fx.bil, Kind: curriculum.GroupTechnical, IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	_, sp2020 := seedStudent(t, pool, fx.bilProg, "22290101", 2020)
	user2023, sp2023 := seedStudent(t, pool, fx.bilProg, "23290101", 2023)

	in2022 := curriculum.CurriculumInput{NameTR: "2022 müfredatı", NameEN: "2022 curriculum", EffectiveFromYear: 2018, EffectiveToYear: intp(2022), TotalECTSRequired: 20}
	v2022, err := repo.CreateCurriculum(ctx, "", fx.bilProg, in2022, "")
	if err != nil {
		t.Fatal(err)
	}
	items := []curriculum.ItemInput{
		{SemesterNo: 1, Type: curriculum.ItemCourse, CourseID: c1, IsCompulsory: true},
		{SemesterNo: 2, Type: curriculum.ItemCourse, CourseID: c2, IsCompulsory: true},
		{SemesterNo: 3, Type: curriculum.ItemElectiveSlot, GroupID: group, TheoryHours: 3, NationalCredit: 3, ECTS: 6},
		{SemesterNo: 4, Type: curriculum.ItemElectiveSlot, GroupID: group, TheoryHours: 3, NationalCredit: 3, ECTS: 6}, // yuva tekrarlanabilir
	}
	for _, it := range items {
		if _, err := repo.AddItem(ctx, "", v2022, it); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repo.AddItem(ctx, "", v2022, items[0]); !errors.Is(err, curriculum.ErrDuplicate) {
		t.Errorf("aynı ders iki kez: %v", err)
	}

	// AKTS toplamı tutmuyor (24 ≠ 20): taslakta düzeltilir.
	var mismatch *curriculum.TotalMismatchError
	if _, err := repo.Activate(ctx, "", v2022); !errors.As(err, &mismatch) || mismatch.Actual != 24 {
		t.Fatalf("toplam uyuşmazlığı: %v", err)
	}
	c, _ := repo.Curriculum(ctx, v2022)
	in2022.TotalECTSRequired = 24
	if err := repo.UpdateCurriculum(ctx, "", v2022, c.Version, in2022); err != nil {
		t.Fatal(err)
	}
	linked, err := repo.Activate(ctx, "", v2022)
	if err != nil || linked != 1 {
		t.Fatalf("yürürlüğe koyma: %d, %v", linked, err)
	}
	if studentCurriculum(t, pool, sp2020) != v2022 || studentCurriculum(t, pool, sp2023) != "" {
		t.Errorf("giriş yılına göre bağlantı yanlış")
	}

	got, _ := repo.CurriculumItems(ctx, v2022)
	sum := curriculum.Summarize(got)
	if sum.TotalECTS != 24 || sum.CompulsoryECTS != 12 || sum.ElectiveECTS != 12 || len(sum.Semesters) != 4 ||
		len(sum.ElectiveKinds) != 1 || sum.ElectiveKinds[0].ECTS != 12 {
		t.Errorf("özet = %+v", sum)
	}
	if got[0].Code() != "COM1001" || got[2].Code() != "COMTE02" || got[2].TheoryHours != 3 {
		t.Errorf("satırlar = %+v", got)
	}

	// Yürürlükteki sürüm: satırlar ve giriş yılı başlangıcı donmuş.
	if _, err := repo.AddItem(ctx, "", v2022, curriculum.ItemInput{SemesterNo: 5, Type: curriculum.ItemElectiveSlot, GroupID: group, ECTS: 6}); !errors.Is(err, curriculum.ErrNotDraft) {
		t.Errorf("yürürlükteki sürüme satır: %v", err)
	}
	c, _ = repo.Curriculum(ctx, v2022)
	frozen := in2022
	frozen.EffectiveFromYear = 2017
	if err := repo.UpdateCurriculum(ctx, "", v2022, c.Version, frozen); !errors.Is(err, curriculum.ErrNotDraft) {
		t.Errorf("başlangıç yılı değişikliği: %v", err)
	}
	early := in2022
	early.EffectiveToYear = intp(2019) // 2020 girişli öğrenci aralık dışında kalırdı
	if err := repo.UpdateCurriculum(ctx, "", v2022, c.Version, early); !errors.Is(err, curriculum.ErrCurriculumInUse) {
		t.Errorf("kapanış yılını öne çekme: %v", err)
	}
	renamed := in2022
	renamed.DecisionRef = "Senato 2022/14"
	if err := repo.UpdateCurriculum(ctx, "", v2022, c.Version, renamed); err != nil {
		t.Errorf("karar bilgisi: %v", err)
	}

	// Yeni sürüm öncekinden kopyalanır; giriş yılları çakışırsa yürürlüğe giremez.
	in2023 := curriculum.CurriculumInput{NameTR: "2023 müfredatı", NameEN: "2023 curriculum", EffectiveFromYear: 2022, TotalECTSRequired: 24}
	v2023, err := repo.CreateCurriculum(ctx, "", fx.bilProg, in2023, v2022)
	if err != nil {
		t.Fatal(err)
	}
	if copied, _ := repo.CurriculumItems(ctx, v2023); len(copied) != 4 {
		t.Errorf("kopyalanan satırlar = %d", len(copied))
	}
	if _, err := repo.Activate(ctx, "", v2023); !errors.Is(err, curriculum.ErrCurriculumOverlap) {
		t.Errorf("çakışan giriş yılları: %v", err)
	}
	c, _ = repo.Curriculum(ctx, v2023)
	in2023.EffectiveFromYear = 2023
	if err := repo.UpdateCurriculum(ctx, "", v2023, c.Version, in2023); err != nil {
		t.Fatal(err)
	}
	if linked, err := repo.Activate(ctx, "", v2023); err != nil || linked != 1 || studentCurriculum(t, pool, sp2023) != v2023 {
		t.Fatalf("2023 sürümü: %d, %v", linked, err)
	}

	refs, err := repo.StudentCurricula(ctx, user2023)
	if err != nil || len(refs) != 1 || refs[0].CurriculumID != v2023 || refs[0].AdmissionYear != 2023 {
		t.Errorf("öğrencinin müfredatı = %+v, %v", refs, err)
	}

	// Arşiv: öğrenimi süren öğrenci varken olmaz.
	if err := repo.Archive(ctx, "", v2022); !errors.Is(err, curriculum.ErrCurriculumInUse) {
		t.Errorf("öğrencisi olan sürümü arşivleme: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE enrollment.student_programs SET status = 'GRADUATED', graduated_on = '2024-06-30' WHERE id = $1`, sp2020); err != nil {
		t.Fatal(err)
	}
	if err := repo.Archive(ctx, "", v2022); err != nil {
		t.Fatal(err)
	}
	if err := repo.Archive(ctx, "", v2022); !errors.Is(err, curriculum.ErrNotActive) {
		t.Errorf("arşivlenmiş sürümü arşivleme: %v", err)
	}
	if err := repo.DeleteCurriculum(ctx, "", v2023); !errors.Is(err, curriculum.ErrNotDraft) {
		t.Errorf("yürürlükteki sürümü silme: %v", err)
	}

	// Program 8 yarıyıllık: 9. yarıyıldaki satır yürürlüğe girmeyi engeller. Taslak silinebilir.
	draft, _ := repo.CreateCurriculum(ctx, "", fx.bilProg, curriculum.CurriculumInput{NameTR: "Taslak", NameEN: "Draft", EffectiveFromYear: 2030, TotalECTSRequired: 6}, "")
	if _, err := repo.AddItem(ctx, "", draft, curriculum.ItemInput{SemesterNo: 9, Type: curriculum.ItemCourse, CourseID: c1, IsCompulsory: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Activate(ctx, "", draft); !errors.Is(err, curriculum.ErrSemesterRange) {
		t.Errorf("program süresini aşan yarıyıl: %v", err)
	}
	if err := repo.DeleteCurriculum(ctx, "", draft); err != nil {
		t.Fatal(err)
	}
	list, _ := repo.ProgramCurricula(ctx, fx.bilProg, true)
	public, _ := repo.ProgramCurricula(ctx, fx.bilProg, false)
	if len(list) != 2 || list[0].ID != v2023 || len(public) != 2 {
		t.Errorf("sürümler = %d / %d", len(list), len(public))
	}
	if n := auditCount(t, pool, "curriculum.activate", v2022); n != 1 {
		t.Errorf("denetim kaydı = %d", n)
	}
}

func TestHTTPCurricula(t *testing.T) {
	s := newServer(t)
	ctx := context.Background()
	c1 := mustCreate(t, s.repo, course("COM1001", s.fx.bil, 6))
	group, err := s.repo.CreateGroup(ctx, "", curriculum.GroupInput{Code: "COMTE02", NameTR: "Teknik seçmeli", NameEN: "Technical elective", Kind: curriculum.GroupTechnical, IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	me, _ := seedStudent(t, s.pool, s.fx.bilProg, "26290101", 2026)
	s.grants[me] = s.grants[student]

	body := map[string]any{"name_tr": "2026 müfredatı", "name_en": "2026 curriculum", "effective_from_year": 2026, "total_ects_required": 10}
	path := "/api/v1/programs/" + s.fx.bilProg + "/curricula"
	if res := s.do("POST", path, student, nil, body); res.status != 403 {
		t.Errorf("öğrenci müfredat açamaz: %d", res.status)
	}
	if res := s.do("POST", "/api/v1/programs/"+s.fx.fizProg+"/curricula", deptMg, nil, body); res.status != 403 {
		t.Errorf("başka bölümün programı: %d", res.status)
	}
	if res := s.do("POST", "/api/v1/programs/01a11b7f-0000-7000-8000-00000000dead/curricula", deptMg, nil, body); res.status != 404 {
		t.Errorf("olmayan program: %d", res.status)
	}
	res := s.do("POST", path, deptMg, nil, body)
	if res.status != 201 || res.body["status"] != "DRAFT" || res.body["program"].(map[string]any)["code"] != "BIL-EN" {
		t.Fatalf("taslak: %d %s", res.status, res.raw)
	}
	id := res.body["id"].(string)

	// Taslak öğrenciye görünmez.
	if res := s.do("GET", path, student, nil, nil); len(res.items()) != 0 {
		t.Errorf("öğrenci taslağı görmemeli: %s", res.raw)
	}
	if res := s.do("GET", "/api/v1/curricula/"+id, student, nil, nil); res.status != 404 {
		t.Errorf("taslak detayı öğrenciye 404: %d", res.status)
	}
	if res := s.do("GET", path, deptMg, nil, nil); len(res.items()) != 1 {
		t.Errorf("yönetici taslağı görmeli: %s", res.raw)
	}

	// Satırlar.
	items := "/api/v1/curricula/" + id + "/items"
	if res := s.do("POST", items, deptMg, nil, map[string]any{"semester_no": 1, "item_type": "LAB"}); res.status != 400 {
		t.Errorf("geçersiz satır: %d", res.status)
	}
	res = s.do("POST", items, deptMg, nil, map[string]any{"semester_no": 1, "item_type": "COURSE", "course_id": c1})
	if res.status != 201 || len(res.body["items"].([]any)) != 1 {
		t.Fatalf("ders satırı: %d %s", res.status, res.raw)
	}
	if res := s.do("POST", items, deptMg, nil, map[string]any{"semester_no": 2, "item_type": "COURSE", "course_id": c1}); res.status != 409 || res.code() != "COURSE_ALREADY_IN_CURRICULUM" {
		t.Errorf("aynı ders: %d %s", res.status, res.raw)
	}
	res = s.do("POST", items, deptMg, nil, map[string]any{"semester_no": 3, "item_type": "ELECTIVE_SLOT", "elective_group_id": group, "theory_hours": 3, "national_credit": 3, "ects": 5})
	if res.status != 201 {
		t.Fatalf("yuva: %d %s", res.status, res.raw)
	}
	slot := res.body["items"].([]any)[1].(map[string]any)
	if slot["code"] != "COMTE02" || slot["is_compulsory"] != false || slot["ects"] != 5.0 {
		t.Errorf("yuva: %v", slot)
	}
	res = s.do("PUT", items+"/"+slot["id"].(string), deptMg, nil, map[string]any{"semester_no": 3, "item_type": "ELECTIVE_SLOT", "elective_group_id": group, "theory_hours": 3, "national_credit": 3, "ects": 6})
	if res.status != 200 || res.body["summary"].(map[string]any)["total_ects"] != 12.0 {
		t.Fatalf("yuva güncelleme: %d %s", res.status, res.raw)
	}

	// Yürürlük: toplam tutmalı.
	if res := s.do("POST", "/api/v1/curricula/"+id+"/activate", deptMg, nil, nil); res.status != 409 || res.code() != "ECTS_TOTAL_MISMATCH" {
		t.Errorf("toplam uyuşmazlığı: %d %s", res.status, res.raw)
	}
	etag := s.do("GET", "/api/v1/curricula/"+id, deptMg, nil, nil).header.Get("ETag")
	body["total_ects_required"] = 12
	if res := s.do("PUT", "/api/v1/curricula/"+id, deptMg, map[string]string{"If-Match": etag}, body); res.status != 200 {
		t.Fatalf("güncelleme: %d %s", res.status, res.raw)
	}
	res = s.do("POST", "/api/v1/curricula/"+id+"/activate", deptMg, nil, nil)
	if res.status != 200 || res.body["status"] != "ACTIVE" || res.body["student_count"] != 1.0 {
		t.Fatalf("yürürlüğe koyma: %d %s", res.status, res.raw)
	}
	if res := s.do("POST", items, deptMg, nil, map[string]any{"semester_no": 2, "item_type": "COURSE", "course_id": c1}); res.status != 409 || res.code() != "CURRICULUM_NOT_DRAFT" {
		t.Errorf("yürürlükteki sürüme satır: %d %s", res.status, res.raw)
	}
	if res := s.do("POST", "/api/v1/curricula/"+id+"/archive", deptMg, nil, nil); res.status != 409 || res.code() != "CURRICULUM_IN_USE" {
		t.Errorf("öğrencisi olan sürüm arşivlenemez: %d %s", res.status, res.raw)
	}

	// Öğrenci kendi ders planını görür.
	res = s.do("GET", "/api/v1/me/curricula", me, nil, nil)
	if res.status != 200 || len(res.items()) != 1 {
		t.Fatalf("ders planım: %d %s", res.status, res.raw)
	}
	mine := res.items()[0].(map[string]any)
	cur, _ := mine["curriculum"].(map[string]any)
	if cur == nil || cur["id"] != id || len(cur["items"].([]any)) != 2 || mine["admission_year"] != 2026.0 {
		t.Errorf("ders planım: %v", mine)
	}
	if res := s.do("GET", "/api/v1/me/curricula", deptMg, nil, nil); res.status != 200 || len(res.items()) != 0 {
		t.Errorf("öğrenci olmayan: %d %s", res.status, res.raw)
	}
	if res := s.do("GET", "/api/v1/curricula/"+id, student, nil, nil); res.status != 200 {
		t.Errorf("yürürlükteki sürüm herkese açık: %d", res.status)
	}
}
