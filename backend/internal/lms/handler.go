package lms

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/files"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

// Handler, ders alanı uçlarını sunar.
type Handler struct {
	svc    *Service
	logger *slog.Logger
}

// NewHandler, bir Handler oluşturur.
func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// Register, route'ları kaydeder. Erişim ilişkiye dayalıdır (bkz. space): ders alanını üyeler
// (öğrenci, öğretim elemanı) ve şubenin bölümünü yöneten personel (salt okunur) görür, içeriği
// şubenin öğretim elemanları düzenler.
func (h *Handler) Register(rt *authz.Router) {
	rt.HandleFunc("GET /api/v1/me/spaces", authz.Authenticated, h.mySpaces)
	rt.HandleFunc("GET /api/v1/me/timeline", authz.Authenticated, h.timeline)
	rt.HandleFunc("GET /api/v1/spaces/{id}", authz.Authenticated, h.getSpace)
	rt.HandleFunc("PUT /api/v1/spaces/{id}", authz.Authenticated, h.setSummary)
	rt.HandleFunc("PUT /api/v1/spaces/{id}/layout", authz.Authenticated, h.reorder)
	rt.HandleFunc("POST /api/v1/spaces/{id}/topics", authz.Authenticated, h.createTopic)
	rt.HandleFunc("PUT /api/v1/topics/{id}", authz.Authenticated, h.updateTopic)
	rt.HandleFunc("DELETE /api/v1/topics/{id}", authz.Authenticated, h.deleteTopic)
	rt.HandleFunc("POST /api/v1/topics/{id}/activities", authz.Authenticated, h.createActivity)
	rt.HandleFunc("GET /api/v1/activities/{id}", authz.Authenticated, h.getActivity)
	rt.HandleFunc("PUT /api/v1/activities/{id}", authz.Authenticated, h.updateActivity)
	rt.HandleFunc("DELETE /api/v1/activities/{id}", authz.Authenticated, h.deleteActivity)
	rt.HandleFunc("GET /api/v1/activities/{id}/files/{fileId}/download-url", authz.Authenticated, h.fileURL)
	rt.HandleFunc("POST /api/v1/activities/{id}/views", authz.Authenticated, h.view)
	h.registerAssignments(rt)
	h.registerForums(rt)
}

func userID(r *http.Request) string {
	p, _ := authn.PrincipalFrom(r.Context())
	return p.UserID
}

// space, şubeyi ve isteği yapanın rolünü çözer. Üye değilse şubenin bölümünü yöneten personel
// izleyicidir; ikisi de değilse ders alanı yokmuş gibi 404.
func (h *Handler) space(w http.ResponseWriter, r *http.Request, sectionID string) (Section, Role, bool) {
	if !httpx.ValidUUID(sectionID) {
		httpx.NotFound(w, r)
		return Section{}, RoleNone, false
	}
	sec, role, err := h.svc.Membership(r.Context(), h.svc.pool, sectionID, userID(r))
	if h.failed(w, r, err) {
		return Section{}, RoleNone, false
	}
	if role == RoleNone {
		perms, ok := authz.PermissionsFrom(r.Context())
		if ok && perms.Allows(permSectionManage, authz.Target{FacultyID: sec.FacultyID, DepartmentID: sec.DepartmentID}) {
			role = RoleViewer
		}
	}
	if role == RoleNone {
		httpx.NotFound(w, r)
		return Section{}, RoleNone, false
	}
	return sec, role, true
}

// editor, şubenin içeriğini düzenleyebilenleri geçirir.
func (h *Handler) editor(w http.ResponseWriter, r *http.Request, sectionID string) (Section, Role, bool) {
	sec, role, ok := h.space(w, r, sectionID)
	if !ok {
		return Section{}, RoleNone, false
	}
	if !role.CanEdit() {
		h.forbidden(w, r)
		return Section{}, RoleNone, false
	}
	return sec, role, true
}

// --- Yanıtlar -------------------------------------------------------------------

type courseResponse struct {
	ID     string `json:"id"`
	Code   string `json:"code"`
	NameTR string `json:"name_tr"`
	NameEN string `json:"name_en"`
}

type termResponse struct {
	ID   string `json:"id"`
	Code string `json:"code"`
}

type instructorResponse struct {
	UserID    string  `json:"user_id"`
	Title     *string `json:"title"`
	FirstName string  `json:"first_name"`
	LastName  string  `json:"last_name"`
	Email     string  `json:"email"`
	Role      Role    `json:"role"`
}

