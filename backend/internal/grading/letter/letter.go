// Package letter, bir dersin bileşen notlarından ders başarı puanını ve harf notunu
// hesaplar. Saf Go paketidir: veritabanına dokunmaz, girdisi not ölçeği, değerlendirme
// planı ve notlardır. Bütün aritmetik yüzde birlik tamsayılarla yapılır; kayan nokta
// kullanılmaz (yol haritası §5 #14).
package letter

import (
	"errors"
	"fmt"
)

// Points, yüzde birlik tamsayıdır: 7550 = 75,50. Hem puanlar hem ağırlıklar (yüzde) bu
// birimdedir.
type Points int64

// Kind, bileşenin plandaki yeridir.
type Kind int

// Bileşen türleri.
const (
	InTerm Kind = iota // ara sınav, ödev, proje ...: ağırlıkları finalinkiyle toplam 100
	Final
	Makeup // bütünleme: girilmişse finalin yerine geçer, ağırlığı finalinkine eşittir
)

// Component, bir bileşenin ağırlığı ve öğrencinin bu bileşendeki notudur. Score nil ve
// Absent false ise not girilmemiştir.
type Component struct {
	Kind   Kind
	Weight Points
	Score  *Points
	Absent bool // girmedi: dönem içi bileşende 0 sayılır, finalde F2
}

func (c Component) entered() bool { return c.Score != nil || c.Absent }

// Grade, not ölçeğinin bir satırıdır. Puan aralığı olmayan harfler (F1, BŞR, BŞZ, MUAF)
// puanla değil durumla verilir.
type Grade struct {
	Letter         string
	Coefficient    *Points // GANO katsayısı (yüzde birlik: 3,50 → 350); BŞR gibi harflerde yok
	Min, Max       *Points // ders başarı puanının tam sayıya yuvarlanmış hali için kapalı aralık
	Passing        bool
	CountsInGPA    bool
	EarnsECTS      bool
	AttendanceFail bool // devamsızlık harfi (F1)
}

// Scale, not ölçeğidir: başarıdan başarısızlığa sıralı harfler.
type Scale []Grade

// Params, yönetmelik parametreleridir (curriculum.regulation_parameters).
type Params struct {
	FinalMinScore Points // final ya da bütünlemede en az bu puan alınmalı (final_min_score)
	PassMinScore  Points // başarılı/başarısız notlu derste geçme puanı (pass_min_score)
}

// Input, bir öğrencinin bir dersteki notlarıdır.
type Input struct {
	Components     []Component
	AttendanceFail bool // devamsız
	PassFail       bool // dersin notlandırması başarılı/başarısız (BŞR/BŞZ)
}

// Reason, harf notunun hangi kuralla verildiğidir.
type Reason string

// Nedenler.
const (
	ReasonRange          Reason = "RANGE"           // ders başarı puanı ölçeğin aralığına göre
	ReasonAttendanceFail Reason = "ATTENDANCE_FAIL" // devamsız
	ReasonFinalAbsent    Reason = "FINAL_ABSENT"    // finale (ya da bütünlemeye) girmedi
	ReasonFinalBelowMin  Reason = "FINAL_BELOW_MIN" // final barajının altında
	ReasonPassFail       Reason = "PASS_FAIL"       // başarılı/başarısız notlu ders
)

// Result, hesap sonucudur. Complete false ise eksik not vardır ve harf notu verilmez.
type Result struct {
	Complete bool
	Missing  []int  // notu girilmemiş bileşenlerin Input.Components içindeki sırası
	Letter   string // Complete ise
	Reason   Reason
	// Weighted, ders başarı puanıdır (iki ondalığa yarım yukarı yuvarlanmış). Devamsız
	// öğrencide ve notu eksik öğrencide hesaplanmaz.
	Weighted *Points
	// Rounded, aralığa bakılan tam sayı puandır: gösterilen ders başarı puanının (Weighted)
	// yarım yukarı yuvarlanmışı. Öğrencinin gördüğü puanla harf notu hep tutarlıdır: ağırlıklı
	// toplam 49,495 ise puan 49,50 gösterilir ve 50 sayılır.
	Rounded *int64
	// UsedMakeup, bütünleme notunun finalin yerine geçtiğini söyler.
	UsedMakeup bool
}

// Hatalar.
var (
	ErrNoFinal     = errors.New("letter: planda final yok")
	ErrScaleGap    = errors.New("letter: ölçekte bu puanı kapsayan harf yok")
	ErrScaleLetter = errors.New("letter: ölçekte gerekli harf yok")
)

