// Package rules, ders seçmenin saf (veritabanına ve saate bağımlı olmayan) iş kurallarıdır:
// öğrencinin müfredatına göre ders durumu (geçti, kaldı, devam ediyor, alınmadı) ve sepet
// kuralları (AKTS üst sınırı, 1. sınıf zorunlu dersleri, alttan ders önceliği, ön koşul,
// çakışma, azami süre...). Girdiler servis katmanında veritabanından toplanır; burada
// sadece karar verilir. Bu yüzden kurallar tablo güdümlü birim testleriyle sınanır.
package rules

import (
	"cmp"
	"slices"
)

// Attempt, öğrencinin bir dersi bir dönemde alışıdır.
type Attempt struct {
	CourseID       string
	TermOrder      int  // dönem sırası: büyük olan daha yeni
	Current        bool // bu dönem alınıyor, henüz notu yok
	Completed      bool // notu verildi
	Passing        bool // harf notu geçer not
	AttendanceFail bool // devamsızlıktan kaldı (F1)
	Letter         string
	ECTS           float64
}

// newer, a denemesinin b'den daha yeni olup olmadığını söyler. Bu dönem alınan ders
// her zaman en yenidir.
func newer(a, b Attempt) bool {
	if a.Current != b.Current {
		return a.Current
	}
	return a.TermOrder > b.TermOrder
}

// Line, müfredatın bir satırıdır: bir ders ya da bir seçmeli yuvası.
type Line struct {
	ID         string
	Semester   int
	CourseID   string // ders satırı
	GroupID    string // seçmeli yuvası: dersler bu havuzdan seçilir
	SlotCount  int    // seçmeli yuvasında alınacak ders sayısı
	Compulsory bool
	ECTS       float64
}

// IsSlot, satırın seçmeli yuvası olup olmadığını söyler.
func (l Line) IsSlot() bool {
	return l.GroupID != ""
}

// Equivalence, eski ve yeni ders kodu arasındaki eşdeğerliktir. Eski dersi geçen yeni
// dersten sayılır; çift yönlüyse yeni dersi geçen eski koddan da sayılır (eski müfredatı
// izleyen öğrenci, artık açılmayan dersin yerine yenisini alır).
type Equivalence struct {
	NewID         string
	OldID         string
	Bidirectional bool
}

// Catalog, eşdeğerlikler ve seçmeli havuzlarıdır.
type Catalog struct {
	satisfiers map[string][]string // ders → onun yerine sayılan dersler (kendisi hariç)
	groups     map[string][]string // havuz → dersleri
	memberOf   map[string][]string // ders → üyesi olduğu havuzlar
}

// NewCatalog, eşdeğerliklerden ve havuz üyeliklerinden bir Catalog oluşturur.
func NewCatalog(eqs []Equivalence, groups map[string][]string) *Catalog {
	c := &Catalog{satisfiers: map[string][]string{}, groups: groups, memberOf: map[string][]string{}}
	for _, e := range eqs {
		c.satisfiers[e.NewID] = append(c.satisfiers[e.NewID], e.OldID)
		if e.Bidirectional {
			c.satisfiers[e.OldID] = append(c.satisfiers[e.OldID], e.NewID)
		}
	}
	for g, courses := range groups {
		for _, id := range courses {
			c.memberOf[id] = append(c.memberOf[id], g)
		}
	}
	return c
}

// Satisfiers, dersin kendisi ve onun yerine sayılan derslerdir.
func (c *Catalog) Satisfiers(courseID string) []string {
	return append([]string{courseID}, c.satisfiers[courseID]...)
}

// InGroup, dersin havuzda olup olmadığını söyler.
func (c *Catalog) InGroup(groupID, courseID string) bool {
	return slices.Contains(c.groups[groupID], courseID)
}

// Group, havuzun dersleridir.
func (c *Catalog) Group(groupID string) []string {
	return c.groups[groupID]
}

// Status, müfredat satırının durumudur.
type Status string

// Satır durumları.
const (
	Passed     Status = "PASSED"      // geçti (yuva: gereken bütün dersler geçildi)
	Failed     Status = "FAILED"      // son denemede kaldı (yuva: havuzdan kalınan ders var, yuva dolmadı)
	InProgress Status = "IN_PROGRESS" // bu dönem alınıyor
	Partial    Status = "PARTIAL"     // yuvanın bir kısmı dolu
	NotTaken   Status = "NOT_TAKEN"
)

