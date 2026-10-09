// Package curriculum, ders kataloğunu, seçmeli ders gruplarını, versiyonlu müfredatları
// ve not ölçeğini yönetir.
package curriculum

import "errors"

// Hatalar.
var (
	ErrNotFound          = errors.New("curriculum: kayıt bulunamadı")
	ErrConflict          = errors.New("curriculum: aynı kodla kayıt zaten var")
	ErrVersionMismatch   = errors.New("curriculum: kayıt bu arada değişti")
	ErrUnknownReference  = errors.New("curriculum: başvurulan kayıt bulunamadı")
	ErrPrerequisiteCycle = errors.New("curriculum: ön koşullar döngü oluşturuyor")
	ErrDuplicate         = errors.New("curriculum: ilişki zaten var")
)

// CourseKind, dersin türüdür.
type CourseKind string

// Ders türleri.
const (
	KindRegular    CourseKind = "REGULAR"
	KindNonCredit  CourseKind = "NON_CREDIT" // ör. Üniversite Yaşamına Uyum: ortalamaya girmez
	KindInternship CourseKind = "INTERNSHIP"
	KindProject    CourseKind = "PROJECT"
	KindPrep       CourseKind = "PREP"
	KindActivity   CourseKind = "ACTIVITY" // ör. Bilimsel, Kültürel ve Sosyal Etkinlikler
)

// Valid, değerin tanımlı ders türlerinden biri olup olmadığını söyler.
func (k CourseKind) Valid() bool {
	switch k {
	case KindRegular, KindNonCredit, KindInternship, KindProject, KindPrep, KindActivity:
		return true
	}
	return false
}

// GradingMode, dersin nasıl notlandırıldığıdır.
type GradingMode string

// Notlandırma biçimleri.
const (
	GradingLetter   GradingMode = "LETTER"
	GradingPassFail GradingMode = "PASS_FAIL" // BŞR / BŞZ
)

// Valid, değerin tanımlı notlandırma biçimlerinden biri olup olmadığını söyler.
func (g GradingMode) Valid() bool {
	return g == GradingLetter || g == GradingPassFail
}

// Requirement, ön koşulun ne istediğidir.
type Requirement string

// Ön koşul gereklilikleri.
const (
	RequirePassed   Requirement = "PASSED"   // dersten geçmiş olmak
	RequireAttended Requirement = "ATTENDED" // dersi almış ve devam şartını sağlamış olmak
)

// Valid, değerin tanımlı gerekliliklerden biri olup olmadığını söyler.
func (r Requirement) Valid() bool {
	return r == RequirePassed || r == RequireAttended
}

// GroupKind, seçmeli ders grubunun türüdür.
type GroupKind string

// Seçmeli grup türleri.
const (
	GroupTechnical         GroupKind = "TECHNICAL"
	GroupUniversityGeneral GroupKind = "UNIVERSITY_GENERAL"
	GroupPedagogical       GroupKind = "PEDAGOGICAL"
	GroupSocial            GroupKind = "SOCIAL"
	GroupFree              GroupKind = "FREE"
)

// Valid, değerin tanımlı grup türlerinden biri olup olmadığını söyler.
func (k GroupKind) Valid() bool {
	switch k {
	case GroupTechnical, GroupUniversityGeneral, GroupPedagogical, GroupSocial, GroupFree:
		return true
	}
	return false
}

// DepartmentRef, bir bölümün özetidir. FacultyID yetki kontrolü içindir.
type DepartmentRef struct {
	ID        string
	Code      string
	NameTR    string
	NameEN    string
	FacultyID string
}

// Course, katalogdaki bir derstir.
type Course struct {
	ID               string
	Code             string
	OwnerDepartment  *DepartmentRef
	NameTR           string
	NameEN           string
	TheoryHours      int
	PracticeHours    int
	NationalCredit   float64
	ECTS             float64
	Language         string
	Kind             CourseKind
	GradingMode      GradingMode
	DescriptionTR    string
	DescriptionEN    string
	LearningOutcomes []string
	IsActive         bool
	Version          int
}

// CourseRef, başka bir kayıtta gösterilen ders özetidir.
type CourseRef struct {
	ID     string
	Code   string
	NameTR string
	NameEN string
	ECTS   float64
}

// Prerequisite, bir dersin ön koşul satırıdır. Aynı GroupNo içindekiler VEYA, farklı
// gruplar VE ile bağlanır.
type Prerequisite struct {
	Course      CourseRef
	Requirement Requirement
	GroupNo     int
}

// EquivalenceRelation, eşdeğerliğin bu derse göre yönüdür.
type EquivalenceRelation string

// Eşdeğerlik yönleri: course_id yeni, equivalent_course_id eski koddur.
const (
	Replaces   EquivalenceRelation = "REPLACES"    // bu ders, diğerinin yerini alır (diğeri eski kod)
	ReplacedBy EquivalenceRelation = "REPLACED_BY" // bu dersin yerini diğeri almıştır
)

// Equivalence, bir dersin başka bir dersle eşdeğerliğidir.
type Equivalence struct {
	ID              string
	Relation        EquivalenceRelation
	Course          CourseRef // diğer ders
	IsBidirectional bool
	ValidFromYear   *int
	Note            string
}

// GroupRef, bir seçmeli grubun özetidir.
type GroupRef struct {
	ID     string
	Code   string
	NameTR string
	NameEN string
	Kind   GroupKind
}

// CourseDetail, ders ve ilişkileridir.
type CourseDetail struct {
	Course
	Prerequisites  []Prerequisite
	RequiredBy     []CourseRef // bu dersi ön koşul olarak isteyen dersler
	Equivalences   []Equivalence
	ElectiveGroups []GroupRef
}

// ElectiveGroup, bir seçmeli ders grubudur (havuz).
type ElectiveGroup struct {
	ID              string
	Code            string
	NameTR          string
	NameEN          string
	OwnerDepartment *DepartmentRef
	Kind            GroupKind
	IsActive        bool
	Version         int
	CourseCount     int
}
