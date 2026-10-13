package curriculum_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/CAPELLAX02/agora/backend/internal/curriculum"
	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/dbtest"
)

func TestDefaultGradeScale(t *testing.T) {
	pool := dbtest.New(t)
	repo := curriculum.NewRepository(pool)
	ctx := context.Background()

	// Varsayılan: yönetmeliğin 2023 değişikliğiyle gelen ölçek; eski ölçek eski notlar için kalır.
	scales, err := repo.GradeScales(ctx)
	if err != nil || len(scales) != 2 {
		t.Fatalf("ölçekler = %d, %v", len(scales), err)
	}
	au := scales[0]
	if au.Code != "AU-LISANS-2023" || !au.IsDefault || len(au.Items) != 14 || scales[1].Code != "AU-LISANS" || scales[1].IsDefault {
		t.Fatalf("ölçekler = %+v", scales)
	}
	letters := func(g curriculum.GradeScale) map[string]curriculum.GradeItem {
		out := map[string]curriculum.GradeItem{}
		for _, it := range g.Items {
			out[it.Letter] = it
		}
		return out
	}
	current, old := letters(au), letters(scales[1])
	if b1 := current["B1"]; *b1.Coefficient != 3.5 || *b1.MinScore != 85 || *b1.MaxScore != 89 || !b1.IsPassing {
		t.Errorf("B1 = %+v", b1)
	}
	if f1 := current["F1"]; *f1.Coefficient != 1.5 || *f1.MinScore != 50 || *f1.MaxScore != 59 || f1.IsPassing || !f1.CountsInGPA || f1.EarnsECTS {
		t.Errorf("F1 = %+v", f1)
	}
	if f3 := current["F3"]; !f3.IsFinalAbsent || f3.IsAttendanceFail || f3.MinScore != nil || !f3.CountsInGPA {
		t.Errorf("F3 = %+v", f3)
	}
	if f4 := current["F4"]; !f4.IsAttendanceFail || f4.MinScore != nil || !f4.CountsInGPA || f4.EarnsECTS {
		t.Errorf("F4 = %+v", f4)
	}
	if f1 := old["F1"]; !f1.IsAttendanceFail || f1.MinScore != nil {
		t.Errorf("eski ölçekte F1 devamsızlık = %+v", f1)
	}
	if bsr := current["BŞR"]; bsr.CountsInGPA || !bsr.EarnsECTS || bsr.Coefficient != nil {
		t.Errorf("BŞR = %+v", bsr)
	}
	if au.Items[0].Letter != "A" || au.Items[13].Letter != "MUAF" {
		t.Errorf("sıra: %s … %s", au.Items[0].Letter, au.Items[13].Letter)
	}

	// Yeni ölçek varsayılan yapılınca eskisi varsayılanlıktan çıkar.
	in := curriculum.GradeScaleInput{Code: "YENI", NameTR: "Yeni ölçek", NameEN: "New scale", EffectiveFromYear: 2030, IsDefault: true, Items: au.Items}
	id, err := repo.CreateGradeScale(ctx, "", in)
	if err != nil {
		t.Fatal(err)
	}
	scales, _ = repo.GradeScales(ctx)
	if scales[0].ID != id || scales[1].IsDefault || scales[2].IsDefault {
		t.Errorf("tek varsayılan ölçek olmalı: %+v", scales)
	}
	in.IsDefault = false
	if err := repo.UpdateGradeScale(ctx, "", id, 1, in); !errors.Is(err, curriculum.ErrDefaultRequired) {
		t.Errorf("varsayılanı kaldırma: %v", err)
	}
	if _, err := repo.CreateGradeScale(ctx, "", curriculum.GradeScaleInput{Code: "YENI", NameTR: "x", NameEN: "x", EffectiveFromYear: 2030, Items: au.Items}); !errors.Is(err, curriculum.ErrConflict) {
		t.Errorf("aynı kod: %v", err)
	}
}

func TestRegulationParameters(t *testing.T) {
	pool := dbtest.New(t)
	repo := curriculum.NewRepository(pool)
	ctx := context.Background()

	day := func(s string) time.Time {
		d, _ := time.Parse(time.DateOnly, s)
		return d
	}
	params, err := repo.RegulationParameters(ctx, day("2026-10-01"))
	if err != nil || len(params) != 9 {
		t.Fatalf("parametreler = %d, %v", len(params), err)
	}
	if err := repo.SetRegulationParameter(ctx, "", "final_min_score", json.RawMessage(`45`), day("2027-02-01"), "Senato kararı"); err != nil {
		t.Fatal(err)
	}
	value := func(at string) string {
		params, _ := repo.RegulationParameters(ctx, day(at))
		for _, p := range params {
			if p.Key == "final_min_score" {
				return string(p.Value)
			}
		}
		return ""
	}
	if value("2027-01-31") != "50" || value("2027-02-01") != "45" {
		t.Errorf("geçerli değerler: %s / %s", value("2027-01-31"), value("2027-02-01"))
	}
	history, _ := repo.RegulationHistory(ctx, "final_min_score")
	if len(history) != 2 || history[1].EffectiveTo == nil || history[0].Note != "Senato kararı" {
		t.Errorf("geçmiş = %+v", history)
	}

	if err := repo.SetRegulationParameter(ctx, "", "final_min_score", json.RawMessage(`40`), day("2027-01-15"), ""); !errors.Is(err, curriculum.ErrNotAfterCurrent) {
		t.Errorf("geçmişe değer: %v", err)
	}
	if err := repo.SetRegulationParameter(ctx, "", "final_min_score", json.RawMessage(`{"x": 1}`), day("2028-01-01"), ""); !errors.Is(err, curriculum.ErrValueTypeMismatch) {
		t.Errorf("tür değişikliği: %v", err)
	}
	if err := repo.SetRegulationParameter(ctx, "", "yok_boyle", json.RawMessage(`1`), day("2028-01-01"), ""); !errors.Is(err, curriculum.ErrUnknownRegulation) {
		t.Errorf("tanımsız parametre: %v", err)
	}
}

