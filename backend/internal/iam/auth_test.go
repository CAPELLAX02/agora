package iam_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/iam"
	"github.com/CAPELLAX02/agora/backend/internal/iam/password"
	"github.com/CAPELLAX02/agora/backend/internal/people"
	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
	"github.com/CAPELLAX02/agora/backend/internal/platform/jwt"
)

// fakeClock, testlerde zamanı elle ilerletmeye yarar. Now metodu, servise
// time.Now yerine method value olarak verilir: clock.Now.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// fakeRevoker, iptal listesine eklenen oturumları bellekte tutar. err doluysa
// Redis kesintisini taklit eder.
type fakeRevoker struct {
	mu      sync.Mutex
	revoked map[string]bool
	err     error
}

func (f *fakeRevoker) Revoke(ctx context.Context, sessionID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.revoked[sessionID] = true
	return nil
}

func (f *fakeRevoker) has(sessionID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.revoked[sessionID]
}

func (f *fakeRevoker) fail(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

var cheapParams = password.Params{Memory: 64, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}

var testAuthConfig = iam.AuthConfig{
	Issuer:            "agora",
	Audience:          "agora-api",
	AccessTokenTTL:    15 * time.Minute,
	IdleTimeout:       2 * time.Hour,
	AbsoluteTimeout:   30 * 24 * time.Hour,
	ReuseGracePeriod:  10 * time.Second,
	MaxFailedAttempts: 5,
	LockoutBase:       time.Minute,
	LockoutMax:        time.Hour,

	WebBaseURL:           "http://web.agora.test",
	ResetTokenTTL:        30 * time.Minute,
	ActivationTokenTTL:   72 * time.Hour,
	ResetRequestInterval: 2 * time.Minute,
}

type authEnv struct {
	pool     *pgxpool.Pool
	auth     *iam.Auth
	clock    *fakeClock
	hasher   *password.Hasher
	verifier *jwt.Verifier
	revoker  *fakeRevoker
}

func newAuthEnv(t *testing.T) *authEnv {
	t.Helper()
	pool := dbtest.New(t)
	ctx := context.Background()

	_, key, _ := ed25519.GenerateKey(rand.Reader)
	signer, err := jwt.NewSigner("test", key)
	if err != nil {
		t.Fatal(err)
	}
	verifier := jwt.NewVerifier("agora", "agora-api",
		map[string]ed25519.PublicKey{"test": key.Public().(ed25519.PublicKey)}, 0)

	clock := &fakeClock{t: time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC)}
	hasher := password.NewHasher(cheapParams, 4)

	revoker := &fakeRevoker{revoked: map[string]bool{}}

	auth, err := iam.NewAuth(ctx, pool, hasher, signer, revoker, testAuthConfig, clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	return &authEnv{pool: pool, auth: auth, clock: clock, hasher: hasher, verifier: verifier, revoker: revoker}
}

// addUser, verilen parolayla (verilen hasher'la hash'lenmiş) bir kullanıcı oluşturur.
func (e *authEnv) addUser(t *testing.T, username, plain string, hasher *password.Hasher) string {
	t.Helper()
	ctx := context.Background()
	hash, err := hasher.Hash(ctx, plain)
	if err != nil {
		t.Fatal(err)
	}
	personID, err := people.NewRepository(e.pool).CreatePerson(ctx, people.NewPerson{FirstName: "Test", LastName: username})
	if err != nil {
		t.Fatal(err)
	}
	id, err := iam.NewRepository(e.pool).CreateUser(ctx, iam.NewUser{
		PersonID: personID, Username: username, Email: username + "@agora.test", PasswordHash: hash,
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (e *authEnv) login(username, plain string) (iam.Tokens, error) {
	return e.auth.Login(context.Background(), iam.LoginInput{
		Username: username, Password: plain, Client: iam.ClientWeb, IP: "203.0.113.7", UserAgent: "test",
	})
}

func (e *authEnv) user(t *testing.T, id string) iam.User {
	t.Helper()
	u, err := iam.NewRepository(e.pool).UserByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func (e *authEnv) sessionRevokeReason(t *testing.T, sessionID string) string {
	t.Helper()
	var reason *string
	if err := e.pool.QueryRow(context.Background(),
		`SELECT revoke_reason FROM iam.sessions WHERE id = $1`, sessionID).Scan(&reason); err != nil {
		t.Fatal(err)
	}
	if reason == nil {
		return ""
	}
	return *reason
}

const pw = "kahve fincanı mavi gökyüzü"

func TestLogin(t *testing.T) {
	e := newAuthEnv(t)
	userID := e.addUser(t, "22290001", pw, e.hasher)

	t.Run("doğru parola oturum açar ve geçerli token'lar üretir", func(t *testing.T) {
		tok, err := e.login(" 22290001 ", pw) // kenar boşlukları kırpılır
		if err != nil {
			t.Fatal(err)
		}

		claims, err := e.verifier.Verify(tok.AccessToken, e.clock.Now())
		if err != nil {
			t.Fatalf("access token doğrulanamadı: %v", err)
		}
		if claims.Subject != userID || claims.SessionID != tok.SessionID || claims.AMR[0] != "pwd" {
			t.Errorf("claim'ler = %+v", claims)
		}
		if got := tok.AccessExpiresAt.Sub(e.clock.Now()); got != 15*time.Minute {
			t.Errorf("access ömrü = %v", got)
		}
		if got := tok.RefreshExpiresAt.Sub(e.clock.Now()); got != 2*time.Hour {
			t.Errorf("refresh ömrü = %v, want boşta kalma süresi (2s)", got)
		}

		// Refresh token veritabanına düz metin olarak yazılmamalı.
		var stored int
		_ = e.pool.QueryRow(context.Background(),
			`SELECT count(*) FROM iam.refresh_tokens WHERE token_hash = convert_to($1, 'UTF8')`, tok.RefreshToken).Scan(&stored)
		if stored != 0 {
			t.Error("refresh token düz metin olarak saklanmış")
		}

		if u := e.user(t, userID); u.LastLoginAt == nil || !u.LastLoginAt.Equal(e.clock.Now()) {
			t.Errorf("last_login_at = %v", u.LastLoginAt)
		}
	})

	t.Run("kullanıcı adı büyük/küçük harf duyarsız", func(t *testing.T) {
		e.addUser(t, "P10001", pw, e.hasher)
		if _, err := e.login("p10001", pw); err != nil {
			t.Errorf("küçük harfli kullanıcı adıyla giriş: %v", err)
		}
	})

	t.Run("yanlış parola ve olmayan kullanıcı aynı hatayı verir", func(t *testing.T) {
		_, errWrong := e.login("22290001", "yanlış parola 123")
		_, errMissing := e.login("99999999", pw)
		if !errors.Is(errWrong, iam.ErrInvalidCredentials) || !errors.Is(errMissing, iam.ErrInvalidCredentials) {
			t.Errorf("yanlış parola: %v, olmayan kullanıcı: %v", errWrong, errMissing)
		}
	})
}

func TestLoginLockout(t *testing.T) {
	e := newAuthEnv(t)
	userID := e.addUser(t, "22290002", pw, e.hasher)

	for i := 1; i <= 4; i++ {
		if _, err := e.login("22290002", "yanlış"); !errors.Is(err, iam.ErrInvalidCredentials) {
			t.Fatalf("%d. deneme: err = %v", i, err)
		}
	}

	// 5. başarısız deneme hesabı 1 dakika kilitler.
	_, err := e.login("22290002", "yanlış")
	var locked *iam.LockedError
	if !errors.As(err, &locked) || !errors.Is(err, iam.ErrAccountLocked) {
		t.Fatalf("5. deneme: err = %v, want LockedError", err)
	}
	if got := locked.Until.Sub(e.clock.Now()); got != time.Minute {
		t.Errorf("ilk kilit süresi = %v, want 1m", got)
	}

	// Kilit sürerken doğru parola bile reddedilir.
	if _, err := e.login("22290002", pw); !errors.Is(err, iam.ErrAccountLocked) {
		t.Errorf("kilitliyken doğru parola: err = %v", err)
	}

	// Kilit bitince bir kez daha yanlış: kilit 2 dakikaya katlanır.
	e.clock.Advance(61 * time.Second)
	_, err = e.login("22290002", "yanlış")
	if !errors.As(err, &locked) || locked.Until.Sub(e.clock.Now()) != 2*time.Minute {
		t.Fatalf("6. deneme: err = %v, want 2 dakikalık kilit", err)
	}

	// Kilit bitince doğru parola girer ve sayaç sıfırlanır.
	e.clock.Advance(121 * time.Second)
	if _, err := e.login("22290002", pw); err != nil {
		t.Fatalf("kilit sonrası doğru parola: %v", err)
	}
	if u := e.user(t, userID); u.FailedLoginCount != 0 || u.LockedUntil != nil {
		t.Errorf("sayaç sıfırlanmadı: count=%d locked=%v", u.FailedLoginCount, u.LockedUntil)
	}
}

func TestLoginDisabledAccount(t *testing.T) {
	e := newAuthEnv(t)
	userID := e.addUser(t, "22290003", pw, e.hasher)
	if _, err := e.pool.Exec(context.Background(), `UPDATE iam.users SET status = 'SUSPENDED' WHERE id = $1`, userID); err != nil {
		t.Fatal(err)
	}

	// Yanlış parolayla gelen biri hesabın askıda olduğunu öğrenemez.
	if _, err := e.login("22290003", "yanlış"); !errors.Is(err, iam.ErrInvalidCredentials) {
		t.Errorf("yanlış parola: err = %v", err)
	}
	if _, err := e.login("22290003", pw); !errors.Is(err, iam.ErrAccountDisabled) {
		t.Errorf("doğru parola: err = %v, want ErrAccountDisabled", err)
	}
}

func TestLoginUpgradesPasswordHash(t *testing.T) {
	e := newAuthEnv(t)
	weaker := cheapParams
	weaker.Iterations = 2 // servisin parametrelerinden farklı
	userID := e.addUser(t, "22290004", pw, password.NewHasher(weaker, 1))

	if _, err := e.login("22290004", pw); err != nil {
		t.Fatal(err)
	}

	u := e.user(t, userID)
	if e.hasher.NeedsRehash(u.PasswordHash) {
		t.Errorf("giriş sonrası hash hâlâ eski parametrelerle: %s", u.PasswordHash[:30])
	}
	if _, err := e.login("22290004", pw); err != nil {
		t.Errorf("yükseltilmiş hash ile giriş: %v", err)
	}
}

func TestRefreshRotation(t *testing.T) {
	e := newAuthEnv(t)
	e.addUser(t, "22290005", pw, e.hasher)
	ctx := context.Background()

	first, err := e.login("22290005", pw)
	if err != nil {
		t.Fatal(err)
	}

	e.clock.Advance(time.Hour)
	second, err := e.auth.Refresh(ctx, first.RefreshToken)
	if err != nil {
		t.Fatalf("ilk refresh: %v", err)
	}
	if second.RefreshToken == first.RefreshToken || second.AccessToken == first.AccessToken {
		t.Error("refresh yeni token'lar üretmedi")
	}
	if second.SessionID != first.SessionID {
		t.Error("refresh oturumu değiştirmemeli")
	}
	if got := second.RefreshExpiresAt.Sub(e.clock.Now()); got != 2*time.Hour {
		t.Errorf("yeni refresh ömrü = %v: boşta kalma süresi her refresh'te yeniden başlamalı", got)
	}

	e.clock.Advance(time.Hour)
	if _, err := e.auth.Refresh(ctx, second.RefreshToken); err != nil {
		t.Errorf("ikinci refresh: %v", err)
	}
}

func TestRefreshReuseDetection(t *testing.T) {
	e := newAuthEnv(t)
	e.addUser(t, "22290006", pw, e.hasher)
	ctx := context.Background()

	first, _ := e.login("22290006", pw)
	second, err := e.auth.Refresh(ctx, first.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("tolerans içinde eski token reddedilir ama oturum yaşar", func(t *testing.T) {
		e.clock.Advance(5 * time.Second)
		if _, err := e.auth.Refresh(ctx, first.RefreshToken); !errors.Is(err, iam.ErrInvalidRefreshToken) {
			t.Errorf("err = %v, want ErrInvalidRefreshToken", err)
		}
		if reason := e.sessionRevokeReason(t, first.SessionID); reason != "" {
			t.Errorf("oturum iptal edildi (%s), edilmemeliydi", reason)
		}
		if e.revoker.has(first.SessionID) {
			t.Error("tolerans içindeki tekrar oturumu iptal listesine eklememeli")
		}
	})

	t.Run("tolerans dışında eski token oturumu tamamen iptal eder", func(t *testing.T) {
		e.clock.Advance(time.Minute)
		if _, err := e.auth.Refresh(ctx, first.RefreshToken); !errors.Is(err, iam.ErrRefreshTokenReused) {
			t.Fatalf("err = %v, want ErrRefreshTokenReused", err)
		}
		if reason := e.sessionRevokeReason(t, first.SessionID); reason != "REUSE_DETECTED" {
			t.Errorf("oturum sonlandırma sebebi = %q, want REUSE_DETECTED", reason)
		}
		// Saldırganın elindeki access token'lar da hemen geçersiz olmalı.
		if !e.revoker.has(first.SessionID) {
			t.Error("oturum iptal listesine eklenmedi")
		}
		// Meşru kullanıcının elindeki en yeni token da artık geçersiz: saldırgan ve
		// kurban ayırt edilemediği için ikisi de yeniden giriş yapmak zorunda.
		if _, err := e.auth.Refresh(ctx, second.RefreshToken); !errors.Is(err, iam.ErrInvalidRefreshToken) {
			t.Errorf("iptal sonrası yeni token: err = %v", err)
		}
	})
}

func TestRefreshExpiry(t *testing.T) {
	e := newAuthEnv(t)
	e.addUser(t, "22290007", pw, e.hasher)
	ctx := context.Background()

	t.Run("boşta kalma süresi dolunca refresh reddedilir", func(t *testing.T) {
		tok, _ := e.login("22290007", pw)
		e.clock.Advance(2*time.Hour + time.Second)
		if _, err := e.auth.Refresh(ctx, tok.RefreshToken); !errors.Is(err, iam.ErrInvalidRefreshToken) {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("mutlak süre refresh'lerle uzatılamaz", func(t *testing.T) {
		tok, _ := e.login("22290007", pw)
		// 30 gün boyunca her saat refresh: oturum asla boşta kalmıyor.
		for elapsed := time.Duration(0); elapsed < 30*24*time.Hour; elapsed += time.Hour {
			e.clock.Advance(time.Hour)
			next, err := e.auth.Refresh(ctx, tok.RefreshToken)
			if err != nil {
				if elapsed < 30*24*time.Hour-time.Hour {
					t.Fatalf("%v sonra refresh reddedildi: %v", elapsed, err)
				}
				return // mutlak sınıra ulaşıldı: beklenen
			}
			tok = next
		}
		t.Error("oturum mutlak süreyi aştığı halde refresh kabul edildi")
	})

	t.Run("bilinmeyen ve boş token", func(t *testing.T) {
		for _, raw := range []string{"", "uydurma-token"} {
			if _, err := e.auth.Refresh(ctx, raw); !errors.Is(err, iam.ErrInvalidRefreshToken) {
				t.Errorf("Refresh(%q) err = %v", raw, err)
			}
		}
	})
}

func TestLogout(t *testing.T) {
	e := newAuthEnv(t)
	e.addUser(t, "22290008", pw, e.hasher)
	ctx := context.Background()

	tok, _ := e.login("22290008", pw)
	if err := e.auth.Logout(ctx, tok.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if reason := e.sessionRevokeReason(t, tok.SessionID); reason != "LOGOUT" {
		t.Errorf("sebep = %q", reason)
	}
	if !e.revoker.has(tok.SessionID) {
		t.Error("çıkışta oturum iptal listesine eklenmedi: access token 15 dk daha çalışırdı")
	}
	if _, err := e.auth.Refresh(ctx, tok.RefreshToken); !errors.Is(err, iam.ErrInvalidRefreshToken) {
		t.Errorf("çıkış sonrası refresh: err = %v", err)
	}
	// Çıkış idempotent: tekrar etmek ya da bilinmeyen token göndermek hata değildir.
	if err := e.auth.Logout(ctx, tok.RefreshToken); err != nil {
		t.Errorf("ikinci çıkış: %v", err)
	}
	if err := e.auth.Logout(ctx, "bilinmeyen"); err != nil {
		t.Errorf("bilinmeyen token ile çıkış: %v", err)
	}
}

// TestConcurrentRefresh, aynı refresh token'la eşzamanlı gelen isteklerden sadece
// birinin başarılı olduğunu doğrular. FOR UPDATE kilidi olmasaydı birden fazla
// istek aynı token'ı "kullanılmamış" görüp her biri yeni bir token alabilirdi.
func TestRefreshDisabledUser(t *testing.T) {
	e := newAuthEnv(t)
	userID := e.addUser(t, "22290010", pw, e.hasher)
	tok, _ := e.login("22290010", pw)

	// Oturum açıkken hesap askıya alınır.
	if _, err := e.pool.Exec(context.Background(), `UPDATE iam.users SET status = 'SUSPENDED' WHERE id = $1`, userID); err != nil {
		t.Fatal(err)
	}

	if _, err := e.auth.Refresh(context.Background(), tok.RefreshToken); !errors.Is(err, iam.ErrAccountDisabled) {
		t.Fatalf("err = %v, want ErrAccountDisabled", err)
	}
	if reason := e.sessionRevokeReason(t, tok.SessionID); reason != "ADMIN" {
		t.Errorf("sebep = %q, want ADMIN", reason)
	}
	if !e.revoker.has(tok.SessionID) {
		t.Error("askıya alınan hesabın oturumu iptal listesine eklenmedi")
	}
}

// TestLogoutWhenRevocationListUnavailable, iptal listesine yazılamadığında çıkışın
// hata döndürdüğünü ve tekrar denemenin durumu düzelttiğini doğrular. Oturum
// veritabanında zaten iptal edildiği için ikinci deneme sadece listeye yazar.
func TestLogoutWhenRevocationListUnavailable(t *testing.T) {
	e := newAuthEnv(t)
	e.addUser(t, "22290011", pw, e.hasher)
	ctx := context.Background()
	tok, _ := e.login("22290011", pw)

	redisDown := errors.New("redis: bağlantı reddedildi")
	e.revoker.fail(redisDown)

	if err := e.auth.Logout(ctx, tok.RefreshToken); !errors.Is(err, redisDown) {
		t.Fatalf("err = %v, Redis hatası bekleniyordu", err)
	}
	if reason := e.sessionRevokeReason(t, tok.SessionID); reason != "LOGOUT" {
		t.Errorf("veritabanındaki iptal geri alınmamalı, sebep = %q", reason)
	}

	e.revoker.fail(nil)
	if err := e.auth.Logout(ctx, tok.RefreshToken); err != nil {
		t.Fatalf("tekrar deneme: %v", err)
	}
	if !e.revoker.has(tok.SessionID) {
		t.Error("tekrar denemede oturum iptal listesine eklenmeliydi")
	}
}

func TestConcurrentRefresh(t *testing.T) {
	e := newAuthEnv(t)
	e.addUser(t, "22290009", pw, e.hasher)
	tok, _ := e.login("22290009", pw)

	// Havuzu önceden ısıt: bağlantılar hazır olmazsa goroutine'ler bağlantı açmayı
	// (TCP + SCRAM) beklerken ilki işini çoktan bitirir ve yarış hiç yaşanmaz.
	// O zaman test, kilit olmasa bile geçer ve sahte bir güvence verir.
	n := int(e.pool.Config().MaxConns)
	conns := make([]*pgxpool.Conn, 0, n)
	for range n {
		c, err := e.pool.Acquire(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		conns = append(conns, c)
	}
	for _, c := range conns {
		c.Release()
	}

	var (
		wg        sync.WaitGroup
		successes atomic.Int32
		start     = make(chan struct{})
	)
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // hepsi aynı anda başlasın
			if _, err := e.auth.Refresh(context.Background(), tok.RefreshToken); err == nil {
				successes.Add(1)
			} else if !errors.Is(err, iam.ErrInvalidRefreshToken) {
				t.Errorf("beklenmeyen hata: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if got := successes.Load(); got != 1 {
		t.Errorf("%d istek başarılı oldu, tam olarak 1 olmalıydı", got)
	}
	if reason := e.sessionRevokeReason(t, tok.SessionID); reason != "" {
		t.Errorf("eşzamanlı meşru istekler oturumu iptal etmemeli, sebep = %q", reason)
	}
}

func TestLockedErrorMessage(t *testing.T) {
	err := &iam.LockedError{Until: time.Date(2026, 10, 8, 9, 1, 0, 0, time.UTC)}
	if !strings.Contains(err.Error(), "2026-10-08T09:01:00Z") {
		t.Errorf("hata mesajı kilit bitişini içermiyor: %s", err)
	}
}

// eventCounts, kullanıcının güvenlik olaylarını türe göre sayar. userID boşsa
// kullanıcısı bilinmeyen olaylar sayılır.
func (e *authEnv) eventCounts(t *testing.T, userID string) map[string]int {
	t.Helper()
	q := `SELECT event_type, count(*) FROM audit.security_events WHERE user_id = $1 GROUP BY 1`
	args := []any{userID}
	if userID == "" {
		q, args = `SELECT event_type, count(*) FROM audit.security_events WHERE user_id IS NULL GROUP BY 1`, nil
	}
	rows, err := e.pool.Query(context.Background(), q, args...)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for rows.Next() {
		var typ string
		var n int
		if err := rows.Scan(&typ, &n); err != nil {
			t.Fatal(err)
		}
		counts[typ] = n
	}
	return counts
}

func TestSecurityEvents(t *testing.T) {
	e := newAuthEnv(t)
	ctx := context.Background()
	userID := e.addUser(t, "22290012", pw, e.hasher)

	if _, err := e.login("yok-boyle-biri", pw); !errors.Is(err, iam.ErrInvalidCredentials) {
		t.Fatal(err)
	}
	if got := e.eventCounts(t, ""); got[audit.EventLoginFailed] != 1 {
		t.Errorf("bilinmeyen kullanıcı olayları = %v", got)
	}

	for range 5 {
		_, _ = e.login("22290012", "yanlış parola 123")
	}
	_, _ = e.login("22290012", pw) // kilitliyken
	got := e.eventCounts(t, userID)
	if got[audit.EventLoginFailed] != 6 || got[audit.EventAccountLocked] != 1 {
		t.Errorf("başarısız giriş olayları = %v, 6 başarısız ve 1 kilit bekleniyordu", got)
	}

	e.clock.Advance(2 * time.Minute)
	first, err := e.login("22290012", pw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.auth.Refresh(ctx, first.RefreshToken); err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(time.Minute)
	_, _ = e.auth.Refresh(ctx, first.RefreshToken) // yeniden kullanım

	second, _ := e.login("22290012", pw)
	_ = e.auth.Logout(ctx, second.RefreshToken)
	_ = e.auth.Logout(ctx, second.RefreshToken) // ikinci çıkış olay yazmamalı

	got = e.eventCounts(t, userID)
	want := map[string]int{
		audit.EventLoginFailed:        6,
		audit.EventAccountLocked:      1,
		audit.EventLoginSucceeded:     2,
		audit.EventRefreshTokenReused: 1,
		audit.EventLogout:             1,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("olaylar =\n  %v\nwant\n  %v", got, want)
	}
}

// countingMetrics, sayaç çağrılarını kaydeder.
type countingMetrics struct {
	mu     sync.Mutex
	counts map[string]int
}

func (m *countingMetrics) inc(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counts[key]++
}

func (m *countingMetrics) LoginSucceeded(client string) { m.inc("login:" + client) }
func (m *countingMetrics) LoginFailed(reason string)    { m.inc("fail:" + reason) }
func (m *countingMetrics) AccountLocked()               { m.inc("locked") }
func (m *countingMetrics) RefreshReuseDetected()        { m.inc("reuse") }
func (m *countingMetrics) PasswordReset(stage string)   { m.inc("reset:" + stage) }

func TestAuthMetrics(t *testing.T) {
	e := newAuthEnv(t)
	ctx := context.Background()
	m := &countingMetrics{counts: map[string]int{}}
	e.auth.SetMetrics(m)
	e.addUser(t, "22290090", pw, e.hasher)

	tok, _ := e.login("22290090", pw)
	_, _ = e.login("yok", pw)
	for range 5 {
		_, _ = e.login("22290090", "yanlış parola 123")
	}
	_, _ = e.auth.Refresh(ctx, tok.RefreshToken)
	e.clock.Advance(time.Minute)
	_, _ = e.auth.Refresh(ctx, tok.RefreshToken) // yeniden kullanım
	e.clock.Advance(time.Hour)
	_ = e.auth.RequestPasswordReset(ctx, "22290090")
	link, _ := e.lastResetLink(t)
	_ = e.auth.ResetPassword(ctx, tokenFromLink(t, link), newPW)

	want := map[string]int{
		"login:WEB": 1, "fail:unknown_user": 1, "fail:wrong_password": 5, "locked": 1,
		"reuse": 1, "reset:requested": 1, "reset:completed": 1,
	}
	if !reflect.DeepEqual(m.counts, want) {
		t.Errorf("sayaçlar =\n  %v\nwant\n  %v", m.counts, want)
	}
}
