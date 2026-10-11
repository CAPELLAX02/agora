package rules

import (
	"cmp"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
)

// Severity, kural ihlalinin ağırlığıdır. Hata sepete eklemeyi ya da gönderimi engeller,
// uyarı sadece bilgi verir (danışman da görür).
type Severity string

// Ağırlıklar.
const (
	Error   Severity = "ERROR"
	Warning Severity = "WARNING"
)

// Code, kural ihlalinin makine okunur kodudur.
type Code string

// Kural kodları.
const (
	CodeHold                Code = "HOLD"
	CodeProgramNotActive    Code = "PROGRAM_NOT_ACTIVE"
	CodeNoCurriculum        Code = "NO_CURRICULUM"
	CodeEmptyBasket         Code = "EMPTY_BASKET"
	CodeDuplicateCourse     Code = "DUPLICATE_COURSE"
	CodeNotInCurriculum     Code = "NOT_IN_CURRICULUM"
	CodePrerequisite        Code = "PREREQUISITE"
	CodeScheduleConflict    Code = "SCHEDULE_CONFLICT"
	CodeECTSLimit           Code = "ECTS_LIMIT"
	CodeFirstYearCompulsory Code = "FIRST_YEAR_COMPULSORY"
	CodeRepeatPriority      Code = "REPEAT_PRIORITY"
	CodeMaxDuration         Code = "MAX_DURATION"
	CodeRepeatUpgrade       Code = "REPEAT_UPGRADE"
	CodeElectiveExceeded    Code = "ELECTIVE_SLOT_EXCEEDED"
)

// Violation, bir kural ihlalidir. Message Türkçe açıklamadır; arayüz Code ve Params ile
// kendi dilinde gösterebilir.
type Violation struct {
	Code      Code
	Severity  Severity
	CourseIDs []string       // ilgili dersler
	Params    map[string]any // ör. limit, toplam, eksik ders kodları
	Message   string
}

// Student, kuralların öğrenciyle ilgili girdileridir.
type Student struct {
	Active        bool     // program kaydı ACTIVE mi
	StatusLabel   string   // aktif değilse mesajda gösterilecek durum (ör. "dondurulmuş")
	Semester      int      // hazırlık hariç kaçıncı yarıyıl
	MaxSemesters  int      // azami süre
	GPA           *float64 // GANO; henüz notu yoksa nil
	BlockingHolds []string // ders seçmeyi kapatan aktif engellerin adları
	HasCurriculum bool
}

// ECTSStep, GANO'ya göre AKTS üst sınırı basamağıdır: GANO ≤ MaxGPA ise en çok ECTS.
type ECTSStep struct {
	MaxGPA float64
	ECTS   float64
}

// Params, yönetmelik parametreleridir (veri olarak: curriculum.regulation_parameters).
type Params struct {
	ECTSSteps                  []ECTSStep
	DefaultECTS                float64 // GANO'su olmayan öğrenci için (dönem yükü)
	MaxCoursesAfterMaxDuration int
}

// ECTSLimit, öğrencinin bu dönem alabileceği en fazla AKTS'dir.
func (p Params) ECTSLimit(gpa *float64) float64 {
	if gpa == nil {
		return p.DefaultECTS
	}
	steps := slices.Clone(p.ECTSSteps)
	slices.SortFunc(steps, func(a, b ECTSStep) int { return cmp.Compare(a.MaxGPA, b.MaxGPA) })
	for _, s := range steps {
		if *gpa <= s.MaxGPA+1e-9 {
			return s.ECTS
		}
	}
	if len(steps) > 0 {
		return steps[len(steps)-1].ECTS
	}
	return p.DefaultECTS
}

// Requirement, ön koşulun türüdür.
type Requirement string

// Ön koşul türleri.
const (
	RequirePassed   Requirement = "PASSED"   // dersi geçmiş olmak
	RequireAttended Requirement = "ATTENDED" // derse devam etmiş olmak (devamsızlıktan kalmamış)
)

// Prerequisite, bir ön koşul satırıdır. Aynı gruptakiler VEYA, gruplar VE ile bağlanır.
type Prerequisite struct {
	CourseID    string
	Requirement Requirement
	Group       int
}

// Slot, haftalık programda bir oturumdur. Saatler gece yarısından itibaren dakikadır, [Start, End).
type Slot struct {
	Day   int // ISO: 1 pazartesi
	Start int
	End   int
}

