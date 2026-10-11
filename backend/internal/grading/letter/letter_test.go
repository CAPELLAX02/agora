package letter_test

import (
	"errors"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/CAPELLAX02/agora/backend/internal/grading/letter"
)

func pts(v float64) *letter.Points {
	p := letter.Points(v*100 + 0.5)
	return &p
}

// auScale, Ankara Üniversitesi lisans not ölçeğidir (migration 00019 ile aynı).
func auScale() letter.Scale {
	ranged := func(l string, coef, lo, hi float64) letter.Grade {
		return letter.Grade{Letter: l, Coefficient: pts(coef), Min: pts(lo), Max: pts(hi), Passing: lo >= 50, CountsInGPA: true, EarnsECTS: lo >= 50}
	}
	return letter.Scale{
		ranged("A", 4, 90, 100), ranged("B1", 3.5, 80, 89), ranged("B2", 3.25, 75, 79), ranged("B3", 3, 70, 74),
		ranged("C1", 2.75, 65, 69), ranged("C2", 2.5, 60, 64), ranged("C3", 2.25, 50, 59), ranged("F2", 0, 0, 49),
		{Letter: "F1", Coefficient: pts(0), CountsInGPA: true, AttendanceFail: true},
		{Letter: "BŞR", Passing: true, EarnsECTS: true},
		{Letter: "BŞZ"},
		{Letter: "MUAF", Passing: true, EarnsECTS: true},
	}
}

var params = letter.Params{FinalMinScore: 5000, PassMinScore: 5000}

// plan: ara sınav %30, ödev %10, final %60, bütünleme %60. Puan nil: girilmedi, -1: girmedi.
func plan(midterm, homework, final, makeup *float64) []letter.Component {
	c := func(kind letter.Kind, w float64, v *float64) letter.Component {
		comp := letter.Component{Kind: kind, Weight: *pts(w)}
		switch {
		case v == nil:
		case *v < 0:
			comp.Absent = true
		default:
			comp.Score = pts(*v)
		}
		return comp
	}
	return []letter.Component{c(letter.InTerm, 30, midterm), c(letter.InTerm, 10, homework), c(letter.Final, 60, final), c(letter.Makeup, 60, makeup)}
}

func f(v float64) *float64 { return &v }

var absent = f(-1)

