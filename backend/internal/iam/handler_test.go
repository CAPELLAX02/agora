package iam_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/iam"
	"github.com/CAPELLAX02/agora/backend/internal/iam/password"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
	"github.com/CAPELLAX02/agora/backend/internal/platform/jwt"
	"github.com/CAPELLAX02/agora/backend/internal/platform/ratelimit"
	"github.com/CAPELLAX02/agora/backend/internal/platform/redistest"
)

// httpEnv, gerçek veritabanı, gerçek servis ve gerçek route'larla çalışan bir test
// sunucusudur. Saat gerçek zamandır, çünkü handler süreleri (expires_in, Retry-After)
// time.Until ile hesaplar.
type httpEnv struct {
	pool   *pgxpool.Pool
	srv    *httptest.Server
	hasher *password.Hasher
	logs   *lockedBuffer
}

// lockedBuffer, sunucu goroutine'lerinin yazdığı log'u testin güvenle okuyabilmesi
// için mutex ile korunan bir tampondur.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func newHTTPEnv(t *testing.T) *httpEnv {
	t.Helper()
	return newHTTPEnvWithLoginLimit(t, 1000) // pratikte sınırsız: hız sınırı ayrı testte
}

func newHTTPEnvWithLoginLimit(t *testing.T, loginLimit int) *httpEnv {
	t.Helper()
	pool := dbtest.New(t)
	rdb := redistest.New(t)

	_, key, _ := ed25519.GenerateKey(rand.Reader)
	signer, err := jwt.NewSigner("test", key)
	if err != nil {
		t.Fatal(err)
	}
	verifier := jwt.NewVerifier("agora", "agora-api",
		map[string]ed25519.PublicKey{"test": key.Public().(ed25519.PublicKey)}, 0)

	// Gerçek saatle tolerans penceresini beklememek için 0: kullanılmış bir token'ın
	// her tekrarı yeniden kullanım sayılır. Tolerans davranışı servis testlerinde.
	cfg := testAuthConfig
	cfg.ReuseGracePeriod = 0

	hasher := password.NewHasher(cheapParams, 4)
	revocations := iam.NewRevocationList(rdb, cfg.AccessTokenTTL)
	auth, err := iam.NewAuth(context.Background(), pool, hasher, signer, revocations, cfg, time.Now)
	if err != nil {
		t.Fatal(err)
	}

	logs := &lockedBuffer{}
	logger := slog.New(slog.NewTextHandler(logs, nil))
	limiter := ratelimit.New(rdb, "login", loginLimit, time.Minute, time.Now)

	mux := http.NewServeMux()
	rt := authz.NewRouter(mux, authn.New(verifier, revocations, logger, time.Now).Require,
		iam.NewPermissionResolver(pool, rdb, time.Hour, logger), logger)
	iam.NewHandler(auth, iam.NewRepository(pool), logger, false).Register(rt, iam.Limits{
		Login:         limiter.ByIP(logger),
		PasswordReset: ratelimit.New(rdb, "password_reset", 1000, time.Minute, time.Now).ByIP(logger),
	})

	srv := httptest.NewServer(httpx.Chain(mux, httpx.RequestID, httpx.WithClientInfo, httpx.Recover(logger)))
	t.Cleanup(srv.Close)

	return &httpEnv{pool: pool, srv: srv, hasher: hasher, logs: logs}
}

// webClient, çerezleri bir tarayıcı gibi saklayan (Path kurallarına uyan) bir istemcidir.
func (e *httpEnv) webClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar}
}

type response struct {
	status int
	header http.Header
	body   []byte
}

func (r response) json(t *testing.T, v any) {
	t.Helper()
	if err := json.Unmarshal(r.body, v); err != nil {
		t.Fatalf("gövde çözülemedi: %v\n%s", err, r.body)
	}
}

func (r response) code(t *testing.T) string {
	t.Helper()
	var p httpx.Problem
	r.json(t, &p)
	return p.Code
}