func (s Slot) overlaps(o Slot) bool {
	return s.Day == o.Day && s.Start < o.End && o.Start < s.End
}

func (s Slot) String() string {
	return fmt.Sprintf("%s %02d:%02d-%02d:%02d", dayNames[s.Day], s.Start/60, s.Start%60, s.End/60, s.End%60)
}

var dayNames = [...]string{"", "Pazartesi", "Salı", "Çarşamba", "Perşembe", "Cuma", "Cumartesi", "Pazar"}

// Item, sepetteki (ya da sepete eklenecek) bir şubedir.
type Item struct {
	CourseID    string
	CourseCode  string
	SectionCode string // ör. "COM3035-1"
	ECTS        float64
	Slots       []Slot
}

// Input, kuralların bütün girdileridir.
type Input struct {
	Student  Student
	Params   Params
	Lines    []Line
	Attempts []Attempt
	Catalog  *Catalog
	Prereqs  map[string][]Prerequisite // ders → ön koşulları
	Offered  map[string]bool           // bu dönem ders seçmeye açık dersler
	Codes    map[string]string         // ders → kod (mesajlar için)
	Basket   []Item

	progress *Progress
}

// Progress, girdinin ders durumudur (bir kez hesaplanır).
func (in *Input) Progress() Progress {
	if in.progress == nil {
		p := ComputeProgress(in.Lines, in.Attempts, in.Catalog)
		in.progress = &p
	}
	return *in.progress
}

func (in *Input) code(courseID string) string {
	if c, ok := in.Codes[courseID]; ok {
		return c
	}
	return courseID
}

// Evaluation, sepetin kurallara göre durumudur.
type Evaluation struct {
	Violations []Violation
	TotalECTS  float64
	ECTSLimit  float64
}

// Blocking, gönderimi engelleyen bir hata olup olmadığını söyler.
func (e Evaluation) Blocking() bool {
	return slices.ContainsFunc(e.Violations, func(v Violation) bool { return v.Severity == Error })
}

// CheckAdd, cand şubesini sepete eklemenin kurallarını denetler. Hata içeren sonuç
// eklemeyi engeller. Gönderim anı kuralları (1. sınıf, alttan önceliği) burada değil,
// Evaluate'te denetlenir: öğrenci sepetini hangi sırayla doldurursa doldursun.
func CheckAdd(in *Input, cand Item) []Violation {
	if vs := eligibility(in); len(vs) > 0 {
		return vs
	}
	var vs []Violation
	if slices.ContainsFunc(in.Basket, func(it Item) bool { return it.CourseID == cand.CourseID }) {
		return []Violation{{
			Code: CodeDuplicateCourse, Severity: Error, CourseIDs: []string{cand.CourseID},
			Message: cand.CourseCode + " dersinin bir şubesi zaten sepetinizde; şube değiştirebilirsiniz.",
		}}
	}
	if v, ok := notInCurriculum(in, cand); ok {
		return []Violation{v}
	}
	if v, ok := prerequisite(in, cand); ok {
		vs = append(vs, v)
	}
	for _, it := range in.Basket {
		if v, ok := conflict(in, cand, it); ok {
			vs = append(vs, v)
		}
	}
	limit := in.Params.ECTSLimit(in.Student.GPA)
	if total := basketECTS(in.Basket) + cand.ECTS; total > limit+1e-9 {
		vs = append(vs, Violation{
			Code: CodeECTSLimit, Severity: Error, CourseIDs: []string{cand.CourseID},
			Params:  map[string]any{"limit": limit, "total": total},
			Message: fmt.Sprintf("AKTS üst sınırınız %s; bu dersle %s olur.", num(limit), num(total)),
		})
	}
	if v, ok := maxDuration(in, len(in.Basket)+1, &cand); ok {
		vs = append(vs, v)
	}
	vs = append(vs, advisories(in, cand, in.Basket)...)
	return vs
}

