// Package enrollment, öğrencilerin program kayıtlarını, danışman atamalarını ve kayıt
// engellerini yönetir. İleride ders seçme (registration) de bu modülde olacak.
package enrollment

import (
	"errors"
	"slices"
	"time"
)

// Hatalar.
var (
	ErrNotFound        = errors.New("enrollment: kayıt bulunamadı")
	ErrConflict        = errors.New("enrollment: öğrenci bu programa zaten kayıtlı")
	ErrUnknownProgram  = errors.New("enrollment: program bulunamadı")
	ErrProgramInactive = errors.New("enrollment: program aktif değil")
	ErrNotAnAdvisor    = errors.New("enrollment: personel bu bölümde danışman değil")
	ErrNotActive       = errors.New("enrollment: program kaydı aktif değil")

	ErrInvalidTransition = errors.New("enrollment: bu durum geçişi yapılamaz")
	ErrVersionMismatch   = errors.New("enrollment: kayıt başka biri tarafından değiştirilmiş")
	ErrHoldExists        = errors.New("enrollment: aynı türde aktif engel var")
	ErrHoldReleased      = errors.New("enrollment: engel zaten kaldırılmış")
)

// Kind, program kaydının türüdür.
type Kind string

// Program kaydı türleri.
const (
	KindMajor       Kind = "MAJOR"
	KindDoubleMajor Kind = "DOUBLE_MAJOR"
	KindMinor       Kind = "MINOR"
)

// Valid, değerin tanımlı türlerden biri olup olmadığını söyler.
func (k Kind) Valid() bool {
	switch k {
	case KindMajor, KindDoubleMajor, KindMinor:
		return true
	}
	return false
}

// AdmissionType, öğrencinin programa giriş yoludur.
type AdmissionType string

// Giriş yolları.
const (
	AdmissionOSYS             AdmissionType = "OSYS"
	AdmissionDGS              AdmissionType = "DGS"
	AdmissionYOS              AdmissionType = "YOS"
	AdmissionTransferInternal AdmissionType = "TRANSFER_INTERNAL"
	AdmissionTransferExternal AdmissionType = "TRANSFER_EXTERNAL"
	AdmissionSpecialTalent    AdmissionType = "SPECIAL_TALENT"
	AdmissionExchange         AdmissionType = "EXCHANGE"
)

// Valid, değerin tanımlı giriş yollarından biri olup olmadığını söyler.
func (a AdmissionType) Valid() bool {
	switch a {
	case AdmissionOSYS, AdmissionDGS, AdmissionYOS, AdmissionTransferInternal,
		AdmissionTransferExternal, AdmissionSpecialTalent, AdmissionExchange:
		return true
	}
	return false
}

// Status, program kaydının durumudur.
type Status string

// Kayıt durumları.
const (
	StatusPrep      Status = "PREP"
	StatusActive    Status = "ACTIVE"
	StatusFrozen    Status = "FROZEN"
	StatusSuspended Status = "SUSPENDED"
	StatusGraduated Status = "GRADUATED"
	StatusWithdrawn Status = "WITHDRAWN"
	StatusDismissed Status = "DISMISSED"
)

// Valid, değerin tanımlı durumlardan biri olup olmadığını söyler.
func (s Status) Valid() bool {
	switch s {
	case StatusPrep, StatusActive, StatusFrozen, StatusSuspended, StatusGraduated, StatusWithdrawn, StatusDismissed:
		return true
	}
	return false
}

// Ongoing, kaydın süren (mezun olmamış, ayrılmamış) bir kayıt olup olmadığını söyler.
// Danışman sadece süren kayıtlara atanır.
func (s Status) Ongoing() bool {
	switch s {
	case StatusPrep, StatusActive, StatusFrozen, StatusSuspended:
		return true
	}
	return false
}

// CanBecome, kaydın to durumuna geçip geçemeyeceğini söyler. Mezuniyet, ayrılma ve
// ilişik kesme kesindir: bu durumlardan çıkılmaz.
func (s Status) CanBecome(to Status) bool {
	switch s {
	case StatusPrep:
		return to == StatusActive || to == StatusWithdrawn || to == StatusDismissed
	case StatusActive:
		return to == StatusFrozen || to == StatusSuspended || to == StatusGraduated ||
			to == StatusWithdrawn || to == StatusDismissed
	case StatusFrozen, StatusSuspended:
		return to == StatusActive || to == StatusWithdrawn || to == StatusDismissed
	}
	return false
}

// ProgramRef, program kaydında gösterilen program özetidir. Bölüm ve birim
// kimlikleri yetki kapsamı kontrolü için de kullanılır.
type ProgramRef struct {
	ID             string
	Code           string
	NameTR         string
	DepartmentID   string
	DepartmentName string
	FacultyID      string
	FacultyName    string

	DurationSemesters int // normal süre
	MaxDurationYears  int // azami süre (hazırlık hariç)
}

// AdvisorRef, program kaydının danışmanıdır.
type AdvisorRef struct {
	AssignmentID string
	StaffID      string
	StaffNo      string
	Title        string // ör. "Prof. Dr.", akademik unvanı yoksa boş
	FirstName    string
	LastName     string
	Since        time.Time
}

