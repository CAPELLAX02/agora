package registration

import (
	"cmp"
	"context"
	"slices"

	"github.com/CAPELLAX02/agora/backend/internal/enrollment"
	"github.com/CAPELLAX02/agora/backend/internal/registration/rules"
)

// Category, alınabilir dersin öğrenci için kategorisidir.
type Category string

// Kategoriler.
const (
	CategoryRepeat   Category = "REPEAT"   // alttan: alt yarıyıldan kalınan ya da alınmamış zorunlu ders
	CategoryCurrent  Category = "CURRENT"  // içinde bulunulan yarıyılın zorunlu dersi
	CategoryElective Category = "ELECTIVE" // dolmamış bir seçmeli yuvası için havuzdan ders
	CategoryUpper    Category = "UPPER"    // üst yarıyılın zorunlu dersi
	CategoryUpgrade  Category = "UPGRADE"  // geçilmiş ders: not yükseltme
)

// Offer, öğrencinin bu dönem alabileceği bir derstir.
type Offer struct {
	CourseID     string
	Code         string
	NameTR       string
	NameEN       string
	ECTS         float64
	Category     Category
	Kind         rules.Kind
	Line         Line
	Status       rules.Status // müfredat satırının durumu
	LastLetter   string
	Prerequisite *rules.Violation // ön koşul sağlanmıyorsa
	Sections     []Section
	BasketItemID string // ders sepetteyse sepet satırı
	Remaining    int    // seçmeli yuvasında kalan ders sayısı
}

// Offers, öğrencinin dönemde alabileceği dersleri müfredatına ve geçmişine göre
// kategorilere ayırarak döndürür: önce alttan dersler, sonra dönem dersleri, seçmeliler,
// üst yarıyıl dersleri ve not yükseltme. Eski koddan kalınan ders, açılan yeni eşdeğeriyle
// görünür. Bir ders tek bir kez (ilk eşleştiği satırla) listelenir.
func (s *Service) Offers(ctx context.Context, sp enrollment.StudentProgram, termID string) ([]Offer, rules.Evaluation, error) {
	reg, err := NewRepository(s.pool).Registration(ctx, sp.ID, sp.Program.ID, termID)
	if err != nil {
		return nil, rules.Evaluation{}, err
	}
	b, err := s.loadWith(ctx, s.pool, sp, termID, reg.Items, true)
	if err != nil {
		return nil, rules.Evaluation{}, err
	}
	inBasket := map[string]string{}
	for _, it := range reg.Items {
		inBasket[it.Section.CourseID] = it.ID
	}
	progress := b.input.Progress()

	var offers []Offer
	seen := map[string]bool{}
	add := func(courseID string, l Line, lp rules.LineProgress, cat Category) {
		secs := b.sections[courseID]
		if len(secs) == 0 || seen[courseID] {
			return
		}
		seen[courseID] = true
		first := secs[0]
		o := Offer{
			CourseID: courseID, Code: first.CourseCode, NameTR: first.NameTR, NameEN: first.NameEN, ECTS: first.ECTS,
			Category: cat, Line: l, Status: lp.Status, Sections: secs, BasketItemID: inBasket[courseID],
			Remaining: lp.Remaining,
		}
		if lp.Last != nil {
			o.LastLetter = lp.Last.Letter
		}
		_, o.Kind, _ = rules.Classify(b.input, courseID)
		if v, unmet := rules.UnmetPrerequisite(b.input, ruleItem(first)); unmet {
			o.Prerequisite = &v
		}
		offers = append(offers, o)
	}

	semester := sp.CurrentSemester
	for i, lp := range progress.Lines {
		l := b.lines[i]
		if l.IsSlot() || lp.Status == rules.InProgress {
			continue
		}
		cat := CategoryCurrent
		switch {
		case lp.Status == rules.Passed:
			cat = CategoryUpgrade
		case l.Semester < semester && l.Compulsory:
			cat = CategoryRepeat
		case l.Semester > semester:
			cat = CategoryUpper
		}
		if cat == CategoryUpgrade {
			continue // not yükseltme en sona
		}
		for _, id := range b.input.Catalog.Satisfiers(l.CourseID) {
			add(id, l, lp, cat)
		}
	}
	for i, lp := range progress.Lines {
		l := b.lines[i]
		if !l.IsSlot() || lp.Remaining == 0 {
			continue
		}
		for _, id := range b.input.Catalog.Group(l.GroupID) {
			if passedCourse(b.input.Attempts, id) {
				continue
			}
			add(id, l, lp, CategoryElective)
		}
	}
	// Not yükseltme: katsayılı harf notuyla geçilen dersler (başarılı/başarısız notlu ders yükseltilmez).
	for i, lp := range progress.Lines {
		l := b.lines[i]
		if !l.IsSlot() && lp.Status == rules.Passed && lp.Last != nil && lp.Last.Graded {
			for _, id := range b.input.Catalog.Satisfiers(l.CourseID) {
				add(id, l, lp, CategoryUpgrade)
			}
		}
	}

	order := []Category{CategoryRepeat, CategoryCurrent, CategoryElective, CategoryUpper, CategoryUpgrade}
	slices.SortStableFunc(offers, func(a, b Offer) int {
		if c := slices.Index(order, a.Category) - slices.Index(order, b.Category); c != 0 {
			return c
		}
		if a.Line.Semester != b.Line.Semester {
			return a.Line.Semester - b.Line.Semester
		}
		if a.Line.Position != b.Line.Position {
			return a.Line.Position - b.Line.Position
		}
		return cmp.Compare(a.Code, b.Code)
	})
	return offers, rules.Evaluate(b.input), nil
}

func passedCourse(attempts []rules.Attempt, courseID string) bool {
	var last *rules.Attempt
	for _, a := range attempts {
		if a.CourseID == courseID && (last == nil || a.TermOrder > last.TermOrder) {
			last = &a
		}
	}
	return last != nil && last.Passing
}
