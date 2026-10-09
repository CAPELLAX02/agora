package grading

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/CAPELLAX02/agora/backend/internal/platform/authn"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

const (
	permRead          = "course:read"
	permPlan          = "assessment_plan:manage"
	permSectionManage = "section:manage"
)

// Store, değerlendirme planı işlemleridir. *Repository bunu sağlar.
type Store interface {
	AssessmentTypes(ctx context.Context) ([]AssessmentType, error)
	Plan(ctx context.Context, sectionID, viewerID string) (Plan, error)
	SetPlan(ctx context.Context, actorID, sectionID string, version int, items []ComponentInput) error
	Lock(ctx context.Context, actorID, sectionID string) error
	Unlock(ctx context.Context, actorID, sectionID, reason string) error
}

// Handler, değerlendirme planı uçlarını sunar.
type Handler struct {
	store  Store
	logger *slog.Logger
}

// NewHandler, bir Handler oluşturur.
func NewHandler(store Store, logger *slog.Logger) *Handler {
	return &Handler{store: store, logger: logger}
}

// Register, route'ları kaydeder. Planı düzenleme yetkisi ilişkiye dayalıdır (şubenin
// öğretim elemanı) ya da bölüm kapsamlı şube yönetimidir; bu yüzden route sadece kimlik
// ister, kararı handler verir.
func (h *Handler) Register(rt *authz.Router) {
	rt.HandleFunc("GET /api/v1/assessment-types", authz.Permission(permRead), h.listTypes)
	rt.HandleFunc("GET /api/v1/sections/{id}/assessment-plan", authz.Permission(permRead), h.getPlan)
	rt.HandleFunc("PUT /api/v1/sections/{id}/assessment-plan", authz.Authenticated, h.setPlan)
	rt.HandleFunc("POST /api/v1/sections/{id}/assessment-plan/lock", authz.Authenticated, h.lock)
	rt.HandleFunc("POST /api/v1/sections/{id}/assessment-plan/unlock", authz.Permission(permSectionManage), h.unlock)
}

// --- Yanıt tipleri -------------------------------------------------------------

type typeResponse struct {
	Code     string `json:"code"`
	NameTR   string `json:"name_tr"`
	NameEN   string `json:"name_en"`
	Category string `json:"category"`
}

type componentResponse struct {
	ID          string       `json:"id"`
	Type        typeResponse `json:"type"`
	SequenceNo  int          `json:"sequence_no"`
	NameTR      *string      `json:"name_tr"`
	NameEN      *string      `json:"name_en"`
	LabelTR     string       `json:"label_tr"`
	LabelEN     string       `json:"label_en"`
	Weight      float64      `json:"weight"`
	ScheduledOn *string      `json:"scheduled_on"`
}

type planResponse struct {
	SectionID    string              `json:"section_id"`
	Components   []componentResponse `json:"components"`
	InTermWeight float64             `json:"in_term_weight"`
	FinalWeight  float64             `json:"final_weight"`
	IsComplete   bool                `json:"is_complete"`
	LockedAt     *time.Time          `json:"locked_at"`
	LockedBy     *string             `json:"locked_by"`
	Editable     bool                `json:"editable"`
	CanUnlock    bool                `json:"can_unlock"`
	Version      int                 `json:"version"`
}

func toTypeResponse(t AssessmentType) typeResponse {
	return typeResponse{Code: t.Code, NameTR: t.NameTR, NameEN: t.NameEN, Category: string(t.Category)}
}

