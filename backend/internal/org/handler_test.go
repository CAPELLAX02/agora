package org

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

// fakeStore, Store interface'inin testlerde kullanılan sahte gerçeklemesidir.
// Aldığı son filtreleri kaydeder, böylece handler'ın sorgu parametrelerini
// doğru çözdüğü de test edilebilir.
type fakeStore struct {
	faculties   []Faculty
	departments []Department
	programs    []Program
	hasMore     bool
	err         error

	lastFacultyFilter FacultyFilter
	lastProgramFilter ProgramFilter
}

func (s *fakeStore) ListFaculties(ctx context.Context, filter FacultyFilter) ([]Faculty, error) {
	s.lastFacultyFilter = filter
	return s.faculties, s.err
}

func (s *fakeStore) GetFaculty(ctx context.Context, id string) (Faculty, error) {
	return findByID(s.faculties, s.err, id, func(f Faculty) string { return f.ID })
}

func (s *fakeStore) ListDepartments(ctx context.Context, facultyID string, includeInactive bool) ([]Department, error) {
	return s.departments, s.err
}

func (s *fakeStore) GetDepartment(ctx context.Context, id string) (Department, error) {
	return findByID(s.departments, s.err, id, func(d Department) string { return d.ID })
}

func (s *fakeStore) ListPrograms(ctx context.Context, filter ProgramFilter) ([]Program, bool, error) {
	s.lastProgramFilter = filter
	return s.programs, s.hasMore, s.err
}

func (s *fakeStore) GetProgram(ctx context.Context, id string) (Program, error) {
	return findByID(s.programs, s.err, id, func(p Program) string { return p.ID })
}

// findByID, sahte verilerde kimliğe göre arama yapar. Generic olduğu için
// birim, bölüm ve program için aynı kod kullanılır.
func findByID[T any](items []T, err error, id string, idOf func(T) string) (T, error) {
	var zero T
	if err != nil {
		return zero, err
	}
	for _, item := range items {
		if idOf(item) == id {
			return item, nil
		}
	}
	return zero, ErrNotFound
}

const (
	muhID     = "01a1168a-b446-7549-aa69-6726d4454c8f"
	bilID     = "01a1168a-b446-7549-aa69-6726d4454c90"
	bilProgID = "01a1168a-b446-7549-aa69-6726d4454c91"
	missingID = "01a1168a-ffff-7fff-bfff-ffffffffffff"
)

func testStore() *fakeStore {
	muh := FacultyRef{ID: muhID, Code: "MUH", NameTR: "Mühendislik Fakültesi"}
	bil := DepartmentRef{ID: bilID, Code: "BIL", NameTR: "Bilgisayar Mühendisliği"}
	return &fakeStore{
		faculties: []Faculty{{
			ID: muhID, Code: "MUH", NameTR: "Mühendislik Fakültesi", NameEN: "Faculty of Engineering",
			UnitType: UnitFaculty, IsActive: true,
			Campus: &CampusRef{ID: "01a1168a-0000-7000-8000-000000000001", Code: "GOLBASI", Name: "50. Yıl Yerleşkesi"},
		}},
		departments: []Department{{
			ID: bilID, Faculty: muh, Code: "BIL", NameTR: "Bilgisayar Mühendisliği", NameEN: "Computer Engineering", IsActive: true,
		}},
		programs: []Program{{
			ID: bilProgID, Department: bil, Faculty: muh, Code: "BIL-EN-NO",
			NameTR: "Bilgisayar Mühendisliği (İngilizce)", NameEN: "Computer Engineering",
			DegreeLevel: DegreeBachelor, Language: LanguageEN, EducationType: EducationDaytime,
			DurationSemesters: 8, MaxDurationYears: 7, TotalECTSRequired: 240, HasPrepClass: true, IsActive: true,
		}},
	}
}

func serve(t *testing.T, store Store, target string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	NewHandler(store, slog.New(slog.NewTextHandler(io.Discard, nil))).Register(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("gövde çözülemedi: %v\n%s", err, rec.Body.String())
	}
	return v
}

// problemFields, bir doğrulama hatası yanıtındaki hatalı alanları küme olarak döndürür.
func problemFields(t *testing.T, rec *httptest.ResponseRecorder) map[string]bool {
	t.Helper()
	p := decode[httpx.Problem](t, rec)
	if p.Code != "VALIDATION_FAILED" {
		t.Fatalf("code = %q, want VALIDATION_FAILED", p.Code)
	}
	fields := map[string]bool{}
	for _, e := range p.Errors {
		fields[e.Field] = true
	}
	return fields
}

