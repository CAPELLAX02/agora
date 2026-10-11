package registration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/CAPELLAX02/agora/backend/internal/academic"
	"github.com/CAPELLAX02/agora/backend/internal/enrollment"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
	"github.com/CAPELLAX02/agora/backend/internal/registration/rules"
)

// registerRegistration, ders seçme route'larını kaydeder.
func (h *Handler) registerRegistration(rt *authz.Router) {
	const base = "/api/v1/student-programs/{id}/registration"
	rt.HandleFunc("GET "+base, authz.Authenticated, h.getRegistration)
	rt.HandleFunc("GET "+base+"/offer", authz.Authenticated, h.offer)
	rt.HandleFunc("GET "+base+"/events", authz.Authenticated, h.events)
	rt.HandleFunc("POST "+base+"/items", authz.Permission("registration:manage_own"), h.addItem)
	rt.HandleFunc("PUT "+base+"/items/{itemId}", authz.Permission("registration:manage_own"), h.changeSection)
	rt.HandleFunc("DELETE "+base+"/items/{itemId}", authz.Permission("registration:manage_own"), h.removeItem)
	rt.HandleFunc("POST "+base+"/submit", authz.Permission("registration:manage_own"), h.submit)
	rt.HandleFunc("POST "+base+"/withdraw", authz.Permission("registration:manage_own"), h.withdraw)
	rt.HandleFunc("POST "+base+"/extension", authz.Permission("registration:override"), h.extension)

	// Danışman (ilişki) ya da öğrenci işleri (registration:override kapsamı): karar politikası handler'da.
	rt.HandleFunc("GET /api/v1/registrations/{id}", authz.Authenticated, h.getByID)
	rt.HandleFunc("POST /api/v1/registrations/{id}/approve", authz.Authenticated, h.decide(Approve))
	rt.HandleFunc("POST /api/v1/registrations/{id}/reject", authz.Authenticated, h.decide(Reject))
	rt.HandleFunc("POST /api/v1/registrations/{id}/revoke", authz.Authenticated, h.decide(Revoke))
	rt.HandleFunc("POST /api/v1/registrations/bulk-approve", authz.Permission("registration:approve"), h.bulkApprove)
	rt.HandleFunc("GET /api/v1/me/advisee-registrations", authz.Authenticated, h.adviseeRegistrations)
	rt.HandleFunc("GET /api/v1/registrations", authz.Permission("registration:override"), h.listRegistrations)
}

// --- Aktör -----------------------------------------------------------------------------

// actor, isteği yapanın program kaydıyla ilişkisidir.
type actor struct {
	userID   string
	owner    bool // öğrencinin kendisi
	advisor  bool // öğrencinin şu anki danışmanı (ve onay yetkisi var)
	override bool // öğrenci işleri: programı kapsayan registration:override
}

func (h *Handler) actorFor(r *http.Request, sp enrollment.StudentProgram) (actor, error) {
	principal, _ := authn.PrincipalFrom(r.Context())
	perms, _ := authz.PermissionsFrom(r.Context())
	a := actor{userID: principal.UserID, override: perms.Allows("registration:override", sp.Program.Target())}
	st, err := h.students.Student(r.Context(), sp.StudentID)
	if err != nil {
		return actor{}, err
	}
	a.owner = st.UserID != "" && st.UserID == principal.UserID
	if sp.Advisor != nil && perms.Has("registration:approve") {
		staffID, err := h.students.StaffByUser(r.Context(), principal.UserID)
		if err != nil && !errors.Is(err, enrollment.ErrNotFound) {
			return actor{}, err
		}
		a.advisor = staffID != "" && staffID == sp.Advisor.StaffID
	}
	return a, nil
}

// ownProgram, program kaydını okur ve isteği yapanın öğrencinin kendisi olduğunu doğrular.
func (h *Handler) ownProgram(w http.ResponseWriter, r *http.Request) (enrollment.StudentProgram, actor, bool) {
	sp, ok := h.readableProgram(w, r)
	if !ok {
		return sp, actor{}, false
	}
	a, err := h.actorFor(r, sp)
	if err != nil {
		h.serverError(w, r, err)
		return sp, actor{}, false
	}
	if !a.owner {
		h.problem(w, r, http.StatusForbidden, "FORBIDDEN", "Ders seçimini sadece öğrencinin kendisi yapabilir.")
		return sp, actor{}, false
	}
	return sp, a, true
}