type spaceSummaryResponse struct {
	SectionID   string               `json:"section_id"`
	SectionCode string               `json:"section_code"`
	Course      courseResponse       `json:"course"`
	Term        termResponse         `json:"term"`
	Role        Role                 `json:"role"`
	Instructors []instructorResponse `json:"instructors"`
	Activities  int                  `json:"activities"`
	Progress    *progressResponse    `json:"progress"`
}

type fileRefResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	MimeType string `json:"mime_type"`
	Size     int64  `json:"size"`
}

type activityResponse struct {
	ID             string              `json:"id"`
	TopicID        string              `json:"topic_id"`
	Type           ActivityType        `json:"type"`
	Title          string              `json:"title"`
	Description    string              `json:"description_html"`
	Visible        bool                `json:"is_visible"`
	Availability   Availability        `json:"availability"`
	AvailableFrom  *time.Time          `json:"available_from"`
	AvailableUntil *time.Time          `json:"available_until"`
	Version        int                 `json:"version"`
	UpdatedAt      time.Time           `json:"updated_at"`
	Files          []fileRefResponse   `json:"files"`
	URL            *string             `json:"url"`
	OpenInNewTab   *bool               `json:"open_in_new_tab"`
	Body           *string             `json:"body_html"`
	Assignment     *assignmentResponse `json:"assignment"`
	Forum          *forumResponse      `json:"forum"`
	Completed      bool                `json:"completed"`
}

type progressResponse struct {
	Completed int `json:"completed"`
	Total     int `json:"total"`
}

type forumResponse struct {
	Kind             string     `json:"kind"`
	StudentsCanStart bool       `json:"students_can_start_threads"`
	Threads          int        `json:"threads"`
	LastPostAt       *time.Time `json:"last_post_at"`
}

type topicResponse struct {
	ID             string             `json:"id"`
	Title          string             `json:"title"`
	Summary        string             `json:"summary_html"`
	Visible        bool               `json:"is_visible"`
	Availability   Availability       `json:"availability"`
	AvailableFrom  *time.Time         `json:"available_from"`
	AvailableUntil *time.Time         `json:"available_until"`
	Version        int                `json:"version"`
	Activities     []activityResponse `json:"activities"`
}

type spaceResponse struct {
	SectionID   string               `json:"section_id"`
	SectionCode string               `json:"section_code"`
	Course      courseResponse       `json:"course"`
	Term        termResponse         `json:"term"`
	Role        Role                 `json:"role"`
	CanEdit     bool                 `json:"can_edit"`
	Summary     string               `json:"summary_html"`
	Instructors []instructorResponse `json:"instructors"`
	Topics      []topicResponse      `json:"topics"`
	Progress    *progressResponse    `json:"progress"`
}

func toCourse(s Section) courseResponse {
	return courseResponse{ID: s.CourseID, Code: s.CourseCode, NameTR: s.CourseNameTR, NameEN: s.CourseNameEN}
}

func toInstructors(list []Instructor) []instructorResponse {
	out := make([]instructorResponse, 0, len(list))
	for _, in := range list {
		r := instructorResponse{UserID: in.UserID, FirstName: in.FirstName, LastName: in.LastName, Email: in.Email, Role: in.Role}
		if in.Title != "" {
			r.Title = &in.Title
		}
		out = append(out, r)
	}
	return out
}

func toActivity(a Activity, now time.Time) activityResponse {
	res := activityResponse{
		ID: a.ID, TopicID: a.TopicID, Type: a.Type, Title: a.Title, Description: a.Description, Visible: a.Visible,
		Availability: a.Window.At(now), AvailableFrom: a.Window.From, AvailableUntil: a.Window.Until,
		Version: a.Version, UpdatedAt: a.UpdatedAt, Files: make([]fileRefResponse, 0, len(a.Files)),
	}
	for _, f := range a.Files {
		res.Files = append(res.Files, fileRefResponse(f))
	}
	if a.Type == TypeURL && a.URL != "" {
		res.URL, res.OpenInNewTab = &a.URL, &a.NewTab
	}
	if a.Type == TypePage && a.Body != "" {
		res.Body = &a.Body
	}
	if a.Assignment != nil {
		ar := toAssignment(*a.Assignment, now)
		res.Assignment = &ar
	}
	if a.Forum != nil {
		fr := forumResponse(*a.Forum)
		res.Forum = &fr
	}
	res.Completed = a.Completed
	return res
}

