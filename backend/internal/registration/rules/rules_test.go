package rules

import (
	"slices"
	"strings"
	"testing"
)

// Kurgusal bir program: üç yarıyıllık çekirdek, 5. yarıyılda iki dersli bir teknik
// seçmeli yuvası, 7. yarıyılda proje ve iki dersli bir yuva daha. MAT1'in yerini
// MAT1N almış (çift yönlü eşdeğerlik), VY'nin ön koşulu PRG1'e devam.
var lines = []Line{
	{ID: "l-mat1", Semester: 1, CourseID: "MAT1", Compulsory: true, ECTS: 6},
	{ID: "l-fiz1", Semester: 1, CourseID: "FIZ1", Compulsory: true, ECTS: 5},
	{ID: "l-prg1", Semester: 1, CourseID: "PRG1", Compulsory: true, ECTS: 7},
	{ID: "l-mat2", Semester: 2, CourseID: "MAT2", Compulsory: true, ECTS: 6},
	{ID: "l-vy", Semester: 3, CourseID: "VY", Compulsory: true, ECTS: 6},
	{ID: "l-alg", Semester: 3, CourseID: "ALG", Compulsory: true, ECTS: 5},
	{ID: "l-os", Semester: 5, CourseID: "OS", Compulsory: true, ECTS: 5},
	{ID: "l-te5", Semester: 5, GroupID: "TE", SlotCount: 2, ECTS: 8},
	{ID: "l-proj", Semester: 7, CourseID: "PROJ", Compulsory: true, ECTS: 6},
	{ID: "l-te7", Semester: 7, GroupID: "TE", SlotCount: 2, ECTS: 12},
}

func catalog() *Catalog {
	return NewCatalog(
		[]Equivalence{{NewID: "MAT1N", OldID: "MAT1", Bidirectional: true}},
		map[string][]string{"TE": {"E1", "E2", "E3", "E4", "E5"}},
	)
}

func passed(course string, term int, letter string) Attempt {
	return Attempt{CourseID: course, TermOrder: term, Completed: true, Passing: true, Letter: letter}
}

func failed(course string, term int) Attempt {
	return Attempt{CourseID: course, TermOrder: term, Completed: true, Letter: "F2"}
}

func absent(course string, term int) Attempt {
	return Attempt{CourseID: course, TermOrder: term, Completed: true, AttendanceFail: true, Letter: "F1"}
}

func current(course string) Attempt {
	return Attempt{CourseID: course, Current: true}
}

func line(p Progress, id string) LineProgress {
	for _, lp := range p.Lines {
		if lp.Line.ID == id {
			return lp
		}
	}
	panic("satır yok: " + id)
}

func TestComputeProgress(t *testing.T) {
	attempts := []Attempt{
		failed("MAT1", 1), passed("MAT1N", 3, "C1"), // kaldı, eşdeğeriyle geçti
		absent("PRG1", 1),                          // devamsızlıktan kaldı
		passed("FIZ1", 1, "B2"), failed("FIZ1", 3), // geçti, not yükseltmede kaldı: son not geçerli
		passed("E1", 5, "A1"), passed("E2", 5, "B1"), passed("E3", 7, "C2"), current("E4"),
		failed("E5", 5),
		passed("X1", 2, "A1"), // müfredat dışı
		current("ALG"),
	}
	p := ComputeProgress(lines, attempts, catalog())

	cases := []struct {
		id     string
		status Status
		letter string
	}{
		{"l-mat1", Passed, "C1"},
		{"l-prg1", Failed, "F1"},
		{"l-fiz1", Failed, "F2"},
		{"l-alg", InProgress, ""},
		{"l-vy", NotTaken, ""},
	}
	for _, c := range cases {
		lp := line(p, c.id)
		if lp.Status != c.status {
			t.Errorf("%s durumu = %s, want %s", c.id, lp.Status, c.status)
		}
		if c.letter != "" && (lp.Last == nil || lp.Last.Letter != c.letter) {
			t.Errorf("%s son notu = %+v, want %s", c.id, lp.Last, c.letter)
		}
	}

	// 5. yarıyıl yuvası ilk iki geçilen dersle dolar, 7. yarıyıl yuvası sonrakilerle.
	te5, te7 := line(p, "l-te5"), line(p, "l-te7")
	if te5.Status != Passed || len(te5.Filled) != 2 || te5.Filled[0].CourseID != "E1" {
		t.Errorf("TE5 = %+v", te5)
	}
	if te7.Status != InProgress || len(te7.Filled) != 1 || len(te7.InProgress) != 1 || te7.Remaining != 0 {
		t.Errorf("TE7 = %+v", te7)
	}
	if len(p.Extra) != 1 || p.Extra[0].CourseID != "X1" {
		t.Errorf("müfredat dışı = %+v", p.Extra)
	}
}

