package registration

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/enrollment"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

// registerAddDrop, ekle-bırak route'larını kaydeder.
func (h *Handler) registerAddDrop(rt *authz.Router) {
	m := h.mutations
	const base = "/api/v1/student-programs/{id}/registration/add-drop"
	rt.HandleFunc("GET "+base, authz.Authenticated, h.listAddDrop)
	rt.HandleFunc("POST "+base, authz.Permission("registration:manage_own"), h.requestAddDrop, m...)
	rt.HandleFunc("POST /api/v1/add-drop-requests/{id}/withdraw", authz.Permission("registration:manage_own"), h.withdrawAddDrop, m...)
	// Karar: danışman aşamasında öğrencinin danışmanı, bölüm başkanı aşamasında
	// registration:approve_add_drop kapsamı ya da öğrenci işleri (override). Politika handler'da.
	rt.HandleFunc("POST /api/v1/add-drop-requests/{id}/approve", authz.Authenticated, h.decideAddDrop(true), m...)
	rt.HandleFunc("POST /api/v1/add-drop-requests/{id}/reject", authz.Authenticated, h.decideAddDrop(false), m...)
	rt.HandleFunc("GET /api/v1/me/add-drop-queue", authz.Authenticated, h.addDropQueue)
}

// --- Yanıt ---------------------------------------------------------------------------------

type addDropItemResponse struct {
	ID      string          `json:"id"`
	Action  string          `json:"action"`
	ECTS    float64         `json:"ects"`
	Section sectionResponse `json:"section"`
}

type addDropDecisionResponse struct {
	By   *string    `json:"by"`
	At   *time.Time `json:"at"`
	Note *string    `json:"note"`
}

type addDropActionsResponse struct {
	CanWithdraw bool `json:"can_withdraw"`
	CanApprove  bool `json:"can_approve"`
	CanReject   bool `json:"can_reject"`
}

