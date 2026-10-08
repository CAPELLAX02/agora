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

// CampusRef, bir birimin bağlı olduğu yerleşkenin özetidir.
type CampusRef struct {
	ID   string
	Code string
	Name string
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
