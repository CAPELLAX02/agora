package authz

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
)

func TestPermissionsAllows(t *testing.T) {
	p := NewPermissions("u1", []Grant{
		{Permission: "grade:read_scoped", ScopeType: ScopeFaculty, ScopeID: "muh"},
		{Permission: "quota:manage", ScopeType: ScopeDepartment, ScopeID: "bil"},
		{Permission: "course:read", ScopeType: ScopeUniversity},
		{Permission: "grade:read_own", ScopeType: ScopeNone},
		{Permission: "curriculum:manage", ScopeType: ScopeProgram, ScopeID: "bil-lisans"},
	})

	bilDept := Target{FacultyID: "muh", DepartmentID: "bil"}
	fizDept := Target{FacultyID: "fen", DepartmentID: "fiz"}
	bilProgram := Target{FacultyID: "muh", DepartmentID: "bil", ProgramID: "bil-lisans"}

	tests := []struct {
		name       string
		permission string
		target     Target
		want       bool
	}{
		{"fakülte kapsamı kendi bölümünü kapsar", "grade:read_scoped", bilDept, true},
		{"fakülte kapsamı başka fakülteyi kapsamaz", "grade:read_scoped", fizDept, false},
		{"bölüm kapsamı kendi bölümünü kapsar", "quota:manage", bilDept, true},
		{"bölüm kapsamı başka bölümü kapsamaz", "quota:manage", fizDept, false},
		{"bölüm kapsamı kendi programını kapsar", "quota:manage", bilProgram, true},
		{"bölüm kapsamı fakülteyi kapsamaz", "quota:manage", Target{FacultyID: "muh"}, false},
		{"program kapsamı sadece kendi programı", "curriculum:manage", bilProgram, true},
		{"program kapsamı bölümü kapsamaz", "curriculum:manage", bilDept, false},
		{"üniversite kapsamı her şeyi kapsar", "course:read", fizDept, true},
		{"NONE kapsamı hiçbir birimi kapsamaz", "grade:read_own", bilDept, false},
		{"olmayan yetki", "role:assign", bilDept, false},
		{"boş hedef kimseyle eşleşmez", "grade:read_scoped", Target{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := p.Allows(tt.permission, tt.target); got != tt.want {
				t.Errorf("Allows(%q, %+v) = %v, want %v", tt.permission, tt.target, got, tt.want)
			}
		})
	}

	if !p.Has("grade:read_own") || p.Has("role:assign") {
		t.Error("Has, kapsamdan bağımsız olarak yetkinin varlığını söylemeli")
	}
}

func TestPermissionsGrantsSorted(t *testing.T) {
	p := NewPermissions("u1", []Grant{
		{Permission: "b:x", ScopeType: ScopeUniversity},
		{Permission: "a:x", ScopeType: ScopeFaculty, ScopeID: "2"},
		{Permission: "a:x", ScopeType: ScopeFaculty, ScopeID: "1"},
	})
	want := []Grant{
		{Permission: "a:x", ScopeType: ScopeFaculty, ScopeID: "1"},
		{Permission: "a:x", ScopeType: ScopeFaculty, ScopeID: "2"},
		{Permission: "b:x", ScopeType: ScopeUniversity},
	}
	if got := p.Grants(); !reflect.DeepEqual(got, want) {
		t.Errorf("Grants() = %+v", got)
	}
	if got := NewPermissions("u1", nil).Grants(); got == nil || len(got) != 0 {
		t.Errorf("yetkisiz kullanıcıda boş (nil olmayan) liste dönmeli: %#v", got)
	}
}

// fakeResolver, kullanıcı kimliğine göre sabit yetkiler ya da hata döndürür.
type fakeResolver struct {
	grants map[string][]Grant
	err    error
	calls  int
}

func (f *fakeResolver) Permissions(ctx context.Context, userID string) (*Permissions, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return NewPermissions(userID, f.grants[userID]), nil
}

// fakeAuthenticate, "X-Test-User" başlığındaki kullanıcıyı kimliği doğrulanmış sayar.
func fakeAuthenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := r.Header.Get("X-Test-User")
		if user == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(authn.WithPrincipal(r.Context(), authn.Principal{UserID: user})))
	})
}

func newTestRouter(resolver Resolver) (*Router, *http.ServeMux) {
	mux := http.NewServeMux()
	return NewRouter(mux, fakeAuthenticate, resolver, slog.New(slog.NewTextHandler(io.Discard, nil))), mux
}

