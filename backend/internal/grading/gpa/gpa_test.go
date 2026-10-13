package gpa_test

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"testing"

	"github.com/CAPELLAX02/agora/backend/internal/grading/gpa"
)

var rules = gpa.Rules{FullLoad: 300, Honor: 300, HighHonor: 350}

var terms = []gpa.Term{
	{ID: "2024-FALL", Order: 1, Regular: true},
	{ID: "2024-SUMMER", Order: 3, Regular: false},
	{ID: "2025-SPRING", Order: 2, Regular: true},
	{ID: "2025-FALL", Order: 4, Regular: true},
}

// Not ölçeği (katsayı, ortalamaya girer, AKTS kazandırır, geçer).
func grade(letter string) gpa.Attempt {
	switch letter {
	case "A":
		return gpa.Attempt{Coefficient: 400, CountsInGPA: true, EarnsECTS: true, Passing: true}
	case "B1":
		return gpa.Attempt{Coefficient: 350, CountsInGPA: true, EarnsECTS: true, Passing: true}
	case "B3":
		return gpa.Attempt{Coefficient: 300, CountsInGPA: true, EarnsECTS: true, Passing: true}
	case "C1":
		return gpa.Attempt{Coefficient: 275, CountsInGPA: true, EarnsECTS: true, Passing: true}
	case "F2", "F1":
		return gpa.Attempt{Coefficient: 0, CountsInGPA: true}
	case "BŞR":
		return gpa.Attempt{EarnsECTS: true, Passing: true}
	case "BŞZ":
		return gpa.Attempt{}
	}
	panic(letter)
}

func taken(term, letter string, ects gpa.ECTS) gpa.Attempt {
	a := grade(letter)
	a.TermID, a.ECTS, a.Completed = term, ects, true
	return a
}

func h(v gpa.Hundredths) *gpa.Hundredths { return &v }

