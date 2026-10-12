package grading

import (
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

const permFinalize = "grade:finalize"

// mayFinalize, harf notlarını ilan edebilecek ve elle harf notu verebilecek olan: şubenin
// sorumlu öğretim elemanı, MFA'lı oturumda `grade:finalize` ile.
func mayFinalize(r *http.Request, sc SectionContext) bool {
	perms, ok := authz.PermissionsFrom(r.Context())
	return ok && sc.InstructorRole == "PRIMARY" && perms.Has(permFinalize)
}

func (h *GradebookHandler) registerLetters(rt *authz.Router) {
	rt.HandleFunc("GET /api/v1/sections/{id}/letter-grades", authz.Authenticated, h.getLetters)
	rt.HandleFunc("PUT /api/v1/sections/{id}/letter-overrides", authz.Permission(permFinalize), h.setOverrides)
	rt.HandleFunc("POST /api/v1/sections/{id}/letter-grades/publish", authz.Permission(permFinalize), h.publishLetters)
	rt.HandleFunc("POST /api/v1/sections/{id}/makeup/publish", authz.Permission(permFinalize), h.publishMakeup)
}

// --- Yanıt tipleri -------------------------------------------------------------

type scaleItemResponse struct {
	Letter   string   `json:"letter"`
	MinScore *float64 `json:"min_score"`
	MaxScore *float64 `json:"max_score"`
	Passing  bool     `json:"passing"`
}

type overrideResponse struct {
	Letter string    `json:"letter"`
	Reason string    `json:"reason"`
	SetBy  string    `json:"set_by"`
	SetAt  time.Time `json:"set_at"`
}

type letterRowResponse struct {
	EnrollmentID      string             `json:"enrollment_id"`
	Student           studentRefResponse `json:"student"`
	AttendanceStatus  string             `json:"attendance_status"`
	Complete          bool               `json:"complete"`
	MissingComponents []string           `json:"missing_components"`
	WeightedScore     *float64           `json:"weighted_score"`
	RoundedScore      *int64             `json:"rounded_score"`
	ComputedLetter    *string            `json:"computed_letter"`
	Reason            *string            `json:"reason"`
	UsedMakeup        bool               `json:"used_makeup"`
	Override          *overrideResponse  `json:"override"`
	Letter            *string            `json:"letter"`
	Passing           *bool              `json:"passing"`
	MakeupEligible    bool               `json:"makeup_eligible"`
	MakeupLetter      *string            `json:"makeup_letter"`
	PreviousLetter    *string            `json:"previous_letter"`
}

type letterCountResponse struct {
	Letter string `json:"letter"`
	Count  int    `json:"count"`
}

type letterSummaryResponse struct {
	Students int      `json:"students"`
	Complete int      `json:"complete"`
	Mean     *float64 `json:"mean"`
	Median   *float64 `json:"median"`
	PassRate *float64 `json:"pass_rate"`
}

type letterGradesResponse struct {
	SectionID         string                `json:"section_id"`
	GradingMode       string                `json:"grading_mode"`
	PublishedAt       *time.Time            `json:"published_at"`
	PublishedBy       *string               `json:"published_by"`
	CanPublish        bool                  `json:"can_publish"`
	CanOverride       bool                  `json:"can_override"`
	PublishWindow     entryWindowResponse   `json:"publish_window"`
	OverrideWindow    entryWindowResponse   `json:"override_window"`
	MakeupPublishedAt *time.Time            `json:"makeup_published_at"`
	CanPublishMakeup  bool                  `json:"can_publish_makeup"`
	MakeupWindow      entryWindowResponse   `json:"makeup_window"`
	FinalMinScore     float64               `json:"final_min_score"`
	Scale             []scaleItemResponse   `json:"scale"`
	Students          []letterRowResponse   `json:"students"`
	Distribution      []letterCountResponse `json:"distribution"`
	Summary           letterSummaryResponse `json:"summary"`
}

func toWindowResponse(w EntryWindow) entryWindowResponse {
	return entryWindowResponse(w)
}

func toLetterGradesResponse(r *http.Request, g LetterGrades) letterGradesResponse {
	finalize := mayFinalize(r, g.SectionContext)
	published := g.PublishedAt != nil
	scale := g.Scale()
	res := letterGradesResponse{
		SectionID: g.SectionID, GradingMode: g.GradingMode, PublishedAt: g.PublishedAt, PublishedBy: optional(g.PublishedBy),
		CanPublish:        finalize && !published && g.LockedAt != nil && g.PublishWindow.Open && g.Ready(),
		CanOverride:       finalize && !published && g.OverrideWindow.Open,
		PublishWindow:     toWindowResponse(g.PublishWindow),
		OverrideWindow:    toWindowResponse(g.OverrideWindow),
		MakeupPublishedAt: g.MakeupPublishedAt,
		CanPublishMakeup:  finalize && published && g.MakeupPublishedAt == nil && g.MakeupWindow.Open,
		MakeupWindow:      toWindowResponse(g.MakeupWindow),
		FinalMinScore:     Points(g.Params.FinalMinScore).Float(),
		Scale:             []scaleItemResponse{},
		Students:          make([]letterRowResponse, 0, len(g.Rows)),
		Distribution:      []letterCountResponse{},
	}
	pointsPtr := func(p *Points) *float64 {
		if p == nil {
			return nil
		}
		v := p.Float()
		return &v
	}
	for _, it := range ManualLetters(scale, g.GradingMode == "PASS_FAIL") {
		res.Scale = append(res.Scale, scaleItemResponse{
			Letter: it.Letter, Passing: it.Passing,
			MinScore: pointsPtr((*Points)(it.Min)), MaxScore: pointsPtr((*Points)(it.Max)),
		})
	}

	counts := map[string]int{}
	var weighted []Points
	passed := 0
	for _, row := range g.Rows {
		rr := letterRowResponse{
			EnrollmentID:      row.EnrollmentID,
			Student:           studentRefResponse{ID: row.StudentID, StudentNo: row.StudentNo, FirstName: row.FirstName, LastName: row.LastName},
			AttendanceStatus:  string(row.Attendance),
			Complete:          row.Result.Complete,
			MissingComponents: append([]string{}, row.Missing...),
			WeightedScore:     pointsPtr(row.Weighted()),
			RoundedScore:      row.Result.Rounded,
			UsedMakeup:        row.Result.UsedMakeup,
			MakeupEligible:    row.MakeupEligible,
			PreviousLetter:    optional(row.PreviousLetter),
		}
		if g.MakeupPublishedAt == nil {
			rr.MakeupLetter = optional(row.MakeupLetter())
		}
		if row.Result.Complete {
			rr.ComputedLetter = optional(row.Result.Letter)
		}
		rr.Reason = optional(row.Reason())
		if row.Override != nil {
			rr.Override = &overrideResponse{Letter: row.Override.Letter, Reason: row.Override.Reason, SetBy: row.Override.SetBy, SetAt: row.Override.SetAt}
		}
		if l := row.Letter(); l != "" {
			rr.Letter = &l
			counts[l]++
			if grade, ok := g.Scales[row.ScaleID].Find(l); ok {
				p := grade.Passing
				rr.Passing = &p
				if p {
					passed++
				}
			}
			if w := row.Weighted(); w != nil {
				weighted = append(weighted, *w)
			}
		}
		res.Students = append(res.Students, rr)
	}

	// Dağılım: dersin notlandırmasına uygun bütün harfler (sıfır olanlar dahil), ayrıca
	// başka bir harf verilmişse o da.
	listed := map[string]bool{}
	for _, it := range ManualLetters(scale, g.GradingMode == "PASS_FAIL") {
		res.Distribution = append(res.Distribution, letterCountResponse{Letter: it.Letter, Count: counts[it.Letter]})
		listed[it.Letter] = true
	}
	for _, it := range scale {
		if !listed[it.Letter] && counts[it.Letter] > 0 {
			res.Distribution = append(res.Distribution, letterCountResponse{Letter: it.Letter, Count: counts[it.Letter]})
		}
	}
	complete := 0
	for _, c := range counts {
		complete += c
	}
	res.Summary = letterSummaryResponse{Students: len(g.Rows), Complete: complete}
	if len(weighted) > 0 {
		var sum Points
		for _, w := range weighted {
			sum += w
		}
		m := mean(sum, len(weighted)).Float()
		res.Summary.Mean = &m
		sort.Slice(weighted, func(i, j int) bool { return weighted[i] < weighted[j] })
		n := len(weighted)
		med := weighted[n/2]
		if n%2 == 0 {
			med = mean(weighted[n/2-1]+weighted[n/2], 2)
		}
		md := med.Float()
		res.Summary.Median = &md
	}
	if complete > 0 {
		rate := float64(passed) / float64(complete)
		res.Summary.PassRate = &rate
	}
	return res
}

// --- Uçlar ---------------------------------------------------------------------

func (h *GradebookHandler) writeLetters(w http.ResponseWriter, r *http.Request, sectionID string) {
	g, err := h.svc.LetterGrades(r.Context(), sectionID, actorID(r))
	if h.gradebookFailed(w, r, err) {
		return
	}
	h.writeJSON(w, r, http.StatusOK, toLetterGradesResponse(r, g))
}

// letterSection, path'teki şubeyi okur ve görüntüleme yetkisini denetler.
func (h *GradebookHandler) letterSection(w http.ResponseWriter, r *http.Request) (SectionContext, bool) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return SectionContext{}, false
	}
	sc, err := h.svc.repo.sectionContext(r.Context(), h.svc.pool, id, actorID(r), false)
	if h.gradebookFailed(w, r, err) {
		return SectionContext{}, false
	}
	if !mayViewGradebook(r, sc) {
		h.problem(w, r, http.StatusForbidden, "FORBIDDEN", "Bu şubenin notlarını sadece öğretim elemanları ve yetkili birimler görebilir.")
		return SectionContext{}, false
	}
	return sc, true
}

