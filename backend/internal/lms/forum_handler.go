package lms

import (
	"net/http"
	"time"

	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

// registerForums, forum uçlarını kaydeder. Forumu görebilen (etkinliğe erişen) konuları okur;
// konu açma ve yanıt forumun türüne ve role göre (bkz. CanStartThread, CanReply); gönderiyi
// yazarı ya da öğretim elemanı düzenler ve siler; sabitleme ve kilitleme öğretim elemanınındır.
func (h *Handler) registerForums(rt *authz.Router) {
	rt.HandleFunc("GET /api/v1/activities/{id}/threads", authz.Authenticated, h.listThreads)
	rt.HandleFunc("POST /api/v1/activities/{id}/threads", authz.Authenticated, h.createThread)
	rt.HandleFunc("GET /api/v1/threads/{id}", authz.Authenticated, h.getThread)
	rt.HandleFunc("PUT /api/v1/threads/{id}", authz.Authenticated, h.updateThread)
	rt.HandleFunc("DELETE /api/v1/threads/{id}", authz.Authenticated, h.deleteThread)
	rt.HandleFunc("POST /api/v1/threads/{id}/posts", authz.Authenticated, h.reply)
	rt.HandleFunc("PUT /api/v1/posts/{id}", authz.Authenticated, h.editPost)
	rt.HandleFunc("DELETE /api/v1/posts/{id}", authz.Authenticated, h.deletePost)
}

type authorResponse struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Instructor bool   `json:"is_instructor"`
}

type threadSummaryResponse struct {
	ID         string         `json:"id"`
	Title      string         `json:"title"`
	Author     authorResponse `json:"author"`
	Pinned     bool           `json:"is_pinned"`
	Locked     bool           `json:"is_locked"`
	PostCount  int            `json:"post_count"`
	LastPostAt time.Time      `json:"last_post_at"`
	CreatedAt  time.Time      `json:"created_at"`
}

type threadListResponse struct {
	Forum    forumResponse           `json:"forum"`
	CanStart bool                    `json:"can_start"`
	Items    []threadSummaryResponse `json:"items"`
}

type postResponse struct {
	ID        string         `json:"id"`
	ParentID  *string        `json:"parent_post_id"`
	Author    authorResponse `json:"author"`
	Body      string         `json:"body_html"`
	CreatedAt time.Time      `json:"created_at"`
	EditedAt  *time.Time     `json:"edited_at"`
	Deleted   bool           `json:"deleted"`
	CanEdit   bool           `json:"can_edit"`
}

type threadResponse struct {
	threadSummaryResponse
	ActivityID  string         `json:"activity_id"`
	Forum       forumResponse  `json:"forum"`
	CanReply    bool           `json:"can_reply"`
	CanModerate bool           `json:"can_moderate"`
	Posts       []postResponse `json:"posts"`
}

func toThreadSummary(t ThreadSummary) threadSummaryResponse {
	return threadSummaryResponse{ID: t.ID, Title: t.Title, Author: authorResponse(t.Author), Pinned: t.Pinned, Locked: t.Locked,
		PostCount: t.PostCount, LastPostAt: t.LastPostAt, CreatedAt: t.CreatedAt}
}

// forum, path'teki forum etkinliğini, ayarlarını, şubesini ve rolü çözer; etkinlik erişilebilir
// olmalı.
func (h *Handler) forum(w http.ResponseWriter, r *http.Request, activityID string) (Activity, ForumSettings, Section, Role, bool) {
	sectionID, typ, err := h.svc.ActivitySection(r.Context(), activityID)
	if h.failed(w, r, err) {
		return Activity{}, ForumSettings{}, Section{}, RoleNone, false
	}
	if typ != TypeForum {
		httpx.NotFound(w, r)
		return Activity{}, ForumSettings{}, Section{}, RoleNone, false
	}
	sec, role, ok := h.space(w, r, sectionID)
	if !ok {
		return Activity{}, ForumSettings{}, Section{}, RoleNone, false
	}
	a, _, err := h.svc.Activity(r.Context(), activityID)
	if h.failed(w, r, err) {
		return Activity{}, ForumSettings{}, Section{}, RoleNone, false
	}
	if h.failed(w, r, h.svc.Accessible(r.Context(), a, role)) {
		return Activity{}, ForumSettings{}, Section{}, RoleNone, false
	}
	if a.Forum == nil {
		httpx.NotFound(w, r)
		return Activity{}, ForumSettings{}, Section{}, RoleNone, false
	}
	return a, *a.Forum, sec, role, true
}