func TestCompute(t *testing.T) {
	full := func(term string, letters ...string) []gpa.Attempt {
		var out []gpa.Attempt
		for _, l := range letters {
			out = append(out, taken(term, l, 60))
		}
		return out
	}
	viaMakeup := taken("2024-FALL", "C1", 60)
	viaMakeup.ViaMakeup = true
	pending := gpa.Attempt{TermID: "2024-FALL", ECTS: 60}

	cases := []struct {
		name     string
		attempts []gpa.Attempt
		terms    []gpa.Term
		want     []gpa.Result
	}{
		{
			name: "ders yoksa sonuç yok",
		},
		{
			// (4,00 × 6 + 3,50 × 4) / 10 = 3,80. BŞR ortalamaya girmez ama AKTS kazandırır;
			// F1 ortalamaya 0 olarak girer, AKTS kazandırmaz: 38 / 13 = 2,923 → 2,92.
			name: "ortalamaya girmeyen ve kalınan dersler",
			attempts: []gpa.Attempt{taken("2024-FALL", "A", 60), taken("2024-FALL", "B1", 40),
				taken("2024-FALL", "BŞR", 20), taken("2024-FALL", "F1", 30)},
			want: []gpa.Result{{TermID: "2024-FALL", AttemptedECTS: 150, EarnedECTS: 120, TermGPA: h(292),
				CumulativeECTS: 120, CumulativeGPA: h(292), Standing: gpa.Normal}},
		},
		{
			// (4,00 × 5 + 3,00 × 3) / 8 = 3,625 → 3,63 (yarım yukarı).
			name:     "yuvarlama",
			attempts: []gpa.Attempt{taken("2024-FALL", "A", 50), taken("2024-FALL", "B3", 30)},
			want: []gpa.Result{{TermID: "2024-FALL", AttemptedECTS: 80, EarnedECTS: 80, TermGPA: h(363),
				CumulativeECTS: 80, CumulativeGPA: h(363), Standing: gpa.Normal}},
		},
		{
			name:     "sadece başarılı/başarısız notlu dersler: ortalama yok",
			attempts: []gpa.Attempt{taken("2024-FALL", "BŞR", 20), taken("2024-FALL", "BŞZ", 20)},
			want: []gpa.Result{{TermID: "2024-FALL", AttemptedECTS: 40, EarnedECTS: 20,
				CumulativeECTS: 20, Standing: gpa.Normal}},
		},
		{
			// Güz: F2 (6) ve A (6) → YANO = GANO = 2,00. Bahar: F2'li ders C1 ile tekrar alındı
			// (son not geçerli) → YANO 2,75; GANO (4 × 6 + 2,75 × 6) / 12 = 3,375 → 3,38.
			// Güzün sonucu tekrardan etkilenmez.
			name: "tekrar alınan derste son not geçerli",
			attempts: func() []gpa.Attempt {
				failed := taken("2024-FALL", "F2", 60)
				failed.SupersededIn = "2025-SPRING"
				return []gpa.Attempt{failed, taken("2024-FALL", "A", 60), taken("2025-SPRING", "C1", 60)}
			}(),
			want: []gpa.Result{
				{TermID: "2024-FALL", AttemptedECTS: 120, EarnedECTS: 60, TermGPA: h(200),
					CumulativeECTS: 60, CumulativeGPA: h(200), Standing: gpa.Normal},
				{TermID: "2025-SPRING", AttemptedECTS: 60, EarnedECTS: 60, TermGPA: h(275),
					CumulativeECTS: 120, CumulativeGPA: h(338), Standing: gpa.Normal},
			},
		},
		{
			// Geçilen ders not yükseltmek için tekrar alındı, daha düşük not geldi: son not geçerli.
			name: "not yükseltmede de son not geçerli",
			attempts: func() []gpa.Attempt {
				first := taken("2024-FALL", "A", 60)
				first.SupersededIn = "2025-FALL"
				return []gpa.Attempt{first, taken("2025-FALL", "C1", 60)}
			}(),
			want: []gpa.Result{
				{TermID: "2024-FALL", AttemptedECTS: 60, EarnedECTS: 60, TermGPA: h(400),
					CumulativeECTS: 60, CumulativeGPA: h(400), Standing: gpa.Normal},
				{TermID: "2025-FALL", AttemptedECTS: 60, EarnedECTS: 60, TermGPA: h(275),
					CumulativeECTS: 60, CumulativeGPA: h(275), Standing: gpa.Normal},
			},
		},
		{
			name:     "yüksek onur: tam yük, hepsi finalde geçti, YANO ≥ 3,50",
			attempts: full("2024-FALL", "A", "A", "B1", "B1", "B3"),
			want: []gpa.Result{{TermID: "2024-FALL", AttemptedECTS: 300, EarnedECTS: 300, TermGPA: h(360),
				CumulativeECTS: 300, CumulativeGPA: h(360), Standing: gpa.HighHonor}},
		},
		{
			name:     "onur: YANO 3,00 ile 3,50 arası",
			attempts: full("2024-FALL", "B1", "B3", "B3", "B3", "C1"),
			want: []gpa.Result{{TermID: "2024-FALL", AttemptedECTS: 300, EarnedECTS: 300, TermGPA: h(305),
				CumulativeECTS: 300, CumulativeGPA: h(305), Standing: gpa.Honor}},
		},
		{
			name:     "eksik yükle onur yok",
			attempts: full("2024-FALL", "A", "A", "A", "A"),
			want: []gpa.Result{{TermID: "2024-FALL", AttemptedECTS: 240, EarnedECTS: 240, TermGPA: h(400),
				CumulativeECTS: 240, CumulativeGPA: h(400), Standing: gpa.Normal}},
		},
		{
			name:     "başarısız dersle onur yok",
			attempts: append(full("2024-FALL", "A", "A", "A", "A", "A"), taken("2024-FALL", "BŞZ", 20)),
			want: []gpa.Result{{TermID: "2024-FALL", AttemptedECTS: 320, EarnedECTS: 300, TermGPA: h(400),
				CumulativeECTS: 300, CumulativeGPA: h(400), Standing: gpa.Normal}},
		},
		{
			name:     "bütünlemeyle geçilen dersle onur yok",
			attempts: append(full("2024-FALL", "A", "A", "A", "A"), viaMakeup),
			want: []gpa.Result{{TermID: "2024-FALL", AttemptedECTS: 300, EarnedECTS: 300, TermGPA: h(375),
				CumulativeECTS: 300, CumulativeGPA: h(375), Standing: gpa.Normal}},
		},
		{
			name:     "notu verilmemiş ders varken onur belli değil",
			attempts: append(full("2024-FALL", "A", "A", "A", "A", "A"), pending),
			want: []gpa.Result{{TermID: "2024-FALL", AttemptedECTS: 300, EarnedECTS: 300, TermGPA: h(400),
				CumulativeECTS: 300, CumulativeGPA: h(400), Standing: gpa.Normal}},
		},
		{
			name:     "disiplin cezası alan öğrenciye onur yok",
			attempts: full("2024-FALL", "A", "A", "A", "A", "A"),
			terms:    []gpa.Term{{ID: "2024-FALL", Order: 1, Regular: true, Discipline: true}},
			want: []gpa.Result{{TermID: "2024-FALL", AttemptedECTS: 300, EarnedECTS: 300, TermGPA: h(400),
				CumulativeECTS: 300, CumulativeGPA: h(400), Standing: gpa.Normal}},
		},
		{
			// Yaz okulu yarıyıl değildir: onur verilmez ama dersleri ortalamalara girer. Sıra
			// dönem sırasına göredir (Order), verilme sırasına göre değil.
			name: "yaz okulu",
			attempts: append(full("2024-SUMMER", "A", "A", "A", "A", "A"),
				taken("2025-SPRING", "C1", 60)),
			want: []gpa.Result{
				{TermID: "2025-SPRING", AttemptedECTS: 60, EarnedECTS: 60, TermGPA: h(275),
					CumulativeECTS: 60, CumulativeGPA: h(275), Standing: gpa.Normal},
				{TermID: "2024-SUMMER", AttemptedECTS: 300, EarnedECTS: 300, TermGPA: h(400),
					CumulativeECTS: 360, CumulativeGPA: h(379), Standing: gpa.Normal},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ts := c.terms
			if ts == nil {
				ts = terms
			}
			got := gpa.Compute(ts, c.attempts, rules)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("\n got  %s\n want %s", show(got), show(c.want))
			}
		})
	}
}