// term, istekteki dönemdir (term_id); verilmezse içinde bulunulan dönem. Dönem yoksa ok true
// ve termID boş döner.
func (h *Handler) term(w http.ResponseWriter, r *http.Request) (string, bool) {
	termID := r.URL.Query().Get("term_id")
	if termID != "" && !httpx.ValidUUID(termID) {
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "term_id", Message: "Geçerli bir UUID olmalı."}})
		return "", false
	}
	if termID == "" {
		err := h.svc.pool.QueryRow(r.Context(), `SELECT id FROM academic.terms WHERE is_current`).Scan(&termID)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", true
		}
		if h.failed(w, r, err) {
			return "", false
		}
	}
	return termID, true
}

func (h *Handler) mySpaces(w http.ResponseWriter, r *http.Request) {
	termID, ok := h.term(w, r)
	if !ok {
		return
	}
	if termID == "" {
		h.writeJSON(w, r, http.StatusOK, httpx.ListResponse[spaceSummaryResponse]{Items: []spaceSummaryResponse{}})
		return
	}
	list, err := h.svc.MySpaces(r.Context(), userID(r), termID)
	if h.failed(w, r, err) {
		return
	}
	res := httpx.ListResponse[spaceSummaryResponse]{Items: make([]spaceSummaryResponse, 0, len(list))}
	for _, s := range list {
		res.Items = append(res.Items, spaceSummaryResponse{
			SectionID: s.ID, SectionCode: s.Code, Course: toCourse(s.Section), Term: termResponse{ID: s.TermID, Code: s.TermCode},
			Role: s.Role, Instructors: toInstructors(s.Instructors), Activities: s.Activities,
			Progress: studentProgress(s.Role, s.Completed, s.Countable),
		})
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

// studentProgress, öğrencinin ilerlemesidir; diğer rollerde nil.
func studentProgress(role Role, completed, total int) *progressResponse {
	if role != RoleStudent {
		return nil
	}
	return &progressResponse{Completed: completed, Total: total}
}

func (h *Handler) writeSpace(w http.ResponseWriter, r *http.Request, sec Section, role Role, status int) {
	sp, err := h.svc.Space(r.Context(), sec, role, userID(r))
	if h.failed(w, r, err) {
		return
	}
	now := h.svc.now()
	res := spaceResponse{
		SectionID: sec.ID, SectionCode: sec.Code, Course: toCourse(sec), Term: termResponse{ID: sec.TermID, Code: sec.TermCode},
		Role: role, CanEdit: role.CanEdit(), Summary: sp.Summary, Instructors: toInstructors(sp.Instructors),
		Topics: make([]topicResponse, 0, len(sp.Topics)),
	}
	if sp.Progress != nil {
		res.Progress = &progressResponse{Completed: sp.Progress.Completed, Total: sp.Progress.Total}
	}
	for _, t := range sp.Topics {
		tr := topicResponse{ID: t.ID, Title: t.Title, Summary: t.Summary, Visible: t.Visible, Availability: t.Window.At(now),
			AvailableFrom: t.Window.From, AvailableUntil: t.Window.Until, Version: t.Version,
			Activities: make([]activityResponse, 0, len(t.Activities))}
		for _, a := range t.Activities {
			tr.Activities = append(tr.Activities, toActivity(a, now))
		}
		res.Topics = append(res.Topics, tr)
	}
	h.writeJSON(w, r, status, res)
}

func (h *Handler) getSpace(w http.ResponseWriter, r *http.Request) {
	if sec, role, ok := h.space(w, r, r.PathValue("id")); ok {
		h.writeSpace(w, r, sec, role, http.StatusOK)
	}
}

func (h *Handler) setSummary(w http.ResponseWriter, r *http.Request) {
	sec, role, ok := h.editor(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	var req struct {
		Summary string `json:"summary_html"`
	}
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	if len(req.Summary) > 50_000 {
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "summary_html", Message: "Metin çok uzun."}})
		return
	}
	if h.failed(w, r, h.svc.SetSummary(r.Context(), userID(r), sec.ID, req.Summary)) {
		return
	}
	h.writeSpace(w, r, sec, role, http.StatusOK)
}

