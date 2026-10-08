package audit

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

// Handler, denetim kayıtlarını okuma uçlarını sunar.
type Handler struct {
	repo   *Repository
	logger *slog.Logger
}

// NewHandler, bir Handler oluşturur.
func NewHandler(repo *Repository, logger *slog.Logger) *Handler {
	return &Handler{repo: repo, logger: logger}
}

// Register, route'ları kaydeder. Denetim kayıtlarını sadece audit:read yetkisi
// olanlar (sistem yöneticisi, denetçi) okuyabilir.
func (h *Handler) Register(rt *authz.Router) {
	rt.HandleFunc("GET /api/v1/audit/log", authz.Permission("audit:read"), h.listLog)
	rt.HandleFunc("GET /api/v1/audit/security-events", authz.Permission("audit:read"), h.listSecurityEvents)

	// Kullanıcının kendi hesabındaki olaylar: giriş geçmişi, başarısız denemeler.
	rt.HandleFunc("GET /api/v1/me/security-events", authz.SelfService, h.listMySecurityEvents)
}

type logEntryResponse struct {
	ID          int64           `json:"id"`
	OccurredAt  time.Time       `json:"occurred_at"`
	ActorUserID *string         `json:"actor_user_id"`
	ActorName   *string         `json:"actor_username"`
	Action      string          `json:"action"`
	EntityType  string          `json:"entity_type"`
	EntityID    *string         `json:"entity_id"`
	Before      json.RawMessage `json:"before"`
	After       json.RawMessage `json:"after"`
	IP          *string         `json:"ip"`
	UserAgent   *string         `json:"user_agent"`
	RequestID   *string         `json:"request_id"`
}

// securityEventResponse, bir güvenlik olayının API biçimidir.
type securityEventResponse struct {
	ID                int64           `json:"id"`
	OccurredAt        time.Time       `json:"occurred_at"`
	Type              string          `json:"type"`
	UserID            *string         `json:"user_id"`
	Username          *string         `json:"username"`
	UsernameAttempted *string         `json:"username_attempted"`
	IP                *string         `json:"ip"`
	UserAgent         *string         `json:"user_agent"`
	RequestID         *string         `json:"request_id"`
	Details           json.RawMessage `json:"details"`
}

func tosecurityEventResponse(e StoredSecurityEvent) securityEventResponse {
	return securityEventResponse{
		ID:                e.ID,
		OccurredAt:        e.OccurredAt,
		Type:              e.Type,
		UserID:            optional(e.UserID),
		Username:          optional(e.Username),
		UsernameAttempted: optional(e.UsernameAttempted),
		IP:                optional(e.IP),
		UserAgent:         optional(e.UserAgent),
		RequestID:         optional(e.RequestID),
		Details:           e.Details,
	}
}

func (h *Handler) listLog(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var errs []httpx.FieldError

	f := LogFilter{
		ActorUserID: uuidParam(q, "actor_user_id", &errs),
		Action:      q.Get("action"),
		EntityType:  q.Get("entity_type"),
		EntityID:    q.Get("entity_id"),
	}
	f.From, f.To, f.After, f.Limit = commonParams(q, &errs)
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}

	entries, hasMore, err := h.repo.ListLog(r.Context(), f)
	if err != nil {
		h.serverError(w, r, err)
		return
	}

	res := httpx.ListResponse[logEntryResponse]{Items: make([]logEntryResponse, 0, len(entries))}
	for _, e := range entries {
		res.Items = append(res.Items, logEntryResponse{
			ID:          e.ID,
			OccurredAt:  e.OccurredAt,
			ActorUserID: optional(e.ActorUserID),
			ActorName:   optional(e.ActorName),
			Action:      e.Action,
			EntityType:  e.EntityType,
			EntityID:    optional(e.EntityID),
			Before:      nullJSON(e.Before),
			After:       nullJSON(e.After),
			IP:          optional(e.IP),
			UserAgent:   optional(e.UserAgent),
			RequestID:   optional(e.RequestID),
		})
	}
	if hasMore {
		last := entries[len(entries)-1]
		res.NextCursor, err = httpx.EncodeCursor(Cursor{OccurredAt: last.OccurredAt, ID: last.ID})
		if err != nil {
			h.serverError(w, r, err)
			return
		}
	}
	h.writeJSON(w, r, res)
}