// Evaluate, sepetin bütün kurallarını denetler: ekleme anı kuralları (sepet sonradan
// geçersizleşmiş olabilir: GANO ya da parametre değişti) ve gönderim anı kuralları.
func Evaluate(in *Input) Evaluation {
	ev := Evaluation{TotalECTS: basketECTS(in.Basket), ECTSLimit: in.Params.ECTSLimit(in.Student.GPA)}
	if vs := eligibility(in); len(vs) > 0 {
		ev.Violations = vs
		return ev
	}
	if len(in.Basket) == 0 {
		ev.Violations = append(ev.Violations, Violation{Code: CodeEmptyBasket, Severity: Error, Message: "Sepetiniz boş."})
		return ev
	}

	for i, it := range in.Basket {
		if v, ok := notInCurriculum(in, it); ok {
			ev.Violations = append(ev.Violations, v)
			continue
		}
		if v, ok := prerequisite(in, it); ok {
			ev.Violations = append(ev.Violations, v)
		}
		for _, other := range in.Basket[i+1:] {
			if v, ok := conflict(in, it, other); ok {
				ev.Violations = append(ev.Violations, v)
			}
		}
	}
	if ev.TotalECTS > ev.ECTSLimit+1e-9 {
		ev.Violations = append(ev.Violations, Violation{
			Code: CodeECTSLimit, Severity: Error,
			Params:  map[string]any{"limit": ev.ECTSLimit, "total": ev.TotalECTS},
			Message: fmt.Sprintf("AKTS üst sınırınız %s, sepetiniz %s AKTS.", num(ev.ECTSLimit), num(ev.TotalECTS)),
		})
	}
	if v, ok := maxDuration(in, len(in.Basket), nil); ok {
		ev.Violations = append(ev.Violations, v)
	}
	if v, ok := firstYear(in); ok {
		ev.Violations = append(ev.Violations, v)
	}
	if v, ok := repeatPriority(in); ok {
		ev.Violations = append(ev.Violations, v)
	}
	for i, it := range in.Basket {
		others := slices.Concat(in.Basket[:i], in.Basket[i+1:])
		ev.Violations = append(ev.Violations, advisories(in, it, others)...)
	}
	return ev
}

// --- Kurallar ------------------------------------------------------------------------

// eligibility: kayıt engeli, program kaydı durumu ve müfredat bağlantısı.
func eligibility(in *Input) []Violation {
	var vs []Violation
	if len(in.Student.BlockingHolds) > 0 {
		vs = append(vs, Violation{
			Code: CodeHold, Severity: Error, Params: map[string]any{"holds": in.Student.BlockingHolds},
			Message: "Ders seçmenizi engelleyen bir kaydınız var (" + strings.Join(in.Student.BlockingHolds, ", ") +
				"). Engel kaldırılmadan ders seçemezsiniz.",
		})
	}
	if !in.Student.Active {
		vs = append(vs, Violation{
			Code: CodeProgramNotActive, Severity: Error, Params: map[string]any{"status": in.Student.StatusLabel},
			Message: "Program kaydınız aktif değil (" + in.Student.StatusLabel + "); ders seçemezsiniz.",
		})
	}
	if !in.Student.HasCurriculum {
		vs = append(vs, Violation{
			Code: CodeNoCurriculum, Severity: Error,
			Message: "Bir ders planına bağlı değilsiniz; öğrenci işlerine başvurun.",
		})
	}
	return vs
}

// lineFor, dersin karşıladığı müfredat satırıdır: önce ders satırları, sonra seçmeli
// yuvaları. Dersin karşıladığı bir satır yoksa ok false döner.
func lineFor(in *Input, courseID string) (LineProgress, bool) {
	p := in.Progress()
	for _, lp := range p.Lines {
		if !lp.Line.IsSlot() && slices.Contains(in.Catalog.Satisfiers(lp.Line.CourseID), courseID) {
			return lp, true
		}
	}
	for _, lp := range p.Lines {
		if lp.Line.IsSlot() && in.Catalog.InGroup(lp.Line.GroupID, courseID) {
			return lp, true
		}
	}
	return LineProgress{}, false
}

func notInCurriculum(in *Input, it Item) (Violation, bool) {
	if _, ok := lineFor(in, it.CourseID); ok {
		return Violation{}, false
	}
	return Violation{
		Code: CodeNotInCurriculum, Severity: Error, CourseIDs: []string{it.CourseID},
		Message: it.CourseCode + " ders planınızda (zorunlu ders, eşdeğeri ya da seçmeli havuzu olarak) yok.",
	}, true
}