func TestElectiveSlotStatuses(t *testing.T) {
	cat := catalog()
	slot := []Line{{ID: "s", Semester: 5, GroupID: "TE", SlotCount: 2}}
	for name, c := range map[string]struct {
		attempts  []Attempt
		status    Status
		remaining int
	}{
		"boş":            {nil, NotTaken, 2},
		"kalınan":        {[]Attempt{failed("E1", 1)}, Failed, 2},
		"yarım":          {[]Attempt{passed("E1", 1, "A1")}, Partial, 1},
		"yarım ve devam": {[]Attempt{passed("E1", 1, "A1"), current("E2")}, InProgress, 0},
		"dolu":           {[]Attempt{passed("E1", 1, "A1"), passed("E2", 3, "B1"), passed("E3", 3, "B1")}, Passed, 0},
	} {
		lp := ComputeProgress(slot, c.attempts, cat).Lines[0]
		if lp.Status != c.status || lp.Remaining != c.remaining {
			t.Errorf("%s: %s/%d, want %s/%d", name, lp.Status, lp.Remaining, c.status, c.remaining)
		}
	}
}

func TestECTSLimit(t *testing.T) {
	p := Params{
		ECTSSteps:   []ECTSStep{{MaxGPA: 2.99, ECTS: 40}, {MaxGPA: 1.99, ECTS: 30}, {MaxGPA: 4, ECTS: 45}},
		DefaultECTS: 30,
	}
	gpa := func(v float64) *float64 { return &v }
	for _, c := range []struct {
		gpa  *float64
		want float64
	}{
		{nil, 30}, {gpa(0), 30}, {gpa(1.99), 30}, {gpa(2), 40}, {gpa(2.99), 40}, {gpa(3), 45}, {gpa(4), 45},
	} {
		if got := p.ECTSLimit(c.gpa); got != c.want {
			t.Errorf("GANO %v → %v, want %v", c.gpa, got, c.want)
		}
	}
}

// fixture, 5. yarıyıldaki, GANO'su 2.50 (40 AKTS) bir öğrencidir. OS'tan F2 ile kaldı
// (devam şartını sağladı), PRG1'e devamsızlıktan kaldı.
func fixture() *Input {
	gpa := 2.5
	return &Input{
		Student: Student{Active: true, Semester: 5, MaxSemesters: 14, GPA: &gpa, HasCurriculum: true},
		Params: Params{
			ECTSSteps:   []ECTSStep{{MaxGPA: 1.99, ECTS: 30}, {MaxGPA: 2.99, ECTS: 40}, {MaxGPA: 4, ECTS: 45}},
			DefaultECTS: 30, MaxCoursesAfterMaxDuration: 5,
		},
		Lines: lines,
		Attempts: []Attempt{
			passed("MAT1", 1, "B1"), passed("FIZ1", 1, "C1"), absent("PRG1", 1), passed("MAT2", 2, "B2"),
			passed("ALG", 3, "A2"), failed("OS", 3),
		},
		Catalog: catalog(),
		Prereqs: map[string][]Prerequisite{"VY": {{CourseID: "PRG1", Requirement: RequireAttended, Group: 1}}},
		Offered: map[string]bool{"PRG1": true, "VY": true, "OS": true, "E1": true, "E2": true, "E3": true, "PROJ": true, "MAT1N": true},
		Codes:   map[string]string{"PRG1": "PRG1", "VY": "VY", "OS": "OS"},
	}
}

func item(course string, ects float64, slots ...Slot) Item {
	return Item{CourseID: course, CourseCode: course, SectionCode: course + "-1", ECTS: ects, Slots: slots}
}

var (
	mon9  = Slot{Day: 1, Start: 9 * 60, End: 11*60 + 50}
	mon10 = Slot{Day: 1, Start: 10 * 60, End: 12*60 + 50}
	tue9  = Slot{Day: 2, Start: 9 * 60, End: 11*60 + 50}
)

func codesOf(vs []Violation) []string {
	var out []string
	for _, v := range vs {
		out = append(out, string(v.Code)+":"+string(v.Severity))
	}
	return out
}

