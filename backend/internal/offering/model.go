// Package offering, bir dönemde açılan dersleri, şubelerini, kontenjanlarını, öğretim
// elemanlarını ve haftalık programını yönetir.
package offering

import (
	"errors"
	"fmt"
)

// Hatalar.
var (
	ErrNotFound          = errors.New("offering: kayıt bulunamadı")
	ErrConflict          = errors.New("offering: aynı kayıt zaten var")
	ErrVersionMismatch   = errors.New("offering: kayıt bu arada değişti")
	ErrUnknownReference  = errors.New("offering: başvurulan kayıt bulunamadı")
	ErrTermClosed        = errors.New("offering: dönem kapanmış")
	ErrInvalidTransition = errors.New("offering: bu durum geçişi yapılamaz")
	ErrHasEnrollments    = errors.New("offering: kayıtlı öğrenci var")
	ErrBelowEnrolled     = errors.New("offering: kontenjan kayıtlı öğrenci sayısının altında")
	ErrQuotaExceeds      = errors.New("offering: program kontenjanlarının toplamı şube kontenjanını aşıyor")
	ErrSectionOverlap    = errors.New("offering: şubenin oturumları çakışıyor")
	ErrClassroomInactive = errors.New("offering: derslik kullanımda değil")
	ErrPrimaryRequired   = errors.New("offering: şubenin tek bir sorumlu öğretim elemanı olmalı")
	ErrNotInstructor     = errors.New("offering: personel akademik değil ya da görevde değil")
	ErrSectionCancelled  = errors.New("offering: şube iptal edilmiş")
)

// ClassroomConflictError, derslik başka bir oturuma verilmişse döner.
type ClassroomConflictError struct{ With SlotRef }

func (e *ClassroomConflictError) Error() string {
	return fmt.Sprintf("offering: derslik %s ile çakışıyor", e.With)
}

// InstructorConflictError, öğretim elemanının aynı saatte başka bir oturumu varsa döner.
type InstructorConflictError struct {
	StaffName string
	With      SlotRef
}

func (e *InstructorConflictError) Error() string {
	return fmt.Sprintf("offering: %s, %s ile çakışıyor", e.StaffName, e.With)
}

// ClassroomTooSmallError, teorik oturumun dersliği şube kontenjanından küçükse döner.
type ClassroomTooSmallError struct {
	Classroom string
	Capacity  int
	Needed    int
}

func (e *ClassroomTooSmallError) Error() string {
	return fmt.Sprintf("offering: %s %d kişilik, şube %d kişilik", e.Classroom, e.Capacity, e.Needed)
}

// SlotRef, çakışma mesajlarında gösterilen oturumdur.
type SlotRef struct {
	CourseCode  string
	SectionCode string
	DayOfWeek   int
	Start, End  string
}

var dayNames = [...]string{"", "Pazartesi", "Salı", "Çarşamba", "Perşembe", "Cuma", "Cumartesi", "Pazar"}

func (s SlotRef) String() string {
	day := ""
	if s.DayOfWeek >= 1 && s.DayOfWeek <= 7 {
		day = dayNames[s.DayOfWeek]
	}
	return fmt.Sprintf("%s-%s (%s %s-%s)", s.CourseCode, s.SectionCode, day, s.Start, s.End)
}

// Status, açılan dersin durumudur.
type Status string

// Ders açma durumları.
const (
	StatusPlanned   Status = "PLANNED"   // hazırlanıyor, ders seçmede görünmez
	StatusOpen      Status = "OPEN"      // ders seçmeye açık
	StatusClosed    Status = "CLOSED"    // dönem sona erdi
	StatusCancelled Status = "CANCELLED" // açılmadı
)

// Valid, değerin tanımlı durumlardan biri olup olmadığını söyler.
func (s Status) Valid() bool {
	switch s {
	case StatusPlanned, StatusOpen, StatusClosed, StatusCancelled:
		return true
	}
	return false
}

// CanBecome, bir durumdan diğerine geçilip geçilemeyeceğini söyler.
func (s Status) CanBecome(next Status) bool {
	switch {
	case s == next:
		return true
	case next == StatusCancelled:
		return s == StatusPlanned || s == StatusOpen
	case s == StatusPlanned:
		return next == StatusOpen
	case s == StatusOpen:
		return next == StatusClosed || next == StatusPlanned
	}
	return false
}

// Ref, kodlu ve adlı bir kaydın özetidir.
type Ref struct {
	ID     string
	Code   string
	NameTR string
	NameEN string
}

// CourseRef, açılan dersin katalog bilgisidir.
type CourseRef struct {
	Ref
	TheoryHours    int
	PracticeHours  int
	NationalCredit float64
	ECTS           float64
	Language       string
}

// Offering, bir dönemde açılan derstir.
type Offering struct {
	ID            string
	TermID        string
	TermCode      string
	Course        CourseRef
	Department    Ref
	FacultyID     string
	Status        Status
	ExternalRef   string
	Note          string
	SectionCount  int
	TotalCapacity int
	TotalEnrolled int
	Version       int
}

// Instructor, şubenin bir öğretim elemanıdır.
type Instructor struct {
	StaffID   string
	StaffNo   string
	Title     string
	FirstName string
	LastName  string
	Role      string
}

// FullName, unvanıyla birlikte adıdır.
func (i Instructor) FullName() string {
	if i.Title == "" {
		return i.FirstName + " " + i.LastName
	}
	return i.Title + " " + i.FirstName + " " + i.LastName
}

// Quota, şubede bir programa ayrılmış kontenjandır.
type Quota struct {
	Program  Ref
	Quota    int
	Enrolled int
}

// ClassroomRef, oturumun dersliğidir.
type ClassroomRef struct {
	ID           string
	Code         string
	Name         string
	BuildingCode string
	Capacity     int
}

// Slot, haftalık programda bir oturumdur.
type Slot struct {
	ID          string
	SectionID   string
	DayOfWeek   int
	Start       string // HH:MM
	End         string // HH:MM, hariç
	Classroom   *ClassroomRef
	SessionType string
}

// Section, açılan dersin bir şubesidir.
type Section struct {
	ID              string
	OfferingID      string
	Code            string
	Capacity        int
	EnrolledCount   int
	QuotaMode       string
	InstructionMode string
	Language        string
	Status          string
	Version         int
	Instructors     []Instructor
	Quotas          []Quota
	Slots           []Slot
}

// ScheduleEntry, haftalık program görünümünde bir oturum ve şubesidir.
type ScheduleEntry struct {
	Slot
	TermID      string
	OfferingID  string
	Course      Ref
	SectionCode string
	Instructors []Instructor
}

// StaffSummary, öğretim elemanı aramasının sonucudur.
type StaffSummary struct {
	StaffID    string
	StaffNo    string
	Title      string
	FirstName  string
	LastName   string
	Department *Ref
}