// prerequisite: her grup (VE) içinde en az bir ders (VEYA) sağlanmalı. Ön koşul dersinin
// eşdeğeri de sayılır; bu dönem alınan ders sayılmaz.
func prerequisite(in *Input, it Item) (Violation, bool) {
	prereqs := in.Prereqs[it.CourseID]
	if len(prereqs) == 0 {
		return Violation{}, false
	}
	groups := map[int][]Prerequisite{}
	for _, p := range prereqs {
		groups[p.Group] = append(groups[p.Group], p)
	}
	var missing []string
	var missingIDs []string
	for _, g := range slices.Sorted(maps.Keys(groups)) {
		ok := slices.ContainsFunc(groups[g], func(p Prerequisite) bool { return satisfied(in, p) })
		if ok {
			continue
		}
		var alts []string
		for _, p := range groups[g] {
			label := in.code(p.CourseID)
			if p.Requirement == RequireAttended {
				label += " (devam)"
			}
			alts = append(alts, label)
			missingIDs = append(missingIDs, p.CourseID)
		}
		missing = append(missing, strings.Join(alts, " ya da "))
	}
	if len(missing) == 0 {
		return Violation{}, false
	}
	return Violation{
		Code: CodePrerequisite, Severity: Error, CourseIDs: append([]string{it.CourseID}, missingIDs...),
		Params:  map[string]any{"missing": missing},
		Message: it.CourseCode + " için ön koşul sağlanmıyor: " + strings.Join(missing, "; ") + ".",
	}, true
}

func satisfied(in *Input, p Prerequisite) bool {
	ids := in.Catalog.Satisfiers(p.CourseID)
	return slices.ContainsFunc(in.Attempts, func(a Attempt) bool {
		if !a.Completed || !slices.Contains(ids, a.CourseID) {
			return false
		}
		if p.Requirement == RequireAttended {
			return !a.AttendanceFail
		}
		return a.Passing
	})
}

// attendanceMet, dersin (ya da eşdeğerinin) devam şartının daha önce sağlandığını söyler:
// notu verilmiş ve devamsızlıktan kalınmamış bir denemesi var. Böyle bir derste derse
// devam zorunlu olmadığından program çakışması engel değil, uyarıdır.
func attendanceMet(in *Input, courseID string) bool {
	lp, ok := lineFor(in, courseID)
	ids := []string{courseID}
	if ok && !lp.Line.IsSlot() {
		ids = in.Catalog.Satisfiers(lp.Line.CourseID)
	}
	return slices.ContainsFunc(in.Attempts, func(a Attempt) bool {
		return a.Completed && !a.AttendanceFail && slices.Contains(ids, a.CourseID)
	})
}

func conflict(in *Input, a, b Item) (Violation, bool) {
	for _, sa := range a.Slots {
		for _, sb := range b.Slots {
			if !sa.overlaps(sb) {
				continue
			}
			v := Violation{
				Code: CodeScheduleConflict, Severity: Error, CourseIDs: []string{a.CourseID, b.CourseID},
				Params:  map[string]any{"sections": []string{a.SectionCode, b.SectionCode}, "slot": sa.String()},
				Message: fmt.Sprintf("%s ile %s çakışıyor (%s).", a.SectionCode, b.SectionCode, sa.String()),
			}
			for _, it := range []Item{a, b} {
				if attendanceMet(in, it.CourseID) {
					v.Severity = Warning
					v.Message += " " + it.CourseCode + " dersinin devam şartını daha önce sağladığınız için alınabilir."
					break
				}
			}
			return v, true
		}
	}
	return Violation{}, false
}

// RemainingCourses, mezuniyet için kalan ders sayısıdır: geçilmemiş zorunlu dersler ve
// seçmeli yuvalarında kalan ders sayıları. Bu dönem alınanlar kalan sayılmaz.
func RemainingCourses(p Progress) int {
	n := 0
	for _, lp := range p.Lines {
		switch {
		case lp.Line.IsSlot():
			n += lp.Remaining
		case lp.Open():
			n++
		}
	}
	return n
}