// termParam, istekteki dönemi (yoksa içinde bulunulan dönemi) döndürür.
func (h *Handler) termParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	if id := r.URL.Query().Get("term_id"); id != "" {
		if !httpx.ValidUUID(id) {
			httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "term_id", Message: "geçerli bir UUID olmalı"}})
			return "", false
		}
		if _, err := h.repo.TermCode(r.Context(), id); err != nil {
			h.writeError(w, r, err)
			return "", false
		}
		return id, true
	}
	id, _, err := h.repo.CurrentTerm(r.Context())
	if err != nil {
		h.writeError(w, r, err)
		return "", false
	}
	return id, true
}

// --- Yanıt tipleri ---------------------------------------------------------------------

type refResponse struct {
	ID   string `json:"id"`
	Code string `json:"code"`
}

type courseResponse struct {
	ID     string  `json:"id"`
	Code   string  `json:"code"`
	NameTR string  `json:"name_tr"`
	NameEN string  `json:"name_en"`
	ECTS   float64 `json:"ects"`
	Credit float64 `json:"credit"`
}

type slotResponse struct {
	Day       int     `json:"day"`
	Start     string  `json:"start"`
	End       string  `json:"end"`
	Kind      string  `json:"kind"`
	Classroom *string `json:"classroom"`
}

type sectionResponse struct {
	ID          string         `json:"id"`
	OfferingID  string         `json:"offering_id"`
	Course      courseResponse `json:"course"`
	SectionCode string         `json:"section_code"`
	Label       string         `json:"label"`
	Capacity    int            `json:"capacity"`
	Enrolled    int            `json:"enrolled"`
	Available   bool           `json:"available"`
	QuotaMode   string         `json:"quota_mode"`
	QuotaLeft   *int           `json:"quota_left"`
	Language    string         `json:"language"`
	Instructors []string       `json:"instructors"`
	Slots       []slotResponse `json:"slots"`
}

func toSectionResponse(s Section) sectionResponse {
	res := sectionResponse{
		ID: s.ID, OfferingID: s.OfferingID,
		Course:      courseResponse{ID: s.CourseID, Code: s.CourseCode, NameTR: s.NameTR, NameEN: s.NameEN, ECTS: s.ECTS, Credit: s.Credit},
		SectionCode: s.Code, Label: s.Label(), Capacity: s.Capacity, Enrolled: s.Enrolled, Available: s.Available(),
		QuotaMode: s.QuotaMode, QuotaLeft: s.QuotaLeft, Language: s.Language,
		Instructors: s.Instructors, Slots: make([]slotResponse, 0, len(s.Slots)),
	}
	if res.Instructors == nil {
		res.Instructors = []string{}
	}
	for _, sl := range s.Slots {
		res.Slots = append(res.Slots, slotResponse{Day: sl.Day, Start: sl.Start, End: sl.End, Kind: sl.Kind, Classroom: optional(sl.Classroom)})
	}
	return res
}

type itemResponse struct {
	ID               string          `json:"id"`
	Kind             string          `json:"kind"`
	CurriculumItemID *string         `json:"curriculum_item_id"`
	ECTS             float64         `json:"ects"`
	Source           string          `json:"source"`
	AddedAt          time.Time       `json:"added_at"`
	Section          sectionResponse `json:"section"`
}

type violationResponse struct {
	Code      string         `json:"code"`
	Severity  string         `json:"severity"`
	CourseIDs []string       `json:"course_ids"`
	Params    map[string]any `json:"params"`
	Message   string         `json:"message"`
}