func TestCheckAdd(t *testing.T) {
	for name, c := range map[string]struct {
		setup func(in *Input)
		cand  Item
		want  []string
	}{
		"temiz ekleme": {cand: item("PRG1", 7, mon9), want: nil},
		"engel": {
			setup: func(in *Input) { in.Student.BlockingHolds = []string{"Katkı payı borcu"} },
			cand:  item("PRG1", 7), want: []string{"HOLD:ERROR"},
		},
		"dondurulmuş kayıt": {
			setup: func(in *Input) { in.Student.Active, in.Student.StatusLabel = false, "dondurulmuş" },
			cand:  item("PRG1", 7), want: []string{"PROGRAM_NOT_ACTIVE:ERROR"},
		},
		"aynı ders iki kez": {
			setup: func(in *Input) { in.Basket = []Item{item("PRG1", 7)} },
			cand:  item("PRG1", 7), want: []string{"DUPLICATE_COURSE:ERROR"},
		},
		"müfredat dışı": {cand: item("X9", 3), want: []string{"NOT_IN_CURRICULUM:ERROR"}},
		"eşdeğer ders müfredattan sayılır": {
			cand: item("MAT1N", 6), want: []string{"REPEAT_UPGRADE:WARNING"},
		},
		"ön koşul: devamsızlıktan kalınan derse devam şartı": {
			cand: item("VY", 6), want: []string{"PREREQUISITE:ERROR"},
		},
		"ön koşul sağlanmış": {
			setup: func(in *Input) { in.Attempts = append(in.Attempts, failed("PRG1", 3)) },
			cand:  item("VY", 6), want: nil,
		},
		"çakışma": {
			setup: func(in *Input) { in.Basket = []Item{item("PRG1", 7, mon9)} },
			cand:  item("E1", 4, mon10), want: []string{"SCHEDULE_CONFLICT:ERROR"},
		},
		"devam şartı sağlanmış tekrar derste çakışma uyarıdır": {
			setup: func(in *Input) { in.Basket = []Item{item("E1", 4, mon10)} },
			cand:  item("OS", 5, mon9), want: []string{"SCHEDULE_CONFLICT:WARNING"},
		},
		"farklı günler çakışmaz": {
			setup: func(in *Input) { in.Basket = []Item{item("PRG1", 7, mon9)} },
			cand:  item("E1", 4, tue9), want: nil,
		},
		"AKTS üst sınırı": {
			setup: func(in *Input) {
				in.Basket = []Item{item("PRG1", 7), item("OS", 5), item("E1", 4), item("E2", 4), item("PROJ", 6), item("VY", 6)}
			},
			cand: item("E3", 9), want: []string{"ECTS_LIMIT:ERROR"},
		},
		"geçilmiş ders: not yükseltme uyarısı": {cand: item("ALG", 5), want: []string{"REPEAT_UPGRADE:WARNING"}},
		"azami süre: sadece kalan dersler": {
			setup: func(in *Input) {
				in.Student.Semester = 15
				in.Attempts = append(in.Attempts, passed("PRG1", 3, "C2"), passed("VY", 3, "C2"),
					passed("E1", 5, "A1"), passed("E2", 5, "A1"), passed("E3", 7, "A1"))
			},
			cand: item("ALG", 5), want: []string{"MAX_DURATION:ERROR", "REPEAT_UPGRADE:WARNING"},
		},
		"azami süre: kalan dersten fazlası": {
			setup: func(in *Input) {
				in.Student.Semester = 15
				in.Attempts = append(in.Attempts, passed("PRG1", 3, "C2"), passed("VY", 3, "C2"),
					passed("E1", 5, "A1"), passed("E2", 5, "A1"), passed("E3", 7, "A1"), passed("E4", 7, "A1"))
				in.Basket = []Item{item("OS", 5), item("PROJ", 6)}
			},
			cand: item("E5", 4), want: []string{"MAX_DURATION:ERROR", "ELECTIVE_SLOT_EXCEEDED:WARNING"},
		},
		"seçmeli havuzu aşımı": {
			setup: func(in *Input) { in.Basket = []Item{item("E1", 4), item("E2", 4), item("E3", 4), item("E4", 4)} },
			cand:  item("E5", 4), want: []string{"ELECTIVE_SLOT_EXCEEDED:WARNING"},
		},
	} {
		in := fixture()
		if c.setup != nil {
			c.setup(in)
		}
		if got := codesOf(CheckAdd(in, c.cand)); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", name, got, c.want)
		}
	}
}