// StudentProgram, öğrencinin bir programdaki kaydıdır.
type StudentProgram struct {
	ID              string
	StudentID       string
	StudentNo       string
	FirstName       string
	LastName        string
	Program         ProgramRef
	Kind            Kind
	AdmissionType   AdmissionType
	AdmissionYear   int
	AdmittedOn      time.Time
	Status          Status
	ClassLevel      int
	CurrentSemester int
	GPA             *float64
	EarnedECTS      float64
	Advisor         *AdvisorRef    // danışman atanmamışsa nil
	Curriculum      *CurriculumRef // izlediği müfredat sürümü; bağlanmamışsa nil
	Version         int
}

// MaxSemesters, kaydın azami süresidir (yarıyıl). Hazırlık sınıfı süreye sayılmaz.
func (sp StudentProgram) MaxSemesters() int {
	return sp.Program.MaxDurationYears * 2
}

// BeyondMaxDuration, öğrencinin azami süreyi aşıp aşmadığını söyler.
func (sp StudentProgram) BeyondMaxDuration() bool {
	return sp.CurrentSemester > sp.MaxSemesters()
}

// CurriculumRef, program kaydının izlediği müfredat sürümünün özetidir.
type CurriculumRef struct {
	ID     string
	NameTR string
	NameEN string
}

// Student, bir öğrenci ve bütün program kayıtlarıdır.
type Student struct {
	ID        string
	PersonID  string
	UserID    string // hesabı yoksa boş
	StudentNo string
	FirstName string
	LastName  string
	Email     string
	Programs  []StudentProgram
}

// AdvisorHistory, bir program kaydının danışman geçmişindeki bir satırdır.
type AdvisorHistory struct {
	AdvisorRef
	Until      *time.Time
	AssignedBy string
	Reason     string
}

// EligibleAdvisor, bir bölümde danışman olarak atanabilecek öğretim elemanıdır.
type EligibleAdvisor struct {
	StaffID     string
	StaffNo     string
	Title       string
	FirstName   string
	LastName    string
	ActiveCount int // şu anki danışmanlık sayısı: yükü dengelemek için
}

// StatusChange, program kaydının durum geçmişindeki bir satırdır.
type StatusChange struct {
	From        *Status
	To          Status
	Reason      string
	DecisionRef string
	ChangedBy   string // değiştirenin adı; sistem değişikliğinde boş
	ChangedAt   time.Time
}

// HoldType, kayıt engelinin türüdür.
type HoldType string

// Engel türleri.
const (
	HoldTuitionDebt          HoldType = "TUITION_DEBT"
	HoldDisciplineSuspension HoldType = "DISCIPLINE_SUSPENSION"
	HoldMissingDocument      HoldType = "MISSING_DOCUMENT"
	HoldLibraryDebt          HoldType = "LIBRARY_DEBT"
)

// Valid, değerin tanımlı engel türlerinden biri olup olmadığını söyler.
func (t HoldType) Valid() bool {
	switch t {
	case HoldTuitionDebt, HoldDisciplineSuspension, HoldMissingDocument, HoldLibraryDebt:
		return true
	}
	return false
}

// DefaultBlocks, türün varsayılan olarak kapattığı işlemlerdir.
func (t HoldType) DefaultBlocks() []HoldBlock {
	switch t {
	case HoldTuitionDebt:
		return []HoldBlock{BlockRegistration, BlockDocument}
	case HoldDisciplineSuspension:
		return []HoldBlock{BlockRegistration}
	case HoldMissingDocument:
		return []HoldBlock{BlockGraduation}
	case HoldLibraryDebt:
		return []HoldBlock{BlockDocument, BlockGraduation}
	}
	return nil
}

// HoldBlock, engelin kapattığı işlemdir.
type HoldBlock string

// Engellenebilen işlemler.
const (
	BlockRegistration HoldBlock = "REGISTRATION"
	BlockDocument     HoldBlock = "DOCUMENT"
	BlockGraduation   HoldBlock = "GRADUATION"
)

// Valid, değerin tanımlı işlemlerden biri olup olmadığını söyler.
func (b HoldBlock) Valid() bool {
	return b == BlockRegistration || b == BlockDocument || b == BlockGraduation
}

// Hold, bir program kaydına konmuş engeldir.
type Hold struct {
	ID               string
	StudentProgramID string
	StudentID        string
	StudentNo        string
	FirstName        string
	LastName         string
	Program          ProgramRef // sadece kimlik, kod, ad ve kapsam alanları dolu
	Type             HoldType
	Blocks           []HoldBlock
	Reason           string
	PlacedBy         string // koyanın adı
	PlacedAt         time.Time
	ReleasedBy       string
	ReleasedAt       *time.Time
	ReleaseReason    string
}

// Active, engelin hâlâ geçerli olup olmadığını söyler.
func (h Hold) Active() bool {
	return h.ReleasedAt == nil
}

// BlocksAction, engelin verilen işlemi kapatıp kapatmadığını söyler.
func (h Hold) BlocksAction(b HoldBlock) bool {
	return h.Active() && slices.Contains(h.Blocks, b)
}
