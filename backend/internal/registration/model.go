package registration

import (
	"errors"
	"time"

	"github.com/CAPELLAX02/agora/backend/internal/registration/rules"
)

// Hatalar.
var (
	ErrNotFound         = errors.New("registration: kayıt bulunamadı")
	ErrSectionNotFound  = errors.New("registration: şube bulunamadı")
	ErrSectionNotOpen   = errors.New("registration: şube ders seçmeye açık değil")
	ErrNoTerm           = errors.New("registration: aktif dönem yok")
	ErrWindowClosed     = errors.New("registration: işlem penceresi kapalı")
	ErrLocked           = errors.New("registration: kayıt danışman onayında ya da onaylanmış")
	ErrSectionFull      = errors.New("registration: şube kontenjanı dolu")
	ErrQuotaFull        = errors.New("registration: programa ayrılan kontenjan dolu")
	ErrNotForProgram    = errors.New("registration: şube bu programa kontenjan ayırmamış")
	ErrVersionMismatch  = errors.New("registration: kayıt başka bir işlemle değişmiş")
	ErrInvalidState     = errors.New("registration: kayıt bu işlem için uygun durumda değil")
	ErrNotAdvisor       = errors.New("registration: öğrencinin danışmanı değilsiniz")
	ErrSameSection      = errors.New("registration: ders zaten bu şubede")
	ErrItemNotFound     = errors.New("registration: sepette böyle bir ders yok")
	ErrReasonRequired   = errors.New("registration: gerekçe zorunlu")
	ErrNotOwner         = errors.New("registration: kayıt öğrencinin kendisine ait değil")
	ErrCourseMismatch   = errors.New("registration: şube aynı dersin şubesi değil")
	ErrExtensionInvalid = errors.New("registration: ek süre geçersiz")
	ErrGradesEntered    = errors.New("registration: kayıttaki derslere not girilmiş")
)

// RuleError, kural motorunun engellediği bir işlemdir. Violations en az bir hata içerir.
type RuleError struct {
	Violations []rules.Violation
}

func (e *RuleError) Error() string {
	return "registration: ders seçme kuralı ihlali"
}

// Status, sepetin (dönem kaydının) durumudur.
type Status string

// Kayıt durumları. NotStarted veritabanında yoktur: öğrencinin henüz sepeti olmadığını söyler.
const (
	StatusNotStarted Status = "NOT_STARTED"
	StatusDraft      Status = "DRAFT"
	StatusSubmitted  Status = "SUBMITTED"
	StatusApproved   Status = "APPROVED"
	StatusRejected   Status = "REJECTED"
)

// Editable, öğrencinin sepeti değiştirebileceği bir durum olup olmadığını söyler.
// Reddedilen sepet düzenlenince yeniden taslak olur.
func (s Status) Editable() bool {
	return s == StatusNotStarted || s == StatusDraft || s == StatusRejected
}

// ItemKind, sepet satırının türüdür.
type ItemKind string

// Satır türleri.
const (
	KindNew                 ItemKind = "NEW"                  // ilk kez alınıyor
	KindRepeatFailed        ItemKind = "REPEAT_FAILED"        // kalınan ders (ya da eşdeğeri) tekrar
	KindRepeatUpgrade       ItemKind = "REPEAT_UPGRADE"       // geçilen ders not yükseltmek için
	KindElectiveReplacement ItemKind = "ELECTIVE_REPLACEMENT" // kalınan seçmelinin yerine havuzdan başka ders
)

// Slot, şubenin haftalık programındaki bir oturumdur.
type Slot struct {
	Day       int
	Start     string // "09:00"
	End       string // "10:50"
	Kind      string // THEORY, PRACTICE, LAB
	Classroom string // "MUH-A/A104"; çevrim içinde boş
}

// Section, ders seçmede gösterilen bir şubedir.
type Section struct {
	ID          string
	OfferingID  string
	CourseID    string
	CourseCode  string
	NameTR      string
	NameEN      string
	ECTS        float64
	Credit      float64
	Code        string // şube kodu: "1"
	Capacity    int
	Enrolled    int
	QuotaMode   string // OPEN, RESERVED
	QuotaLeft   *int   // RESERVED şubede öğrencinin programına kalan kontenjan; ayrılmamışsa 0
	Language    string
	Instructors []string // unvanlı adlar, sorumlu önce
	Slots       []Slot
}

// Label, şubenin "COM3035-1" biçimindeki adıdır.
func (s Section) Label() string {
	return s.CourseCode + "-" + s.Code
}

// Available, öğrencinin bu şubede yer bulup bulamayacağını söyler.
func (s Section) Available() bool {
	if s.Enrolled >= s.Capacity {
		return false
	}
	return s.QuotaLeft == nil || *s.QuotaLeft > 0
}

// Item, sepetteki bir şubedir.
type Item struct {
	ID               string
	Section          Section
	Kind             ItemKind
	CurriculumItemID string
	ECTS             float64
	Source           string
	Status           string
	AddedAt          time.Time
}

// Registration, öğrencinin bir dönemdeki ders seçme kaydıdır (sepet).
type Registration struct {
	ID               string // henüz yoksa boş
	StudentProgramID string
	TermID           string
	TermCode         string
	Status           Status
	TotalECTS        float64
	MaxECTSAllowed   *float64
	GPASnapshot      *float64
	SubmittedAt      *time.Time
	DecidedAt        *time.Time
	DecidedBy        string // karar verenin adı
	DecisionNote     string
	ExtensionUntil   *time.Time
	ExtensionReason  string
	Version          int
	Items            []Item
}

// Event, kaydın olay günlüğündeki bir satırdır.
type Event struct {
	ID         int64
	Type       string
	Actor      string // yapanın adı
	Payload    map[string]any
	OccurredAt time.Time
}
