package messaging

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

// Handler, mesajlaşmanın HTTP uçlarıdır. Konuşmaya erişemeyen için konuşma yokmuş gibi 404.
type Handler struct {
	svc    *Service
	logger *slog.Logger
	send   []httpx.Middleware // ileti gönderme ucunun ek ara katmanları (ör. kullanıcı başına hız sınırı)
}

// NewHandler, bir Handler oluşturur.
func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// WithSendMiddleware, ileti gönderme ucuna ara katmanlar ekler.
func (h *Handler) WithSendMiddleware(mws ...httpx.Middleware) *Handler {
	h.send = mws
	return h
}

// Register, uçları yönlendiriciye ekler.
func (h *Handler) Register(rt *authz.Router) {
	rt.HandleFunc("GET /api/v1/me/conversations", authz.Authenticated, h.list)
	rt.HandleFunc("GET /api/v1/me/conversations/unread-count", authz.Authenticated, h.unread)
	rt.HandleFunc("GET /api/v1/messaging/contacts", authz.Authenticated, h.contacts)
	rt.HandleFunc("POST /api/v1/conversations", authz.Authenticated, h.create)
	rt.HandleFunc("GET /api/v1/conversations/{id}", authz.Authenticated, h.get)
	rt.HandleFunc("GET /api/v1/conversations/{id}/messages", authz.Authenticated, h.messages)
	rt.HandleFunc("POST /api/v1/conversations/{id}/messages", authz.Authenticated, h.post, h.send...)
	rt.HandleFunc("POST /api/v1/conversations/{id}/read", authz.Authenticated, h.read)
}

type personDTO struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Number string `json:"number"`
	Staff  bool   `json:"is_staff"`
}

type messageDTO struct {
	ID        string     `json:"id"`
	Sender    personDTO  `json:"sender"`
	Body      string     `json:"body"`
	ReplyToID *string    `json:"reply_to_id"`
	CreatedAt time.Time  `json:"created_at"`
	EditedAt  *time.Time `json:"edited_at"`
	Deleted   bool       `json:"deleted"`
	Mine      bool       `json:"mine"`
}

type conversationDTO struct {
	ID            string      `json:"id"`
	Kind          string      `json:"kind"`
	Title         string      `json:"title"`
	SectionID     *string     `json:"section_id"`
	Peer          *personDTO  `json:"peer"`
	LastMessage   *messageDTO `json:"last_message"`
	LastMessageAt *time.Time  `json:"last_message_at"`
	CreatedAt     time.Time   `json:"created_at"`
	Unread        int         `json:"unread"`
	Starred       bool        `json:"is_starred"`
	MutedUntil    *time.Time  `json:"muted_until"`
	Moderator     bool        `json:"is_moderator"`
	MemberCount   *int        `json:"member_count,omitempty"`
	Members       []personDTO `json:"members,omitempty"`
}

func toPerson(p Person) personDTO {
	return personDTO(p)
}

func toMessage(m Message, me string) messageDTO {
	return messageDTO{ID: m.ID, Sender: toPerson(m.Sender), Body: m.Body, ReplyToID: m.ReplyToID, CreatedAt: m.CreatedAt,
		EditedAt: m.EditedAt, Deleted: m.DeletedAt != nil, Mine: m.Sender.ID == me}
}

