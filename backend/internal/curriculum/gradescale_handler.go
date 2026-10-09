package curriculum

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/httpx"
)

const permGradeScaleManage = "gradescale:manage"

func (h *Handler) registerGradeScales(rt *authz.Router) {
	rt.HandleFunc("GET /api/v1/grade-scales", authz.Permission(permCurriculumRead), h.listGradeScales)
	rt.HandleFunc("POST /api/v1/grade-scales", authz.Permission(permGradeScaleManage), h.createGradeScale)
	rt.HandleFunc("GET /api/v1/grade-scales/{id}", authz.Permission(permCurriculumRead), h.getGradeScale)
	rt.HandleFunc("PUT /api/v1/grade-scales/{id}", authz.Permission(permGradeScaleManage), h.updateGradeScale)

	rt.HandleFunc("GET /api/v1/regulation-parameters", authz.Permission(permCurriculumRead), h.listRegulations)
	rt.HandleFunc("GET /api/v1/regulation-parameters/{key}", authz.Permission(permCurriculumRead), h.regulationHistory)
	rt.HandleFunc("POST /api/v1/regulation-parameters/{key}", authz.Permission(permGradeScaleManage), h.setRegulation)
}

// --- Yanıt tipleri -------------------------------------------------------------

// gradeItemJSON, harf notunun hem istek hem yanıt biçimidir.
type gradeItemJSON struct {
	Letter           string   `json:"letter"`
	Coefficient      *float64 `json:"coefficient"`
	MinScore         *float64 `json:"min_score"`
	MaxScore         *float64 `json:"max_score"`
	IsPassing        bool     `json:"is_passing"`
	CountsInGPA      bool     `json:"counts_in_gpa"`
	EarnsECTS        bool     `json:"earns_ects"`
	IsAttendanceFail bool     `json:"is_attendance_fail"`
}

type gradeScaleResponse struct {
	ID                string          `json:"id"`
	Code              string          `json:"code"`
	NameTR            string          `json:"name_tr"`
	NameEN            string          `json:"name_en"`
	EffectiveFromYear int             `json:"effective_from_year"`
	IsDefault         bool            `json:"is_default"`
	Items             []gradeItemJSON `json:"items"`
	Version           int             `json:"version"`
}

type regulationResponse struct {
	Key           string          `json:"key"`
	Value         json.RawMessage `json:"value"`
	EffectiveFrom string          `json:"effective_from"`
	EffectiveTo   *string         `json:"effective_to"`
	DescriptionTR string          `json:"description_tr"`
	Note          *string         `json:"note"`
}

func toGradeScaleResponse(g GradeScale) gradeScaleResponse {
	return gradeScaleResponse{
		ID: g.ID, Code: g.Code, NameTR: g.NameTR, NameEN: g.NameEN, EffectiveFromYear: g.EffectiveFromYear,
		IsDefault: g.IsDefault, Version: g.Version,
		Items: mapSlice(g.Items, func(it GradeItem) gradeItemJSON { return gradeItemJSON(it) }),
	}
}

func toRegulationResponse(p RegulationParameter) regulationResponse {
	res := regulationResponse{
		Key: p.Key, Value: p.Value, EffectiveFrom: p.EffectiveFrom.Format(time.DateOnly),
		DescriptionTR: p.DescriptionTR, Note: optional(p.Note),
	}
	if p.EffectiveTo != nil {
		to := p.EffectiveTo.Format(time.DateOnly)
		res.EffectiveTo = &to
	}
	return res
}

// --- Not ölçekleri -------------------------------------------------------------

func (h *Handler) listGradeScales(w http.ResponseWriter, r *http.Request) {
	scales, err := h.store.GradeScales(r.Context())
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	h.writeJSON(w, r, http.StatusOK, httpx.ListResponse[gradeScaleResponse]{Items: mapSlice(scales, toGradeScaleResponse)})
}

func (h *Handler) getGradeScale(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return
	}
	h.writeGradeScale(w, r, http.StatusOK, id)
}

