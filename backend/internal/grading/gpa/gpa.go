// Package gpa, öğrencinin ders geçmişinden dönem sonuçlarını hesaplar: yarıyıl ağırlıklı not
// ortalaması (YANO), genel ağırlıklı not ortalaması (GANO), alınan ve kazanılan AKTS, onur
// durumu. Saf Go paketidir: veritabanına dokunmaz. Aritmetik tamsayılarla yapılır:
// katsayılar ve ortalamalar yüzde birlik (3,50 → 350), AKTS onda birlik (7,5 → 75) birimdedir.
//
// Kurallar (Ankara Üniversitesi Ön Lisans ve Lisans Eğitim-Öğretim Yönetmeliği md. 36, 39, 42):
//   - YANO: dönemde alınan derslerin katsayı × AKTS toplamının, AKTS toplamına bölümü.
//     Katsayısı ortalamaya girmeyen harfler (BŞR, BŞZ, MUAF) katılmaz.
//   - GANO: aynı hesap, girişten itibaren alınan bütün derslerle; tekrar alınan derste son
//     not geçerlidir (eşdeğer ders ve kalınan seçmelinin yerine alınan ders dahil).
//   - Ortalamalar virgülden sonra iki haneye yuvarlanır (yarım yukarı).
//   - Onur: dönemin bütün ders yükünü alıp derslerini finalde başaran (bütünlemeyle değil),
//     disiplin cezası almayan öğrenci; YANO eşiğe göre onur ya da yüksek onur.
package gpa

import (
	"cmp"
	"slices"
)

// ECTS, onda birlik AKTS'dir: 75 = 7,5 AKTS.
type ECTS int64

// Hundredths, yüzde birlik değerdir: katsayılar ve ortalamalar (350 = 3,50).
type Hundredths int64

// Standing, dönemin onur durumudur.
type Standing string

// Onur durumları.
const (
	Normal    Standing = "NORMAL"
	Honor     Standing = "HONOR"      // üstün başarı
	HighHonor Standing = "HIGH_HONOR" // çok üstün başarı
)

// Attempt, bir dersin bir dönemde alınışıdır.
type Attempt struct {
	TermID    string
	Completed bool // notu verildi; false ise dönem sürüyor
	ECTS      ECTS
	// Coefficient, harfin katsayısıdır; CountsInGPA false ise kullanılmaz.
	Coefficient Hundredths
	CountsInGPA bool
	EarnsECTS   bool
	Passing     bool
	// ViaMakeup, harfin bütünlemeyle verildiğini söyler: o dönem onur verilmez.
	ViaMakeup bool
	// SupersededIn, dersin yerini alan denemenin dönemidir (son not geçerli); boşsa deneme
	// hâlâ geçerlidir. O dönemden itibaren GANO'ya bu deneme değil, yenisi girer.
	SupersededIn string
}

// Term, bir dönemdir. Order, dönemlerin sırasıdır: büyük olan daha yeni.
type Term struct {
	ID         string
	Order      int
	Regular    bool  // güz ya da bahar yarıyılı; yaz okulunda onur verilmez
	Discipline bool  // öğrenci bu dönemde disiplin cezası aldı
	Rules      Rules // dönemde geçerli yönetmelik parametreleri
}

// Rules, onur kararının yönetmelik parametreleridir.
type Rules struct {
	FullLoad  ECTS       // bir yarıyılın ders yükü (semester_ects_load)
	Honor     Hundredths // onur için en düşük YANO (honor_gpa.HONOR)
	HighHonor Hundredths // yüksek onur için en düşük YANO (honor_gpa.HIGH_HONOR)
}

// Result, bir dönemin sonucudur.
type Result struct {
	TermID         string
	AttemptedECTS  ECTS        // dönemde notu verilen derslerin AKTS'si
	EarnedECTS     ECTS        // bunlardan kazanılanlar
	TermGPA        *Hundredths // YANO; ortalamaya giren ders yoksa nil
	CumulativeECTS ECTS        // dönem sonunda geçerli notlarla kazanılan AKTS
	CumulativeGPA  *Hundredths // GANO
	Standing       Standing
}

// average, Σ katsayı × AKTS / Σ AKTS'yi yüzde birliğe yarım yukarı yuvarlar.
type average struct {
	points int64 // Σ katsayı (yüzde birlik) × AKTS (onda birlik)
	ects   int64
}

func (a *average) add(at Attempt) {
	if at.CountsInGPA {
		a.points += int64(at.Coefficient) * int64(at.ECTS)
		a.ects += int64(at.ECTS)
	}
}

func (a average) value() *Hundredths {
	if a.ects == 0 {
		return nil
	}
	v := Hundredths((2*a.points + a.ects) / (2 * a.ects))
	return &v
}

// Compute, notu verilmiş dersi olan her dönemin sonucunu dönem sırasıyla döndürür. Bilinmeyen
// bir döneme ait denemeler yok sayılır.
func Compute(terms []Term, attempts []Attempt) []Result {
	order := make(map[string]int, len(terms))
	byID := make(map[string]Term, len(terms))
	for _, t := range terms {
		order[t.ID], byID[t.ID] = t.Order, t
	}
	sorted := slices.Clone(terms)
	slices.SortFunc(sorted, func(a, b Term) int { return cmp.Compare(a.Order, b.Order) })

	var out []Result
	for _, t := range sorted {
		var (
			res              = Result{TermID: t.ID}
			term, cumulative average
			completed        bool
			pending          bool // dönemde notu henüz verilmemiş ders var
			eligible         = t.Regular && !t.Discipline
		)
		for _, a := range attempts {
			o, ok := order[a.TermID]
			if !ok || o > t.Order {
				continue
			}
			if a.TermID == t.ID {
				if !a.Completed {
					pending = true
					continue
				}
				completed = true
				res.AttemptedECTS += a.ECTS
				if a.EarnsECTS {
					res.EarnedECTS += a.ECTS
				}
				term.add(a)
				if !a.Passing || a.ViaMakeup {
					eligible = false
				}
			}
			if !a.Completed {
				continue
			}
			if s, ok := order[a.SupersededIn]; ok && s <= t.Order {
				continue // bu dönemde yerini sonraki deneme aldı
			}
			cumulative.add(a)
			if a.EarnsECTS {
				res.CumulativeECTS += a.ECTS
			}
		}
		if !completed {
			continue
		}
		res.TermGPA, res.CumulativeGPA = term.value(), cumulative.value()
		res.Standing = standing(res, eligible && !pending, t.Rules)
		out = append(out, res)
	}
	return out
}

func standing(r Result, eligible bool, rules Rules) Standing {
	switch {
	case !eligible || r.TermGPA == nil || r.AttemptedECTS < rules.FullLoad:
		return Normal
	case rules.HighHonor > 0 && *r.TermGPA >= rules.HighHonor:
		return HighHonor
	case rules.Honor > 0 && *r.TermGPA >= rules.Honor:
		return Honor
	}
	return Normal
}

// Latest, son dönemin sonucudur (öğrencinin güncel GANO'su ve kazanılan AKTS'si); sonuç
// yoksa ok false döner.
func Latest(results []Result) (Result, bool) {
	if len(results) == 0 {
		return Result{}, false
	}
	return results[len(results)-1], true
}