func serve(h http.Handler, path, user string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if user != "" {
		req.Header.Set("X-Test-User", user)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRouterPolicies(t *testing.T) {
	resolver := &fakeResolver{grants: map[string][]Grant{
		"admin": {{Permission: "user:read", ScopeType: ScopeUniversity}},
	}}
	rt, mux := newTestRouter(resolver)

	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, has := PermissionsFrom(r.Context()); has {
			w.Header().Set("X-Has-Permissions", "1")
		}
	})
	rt.Handle("GET /public", Public, ok)
	rt.Handle("GET /authenticated", Authenticated, ok)
	rt.Handle("GET /users", Permission("user:read"), ok)

	tests := []struct {
		path, user string
		wantStatus int
		wantCode   string
	}{
		{"/public", "", 200, ""},
		{"/authenticated", "", 401, ""},
		{"/authenticated", "ogrenci", 200, ""},
		{"/users", "", 401, ""},
		{"/users", "ogrenci", 403, "FORBIDDEN"},
		{"/users", "admin", 200, ""},
	}
	for _, tt := range tests {
		t.Run(tt.path+" "+tt.user, func(t *testing.T) {
			rec := serve(mux, tt.path, tt.user)
			if rec.Code != tt.wantStatus {
				t.Fatalf("durum = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantCode != "" && !strings.Contains(rec.Body.String(), `"code":"`+tt.wantCode+`"`) {
				t.Errorf("gövde = %s", rec.Body.String())
			}
			if rec.Code == 200 && tt.path != "/public" && rec.Header().Get("X-Has-Permissions") != "1" {
				t.Error("yetkiler context'e konmadı")
			}
		})
	}

	// Herkese açık route'lar yetki çözümüne hiç gitmez (gereksiz DB ve Redis yükü olmasın).
	resolver.calls = 0
	serve(mux, "/public", "admin")
	if resolver.calls != 0 {
		t.Errorf("public route'ta resolver %d kez çağrıldı", resolver.calls)
	}
}

func TestRouterInactiveAccount(t *testing.T) {
	rt, mux := newTestRouter(&fakeResolver{err: ErrAccountInactive})
	rt.Handle("GET /me", Authenticated, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("aktif olmayan hesabın isteği handler'a ulaşmamalı")
	}))

	rec := serve(mux, "/me", "askida")
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"code":"ACCOUNT_DISABLED"`) {
		t.Errorf("yanıt = %d %s", rec.Code, rec.Body.String())
	}
}

func TestRouterResolverError(t *testing.T) {
	rt, mux := newTestRouter(&fakeResolver{err: errors.New("veritabanı yok")})
	rt.Handle("GET /me", Authenticated, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	if rec := serve(mux, "/me", "u1"); rec.Code != http.StatusInternalServerError {
		t.Errorf("durum = %d, want 500", rec.Code)
	}
}

// TestRouterMiddlewareOrder, ek middleware'lerin politika kontrolünden sonra
// çalıştığını doğrular: yetkisiz bir istek onlara hiç ulaşmaz.
func TestRouterMiddlewareOrder(t *testing.T) {
	rt, mux := newTestRouter(&fakeResolver{})
	var reached bool
	mw := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reached = true
			next.ServeHTTP(w, r)
		})
	}
	rt.Handle("GET /x", Permission("user:read"), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), mw)

	serve(mux, "/x", "ogrenci")
	if reached {
		t.Error("yetkisiz istek middleware'e ulaştı")
	}
}

// TestRouterRequiresPolicy, politika verilmeden kaydedilen route'un uygulama
// açılırken panic'e yol açtığını doğrular: varsayılan ret.
func TestRouterRequiresPolicy(t *testing.T) {
	rt, _ := newTestRouter(&fakeResolver{})
	defer func() {
		r := recover()
		if r == nil || !strings.Contains(r.(string), `"GET /unutulan"`) {
			t.Errorf("politikasız route panic'e yol açmalıydı, recover = %v", r)
		}
	}()
	rt.Handle("GET /unutulan", Policy{}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
}

func TestPolicyString(t *testing.T) {
	for p, want := range map[Policy]string{
		Public:                  "public",
		Authenticated:           "authenticated",
		Permission("user:read"): "permission:user:read",
		{}:                      "tanımsız",
	} {
		if got := p.String(); got != want {
			t.Errorf("String() = %q, want %q", got, want)
		}
	}
}
