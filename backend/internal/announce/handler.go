package announce

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

const permPublish = "announcement:publish"

// Handler, duyuruların HTTP uçlarıdır. Herkes hedef kitlesinde olduğu yayındaki duyuruları okur;
// announcement:publish sahibi kapsamındaki birimlere duyuru yazar ve kendi duyurularını ya da
// bütün hedefleri kapsamında olanları yönetir.
type Handler struct {
	svc    *Service
	logger *slog.Logger
}

// NewHandler, bir Handler oluşturur.
func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// Register, uçları yönlendiriciye ekler.
func (h *Handler) Register(rt *authz.Router) {
	rt.HandleFunc("GET /api/v1/me/announcements", authz.Permission("announcement:read"), h.feed)
	rt.HandleFunc("GET /api/v1/announcement-targets", authz.Permission(permPublish), h.targets)
	rt.HandleFunc("GET /api/v1/announcements", authz.Permission(permPublish), h.managed)
	rt.HandleFunc("POST /api/v1/announcements", authz.Permission(permPublish), h.create)
	rt.HandleFunc("GET /api/v1/announcements/{id}", authz.Authenticated, h.get)
	rt.HandleFunc("PUT /api/v1/announcements/{id}", authz.Permission(permPublish), h.update)
	rt.HandleFunc("DELETE /api/v1/announcements/{id}", authz.Permission(permPublish), h.delete)
	rt.HandleFunc("POST /api/v1/announcements/{id}/publish", authz.Permission(permPublish), h.publish)
	rt.HandleFunc("POST /api/v1/announcements/{id}/archive", authz.Permission(permPublish), h.archive)
}

// --- Yanıtlar -----------------------------------------------------------------------------

type audienceDTO struct {
	ScopeType string  `json:"scope_type"`
	ScopeID   *string `json:"scope_id"`
	ScopeName string  `json:"scope_name,omitempty"`
	People    string  `json:"people"`
}

type authorDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type statsDTO struct {
	Audience *int `json:"audience,omitempty"` // hedef kitle büyüklüğü: sadece ayrıntıda
	Reads    int  `json:"reads"`
}