func (h *Handler) writeGradeScale(w http.ResponseWriter, r *http.Request, status int, id string) {
	g, err := h.store.GradeScale(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		httpx.NotFound(w, r)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	w.Header().Set("ETag", httpx.ETag(g.Version))
	h.writeJSON(w, r, status, toGradeScaleResponse(g))
}

type gradeScaleRequest struct {
	Code              string          `json:"code"`
	NameTR            string          `json:"name_tr"`
	NameEN            string          `json:"name_en"`
	EffectiveFromYear int             `json:"effective_from_year"`
	IsDefault         bool            `json:"is_default"`
	Items             []gradeItemJSON `json:"items"`
}

var (
	scaleCodePattern = regexp.MustCompile(`^[A-Z0-9-]{2,30}$`)
	letterPattern    = regexp.MustCompile(`^[A-ZÇĞİÖŞÜ][A-ZÇĞİÖŞÜ0-9]{0,4}$`)
)

// validate, ölçeği doğrular. Puanla verilen harflerin aralıkları tam sayıdır ve 0-100
// arasını boşluksuz, çakışmasız kapsar: her ders başarı puanının tek bir harfi olur.
func (req gradeScaleRequest) validate(creating bool) (GradeScaleInput, []httpx.FieldError) {
	in := GradeScaleInput{
		Code: strings.ToUpper(strings.TrimSpace(req.Code)), NameTR: strings.TrimSpace(req.NameTR), NameEN: strings.TrimSpace(req.NameEN),
		EffectiveFromYear: req.EffectiveFromYear, IsDefault: req.IsDefault,
	}
	var errs []httpx.FieldError
	switch {
	case creating && !scaleCodePattern.MatchString(in.Code):
		errs = append(errs, httpx.FieldError{Field: "code", Message: "2-30 büyük harf, rakam ya da tire olmalı."})
	case !creating && req.Code != "":
		errs = append(errs, httpx.FieldError{Field: "code", Message: "Ölçek kodu değiştirilemez."})
	}
	errs = append(errs, requiredName("name_tr", in.NameTR)...)
	errs = append(errs, requiredName("name_en", in.NameEN)...)
	if in.EffectiveFromYear < 1946 || in.EffectiveFromYear > 2100 {
		errs = append(errs, httpx.FieldError{Field: "effective_from_year", Message: "1946 ile 2100 arasında olmalı."})
	}
	if len(req.Items) == 0 || len(req.Items) > 30 {
		errs = append(errs, httpx.FieldError{Field: "items", Message: "1 ile 30 arasında harf notu olmalı."})
	}

	seen := map[string]bool{}
	var ranged []GradeItem
	for _, it := range req.Items {
		item := GradeItem(it)
		item.Letter = strings.ToUpper(strings.TrimSpace(item.Letter))
		switch {
		case !letterPattern.MatchString(item.Letter):
			errs = append(errs, httpx.FieldError{Field: "items.letter", Message: "1-5 büyük harf ya da rakam olmalı (ör. B1, BŞR)."})
		case seen[item.Letter]:
			errs = append(errs, httpx.FieldError{Field: "items.letter", Message: fmt.Sprintf("%s birden fazla kez verilmiş.", item.Letter)})
		}
		seen[item.Letter] = true
		if c := item.Coefficient; c != nil && (*c < 0 || *c > 4 || math.Round(*c*100) != *c*100) {
			errs = append(errs, httpx.FieldError{Field: "items.coefficient", Message: "0 ile 4 arasında, en fazla iki ondalık basamaklı olmalı."})
		}
		if item.CountsInGPA && item.Coefficient == nil {
			errs = append(errs, httpx.FieldError{Field: "items.coefficient", Message: fmt.Sprintf("%s ortalamaya giriyorsa katsayısı olmalı.", item.Letter)})
		}
		switch {
		case (item.MinScore == nil) != (item.MaxScore == nil):
			errs = append(errs, httpx.FieldError{Field: "items.min_score", Message: "Puan aralığının iki ucu birlikte verilmeli."})
		case item.MinScore != nil:
			lo, hi := *item.MinScore, *item.MaxScore
			if lo != math.Trunc(lo) || hi != math.Trunc(hi) || lo < 0 || hi > 100 || lo > hi {
				errs = append(errs, httpx.FieldError{Field: "items.min_score", Message: "Puan aralığı 0-100 arasında tam sayılardan oluşmalı."})
				continue
			}
			if item.IsAttendanceFail {
				errs = append(errs, httpx.FieldError{Field: "items.is_attendance_fail", Message: "Devamsızlık notu puanla verilmez."})
			}
			ranged = append(ranged, item)
		}
		in.Items = append(in.Items, item)
	}
	if len(errs) == 0 {
		slices.SortFunc(ranged, func(a, b GradeItem) int { return cmp.Compare(*a.MinScore, *b.MinScore) })
		next := 0.0
		for _, it := range ranged {
			if *it.MinScore != next {
				errs = append(errs, httpx.FieldError{Field: "items.min_score", Message: fmt.Sprintf("Puan aralıkları %.0f puanda boşluk ya da çakışma içeriyor.", next)})
				break
			}
			next = *it.MaxScore + 1
		}
		if len(errs) == 0 && next != 101 {
			errs = append(errs, httpx.FieldError{Field: "items.max_score", Message: "Puan aralıkları 0-100 arasını tümüyle kapsamalı."})
		}
	}
	return in, errs
}

func (h *Handler) createGradeScale(w http.ResponseWriter, r *http.Request) {
	var req gradeScaleRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	in, errs := req.validate(true)
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	// Not ölçeği üniversite geneli bir tanımdır.
	if !h.allowedTarget(w, r, permGradeScaleManage, authz.Target{}) {
		return
	}
	id, err := h.store.CreateGradeScale(r.Context(), actorID(r), in)
	if h.gradeScaleWriteFailed(w, r, err) {
		return
	}
	w.Header().Set("Location", "/api/v1/grade-scales/"+id)
	h.writeGradeScale(w, r, http.StatusCreated, id)
}

func (h *Handler) updateGradeScale(w http.ResponseWriter, r *http.Request) {
	version, ok := httpx.IfMatchVersion(r)
	if !ok {
		httpx.PreconditionRequired(w, r)
		return
	}
	id := r.PathValue("id")
	if !httpx.ValidUUID(id) {
		httpx.NotFound(w, r)
		return
	}
	var req gradeScaleRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	in, errs := req.validate(false)
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	if !h.allowedTarget(w, r, permGradeScaleManage, authz.Target{}) {
		return
	}
	if h.gradeScaleWriteFailed(w, r, h.store.UpdateGradeScale(r.Context(), actorID(r), id, version, in)) {
		return
	}
	h.writeGradeScale(w, r, http.StatusOK, id)
}

func (h *Handler) gradeScaleWriteFailed(w http.ResponseWriter, r *http.Request, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, ErrNotFound):
		httpx.NotFound(w, r)
	case errors.Is(err, ErrVersionMismatch):
		httpx.PreconditionFailed(w, r)
	case errors.Is(err, ErrConflict):
		h.problem(w, r, http.StatusConflict, "GRADE_SCALE_CODE_TAKEN", "Bu kodla bir not ölçeği zaten var.")
	case errors.Is(err, ErrDefaultRequired):
		h.problem(w, r, http.StatusConflict, "DEFAULT_SCALE_REQUIRED",
			"Varsayılan ölçek doğrudan kaldırılamaz; başka bir ölçeği varsayılan yapın.")
	case errors.Is(err, ErrScoreRangeOverlap), errors.Is(err, ErrDuplicate):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "items", Message: "Harfler ya da puan aralıkları tekrarlanıyor."}})
	default:
		h.serverError(w, r, err)
	}
	return true
}