func toViolations(vs []rules.Violation) []violationResponse {
	out := make([]violationResponse, 0, len(vs))
	for _, v := range vs {
		item := violationResponse{Code: string(v.Code), Severity: string(v.Severity), CourseIDs: v.CourseIDs, Params: v.Params, Message: v.Message}
		if item.CourseIDs == nil {
			item.CourseIDs = []string{}
		}
		if item.Params == nil {
			item.Params = map[string]any{}
		}
		out = append(out, item)
	}
	return out
}

type windowResponse struct {
	Open         bool       `json:"open"`
	Extended     bool       `json:"extended"`
	StartsAt     *time.Time `json:"starts_at"`
	EndsAt       *time.Time `json:"ends_at"`
	NextStartsAt *time.Time `json:"next_starts_at"`
}

func toWindowResponse(w Window) windowResponse {
	res := windowResponse{Open: w.Open, Extended: w.Extended}
	if w.Current != nil {
		res.StartsAt, res.EndsAt = &w.Current.StartsAt, &w.Current.EndsAt
	}
	if w.Next != nil {
		res.NextStartsAt = &w.Next.StartsAt
	}
	return res
}

type evaluationResponse struct {
	Blocking   bool                `json:"blocking"`
	Violations []violationResponse `json:"violations"`
}

type actionsResponse struct {
	CanEdit     bool `json:"can_edit"`
	CanSubmit   bool `json:"can_submit"`
	CanWithdraw bool `json:"can_withdraw"`
	CanApprove  bool `json:"can_approve"`
	CanReject   bool `json:"can_reject"`
	CanRevoke   bool `json:"can_revoke"`
	CanExtend   bool `json:"can_extend"`
}

type studentSummaryResponse struct {
	ID          string   `json:"id"`
	StudentNo   string   `json:"student_no"`
	FirstName   string   `json:"first_name"`
	LastName    string   `json:"last_name"`
	ProgramCode string   `json:"program_code"`
	Semester    int      `json:"semester"`
	ClassLevel  int      `json:"class_level"`
	GPA         *float64 `json:"gpa"`
	Advisor     *string  `json:"advisor"`
}

type registrationResponse struct {
	ID                 *string                `json:"id"`
	StudentProgramID   string                 `json:"student_program_id"`
	Student            studentSummaryResponse `json:"student"`
	Term               refResponse            `json:"term"`
	Status             string                 `json:"status"`
	Items              []itemResponse         `json:"items"`
	TotalECTS          float64                `json:"total_ects"`
	ECTSLimit          float64                `json:"ects_limit"`
	MaxECTSAllowed     *float64               `json:"max_ects_allowed"`
	GPASnapshot        *float64               `json:"gpa_snapshot"`
	SubmittedAt        *time.Time             `json:"submitted_at"`
	DecidedAt          *time.Time             `json:"decided_at"`
	DecidedBy          *string                `json:"decided_by"`
	DecisionNote       *string                `json:"decision_note"`
	ExtensionUntil     *time.Time             `json:"extension_until"`
	ExtensionReason    *string                `json:"extension_reason"`
	Evaluation         evaluationResponse     `json:"evaluation"`
	RegistrationWindow windowResponse         `json:"registration_window"`
	ApprovalWindow     windowResponse         `json:"approval_window"`
	Actions            actionsResponse        `json:"actions"`
	Warnings           []violationResponse    `json:"warnings"` // son işlemin uyarıları
	Version            int                    `json:"version"`
}

