package registration

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/enrollment"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

// Handler, ders durumu ve ders seçme uçlarını sunar.
type Handler struct {
	pool       *pgxpool.Pool
	repo       *Repository
	service    *Service
	students   *enrollment.Repository
	studentSvc *enrollment.Service
	logger     *slog.Logger
}

// NewHandler, bir Handler oluşturur. now, pencere kontrolleri için saattir (testlerde
// sabitlenir); metrics nil olabilir.
func NewHandler(pool *pgxpool.Pool, logger *slog.Logger, now func() time.Time, metrics Metrics) *Handler {
	return &Handler{
		pool: pool, repo: NewRepository(pool), service: NewService(pool, now, metrics),
		students: enrollment.NewRepository(pool), studentSvc: enrollment.NewService(pool),
		logger: logger,
	}
}

// Register, route'ları kaydeder. Program kaydına bağlı uçlar sadece kimlik ister: öğrenci
// kendi kaydını, danışman danışmanı olduğu öğrencinin kaydını, kapsamlı personel
// kapsamındaki kayıtları görür (politika fonksiyonu karar verir).
func (h *Handler) Register(rt *authz.Router) {
	rt.HandleFunc("GET /api/v1/student-programs/{id}/progress", authz.Authenticated, h.progress)
	h.registerRegistration(rt)
}

// readableProgram, program kaydını okur ve kullanıcının görebildiğini doğrular. Erişim
// yoksa (kaydın varlığını sızdırmamak için) 404 yazar.
func (h *Handler) readableProgram(w http.ResponseWriter, r *http.Request) (enrollment.StudentProgram, bool) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return enrollment.StudentProgram{}, false
	}
	sp, err := h.students.StudentProgram(r.Context(), id)
	if errors.Is(err, enrollment.ErrNotFound) {
		httpx.NotFound(w, r)
		return enrollment.StudentProgram{}, false
	}
	if err != nil {
		h.serverError(w, r, err)
		return enrollment.StudentProgram{}, false
	}
	principal, _ := authn.PrincipalFrom(r.Context())
	perms, _ := authz.PermissionsFrom(r.Context())
	ok, err := h.studentSvc.CanReadProgram(r.Context(), perms, principal.UserID, sp)
	if err != nil {
		h.serverError(w, r, err)
		return enrollment.StudentProgram{}, false
	}
	if !ok {
		httpx.NotFound(w, r)
		return enrollment.StudentProgram{}, false
	}
	return sp, true
}

func (h *Handler) writeJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	if err := httpx.WriteJSON(w, status, v); err != nil {
		h.logger.Error("yanıt yazılamadı", "err", err, "request_id", httpx.RequestIDFrom(r.Context()))
	}
}

func (h *Handler) serverError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.Error("beklenmeyen hata", "err", err, "path", r.URL.Path, "request_id", httpx.RequestIDFrom(r.Context()))
	httpx.InternalServerError(w, r)
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