func TestCompute(t *testing.T) {
	scale := auScale()
	for _, tc := range []struct {
		name       string
		in         letter.Input
		letter     string
		reason     letter.Reason
		weighted   *float64 // nil: hesaplanmaz
		missing    []int
		usedMakeup bool
	}{
		{name: "aralık", in: letter.Input{Components: plan(f(80), f(100), f(90), nil)}, letter: "B1", reason: letter.ReasonRange, weighted: f(88)},
		{name: "89,5 yukarı yuvarlanır", in: letter.Input{Components: plan(f(85), f(100), f(90), nil)}, letter: "A", reason: letter.ReasonRange, weighted: f(89.5)},
		{name: "89,49 aşağı yuvarlanır", in: letter.Input{Components: plan(f(84.97), f(100), f(90), nil)}, letter: "B1", reason: letter.ReasonRange, weighted: f(89.49)},
		{name: "ödeve girmedi: 0", in: letter.Input{Components: plan(f(80), absent, f(90), nil)}, letter: "B2", reason: letter.ReasonRange, weighted: f(78)},
		{name: "ödev girilmemiş", in: letter.Input{Components: plan(f(80), nil, f(90), nil)}, missing: []int{1}},
		{name: "final girilmemiş", in: letter.Input{Components: plan(f(80), f(70), nil, nil)}, missing: []int{2}},
		{name: "final barajı", in: letter.Input{Components: plan(f(100), f(100), f(45), nil)}, letter: "F2", reason: letter.ReasonFinalBelowMin, weighted: f(67)},
		{name: "finale girmedi", in: letter.Input{Components: plan(f(100), f(100), absent, nil)}, letter: "F2", reason: letter.ReasonFinalAbsent},
		{name: "ortalama 50'nin altı", in: letter.Input{Components: plan(f(20), f(0), f(60), nil)}, letter: "F2", reason: letter.ReasonRange, weighted: f(42)},
		{name: "devamsız", in: letter.Input{Components: plan(f(100), f(100), f(100), nil), AttendanceFail: true}, letter: "F1", reason: letter.ReasonAttendanceFail},
		{name: "devamsız, notlar eksik", in: letter.Input{Components: plan(nil, nil, nil, nil), AttendanceFail: true}, letter: "F1", reason: letter.ReasonAttendanceFail},
		{name: "bütünleme finalin yerine", in: letter.Input{Components: plan(f(80), f(100), f(30), f(70))}, letter: "B2", reason: letter.ReasonRange, weighted: f(76), usedMakeup: true},
		{name: "bütünleme düşükse de geçerli", in: letter.Input{Components: plan(f(80), f(100), f(90), f(50))}, letter: "C2", reason: letter.ReasonRange, weighted: f(64), usedMakeup: true},
		{name: "bütünlemede baraj", in: letter.Input{Components: plan(f(100), f(100), f(30), f(45))}, letter: "F2", reason: letter.ReasonFinalBelowMin, weighted: f(67), usedMakeup: true},
		{name: "bütünlemeye girmedi: final geçerli", in: letter.Input{Components: plan(f(80), f(100), f(30), absent)}, letter: "F2", reason: letter.ReasonFinalBelowMin, weighted: f(52)},
		{name: "bütünlemede final girilmemiş olsa da", in: letter.Input{Components: plan(f(80), f(100), nil, f(70))}, letter: "B2", reason: letter.ReasonRange, weighted: f(76), usedMakeup: true},
		{name: "başarılı/başarısız: geçti", in: letter.Input{Components: plan(f(60), f(60), f(60), nil), PassFail: true}, letter: "BŞR", reason: letter.ReasonPassFail, weighted: f(60)},
		{name: "başarılı/başarısız: ortalama düşük", in: letter.Input{Components: plan(f(20), f(0), f(55), nil), PassFail: true}, letter: "BŞZ", reason: letter.ReasonPassFail, weighted: f(39)},
		{name: "başarılı/başarısız: baraj", in: letter.Input{Components: plan(f(100), f(100), f(45), nil), PassFail: true}, letter: "BŞZ", reason: letter.ReasonFinalBelowMin, weighted: f(67)},
		{name: "başarılı/başarısız: devamsız", in: letter.Input{Components: plan(f(100), f(100), f(100), nil), PassFail: true, AttendanceFail: true}, letter: "BŞZ", reason: letter.ReasonAttendanceFail},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := letter.Compute(scale, params, tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if tc.missing != nil {
				if res.Complete || !slices.Equal(res.Missing, tc.missing) || res.Letter != "" {
					t.Errorf("eksik = %+v", res)
				}
				return
			}
			if !res.Complete || res.Letter != tc.letter || res.Reason != tc.reason || res.UsedMakeup != tc.usedMakeup {
				t.Errorf("sonuç = %s (%s, bütünleme %v), beklenen %s (%s)", res.Letter, res.Reason, res.UsedMakeup, tc.letter, tc.reason)
			}
			switch {
			case tc.weighted == nil && res.Weighted != nil:
				t.Errorf("ders başarı puanı hesaplanmamalı: %d", *res.Weighted)
			case tc.weighted != nil && (res.Weighted == nil || *res.Weighted != *pts(*tc.weighted)):
				t.Errorf("ders başarı puanı = %v, beklenen %v", res.Weighted, *tc.weighted)
			}
		})
	}
}

// TestRoundingConsistency: ağırlıklı toplam 49,495 ise gösterilen puan 49,50 olur ve harf
// notu bu puana göre verilir (C3); gösterilen puanla harf notu çelişmez.
func TestRoundingConsistency(t *testing.T) {
	half := []letter.Component{
		{Kind: letter.InTerm, Weight: 5000, Score: pts(48.99)},
		{Kind: letter.Final, Weight: 5000, Score: pts(50)},
	}
	res, err := letter.Compute(auScale(), params, letter.Input{Components: half})
	if err != nil {
		t.Fatal(err)
	}
	if *res.Weighted != 4950 || *res.Rounded != 50 || res.Letter != "C3" {
		t.Errorf("49,495 → puan %d, yuvarlanmış %d, harf %s", *res.Weighted, *res.Rounded, res.Letter)
	}
}