// maxDuration: azami süreyi aşan öğrenci, mezuniyet için kalan ders sayısı sınırı
// aşmıyorsa sadece kalan derslerini (en çok sınır kadar) alabilir.
func maxDuration(in *Input, count int, cand *Item) (Violation, bool) {
	s := in.Student
	if s.MaxSemesters == 0 || s.Semester <= s.MaxSemesters {
		return Violation{}, false
	}
	limit := in.Params.MaxCoursesAfterMaxDuration
	remaining := RemainingCourses(in.Progress())
	switch {
	case remaining > limit:
		return Violation{
			Code: CodeMaxDuration, Severity: Error, Params: map[string]any{"remaining": remaining, "limit": limit},
			Message: fmt.Sprintf("Azami öğrenim süresini aştınız. Mezuniyet için %d dersiniz kaldığından "+
				"(en çok %d) ders seçemezsiniz; öğrenci işlerine başvurun.", remaining, limit),
		}, true
	case count > remaining:
		return Violation{
			Code: CodeMaxDuration, Severity: Error, Params: map[string]any{"remaining": remaining, "limit": limit},
			Message: fmt.Sprintf("Azami süre sonunda sadece mezuniyet için kalan %d dersinizi alabilirsiniz.", remaining),
		}, true
	case cand != nil:
		if lp, ok := lineFor(in, cand.CourseID); ok && !lp.Open() {
			return Violation{
				Code: CodeMaxDuration, Severity: Error, CourseIDs: []string{cand.CourseID},
				Message: "Azami süre sonunda sadece mezuniyet için kalan derslerinizi alabilirsiniz.",
			}, true
		}
	}
	return Violation{}, false
}

// firstYear: 1. sınıftaki (ilk iki yarıyıl) öğrenci, yarıyılının açılan ve ön koşulu
// sağlanan bütün zorunlu derslerini almak zorundadır.
func firstYear(in *Input) (Violation, bool) {
	if in.Student.Semester > 2 {
		return Violation{}, false
	}
	var missing []string
	for _, lp := range in.Progress().Lines {
		l := lp.Line
		if l.IsSlot() || !l.Compulsory || l.Semester != in.Student.Semester || !lp.Open() {
			continue
		}
		if !offered(in, l) || inBasket(in, l) {
			continue
		}
		missing = append(missing, l.CourseID)
	}
	if len(missing) == 0 {
		return Violation{}, false
	}
	return Violation{
		Code: CodeFirstYearCompulsory, Severity: Error, CourseIDs: missing,
		Params:  map[string]any{"courses": codes(in, missing)},
		Message: "1. sınıfta yarıyılınızın bütün zorunlu derslerini almalısınız: " + strings.Join(codes(in, missing), ", ") + ".",
	}, true
}

// repeatPriority: alt yarıyıllardan kalınan ya da hiç alınmamış zorunlu dersler, açılmışsa
// (kendisi ya da eşdeğeri) ve ön koşulu sağlanıyorsa öncelikle alınmalıdır.
func repeatPriority(in *Input) (Violation, bool) {
	if in.Student.Semester <= 1 {
		return Violation{}, false
	}
	var missing []string
	for _, lp := range in.Progress().Lines {
		l := lp.Line
		if l.IsSlot() || !l.Compulsory || l.Semester >= in.Student.Semester || !lp.Open() {
			continue
		}
		if !offered(in, l) || inBasket(in, l) {
			continue
		}
		missing = append(missing, l.CourseID)
	}
	if len(missing) == 0 {
		return Violation{}, false
	}
	return Violation{
		Code: CodeRepeatPriority, Severity: Error, CourseIDs: missing,
		Params: map[string]any{"courses": codes(in, missing)},
		Message: "Alt yarıyıllardan kaldığınız ya da almadığınız dersleri öncelikle almalısınız: " +
			strings.Join(codes(in, missing), ", ") + ".",
	}, true
}