func toRegistrationResponse(v View, sp enrollment.StudentProgram, a actor) registrationResponse {
	res := registrationResponse{
		ID: optional(v.ID), StudentProgramID: sp.ID,
		Student: studentSummaryResponse{
			ID: sp.StudentID, StudentNo: sp.StudentNo, FirstName: sp.FirstName, LastName: sp.LastName,
			ProgramCode: sp.Program.Code, Semester: sp.CurrentSemester, ClassLevel: sp.ClassLevel, GPA: sp.GPA,
		},
		Term: refResponse{ID: v.TermID, Code: v.TermCode}, Status: string(v.Status),
		Items: make([]itemResponse, 0, len(v.Items)), TotalECTS: v.Evaluation.TotalECTS, ECTSLimit: v.Evaluation.ECTSLimit,
		MaxECTSAllowed: v.MaxECTSAllowed, GPASnapshot: v.GPASnapshot, SubmittedAt: v.SubmittedAt, DecidedAt: v.DecidedAt,
		DecidedBy: optional(v.DecidedBy), DecisionNote: optional(v.DecisionNote),
		ExtensionUntil: v.ExtensionUntil, ExtensionReason: optional(v.ExtensionReason),
		Evaluation:         evaluationResponse{Blocking: v.Evaluation.Blocking(), Violations: toViolations(v.Evaluation.Violations)},
		RegistrationWindow: toWindowResponse(v.RegWindow), ApprovalWindow: toWindowResponse(v.ApprovalWindow),
		Warnings: []violationResponse{}, Version: v.Version,
	}
	if sp.Advisor != nil {
		name := strings.TrimSpace(sp.Advisor.Title + " " + sp.Advisor.FirstName + " " + sp.Advisor.LastName)
		res.Student.Advisor = &name
	}
	for _, it := range v.Items {
		res.Items = append(res.Items, itemResponse{
			ID: it.ID, Kind: string(it.Kind), CurriculumItemID: optional(it.CurriculumItemID), ECTS: it.ECTS,
			Source: it.Source, AddedAt: it.AddedAt, Section: toSectionResponse(it.Section),
		})
	}
	decider := (a.advisor && v.ApprovalWindow.Usable()) || a.override
	res.Actions = actionsResponse{
		CanEdit:     a.owner && v.Status.Editable() && v.RegWindow.Usable(),
		CanSubmit:   a.owner && (v.Status == StatusDraft || v.Status == StatusRejected) && v.RegWindow.Usable() && !v.Evaluation.Blocking(),
		CanWithdraw: a.owner && v.Status == StatusSubmitted && v.RegWindow.Usable(),
		CanApprove:  decider && v.Status == StatusSubmitted && !v.Evaluation.Blocking(),
		CanReject:   decider && v.Status == StatusSubmitted,
		CanRevoke:   decider && v.Status == StatusApproved,
		CanExtend:   a.override,
	}
	return res
}

// --- Öğrenci uçları ----------------------------------------------------------------------

func (h *Handler) getRegistration(w http.ResponseWriter, r *http.Request) {
	sp, ok := h.readableProgram(w, r)
	if !ok {
		return
	}
	termID, ok := h.termParam(w, r)
	if !ok {
		return
	}
	h.respond(w, r, sp, termID, http.StatusOK, nil)
}

// respond, kaydın güncel halini isteği yapanın işlem izinleriyle yazar.
func (h *Handler) respond(w http.ResponseWriter, r *http.Request, sp enrollment.StudentProgram, termID string, status int, warnings []rules.Violation) {
	a, err := h.actorFor(r, sp)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	v, err := h.service.Evaluate(r.Context(), sp, termID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	res := toRegistrationResponse(v, sp, a)
	res.Warnings = toViolations(warnings)
	if v.ID != "" {
		w.Header().Set("ETag", httpx.ETag(v.Version))
	}
	h.writeJSON(w, r, status, res)
}

type sectionRequest struct {
	SectionID string `json:"section_id"`
}

func (h *Handler) addItem(w http.ResponseWriter, r *http.Request) {
	sp, a, ok := h.ownProgram(w, r)
	if !ok {
		return
	}
	var req sectionRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	if !httpx.ValidUUID(req.SectionID) {
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "section_id", Message: "Şube zorunlu."}})
		return
	}
	warnings, err := h.service.AddItem(r.Context(), a.userID, sp, req.SectionID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	sec, err := h.repo.SectionForRegistration(r.Context(), req.SectionID, sp.Program.ID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	h.respond(w, r, sp, sec.TermID, http.StatusCreated, warnings)
}

func (h *Handler) changeSection(w http.ResponseWriter, r *http.Request) {
	sp, a, ok := h.ownProgram(w, r)
	if !ok {
		return
	}
	itemID := r.PathValue("itemId")
	if !httpx.ValidUUID(itemID) {
		httpx.NotFound(w, r)
		return
	}
	var req sectionRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	if !httpx.ValidUUID(req.SectionID) {
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "section_id", Message: "Şube zorunlu."}})
		return
	}
	warnings, err := h.service.ChangeSection(r.Context(), a.userID, sp, itemID, req.SectionID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	sec, err := h.repo.SectionForRegistration(r.Context(), req.SectionID, sp.Program.ID)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	h.respond(w, r, sp, sec.TermID, http.StatusOK, warnings)
}