func (h *GradebookHandler) getLetters(w http.ResponseWriter, r *http.Request) {
	sc, ok := h.letterSection(w, r)
	if !ok {
		return
	}
	h.writeLetters(w, r, sc.SectionID)
}

type overridesRequest struct {
	Items []struct {
		EnrollmentID string  `json:"enrollment_id"`
		Letter       *string `json:"letter"`
		Reason       string  `json:"reason"`
	} `json:"items"`
}

func (h *GradebookHandler) setOverrides(w http.ResponseWriter, r *http.Request) {
	sc, ok := h.letterSection(w, r)
	if !ok {
		return
	}
	if !mayFinalize(r, sc) {
		h.problem(w, r, http.StatusForbidden, "FORBIDDEN", "Elle harf notunu sadece şubenin sorumlu öğretim elemanı verebilir.")
		return
	}
	var req overridesRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	if len(req.Items) == 0 || len(req.Items) > 1000 {
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "items", Message: "1 ile 1000 arasında öğrenci olmalı."}})
		return
	}
	var errs []httpx.FieldError
	seen := map[string]bool{}
	items := make([]OverrideInput, 0, len(req.Items))
	for _, it := range req.Items {
		in := OverrideInput{EnrollmentID: it.EnrollmentID, Reason: strings.TrimSpace(it.Reason)}
		if it.Letter != nil {
			in.Letter = strings.TrimSpace(*it.Letter)
		}
		switch {
		case !httpx.ValidUUID(it.EnrollmentID):
			errs = append(errs, httpx.FieldError{Field: "items.enrollment_id", Message: "geçerli bir UUID olmalı"})
		case seen[it.EnrollmentID]:
			errs = append(errs, httpx.FieldError{Field: "items.enrollment_id", Message: "Bir öğrenci birden fazla kez verilmiş."})
		case in.Letter != "" && (utf8.RuneCountInString(in.Reason) < 5 || utf8.RuneCountInString(in.Reason) > 500):
			errs = append(errs, httpx.FieldError{Field: "items.reason", Message: "Elle harf notu için 5 ile 500 karakter arasında bir gerekçe yazın."})
		}
		seen[it.EnrollmentID] = true
		items = append(items, in)
	}
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	if h.gradebookFailed(w, r, h.svc.SetOverrides(r.Context(), actorID(r), sc, items)) {
		return
	}
	h.writeLetters(w, r, sc.SectionID)
}

func (h *GradebookHandler) publishLetters(w http.ResponseWriter, r *http.Request) {
	sc, ok := h.letterSection(w, r)
	if !ok {
		return
	}
	if !mayFinalize(r, sc) {
		h.problem(w, r, http.StatusForbidden, "FORBIDDEN", "Harf notlarını sadece şubenin sorumlu öğretim elemanı ilan edebilir.")
		return
	}
	if _, err := h.svc.PublishGrades(r.Context(), actorID(r), sc); h.gradebookFailed(w, r, err) {
		return
	}
	h.writeLetters(w, r, sc.SectionID)
}

func (h *GradebookHandler) publishMakeup(w http.ResponseWriter, r *http.Request) {
	sc, ok := h.letterSection(w, r)
	if !ok {
		return
	}
	if !mayFinalize(r, sc) {
		h.problem(w, r, http.StatusForbidden, "FORBIDDEN", "Bütünleme sonuçlarını sadece şubenin sorumlu öğretim elemanı ilan edebilir.")
		return
	}
	if _, err := h.svc.PublishMakeup(r.Context(), actorID(r), sc); h.gradebookFailed(w, r, err) {
		return
	}
	h.writeLetters(w, r, sc.SectionID)
}