func (h *Handler) reorder(w http.ResponseWriter, r *http.Request) {
	sec, role, ok := h.editor(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	var req struct {
		Topics []struct {
			ID          string   `json:"id"`
			ActivityIDs []string `json:"activity_ids"`
		} `json:"topics"`
	}
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	layout := make(Layout, 0, len(req.Topics))
	for _, t := range req.Topics {
		layout = append(layout, struct {
			TopicID     string
			ActivityIDs []string
		}{t.ID, t.ActivityIDs})
	}
	if h.failed(w, r, h.svc.Reorder(r.Context(), userID(r), sec.ID, layout)) {
		return
	}
	h.writeSpace(w, r, sec, role, http.StatusOK)
}

type windowRequest struct {
	AvailableFrom  *time.Time `json:"available_from"`
	AvailableUntil *time.Time `json:"available_until"`
}

type topicRequest struct {
	Title   string `json:"title"`
	Summary string `json:"summary_html"`
	Visible *bool  `json:"is_visible"`
	windowRequest
}

func (req topicRequest) input() TopicInput {
	return TopicInput{Title: req.Title, Summary: req.Summary, Visible: req.Visible == nil || *req.Visible,
		Window: Window{From: req.AvailableFrom, Until: req.AvailableUntil}}
}

func (h *Handler) createTopic(w http.ResponseWriter, r *http.Request) {
	sec, role, ok := h.editor(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	var req topicRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	if _, err := h.svc.CreateTopic(r.Context(), userID(r), sec.ID, req.input()); h.failed(w, r, err) {
		return
	}
	h.writeSpace(w, r, sec, role, http.StatusCreated)
}

// topicEditor, path'teki bölümün şubesinde düzenleme yetkisini denetler.
func (h *Handler) topicEditor(w http.ResponseWriter, r *http.Request) (string, Section, Role, bool) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return "", Section{}, RoleNone, false
	}
	sectionID, err := h.svc.TopicSection(r.Context(), id)
	if h.failed(w, r, err) {
		return "", Section{}, RoleNone, false
	}
	sec, role, ok := h.editor(w, r, sectionID)
	return id, sec, role, ok
}

func (h *Handler) updateTopic(w http.ResponseWriter, r *http.Request) {
	version, ok := httpx.IfMatchVersion(r)
	if !ok {
		httpx.PreconditionRequired(w, r)
		return
	}
	id, sec, role, ok := h.topicEditor(w, r)
	if !ok {
		return
	}
	var req topicRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	if h.failed(w, r, h.svc.UpdateTopic(r.Context(), userID(r), id, version, req.input())) {
		return
	}
	h.writeSpace(w, r, sec, role, http.StatusOK)
}

func (h *Handler) deleteTopic(w http.ResponseWriter, r *http.Request) {
	id, _, _, ok := h.topicEditor(w, r)
	if !ok {
		return
	}
	if h.failed(w, r, h.svc.DeleteTopic(r.Context(), userID(r), id)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type activityRequest struct {
	Type        ActivityType `json:"type"`
	Title       string       `json:"title"`
	Description string       `json:"description_html"`
	Visible     *bool        `json:"is_visible"`
	windowRequest
	FileIDs    []string           `json:"file_ids"`
	URL        string             `json:"url"`
	NewTab     *bool              `json:"open_in_new_tab"`
	Body       string             `json:"body_html"`
	Assignment *assignmentRequest `json:"assignment"`
	Forum      *struct {
		StudentsCanStart *bool `json:"students_can_start_threads"`
	} `json:"forum"`
}

func (req activityRequest) input(maxUpload int64) (ActivityInput, error) {
	in := ActivityInput{
		Type: req.Type, Title: req.Title, Description: req.Description, Visible: req.Visible == nil || *req.Visible,
		Window: Window{From: req.AvailableFrom, Until: req.AvailableUntil}, FileIDs: req.FileIDs,
		URL: req.URL, NewTab: req.NewTab == nil || *req.NewTab, Body: req.Body,
	}
	if req.Assignment != nil {
		a, err := req.Assignment.settings(maxUpload)
		if err != nil {
			return ActivityInput{}, err
		}
		in.Assignment = &a
	}
	if req.Forum != nil {
		in.Forum = &ForumSettings{StudentsCanStart: req.Forum.StudentsCanStart == nil || *req.Forum.StudentsCanStart}
	}
	return in, nil
}

func (h *Handler) createActivity(w http.ResponseWriter, r *http.Request) {
	topicID, _, role, ok := h.topicEditor(w, r)
	if !ok {
		return
	}
	var req activityRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	for _, f := range req.FileIDs {
		if !httpx.ValidUUID(f) {
			httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "file_ids", Message: "Geçerli UUID'ler olmalı."}})
			return
		}
	}
	in, err := req.input(h.svc.files.MaxBytes())
	if h.failed(w, r, err) {
		return
	}
	id, err := h.svc.CreateActivity(r.Context(), userID(r), topicID, in)
	if h.failed(w, r, err) {
		return
	}
	w.Header().Set("Location", "/api/v1/activities/"+id)
	h.writeActivity(w, r, id, role, http.StatusCreated)
}