func (h *Handler) listSecurityEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var errs []httpx.FieldError

	f := SecurityEventFilter{
		UserID: uuidParam(q, "user_id", &errs),
		Type:   q.Get("type"),
	}
	f.From, f.To, f.After, f.Limit = commonParams(q, &errs)
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}

	events, hasMore, err := h.repo.ListSecurityEvents(r.Context(), f)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	res, err := securityEventPage(events, hasMore)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	h.writeJSON(w, r, res)
}

func (h *Handler) listMySecurityEvents(w http.ResponseWriter, r *http.Request) {
	principal, ok := authn.PrincipalFrom(r.Context())
	if !ok {
		h.serverError(w, r, errors.New("audit: /me/security-events kimlik doğrulama olmadan çağrıldı"))
		return
	}

	var errs []httpx.FieldError
	f := parseSecurityEventParams(r.URL.Query(), &errs)
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	f.UserID = principal.UserID // başka bir kullanıcının olayları istenemez

	events, hasMore, err := h.repo.ListSecurityEvents(r.Context(), f)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	res, err := securityEventPage(events, hasMore)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	h.writeJSON(w, r, res)
}

// securityEventPage, bir güvenlik olayı sayfasını liste yanıtına çevirir.
func securityEventPage(events []StoredSecurityEvent, hasMore bool) (httpx.ListResponse[securityEventResponse], error) {
	res := httpx.ListResponse[securityEventResponse]{Items: make([]securityEventResponse, 0, len(events))}
	for _, e := range events {
		res.Items = append(res.Items, tosecurityEventResponse(e))
	}
	if hasMore {
		last := events[len(events)-1]
		next, err := httpx.EncodeCursor(Cursor{OccurredAt: last.OccurredAt, ID: last.ID})
		if err != nil {
			return res, err
		}
		res.NextCursor = next
	}
	return res, nil
}

// parseSecurityEventParams, güvenlik olayı listelerinin ortak sorgu parametrelerini okur.
func parseSecurityEventParams(q url.Values, errs *[]httpx.FieldError) SecurityEventFilter {
	f := SecurityEventFilter{Type: q.Get("type")}
	f.From, f.To, f.After, f.Limit = commonParams(q, errs)
	return f
}

// commonParams, zaman aralığı, cursor ve limit parametrelerini okur.
func commonParams(q url.Values, errs *[]httpx.FieldError) (from, to *time.Time, after *Cursor, limit int) {
	from = timeParam(q, "from", errs)
	to = timeParam(q, "to", errs)
	if from != nil && to != nil && !from.Before(*to) {
		*errs = append(*errs, httpx.FieldError{Field: "to", Message: "from'dan sonra olmalı"})
	}

	limit, limitErr := httpx.ParseLimit(q)
	if limitErr != nil {
		*errs = append(*errs, *limitErr)
	}

	if c := q.Get("cursor"); c != "" {
		cur, err := httpx.DecodeCursor[Cursor](c)
		if err != nil || cur.ID <= 0 {
			*errs = append(*errs, httpx.FieldError{Field: "cursor", Message: "geçersiz cursor"})
		} else {
			after = &cur
		}
	}
	return from, to, after, limit
}

func timeParam(q url.Values, name string, errs *[]httpx.FieldError) *time.Time {
	v := q.Get(name)
	if v == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		*errs = append(*errs, httpx.FieldError{Field: name, Message: "RFC 3339 biçiminde bir zaman olmalı (ör. 2026-10-08T00:00:00Z)"})
		return nil
	}
	return &t
}

func uuidParam(q url.Values, name string, errs *[]httpx.FieldError) string {
	v := q.Get(name)
	if v != "" && !httpx.ValidUUID(v) {
		*errs = append(*errs, httpx.FieldError{Field: name, Message: "geçerli bir UUID olmalı"})
	}
	return v
}

func (h *Handler) writeJSON(w http.ResponseWriter, r *http.Request, v any) {
	if err := httpx.WriteJSON(w, http.StatusOK, v); err != nil {
		h.logger.Error("yanıt yazılamadı", "err", err, "request_id", httpx.RequestIDFrom(r.Context()))
	}
}

func (h *Handler) serverError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.Error("beklenmeyen hata", "err", err, "path", r.URL.Path, "request_id", httpx.RequestIDFrom(r.Context()))
	httpx.InternalServerError(w, r)
}

// nullJSON, boş bir JSON değerini açıkça null yapar: json.RawMessage(nil) kodlanırken
// "null" yazılır ama okunurken boş dizi gelebilir.
func nullJSON(m json.RawMessage) json.RawMessage {
	if len(m) == 0 {
		return json.RawMessage("null")
	}
	return m
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