func (h *Handler) listThreads(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return
	}
	a, f, sec, role, ok := h.forum(w, r, id)
	if !ok {
		return
	}
	list, err := h.svc.Threads(r.Context(), sec.ID, a.ID)
	if h.failed(w, r, err) {
		return
	}
	res := threadListResponse{Forum: forumResponse(f), CanStart: f.CanStartThread(role), Items: make([]threadSummaryResponse, 0, len(list))}
	for _, t := range list {
		res.Items = append(res.Items, toThreadSummary(t))
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

func (h *Handler) forumForbidden(w http.ResponseWriter, r *http.Request, detail string) {
	_ = httpx.WriteProblem(w, r, httpx.Problem{Status: http.StatusForbidden, Code: "FORBIDDEN", Detail: detail})
}

func (h *Handler) createThread(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return
	}
	a, f, _, role, ok := h.forum(w, r, id)
	if !ok {
		return
	}
	if !f.CanStartThread(role) {
		h.forumForbidden(w, r, "Bu forumda konuyu öğretim elemanları açar.")
		return
	}
	var req struct {
		Title string `json:"title"`
		Body  string `json:"body_html"`
	}
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	threadID, err := h.svc.CreateThread(r.Context(), userID(r), a.ID, req.Title, req.Body)
	if h.failed(w, r, err) {
		return
	}
	w.Header().Set("Location", "/api/v1/threads/"+threadID)
	h.writeThread(w, r, threadID, f, role, http.StatusCreated)
}

// thread, path'teki konunun forumunu çözer ve erişimi denetler.
func (h *Handler) thread(w http.ResponseWriter, r *http.Request) (string, ForumSettings, Role, bool) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return "", ForumSettings{}, RoleNone, false
	}
	activityID, _, _, err := h.svc.ThreadRef(r.Context(), id)
	if h.failed(w, r, err) {
		return "", ForumSettings{}, RoleNone, false
	}
	_, f, _, role, ok := h.forum(w, r, activityID)
	return id, f, role, ok
}

func (h *Handler) writeThread(w http.ResponseWriter, r *http.Request, threadID string, f ForumSettings, role Role, status int) {
	t, err := h.svc.Thread(r.Context(), threadID)
	if h.failed(w, r, err) {
		return
	}
	me := userID(r)
	res := threadResponse{threadSummaryResponse: toThreadSummary(t.ThreadSummary), ActivityID: t.ActivityID, Forum: forumResponse(f),
		CanReply: f.CanReply(role, t.Locked), CanModerate: role.CanEdit(), Posts: make([]postResponse, 0, len(t.Posts))}
	for _, p := range t.Posts {
		res.Posts = append(res.Posts, postResponse{ID: p.ID, ParentID: p.ParentID, Author: authorResponse(p.Author), Body: p.Body,
			CreatedAt: p.CreatedAt, EditedAt: p.EditedAt, Deleted: p.Deleted,
			CanEdit: !p.Deleted && (role.CanEdit() || (p.Author.ID == me && role != RoleViewer))})
	}
	h.writeJSON(w, r, status, res)
}

func (h *Handler) getThread(w http.ResponseWriter, r *http.Request) {
	if id, f, role, ok := h.thread(w, r); ok {
		h.writeThread(w, r, id, f, role, http.StatusOK)
	}
}

func (h *Handler) updateThread(w http.ResponseWriter, r *http.Request) {
	id, f, role, ok := h.thread(w, r)
	if !ok {
		return
	}
	var req struct {
		Title  *string `json:"title"`
		Pinned *bool   `json:"is_pinned"`
		Locked *bool   `json:"is_locked"`
	}
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	t, err := h.svc.Thread(r.Context(), id)
	if h.failed(w, r, err) {
		return
	}
	switch {
	case (req.Pinned != nil || req.Locked != nil) && !role.CanEdit():
		h.forumForbidden(w, r, "Konuyu sabitlemek ve kilitlemek öğretim elemanlarınındır.")
		return
	case req.Title != nil && !role.CanEdit() && (t.Author.ID != userID(r) || role == RoleViewer):
		h.forumForbidden(w, r, "Konunun başlığını yazarı ya da öğretim elemanı değiştirir.")
		return
	}
	if h.failed(w, r, h.svc.UpdateThread(r.Context(), userID(r), id, ThreadUpdate{Title: req.Title, Pinned: req.Pinned, Locked: req.Locked})) {
		return
	}
	h.writeThread(w, r, id, f, role, http.StatusOK)
}

