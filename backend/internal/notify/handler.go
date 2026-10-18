package notify

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

// Handler, bildirim merkezinin HTTP uçlarıdır. Herkes sadece kendi bildirimlerini görür.
type Handler struct {
	center *Center
	logger *slog.Logger
}

// NewHandler, bir Handler oluşturur.
func NewHandler(center *Center, logger *slog.Logger) *Handler {
	return &Handler{center: center, logger: logger}
}

// Register, uçları yönlendiriciye ekler.
func (h *Handler) Register(rt *authz.Router) {
	rt.HandleFunc("GET /api/v1/me/notifications", authz.Authenticated, h.list)
	rt.HandleFunc("GET /api/v1/me/notifications/unread-count", authz.Authenticated, h.unreadCount)
	rt.HandleFunc("POST /api/v1/me/notifications/read-all", authz.Authenticated, h.readAll)
	rt.HandleFunc("POST /api/v1/me/notifications/{id}/read", authz.Authenticated, h.markRead)
	rt.HandleFunc("POST /api/v1/me/notifications/{id}/archive", authz.Authenticated, h.archive)
	rt.HandleFunc("DELETE /api/v1/me/notifications/{id}/archive", authz.Authenticated, h.unarchive)
	rt.HandleFunc("GET /api/v1/me/notification-preferences", authz.Authenticated, h.preferences)
	rt.HandleFunc("PUT /api/v1/me/notification-preferences", authz.Authenticated, h.setPreferences)
}

type notificationResponse struct {
	ID         int64      `json:"id"`
	CreatedAt  time.Time  `json:"created_at"`
	Type       string     `json:"type"`
	Category   string     `json:"category"`
	TitleTR    string     `json:"title_tr"`
	TitleEN    string     `json:"title_en"`
	BodyTR     string     `json:"body_tr"`
	BodyEN     string     `json:"body_en"`
	Link       *string    `json:"link"`
	ReadAt     *time.Time `json:"read_at"`
	ArchivedAt *time.Time `json:"archived_at"`
}

func toResponse(it Item) notificationResponse {
	res := notificationResponse{ID: it.ID, CreatedAt: it.CreatedAt, Type: string(it.Type), Category: it.Category,
		TitleTR: it.Text.TitleTR, TitleEN: it.Text.TitleEN, BodyTR: it.Text.BodyTR, BodyEN: it.Text.BodyEN,
		ReadAt: it.ReadAt, ArchivedAt: it.ArchivedAt}
	if it.Link != "" {
		res.Link = &it.Link
	}
	return res
}