type addDropStudentResponse struct {
	ID          string `json:"id"`
	StudentNo   string `json:"student_no"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	ProgramCode string `json:"program_code"`
}

type addDropResponse struct {
	ID               string                   `json:"id"`
	RegistrationID   string                   `json:"registration_id"`
	StudentProgramID string                   `json:"student_program_id"`
	Student          addDropStudentResponse   `json:"student"`
	TermID           string                   `json:"term_id"`
	Status           string                   `json:"status"`
	Stage            *string                  `json:"stage"`
	StudentNote      *string                  `json:"student_note"`
	Advisor          *addDropDecisionResponse `json:"advisor"`
	DeptHead         *addDropDecisionResponse `json:"dept_head"`
	Items            []addDropItemResponse    `json:"items"`
	ECTSDelta        float64                  `json:"ects_delta"`
	Actions          addDropActionsResponse   `json:"actions"`
	Version          int                      `json:"version"`
	CreatedAt        time.Time                `json:"created_at"`
}

func decision(by string, at *time.Time, note string) *addDropDecisionResponse {
	if at == nil {
		return nil
	}
	return &addDropDecisionResponse{By: optional(by), At: at, Note: optional(note)}
}

// addDropActor, isteği yapanın talep üzerindeki yetkileridir.
type addDropActor struct {
	actor
	head bool // programı kapsayan registration:approve_add_drop
}

func (h *Handler) addDropActorFor(r *http.Request, sp enrollment.StudentProgram) (addDropActor, error) {
	a, err := h.actorFor(r, sp)
	if err != nil {
		return addDropActor{}, err
	}
	perms, _ := authz.PermissionsFrom(r.Context())
	return addDropActor{actor: a, head: perms.Allows("registration:approve_add_drop", sp.Program.Target())}, nil
}

// decider, talebin aşamasında kararı kimin hangi sıfatla vereceğini söyler.
func (h *Handler) addDropDecider(ctx context.Context, sp enrollment.StudentProgram, a addDropActor, req AddDropRequest) (Decider, bool, error) {
	if !req.Status.Open() {
		return Decider{}, false, nil
	}
	role := a.advisor
	window := WindowAddDropAdvisor
	if req.Status == AddDropPendingHead {
		role, window = a.head, WindowAddDropHead
	}
	if role {
		w, err := h.service.calendarWindow(ctx, sp, req.TermID, window)
		if err != nil {
			return Decider{}, false, err
		}
		if w.Open || !a.override {
			return Decider{UserID: a.userID}, true, nil
		}
	}
	if a.override {
		return Decider{UserID: a.userID, Override: true}, true, nil
	}
	return Decider{}, false, nil
}

func (h *Handler) toAddDropResponse(ctx context.Context, sp enrollment.StudentProgram, a addDropActor, req AddDropRequest) (addDropResponse, error) {
	res := addDropResponse{
		ID: req.ID, RegistrationID: req.RegistrationID, StudentProgramID: sp.ID,
		Student: addDropStudentResponse{
			ID: sp.StudentID, StudentNo: sp.StudentNo, FirstName: sp.FirstName, LastName: sp.LastName, ProgramCode: sp.Program.Code,
		},
		TermID: req.TermID, Status: string(req.Status), StudentNote: optional(req.StudentNote),
		Advisor:  decision(req.AdvisorBy, req.AdvisorAt, req.AdvisorNote),
		DeptHead: decision(req.HeadBy, req.HeadAt, req.HeadNote),
		Items:    make([]addDropItemResponse, 0, len(req.Items)), Version: req.Version, CreatedAt: req.CreatedAt,
	}
	if req.Status.Open() {
		stage := AddDropStage(req.Status)
		res.Stage = &stage
	}
	for _, it := range req.Items {
		res.Items = append(res.Items, addDropItemResponse{ID: it.ID, Action: it.Action, ECTS: it.ECTS, Section: toSectionResponse(it.Section)})
		if it.Action == "ADD" {
			res.ECTSDelta += it.ECTS
		} else {
			res.ECTSDelta -= it.ECTS
		}
	}
	res.ECTSDelta = round1(res.ECTSDelta)
	_, ok, err := h.addDropDecider(ctx, sp, a, req)
	if err != nil {
		return res, err
	}
	res.Actions = addDropActionsResponse{
		CanWithdraw: a.owner && req.Status.Open(),
		CanApprove:  ok, CanReject: ok,
	}
	return res, nil
}

// --- Uçlar ----------------------------------------------------------------------------------

func (h *Handler) listAddDrop(w http.ResponseWriter, r *http.Request) {
	sp, ok := h.readableProgram(w, r)
	if !ok {
		return
	}
	termID, ok := h.termParam(w, r)
	if !ok {
		return
	}
	res := httpx.ListResponse[addDropResponse]{Items: []addDropResponse{}}
	reg, err := h.repo.Registration(r.Context(), sp.ID, sp.Program.ID, termID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if reg.ID != "" {
		list, err := h.repo.AddDropRequests(r.Context(), reg.ID, sp.Program.ID)
		if err != nil {
			h.serverError(w, r, err)
			return
		}
		a, err := h.addDropActorFor(r, sp)
		if err != nil {
			h.serverError(w, r, err)
			return
		}
		for _, req := range list {
			item, err := h.toAddDropResponse(r.Context(), sp, a, req)
			if err != nil {
				h.serverError(w, r, err)
				return
			}
			res.Items = append(res.Items, item)
		}
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

type addDropRequestBody struct {
	TermID string   `json:"term_id"`
	Adds   []string `json:"adds"`
	Drops  []string `json:"drops"`
	Note   string   `json:"note"`
}

func (h *Handler) requestAddDrop(w http.ResponseWriter, r *http.Request) {
	sp, a, ok := h.ownProgram(w, r)
	if !ok {
		return
	}
	var req addDropRequestBody
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	var errs []httpx.FieldError
	for field, ids := range map[string][]string{"adds": req.Adds, "drops": req.Drops} {
		if len(ids) > 10 {
			errs = append(errs, httpx.FieldError{Field: field, Message: "En fazla 10 ders."})
		}
		for _, id := range ids {
			if !httpx.ValidUUID(id) {
				errs = append(errs, httpx.FieldError{Field: field, Message: "Geçerli kimlikler olmalı."})
				break
			}
		}
	}
	if len(req.Adds)+len(req.Drops) == 0 {
		errs = append(errs, httpx.FieldError{Field: "adds", Message: "En az bir ekleme ya da bırakma olmalı."})
	}
	note := strings.TrimSpace(req.Note)
	if utf8.RuneCountInString(note) > 1000 {
		errs = append(errs, httpx.FieldError{Field: "note", Message: "En fazla 1000 karakter."})
	}
	if req.TermID != "" && !httpx.ValidUUID(req.TermID) {
		errs = append(errs, httpx.FieldError{Field: "term_id", Message: "geçerli bir UUID olmalı"})
	}
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	termID := req.TermID
	if termID == "" {
		var err error
		if termID, _, err = h.repo.CurrentTerm(r.Context()); err != nil {
			h.writeError(w, r, err)
			return
		}
	}
	id, err := h.service.RequestAddDrop(r.Context(), a.userID, sp, termID, AddDropInput{Adds: req.Adds, Drops: req.Drops, Note: note})
	if err != nil {
		h.writeAddDropError(w, r, err)
		return
	}
	h.respondAddDrop(w, r, sp, id, http.StatusCreated)
}

func (h *Handler) respondAddDrop(w http.ResponseWriter, r *http.Request, sp enrollment.StudentProgram, id string, status int) {
	req, err := h.repo.AddDropRequest(r.Context(), id, sp.Program.ID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	a, err := h.addDropActorFor(r, sp)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	res, err := h.toAddDropResponse(r.Context(), sp, a, req)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	w.Header().Set("ETag", httpx.ETag(req.Version))
	h.writeJSON(w, r, status, res)
}

// addDropProgram, talebin program kaydını okur ve isteği yapanın öğrenciyi görebildiğini
// doğrular. Erişim yoksa 404 yazar.
func (h *Handler) addDropProgram(w http.ResponseWriter, r *http.Request) (enrollment.StudentProgram, AddDropRequest, bool) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return enrollment.StudentProgram{}, AddDropRequest{}, false
	}
	var spID string
	err := h.pool.QueryRow(r.Context(), `
		SELECT r.student_program_id FROM enrollment.add_drop_requests a
		JOIN enrollment.registrations r ON r.id = a.registration_id WHERE a.id = $1`, id).Scan(&spID)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.NotFound(w, r)
		return enrollment.StudentProgram{}, AddDropRequest{}, false
	}
	if err != nil {
		h.serverError(w, r, err)
		return enrollment.StudentProgram{}, AddDropRequest{}, false
	}
	sp, err := h.students.StudentProgram(r.Context(), spID)
	if err != nil {
		h.serverError(w, r, err)
		return enrollment.StudentProgram{}, AddDropRequest{}, false
	}
	principal, _ := authn.PrincipalFrom(r.Context())
	perms, _ := authz.PermissionsFrom(r.Context())
	readable, err := h.studentSvc.CanReadProgram(r.Context(), perms, principal.UserID, sp)
	if err != nil {
		h.serverError(w, r, err)
		return enrollment.StudentProgram{}, AddDropRequest{}, false
	}
	// Bölüm başkanı öğrenci bilgisini okuma yetkisi olmadan da kapsamındaki talebi görür.
	if !readable && !perms.Allows("registration:approve_add_drop", sp.Program.Target()) {
		httpx.NotFound(w, r)
		return enrollment.StudentProgram{}, AddDropRequest{}, false
	}
	req, err := h.repo.AddDropRequest(r.Context(), id, sp.Program.ID)
	if err != nil {
		h.writeAddDropError(w, r, err)
		return enrollment.StudentProgram{}, AddDropRequest{}, false
	}
	return sp, req, true
}

func (h *Handler) withdrawAddDrop(w http.ResponseWriter, r *http.Request) {
	sp, req, ok := h.addDropProgram(w, r)
	if !ok {
		return
	}
	a, err := h.actorFor(r, sp)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	if !a.owner {
		h.problem(w, r, http.StatusForbidden, "FORBIDDEN", "Talebi sadece öğrencinin kendisi geri çekebilir.")
		return
	}
	version, ok := httpx.IfMatchVersion(r)
	if !ok {
		httpx.PreconditionRequired(w, r)
		return
	}
	if err := h.service.WithdrawAddDrop(r.Context(), a.userID, sp, req.ID, version); err != nil {
		h.writeAddDropError(w, r, err)
		return
	}
	h.respondAddDrop(w, r, sp, req.ID, http.StatusOK)
}

func (h *Handler) decideAddDrop(approve bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sp, req, ok := h.addDropProgram(w, r)
		if !ok {
			return
		}
		a, err := h.addDropActorFor(r, sp)
		if err != nil {
			h.serverError(w, r, err)
			return
		}
		d, allowed, err := h.addDropDecider(r.Context(), sp, a, req)
		if err != nil {
			h.serverError(w, r, err)
			return
		}
		if !allowed {
			if !req.Status.Open() {
				h.problem(w, r, http.StatusConflict, "INVALID_ADD_DROP_STATE", "Talep karar bekleyen durumda değil.")
				return
			}
			h.problem(w, r, http.StatusForbidden, "FORBIDDEN", "Bu talebin bulunduğu aşamada karar yetkiniz yok.")
			return
		}
		version, ok := httpx.IfMatchVersion(r)
		if !ok {
			httpx.PreconditionRequired(w, r)
			return
		}
		var body decisionRequest
		if err := httpx.ReadJSON(w, r, &body); err != nil {
			httpx.InvalidBody(w, r, err)
			return
		}
		note := strings.TrimSpace(body.Note)
		if utf8.RuneCountInString(note) > 1000 {
			httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "note", Message: "En fazla 1000 karakter."}})
			return
		}
		if (!approve || d.Override) && utf8.RuneCountInString(note) < 5 {
			httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "note", Message: "En az 5 karakterlik bir gerekçe yazın."}})
			return
		}
		if err := h.service.DecideAddDrop(r.Context(), d, sp, req.ID, version, approve, note); err != nil {
			h.writeAddDropError(w, r, err)
			return
		}
		h.respondAddDrop(w, r, sp, req.ID, http.StatusOK)
	}
}

func (h *Handler) writeAddDropError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrAddDropNotFound):
		httpx.NotFound(w, r)
	case errors.Is(err, ErrAddDropOpen):
		h.problem(w, r, http.StatusConflict, "ADD_DROP_OPEN", "Karar bekleyen bir ekle-bırak talebiniz var.")
	case errors.Is(err, ErrNotApproved):
		h.problem(w, r, http.StatusConflict, "REGISTRATION_NOT_APPROVED", "Ekle-bırak sadece danışmanınızın onayladığı kayıtta yapılır.")
	case errors.Is(err, ErrAddDropEmpty):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "adds", Message: "En az bir ekleme ya da bırakma olmalı."}})
	case errors.Is(err, ErrDropNotInBasket):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "drops", Message: "Bırakılacak ders kaydınızda yok."}})
	case errors.Is(err, ErrAddAlreadyTaken):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "adds", Message: "Eklenecek ders zaten kaydınızda; şube değiştirmek için önce bırakın."}})
	case errors.Is(err, ErrInvalidState):
		h.problem(w, r, http.StatusConflict, "INVALID_ADD_DROP_STATE", "Talep bu işlem için uygun durumda değil.")
	default:
		h.writeError(w, r, err)
	}
}

// --- Kuyruk ---------------------------------------------------------------------------------

// AddDropQueue, isteği yapanın karar vereceği talepleri döndürür: danışmanı olduğu
// öğrencilerin danışman aşamasındaki talepleri ve kapsamındaki programların bölüm
// başkanı aşamasındaki talepleri (en eski önce).
func (r *Repository) AddDropQueue(ctx context.Context, termID, advisorStaffID string, headScope authz.ScopeSet) ([]AddDropRequest, error) {
	var w db.Where
	w.Add("r.term_id = $1", termID)
	cond := "(a.status = 'PENDING_ADVISOR' AND r.student_program_id IN (SELECT student_program_id FROM enrollment.advisor_assignments WHERE advisor_staff_id = $1 AND valid_until IS NULL))"
	args := []any{nullable(advisorStaffID)}
	switch {
	case headScope.University:
		cond += " OR a.status = 'PENDING_DEPT_HEAD'"
	case !headScope.Empty():
		cond += ` OR (a.status = 'PENDING_DEPT_HEAD' AND r.student_program_id IN (
			SELECT sp.id FROM enrollment.student_programs sp JOIN org.programs p ON p.id = sp.program_id
			JOIN org.departments d ON d.id = p.department_id
			WHERE d.faculty_id = ANY($2::uuid[]) OR d.id = ANY($3::uuid[]) OR p.id = ANY($4::uuid[])))`
		args = append(args, headScope.FacultyIDs, headScope.DepartmentIDs, headScope.ProgramIDs)
	}
	w.Add("("+cond+")", args...)
	rows, err := r.db.Query(ctx, addDropSelect+"\n\t"+w.SQL()+"\n\tORDER BY a.created_at, a.id", w.Args()...)
	if err != nil {
		return nil, fmt.Errorf("registration: ekle-bırak kuyruğu okunamadı: %w", err)
	}
	list, err := pgx.CollectRows(rows, scanAddDrop)
	if err != nil {
		return nil, fmt.Errorf("registration: ekle-bırak kuyruğu okunamadı: %w", err)
	}
	return list, nil
}

func (h *Handler) addDropQueue(w http.ResponseWriter, r *http.Request) {
	termID, ok := h.termParam(w, r)
	if !ok {
		return
	}
	principal, _ := authn.PrincipalFrom(r.Context())
	perms, _ := authz.PermissionsFrom(r.Context())
	staffID, err := h.students.StaffByUser(r.Context(), principal.UserID)
	if err != nil && !errors.Is(err, enrollment.ErrNotFound) {
		h.serverError(w, r, err)
		return
	}
	res := httpx.ListResponse[addDropResponse]{Items: []addDropResponse{}}
	if staffID == "" && !perms.Has("registration:approve_add_drop") {
		h.writeJSON(w, r, http.StatusOK, res)
		return
	}
	list, err := h.repo.AddDropQueue(r.Context(), termID, staffID, perms.ScopesOf("registration:approve_add_drop"))
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	for _, req := range list {
		sp, err := h.students.StudentProgram(r.Context(), req.StudentProgramID)
		if err != nil {
			h.serverError(w, r, err)
			return
		}
		if req.Items, err = h.repo.addDropItems(r.Context(), req.ID, sp.Program.ID); err != nil {
			h.serverError(w, r, err)
			return
		}
		a, err := h.addDropActorFor(r, sp)
		if err != nil {
			h.serverError(w, r, err)
			return
		}
		item, err := h.toAddDropResponse(r.Context(), sp, a, req)
		if err != nil {
			h.serverError(w, r, err)
			return
		}
		res.Items = append(res.Items, item)
	}
	h.writeJSON(w, r, http.StatusOK, res)
}