func (r response) cookie(name string) *http.Cookie {
	for _, c := range (&http.Response{Header: r.header}).Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// do, istek gönderir. body string ise olduğu gibi, değilse JSON olarak gönderilir.
func (e *httpEnv) do(t *testing.T, c *http.Client, method, path string, headers map[string]string, body any) response {
	t.Helper()

	var reader io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		reader = strings.NewReader(b)
	default:
		data, err := json.Marshal(b)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, e.srv.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	if c == nil {
		c = e.srv.Client()
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return response{status: resp.StatusCode, header: resp.Header, body: data}
}

var (
	web    = map[string]string{iam.ClientHeader: "web"}
	mobile = map[string]string{iam.ClientHeader: "mobile"}
)

type tokenBody struct {
	AccessToken        string `json:"access_token"`
	TokenType          string `json:"token_type"`
	ExpiresIn          int    `json:"expires_in"`
	RefreshToken       string `json:"refresh_token"`
	RefreshExpiresIn   int    `json:"refresh_expires_in"`
	SessionID          string `json:"session_id"`
	MustChangePassword bool   `json:"must_change_password"`
}

func credentials(username string) map[string]string {
	return map[string]string{"username": username, "password": pw}
}

func (e *httpEnv) addUser(t *testing.T, username string) string {
	t.Helper()
	ae := &authEnv{pool: e.pool}
	return ae.addUser(t, username, pw, e.hasher)
}

func TestHTTPWebFlow(t *testing.T) {
	e := newHTTPEnv(t)
	e.addUser(t, "22290001")
	browser := e.webClient(t)

	// Giriş: refresh token gövdede değil, HttpOnly çerezde.
	res := e.do(t, browser, "POST", "/api/v1/auth/login", web, credentials("22290001"))
	if res.status != http.StatusOK {
		t.Fatalf("giriş: durum = %d\n%s", res.status, res.body)
	}
	var tok tokenBody
	res.json(t, &tok)
	if tok.AccessToken == "" || tok.TokenType != "Bearer" || tok.ExpiresIn != 900 || tok.SessionID == "" {
		t.Errorf("token yanıtı = %+v", tok)
	}
	if tok.RefreshToken != "" || tok.RefreshExpiresIn != 0 {
		t.Error("web istemcisine refresh token gövdede verilmemeli")
	}
	if cc := res.header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}

	c := res.cookie(iam.RefreshCookieName)
	if c == nil {
		t.Fatal("refresh çerezi yok")
	}
	if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/api/v1/auth" || c.MaxAge != 7200 {
		t.Errorf("çerez = %+v", c)
	}
	if c.Secure {
		t.Error("secureCookie=false iken çerez Secure olmamalı")
	}

	// Access token ile korumalı uç nokta.
	res = e.do(t, browser, "GET", "/api/v1/me", map[string]string{"Authorization": "Bearer " + tok.AccessToken}, nil)
	if res.status != http.StatusOK {
		t.Fatalf("/me: durum = %d\n%s", res.status, res.body)
	}

	// Refresh: tarayıcı çerezi kendisi gönderir (Path eşleşiyor), yeni çerez gelir.
	res = e.do(t, browser, "POST", "/api/v1/auth/refresh", web, nil)
	if res.status != http.StatusOK {
		t.Fatalf("refresh: durum = %d\n%s", res.status, res.body)
	}
	rotated := res.cookie(iam.RefreshCookieName)
	if rotated == nil || rotated.Value == c.Value {
		t.Fatal("refresh çerezi dönmedi (rotasyon yok)")
	}

	// Çıkış: 204 ve çerez silinir.
	res = e.do(t, browser, "POST", "/api/v1/auth/logout", web, nil)
	if res.status != http.StatusNoContent {
		t.Fatalf("çıkış: durum = %d\n%s", res.status, res.body)
	}
	if cleared := res.cookie(iam.RefreshCookieName); cleared == nil || cleared.MaxAge >= 0 {
		t.Errorf("çıkışta çerez silinmedi: %+v", cleared)
	}

	// Çıkıştan sonra elde kalan eski çerez değeriyle refresh reddedilir.
	res = e.do(t, nil, "POST", "/api/v1/auth/refresh",
		map[string]string{iam.ClientHeader: "web", "Cookie": iam.RefreshCookieName + "=" + rotated.Value}, nil)
	if res.status != http.StatusUnauthorized || res.code(t) != "INVALID_REFRESH_TOKEN" {
		t.Errorf("çıkıştan sonra refresh: durum = %d\n%s", res.status, res.body)
	}
}

func TestHTTPMobileFlow(t *testing.T) {
	e := newHTTPEnv(t)
	e.addUser(t, "22290001")

	res := e.do(t, nil, "POST", "/api/v1/auth/login", mobile, credentials("22290001"))
	if res.status != http.StatusOK {
		t.Fatalf("giriş: durum = %d\n%s", res.status, res.body)
	}
	if res.cookie(iam.RefreshCookieName) != nil {
		t.Error("mobil istemciye çerez verilmemeli")
	}
	var tok tokenBody
	res.json(t, &tok)
	if tok.RefreshToken == "" || tok.RefreshExpiresIn != 7200 {
		t.Fatalf("mobil istemciye refresh token gövdede verilmeli: %+v", tok)
	}

	res = e.do(t, nil, "POST", "/api/v1/auth/refresh", mobile, map[string]string{"refresh_token": tok.RefreshToken})
	if res.status != http.StatusOK {
		t.Fatalf("refresh: durum = %d\n%s", res.status, res.body)
	}
	var next tokenBody
	res.json(t, &next)
	if next.RefreshToken == "" || next.RefreshToken == tok.RefreshToken || next.SessionID != tok.SessionID {
		t.Errorf("rotasyon hatalı: önce %+v, sonra %+v", tok, next)
	}

	res = e.do(t, nil, "POST", "/api/v1/auth/logout", mobile, map[string]string{"refresh_token": next.RefreshToken})
	if res.status != http.StatusNoContent {
		t.Fatalf("çıkış: durum = %d\n%s", res.status, res.body)
	}
	res = e.do(t, nil, "POST", "/api/v1/auth/refresh", mobile, map[string]string{"refresh_token": next.RefreshToken})
	if res.status != http.StatusUnauthorized {
		t.Errorf("çıkıştan sonra refresh: durum = %d", res.status)
	}
}