func (h *Handler) writeActivity(w http.ResponseWriter, r *http.Request, id string, role Role, status int) {
	a, _, err := h.svc.Activity(r.Context(), id)
	if h.failed(w, r, err) {
		return
	}
	if h.failed(w, r, h.svc.Accessible(r.Context(), a, role)) {
		return
	}
	w.Header().Set("ETag", httpx.ETag(a.Version))
	h.writeJSON(w, r, status, toActivity(a, h.svc.now()))
}

// activity, path'teki etkinliğin şubesini ve isteği yapanın rolünü çözer.
func (h *Handler) activity(w http.ResponseWriter, r *http.Request) (string, Section, Role, bool) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return "", Section{}, RoleNone, false
	}
	sectionID, _, err := h.svc.ActivitySection(r.Context(), id)
	if h.failed(w, r, err) {
		return "", Section{}, RoleNone, false
	}
	sec, role, ok := h.space(w, r, sectionID)
	return id, sec, role, ok
}

func (h *Handler) getActivity(w http.ResponseWriter, r *http.Request) {
	id, _, role, ok := h.activity(w, r)
	if !ok {
		return
	}
	h.writeActivity(w, r, id, role, http.StatusOK)
	h.viewed(r, id, role)
}

// viewed, öğrencinin görüntüleyince tamamlanan bir etkinliği (sayfa, klasör, forum, bağlantı)
// açtığını kaydeder; erişim denetlenmiş olmalı. Hata yanıtı bozmaz, günlüğe yazılır.
func (h *Handler) viewed(r *http.Request, activityID string, role Role) {
	if role != RoleStudent {
		return
	}
	_, typ, err := h.svc.ActivitySection(r.Context(), activityID)
	if err == nil && typ.completesOnView() {
		err = h.svc.MarkComplete(r.Context(), userID(r), activityID, typ)
	}
	if err != nil {
		h.logger.Error("tamamlama kaydedilemedi", "err", err, "request_id", httpx.RequestIDFrom(r.Context()))
	}
}

// view, öğrencinin etkinliği ders alanından açtığını kaydeder (ör. bağlantı yeni sekmede açıldı).
func (h *Handler) view(w http.ResponseWriter, r *http.Request) {
	id, _, role, ok := h.activity(w, r)
	if !ok {
		return
	}
	a, _, err := h.svc.Activity(r.Context(), id)
	if h.failed(w, r, err) {
		return
	}
	if h.failed(w, r, h.svc.Accessible(r.Context(), a, role)) {
		return
	}
	h.viewed(r, id, role)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) updateActivity(w http.ResponseWriter, r *http.Request) {
	version, ok := httpx.IfMatchVersion(r)
	if !ok {
		httpx.PreconditionRequired(w, r)
		return
	}
	id, _, role, ok := h.activity(w, r)
	if !ok {
		return
	}
	if !role.CanEdit() {
		h.forbidden(w, r)
		return
	}
	var req activityRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	in, err := req.input(h.svc.files.MaxBytes())
	if h.failed(w, r, err) {
		return
	}
	if h.failed(w, r, h.svc.UpdateActivity(r.Context(), userID(r), id, version, in)) {
		return
	}
	h.writeActivity(w, r, id, role, http.StatusOK)
}