func TestComputeErrors(t *testing.T) {
	noFinal := []letter.Component{{Kind: letter.InTerm, Weight: 10000, Score: pts(70)}}
	if _, err := letter.Compute(auScale(), params, letter.Input{Components: noFinal}); !errors.Is(err, letter.ErrNoFinal) {
		t.Errorf("finalsiz plan: %v", err)
	}
	gap := slices.DeleteFunc(auScale(), func(g letter.Grade) bool { return g.Letter == "B2" })
	if _, err := letter.Compute(gap, params, letter.Input{Components: plan(f(80), f(70), f(76), nil)}); !errors.Is(err, letter.ErrScaleGap) {
		t.Errorf("ölçek boşluğu: %v", err)
	}
	noF1 := slices.DeleteFunc(auScale(), func(g letter.Grade) bool { return g.AttendanceFail })
	if _, err := letter.Compute(noF1, params, letter.Input{Components: plan(f(80), f(70), f(76), nil), AttendanceFail: true}); !errors.Is(err, letter.ErrScaleLetter) {
		t.Errorf("devamsızlık harfi yok: %v", err)
	}
}

// randomInput, rastgele ağırlıklı (toplam 100) bir plan ve notlar üretir.
func randomInput(r *rand.Rand) letter.Input {
	n := 1 + r.IntN(4) // dönem içi bileşen sayısı
	weights := make([]int64, n+1)
	remaining := int64(10000)
	for i := range n {
		w := 100 * (1 + r.Int64N(remaining/100-int64(n-i)))
		w = min(w, remaining-100*int64(n-i))
		weights[i], remaining = w, remaining-w
	}
	weights[n] = remaining
	comps := make([]letter.Component, 0, n+2)
	for i, w := range weights {
		kind := letter.InTerm
		if i == n {
			kind = letter.Final
		}
		c := letter.Component{Kind: kind, Weight: letter.Points(w)}
		if r.IntN(10) == 0 {
			c.Absent = true
		} else {
			c.Score = new(letter.Points)
			*c.Score = letter.Points(r.Int64N(10001))
		}
		comps = append(comps, c)
	}
	comps = append(comps, letter.Component{Kind: letter.Makeup, Weight: letter.Points(weights[n])})
	return letter.Input{Components: comps}
}

// TestProperties: özellik tabanlı testler. Rastgele binlerce plan ve not için
//   - sonuç her zaman ölçekte bir harftir, ders başarı puanı 0-100 arasıdır;
//   - bir notu artırmak harf notunu hiçbir zaman kötüleştirmez (monotonluk);
//   - "girmedi" yerine 0 puan aynı harfi verir (dönem içi bileşenlerde).
func TestProperties(t *testing.T) {
	scale := auScale()
	r := rand.New(rand.NewPCG(2026, 10))
	for range 5000 {
		in := randomInput(r)
		base, err := letter.Compute(scale, params, in)
		if err != nil {
			t.Fatal(err)
		}
		if !base.Complete || scale.Rank(base.Letter) < 0 {
			t.Fatalf("sonuç ölçekte değil: %+v", base)
		}
		if base.Weighted != nil && (*base.Weighted < 0 || *base.Weighted > 10000) {
			t.Fatalf("ders başarı puanı aralık dışı: %d", *base.Weighted)
		}

		i := r.IntN(len(in.Components) - 1) // bütünleme hariç
		better := slices.Clone(in.Components)
		c := better[i]
		switch {
		case c.Absent:
			c.Absent, c.Score = false, new(letter.Points)
			*c.Score = letter.Points(r.Int64N(10001))
		default:
			s := min(*c.Score+letter.Points(r.Int64N(3000)), 10000)
			c.Score = &s
		}
		better[i] = c
		next, err := letter.Compute(scale, params, letter.Input{Components: better})
		if err != nil {
			t.Fatal(err)
		}
		if scale.Rank(next.Letter) > scale.Rank(base.Letter) {
			t.Fatalf("not artınca harf kötüleşti: %s → %s\n%+v\n%+v", base.Letter, next.Letter, in.Components, better)
		}

		if in.Components[i].Kind == letter.InTerm && in.Components[i].Absent {
			zero := slices.Clone(in.Components)
			zero[i].Absent, zero[i].Score = false, new(letter.Points)
			z, err := letter.Compute(scale, params, letter.Input{Components: zero})
			if err != nil {
				t.Fatal(err)
			}
			if z.Letter != base.Letter {
				t.Fatalf("girmedi ile 0 farklı: %s, %s", base.Letter, z.Letter)
			}
		}
	}
}