func (h *Handler) removeItem(w http.ResponseWriter, r *http.Request) {
	sp, a, ok := h.ownProgram(w, r)
	if !ok {
		return
	}
	itemID := r.PathValue("itemId")
	if !httpx.ValidUUID(itemID) {
		httpx.NotFound(w, r)
		return
	}
	var termID string
	if err := h.pool.QueryRow(r.Context(), `
		SELECT r.term_id FROM enrollment.registration_items i JOIN enrollment.registrations r ON r.id = i.registration_id
		WHERE i.id = $1 AND r.student_program_id = $2`, itemID, sp.ID).Scan(&termID); err != nil {
		httpx.NotFound(w, r)
		return
	}
	if err := h.service.RemoveItem(r.Context(), a.userID, sp, itemID); err != nil {
		h.writeError(w, r, err)
		return
	}
	h.respond(w, r, sp, termID, http.StatusOK, nil)
}

func (h *Handler) submit(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, h.service.Submit)
}

func (h *Handler) withdraw(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, h.service.Withdraw)
}

func (h *Handler) transition(w http.ResponseWriter, r *http.Request,
	fn func(ctx context.Context, actorID string, sp enrollment.StudentProgram, termID string, version int) error) {
	sp, a, ok := h.ownProgram(w, r)
	if !ok {
		return
	}
	termID, ok := h.termParam(w, r)
	if !ok {
		return
	}
	version, ok := httpx.IfMatchVersion(r)
	if !ok {
		httpx.PreconditionRequired(w, r)
		return
	}
	if err := fn(r.Context(), a.userID, sp, termID, version); err != nil {
		h.writeError(w, r, err)
		return
	}
	h.respond(w, r, sp, termID, http.StatusOK, nil)
}

type extensionRequest struct {
	TermID string `json:"term_id"`
	Until  string `json:"until"`
	Reason string `json:"reason"`
}

func (h *Handler) extension(w http.ResponseWriter, r *http.Request) {
	sp, ok := h.readableProgram(w, r)
	if !ok {
		return
	}
	perms, _ := authz.PermissionsFrom(r.Context())
	if !perms.Allows("registration:override", sp.Program.Target()) {
		h.problem(w, r, http.StatusForbidden, "FORBIDDEN", "Bu birim için yetkiniz yok.")
		return
	}
	var req extensionRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	var errs []httpx.FieldError
	until, err := time.Parse(time.RFC3339, req.Until)
	if err != nil {
		errs = append(errs, httpx.FieldError{Field: "until", Message: "RFC 3339 biçiminde bir zaman olmalı."})
	}
	reason := strings.TrimSpace(req.Reason)
	if utf8.RuneCountInString(reason) < 5 || utf8.RuneCountInString(reason) > 500 {
		errs = append(errs, httpx.FieldError{Field: "reason", Message: "Gerekçe 5-500 karakter olmalı."})
	}
	termID := req.TermID
	if termID != "" && !httpx.ValidUUID(termID) {
		errs = append(errs, httpx.FieldError{Field: "term_id", Message: "geçerli bir UUID olmalı"})
	}
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	if termID == "" {
		if termID, _, err = h.repo.CurrentTerm(r.Context()); err != nil {
			h.writeError(w, r, err)
			return
		}
	}
	principal, _ := authn.PrincipalFrom(r.Context())
	if err := h.service.GrantExtension(r.Context(), principal.UserID, sp, termID, until, reason); err != nil {
		h.writeError(w, r, err)
		return
	}
	h.respond(w, r, sp, termID, http.StatusOK, nil)
}