// LineProgress, bir müfredat satırının öğrenci için durumudur.
type LineProgress struct {
	Line   Line
	Status Status
	// Ders satırı: son deneme (dersin kendisi ya da eşdeğeri). Alınmadıysa nil.
	Last *Attempt
	// Seçmeli yuvası: yuvayı dolduran geçilmiş dersler ve bu dönem alınanlar.
	Filled     []Attempt
	InProgress []Attempt
	Remaining  int // yuvada henüz karşılanmamış ders sayısı
}

// Open, satırın hâlâ karşılanması gerekip gerekmediğini söyler.
func (p LineProgress) Open() bool {
	return p.Status != Passed && p.Status != InProgress
}

// Progress, öğrencinin müfredatına göre ders durumudur.
type Progress struct {
	Lines []LineProgress
	Extra []Attempt // hiçbir satırla eşleşmeyen derslerin son denemeleri
}

// ComputeProgress, öğrencinin denemelerini müfredat satırlarına eşler. Her dersin son
// denemesi geçerlidir (tekrar alınan derste son not geçerli). Ders satırları önce,
// yarıyıl sırasıyla eşlenir; seçmeli yuvaları kalan derslerden, havuzdaki geçilmiş
// derslerle yarıyıl sırasıyla doldurulur. Bir deneme tek bir satırı karşılar.
func ComputeProgress(lines []Line, attempts []Attempt, cat *Catalog) Progress {
	latest := map[string]Attempt{}
	for _, a := range attempts {
		if cur, ok := latest[a.CourseID]; !ok || newer(a, cur) {
			latest[a.CourseID] = a
		}
	}
	used := map[string]bool{}

	ordered := slices.Clone(lines)
	slices.SortStableFunc(ordered, func(a, b Line) int { return cmp.Compare(a.Semester, b.Semester) })
	result := make(map[string]LineProgress, len(lines))

	for _, l := range ordered {
		if l.IsSlot() {
			continue
		}
		p := LineProgress{Line: l, Status: NotTaken}
		for _, id := range cat.Satisfiers(l.CourseID) {
			a, ok := latest[id]
			if !ok || used[id] {
				continue
			}
			used[id] = true
			if p.Last == nil || newer(a, *p.Last) {
				p.Last = &a
			}
		}
		if p.Last != nil {
			p.Status = attemptStatus(*p.Last)
		}
		result[l.ID] = p
	}

	for _, l := range ordered {
		if !l.IsSlot() {
			continue
		}
		var passed, current []Attempt
		failed := false
		for _, id := range cat.Group(l.GroupID) {
			a, ok := latest[id]
			if !ok || used[id] {
				continue
			}
			switch {
			case a.Current:
				current = append(current, a)
			case a.Passing:
				passed = append(passed, a)
			default:
				// Kalınan seçmeli tekrar alınmak zorunda değil (havuzdan başka ders seçilebilir)
				// ama müfredat dışı da sayılmaz.
				failed = true
				used[id] = true
			}
		}
		byTerm := func(a, b Attempt) int { return cmp.Compare(a.TermOrder, b.TermOrder) }
		slices.SortStableFunc(passed, byTerm)

		p := LineProgress{Line: l}
		for _, a := range passed {
			if len(p.Filled) == l.SlotCount {
				break
			}
			p.Filled = append(p.Filled, a)
			used[a.CourseID] = true
		}
		for _, a := range current {
			if len(p.Filled)+len(p.InProgress) == l.SlotCount {
				break
			}
			p.InProgress = append(p.InProgress, a)
			used[a.CourseID] = true
		}
		p.Remaining = l.SlotCount - len(p.Filled) - len(p.InProgress)
		switch {
		case len(p.Filled) == l.SlotCount:
			p.Status = Passed
		case p.Remaining == 0:
			p.Status = InProgress
		case failed && len(p.Filled)+len(p.InProgress) == 0:
			p.Status = Failed
		case len(p.Filled)+len(p.InProgress) > 0:
			p.Status = Partial
		default:
			p.Status = NotTaken
		}
		result[l.ID] = p
	}

	var progress Progress
	for _, l := range lines {
		progress.Lines = append(progress.Lines, result[l.ID])
	}
	for id, a := range latest {
		if !used[id] {
			progress.Extra = append(progress.Extra, a)
		}
	}
	slices.SortFunc(progress.Extra, func(a, b Attempt) int {
		return cmp.Or(cmp.Compare(a.TermOrder, b.TermOrder), cmp.Compare(a.CourseID, b.CourseID))
	})
	return progress
}

func attemptStatus(a Attempt) Status {
	switch {
	case a.Current:
		return InProgress
	case a.Passing:
		return Passed
	default:
		return Failed
	}
}
