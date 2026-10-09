// Package academic, akademik takvimi yönetir: akademik yıllar, dönemler ve tipli zaman
// pencereleri (ders seçme, danışman onayı, not girişi ...). Kayıt ve not kuralları bir
// işlemin yapılıp yapılamayacağını pencere motoruna (Window) sorar.
package academic

import (
	"errors"
	"strconv"
	"time"
)

// Hatalar.
var (
	ErrNotFound         = errors.New("academic: kayıt bulunamadı")
	ErrConflict         = errors.New("academic: kayıt zaten var")
	ErrVersionMismatch  = errors.New("academic: kayıt bu arada değişti")
	ErrEventOverlap     = errors.New("academic: aynı kapsamda aynı türden çakışan bir olay var")
	ErrUnknownEventType = errors.New("academic: bilinmeyen olay türü")
	ErrUnknownScope     = errors.New("academic: kapsam birimi bulunamadı")
	ErrOutsideYear      = errors.New("academic: dönem akademik yılın dışında")
	ErrTermClosed       = errors.New("academic: kapanmış dönem aktif yapılamaz")
	ErrNoCurrentTerm    = errors.New("academic: aktif dönem tanımlı değil")
)

// TermType, dönemin türüdür.
type TermType string

// Dönem türleri.
const (
	TermFall   TermType = "FALL"
	TermSpring TermType = "SPRING"
	TermSummer TermType = "SUMMER"
)

// Valid, değerin tanımlı dönem türlerinden biri olup olmadığını söyler.
func (t TermType) Valid() bool {
	return t == TermFall || t == TermSpring || t == TermSummer
}

// TermStatus, dönemin yaşam döngüsündeki yeridir.
type TermStatus string

// Dönem durumları.
const (
	TermPlanned TermStatus = "PLANNED"
	TermActive  TermStatus = "ACTIVE"
	TermClosed  TermStatus = "CLOSED"
)

// Valid, değerin tanımlı dönem durumlarından biri olup olmadığını söyler.
func (s TermStatus) Valid() bool {
	return s == TermPlanned || s == TermActive || s == TermClosed
}

// ScopeType, takvim olayının geçerli olduğu kapsamdır.
type ScopeType string

// Kapsamlar. Dar kapsam geniş kapsamı geçersiz kılar: program > birim > üniversite.
const (
	ScopeUniversity ScopeType = "UNIVERSITY"
	ScopeFaculty    ScopeType = "FACULTY"
	ScopeProgram    ScopeType = "PROGRAM"
)

// Valid, değerin tanımlı kapsamlardan biri olup olmadığını söyler.
func (s ScopeType) Valid() bool {
	return s == ScopeUniversity || s == ScopeFaculty || s == ScopeProgram
}

// AcademicYear, bir akademik yıldır (2026 → "2026-2027").
type AcademicYear struct {
	ID        string
	StartYear int
	StartsOn  time.Time
	EndsOn    time.Time
}

// Label, yılın okunur adıdır.
func (y AcademicYear) Label() string {
	return yearLabel(y.StartYear)
}

func yearLabel(start int) string {
	return strconv.Itoa(start) + "-" + strconv.Itoa(start+1)
}

// Term, bir dönemdir (ör. 2026-FALL).
type Term struct {
	ID             string
	AcademicYearID string
	StartYear      int
	Type           TermType
	Code           string
	StartsOn       time.Time
	EndsOn         time.Time
	Status         TermStatus
	IsCurrent      bool
	Version        int
}

// TermCode, yıl ve türden dönem kodunu üretir: 2026 + FALL → "2026-FALL". Bahar dönemi
// de yılın başladığı yılın koduyla anılır (2026-SPRING, Şubat 2027'de başlar).
func TermCode(startYear int, t TermType) string {
	return strconv.Itoa(startYear) + "-" + string(t)
}

// EventType, takvim olay türüdür.
type EventType struct {
	Code           string
	NameTR         string
	NameEN         string
	Category       string
	IsActionWindow bool
	SortOrder      int
}

// Event, bir türün belirli bir kapsamda geçerli olduğu zaman aralığıdır: [StartsAt, EndsAt).
type Event struct {
	ID          string
	TermID      string
	Type        EventType
	TitleTR     string
	TitleEN     string
	StartsAt    time.Time
	EndsAt      time.Time
	ScopeType   ScopeType
	ScopeID     string
	ScopeName   string
	IsPublished bool
	Note        string
	Version     int
}

// Contains, t anının olayın aralığında olup olmadığını söyler.
func (e Event) Contains(t time.Time) bool {
	return !t.Before(e.StartsAt) && t.Before(e.EndsAt)
}

// Window, bir işlem penceresinin belirli bir hedef (program/birim) için çözümlenmiş
// halidir. Events, hedefe uygulanan olaylardır: en dar kapsamda tanımlı olanlar.
type Window struct {
	Type    EventType
	Scope   ScopeType // olayların geldiği kapsam
	Open    bool
	Current *Event // şu an açık olan olay
	Next    *Event // açık değilse sıradaki olay
	Events  []Event
}

// WindowTarget, pencerenin sorulduğu yerdir. Program verilirse birimi de doldurulmalıdır.
type WindowTarget struct {
	FacultyID string
	ProgramID string
}