// Compute, harf notunu hesaplar. Kurallar sırasıyla:
//  1. Devamsız öğrenci F1 alır (başarılı/başarısız notlu derste BŞZ).
//  2. Bütünleme notu (puan olarak) girilmişse finalin yerine geçer; bütünlemeye girmeyenin
//     notu final notuyla hesaplanır.
//  3. Dönem içi bileşenlerden ya da finalden biri girilmemişse hesaplanmaz.
//  4. Final (ya da yerine geçen bütünleme) girmedi ise F2; final barajının altındaysa F2
//     (ders başarı puanı yine hesaplanır).
//  5. Ders başarı puanı = Σ puan × ağırlık / 100, iki ondalığa yarım yukarı yuvarlanır;
//     harf notu bu puanın tam sayıya yarım yukarı yuvarlanmışının ölçekteki aralığıdır. Başarılı/başarısız notlu derste geçme puanı ve üstü BŞR,
//     altı BŞZ.
func Compute(scale Scale, params Params, in Input) (Result, error) {
	finalIdx, makeupIdx := -1, -1
	for i, c := range in.Components {
		switch c.Kind {
		case Final:
			finalIdx = i
		case Makeup:
			makeupIdx = i
		}
	}
	if finalIdx < 0 {
		return Result{}, ErrNoFinal
	}

	if in.AttendanceFail {
		if in.PassFail {
			return passFail(scale, false, Result{Complete: true, Reason: ReasonAttendanceFail})
		}
		g, ok := scale.attendanceFail()
		if !ok {
			return Result{}, fmt.Errorf("%w: devamsızlık harfi", ErrScaleLetter)
		}
		return Result{Complete: true, Letter: g.Letter, Reason: ReasonAttendanceFail}, nil
	}

	// Finalin yerine geçen bileşen: puanla girilmiş bütünleme, yoksa final.
	exam := finalIdx
	res := Result{}
	if makeupIdx >= 0 && in.Components[makeupIdx].Score != nil {
		exam, res.UsedMakeup = makeupIdx, true
	}

	// counted: hesaba giren bileşenler; final ile bütünlemeden sadece biri sayılır.
	counted := func(c Component) bool {
		switch c.Kind {
		case Final:
			return !res.UsedMakeup
		case Makeup:
			return res.UsedMakeup
		}
		return true
	}
	for i, c := range in.Components {
		if counted(c) && !c.entered() {
			res.Missing = append(res.Missing, i)
		}
	}
	if len(res.Missing) > 0 {
		return res, nil
	}
	res.Complete = true

	if in.Components[exam].Absent {
		res.Reason = ReasonFinalAbsent
		return failed(scale, in.PassFail, res)
	}

	// Σ puan × ağırlık: birimi 10⁻⁴ puan·yüzde; /10⁴ → yüzde birlik ders başarı puanı.
	var sum int64
	for _, c := range in.Components {
		if counted(c) && c.Score != nil { // girmedi: 0
			sum += int64(*c.Score) * int64(c.Weight)
		}
	}
	weighted := Points(roundDiv(sum, 10_000))
	rounded := roundDiv(int64(weighted), 100)
	res.Weighted, res.Rounded = &weighted, &rounded

	if *in.Components[exam].Score < params.FinalMinScore {
		res.Reason = ReasonFinalBelowMin
		return failed(scale, in.PassFail, res)
	}
	if in.PassFail {
		res.Reason = ReasonPassFail
		return passFail(scale, Points(rounded*100) >= params.PassMinScore, res)
	}
	g, ok := scale.byScore(rounded)
	if !ok {
		return Result{}, fmt.Errorf("%w: %d", ErrScaleGap, rounded)
	}
	res.Letter, res.Reason = g.Letter, ReasonRange
	return res, nil
}

// failed, başarısızlık harfini verir: puanla verilen en düşük harf (F2), başarılı/başarısız
// notlu derste BŞZ.
func failed(scale Scale, passFailCourse bool, res Result) (Result, error) {
	if passFailCourse {
		return passFail(scale, false, res)
	}
	g, ok := scale.lowestRanged()
	if !ok {
		return Result{}, fmt.Errorf("%w: başarısızlık harfi", ErrScaleLetter)
	}
	res.Letter = g.Letter
	return res, nil
}

// passFail, başarılı/başarısız notlu dersin harfini verir: aralığı ve katsayısı olmayan,
// GANO'ya girmeyen harflerden geçen (BŞR) ya da kalan (BŞZ). Muafiyet harfi (AKTS
// kazandırır ama başarılı ders gibi davranır) ayrıca ayırt edilmez: ölçekte BŞR'den sonra gelir.
func passFail(scale Scale, passed bool, res Result) (Result, error) {
	for _, g := range scale {
		if g.Min == nil && !g.CountsInGPA && !g.AttendanceFail && g.Passing == passed {
			res.Letter = g.Letter
			return res, nil
		}
	}
	return Result{}, fmt.Errorf("%w: başarılı/başarısız harfi", ErrScaleLetter)
}

func (s Scale) byScore(rounded int64) (Grade, bool) {
	p := Points(rounded * 100)
	for _, g := range s {
		if g.Min != nil && g.Max != nil && *g.Min <= p && p <= *g.Max {
			return g, true
		}
	}
	return Grade{}, false
}

func (s Scale) attendanceFail() (Grade, bool) {
	for _, g := range s {
		if g.AttendanceFail {
			return g, true
		}
	}
	return Grade{}, false
}

func (s Scale) lowestRanged() (Grade, bool) {
	var (
		low   Grade
		found bool
	)
	for _, g := range s {
		if g.Min != nil && (!found || *g.Min < *low.Min) {
			low, found = g, true
		}
	}
	return low, found && !low.Passing
}

// Rank, harfin ölçekteki sırasıdır (0 en iyi); ölçekte yoksa -1.
func (s Scale) Rank(letter string) int {
	for i, g := range s {
		if g.Letter == letter {
			return i
		}
	}
	return -1
}

// Find, harfi ölçekte bulur.
func (s Scale) Find(letter string) (Grade, bool) {
	if i := s.Rank(letter); i >= 0 {
		return s[i], true
	}
	return Grade{}, false
}

// roundDiv, negatif olmayan a'yı b'ye bölüp yarım yukarı yuvarlar.
func roundDiv(a, b int64) int64 {
	return (2*a + b) / (2 * b)
}
