package org_test

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

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/org"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
)

type orgFixture struct {
	campus, muh, fen string
}

func seed(t *testing.T, pool *pgxpool.Pool) orgFixture {
	t.Helper()
	var f orgFixture
	err := pool.QueryRow(context.Background(), `
		WITH c AS (
			INSERT INTO org.campuses (code, name) VALUES ('GOLBASI', '50. Yıl Yerleşkesi') RETURNING id
		), m AS (
			INSERT INTO org.faculties (code, name_tr, name_en, unit_type, campus_id)
			SELECT 'MUH', 'Mühendislik Fakültesi', 'Faculty of Engineering', 'FACULTY', id FROM c RETURNING id
		), s AS (
			INSERT INTO org.faculties (code, name_tr, name_en, unit_type, campus_id)
			SELECT 'FEN', 'Fen Fakültesi', 'Faculty of Science', 'FACULTY', id FROM c RETURNING id
		)
		SELECT (SELECT id FROM c), (SELECT id FROM m), (SELECT id FROM s)`).Scan(&f.campus, &f.muh, &f.fen)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// grants, test kullanıcılarının yetkileridir: "merkez" bütün üniversitede,
// "muh-sekreter" sadece Mühendislik Fakültesinde derslik yönetebilir.
type grants map[string][]authz.Grant

func (g grants) Permissions(ctx context.Context, userID string) (*authz.Permissions, error) {
	return authz.NewPermissions(userID, g[userID]), nil
}

const (
	central   = "01a11b7f-0000-7000-8000-0000000000c1"
	facultyMg = "01a11b7f-0000-7000-8000-0000000000c2"
)

type server struct {
	t  *testing.T
	h  http.Handler
	fx orgFixture
	db *pgxpool.Pool
}

func newServer(t *testing.T) *server {
	t.Helper()
	pool := dbtest.New(t)
	fx := seed(t, pool)

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
	resolver := grants{
		central:   {{Permission: "classroom:manage", ScopeType: authz.ScopeUniversity}},
		facultyMg: {{Permission: "classroom:manage", ScopeType: authz.ScopeFaculty, ScopeID: fx.muh}},
	}
	mux := http.NewServeMux()
	org.NewRoomsHandler(org.NewRepository(pool), logger).Register(authz.NewRouter(mux, authenticate, resolver, logger))
	return &server{t: t, h: mux, fx: fx, db: pool}
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
	if user != "" {
		req.Header.Set("X-Test-User", user)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	s.h.ServeHTTP(rec, req)

	res := result{status: rec.Code, header: rec.Header(), raw: rec.Body.String()}
	_ = json.Unmarshal(rec.Body.Bytes(), &res.body)
	return res
}

func (r result) code() string {
	c, _ := r.body["code"].(string)
	return c
}

func TestBuildingsAndScope(t *testing.T) {
	s := newServer(t)

	res := s.do("POST", "/api/v1/buildings", facultyMg, nil,
		map[string]string{"campus_id": s.fx.campus, "faculty_id": s.fx.muh, "code": "MUH-A", "name": "Mühendislik A Blok"})
	if res.status != http.StatusCreated || res.header.Get("ETag") != `"1"` || res.header.Get("Location") == "" {
		t.Fatalf("birim yetkilisi kendi binasını oluşturur: %d %s", res.status, res.raw)
	}
	muhBuilding := res.body["id"].(string)

	for name, body := range map[string]map[string]string{
		"başka fakülte": {"campus_id": s.fx.campus, "faculty_id": s.fx.fen, "code": "FEN-A", "name": "Fen A"},
		"birimsiz bina": {"campus_id": s.fx.campus, "code": "ORTAK", "name": "Ortak Derslikler"},
	} {
		if res := s.do("POST", "/api/v1/buildings", facultyMg, nil, body); res.status != http.StatusForbidden {
			t.Errorf("%s: %d %s", name, res.status, res.raw)
		}
	}
	if res := s.do("POST", "/api/v1/buildings", central, nil,
		map[string]string{"campus_id": s.fx.campus, "code": "ORTAK", "name": "Ortak Derslikler"}); res.status != http.StatusCreated {
		t.Errorf("merkez birimsiz bina oluşturur: %d %s", res.status, res.raw)
	}
	if res := s.do("POST", "/api/v1/buildings", central, nil,
		map[string]string{"campus_id": s.fx.campus, "code": "MUH-A", "name": "Tekrar"}); res.status != 409 {
		t.Errorf("aynı kod: %d", res.status)
	}

	// Bir birim yetkilisi kendi binasını başka birime "taşıyamaz".
	res = s.do("PUT", "/api/v1/buildings/"+muhBuilding, facultyMg, map[string]string{"If-Match": `"1"`},
		map[string]any{"faculty_id": s.fx.fen, "name": "Mühendislik A Blok", "is_active": true})
	if res.status != http.StatusForbidden {
		t.Errorf("başka birime taşıma: %d %s", res.status, res.raw)
	}

	if res := s.do("GET", "/api/v1/buildings", "", nil, nil); res.status != 200 || len(res.body["items"].([]any)) != 2 {
		t.Errorf("herkese açık bina listesi: %d %s", res.status, res.raw)
	}
}

func TestClassrooms(t *testing.T) {
	s := newServer(t)
	res := s.do("POST", "/api/v1/buildings", central, nil,
		map[string]string{"campus_id": s.fx.campus, "faculty_id": s.fx.muh, "code": "MUH-A", "name": "Mühendislik A Blok"})
	building := res.body["id"].(string)

	room := func(code string, capacity int, roomType string) map[string]any {
		return map[string]any{
			"building_id": building, "code": code, "name": "Derslik " + code, "capacity": capacity,
			"exam_capacity": capacity / 2, "room_type": roomType, "features": []string{"SMART_BOARD", "PROJECTOR", "PROJECTOR"},
		}
	}

	t.Run("doğrulama", func(t *testing.T) {
		bad := room("a101", 40, "SINIF")
		bad["exam_capacity"] = 50
		bad["features"] = []string{"JAKUZI"}
		res := s.do("POST", "/api/v1/classrooms", central, nil, bad)
		if res.status != 400 {
			t.Fatalf("durum = %d", res.status)
		}
		for _, field := range []string{"code", "exam_capacity", "room_type", "features"} {
			if !strings.Contains(res.raw, `"field":"`+field+`"`) {
				t.Errorf("%s alan hatası yok: %s", field, res.raw)
			}
		}
	})

	var classroomID string
	for i, r := range []map[string]any{room("A101", 60, "LECTURE"), room("A102", 30, "LECTURE"), room("LAB-1", 30, "LAB"), room("AMFI", 200, "AMPHI")} {
		res := s.do("POST", "/api/v1/classrooms", facultyMg, nil, r)
		if res.status != http.StatusCreated {
			t.Fatalf("%d. derslik: %d %s", i, res.status, res.raw)
		}
		if i == 0 {
			classroomID = res.body["id"].(string)
			if got := res.body["features"].([]any); len(got) != 2 || got[0] != "PROJECTOR" {
				t.Errorf("donanımlar sıralanıp tekilleştirilmeli: %v", got)
			}
		}
	}
	if res := s.do("POST", "/api/v1/classrooms", central, nil, room("A101", 10, "LECTURE")); res.status != 409 || res.code() != "CODE_TAKEN" {
		t.Errorf("aynı kod: %d %s", res.status, res.raw)
	}

	t.Run("filtre ve sayfalama", func(t *testing.T) {
		res := s.do("GET", "/api/v1/classrooms?min_capacity=50", "", nil, nil)
		if items := res.body["items"].([]any); len(items) != 2 {
			t.Errorf("en az 50 kişilik = %d", len(items))
		}
		res = s.do("GET", "/api/v1/classrooms?room_type=LAB", "", nil, nil)
		if items := res.body["items"].([]any); len(items) != 1 {
			t.Errorf("laboratuvar = %d", len(items))
		}

		var codes []string
		cursor := ""
		for range 5 {
			path := "/api/v1/classrooms?limit=3"
			if cursor != "" {
				path += "&cursor=" + cursor
			}
			res := s.do("GET", path, "", nil, nil)
			for _, it := range res.body["items"].([]any) {
				codes = append(codes, it.(map[string]any)["code"].(string))
			}
			cursor, _ = res.body["next_cursor"].(string)
			if cursor == "" {
				break
			}
		}
		if strings.Join(codes, ",") != "A101,A102,AMFI,LAB-1" {
			t.Errorf("sayfalı liste = %v", codes)
		}
	})

	t.Run("iyimser kilit", func(t *testing.T) {
		update := map[string]any{"name": "Derslik A101 (yenilendi)", "capacity": 64, "exam_capacity": 32,
			"room_type": "LECTURE", "features": []string{"PROJECTOR"}, "is_active": true}

		if res := s.do("PUT", "/api/v1/classrooms/"+classroomID, central, nil, update); res.status != 428 {
			t.Errorf("If-Match yok: %d", res.status)
		}

		// İki kişi aynı sürümü (1) okuyup düzenliyor: ilki kazanır, ikincisi reddedilir.
		first := s.do("PUT", "/api/v1/classrooms/"+classroomID, central, map[string]string{"If-Match": `"1"`}, update)
		if first.status != 200 || first.header.Get("ETag") != `"2"` || first.body["capacity"].(float64) != 64 {
			t.Fatalf("ilk güncelleme: %d %s", first.status, first.raw)
		}
		update["capacity"] = 10
		update["exam_capacity"] = 5
		second := s.do("PUT", "/api/v1/classrooms/"+classroomID, facultyMg, map[string]string{"If-Match": `"1"`}, update)
		if second.status != 412 || second.code() != "VERSION_MISMATCH" {
			t.Errorf("eski sürümle güncelleme ezilmemeli: %d %s", second.status, second.raw)
		}

		var before, after string
		err := s.db.QueryRow(context.Background(), `
			SELECT before->>'capacity', after->>'capacity' FROM audit.audit_log
			WHERE action = 'classroom.update' AND entity_id = $1`, classroomID).Scan(&before, &after)
		if err != nil || before != "60" || after != "64" {
			t.Errorf("denetim kaydı = %s → %s, %v", before, after, err)
		}
	})

	t.Run("pasif derslik listede görünmez", func(t *testing.T) {
		res := s.do("GET", "/api/v1/classrooms/"+classroomID, "", nil, nil)
		etag := res.header.Get("ETag")
		update := map[string]any{"name": "A101", "capacity": 64, "exam_capacity": 32, "room_type": "LECTURE", "is_active": false}
		if res := s.do("PUT", "/api/v1/classrooms/"+classroomID, central, map[string]string{"If-Match": etag}, update); res.status != 200 {
			t.Fatalf("pasifleştirme: %d %s", res.status, res.raw)
		}
		if items := s.do("GET", "/api/v1/classrooms", "", nil, nil).body["items"].([]any); len(items) != 3 {
			t.Errorf("aktif derslikler = %d, 3 olmalı", len(items))
		}
		if items := s.do("GET", "/api/v1/classrooms?include_inactive=true", "", nil, nil).body["items"].([]any); len(items) != 4 {
			t.Errorf("hepsi = %d, 4 olmalı", len(items))
		}
	})
}