type eventResponse struct {
	ID         int64          `json:"id"`
	Type       string         `json:"type"`
	Actor      *string        `json:"actor"`
	Payload    map[string]any `json:"payload"`
	OccurredAt time.Time      `json:"occurred_at"`
}

func (h *Handler) events(w http.ResponseWriter, r *http.Request) {
	sp, ok := h.readableProgram(w, r)
	if !ok {
		return
	}
	termID, ok := h.termParam(w, r)
	if !ok {
		return
	}
	res := httpx.ListResponse[eventResponse]{Items: []eventResponse{}}
	reg, err := h.repo.Registration(r.Context(), sp.ID, sp.Program.ID, termID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if reg.ID != "" {
		list, err := h.repo.Events(r.Context(), reg.ID)
		if err != nil {
			h.serverError(w, r, err)
			return
		}
		for _, e := range list {
			res.Items = append(res.Items, eventResponse{ID: e.ID, Type: e.Type, Actor: optional(e.Actor), Payload: e.Payload, OccurredAt: e.OccurredAt})
		}
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

// --- Alınabilir dersler ----------------------------------------------------------------

type offerLineResponse struct {
	ItemID       string `json:"item_id"`
	SemesterNo   int    `json:"semester_no"`
	ItemType     string `json:"item_type"`
	Code         string `json:"code"`
	NameTR       string `json:"name_tr"`
	NameEN       string `json:"name_en"`
	IsCompulsory bool   `json:"is_compulsory"`
}

type offerResponse struct {
	Course       courseResponse     `json:"course"`
	Category     string             `json:"category"`
	Kind         string             `json:"kind"`
	Line         offerLineResponse  `json:"line"`
	Status       string             `json:"status"`
	LastLetter   *string            `json:"last_letter"`
	Prerequisite *violationResponse `json:"prerequisite"`
	Sections     []sectionResponse  `json:"sections"`
	BasketItemID *string            `json:"basket_item_id"`
}

type offerListResponse struct {
	Term      refResponse     `json:"term"`
	Semester  int             `json:"semester"`
	ECTSLimit float64         `json:"ects_limit"`
	TotalECTS float64         `json:"total_ects"`
	Items     []offerResponse `json:"items"`
}

func (h *Handler) offer(w http.ResponseWriter, r *http.Request) {
	sp, ok := h.readableProgram(w, r)
	if !ok {
		return
	}
	termID, ok := h.termParam(w, r)
	if !ok {
		return
	}
	code, err := h.repo.TermCode(r.Context(), termID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	offers, ev, err := h.service.Offers(r.Context(), sp, termID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	res := offerListResponse{
		Term: refResponse{ID: termID, Code: code}, Semester: sp.CurrentSemester,
		ECTSLimit: ev.ECTSLimit, TotalECTS: ev.TotalECTS, Items: make([]offerResponse, 0, len(offers)),
	}
	for _, o := range offers {
		first := o.Sections[0]
		item := offerResponse{
			Course:   courseResponse{ID: o.CourseID, Code: o.Code, NameTR: o.NameTR, NameEN: o.NameEN, ECTS: o.ECTS, Credit: first.Credit},
			Category: string(o.Category), Kind: string(o.Kind), Status: string(o.Status), LastLetter: optional(o.LastLetter),
			Line: offerLineResponse{
				ItemID: o.Line.ID, SemesterNo: o.Line.Semester, ItemType: "COURSE", Code: o.Line.Code,
				NameTR: o.Line.NameTR, NameEN: o.Line.NameEN, IsCompulsory: o.Line.Compulsory,
			},
			Sections: make([]sectionResponse, 0, len(o.Sections)), BasketItemID: optional(o.BasketItemID),
		}
		if o.Line.IsSlot() {
			item.Line.ItemType = "ELECTIVE_SLOT"
		}
		if o.Prerequisite != nil {
			v := toViolations([]rules.Violation{*o.Prerequisite})[0]
			item.Prerequisite = &v
		}
		for _, s := range o.Sections {
			item.Sections = append(item.Sections, toSectionResponse(s))
		}
		res.Items = append(res.Items, item)
	}
	h.writeJSON(w, r, http.StatusOK, res)
}

// --- Hata eşleme -------------------------------------------------------------------------

type ruleProblem struct {
	Type       string              `json:"type"`
	Title      string              `json:"title"`
	Status     int                 `json:"status"`
	Detail     string              `json:"detail"`
	Instance   string              `json:"instance"`
	Code       string              `json:"code"`
	RequestID  string              `json:"request_id,omitempty"`
	Violations []violationResponse `json:"violations"`
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	var re *RuleError
	switch {
	case errors.As(err, &re):
		var errorsOnly []rules.Violation
		for _, v := range re.Violations {
			if v.Severity == rules.Error {
				errorsOnly = append(errorsOnly, v)
			}
		}
		detail := "Ders seçme kuralları bu işleme izin vermiyor."
		if len(errorsOnly) > 0 {
			detail = errorsOnly[0].Message
		}
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(ruleProblem{
			Type: "about:blank", Title: http.StatusText(http.StatusUnprocessableEntity), Status: http.StatusUnprocessableEntity,
			Detail: detail, Instance: r.URL.Path, Code: "RULE_VIOLATION", RequestID: httpx.RequestIDFrom(r.Context()),
			Violations: toViolations(re.Violations),
		})
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrItemNotFound):
		httpx.NotFound(w, r)
	case errors.Is(err, ErrSectionNotFound):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "section_id", Message: "Şube bulunamadı."}})
	case errors.Is(err, ErrCourseMismatch):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "section_id", Message: "Aynı dersin başka bir şubesi seçilmeli."}})
	case errors.Is(err, ErrReasonRequired):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "note", Message: "Gerekçe zorunlu."}})
	case errors.Is(err, ErrExtensionInvalid):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "until", Message: "Ek süre şu andan sonra ve en fazla 30 gün sonra bitmeli."}})
	case errors.Is(err, ErrVersionMismatch):
		httpx.PreconditionFailed(w, r)
	case errors.Is(err, ErrSectionFull):
		h.problem(w, r, http.StatusConflict, "SECTION_FULL", "Şubenin kontenjanı dolu.")
	case errors.Is(err, ErrQuotaFull):
		h.problem(w, r, http.StatusConflict, "QUOTA_FULL", "Bu şubede programınıza ayrılan kontenjan dolu.")
	case errors.Is(err, ErrNotForProgram):
		h.problem(w, r, http.StatusConflict, "SECTION_NOT_FOR_PROGRAM", "Bu şube programınıza kontenjan ayırmamış.")
	case errors.Is(err, ErrSectionNotOpen):
		h.problem(w, r, http.StatusConflict, "SECTION_NOT_OPEN", "Şube bu dönem ders seçmeye açık değil.")
	case errors.Is(err, ErrWindowClosed):
		h.problem(w, r, http.StatusConflict, "WINDOW_CLOSED", "Bu işlemin takvim penceresi kapalı.")
	case errors.Is(err, ErrLocked):
		h.problem(w, r, http.StatusConflict, "REGISTRATION_LOCKED",
			"Kaydınız danışman onayında ya da onaylanmış; değiştirmek için önce gönderimi geri çekin.")
	case errors.Is(err, ErrInvalidState):
		h.problem(w, r, http.StatusConflict, "INVALID_REGISTRATION_STATE", "Kayıt bu işlem için uygun durumda değil.")
	case errors.Is(err, ErrSameSection):
		h.problem(w, r, http.StatusConflict, "SAME_SECTION", "Ders zaten bu şubede.")
	case errors.Is(err, ErrNoTerm):
		h.problem(w, r, http.StatusConflict, "NO_ACTIVE_TERM", "Tanımlı bir aktif dönem yok.")
	case errors.Is(err, academic.ErrNotFound):
		httpx.NotFound(w, r)
	default:
		h.serverError(w, r, err)
	}
}

func (h *Handler) problem(w http.ResponseWriter, r *http.Request, status int, code, detail string) {
	_ = httpx.WriteProblem(w, r, httpx.Problem{Status: status, Code: code, Detail: detail})
}