func show(rs []gpa.Result) string {
	v := func(p *gpa.Hundredths) any {
		if p == nil {
			return nil
		}
		return *p
	}
	s := ""
	for _, r := range rs {
		s += fmt.Sprintf("%s:%s %d/%d %v %d %v; ", r.TermID, r.Standing, r.AttemptedECTS, r.EarnedECTS,
			v(r.TermGPA), r.CumulativeECTS, v(r.CumulativeGPA))
	}
	return s
}

// TestProperties: sonuç denemelerin sırasından bağımsızdır, ortalamalar 0-4 arasındadır,
// kazanılan AKTS alınanı aşmaz ve son dönemin GANO'su sadece geçerli denemelerden gelir.
func TestProperties(t *testing.T) {
	letters := []string{"A", "B1", "B3", "C1", "F2", "F1", "BŞR", "BŞZ"}
	ids := []string{"2024-FALL", "2025-SPRING", "2024-SUMMER", "2025-FALL"}
	rng := rand.New(rand.NewPCG(1, 2))
	for range 500 {
		var attempts []gpa.Attempt
		for range 1 + rng.IntN(12) {
			a := taken(ids[rng.IntN(len(ids))], letters[rng.IntN(len(letters))], gpa.ECTS(10*(1+rng.IntN(8))))
			if rng.IntN(4) == 0 {
				a.Completed = false
			}
			attempts = append(attempts, a)
		}
		got := gpa.Compute(terms, attempts, rules)
		shuffled := append([]gpa.Attempt(nil), attempts...)
		rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		if again := gpa.Compute(terms, shuffled, rules); !reflect.DeepEqual(got, again) {
			t.Fatalf("sıraya bağlı sonuç:\n%s\n%s", show(got), show(again))
		}
		for _, r := range got {
			for _, v := range []*gpa.Hundredths{r.TermGPA, r.CumulativeGPA} {
				if v != nil && (*v < 0 || *v > 400) {
					t.Fatalf("ortalama aralık dışında: %s", show(got))
				}
			}
			if r.EarnedECTS > r.AttemptedECTS {
				t.Fatalf("kazanılan > alınan: %s", show(got))
			}
		}
	}
}
