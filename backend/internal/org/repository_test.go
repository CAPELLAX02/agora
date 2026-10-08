package org

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
)

// TestRepository, gerçek bir PostgreSQL'e karşı çalışan entegrasyon testidir.
// Konteyner bir kez başlatılır, alt testler aynı veriyi paylaşır.
func TestRepository(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	seed(t, ctx, pool)
	repo := NewRepository(pool)

	facultyCodes := func(fs []Faculty) []string { return codesOf(fs, func(f Faculty) string { return f.Code }) }
	programCodes := func(ps []Program) []string { return codesOf(ps, func(p Program) string { return p.Code }) }

	t.Run("birimler: sadece aktifler, Türkçe alfabetik sıra", func(t *testing.T) {
		got, err := repo.ListFaculties(ctx, FacultyFilter{})
		if err != nil {
			t.Fatal(err)
		}
		// Varsayılan (C) sıralamada Ç ve İ, Z'den sonra gelirdi.
		want := []string{"CEVRE", "ILT", "MUH", "ZRT"}
		if !slices.Equal(facultyCodes(got), want) {
			t.Errorf("sıra = %v, want %v", facultyCodes(got), want)
		}
	})

	t.Run("birimler: pasifler, tür ve arama filtreleri", func(t *testing.T) {
		all, err := repo.ListFaculties(ctx, FacultyFilter{IncludeInactive: true})
		if err != nil || len(all) != 5 {
			t.Fatalf("pasifler dahil = %v, %v; want 5 kayıt", facultyCodes(all), err)
		}

		myo, err := repo.ListFaculties(ctx, FacultyFilter{UnitType: UnitVocationalSchool})
		if err != nil || !slices.Equal(facultyCodes(myo), []string{"CEVRE"}) {
			t.Errorf("tür filtresi = %v, %v", facultyCodes(myo), err)
		}

		search, err := repo.ListFaculties(ctx, FacultyFilter{Query: "ENGINEERING"})
		if err != nil || !slices.Equal(facultyCodes(search), []string{"MUH"}) {
			t.Errorf("arama = %v, %v", facultyCodes(search), err)
		}
	})

	t.Run("birim: yerleşke JOIN ve NULL yerleşke", func(t *testing.T) {
		muh := mustFaculty(t, ctx, repo, "MUH")
		if muh.Campus == nil || muh.Campus.Name != "50. Yıl Yerleşkesi" {
			t.Errorf("MUH yerleşkesi = %+v", muh.Campus)
		}
		if zrt := mustFaculty(t, ctx, repo, "ZRT"); zrt.Campus != nil {
			t.Errorf("ZRT yerleşkesi nil olmalıydı: %+v", zrt.Campus)
		}
	})

	t.Run("olmayan kimlikler ErrNotFound döner", func(t *testing.T) {
		const missing = "01a1168a-ffff-7fff-bfff-ffffffffffff"
		if _, err := repo.GetFaculty(ctx, missing); !errors.Is(err, ErrNotFound) {
			t.Errorf("GetFaculty err = %v", err)
		}
		if _, err := repo.GetDepartment(ctx, missing); !errors.Is(err, ErrNotFound) {
			t.Errorf("GetDepartment err = %v", err)
		}
		if _, err := repo.GetProgram(ctx, missing); !errors.Is(err, ErrNotFound) {
			t.Errorf("GetProgram err = %v", err)
		}
	})

	t.Run("bölümler: sadece istenen birimin bölümleri, sıralı", func(t *testing.T) {
		muh := mustFaculty(t, ctx, repo, "MUH")
		got, err := repo.ListDepartments(ctx, muh.ID, false)
		if err != nil {
			t.Fatal(err)
		}
		codes := codesOf(got, func(d Department) string { return d.Code })
		if !slices.Equal(codes, []string{"BIL", "EEM"}) {
			t.Errorf("bölümler = %v, want [BIL EEM]", codes)
		}
		if got[0].Faculty.Code != "MUH" {
			t.Errorf("bölümün birimi = %+v", got[0].Faculty)
		}
	})

	t.Run("programlar: filtreler", func(t *testing.T) {
		muh := mustFaculty(t, ctx, repo, "MUH")

		byFaculty, _, err := repo.ListPrograms(ctx, ProgramFilter{FacultyID: muh.ID, Limit: 50})
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"BIL-EN-NO", "BIL-YL", "EEM-EN-NO"}; !slices.Equal(programCodes(byFaculty), want) {
			t.Errorf("birim filtresi = %v, want %v", programCodes(byFaculty), want)
		}

		masters, _, err := repo.ListPrograms(ctx, ProgramFilter{DegreeLevel: DegreeMaster, Limit: 50})
		if err != nil || !slices.Equal(programCodes(masters), []string{"BIL-YL"}) {
			t.Errorf("derece filtresi = %v, %v", programCodes(masters), err)
		}

		evening, _, err := repo.ListPrograms(ctx, ProgramFilter{EducationType: EducationEvening, Language: LanguageTR, Limit: 50})
		if err != nil || !slices.Equal(programCodes(evening), []string{"IKT-IO"}) {
			t.Errorf("öğretim türü + dil filtresi = %v, %v", programCodes(evening), err)
		}

		p := byFaculty[0]
		if p.Department.Code != "BIL" || p.Faculty.Code != "MUH" || p.TotalECTSRequired != 240 || p.YoksisCode != nil {
			t.Errorf("program alanları = %+v", p)
		}
	})

	t.Run("programlar: sayfa sayfa gezinmek tek seferde almakla aynı sonucu verir", func(t *testing.T) {
		all, hasMore, err := repo.ListPrograms(ctx, ProgramFilter{Limit: 100})
		if err != nil || hasMore {
			t.Fatalf("tüm liste: hasMore=%v err=%v", hasMore, err)
		}

		var (
			walked []Program
			after  *ProgramCursor
			pages  int
		)
		for {
			page, more, err := repo.ListPrograms(ctx, ProgramFilter{Limit: 2, After: after})
			if err != nil {
				t.Fatal(err)
			}
			pages++
			walked = append(walked, page...)
			if !more {
				break
			}
			last := page[len(page)-1]
			after = &ProgramCursor{NameTR: last.NameTR, ID: last.ID}
		}

		if !slices.Equal(programCodes(walked), programCodes(all)) {
			t.Errorf("sayfalı = %v\ntek sefer = %v", programCodes(walked), programCodes(all))
		}
		if want := (len(all) + 1) / 2; pages != want {
			t.Errorf("sayfa sayısı = %d, want %d", pages, want)
		}
		// Türkçe sıra: "İktisat" ve "İktisat (İkinci Öğretim)", "Elektrik"ten sonra
		// ve "Matematik"ten önce gelmeli. Varsayılan sıralamada en sona düşerlerdi.
		codes := programCodes(all)
		if slices.Index(codes, "IKT-NO") > slices.Index(codes, "MAT-NO") {
			t.Errorf("Türkçe sıralama bozuk: %v", codes)
		}
	})
}

