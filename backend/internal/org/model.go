// Package org, üniversitenin organizasyon yapısını (yerleşke, fakülte, bölüm, program) yönetir.
package org

import (
	"errors"
	"time"
)

// ErrNotFound, istenen kaydın bulunamadığını bildirir.
var ErrNotFound = errors.New("org: kayıt bulunamadı")

// UnitType, akademik birimin türüdür.
type UnitType string

// Akademik birim türleri. Değerler veritabanındaki CHECK kısıtıyla birebir aynıdır.
const (
	UnitFaculty          UnitType = "FACULTY"
	UnitVocationalSchool UnitType = "VOCATIONAL_SCHOOL"
	UnitSchool           UnitType = "SCHOOL"
	UnitInstitute        UnitType = "INSTITUTE"
	UnitConservatory     UnitType = "CONSERVATORY"
)

// Valid, değerin tanımlı birim türlerinden biri olup olmadığını söyler.
func (u UnitType) Valid() bool {
	switch u {
	case UnitFaculty, UnitVocationalSchool, UnitSchool, UnitInstitute, UnitConservatory:
		return true
	}
	return false
}

// DegreeLevel, programın derecesidir.
type DegreeLevel string

// Program dereceleri.
const (
	DegreeAssociate DegreeLevel = "ASSOCIATE" // ön lisans
	DegreeBachelor  DegreeLevel = "BACHELOR"  // lisans
	DegreeMaster    DegreeLevel = "MASTER"    // yüksek lisans
	DegreePhD       DegreeLevel = "PHD"       // doktora
)

// Valid, değerin tanımlı derecelerden biri olup olmadığını söyler.
func (d DegreeLevel) Valid() bool {
	switch d {
	case DegreeAssociate, DegreeBachelor, DegreeMaster, DegreePhD:
		return true
	}
	return false
}

// Language, programın öğretim dilidir.
type Language string

// Öğretim dilleri.
const (
	LanguageTR    Language = "TR"
	LanguageEN    Language = "EN"
	LanguageMixed Language = "MIXED"
)

// Valid, değerin tanımlı dillerden biri olup olmadığını söyler.
func (l Language) Valid() bool {
	switch l {
	case LanguageTR, LanguageEN, LanguageMixed:
		return true
	}
	return false
}

// EducationType, programın öğretim türüdür.
type EducationType string

// Öğretim türleri.
const (
	EducationDaytime  EducationType = "DAYTIME"  // normal öğretim (N.Ö.)
	EducationEvening  EducationType = "EVENING"  // ikinci öğretim (İ.Ö.)
	EducationDistance EducationType = "DISTANCE" // uzaktan öğretim
)

// Valid, değerin tanımlı öğretim türlerinden biri olup olmadığını söyler.
func (e EducationType) Valid() bool {
	switch e {
	case EducationDaytime, EducationEvening, EducationDistance:
		return true
	}
	return false
}

// CampusRef, bir birimin bağlı olduğu yerleşkenin özetidir.
type CampusRef struct {
	ID   string
	Code string
	Name string
}

// FacultyRef, başka bir kaydın içinde gösterilen birim özetidir.
type FacultyRef struct {
	ID     string
	Code   string
	NameTR string
}

// DepartmentRef, başka bir kaydın içinde gösterilen bölüm özetidir.
type DepartmentRef struct {
	ID     string
	Code   string
	NameTR string
}

// Faculty, bir akademik birimdir: fakülte, meslek yüksekokulu, enstitü vb.
type Faculty struct {
	ID        string
	Code      string
	NameTR    string
	NameEN    string
	UnitType  UnitType
	Campus    *CampusRef // yerleşke atanmamışsa nil
	IsActive  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// FacultyFilter, fakülte listesini daraltan ölçütlerdir. Sıfır değeri "filtre yok" demektir.
type FacultyFilter struct {
	UnitType        UnitType // boşsa tüm türler
	Query           string   // kod veya adda geçen metin
	IncludeInactive bool     // varsayılan olarak sadece aktif birimler
}

// Department, bir birime bağlı akademik bölümdür.
type Department struct {
	ID        string
	Faculty   FacultyRef
	Code      string
	NameTR    string
	NameEN    string
	IsActive  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Program, bir bölümün sunduğu diploma programıdır.
// Ör. "Bilgisayar Mühendisliği (İngilizce) (N.Ö.)".
type Program struct {
	ID                string
	Department        DepartmentRef
	Faculty           FacultyRef
	Code              string
	YoksisCode        *string // atanmamışsa nil
	NameTR            string
	NameEN            string
	DegreeLevel       DegreeLevel
	Language          Language
	EducationType     EducationType
	DurationSemesters int
	MaxDurationYears  int
	TotalECTSRequired float64
	HasPrepClass      bool
	IsActive          bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// ProgramCursor, program listesinde bir sonraki sayfanın nereden başlayacağını tutar.
// Liste (name_tr, id) sırasında olduğu için ikisi birlikte benzersiz bir konum belirtir.
type ProgramCursor struct {
	NameTR string `json:"n"`
	ID     string `json:"i"`
}

// ProgramFilter, program listesini daraltan ve sayfalayan ölçütlerdir.
type ProgramFilter struct {
	FacultyID       string
	DepartmentID    string
	DegreeLevel     DegreeLevel
	Language        Language
	EducationType   EducationType
	Query           string
	IncludeInactive bool
	After           *ProgramCursor // nil ise ilk sayfa
	Limit           int
}