func (h *Handler) toPlanResponse(r *http.Request, p Plan) planResponse {
	res := planResponse{SectionID: p.SectionID, LockedAt: p.LockedAt, Version: p.Version, Components: []componentResponse{}}
	if p.LockedByName != "" {
		res.LockedBy = &p.LockedByName
	}
	perType := map[string]int{}
	for _, c := range p.Components {
		perType[c.Type.Code]++
	}
	var inTerm, final, finals int64
	for _, c := range p.Components {
		numbered := perType[c.Type.Code] > 1
		cr := componentResponse{
			ID: c.ID, Type: toTypeResponse(c.Type), SequenceNo: c.SequenceNo, NameTR: optional(c.NameTR), NameEN: optional(c.NameEN),
			LabelTR: c.Label(false, numbered), LabelEN: c.Label(true, numbered), Weight: c.Weight,
		}
		if c.ScheduledOn != nil {
			d := c.ScheduledOn.Format(time.DateOnly)
			cr.ScheduledOn = &d
		}
		switch c.Type.Category {
		case CategoryInTerm:
			inTerm += cents(c.Weight)
		case CategoryFinal:
			final += cents(c.Weight)
			finals++
		}
		res.Components = append(res.Components, cr)
	}
	res.InTermWeight, res.FinalWeight = float64(inTerm)/100, float64(final)/100
	res.IsComplete = finals == 1 && inTerm+final == 10000
	res.CanUnlock = p.LockedAt != nil && mayManageSection(r, p.SectionContext)
	res.Editable = p.LockedAt == nil && p.SectionStatus == "ACTIVE" && mayEdit(r, p.SectionContext)
	return res
}

// cents, ağırlığı yüzde birlik tam sayıya çevirir: toplamlar kayan nokta artığı olmadan
// karşılaştırılır.
func cents(w float64) int64 {
	return int64(math.Round(w * 100))
}

// --- Yetki ---------------------------------------------------------------------

func sectionTarget(sc SectionContext) authz.Target {
	return authz.Target{FacultyID: sc.FacultyID, DepartmentID: sc.DepartmentID}
}

func mayManageSection(r *http.Request, sc SectionContext) bool {
	perms, ok := authz.PermissionsFrom(r.Context())
	return ok && perms.Allows(permSectionManage, sectionTarget(sc))
}

// mayEdit, planı düzenleyebilecekler: şubenin sorumlu ya da ortak öğretim elemanı
// (değerlendirme planı yetkisiyle) ve şubeyi yönetebilenler (bölüm başkanı). Öğretim
// elemanı rolü bölüm kapsamlı olsa da başka şubelerin planına dokunamaz.
func mayEdit(r *http.Request, sc SectionContext) bool {
	perms, ok := authz.PermissionsFrom(r.Context())
	if !ok {
		return false
	}
	if (sc.InstructorRole == "PRIMARY" || sc.InstructorRole == "CO_INSTRUCTOR") && perms.Has(permPlan) {
		return true
	}
	return perms.Allows(permSectionManage, sectionTarget(sc))
}

// --- Uçlar ---------------------------------------------------------------------

func (h *Handler) listTypes(w http.ResponseWriter, r *http.Request) {
	types, err := h.store.AssessmentTypes(r.Context())
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	h.writeJSON(w, r, http.StatusOK, httpx.ListResponse[typeResponse]{Items: mapSlice(types, toTypeResponse)})
}

func (h *Handler) loadPlan(w http.ResponseWriter, r *http.Request) (Plan, bool) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return Plan{}, false
	}
	p, err := h.store.Plan(r.Context(), id, actorID(r))
	if errors.Is(err, ErrNotFound) {
		httpx.NotFound(w, r)
		return Plan{}, false
	}
	if err != nil {
		h.serverError(w, r, err)
		return Plan{}, false
	}
	return p, true
}

func (h *Handler) writePlan(w http.ResponseWriter, r *http.Request) {
	p, ok := h.loadPlan(w, r)
	if !ok {
		return
	}
	w.Header().Set("ETag", httpx.ETag(p.Version))
	h.writeJSON(w, r, http.StatusOK, h.toPlanResponse(r, p))
}

func (h *Handler) getPlan(w http.ResponseWriter, r *http.Request) {
	h.writePlan(w, r)
}