func TestHTTPLoginErrors(t *testing.T) {
	e := newHTTPEnv(t)
	e.addUser(t, "22290001")
	disabledID := e.addUser(t, "22290002")
	if _, err := e.pool.Exec(context.Background(),
		`UPDATE iam.users SET status = 'SUSPENDED' WHERE id = $1`, disabledID); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		headers    map[string]string
		body       any
		wantStatus int
		wantCode   string
	}{
		{"istemci başlığı yok", nil, credentials("22290001"), 400, "INVALID_CLIENT"},
		{"bilinmeyen istemci", map[string]string{iam.ClientHeader: "desktop"}, credentials("22290001"), 400, "INVALID_CLIENT"},
		{"yanlış parola", web, map[string]string{"username": "22290001", "password": "yanlis parola 123"}, 401, "INVALID_CREDENTIALS"},
		{"olmayan kullanıcı", web, credentials("99999999"), 401, "INVALID_CREDENTIALS"},
		{"askıdaki hesap", web, credentials("22290002"), 403, "ACCOUNT_DISABLED"},
		{"boş alanlar", web, map[string]string{"username": "  ", "password": ""}, 400, "VALIDATION_FAILED"},
		{"NUL içeren kullanıcı adı", web, map[string]string{"username": "a\x00b", "password": "x"}, 400, "VALIDATION_FAILED"},
		{"çok uzun kullanıcı adı", web, map[string]string{"username": strings.Repeat("a", 65), "password": "x"}, 400, "VALIDATION_FAILED"},
		{"bilinmeyen alan", web, `{"username":"a","password":"b","role":"admin"}`, 400, "INVALID_BODY"},
		{"bozuk JSON", web, `{"username":`, 400, "INVALID_BODY"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := e.do(t, nil, "POST", "/api/v1/auth/login", tt.headers, tt.body)
			if res.status != tt.wantStatus || res.code(t) != tt.wantCode {
				t.Errorf("durum = %d, want %d %s\n%s", res.status, tt.wantStatus, tt.wantCode, res.body)
			}
		})
	}

	t.Run("form gövdesi reddedilir", func(t *testing.T) {
		req, _ := http.NewRequest("POST", e.srv.URL+"/api/v1/auth/login", strings.NewReader("username=a&password=b"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set(iam.ClientHeader, "web")
		resp, err := e.srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnsupportedMediaType {
			t.Errorf("durum = %d, want 415", resp.StatusCode)
		}
	})
}

func TestHTTPLoginLockout(t *testing.T) {
	e := newHTTPEnv(t)
	e.addUser(t, "22290001")
	wrong := map[string]string{"username": "22290001", "password": "yanlis parola 123"}

	for i := 1; i <= 4; i++ {
		if res := e.do(t, nil, "POST", "/api/v1/auth/login", web, wrong); res.status != 401 {
			t.Fatalf("%d. deneme: durum = %d", i, res.status)
		}
	}

	res := e.do(t, nil, "POST", "/api/v1/auth/login", web, wrong)
	if res.status != http.StatusTooManyRequests || res.code(t) != "ACCOUNT_LOCKED" {
		t.Fatalf("5. deneme: durum = %d\n%s", res.status, res.body)
	}
	retry, err := strconv.Atoi(res.header.Get("Retry-After"))
	if err != nil || retry < 59 || retry > 60 {
		t.Errorf("Retry-After = %q, ~60 bekleniyordu", res.header.Get("Retry-After"))
	}

	// Kilitliyken doğru parola da reddedilir.
	if res := e.do(t, nil, "POST", "/api/v1/auth/login", web, credentials("22290001")); res.status != 429 {
		t.Errorf("kilitliyken doğru parola: durum = %d", res.status)
	}
}

func TestHTTPRefreshErrors(t *testing.T) {
	e := newHTTPEnv(t)

	t.Run("web: çerez yok", func(t *testing.T) {
		res := e.do(t, nil, "POST", "/api/v1/auth/refresh", web, nil)
		if res.status != 401 || res.code(t) != "INVALID_REFRESH_TOKEN" {
			t.Errorf("durum = %d\n%s", res.status, res.body)
		}
		if c := res.cookie(iam.RefreshCookieName); c == nil || c.MaxAge >= 0 {
			t.Error("geçersiz refresh'te çerez silinmeli")
		}
	})

	t.Run("web: başlık yoksa çerez okunmaz", func(t *testing.T) {
		// Başka bir sitenin formu çerezi gönderebilir ama X-Agora-Client başlığını ekleyemez.
		res := e.do(t, nil, "POST", "/api/v1/auth/refresh",
			map[string]string{"Cookie": iam.RefreshCookieName + "=herhangi"}, nil)
		if res.status != 400 || res.code(t) != "INVALID_CLIENT" {
			t.Errorf("durum = %d\n%s", res.status, res.body)
		}
	})

	t.Run("mobil: gövde yok", func(t *testing.T) {
		res := e.do(t, nil, "POST", "/api/v1/auth/refresh", mobile, nil)
		if res.status != 415 {
			t.Errorf("durum = %d\n%s", res.status, res.body)
		}
	})

	t.Run("mobil: bilinmeyen token", func(t *testing.T) {
		res := e.do(t, nil, "POST", "/api/v1/auth/refresh", mobile, map[string]string{"refresh_token": "uydurma"})
		if res.status != 401 || res.code(t) != "INVALID_REFRESH_TOKEN" {
			t.Errorf("durum = %d\n%s", res.status, res.body)
		}
	})

	t.Run("çıkış her zaman başarılı", func(t *testing.T) {
		res := e.do(t, nil, "POST", "/api/v1/auth/logout", mobile, map[string]string{"refresh_token": "uydurma"})
		if res.status != http.StatusNoContent {
			t.Errorf("durum = %d\n%s", res.status, res.body)
		}
	})
}

func TestHTTPRefreshReuse(t *testing.T) {
	e := newHTTPEnv(t)
	e.addUser(t, "22290001")
	browser := e.webClient(t)

	res := e.do(t, browser, "POST", "/api/v1/auth/login", web, credentials("22290001"))
	stolen := res.cookie(iam.RefreshCookieName).Value

	// Meşru kullanıcı refresh yapar, tarayıcısında yeni çerez var.
	if res := e.do(t, browser, "POST", "/api/v1/auth/refresh", web, nil); res.status != 200 {
		t.Fatalf("refresh: durum = %d", res.status)
	}

	// Saldırgan çaldığı eski token'ı kullanır.
	res = e.do(t, nil, "POST", "/api/v1/auth/refresh",
		map[string]string{iam.ClientHeader: "web", "Cookie": iam.RefreshCookieName + "=" + stolen}, nil)
	if res.status != 401 || res.code(t) != "INVALID_REFRESH_TOKEN" {
		t.Fatalf("yeniden kullanım: durum = %d\n%s", res.status, res.body)
	}
	if c := res.cookie(iam.RefreshCookieName); c == nil || c.MaxAge >= 0 {
		t.Error("yeniden kullanımda çerez silinmeli")
	}
	if !strings.Contains(e.logs.String(), "refresh token yeniden kullanıldı") {
		t.Errorf("güvenlik olayı log'a yazılmadı:\n%s", e.logs.String())
	}

	// Oturum iptal edildiği için meşru kullanıcının yeni çerezi de artık geçersiz.
	if res := e.do(t, browser, "POST", "/api/v1/auth/refresh", web, nil); res.status != 401 {
		t.Errorf("iptal edilen oturumda refresh: durum = %d", res.status)
	}
}

// TestHTTPLogoutRevokesAccessToken, çıkıştan sonra elde kalan access token'ın
// süresi dolmadan reddedildiğini doğrular.
func TestHTTPLogoutRevokesAccessToken(t *testing.T) {
	e := newHTTPEnv(t)
	e.addUser(t, "22290001")

	res := e.do(t, nil, "POST", "/api/v1/auth/login", mobile, credentials("22290001"))
	var tok tokenBody
	res.json(t, &tok)
	bearer := map[string]string{"Authorization": "Bearer " + tok.AccessToken}

	if res := e.do(t, nil, "GET", "/api/v1/me", bearer, nil); res.status != http.StatusOK {
		t.Fatalf("çıkıştan önce /me: durum = %d", res.status)
	}

	if res := e.do(t, nil, "POST", "/api/v1/auth/logout", mobile,
		map[string]string{"refresh_token": tok.RefreshToken}); res.status != http.StatusNoContent {
		t.Fatalf("çıkış: durum = %d", res.status)
	}

	// Token'ın imzası ve süresi hâlâ geçerli, ama oturum iptal edildi.
	res = e.do(t, nil, "GET", "/api/v1/me", bearer, nil)
	if res.status != http.StatusUnauthorized || res.code(t) != "SESSION_REVOKED" {
		t.Errorf("çıkıştan sonra /me: durum = %d\n%s", res.status, res.body)
	}
}

func TestHTTPLoginRateLimit(t *testing.T) {
	e := newHTTPEnvWithLoginLimit(t, 3)
	e.addUser(t, "22290001")

	// Sınır IP başına: doğru ya da yanlış, her deneme bir hak tüketir.
	for i := 1; i <= 3; i++ {
		if res := e.do(t, nil, "POST", "/api/v1/auth/login", web, credentials("22290001")); res.status != http.StatusOK {
			t.Fatalf("%d. giriş: durum = %d", i, res.status)
		}
	}

	res := e.do(t, nil, "POST", "/api/v1/auth/login", web, credentials("22290001"))
	if res.status != http.StatusTooManyRequests || res.code(t) != "RATE_LIMITED" {
		t.Fatalf("4. giriş: durum = %d\n%s", res.status, res.body)
	}
	if res.header.Get("Retry-After") == "" {
		t.Error("Retry-After yok")
	}

	// Hız sınırı sadece girişte: diğer auth uçları etkilenmez.
	if res := e.do(t, nil, "POST", "/api/v1/auth/refresh", web, nil); res.status != http.StatusUnauthorized {
		t.Errorf("refresh hız sınırına takılmamalı: durum = %d", res.status)
	}
}

func TestHTTPMe(t *testing.T) {
	e := newHTTPEnv(t)
	userID := e.addUser(t, "P10002")
	deptID, _ := seedOrg(t, e.pool)
	repo := iam.NewRepository(e.pool)
	ctx := context.Background()
	if err := repo.AssignRole(ctx, userID, "INSTRUCTOR", iam.ScopeDepartment, deptID); err != nil {
		t.Fatal(err)
	}
	if err := repo.AssignRole(ctx, userID, "CENTRAL_REGISTRAR", iam.ScopeUniversity, ""); err != nil {
		t.Fatal(err)
	}

	res := e.do(t, nil, "POST", "/api/v1/auth/login", mobile, credentials("P10002"))
	var tok tokenBody
	res.json(t, &tok)

	res = e.do(t, nil, "GET", "/api/v1/me", map[string]string{"Authorization": "bearer " + tok.AccessToken}, nil)
	if res.status != http.StatusOK {
		t.Fatalf("durum = %d\n%s", res.status, res.body)
	}

	var me struct {
		ID          string     `json:"id"`
		Username    string     `json:"username"`
		FirstName   string     `json:"first_name"`
		LastName    string     `json:"last_name"`
		LastLoginAt *time.Time `json:"last_login_at"`
		SessionID   string     `json:"session_id"`
		Roles       []struct {
			Role      string  `json:"role"`
			RoleName  string  `json:"role_name"`
			ScopeType string  `json:"scope_type"`
			ScopeID   *string `json:"scope_id"`
			ScopeName *string `json:"scope_name"`
		} `json:"roles"`
	}
	res.json(t, &me)

	if me.ID != userID || me.Username != "P10002" || me.FirstName != "Test" || me.LastName != "P10002" {
		t.Errorf("profil = %+v", me)
	}
	if me.LastLoginAt == nil || me.SessionID != tok.SessionID {
		t.Errorf("son giriş veya oturum eksik: %+v", me)
	}
	if len(me.Roles) != 2 {
		t.Fatalf("%d rol döndü, 2 bekleniyordu: %s", len(me.Roles), res.body)
	}

	central, instructor := me.Roles[0], me.Roles[1] // rol koduna göre sıralı
	if central.Role != "CENTRAL_REGISTRAR" || central.ScopeID != nil || central.ScopeName != nil {
		t.Errorf("üniversite kapsamlı rol = %+v", central)
	}
	if instructor.Role != "INSTRUCTOR" || instructor.ScopeType != "DEPARTMENT" ||
		instructor.ScopeID == nil || *instructor.ScopeID != deptID ||
		instructor.ScopeName == nil || *instructor.ScopeName != "Bilgisayar Mühendisliği" {
		t.Errorf("bölüm kapsamlı rol = %+v", instructor)
	}
	if instructor.RoleName == "" {
		t.Error("rol adı boş")
	}
}

func TestHTTPMeRequiresToken(t *testing.T) {
	e := newHTTPEnv(t)

	tests := []struct {
		name          string
		authorization string
		wantCode      string
		wantChallenge string
	}{
		{"başlık yok", "", "AUTHENTICATION_REQUIRED", `Bearer realm="agora"`},
		{"başka şema", "Basic YWxpOnBhcm9sYQ==", "AUTHENTICATION_REQUIRED", `Bearer realm="agora"`},
		{"boş token", "Bearer ", "AUTHENTICATION_REQUIRED", `Bearer realm="agora"`},
		{"sahte token", "Bearer a.b.c", "INVALID_TOKEN", `Bearer realm="agora", error="invalid_token"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var headers map[string]string
			if tt.authorization != "" {
				headers = map[string]string{"Authorization": tt.authorization}
			}
			res := e.do(t, nil, "GET", "/api/v1/me", headers, nil)
			if res.status != http.StatusUnauthorized || res.code(t) != tt.wantCode {
				t.Errorf("durum = %d\n%s", res.status, res.body)
			}
			if got := res.header.Get("WWW-Authenticate"); got != tt.wantChallenge {
				t.Errorf("WWW-Authenticate = %q, want %q", got, tt.wantChallenge)
			}
		})
	}
}

func (e *httpEnv) bearer(t *testing.T, username string) map[string]string {
	t.Helper()
	res := e.do(t, nil, "POST", "/api/v1/auth/login", mobile, credentials(username))
	if res.status != http.StatusOK {
		t.Fatalf("%s girişi: durum = %d\n%s", username, res.status, res.body)
	}
	var tok tokenBody
	res.json(t, &tok)
	return map[string]string{"Authorization": "Bearer " + tok.AccessToken}
}

func TestHTTPPermissions(t *testing.T) {
	e := newHTTPEnv(t)
	ctx := context.Background()
	repo := iam.NewRepository(e.pool)

	adminID := e.addUser(t, "P90001")
	if err := repo.AssignRole(ctx, adminID, "SYSTEM_ADMIN", iam.ScopeUniversity, ""); err != nil {
		t.Fatal(err)
	}
	headID := e.addUser(t, "P10002")
	deptID, _ := seedOrg(t, e.pool)
	if err := repo.AssignRole(ctx, headID, "DEPARTMENT_HEAD", iam.ScopeDepartment, deptID); err != nil {
		t.Fatal(err)
	}
	studentID := e.addUser(t, "22290001")
	if err := repo.AssignRole(ctx, studentID, "STUDENT", iam.ScopeNone, ""); err != nil {
		t.Fatal(err)
	}

	admin, head, student := e.bearer(t, "P90001"), e.bearer(t, "P10002"), e.bearer(t, "22290001")

	t.Run("kendi yetkileri", func(t *testing.T) {
		res := e.do(t, nil, "GET", "/api/v1/me/permissions", head, nil)
		if res.status != http.StatusOK {
			t.Fatalf("durum = %d\n%s", res.status, res.body)
		}
		var body struct {
			Items []struct {
				Permission string  `json:"permission"`
				ScopeType  string  `json:"scope_type"`
				ScopeID    *string `json:"scope_id"`
			} `json:"items"`
		}
		res.json(t, &body)

		var found bool
		for _, g := range body.Items {
			if g.Permission == "quota:manage" {
				found = g.ScopeType == "DEPARTMENT" && g.ScopeID != nil && *g.ScopeID == deptID
			}
			if g.Permission == "role:assign" {
				t.Error("bölüm başkanında rol atama yetkisi olmamalı")
			}
		}
		if !found {
			t.Errorf("quota:manage bölüm kapsamıyla dönmedi: %s", res.body)
		}
	})

	t.Run("user:read gerektiren uç", func(t *testing.T) {
		if res := e.do(t, nil, "GET", "/api/v1/users/"+headID, admin, nil); res.status != http.StatusOK {
			t.Errorf("yönetici: durum = %d\n%s", res.status, res.body)
		}
		for name, h := range map[string]map[string]string{"öğrenci": student, "bölüm başkanı": head} {
			res := e.do(t, nil, "GET", "/api/v1/users/"+adminID, h, nil)
			if res.status != http.StatusForbidden || res.code(t) != "FORBIDDEN" {
				t.Errorf("%s: durum = %d\n%s", name, res.status, res.body)
			}
		}
		if res := e.do(t, nil, "GET", "/api/v1/users/"+headID, nil, nil); res.status != http.StatusUnauthorized {
			t.Errorf("token yok: durum = %d", res.status)
		}
		if res := e.do(t, nil, "GET", "/api/v1/users/01a11b7f-0000-7000-8000-000000000000", admin, nil); res.status != http.StatusNotFound {
			t.Errorf("olmayan kullanıcı: durum = %d", res.status)
		}
		if res := e.do(t, nil, "GET", "/api/v1/users/bozuk-id", admin, nil); res.status != http.StatusNotFound {
			t.Errorf("geçersiz kimlik: durum = %d", res.status)
		}
	})

	t.Run("rol ataması anında etkili", func(t *testing.T) {
		if err := repo.AssignRole(ctx, studentID, "AUDITOR", iam.ScopeUniversity, ""); err != nil {
			t.Fatal(err)
		}
		// Aynı access token: yetkiler token'da değil, her istekte sunucuda çözülüyor.
		if res := e.do(t, nil, "GET", "/api/v1/users/"+adminID, student, nil); res.status != http.StatusOK {
			t.Errorf("rol atandıktan sonra: durum = %d\n%s", res.status, res.body)
		}
	})

	t.Run("askıya alınan hesap anında durur", func(t *testing.T) {
		if _, err := e.pool.Exec(ctx, `UPDATE iam.users SET status = 'SUSPENDED' WHERE id = $1`, headID); err != nil {
			t.Fatal(err)
		}
		res := e.do(t, nil, "GET", "/api/v1/me", head, nil)
		if res.status != http.StatusForbidden || res.code(t) != "ACCOUNT_DISABLED" {
			t.Errorf("askıdaki hesapla /me: durum = %d\n%s", res.status, res.body)
		}
	})
}

// TestHTTPMustChangePassword, ilk girişte parola değiştirme zorunluluğunun uçtan uca
// işlediğini doğrular: kullanıcı sadece kendi hesabıyla ilgili uçlara erişebilir,
// parolasını değiştirince her şey açılır.
func TestHTTPMustChangePassword(t *testing.T) {
	e := newHTTPEnv(t)
	ctx := context.Background()
	userID := e.addUser(t, "P90002")
	if err := iam.NewRepository(e.pool).AssignRole(ctx, userID, "SYSTEM_ADMIN", iam.ScopeUniversity, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, `UPDATE iam.users SET must_change_password = true WHERE id = $1`, userID); err != nil {
		t.Fatal(err)
	}

	res := e.do(t, nil, "POST", "/api/v1/auth/login", mobile, credentials("P90002"))
	var tok tokenBody
	res.json(t, &tok)
	if !tok.MustChangePassword {
		t.Fatal("giriş yanıtı must_change_password = true demeli")
	}
	bearer := map[string]string{"Authorization": "Bearer " + tok.AccessToken}

	if res := e.do(t, nil, "GET", "/api/v1/me", bearer, nil); res.status != http.StatusOK {
		t.Errorf("/me açık olmalı: %d", res.status)
	}
	res = e.do(t, nil, "GET", "/api/v1/users/"+userID, bearer, nil)
	if res.status != http.StatusForbidden || res.code(t) != "PASSWORD_CHANGE_REQUIRED" {
		t.Fatalf("parola değişmeden yetkili uç: %d\n%s", res.status, res.body)
	}

	t.Run("politika ihlali alan koduyla döner", func(t *testing.T) {
		res := e.do(t, nil, "POST", "/api/v1/me/password", bearer,
			map[string]string{"current_password": pw, "new_password": "kisa"})
		var p struct {
			Code   string `json:"code"`
			Errors []struct {
				Field string `json:"field"`
				Code  string `json:"code"`
			} `json:"errors"`
		}
		res.json(t, &p)
		if res.status != 400 || p.Code != "VALIDATION_FAILED" || len(p.Errors) == 0 ||
			p.Errors[0].Field != "new_password" || p.Errors[0].Code != "TOO_SHORT" {
			t.Errorf("yanıt = %d %s", res.status, res.body)
		}
	})

	t.Run("yanlış mevcut parola", func(t *testing.T) {
		res := e.do(t, nil, "POST", "/api/v1/me/password", bearer,
			map[string]string{"current_password": "yanlış parola 99", "new_password": newPW})
		if res.status != 400 || res.code(t) != "INVALID_CURRENT_PASSWORD" {
			t.Errorf("yanıt = %d %s", res.status, res.body)
		}
	})

	res = e.do(t, nil, "POST", "/api/v1/me/password", bearer,
		map[string]string{"current_password": pw, "new_password": newPW})
	if res.status != http.StatusNoContent {
		t.Fatalf("parola değiştirme: %d\n%s", res.status, res.body)
	}

	// Aynı access token ile artık yetkili uçlar açık: zorunluluk her istekte güncel okunur.
	if res := e.do(t, nil, "GET", "/api/v1/users/"+userID, bearer, nil); res.status != http.StatusOK {
		t.Errorf("parola değiştikten sonra: %d\n%s", res.status, res.body)
	}
}

func TestHTTPPasswordReset(t *testing.T) {
	e := newHTTPEnv(t)
	e.addUser(t, "22290040")

	// Var olan ve olmayan hesap için yanıt birebir aynı olmalı: durum ve gövde.
	known := e.do(t, nil, "POST", "/api/v1/auth/password/forgot", nil, map[string]string{"identifier": "22290040"})
	unknown := e.do(t, nil, "POST", "/api/v1/auth/password/forgot", nil, map[string]string{"identifier": "yok@agora.test"})
	if known.status != http.StatusAccepted || unknown.status != http.StatusAccepted ||
		string(known.body) != string(unknown.body) {
		t.Fatalf("yanıtlar farklı: %d %q / %d %q", known.status, known.body, unknown.status, unknown.body)
	}
	if res := e.do(t, nil, "POST", "/api/v1/auth/password/forgot", nil, map[string]string{"identifier": " "}); res.status != 400 {
		t.Errorf("boş kimlik: %d", res.status)
	}

	var link string
	if err := e.pool.QueryRow(context.Background(),
		`SELECT payload->>'link' FROM communication.email_outbox ORDER BY created_at DESC LIMIT 1`).Scan(&link); err != nil {
		t.Fatal(err)
	}
	_, token, _ := strings.Cut(link, "#token=")

	res := e.do(t, nil, "POST", "/api/v1/auth/password/reset/verify", nil, map[string]string{"token": token})
	var info struct {
		Purpose   string    `json:"purpose"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	res.json(t, &info)
	if res.status != 200 || info.Purpose != "RESET" || info.ExpiresAt.IsZero() {
		t.Errorf("doğrulama = %d %s", res.status, res.body)
	}

	res = e.do(t, nil, "POST", "/api/v1/auth/password/reset", nil, map[string]string{"token": token, "new_password": "kisa"})
	if res.status != 400 || res.code(t) != "VALIDATION_FAILED" {
		t.Errorf("politika ihlali: %d %s", res.status, res.body)
	}

	res = e.do(t, nil, "POST", "/api/v1/auth/password/reset", nil, map[string]string{"token": token, "new_password": newPW})
	if res.status != http.StatusNoContent {
		t.Fatalf("sıfırlama: %d %s", res.status, res.body)
	}
	if res := e.do(t, nil, "POST", "/api/v1/auth/login", mobile,
		map[string]string{"username": "22290040", "password": newPW}); res.status != 200 {
		t.Errorf("yeni parolayla giriş: %d", res.status)
	}

	res = e.do(t, nil, "POST", "/api/v1/auth/password/reset", nil, map[string]string{"token": token, "new_password": newPW + "x"})
	if res.status != 400 || res.code(t) != "INVALID_RESET_TOKEN" {
		t.Errorf("kullanılmış bağlantı: %d %s", res.status, res.body)
	}
}

func TestHTTPSessions(t *testing.T) {
	e := newHTTPEnv(t)
	e.addUser(t, "22290060")
	e.addUser(t, "22290061")

	login := func(username, ua string) tokenBody {
		res := e.do(t, nil, "POST", "/api/v1/auth/login",
			map[string]string{iam.ClientHeader: "mobile", "User-Agent": ua}, credentials(username))
		var tok tokenBody
		res.json(t, &tok)
		return tok
	}
	laptop := login("22290060", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/140.0.0.0 Safari/537.36")
	phone := login("22290060", "Mozilla/5.0 (iPhone; CPU iPhone OS 26_0 like Mac OS X) Version/26.0 Mobile/15E148 Safari/604.1")
	stranger := login("22290061", "curl/8")
	bearer := map[string]string{"Authorization": "Bearer " + laptop.AccessToken}

	res := e.do(t, nil, "GET", "/api/v1/me/sessions", bearer, nil)
	var list struct {
		Items []struct {
			ID      string  `json:"id"`
			Browser *string `json:"browser"`
			OS      *string `json:"os"`
			Device  string  `json:"device"`
			IP      *string `json:"ip"`
			Current bool    `json:"current"`
		} `json:"items"`
	}
	res.json(t, &list)
	if res.status != 200 || len(list.Items) != 2 {
		t.Fatalf("oturumlar = %d %s", res.status, res.body)
	}
	for _, s := range list.Items {
		switch s.ID {
		case laptop.SessionID:
			if !s.Current || s.Browser == nil || *s.Browser != "Chrome 140" || *s.OS != "Windows" || s.IP == nil {
				t.Errorf("dizüstü oturumu = %+v", s)
			}
		case phone.SessionID:
			if s.Current || s.Device != "MOBILE" || *s.OS != "iOS" {
				t.Errorf("telefon oturumu = %+v", s)
			}
		default:
			t.Errorf("başka kullanıcının oturumu listelendi: %s", s.ID)
		}
	}

	if res := e.do(t, nil, "DELETE", "/api/v1/me/sessions/"+stranger.SessionID, bearer, nil); res.status != 404 {
		t.Errorf("başkasının oturumu: %d", res.status)
	}
	if res := e.do(t, nil, "DELETE", "/api/v1/me/sessions/"+phone.SessionID, bearer, nil); res.status != http.StatusNoContent {
		t.Fatalf("telefonu kapatma: %d %s", res.status, res.body)
	}
	// Kapatılan oturumun access token'ı da hemen geçersiz.
	if res := e.do(t, nil, "GET", "/api/v1/me", map[string]string{"Authorization": "Bearer " + phone.AccessToken}, nil); res.status != 401 {
		t.Errorf("kapatılan oturumla /me: %d", res.status)
	}

	other := login("22290060", "ikinci cihaz")
	res = e.do(t, nil, "DELETE", "/api/v1/me/sessions", bearer, nil)
	var ended struct {
		Revoked int `json:"revoked"`
	}
	res.json(t, &ended)
	if res.status != 200 || ended.Revoked != 1 {
		t.Errorf("diğer oturumları kapatma = %d %s", res.status, res.body)
	}
	if res := e.do(t, nil, "GET", "/api/v1/me", map[string]string{"Authorization": "Bearer " + other.AccessToken}, nil); res.status != 401 {
		t.Errorf("kapatılan diğer oturum: %d", res.status)
	}
	if res := e.do(t, nil, "GET", "/api/v1/me", bearer, nil); res.status != 200 {
		t.Errorf("isteği yapan oturum açık kalmalı: %d", res.status)
	}
}