func (h *Handler) deleteActivity(w http.ResponseWriter, r *http.Request) {
	id, _, role, ok := h.activity(w, r)
	if !ok {
		return
	}
	if !role.CanEdit() {
		h.forbidden(w, r)
		return
	}
	if h.failed(w, r, h.svc.DeleteActivity(r.Context(), userID(r), id)) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) fileURL(w http.ResponseWriter, r *http.Request) {
	id, _, role, ok := h.activity(w, r)
	if !ok {
		return
	}
	fileID := r.PathValue("fileId")
	if !httpx.ValidUUID(fileID) {
		httpx.NotFound(w, r)
		return
	}
	a, _, err := h.svc.Activity(r.Context(), id)
	if h.failed(w, r, err) {
		return
	}
	if h.failed(w, r, h.svc.Accessible(r.Context(), a, role)) {
		return
	}
	u, exp, err := h.svc.FileURL(r.Context(), a, fileID)
	if h.failed(w, r, err) {
		return
	}
	// Dosyayı indiren öğrenci dosya ve klasör etkinliğini tamamlamış olur.
	if role == RoleStudent {
		if err := h.svc.MarkComplete(r.Context(), userID(r), id, a.Type); err != nil {
			h.logger.Error("tamamlama kaydedilemedi", "err", err, "request_id", httpx.RequestIDFrom(r.Context()))
		}
	}
	h.writeJSON(w, r, http.StatusOK, files.URLResponse{URL: u, ExpiresAt: exp})
}

func (h *Handler) forbidden(w http.ResponseWriter, r *http.Request) {
	_ = httpx.WriteProblem(w, r, httpx.Problem{Status: http.StatusForbidden, Code: "FORBIDDEN",
		Detail: "Ders içeriğini sadece şubenin öğretim elemanları düzenleyebilir."})
}

func (h *Handler) failed(w http.ResponseWriter, r *http.Request, err error) bool {
	var (
		verr     *ValidationError
		conflict *ConflictError
	)
	switch {
	case err == nil:
		return false
	case errors.Is(err, ErrNotFound):
		httpx.NotFound(w, r)
	case errors.Is(err, ErrVersionMismatch):
		httpx.PreconditionFailed(w, r)
	case errors.Is(err, ErrPreconditionRequired):
		httpx.PreconditionRequired(w, r)
	case errors.As(err, &conflict):
		_ = httpx.WriteProblem(w, r, httpx.Problem{Status: http.StatusConflict, Code: conflict.Code, Detail: conflict.Detail})
	case errors.Is(err, ErrNotAvailable):
		_ = httpx.WriteProblem(w, r, httpx.Problem{Status: http.StatusForbidden, Code: "NOT_AVAILABLE",
			Detail: "Bu etkinlik henüz açılmadı ya da kapandı."})
	case errors.As(err, &verr):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: verr.Field, Message: verr.Message}})
	default:
		return files.Failed(w, r, h.logger, err)
	}
	return true
}

func (h *Handler) writeJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	if err := httpx.WriteJSON(w, status, v); err != nil {
		h.logger.Error("yanıt yazılamadı", "err", err, "request_id", httpx.RequestIDFrom(r.Context()))
	}
}

type timelineItemResponse struct {
	Kind         string         `json:"kind"`
	ActivityID   string         `json:"activity_id"`
	SectionID    string         `json:"section_id"`
	SectionCode  string         `json:"section_code"`
	Course       courseResponse `json:"course"`
	Title        string         `json:"title"`
	DueAt        *time.Time     `json:"due_at"`
	CutoffAt     *time.Time     `json:"cutoff_at"`
	NeedsGrading int            `json:"needs_grading"`
}

func (h *Handler) timeline(w http.ResponseWriter, r *http.Request) {
	termID, ok := h.term(w, r)
	if !ok {
		return
	}
	res := httpx.ListResponse[timelineItemResponse]{Items: []timelineItemResponse{}}
	if termID != "" {
		items, err := h.svc.Timeline(r.Context(), userID(r), termID)
		if h.failed(w, r, err) {
			return
		}
		for _, it := range items {
			res.Items = append(res.Items, timelineItemResponse{Kind: it.Kind, ActivityID: it.ActivityID, SectionID: it.SectionID,
				SectionCode: it.SectionCode, Course: courseResponse{ID: it.CourseID, Code: it.CourseCode, NameTR: it.CourseNameTR, NameEN: it.CourseNameEN},
				Title: it.Title, DueAt: it.DueAt, CutoffAt: it.CutoffAt, NeedsGrading: it.NeedsGrading})
		}
	}
	h.writeJSON(w, r, http.StatusOK, res)
}