// cursor, listenin bir sonraki sayfasının başlangıcıdır.
type cursor struct {
	Before int64 `json:"b"`
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := ListFilter{UserID: userID(r), Unread: q.Get("unread") == "true", Archived: q.Get("archived") == "true"}
	var errs []httpx.FieldError
	limit, limitErr := httpx.ParseLimit(q)
	if limitErr != nil {
		errs = append(errs, *limitErr)
	}
	f.Limit = limit
	if c := q.Get("cursor"); c != "" {
		cur, err := httpx.DecodeCursor[cursor](c)
		if err != nil || cur.Before <= 0 {
			errs = append(errs, httpx.FieldError{Field: "cursor", Message: "geçersiz cursor"})
		}
		f.Before = cur.Before
	}
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	items, hasMore, err := h.center.List(r.Context(), f)
	if h.failed(w, r, err) {
		return
	}
	res := httpx.ListResponse[notificationResponse]{Items: make([]notificationResponse, 0, len(items))}
	for _, it := range items {
		res.Items = append(res.Items, toResponse(it))
	}
	if hasMore {
		next, err := httpx.EncodeCursor(cursor{Before: items[len(items)-1].ID})
		if h.failed(w, r, err) {
			return
		}
		res.NextCursor = next
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

func (h *Handler) unreadCount(w http.ResponseWriter, r *http.Request) {
	n, err := h.center.UnreadCount(r.Context(), userID(r))
	if h.failed(w, r, err) {
		return
	}
	h.writeJSON(w, r, http.StatusOK, map[string]int{"count": n})
}

func (h *Handler) readAll(w http.ResponseWriter, r *http.Request) {
	n, err := h.center.MarkAllRead(r.Context(), userID(r))
	if h.failed(w, r, err) {
		return
	}
	h.writeJSON(w, r, http.StatusOK, map[string]int{"updated": n})
}

func (h *Handler) markRead(w http.ResponseWriter, r *http.Request) {
	h.change(w, r, h.center.MarkRead)
}

func (h *Handler) archive(w http.ResponseWriter, r *http.Request) {
	h.change(w, r, h.center.Archive)
}

func (h *Handler) unarchive(w http.ResponseWriter, r *http.Request) {
	h.change(w, r, h.center.Unarchive)
}

// change, tek bir bildirimin durumunu değiştirir; bildirim yoksa ya da başkasınınsa 404.
func (h *Handler) change(w http.ResponseWriter, r *http.Request, fn func(context.Context, string, int64) error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.NotFound(w, r)
		return
	}
	if h.failed(w, r, fn(r.Context(), userID(r), id)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type preferenceResponse struct {
	Type      string `json:"type"`
	Category  string `json:"category"`
	NameTR    string `json:"name_tr"`
	NameEN    string `json:"name_en"`
	Mandatory bool   `json:"mandatory"`
	InApp     bool   `json:"in_app"`
	Email     bool   `json:"email"`
}

func (h *Handler) writePreferences(w http.ResponseWriter, r *http.Request, prefs []Preference) {
	res := httpx.ListResponse[preferenceResponse]{Items: make([]preferenceResponse, 0, len(prefs))}
	for _, p := range prefs {
		res.Items = append(res.Items, preferenceResponse{Type: string(p.Type), Category: p.Category, NameTR: p.NameTR,
			NameEN: p.NameEN, Mandatory: p.Mandatory, InApp: p.InApp, Email: p.Email})
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

func (h *Handler) preferences(w http.ResponseWriter, r *http.Request) {
	prefs, err := h.center.Preferences(r.Context(), userID(r))
	if h.failed(w, r, err) {
		return
	}
	h.writePreferences(w, r, prefs)
}

type preferencesRequest struct {
	Items []struct {
		Type  string `json:"type"`
		InApp *bool  `json:"in_app"`
		Email *bool  `json:"email"`
	} `json:"items"`
}

func (h *Handler) setPreferences(w http.ResponseWriter, r *http.Request) {
	var req preferencesRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	var (
		errs []httpx.FieldError
		in   []PreferenceInput
	)
	for i, it := range req.Items {
		field := "items[" + strconv.Itoa(i) + "]"
		if it.Type == "" || it.InApp == nil || it.Email == nil {
			errs = append(errs, httpx.FieldError{Field: field, Message: "type, in_app ve email zorunlu"})
			continue
		}
		in = append(in, PreferenceInput{Type: Type(it.Type), InApp: *it.InApp, Email: *it.Email})
	}
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	prefs, err := h.center.SetPreferences(r.Context(), userID(r), in)
	switch {
	case errors.Is(err, ErrMandatory):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "items", Message: "zorunlu bildirimler kapatılamaz"}})
		return
	case errors.Is(err, ErrUnknownType):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "items", Message: "bilinmeyen bildirim türü"}})
		return
	case h.failed(w, r, err):
		return
	}
	h.writePreferences(w, r, prefs)
}

func userID(r *http.Request) string {
	p, _ := authn.PrincipalFrom(r.Context())
	return p.UserID
}

func (h *Handler) failed(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, ErrNotFound):
		httpx.NotFound(w, r)
	default:
		h.logger.Error("bildirim isteği başarısız", "err", err, "request_id", httpx.RequestIDFrom(r.Context()))
		httpx.InternalServerError(w, r)
	}
	return true
}

func (h *Handler) writeJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	if err := httpx.WriteJSON(w, status, v); err != nil {
		h.logger.Error("yanıt yazılamadı", "err", err, "request_id", httpx.RequestIDFrom(r.Context()))
	}
}