func TestEvaluate(t *testing.T) {
	for name, c := range map[string]struct {
		setup func(in *Input)
		want  []string
	}{
		"boş sepet": {want: []string{"EMPTY_BASKET:ERROR"}},
		"alttan dersler eksik": {
			setup: func(in *Input) { in.Basket = []Item{item("E1", 4), item("E2", 4)} },
			want:  []string{"REPEAT_PRIORITY:ERROR"},
		},
		"alttan dersler sepette": {
			setup: func(in *Input) { in.Basket = []Item{item("PRG1", 7), item("OS", 5), item("E1", 4)} },
			want:  nil,
		},
		"1. sınıf zorunlu dersleri": {
			setup: func(in *Input) {
				in.Student.Semester, in.Student.GPA, in.Attempts = 1, nil, nil
				in.Offered = map[string]bool{"MAT1N": true, "FIZ1": true, "PRG1": true}
				in.Basket = []Item{item("MAT1N", 6), item("PRG1", 7)}
			},
			want: []string{"FIRST_YEAR_COMPULSORY:ERROR"},
		},
		"azami süre: kalan ders sınırın üstünde": {
			setup: func(in *Input) { in.Student.Semester = 15; in.Basket = []Item{item("PRG1", 7)} },
			want:  []string{"MAX_DURATION:ERROR", "REPEAT_PRIORITY:ERROR"},
		},
		"azami süre: kalan dersler alınabilir": {
			setup: func(in *Input) {
				in.Student.Semester = 15
				in.Attempts = append(in.Attempts, passed("PRG1", 3, "C2"), passed("VY", 3, "C2"),
					passed("E1", 5, "A1"), passed("E2", 5, "A1"), passed("E3", 7, "A1"))
				in.Basket = []Item{item("OS", 5), item("PROJ", 6), item("E4", 6)}
			},
			want: nil,
		},
		"çakışan şubeler": {
			setup: func(in *Input) { in.Basket = []Item{item("PRG1", 7, mon9), item("OS", 5, tue9), item("E1", 4, mon10)} },
			want:  []string{"SCHEDULE_CONFLICT:ERROR"},
		},
		"engel diğer kuralları gizler": {
			setup: func(in *Input) { in.Student.BlockingHolds = []string{"Disiplin cezası"} },
			want:  []string{"HOLD:ERROR"},
		},
	} {
		in := fixture()
		if c.setup != nil {
			c.setup(in)
		}
		ev := Evaluate(in)
		if got := codesOf(ev.Violations); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", name, got, c.want)
		}
		if ev.Blocking() != slices.ContainsFunc(c.want, func(s string) bool { return strings.HasSuffix(s, ":ERROR") }) {
			t.Errorf("%s: Blocking() = %v", name, ev.Blocking())
		}
	}
}

func TestMessages(t *testing.T) {
	in := fixture()
	in.Basket = []Item{item("E1", 4, mon10)}
	vs := CheckAdd(in, item("OS", 5, mon9))
	if len(vs) != 1 || vs[0].Message != "OS-1 ile E1-1 çakışıyor (Pazartesi 09:00-11:50). OS dersinin devam şartını daha önce sağladığınız için alınabilir." {
		t.Errorf("mesaj = %q", vs[0].Message)
	}
	vs = CheckAdd(fixture(), item("VY", 6))
	if vs[0].Message != "VY için ön koşul sağlanmıyor: PRG1 (devam)." {
		t.Errorf("mesaj = %q", vs[0].Message)
	}
}

func TestClassify(t *testing.T) {
	in := fixture()
	in.Attempts = append(in.Attempts, failed("E5", 3), passed("E1", 3, "B1"))
	for course, want := range map[string]Kind{
		"MAT1N": KindRepeatUpgrade, // MAT1'i geçti; eşdeğerini almak not yükseltmedir
		"PRG1":  KindRepeatFailed,  // devamsızlıktan kaldı
		"OS":    KindRepeatFailed,
		"VY":    KindNew,
		"E1":    KindRepeatUpgrade,
		"E5":    KindRepeatFailed,
		"E2":    KindElectiveReplacement, // E5'ten kaldı, havuzdan başka ders
	} {
		_, kind, ok := Classify(in, course)
		if !ok || kind != want {
			t.Errorf("%s = %s (%v), want %s", course, kind, ok, want)
		}
	}
	if _, _, ok := Classify(in, "X9"); ok {
		t.Error("müfredat dışı ders sınıflanmamalı")
	}
}