func (h *Handler) deleteThread(w http.ResponseWriter, r *http.Request) {
	id, _, role, ok := h.thread(w, r)
	if !ok {
		return
	}
	t, err := h.svc.Thread(r.Context(), id)
	if h.failed(w, r, err) {
		return
	}
	if !role.CanEdit() && (t.Author.ID != userID(r) || role == RoleViewer) {
		h.forumForbidden(w, r, "Konuyu yazarı ya da öğretim elemanı siler.")
		return
	}
	if h.failed(w, r, h.svc.DeleteThread(r.Context(), userID(r), id)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) reply(w http.ResponseWriter, r *http.Request) {
	id, f, role, ok := h.thread(w, r)
	if !ok {
		return
	}
	_, _, locked, err := h.svc.ThreadRef(r.Context(), id)
	if h.failed(w, r, err) {
		return
	}
	if !f.CanReply(role, locked) {
		detail := "Bu foruma öğretim elemanları yazar."
		if locked {
			detail = "Konu kilitli: yeni yanıt yazılamaz."
		}
		h.forumForbidden(w, r, detail)
		return
	}
	var req struct {
		Body     string  `json:"body_html"`
		ParentID *string `json:"parent_post_id"`
	}
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	parent := ""
	if req.ParentID != nil {
		if !httpx.ValidUUID(*req.ParentID) {
			httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "parent_post_id", Message: "Geçerli bir UUID olmalı."}})
			return
		}
		parent = *req.ParentID
	}
	if _, err := h.svc.Reply(r.Context(), userID(r), id, parent, req.Body); h.failed(w, r, err) {
		return
	}
	h.writeThread(w, r, id, f, role, http.StatusCreated)
}

// post, path'teki gönderiyi, konusunu ve düzenleme yetkisini çözer: yazarı ya da öğretim elemanı.
func (h *Handler) post(w http.ResponseWriter, r *http.Request) (postID, threadID string, f ForumSettings, role Role, ok bool) {
	postID = r.PathValue("id")
	if !httpx.ValidUUID(postID) {
		httpx.NotFound(w, r)
		return "", "", ForumSettings{}, RoleNone, false
	}
	threadID, authorID, _, err := h.svc.PostRef(r.Context(), postID)
	if h.failed(w, r, err) {
		return "", "", ForumSettings{}, RoleNone, false
	}
	activityID, _, _, err := h.svc.ThreadRef(r.Context(), threadID)
	if h.failed(w, r, err) {
		return "", "", ForumSettings{}, RoleNone, false
	}
	_, f, _, role, ok = h.forum(w, r, activityID)
	if !ok {
		return "", "", ForumSettings{}, RoleNone, false
	}
	if !role.CanEdit() && (authorID != userID(r) || role == RoleViewer) {
		h.forumForbidden(w, r, "Gönderiyi yazarı ya da öğretim elemanı değiştirir.")
		return "", "", ForumSettings{}, RoleNone, false
	}
	return postID, threadID, f, role, true
}

func (h *Handler) editPost(w http.ResponseWriter, r *http.Request) {
	postID, threadID, f, role, ok := h.post(w, r)
	if !ok {
		return
	}
	var req struct {
		Body string `json:"body_html"`
	}
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	if h.failed(w, r, h.svc.EditPost(r.Context(), userID(r), postID, req.Body)) {
		return
	}
	h.writeThread(w, r, threadID, f, role, http.StatusOK)
}

func (h *Handler) deletePost(w http.ResponseWriter, r *http.Request) {
	postID, _, _, _, ok := h.post(w, r)
	if !ok {
		return
	}
	if h.failed(w, r, h.svc.DeletePost(r.Context(), userID(r), postID)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