type announcementResponse struct {
	ID           string        `json:"id"`
	Title        string        `json:"title"`
	Summary      string        `json:"summary"`
	Body         string        `json:"body_html"`
	Type         string        `json:"type"`
	PublishFrom  time.Time     `json:"publish_from"`
	PublishUntil *time.Time    `json:"publish_until"`
	Pinned       bool          `json:"is_pinned"`
	Status       string        `json:"status"`
	Author       authorDTO     `json:"author"`
	Audiences    []audienceDTO `json:"audiences"`
	NotifiedAt   *time.Time    `json:"notified_at"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
	Version      int           `json:"version"`
	ReadAt       *time.Time    `json:"read_at"` // isteği yapanın okuduğu an (akışta)
	CanManage    bool          `json:"can_manage"`
	Stats        *statsDTO     `json:"stats,omitempty"`
}

func toResponse(a Announcement) announcementResponse {
	res := announcementResponse{ID: a.ID, Title: a.Title, Summary: a.Summary, Body: a.Body, Type: a.Type,
		PublishFrom: a.PublishFrom, PublishUntil: a.PublishUntil, Pinned: a.Pinned, Status: a.Status,
		Author: authorDTO{ID: a.AuthorID, Name: a.AuthorName}, Audiences: make([]audienceDTO, 0, len(a.Audiences)),
		NotifiedAt: a.NotifiedAt, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt, Version: a.Version, ReadAt: a.ReadAt}
	for _, x := range a.Audiences {
		d := audienceDTO{ScopeType: x.ScopeType, ScopeName: x.ScopeName, People: x.People}
		if x.ScopeID != "" {
			id := x.ScopeID
			d.ScopeID = &id
		}
		res.Audiences = append(res.Audiences, d)
	}
	return res
}

// --- Okuma --------------------------------------------------------------------------------

func (h *Handler) feed(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, limitErr := httpx.ParseLimit(q)
	var errs []httpx.FieldError
	if limitErr != nil {
		errs = append(errs, *limitErr)
	}
	var after *FeedCursor
	if c := q.Get("cursor"); c != "" {
		cur, err := httpx.DecodeCursor[FeedCursor](c)
		if err != nil || !httpx.ValidUUID(cur.ID) {
			errs = append(errs, httpx.FieldError{Field: "cursor", Message: "geçersiz cursor"})
		}
		after = &cur
	}
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	list, more, err := h.svc.Feed(r.Context(), userID(r), after, limit)
	if h.failed(w, r, err) {
		return
	}
	res := httpx.ListResponse[announcementResponse]{Items: make([]announcementResponse, 0, len(list))}
	for _, a := range list {
		res.Items = append(res.Items, toResponse(a))
	}
	if more {
		last := list[len(list)-1]
		next, err := httpx.EncodeCursor(FeedCursor{Pinned: last.Pinned, From: last.PublishFrom, ID: last.ID})
		if h.failed(w, r, err) {
			return
		}
		res.NextCursor = next
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

// manages, isteği yapanın duyuruyu yönetip yönetemeyeceğidir: yayımlama yetkisi olan yazar ya da
// bütün hedefleri kapsamında olan.
func (h *Handler) manages(r *http.Request, a Announcement) (bool, error) {
	perms, ok := authz.PermissionsFrom(r.Context())
	if !ok || !perms.Has(permPublish) {
		return false, nil
	}
	if a.AuthorID == userID(r) {
		return true, nil
	}
	targets, err := h.svc.Targets(r.Context(), a.Audiences)
	if err != nil {
		return false, err
	}
	return Allowed(perms, a.Audiences, targets), nil
}

// load, duyuruyu okur; yönetemeyen ve hedef kitlesinde olmayan için yokmuş gibi 404.
func (h *Handler) load(w http.ResponseWriter, r *http.Request) (Announcement, bool, bool) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return Announcement{}, false, false
	}
	a, err := h.svc.Get(r.Context(), id)
	if h.failed(w, r, err) {
		return Announcement{}, false, false
	}
	manage, err := h.manages(r, a)
	if h.failed(w, r, err) {
		return Announcement{}, false, false
	}
	return a, manage, true
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	a, manage, ok := h.load(w, r)
	if !ok {
		return
	}
	visible, err := h.svc.Visible(r.Context(), a.ID, userID(r))
	if h.failed(w, r, err) {
		return
	}
	if !visible && !manage {
		httpx.NotFound(w, r)
		return
	}
	if visible {
		if err := h.svc.MarkRead(r.Context(), a.ID, userID(r)); err != nil {
			h.logger.Error("duyuru okundu yapılamadı", "err", err, "request_id", httpx.RequestIDFrom(r.Context()))
		}
	}
	res := toResponse(a)
	res.CanManage = manage
	if manage {
		audience, reads, err := h.svc.Stats(r.Context(), a.ID)
		if h.failed(w, r, err) {
			return
		}
		res.Stats = &statsDTO{Audience: &audience, Reads: reads}
	}
	w.Header().Set("ETag", httpx.ETag(a.Version))
	h.writeJSON(w, r, http.StatusOK, res)
}

func (h *Handler) managed(w http.ResponseWriter, r *http.Request) {
	perms, _ := authz.PermissionsFrom(r.Context())
	list, reads, err := h.svc.Managed(r.Context(), userID(r), perms.ScopesOf(permPublish))
	if h.failed(w, r, err) {
		return
	}
	res := httpx.ListResponse[announcementResponse]{Items: make([]announcementResponse, 0, len(list))}
	for _, a := range list {
		item := toResponse(a)
		item.CanManage = true
		item.Stats = &statsDTO{Reads: reads[a.ID]}
		res.Items = append(res.Items, item)
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

type unitDTO struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	ParentID *string `json:"parent_id"`
}

func (h *Handler) targets(w http.ResponseWriter, r *http.Request) {
	perms, _ := authz.PermissionsFrom(r.Context())
	scope := perms.ScopesOf(permPublish)
	faculties, departments, programs, err := h.svc.TargetUnits(r.Context(), scope)
	if h.failed(w, r, err) {
		return
	}
	conv := func(units []Unit) []unitDTO {
		out := make([]unitDTO, 0, len(units))
		for _, u := range units {
			d := unitDTO{ID: u.ID, Name: u.Name}
			if u.ParentID != "" {
				parent := u.ParentID
				d.ParentID = &parent
			}
			out = append(out, d)
		}
		return out
	}
	h.writeJSON(w, r, http.StatusOK, map[string]any{
		"university": scope.University, "faculties": conv(faculties), "departments": conv(departments), "programs": conv(programs),
	})
}

// --- Yazma --------------------------------------------------------------------------------

type audienceInput struct {
	ScopeType string  `json:"scope_type"`
	ScopeID   *string `json:"scope_id"`
	People    string  `json:"people"`
}

type announcementRequest struct {
	Title        string          `json:"title"`
	Summary      string          `json:"summary"`
	Body         string          `json:"body_html"`
	Type         string          `json:"type"`
	PublishFrom  *time.Time      `json:"publish_from"`
	PublishUntil *time.Time      `json:"publish_until"`
	Pinned       bool            `json:"is_pinned"`
	Audiences    []audienceInput `json:"audiences"`
	Publish      bool            `json:"publish"` // sadece oluştururken: taslak yerine hemen yayımla
}

func (req announcementRequest) input() Input {
	in := Input{Title: req.Title, Summary: req.Summary, Body: req.Body, Type: req.Type, PublishFrom: req.PublishFrom,
		PublishUntil: req.PublishUntil, Pinned: req.Pinned}
	for _, a := range req.Audiences {
		x := Audience{ScopeType: a.ScopeType, People: a.People}
		if a.ScopeID != nil {
			x.ScopeID = *a.ScopeID
		}
		in.Audiences = append(in.Audiences, x)
	}
	return in
}

// authorize, isteği yapanın yeni hedeflerin hepsine yazabildiğini denetler.
func (h *Handler) authorize(w http.ResponseWriter, r *http.Request, in Input) bool {
	for _, a := range in.Audiences {
		if a.ScopeID != "" && !httpx.ValidUUID(a.ScopeID) {
			httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "audiences", Message: "geçersiz birim"}})
			return false
		}
	}
	targets, err := h.svc.Targets(r.Context(), in.Audiences)
	if h.failed(w, r, err) {
		return false
	}
	perms, _ := authz.PermissionsFrom(r.Context())
	if !Allowed(perms, in.Audiences, targets) {
		_ = httpx.WriteProblem(w, r, httpx.Problem{Status: http.StatusForbidden, Code: "FORBIDDEN",
			Detail: "Seçilen hedeflerden biri duyuru yetkinizin kapsamında değil."})
		return false
	}
	return true
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req announcementRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	in := req.input()
	if err := in.validate(); h.failed(w, r, err) {
		return
	}
	if !h.authorize(w, r, in) {
		return
	}
	id, err := h.svc.Create(r.Context(), userID(r), in, req.Publish)
	if h.failed(w, r, err) {
		return
	}
	h.respond(w, r, id, http.StatusCreated)
}

func (h *Handler) respond(w http.ResponseWriter, r *http.Request, id string, status int) {
	a, err := h.svc.Get(r.Context(), id)
	if h.failed(w, r, err) {
		return
	}
	res := toResponse(a)
	res.CanManage = true
	w.Header().Set("ETag", httpx.ETag(a.Version))
	h.writeJSON(w, r, status, res)
}

// managedOne, path'teki duyuruyu yönetebilen için yükler; yönetemeyene yokmuş gibi 404.
func (h *Handler) managedOne(w http.ResponseWriter, r *http.Request) (Announcement, bool) {
	a, manage, ok := h.load(w, r)
	if !ok {
		return Announcement{}, false
	}
	if !manage {
		httpx.NotFound(w, r)
		return Announcement{}, false
	}
	return a, true
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	a, ok := h.managedOne(w, r)
	if !ok {
		return
	}
	version, ok := httpx.IfMatchVersion(r)
	if !ok {
		httpx.PreconditionRequired(w, r)
		return
	}
	var req announcementRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	in := req.input()
	if err := in.validate(); h.failed(w, r, err) {
		return
	}
	if !h.authorize(w, r, in) {
		return
	}
	if h.failed(w, r, h.svc.Update(r.Context(), userID(r), a.ID, version, in)) {
		return
	}
	h.respond(w, r, a.ID, http.StatusOK)
}

func (h *Handler) transition(w http.ResponseWriter, r *http.Request, fn func(id string, version int) error) {
	a, ok := h.managedOne(w, r)
	if !ok {
		return
	}
	version, ok := httpx.IfMatchVersion(r)
	if !ok {
		httpx.PreconditionRequired(w, r)
		return
	}
	if h.failed(w, r, fn(a.ID, version)) {
		return
	}
	h.respond(w, r, a.ID, http.StatusOK)
}

func (h *Handler) publish(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, func(id string, version int) error { return h.svc.Publish(r.Context(), userID(r), id, version) })
}

func (h *Handler) archive(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, func(id string, version int) error { return h.svc.Archive(r.Context(), userID(r), id, version) })
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	a, ok := h.managedOne(w, r)
	if !ok {
		return
	}
	if h.failed(w, r, h.svc.Delete(r.Context(), userID(r), a.ID)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Yardımcılar --------------------------------------------------------------------------

func userID(r *http.Request) string {
	p, _ := authn.PrincipalFrom(r.Context())
	return p.UserID
}

func (h *Handler) failed(w http.ResponseWriter, r *http.Request, err error) bool {
	var verr *ValidationError
	switch {
	case err == nil:
		return false
	case errors.As(err, &verr):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: verr.Field, Message: verr.Message}})
	case errors.Is(err, ErrNotFound):
		httpx.NotFound(w, r)
	case errors.Is(err, ErrVersionMismatch):
		httpx.PreconditionFailed(w, r)
	case errors.Is(err, ErrInvalidState):
		_ = httpx.WriteProblem(w, r, httpx.Problem{Status: http.StatusConflict, Code: "INVALID_ANNOUNCEMENT_STATE",
			Detail: "Duyuru bu işlem için uygun durumda değil."})
	default:
		h.logger.Error("duyuru isteği başarısız", "err", err, "request_id", httpx.RequestIDFrom(r.Context()))
		httpx.InternalServerError(w, r)
	}
	return true
}

func (h *Handler) writeJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	if err := httpx.WriteJSON(w, status, v); err != nil {
		h.logger.Error("yanıt yazılamadı", "err", err, "request_id", httpx.RequestIDFrom(r.Context()))
	}
}
