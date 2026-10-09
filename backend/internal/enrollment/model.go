// Package enrollment, öğrencilerin program kayıtlarını ve danışman atamalarını yönetir.
// İleride ders seçme (registration) de bu modülde olacak.
package enrollment

import (
	"errors"
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