func TestHTTPGradeScales(t *testing.T) {
	s := newServer(t)
	s.grants[registrar] = append(s.grants[registrar], authz.Grant{Permission: "gradescale:manage", ScopeType: authz.ScopeUniversity})
	s.grants[deptMg] = append(s.grants[deptMg], authz.Grant{Permission: "gradescale:manage", ScopeType: authz.ScopeDepartment, ScopeID: s.fx.bil})

	res := s.do("GET", "/api/v1/grade-scales", student, nil, nil)
	if res.status != 200 || len(res.items()) != 2 {
		t.Fatalf("ölçekler: %d %s", res.status, res.raw)
	}
	au := res.items()[0].(map[string]any)

	f := func(v float64) *float64 { return &v }
	item := func(letter string, coef *float64, lo, hi *float64, passing bool) map[string]any {
		return map[string]any{"letter": letter, "coefficient": coef, "min_score": lo, "max_score": hi,
			"is_passing": passing, "counts_in_gpa": coef != nil, "earns_ects": passing}
	}
	body := map[string]any{
		"code": "basit", "name_tr": "Basit ölçek", "name_en": "Simple scale", "effective_from_year": 2027,
		"items": []any{item("G", f(4), f(50), f(100), true), item("K", f(0), f(0), f(49), false), item("DZ", nil, nil, nil, false)},
	}
	if res := s.do("POST", "/api/v1/grade-scales", deptMg, nil, body); res.status != 403 {
		t.Errorf("bölüm kapsamlı yetki yetmez: %d", res.status)
	}
	gap := map[string]any{}
	for k, v := range body {
		gap[k] = v
	}
	gap["items"] = []any{item("G", f(4), f(60), f(100), true), item("K", f(0), f(0), f(49), false)}
	if res := s.do("POST", "/api/v1/grade-scales", registrar, nil, gap); res.status != 400 {
		t.Errorf("50-59 boşluğu: %d %s", res.status, res.raw)
	}
	for name, items := range map[string][]any{
		"aralıklı finale girmedi": {item("G", f(4), f(50), f(100), true),
			map[string]any{"letter": "K", "coefficient": 0, "min_score": 0, "max_score": 49, "is_passing": false, "counts_in_gpa": true, "is_final_absent": true}},
		"iki devamsızlık harfi": {item("G", f(4), f(0), f(100), true),
			map[string]any{"letter": "D1", "coefficient": 0, "is_passing": false, "counts_in_gpa": true, "is_attendance_fail": true},
			map[string]any{"letter": "D2", "coefficient": 0, "is_passing": false, "counts_in_gpa": true, "is_attendance_fail": true}},
	} {
		bad := map[string]any{}
		for k, v := range body {
			bad[k] = v
		}
		bad["items"] = items
		if res := s.do("POST", "/api/v1/grade-scales", registrar, nil, bad); res.status != 400 {
			t.Errorf("%s: %d %s", name, res.status, res.raw)
		}
	}
	res = s.do("POST", "/api/v1/grade-scales", registrar, nil, body)
	if res.status != 201 || res.body["code"] != "BASIT" || len(res.body["items"].([]any)) != 3 || res.body["is_default"] != false {
		t.Fatalf("ölçek: %d %s", res.status, res.raw)
	}
	id := res.body["id"].(string)
	body["code"], body["name_en"] = "", "Simple"
	if res := s.do("PUT", "/api/v1/grade-scales/"+id, registrar, map[string]string{"If-Match": `"1"`}, body); res.status != 200 || res.body["name_en"] != "Simple" {
		t.Errorf("güncelleme: %d %s", res.status, res.raw)
	}
	if res := s.do("PUT", "/api/v1/grade-scales/"+au["id"].(string), registrar, map[string]string{"If-Match": `"1"`},
		map[string]any{"name_tr": "x", "name_en": "x", "effective_from_year": 2016, "is_default": false, "items": au["items"]}); res.status != 409 || res.code() != "DEFAULT_SCALE_REQUIRED" {
		t.Errorf("varsayılanı kaldırma: %d %s", res.status, res.raw)
	}

	// Yönetmelik parametreleri.
	res = s.do("GET", "/api/v1/regulation-parameters?at=2026-10-01", student, nil, nil)
	if res.status != 200 || len(res.items()) != 9 {
		t.Fatalf("parametreler: %d %s", res.status, res.raw)
	}
	if res := s.do("POST", "/api/v1/regulation-parameters/final_min_score", deptMg, nil, map[string]any{"value": 45, "effective_from": "2027-02-01"}); res.status != 403 {
		t.Errorf("bölüm kapsamı: %d", res.status)
	}
	res = s.do("POST", "/api/v1/regulation-parameters/final_min_score", registrar, nil, map[string]any{"value": 45, "effective_from": "2027-02-01", "note": "Senato"})
	if res.status != 201 || len(res.items()) != 2 || res.items()[0].(map[string]any)["value"] != 45.0 {
		t.Fatalf("yeni değer: %d %s", res.status, res.raw)
	}
	if res := s.do("POST", "/api/v1/regulation-parameters/final_min_score", registrar, nil, map[string]any{"value": "elli", "effective_from": "2028-02-01"}); res.status != 400 {
		t.Errorf("tür değişikliği: %d", res.status)
	}
	if res := s.do("GET", "/api/v1/regulation-parameters/yok_boyle", student, nil, nil); res.status != 404 {
		t.Errorf("tanımsız parametre: %d", res.status)
	}
}
