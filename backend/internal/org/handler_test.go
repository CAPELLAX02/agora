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
// Aldığı son filtreyi kaydeder, böylece handler'ın sorgu parametrelerini
// doğru çözdüğü de test edilebilir.
type fakeStore struct {
	faculties  []Faculty
	err        error
	lastFilter FacultyFilter
}

func (s *fakeStore) ListFaculties(ctx context.Context, filter FacultyFilter) ([]Faculty, error) {
	s.lastFilter = filter
	return s.faculties, s.err
}

func (s *fakeStore) GetFaculty(ctx context.Context, id string) (Faculty, error) {
	if s.err != nil {
		return Faculty{}, s.err
	}
	for _, f := range s.faculties {
		if f.ID == id {
			return f, nil
		}
	}
	return Faculty{}, ErrNotFound
}

const muhID = "01a1168a-b446-7549-aa69-6726d4454c8f"

func testFaculties() []Faculty {
	return []Faculty{
		{
			ID: muhID, Code: "MUH", NameTR: "Mühendislik Fakültesi", NameEN: "Faculty of Engineering",
			UnitType: UnitFaculty, IsActive: true,
			Campus: &CampusRef{ID: "01a1168a-0000-7000-8000-000000000001", Code: "GOLBASI", Name: "50. Yıl Yerleşkesi"},
		},
	}
}

func newTestHandler(store Store) http.Handler {
	mux := http.NewServeMux()
	NewHandler(store, slog.New(slog.NewTextHandler(io.Discard, nil))).Register(mux)
	return mux
}

func TestListFaculties(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		storeErr   error
		wantStatus int
		wantFilter FacultyFilter
		wantFields []string // 400 yanıtında hatalı olarak bildirilmesi beklenen alanlar
	}{
		{
			name:       "varsayılan filtre",
			wantStatus: http.StatusOK,
			wantFilter: FacultyFilter{},
		},
		{
			name:       "tüm filtreler",
			query:      "?type=FACULTY&q=+mühendis+&include_inactive=true",
			wantStatus: http.StatusOK,
			wantFilter: FacultyFilter{UnitType: UnitFaculty, Query: "mühendis", IncludeInactive: true},
		},
		{
			name:       "geçersiz parametreler birlikte raporlanır",
			query:      "?type=UNIVERSITY&include_inactive=belki&q=" + strings.Repeat("a", 101),
			wantStatus: http.StatusBadRequest,
			wantFields: []string{"type", "q", "include_inactive"},
		},
		{
			name:       "veri katmanı hatası 500 döner",
			storeErr:   errors.New("bağlantı koptu"),
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeStore{faculties: testFaculties(), err: tt.storeErr}
			rec := httptest.NewRecorder()
			newTestHandler(store).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/faculties"+tt.query, nil))

			if rec.Code != tt.wantStatus {
				t.Fatalf("durum = %d, want %d\n%s", rec.Code, tt.wantStatus, rec.Body.String())
			}

			switch tt.wantStatus {
			case http.StatusOK:
				if store.lastFilter != tt.wantFilter {
					t.Errorf("filtre = %+v, want %+v", store.lastFilter, tt.wantFilter)
				}
				var got httpx.ListResponse[facultyResponse]
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatalf("gövde çözülemedi: %v", err)
				}
				if len(got.Items) != 1 || got.Items[0].Code != "MUH" || got.Items[0].Campus == nil {
					t.Errorf("beklenmeyen liste: %+v", got.Items)
				}

			case http.StatusBadRequest:
				var p httpx.Problem
				if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
					t.Fatalf("gövde çözülemedi: %v", err)
				}
				if p.Code != "VALIDATION_FAILED" {
					t.Errorf("code = %q, want VALIDATION_FAILED", p.Code)
				}
				got := map[string]bool{}
				for _, e := range p.Errors {
					got[e.Field] = true
				}
				for _, f := range tt.wantFields {
					if !got[f] {
						t.Errorf("%q alanı hata listesinde yok: %+v", f, p.Errors)
					}
				}

			case http.StatusInternalServerError:
				if strings.Contains(rec.Body.String(), "bağlantı koptu") {
					t.Error("iç hata ayrıntısı istemciye sızdı")
				}
			}
		})
	}
}

func TestListFacultiesEmptyIsArray(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestHandler(&fakeStore{faculties: []Faculty{}}).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/faculties", nil))

	if got := strings.TrimSpace(rec.Body.String()); got != `{"items":[]}` {
		t.Errorf("boş liste = %s, want {\"items\":[]}", got)
	}
}

func TestGetFaculty(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		wantStatus int
	}{
		{name: "var olan birim", id: muhID, wantStatus: http.StatusOK},
		{name: "olmayan birim", id: "01a1168a-ffff-7fff-bfff-ffffffffffff", wantStatus: http.StatusNotFound},
		{name: "geçersiz kimlik veritabanına gitmeden 404", id: "fakulte-1", wantStatus: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			newTestHandler(&fakeStore{faculties: testFaculties()}).
				ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/faculties/"+tt.id, nil))

			if rec.Code != tt.wantStatus {
				t.Fatalf("durum = %d, want %d\n%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantStatus != http.StatusOK {
				return
			}

			var got facultyResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("gövde çözülemedi: %v", err)
			}
			if got.ID != muhID || got.NameTR != "Mühendislik Fakültesi" || got.UnitType != "FACULTY" {
				t.Errorf("beklenmeyen birim: %+v", got)
			}
		})
	}
}