// --- Yönetmelik parametreleri --------------------------------------------------

// listRegulations, verilen günde (varsayılan bugün) geçerli parametreleri döndürür.
func (h *Handler) listRegulations(w http.ResponseWriter, r *http.Request) {
	at := time.Now()
	if v := r.URL.Query().Get("at"); v != "" {
		d, err := time.Parse(time.DateOnly, v)
		if err != nil {
			httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "at", Message: "YYYY-AA-GG biçiminde bir tarih olmalı."}})
			return
		}
		at = d
	}
	params, err := h.store.RegulationParameters(r.Context(), at)
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	h.writeJSON(w, r, http.StatusOK, httpx.ListResponse[regulationResponse]{Items: mapSlice(params, toRegulationResponse)})
}

func (h *Handler) regulationHistory(w http.ResponseWriter, r *http.Request) {
	h.writeRegulationHistory(w, r, http.StatusOK, r.PathValue("key"))
}

func (h *Handler) writeRegulationHistory(w http.ResponseWriter, r *http.Request, status int, key string) {
	history, err := h.store.RegulationHistory(r.Context(), key)
	if errors.Is(err, ErrUnknownRegulation) {
		httpx.NotFound(w, r)
		return
	}
	if err != nil {
		h.serverError(w, r, err)
		return
	}
	h.writeJSON(w, r, status, httpx.ListResponse[regulationResponse]{Items: mapSlice(history, toRegulationResponse)})
}

type regulationRequest struct {
	Value         json.RawMessage `json:"value"`
	EffectiveFrom string          `json:"effective_from"`
	Note          string          `json:"note"`
}

func (h *Handler) setRegulation(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	var req regulationRequest
	if err := httpx.ReadJSON(w, r, &req); err != nil {
		httpx.InvalidBody(w, r, err)
		return
	}
	var errs []httpx.FieldError
	if len(req.Value) == 0 || string(req.Value) == "null" {
		errs = append(errs, httpx.FieldError{Field: "value", Message: "Zorunlu."})
	}
	from, err := time.Parse(time.DateOnly, req.EffectiveFrom)
	if err != nil {
		errs = append(errs, httpx.FieldError{Field: "effective_from", Message: "YYYY-AA-GG biçiminde bir tarih olmalı."})
	}
	note := strings.TrimSpace(req.Note)
	if utf8.RuneCountInString(note) > 1000 {
		errs = append(errs, httpx.FieldError{Field: "note", Message: "En fazla 1000 karakter."})
	}
	if len(errs) > 0 {
		httpx.ValidationFailed(w, r, errs)
		return
	}
	if !h.allowedTarget(w, r, permGradeScaleManage, authz.Target{}) {
		return
	}
	err = h.store.SetRegulationParameter(r.Context(), actorID(r), key, req.Value, from, note)
	switch {
	case errors.Is(err, ErrUnknownRegulation):
		httpx.NotFound(w, r)
		return
	case errors.Is(err, ErrNotAfterCurrent):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "effective_from", Message: "Yürürlükteki değerin başlangıcından sonra olmalı."}})
		return
	case errors.Is(err, ErrValueTypeMismatch):
		httpx.ValidationFailed(w, r, []httpx.FieldError{{Field: "value", Message: "Değerin türü (sayı, nesne, dizi ...) önceki değerle aynı olmalı."}})
		return
	case err != nil:
		h.serverError(w, r, err)
		return
	}
	h.writeRegulationHistory(w, r, http.StatusCreated, key)
}