func TestListFaculties(t *testing.T) {
	t.Run("filtreler çözülür", func(t *testing.T) {
		store := testStore()
		rec := serve(t, store, "/api/v1/faculties?type=FACULTY&q=+mühendis+&include_inactive=true")

		if rec.Code != http.StatusOK {
			t.Fatalf("durum = %d\n%s", rec.Code, rec.Body.String())
		}
		want := FacultyFilter{UnitType: UnitFaculty, Query: "mühendis", IncludeInactive: true}
		if store.lastFacultyFilter != want {
			t.Errorf("filtre = %+v, want %+v", store.lastFacultyFilter, want)
		}
		got := decode[httpx.ListResponse[facultyResponse]](t, rec)
		if len(got.Items) != 1 || got.Items[0].Campus == nil || got.NextCursor != "" {
			t.Errorf("beklenmeyen yanıt: %+v", got)
		}
	})

	t.Run("geçersiz parametreler birlikte raporlanır", func(t *testing.T) {
		rec := serve(t, testStore(), "/api/v1/faculties?type=UNIVERSITY&include_inactive=belki&q="+strings.Repeat("a", 101))

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("durum = %d, want 400", rec.Code)
		}
		fields := problemFields(t, rec)
		for _, f := range []string{"type", "q", "include_inactive"} {
			if !fields[f] {
				t.Errorf("%q alanı hata listesinde yok: %v", f, fields)
			}
		}
	})

	t.Run("veri katmanı hatası 500 döner ve ayrıntı sızmaz", func(t *testing.T) {
		store := testStore()
		store.err = errors.New("bağlantı koptu")
		rec := serve(t, store, "/api/v1/faculties")

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("durum = %d, want 500", rec.Code)
		}
		if strings.Contains(rec.Body.String(), "bağlantı koptu") {
			t.Error("iç hata ayrıntısı istemciye sızdı")
		}
	})

	t.Run("boş liste null değil []", func(t *testing.T) {
		rec := serve(t, &fakeStore{faculties: []Faculty{}}, "/api/v1/faculties")
		if got := strings.TrimSpace(rec.Body.String()); got != `{"items":[]}` {
			t.Errorf("boş liste = %s, want {\"items\":[]}", got)
		}
	})
}

func TestGetByID(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		wantStatus int
		wantCode   string
	}{
		{"birim", "/api/v1/faculties/" + muhID, http.StatusOK, "MUH"},
		{"bölüm", "/api/v1/departments/" + bilID, http.StatusOK, "BIL"},
		{"program", "/api/v1/programs/" + bilProgID, http.StatusOK, "BIL-EN-NO"},
		{"olmayan birim", "/api/v1/faculties/" + missingID, http.StatusNotFound, ""},
		{"olmayan bölüm", "/api/v1/departments/" + missingID, http.StatusNotFound, ""},
		{"olmayan program", "/api/v1/programs/" + missingID, http.StatusNotFound, ""},
		{"geçersiz kimlik veritabanına gitmeden 404", "/api/v1/programs/bil-1", http.StatusNotFound, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serve(t, testStore(), tt.target)
			if rec.Code != tt.wantStatus {
				t.Fatalf("durum = %d, want %d\n%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantCode == "" {
				return
			}
			if got := decode[map[string]any](t, rec)["code"]; got != tt.wantCode {
				t.Errorf("code = %v, want %s", got, tt.wantCode)
			}
		})
	}
}

func TestListFacultyDepartments(t *testing.T) {
	t.Run("birimin bölümleri", func(t *testing.T) {
		rec := serve(t, testStore(), "/api/v1/faculties/"+muhID+"/departments")
		if rec.Code != http.StatusOK {
			t.Fatalf("durum = %d\n%s", rec.Code, rec.Body.String())
		}
		got := decode[httpx.ListResponse[departmentResponse]](t, rec)
		if len(got.Items) != 1 || got.Items[0].Faculty.Code != "MUH" {
			t.Errorf("beklenmeyen bölümler: %+v", got.Items)
		}
	})

	t.Run("olmayan birim 404", func(t *testing.T) {
		rec := serve(t, testStore(), "/api/v1/faculties/"+missingID+"/departments")
		if rec.Code != http.StatusNotFound {
			t.Errorf("durum = %d, want 404", rec.Code)
		}
	})
}

func TestListPrograms(t *testing.T) {
	t.Run("filtreler ve varsayılan limit", func(t *testing.T) {
		store := testStore()
		rec := serve(t, store, "/api/v1/programs?faculty_id="+muhID+
			"&degree_level=BACHELOR&language=EN&education_type=DAYTIME&q=bil")

		if rec.Code != http.StatusOK {
			t.Fatalf("durum = %d\n%s", rec.Code, rec.Body.String())
		}
		want := ProgramFilter{
			FacultyID: muhID, DegreeLevel: DegreeBachelor, Language: LanguageEN,
			EducationType: EducationDaytime, Query: "bil", Limit: httpx.DefaultPageLimit,
		}
		if got := store.lastProgramFilter; got != want {
			t.Errorf("filtre = %+v, want %+v", got, want)
		}
	})

	t.Run("sonraki sayfa varsa cursor döner ve geri çözülür", func(t *testing.T) {
		store := testStore()
		store.hasMore = true
		rec := serve(t, store, "/api/v1/programs?limit=1")

		got := decode[httpx.ListResponse[programResponse]](t, rec)
		if got.NextCursor == "" {
			t.Fatal("hasMore=true iken next_cursor boş")
		}

		serve(t, store, "/api/v1/programs?limit=1&cursor="+got.NextCursor)
		after := store.lastProgramFilter.After
		if after == nil || after.ID != bilProgID || after.NameTR != "Bilgisayar Mühendisliği (İngilizce)" {
			t.Errorf("cursor geri çözülemedi: %+v", after)
		}
	})

	t.Run("son sayfada cursor yok", func(t *testing.T) {
		rec := serve(t, testStore(), "/api/v1/programs")
		if strings.Contains(rec.Body.String(), "next_cursor") {
			t.Errorf("son sayfada next_cursor olmamalı: %s", rec.Body.String())
		}
	})

	t.Run("geçersiz parametreler birlikte raporlanır", func(t *testing.T) {
		rec := serve(t, testStore(), "/api/v1/programs?faculty_id=muh&department_id=x&degree_level=LISANS"+
			"&language=DE&education_type=NIGHT&limit=500&cursor=bozuk")

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("durum = %d, want 400", rec.Code)
		}
		fields := problemFields(t, rec)
		for _, f := range []string{"faculty_id", "department_id", "degree_level", "language", "education_type", "limit", "cursor"} {
			if !fields[f] {
				t.Errorf("%q alanı hata listesinde yok: %v", f, fields)
			}
		}
	})
}