type planRequest struct {
	Components []struct {
		Type        string  `json:"type"`
		SequenceNo  int     `json:"sequence_no"`
		NameTR      string  `json:"name_tr"`
		NameEN      string  `json:"name_en"`
		Weight      float64 `json:"weight"`
		ScheduledOn string  `json:"scheduled_on"`
	} `json:"components"`
}

// validate, planı doğrular: dönem içi bileşenlerin ve finalin ağırlıkları toplamı 100,
// tek final; bütünleme istemciden gelmez (finalden türetilir).
func (req planRequest) validate(types map[string]AssessmentType, sc SectionContext) ([]ComponentInput, []httpx.FieldError) {
	var errs []httpx.FieldError
	if len(req.Components) == 0 || len(req.Components) > 20 {
		errs = append(errs, httpx.FieldError{Field: "components", Message: "1 ile 20 arasında bileşen olmalı."})
	}
	items := make([]ComponentInput, 0, len(req.Components))
	nextSeq := map[string]int{}
	seen := map[string]bool{}
	var total int64
	finals := 0
	for _, c := range req.Components {
		t, ok := types[c.Type]
		switch {
		case !ok:
			errs = append(errs, httpx.FieldError{Field: "components.type", Message: fmt.Sprintf("%q tanımlı bir değerlendirme türü değil.", c.Type)})
			continue
		case t.Category == CategoryMakeup:
			errs = append(errs, httpx.FieldError{Field: "components.type", Message: "Bütünleme finalden türetilir; plana eklenmez."})
			continue
		case t.Category == CategoryFinal:
			finals++
		}
		in := ComponentInput{
			TypeCode: c.Type, SequenceNo: c.SequenceNo, NameTR: strings.TrimSpace(c.NameTR), NameEN: strings.TrimSpace(c.NameEN), Weight: c.Weight,
		}
		if in.SequenceNo == 0 {
			nextSeq[c.Type]++
			in.SequenceNo = nextSeq[c.Type]
		}
		if in.SequenceNo < 1 || in.SequenceNo > 20 {
			errs = append(errs, httpx.FieldError{Field: "components.sequence_no", Message: "1 ile 20 arasında olmalı."})
		}
		if seen[in.key()] {
			errs = append(errs, httpx.FieldError{Field: "components.sequence_no", Message: fmt.Sprintf("%s %d birden fazla kez verilmiş.", t.NameTR, in.SequenceNo)})
		}
		seen[in.key()] = true
		if in.Weight <= 0 || in.Weight > 100 || float64(cents(in.Weight)) != in.Weight*100 {
			errs = append(errs, httpx.FieldError{Field: "components.weight", Message: "0'dan büyük, en fazla 100 ve en fazla iki ondalık basamaklı olmalı."})
		}
		total += cents(in.Weight)
		for field, v := range map[string]string{"components.name_tr": in.NameTR, "components.name_en": in.NameEN} {
			if utf8.RuneCountInString(v) > 100 {
				errs = append(errs, httpx.FieldError{Field: field, Message: "En fazla 100 karakter."})
			}
		}
		if c.ScheduledOn != "" {
			d, err := time.Parse(time.DateOnly, c.ScheduledOn)
			switch {
			case err != nil:
				errs = append(errs, httpx.FieldError{Field: "components.scheduled_on", Message: "YYYY-AA-GG biçiminde bir tarih olmalı."})
			case d.Before(sc.TermStartsOn) || d.After(sc.TermEndsOn):
				errs = append(errs, httpx.FieldError{Field: "components.scheduled_on", Message: "Dönemin tarihleri içinde olmalı."})
			default:
				in.ScheduledOn = &d
			}
		}
		items = append(items, in)
	}
	if finals != 1 {
		errs = append(errs, httpx.FieldError{Field: "components", Message: "Planda tam olarak bir final sınavı olmalı."})
	}
	if len(errs) == 0 && total != 10000 {
		errs = append(errs, httpx.FieldError{Field: "components.weight", Message: fmt.Sprintf("Ağırlıkların toplamı 100 olmalı (şu an %.2f).", float64(total)/100)})
	}
	return items, errs
}

