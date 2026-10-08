// Package authz, yetkilendirmeyi (authorization) sağlar: kimliği doğrulanmış
// kullanıcının bir eylemi hangi organizasyon kapsamında yapabileceğine karar verir.
package authz

import (
	"cmp"
	"context"
	"errors"
	"slices"
)

// Kapsam türleri. iam.role_assignments.scope_type ile aynı değerlerdir.
const (
	ScopeUniversity = "UNIVERSITY"
	ScopeFaculty    = "FACULTY"
	ScopeDepartment = "DEPARTMENT"
	ScopeProgram    = "PROGRAM"
	ScopeNone       = "NONE"
)

// ErrAccountInactive, kullanıcının hesabı aktif olmadığında döner.
var ErrAccountInactive = errors.New("authz: hesap aktif değil")

// Grant, bir yetkinin hangi kapsamda verildiğidir.
type Grant struct {
	Permission string `json:"permission"`
	ScopeType  string `json:"scope_type"`
	ScopeID    string `json:"scope_id,omitempty"` // UNIVERSITY ve NONE kapsamlarında boş
}

// Target, üzerinde işlem yapılan kaynağın organizasyondaki yeridir. Çağıran taraf
// bildiği bütün üst birimleri doldurmalı: bir bölüm için hem DepartmentID hem
// FacultyID. Fakülte kapsamlı bir yetki, o fakültenin bölümlerini de kapsar.
type Target struct {
	FacultyID    string
	DepartmentID string
	ProgramID    string
}

// Permissions, bir kullanıcının belirli bir andaki yetkileridir.
type Permissions struct {
	userID                 string
	grants                 map[string][]Grant // yetki kodu → kapsamlar
	passwordChangeRequired bool
}

// NewPermissions, grant listesinden bir yetki kümesi oluşturur.
func NewPermissions(userID string, grants []Grant) *Permissions {
	p := &Permissions{userID: userID, grants: make(map[string][]Grant, len(grants))}
	for _, g := range grants {
		p.grants[g.Permission] = append(p.grants[g.Permission], g)
	}
	return p
}

// UserID, yetkilerin sahibidir.
func (p *Permissions) UserID() string {
	return p.userID
}

// RequirePasswordChange, kullanıcının parolasını değiştirmeden SelfService dışındaki
// route'lara erişemeyeceğini işaretler.
func (p *Permissions) RequirePasswordChange() {
	p.passwordChangeRequired = true
}

// PasswordChangeRequired, kullanıcının parolasını değiştirmesi gerekip gerekmediğini söyler.
func (p *Permissions) PasswordChangeRequired() bool {
	return p.passwordChangeRequired
}

// Has, yetkinin herhangi bir kapsamda verilip verilmediğini söyler. Route
// düzeyindeki kaba kontrol içindir: "bu kişi hiç not girebilir mi?"
func (p *Permissions) Has(permission string) bool {
	return len(p.grants[permission]) > 0
}

// Allows, yetkinin target'ı kapsayan bir kapsamda verilip verilmediğini söyler.
// Üniversite kapsamı her şeyi, fakülte kapsamı o fakültenin bölüm ve
// programlarını, bölüm kapsamı o bölümün programlarını kapsar.
//
// NONE kapsamlı yetkiler (ör. öğrencinin "kendi notlarını görme" yetkisi) hiçbir
// birimi kapsamaz: sahiplik kontrolü politika fonksiyonlarında yapılır.
func (p *Permissions) Allows(permission string, t Target) bool {
	for _, g := range p.grants[permission] {
		switch g.ScopeType {
		case ScopeUniversity:
			return true
		case ScopeFaculty:
			if t.FacultyID != "" && g.ScopeID == t.FacultyID {
				return true
			}
		case ScopeDepartment:
			if t.DepartmentID != "" && g.ScopeID == t.DepartmentID {
				return true
			}
		case ScopeProgram:
			if t.ProgramID != "" && g.ScopeID == t.ProgramID {
				return true
			}
		}
	}
	return false
}

// Grants, bütün yetkileri yetki koduna ve kapsama göre sıralı döndürür.
func (p *Permissions) Grants() []Grant {
	out := []Grant{}
	for _, gs := range p.grants {
		out = append(out, gs...)
	}
	slices.SortFunc(out, func(a, b Grant) int {
		return cmp.Or(
			cmp.Compare(a.Permission, b.Permission),
			cmp.Compare(a.ScopeType, b.ScopeType),
			cmp.Compare(a.ScopeID, b.ScopeID),
		)
	})
	return out
}

// Resolver, bir kullanıcının güncel yetkilerini çözen bileşendir. Hesap aktif
// değilse ErrAccountInactive döner.
type Resolver interface {
	Permissions(ctx context.Context, userID string) (*Permissions, error)
}

type ctxKey struct{}

// WithPermissions, p'yi taşıyan yeni bir context döndürür.
func WithPermissions(ctx context.Context, p *Permissions) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

// PermissionsFrom, context'teki yetkileri döndürür. İstek korumalı bir route'tan
// geçmediyse ok false olur.
func PermissionsFrom(ctx context.Context) (p *Permissions, ok bool) {
	p, ok = ctx.Value(ctxKey{}).(*Permissions)
	return p, ok
}