// advisories: hata olmayan bilgilendirmeler. Geçilmiş dersi not yükseltmek için almak ve
// bir seçmeli havuzundan gerekenden fazla ders almak.
func advisories(in *Input, it Item, others []Item) []Violation {
	lp, ok := lineFor(in, it.CourseID)
	if !ok {
		return nil
	}
	var vs []Violation
	if !lp.Line.IsSlot() && lp.Status == Passed {
		letter := ""
		if lp.Last != nil {
			letter = " (" + lp.Last.Letter + ")"
		}
		vs = append(vs, Violation{
			Code: CodeRepeatUpgrade, Severity: Warning, CourseIDs: []string{it.CourseID},
			Message: it.CourseCode + " dersini geçtiniz" + letter + ". Not yükseltmek için alırsanız son aldığınız not geçerli olur.",
		})
	}
	if lp.Line.IsSlot() {
		taken := 0
		for _, o := range others {
			if in.Catalog.InGroup(lp.Line.GroupID, o.CourseID) {
				taken++
			}
		}
		if taken >= groupRemaining(in, lp.Line.GroupID) {
			vs = append(vs, Violation{
				Code: CodeElectiveExceeded, Severity: Warning, CourseIDs: []string{it.CourseID},
				Message: it.CourseCode + " ile bu seçmeli havuzundan ders planınızın gerektirdiğinden fazla ders almış olursunuz.",
			})
		}
	}
	return vs
}

// groupRemaining, bir havuzun bütün yuvalarında kalan ders sayısıdır.
func groupRemaining(in *Input, groupID string) int {
	n := 0
	for _, lp := range in.Progress().Lines {
		if lp.Line.GroupID == groupID {
			n += lp.Remaining
		}
	}
	return n
}

// --- Yardımcılar -------------------------------------------------------------------------

// offered, satırın bu dönem alınabilir olup olmadığını söyler: kendisi ya da eşdeğeri
// açılmış ve ön koşulu sağlanıyor.
func offered(in *Input, l Line) bool {
	return slices.ContainsFunc(in.Catalog.Satisfiers(l.CourseID), func(id string) bool {
		if !in.Offered[id] {
			return false
		}
		_, unmet := prerequisite(in, Item{CourseID: id, CourseCode: in.code(id)})
		return !unmet
	})
}

func inBasket(in *Input, l Line) bool {
	ids := in.Catalog.Satisfiers(l.CourseID)
	return slices.ContainsFunc(in.Basket, func(it Item) bool { return slices.Contains(ids, it.CourseID) })
}

func basketECTS(items []Item) float64 {
	total := 0.0
	for _, it := range items {
		total += it.ECTS
	}
	return math.Round(total*10) / 10
}

func codes(in *Input, ids []string) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = in.code(id)
	}
	return out
}

func num(v float64) string {
	if v == math.Trunc(v) {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%.1f", v)
}

// Kind, bir dersin öğrencinin geçmişine göre alınış türüdür.
type Kind string

// Alınış türleri.
const (
	KindNew                 Kind = "NEW"                  // ilk kez
	KindRepeatFailed        Kind = "REPEAT_FAILED"        // kalınan dersin (ya da eşdeğerinin) tekrarı
	KindRepeatUpgrade       Kind = "REPEAT_UPGRADE"       // geçilen dersi not yükseltmek için
	KindElectiveReplacement Kind = "ELECTIVE_REPLACEMENT" // kalınan seçmelinin yerine havuzdan başka ders
)

// Classify, dersin karşıladığı müfredat satırını ve alınış türünü döndürür. Ders hiçbir
// satırı karşılamıyorsa ok false döner.
func Classify(in *Input, courseID string) (line Line, kind Kind, ok bool) {
	lp, ok := lineFor(in, courseID)
	if !ok {
		return Line{}, "", false
	}
	if !lp.Line.IsSlot() {
		switch {
		case lp.Last == nil || lp.Status == InProgress:
			return lp.Line, KindNew, true
		case lp.Status == Passed:
			return lp.Line, KindRepeatUpgrade, true
		default:
			return lp.Line, KindRepeatFailed, true
		}
	}
	var own *Attempt
	groupFailed := false
	for _, a := range in.Attempts {
		if !a.Completed || !in.Catalog.InGroup(lp.Line.GroupID, a.CourseID) {
			continue
		}
		if a.CourseID == courseID && (own == nil || newer(a, *own)) {
			own = &a
		}
		if !a.Passing {
			groupFailed = true
		}
	}
	switch {
	case own != nil && own.Passing:
		return lp.Line, KindRepeatUpgrade, true
	case own != nil:
		return lp.Line, KindRepeatFailed, true
	case groupFailed:
		return lp.Line, KindElectiveReplacement, true
	}
	return lp.Line, KindNew, true
}

// UnmetPrerequisite, dersin ön koşulu sağlanmıyorsa ihlali döndürür.
func UnmetPrerequisite(in *Input, it Item) (Violation, bool) {
	return prerequisite(in, it)
}
