// Package iam, kullanıcı hesaplarını, oturumları, rolleri ve yetkileri yönetir.
package iam

import (
	"errors"
	"time"
)

// Hatalar.
var (
	ErrNotFound          = errors.New("iam: kayıt bulunamadı")
	ErrConflict          = errors.New("iam: kayıt zaten var")
	ErrInvalidRoleScope  = errors.New("iam: rol bu kapsam türünde atanamaz")
	ErrUnknownPermission = errors.New("iam: bilinmeyen yetki")
)

// UserStatus, hesabın durumudur.
type UserStatus string

// Hesap durumları.
const (
	StatusPending   UserStatus = "PENDING"
	StatusActive    UserStatus = "ACTIVE"
	StatusSuspended UserStatus = "SUSPENDED"
	StatusDisabled  UserStatus = "DISABLED"
)

// ScopeType, bir rol atamasının geçerli olduğu organizasyon kapsamıdır.
type ScopeType string

// Kapsam türleri.
const (
	ScopeUniversity ScopeType = "UNIVERSITY"
	ScopeFaculty    ScopeType = "FACULTY"
	ScopeDepartment ScopeType = "DEPARTMENT"
	ScopeProgram    ScopeType = "PROGRAM"
	ScopeNone       ScopeType = "NONE"
)

// User, oturum açabilen bir hesaptır.
type User struct {
	ID                 string
	PersonID           string
	Username           string
	Email              string
	Status             UserStatus
	PasswordHash       string
	MustChangePassword bool
	FailedLoginCount   int
	LockedUntil        *time.Time
	LastLoginAt        *time.Time
	PermVersion        int
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// NewUser, oluşturulacak hesabın bilgileridir. PasswordHash önceden hash'lenmiş olmalıdır.
type NewUser struct {
	PersonID           string
	Username           string
	Email              string
	PasswordHash       string
	MustChangePassword bool
}

// Grant, kullanıcının sahip olduğu bir yetkinin hangi rolden ve hangi kapsamda geldiğidir.
// Aynı yetki farklı rollerden ve kapsamlardan birden fazla kez gelebilir.
type Grant struct {
	Permission string
	Role       string
	ScopeType  ScopeType
	ScopeID    string // UNIVERSITY ve NONE kapsamlarında boş
}