func toConversation(c Conversation, me string) conversationDTO {
	res := conversationDTO{ID: c.ID, Kind: c.Kind, Title: c.Title, LastMessageAt: c.LastMessageAt, CreatedAt: c.CreatedAt,
		Unread: c.Unread, Starred: c.Starred, MutedUntil: c.MutedUntil, Moderator: c.Moderator}
	if c.SectionID != "" {
		id := c.SectionID
		res.SectionID = &id
	}
	if c.Peer != nil {
		p := toPerson(*c.Peer)
		res.Peer = &p
	}
	if c.LastMessage != nil {
		m := toMessage(*c.LastMessage, me)
		res.LastMessage = &m
	}
	return res
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	me := userID(r)
	list, err := h.svc.Conversations(r.Context(), me)
	if h.failed(w, r, err) {
		return
	}
	res := httpx.ListResponse[conversationDTO]{Items: make([]conversationDTO, 0, len(list))}
	for _, c := range list {
		res.Items = append(res.Items, toConversation(c, me))
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

func (h *Handler) unread(w http.ResponseWriter, r *http.Request) {
	n, err := h.svc.UnreadTotal(r.Context(), userID(r))
	if h.failed(w, r, err) {
		return
	}
	h.writeJSON(w, r, http.StatusOK, map[string]int{"count": n})
}

func (h *Handler) contacts(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.Contacts(r.Context(), userID(r), r.URL.Query().Get("q"), 20)
	if h.failed(w, r, err) {
		return
	}
	res := httpx.ListResponse[personDTO]{Items: make([]personDTO, 0, len(list))}
	for _, p := range list {
		res.Items = append(res.Items, toPerson(p))
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

type createRequest struct {
	Kind      string   `json:"kind"`
	UserID    string   `json:"user_id"`
	Title     string   `json:"title"`
	MemberIDs []string `json:"member_ids"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	var (
		id  string
		err error
	)
	switch req.Kind {
	case KindDirect:
		if !httpx.ValidUUID(req.UserID) {
			httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "user_id", Message: "geçersiz kullanıcı"}})
			return
		}
		id, err = h.svc.Direct(r.Context(), userID(r), req.UserID)
	case KindGroup:
		for _, m := range req.MemberIDs {
			if !httpx.ValidUUID(m) {
				httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "member_ids", Message: "geçersiz kullanıcı"}})
				return
			}
		}
		id, err = h.svc.CreateGroup(r.Context(), userID(r), req.Title, req.MemberIDs)
	default:
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "kind", Message: "DIRECT ya da GROUP olmalı"}})
		return
	}
	if h.failed(w, r, err) {
		return
	}
	h.writeConversation(w, r, id, http.StatusCreated)
}

func (h *Handler) writeConversation(w http.ResponseWriter, r *http.Request, id string, status int) {
	me := userID(r)
	c, members, err := h.svc.Conversation(r.Context(), id, me)
	if h.failed(w, r, err) {
		return
	}
	res := toConversation(c, me)
	res.MemberCount = &c.MemberCount
	res.Members = make([]personDTO, 0, len(members))
	for _, p := range members {
		res.Members = append(res.Members, toPerson(p))
	}
	h.writeJSON(w, r, status, res)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return
	}
	h.writeConversation(w, r, id, http.StatusOK)
}

func (h *Handler) messages(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return
	}
	q := r.URL.Query()
	limit, limitErr := httpx.ParseLimit(q)
	before := q.Get("before")
	var errs []httpx.FieldError
	if limitErr != nil {
		errs = append(errs, *limitErr)
	}
	if before != "" && !httpx.ValidUUID(before) {
		errs = append(errs, httpx.FieldError{Field: "before", Message: "geçersiz ileti"})
	}
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	me := userID(r)
	list, more, err := h.svc.Messages(r.Context(), id, me, before, limit)
	if h.failed(w, r, err) {
		return
	}
	res := struct {
		Items   []messageDTO `json:"items"`
		HasMore bool         `json:"has_more"`
	}{Items: make([]messageDTO, 0, len(list)), HasMore: more}
	for _, m := range list {
		res.Items = append(res.Items, toMessage(m, me))
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

type postRequest struct {
	Body      string `json:"body"`
	ReplyToID string `json:"reply_to_id"`
}

func (h *Handler) post(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return
	}
	var req postRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	if req.ReplyToID != "" && !httpx.ValidUUID(req.ReplyToID) {
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "reply_to_id", Message: "geçersiz ileti"}})
		return
	}
	me := userID(r)
	msgID, err := h.svc.Send(r.Context(), me, id, req.Body, req.ReplyToID)
	if h.failed(w, r, err) {
		return
	}
	m, err := h.svc.Message(r.Context(), msgID)
	if h.failed(w, r, err) {
		return
	}
	h.writeJSON(w, r, http.StatusCreated, toMessage(m, me))
}

type readRequest struct {
	MessageID string `json:"message_id"`
}

func (h *Handler) read(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return
	}
	var req readRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	if !httpx.ValidUUID(req.MessageID) {
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "message_id", Message: "geçersiz ileti"}})
		return
	}
	if h.failed(w, r, h.svc.MarkRead(r.Context(), userID(r), id, req.MessageID)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

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
	case errors.Is(err, ErrNotAllowed):
		_ = httpx.WriteProblem(w, r, httpx.Problem{Status: http.StatusForbidden, Code: "MESSAGING_NOT_ALLOWED",
			Detail: "Bu kişiye ileti gönderemezsiniz."})
	default:
		h.logger.Error("mesajlaşma isteği başarısız", "err", err, "request_id", httpx.RequestIDFrom(r.Context()))
		httpx.InternalServerError(w, r)
	}
	return true
}

func (h *Handler) writeJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	if err := httpx.WriteJSON(w, status, v); err != nil {
		h.logger.Error("yanıt yazılamadı", "err", err, "request_id", httpx.RequestIDFrom(r.Context()))
	}
}