func (h *Handler) setPlan(w http.ResponseWriter, r *http.Request) {
	version, ok := httpx.IfMatchVersion(r)
	if !ok {
		httpx.PreconditionRequired(w, r)
		return
	}
	p, ok := h.loadPlan(w, r)
	if !ok {
		return
	}
	if !mayEdit(r, p.SectionContext) {
		h.problem(w, r, http.StatusForbidden, "FORBIDDEN", "Bu şubenin değerlendirme planını sadece öğretim elemanları ve bölüm yönetimi düzenleyebilir.")
		return
	}
	var req planRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	types, err := h.store.AssessmentTypes(r.Context())
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	byCode := make(map[string]AssessmentType, len(types))
	for _, t := range types {
		byCode[t.Code] = t
	}
	items, errs := req.validate(byCode, p.SectionContext)
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	if h.failed(w, r, h.store.SetPlan(r.Context(), actorID(r), p.SectionID, version, items)) {
		return
	}
	h.writePlan(w, r)
}

func (h *Handler) lock(w http.ResponseWriter, r *http.Request) {
	p, ok := h.loadPlan(w, r)
	if !ok {
		return
	}
	if !mayEdit(r, p.SectionContext) {
		h.problem(w, r, http.StatusForbidden, "FORBIDDEN", "Planı sadece şubenin öğretim elemanları ve bölüm yönetimi kilitleyebilir.")
		return
	}
	if h.failed(w, r, h.store.Lock(r.Context(), actorID(r), p.SectionID)) {
		return
	}
	h.writePlan(w, r)
}

type unlockRequest struct {
	Reason string `json:"reason"`
}

func (h *Handler) unlock(w http.ResponseWriter, r *http.Request) {
	p, ok := h.loadPlan(w, r)
	if !ok {
		return
	}
	var req unlockRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if n := utf8.RuneCountInString(reason); n < 5 || n > 500 {
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "reason", Message: "5 ile 500 karakter arasında bir gerekçe yazın."}})
		return
	}
	if !mayManageSection(r, p.SectionContext) {
		h.problem(w, r, http.StatusForbidden, "FORBIDDEN", "Plan kilidini sadece şubeyi yöneten bölüm açabilir.")
		return
	}
	if h.failed(w, r, h.store.Unlock(r.Context(), actorID(r), p.SectionID, reason)) {
		return
	}
	h.writePlan(w, r)
}

func (h *Handler) failed(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, ErrNotFound):
		httpx.NotFound(w, r)
	case errors.Is(err, ErrVersionMismatch):
		httpx.PreconditionFailed(w, r)
	case errors.Is(err, ErrPlanLocked):
		h.problem(w, r, http.StatusConflict, "ASSESSMENT_PLAN_LOCKED", "Değerlendirme planı kilitli; değişiklik için bölümden kilidin açılmasını isteyin.")
	case errors.Is(err, ErrPlanIncomplete):
		h.problem(w, r, http.StatusConflict, "ASSESSMENT_PLAN_INCOMPLETE", "Plan kilitlenmeden önce tamamlanmalı: tek final ve toplam ağırlık 100.")
	case errors.Is(err, ErrSectionInactive):
		h.problem(w, r, http.StatusConflict, "SECTION_CANCELLED", "Şube iptal edilmiş.")
	default:
		h.serverError(w, r, err)
	}
	return true
}

// --- Yardımcılar ---------------------------------------------------------------

func actorID(r *http.Request) string {
	p, _ := authn.PrincipalFrom(r.Context())
	return p.UserID
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func mapSlice[T, U any](in []T, f func(T) U) []U {
	out := make([]U, 0, len(in))
	for _, v := range in {
		out = append(out, f(v))
	}
	return out
}

func (h *Handler) problem(w http.ResponseWriter, r *http.Request, status int, code, detail string) {
	_ = httpx.WriteProblem(w, r, httpx.Problem{Status: status, Code: code, Detail: detail})
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
