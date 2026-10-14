package files_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/files"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
	"github.com/CAPELLAX02/agora/backend/internal/platform/s3test"
)

type noGrants struct{}

func (noGrants) Permissions(_ context.Context, userID string) (*authz.Permissions, error) {
	return authz.NewPermissions(userID, nil), nil
}

type server struct {
	t     *testing.T
	h     http.Handler
	pool  *pgxpool.Pool
	svc   *files.Service
	clock *time.Time
}

func newServer(t *testing.T) (*server, string, string) {
	t.Helper()
	pool := dbtest.New(t)
	store, _ := s3test.New(t)
	now := time.Now()
	s := &server{t: t, pool: pool, clock: &now}
	s.svc = files.NewService(pool, store, 64<<10, func() time.Time { return *s.clock })
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	authenticate := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(authn.WithPrincipal(r.Context(), authn.Principal{UserID: r.Header.Get("X-Test-User")})))
		})
	}
	mux := http.NewServeMux()
	files.NewHandler(s.svc, logger).Register(authz.NewRouter(mux, authenticate, noGrants{}, logger))
	s.h = mux

	user := func(name string) string {
		var id string
		if err := pool.QueryRow(context.Background(), `
			WITH p AS (INSERT INTO people.persons (first_name, last_name) VALUES ($1, 'Test') RETURNING id)
			INSERT INTO iam.users (person_id, username, email, password_hash)
			SELECT id, $1, $1 || '@test', 'x' FROM p RETURNING id`, name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	return s, user("ali"), user("ece")
}

type result struct {
	status int
	body   map[string]any
	raw    string
}

func (r result) code() string { c, _ := r.body["code"].(string); return c }

func (s *server) upload(user, name string, content []byte) result {
	s.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("purpose", "deneme") // dosya dışı alanlar atlanır
	fw, _ := mw.CreateFormFile("file", name)
	_, _ = fw.Write(content)
	_ = mw.Close()
	req := httptest.NewRequest("POST", "/api/v1/files", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return s.serve(req, user)
}

func (s *server) do(method, path, user string) result {
	s.t.Helper()
	return s.serve(httptest.NewRequest(method, path, nil), user)
}

func (s *server) serve(req *http.Request, user string) result {
	req.Header.Set("X-Test-User", user)
	rec := httptest.NewRecorder()
	s.h.ServeHTTP(rec, req)
	res := result{status: rec.Code, raw: rec.Body.String()}
	_ = json.Unmarshal(rec.Body.Bytes(), &res.body)
	return res
}

var pdf = append([]byte("%PDF-1.7\n"), bytes.Repeat([]byte("ödev içeriği "), 200)...)

func TestUploadAndDownload(t *testing.T) {
	s, ali, ece := newServer(t)
	ctx := context.Background()

	res := s.upload(ali, `C:\Belgeler\1. Ödev "son".pdf`, pdf)
	if res.status != 201 || res.body["name"] != "1. Ödev son.pdf" || res.body["mime_type"] != "application/pdf" ||
		res.body["size"] != float64(len(pdf)) || len(res.body["sha256"].(string)) != 64 {
		t.Fatalf("yükleme: %d %s", res.status, res.raw)
	}
	id := res.body["id"].(string)

	// Sadece yükleyen görür.
	if res := s.do("GET", "/api/v1/files/"+id, ece); res.status != 404 {
		t.Errorf("başkasının dosyası: %d", res.status)
	}
	res = s.do("GET", "/api/v1/files/"+id+"/download-url", ali)
	if res.status != 200 {
		t.Fatalf("indirme adresi: %d %s", res.status, res.raw)
	}
	got, err := http.Get(res.body["url"].(string))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(got.Body)
	_ = got.Body.Close()
	if got.StatusCode != 200 || !bytes.Equal(body, pdf) ||
		!strings.Contains(got.Header.Get("Content-Disposition"), `filename*=UTF-8''1.%20%C3%96dev%20son.pdf`) {
		t.Errorf("indirme: %d %d bayt, %q", got.StatusCode, len(body), got.Header.Get("Content-Disposition"))
	}

	// Bağlanan dosya silinemez; bağlanmamış silinir.
	if _, err := s.svc.Attach(ctx, s.pool, id, ece); err == nil {
		t.Error("başkası bağlayamaz")
	}
	if _, err := s.svc.Attach(ctx, s.pool, id, ali); err != nil {
		t.Fatal(err)
	}
	if _, err := s.svc.Attach(ctx, s.pool, id, ali); err != files.ErrAttached {
		t.Errorf("ikinci bağlama: %v", err)
	}
	if res := s.do("DELETE", "/api/v1/files/"+id, ali); res.status != 409 || res.code() != "FILE_IN_USE" {
		t.Errorf("bağlı dosyayı silme: %d %s", res.status, res.raw)
	}
	other := s.upload(ali, "notlar.txt", []byte("hafta 1: giriş\n")).body["id"].(string)
	if res := s.do("DELETE", "/api/v1/files/"+other, ece); res.status != 404 {
		t.Errorf("başkası silemez: %d", res.status)
	}
	if res := s.do("DELETE", "/api/v1/files/"+other, ali); res.status != 204 {
		t.Errorf("silme: %d %s", res.status, res.raw)
	}
	if res := s.do("GET", "/api/v1/files/"+other, ali); res.status != 404 {
		t.Errorf("silinen dosya: %d", res.status)
	}
}

func TestUploadRejects(t *testing.T) {
	s, ali, _ := newServer(t)
	for name, tc := range map[string]struct {
		file    string
		content []byte
		status  int
		code    string
	}{
		"izin verilmeyen uzantı":     {"kurulum.exe", []byte("MZ\x90\x00"), 415, "FILE_TYPE_NOT_ALLOWED"},
		"uzantısız":                  {"README", []byte("metin"), 415, "FILE_TYPE_NOT_ALLOWED"},
		"içerik uzantıyla uyuşmuyor": {"ödev.pdf", []byte("aslında metin"), 415, "FILE_CONTENT_MISMATCH"},
		"metin değil":                {"kod.py", []byte{0x7f, 'E', 'L', 'F', 0, 0, 1}, 415, "FILE_CONTENT_MISMATCH"},
		"boş":                        {"bos.txt", nil, 400, "FILE_EMPTY"},
		"çok büyük":                  {"büyük.txt", bytes.Repeat([]byte("a"), 65<<10), 413, "FILE_TOO_LARGE"},
	} {
		t.Run(name, func(t *testing.T) {
			if res := s.upload(ali, tc.file, tc.content); res.status != tc.status || res.code() != tc.code {
				t.Errorf("%d %s", res.status, res.raw)
			}
		})
	}
	var n int
	_ = s.pool.QueryRow(context.Background(), `SELECT count(*) FROM platform.files`).Scan(&n)
	if n != 0 {
		t.Errorf("reddedilen yüklemeden %d kayıt kaldı", n)
	}

	req := httptest.NewRequest("POST", "/api/v1/files", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	if res := s.serve(req, ali); res.status != 415 {
		t.Errorf("JSON gövde: %d", res.status)
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("purpose", "dosya yok")
	_ = mw.Close()
	req = httptest.NewRequest("POST", "/api/v1/files", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if res := s.serve(req, ali); res.status != 400 || res.code() != "NO_FILE" {
		t.Errorf("dosyasız gövde: %d %s", res.status, res.raw)
	}
}

func TestCleanupUnattached(t *testing.T) {
	s, ali, _ := newServer(t)
	ctx := context.Background()
	old := s.upload(ali, "eski.txt", []byte("eski")).body["id"].(string)
	kept := s.upload(ali, "bagli.txt", []byte("bağlı")).body["id"].(string)
	if _, err := s.svc.Attach(ctx, s.pool, kept, ali); err != nil {
		t.Fatal(err)
	}
	url := s.do("GET", "/api/v1/files/"+old+"/download-url", ali).body["url"].(string)

	*s.clock = s.clock.Add(2 * time.Hour)
	fresh := s.upload(ali, "yeni.txt", []byte("yeni")).body["id"].(string)
	n, err := s.svc.CleanupUnattached(ctx, time.Hour, 100)
	if err != nil || n != 1 {
		t.Fatalf("temizlik: %d %v", n, err)
	}
	for id, want := range map[string]int{old: 404, kept: 200, fresh: 200} {
		if res := s.do("GET", "/api/v1/files/"+id, ali); res.status != want {
			t.Errorf("%s: %d, beklenen %d", id, res.status, want)
		}
	}
	// Silinen dosyanın içeriği de depodan gider.
	if got, err := http.Get(url); err != nil || got.StatusCode != 404 {
		t.Errorf("silinen nesne: %v %v", got.StatusCode, err)
	}
	if _, err := s.svc.Attach(ctx, s.pool, old, ali); err == nil {
		t.Error("silinen dosya bağlanamaz")
	}
}
