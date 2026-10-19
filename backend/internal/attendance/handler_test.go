package attendance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/CAPELLAX02/agora/backend/internal/attendance"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
)

// readOwn, herkese kendi devamsızlığını okuma yetkisi verir (öğrenci rolü gibi).
type readOwn struct{}

func (readOwn) Permissions(_ context.Context, userID string) (*authz.Permissions, error) {
	return authz.NewPermissions(userID, []authz.Grant{{Permission: "attendance:read_own", ScopeType: authz.ScopeNone}}), nil
}

type response struct {
	status int
	header http.Header
	body   map[string]any
	raw    string
}

func TestHandler(t *testing.T) {
	pool := dbtest.New(t)
	f := seed(t, pool)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	authenticate := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(authn.WithPrincipal(r.Context(), authn.Principal{UserID: r.Header.Get("X-Test-User")})))
		})
	}
	mux := http.NewServeMux()
	attendance.NewHandler(attendance.NewService(pool, time.Now), logger).Register(authz.NewRouter(mux, authenticate, readOwn{}, logger))
	do := func(method, path, user string, body any, header ...string) response {
		t.Helper()
		var reader io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			reader = bytes.NewReader(b)
		}
		req := httptest.NewRequest(method, path, reader)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Test-User", user)
		for i := 0; i+1 < len(header); i += 2 {
			req.Header.Set(header[i], header[i+1])
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		res := response{status: rec.Code, header: rec.Header(), raw: rec.Body.String()}
		_ = json.Unmarshal(rec.Body.Bytes(), &res.body)
		return res
	}
	ali := f.aliUser(t, pool)

	// Şubenin öğretim elemanı oturumları görür; başkası (öğretim elemanı ya da öğrenci) göremez.
	for _, u := range []string{f.stranger, ali} {
		if res := do("GET", "/api/v1/sections/"+f.section+"/sessions", u, nil); res.status != 403 {
			t.Errorf("yetkisiz oturum listesi: %d", res.status)
		}
	}
	res := do("GET", "/api/v1/sections/"+f.section+"/sessions", f.teacher, nil)
	if res.status != 200 || res.body["section"].(map[string]any)["course_code"] != "BIL1001" {
		t.Fatalf("oturumlar: %d %s", res.status, res.raw)
	}
	var past map[string]any
	for _, x := range res.body["items"].([]any) {
		s := x.(map[string]any)
		if at, _ := time.Parse(time.RFC3339, s["starts_at"].(string)); at.Before(time.Now()) {
			past = s
		}
	}
	id := past["id"].(string)

	// Yoklama listesi ve yazma (If-Match ile).
	roll := do("GET", "/api/v1/sessions/"+id+"/attendance", f.teacher, nil)
	if roll.status != 200 || len(roll.body["items"].([]any)) != 3 || roll.header.Get("ETag") != `"1"` {
		t.Fatalf("yoklama listesi: %d %s", roll.status, roll.raw)
	}
	if first := roll.body["items"].([]any)[0].(map[string]any); first["status"] != nil || first["student_no"] != "25000001" {
		t.Errorf("alınmamış yoklama: %v", first)
	}
	body := map[string]any{"items": []any{map[string]any{"enrollment_id": f.ali, "status": "ABSENT"}}}
	if res := do("PUT", "/api/v1/sessions/"+id+"/attendance", f.teacher, body); res.status != 428 {
		t.Errorf("If-Match'siz: %d", res.status)
	}
	if res := do("PUT", "/api/v1/sessions/"+id+"/attendance", f.stranger, body, "If-Match", `"1"`); res.status != 403 {
		t.Errorf("başkasının yoklaması: %d", res.status)
	}
	res = do("PUT", "/api/v1/sessions/"+id+"/attendance", f.teacher, body, "If-Match", `"1"`)
	if res.status != 200 || res.body["session"].(map[string]any)["status"] != "HELD" || res.body["session"].(map[string]any)["absent"] != 1.0 {
		t.Fatalf("yoklama: %d %s", res.status, res.raw)
	}
	if res := do("PUT", "/api/v1/sessions/"+id+"/attendance", f.teacher, body, "If-Match", `"1"`); res.status != 412 {
		t.Errorf("eski sürüm: %d", res.status)
	}
	if res := do("PUT", "/api/v1/sessions/"+id, f.teacher, map[string]any{"cancelled": true}, "If-Match", `"2"`); res.status != 409 {
		t.Errorf("yoklaması alınmış oturumun iptali: %d %s", res.status, res.raw)
	}

	// Telafi oturumu.
	start := day(3).Add(18 * time.Hour)
	res = do("POST", "/api/v1/sections/"+f.section+"/sessions", f.teacher, map[string]any{"session_type": "THEORY",
		"starts_at": start, "ends_at": start.Add(2 * time.Hour), "topic": "Telafi"})
	if res.status != 201 || res.body["kind"] != "MAKEUP" || res.body["classroom"] != nil {
		t.Errorf("telafi: %d %s", res.status, res.raw)
	}

	// Özet ve öğrencinin raporu.
	sum := do("GET", "/api/v1/sections/"+f.section+"/attendance-summary", f.teacher, nil)
	if sum.status != 200 || len(sum.body["items"].([]any)) != 3 {
		t.Fatalf("özet: %d %s", sum.status, sum.raw)
	}
	mine := do("GET", "/api/v1/me/attendance", ali, nil)
	if mine.status != 200 || len(mine.body["items"].([]any)) != 1 {
		t.Fatalf("öğrencinin raporu: %d %s", mine.status, mine.raw)
	}
	course := mine.body["items"].([]any)[0].(map[string]any)
	usage := course["usage"].([]any)
	if len(usage) != 2 || usage[1].(map[string]any)["absences"] != 1.0 || usage[1].(map[string]any)["limit"] != 1.0 {
		t.Errorf("devamsızlık: %v", usage)
	}
	if res := do("GET", "/api/v1/me/attendance?term_id=x", ali, nil); res.status != 400 {
		t.Errorf("geçersiz dönem: %d", res.status)
	}
}