func codesOf[T any](items []T, code func(T) string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, code(item))
	}
	return out
}

func mustFaculty(t *testing.T, ctx context.Context, repo *Repository, code string) Faculty {
	t.Helper()
	fs, err := repo.ListFaculties(ctx, FacultyFilter{Query: code, IncludeInactive: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range fs {
		if f.Code == code {
			return f
		}
	}
	t.Fatalf("%s birimi bulunamadı", code)
	return Faculty{}
}

func seed(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()

	_, err := pool.Exec(ctx, `
		INSERT INTO org.campuses (code, name) VALUES ('GOLBASI', '50. Yıl Yerleşkesi');

		INSERT INTO org.faculties (code, name_tr, name_en, unit_type, campus_id, is_active) VALUES
			('MUH',   'Mühendislik Fakültesi',     'Faculty of Engineering',           'FACULTY',
				(SELECT id FROM org.campuses WHERE code = 'GOLBASI'), true),
			('ZRT',   'Ziraat Fakültesi',          'Faculty of Agriculture',           'FACULTY', NULL, true),
			('ILT',   'İletişim Fakültesi',        'Faculty of Communication',         'FACULTY', NULL, true),
			('SBF',   'Siyasal Bilgiler Fakültesi','Faculty of Political Science',     'FACULTY', NULL, false),
			('CEVRE', 'Çevre Meslek Yüksekokulu',  'Vocational School of Environment', 'VOCATIONAL_SCHOOL', NULL, true);

		INSERT INTO org.departments (faculty_id, code, name_tr, name_en)
		SELECT f.id, v.code, v.name_tr, v.name_en
		FROM (VALUES
			('MUH', 'EEM', 'Elektrik-Elektronik Mühendisliği', 'Electrical and Electronics Engineering'),
			('MUH', 'BIL', 'Bilgisayar Mühendisliği',          'Computer Engineering'),
			('SBF', 'IKT', 'İktisat',                          'Economics'),
			('ILT', 'GAZ', 'Gazetecilik',                      'Journalism')
		) AS v(faculty_code, code, name_tr, name_en)
		JOIN org.faculties f ON f.code = v.faculty_code;

		INSERT INTO org.programs (department_id, code, name_tr, name_en, degree_level, language,
		                          education_type, duration_semesters, max_duration_years, total_ects_required)
		SELECT d.id, v.code, v.name_tr, v.name_en, v.degree, v.lang, v.edu, v.sem, v.max_years, v.ects
		FROM (VALUES
			('BIL', 'BIL-EN-NO', 'Bilgisayar Mühendisliği (İngilizce)', 'Computer Engineering',       'BACHELOR', 'EN', 'DAYTIME', 8, 7, 240.0),
			('BIL', 'BIL-YL',    'Bilgisayar Mühendisliği Yüksek Lisans', 'Computer Engineering (MSc)', 'MASTER', 'TR', 'DAYTIME', 4, 3, 120.0),
			('EEM', 'EEM-EN-NO', 'Elektrik-Elektronik Mühendisliği',    'Electrical Engineering',     'BACHELOR', 'EN', 'DAYTIME', 8, 7, 240.0),
			('IKT', 'IKT-NO',    'İktisat',                             'Economics',                  'BACHELOR', 'TR', 'DAYTIME', 8, 7, 240.0),
			('IKT', 'IKT-IO',    'İktisat (İkinci Öğretim)',            'Economics (Evening)',        'BACHELOR', 'TR', 'EVENING', 8, 7, 240.0),
			('GAZ', 'GAZ-NO',    'Gazetecilik',                         'Journalism',                 'BACHELOR', 'TR', 'DAYTIME', 8, 7, 240.0),
			('GAZ', 'MAT-NO',    'Matematik (test)',                    'Mathematics',                'BACHELOR', 'TR', 'DAYTIME', 8, 7, 240.0)
		) AS v(dept, code, name_tr, name_en, degree, lang, edu, sem, max_years, ects)
		JOIN org.departments d ON d.code = v.dept;
	`)
	if err != nil {
		t.Fatalf("test verisi eklenemedi: %v", err)
	}
}
