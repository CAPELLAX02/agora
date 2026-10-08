package metrics

import "github.com/prometheus/client_golang/prometheus"

// Auth, kimlik doğrulama ve kötüye kullanım koruması metrikleridir. Ani bir başarısız
// giriş artışı parola püskürtme (password spraying), kilitlenme ve token yeniden
// kullanımı artışı da hesap ele geçirme girişimi işaretidir.
//
// iam.Metrics interface'ini örtük olarak sağlar.
type Auth struct {
	logins        *prometheus.CounterVec
	loginFailures *prometheus.CounterVec
	lockouts      prometheus.Counter
	reuse         prometheus.Counter
	resets        *prometheus.CounterVec
	rateLimited   *prometheus.CounterVec
}

// NewAuth, kimlik doğrulama metriklerini oluşturur ve registry'ye kaydeder.
func NewAuth(reg prometheus.Registerer) *Auth {
	m := &Auth{
		logins: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "logins_total",
			Help: "Başarılı girişler, istemci türüne göre.",
		}, []string{"client"}),
		loginFailures: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "login_failures_total",
			Help: "Başarısız girişler, sebebe göre (unknown_user, wrong_password, account_locked, account_inactive...).",
		}, []string{"reason"}),
		lockouts: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace, Name: "account_lockouts_total",
			Help: "Çok sayıda başarısız deneme sonucu kilitlenen hesaplar.",
		}),
		reuse: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace, Name: "refresh_reuse_detected_total",
			Help: "Yeniden kullanılan (çalınmış olabilecek) refresh token'lar.",
		}),
		resets: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "password_resets_total",
			Help: "Parola sıfırlama adımları (requested, completed).",
		}, []string{"stage"}),
		rateLimited: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Name: "rate_limited_total",
			Help: "Hız sınırına takılan istekler, sınırlayıcıya göre.",
		}, []string{"limiter"}),
	}
	reg.MustRegister(m.logins, m.loginFailures, m.lockouts, m.reuse, m.resets, m.rateLimited)
	return m
}

// LoginSucceeded, başarılı bir girişi sayar.
func (m *Auth) LoginSucceeded(client string) { m.logins.WithLabelValues(client).Inc() }

// LoginFailed, başarısız bir girişi sayar.
func (m *Auth) LoginFailed(reason string) { m.loginFailures.WithLabelValues(reason).Inc() }

// AccountLocked, bir hesabın kilitlendiğini sayar.
func (m *Auth) AccountLocked() { m.lockouts.Inc() }

// RefreshReuseDetected, yeniden kullanılan bir refresh token'ı sayar.
func (m *Auth) RefreshReuseDetected() { m.reuse.Inc() }

// PasswordReset, bir parola sıfırlama adımını sayar.
func (m *Auth) PasswordReset(stage string) { m.resets.WithLabelValues(stage).Inc() }

// RateLimited, hız sınırına takılan bir isteği sayar.
func (m *Auth) RateLimited(limiter string) { m.rateLimited.WithLabelValues(limiter).Inc() }
