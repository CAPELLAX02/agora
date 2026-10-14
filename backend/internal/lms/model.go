// Package lms, öğrenme yönetimidir (e-Kampüs'ün karşılığı): ders alanı, bölümler ve
// etkinlikler. Ders alanının anahtarı şubedir ve üyelik saklanmaz: öğrenci şubede bırakmamış
// kesin kaydı olan, öğretim elemanı şubeye atanmış olandır (yol haritası §5 #19).
package lms

import (
	"errors"
	"time"

	"github.com/microcosm-cc/bluemonday"
)

// Hatalar.
var (
	ErrNotFound        = errors.New("lms: kayıt bulunamadı")
	ErrVersionMismatch = errors.New("lms: kayıt bu arada değişti")
	ErrNotAvailable    = errors.New("lms: etkinlik henüz açık değil ya da kapandı")
)

// Role, kullanıcının ders alanındaki rolüdür.
type Role string

// Roller. Viewer, şubenin bölümünü yöneten (salt okunur) personeldir.
const (
	RoleNone      Role = ""
	RoleStudent   Role = "STUDENT"
	RolePrimary   Role = "PRIMARY"
	RoleCoTeacher Role = "CO_INSTRUCTOR"
	RoleAssistant Role = "ASSISTANT"
	RoleViewer    Role = "VIEWER"
)

// permSectionManage, şubenin bölümünü yönetme yetkisidir: ders alanını salt okunur görmeyi sağlar.
const permSectionManage = "section:manage"

// CanEdit, rolün içerik düzenleyebildiğini söyler: şubenin bütün öğretim elemanları.
func (r Role) CanEdit() bool {
	return r == RolePrimary || r == RoleCoTeacher || r == RoleAssistant
}

// ActivityType, etkinliğin türüdür.
type ActivityType string

// Etkinlik türleri.
const (
	TypeResource   ActivityType = "RESOURCE" // tek dosya
	TypeFolder     ActivityType = "FOLDER"   // birden çok dosya
	TypeURL        ActivityType = "URL"
	TypePage       ActivityType = "PAGE"
	TypeLabel      ActivityType = "LABEL" // bölüm içinde metin
	TypeAssignment ActivityType = "ASSIGNMENT"
	TypeForum      ActivityType = "FORUM"
)

// Availability, bir bölümün ya da etkinliğin erişim durumudur.
type Availability string

// Erişim durumları.
const (
	Open   Availability = "OPEN"
	NotYet Availability = "NOT_YET" // açılış zamanı gelmedi
	Closed Availability = "CLOSED"  // kapanış zamanı geçti
)

// Window, erişim zamanlarıdır; boş uç sınırsızdır.
type Window struct {
	From  *time.Time
	Until *time.Time
}

// At, verilen andaki erişim durumudur.
func (w Window) At(now time.Time) Availability {
	switch {
	case w.From != nil && now.Before(*w.From):
		return NotYet
	case w.Until != nil && !now.Before(*w.Until):
		return Closed
	}
	return Open
}

// Section, ders alanının şubesidir.
type Section struct {
	ID           string
	Code         string
	CourseID     string
	CourseCode   string
	CourseNameTR string
	CourseNameEN string
	TermID       string
	TermCode     string
	DepartmentID string
	FacultyID    string
}

// Instructor, şubenin öğretim elemanıdır.
type Instructor struct {
	UserID    string
	Title     string
	FirstName string
	LastName  string
	Email     string
	Role      Role
}

// SpaceSummary, "Derslerim" listesindeki bir ders alanıdır.
type SpaceSummary struct {
	Section
	Role        Role
	Instructors []Instructor
	Activities  int // görünür etkinlik sayısı
}

// FileRef, bir etkinliğin dosyasıdır.
type FileRef struct {
	ID       string
	Name     string
	MimeType string
	Size     int64
}

// Topic, bir bölümdür.
type Topic struct {
	ID         string
	Title      string
	Summary    string
	Position   int
	Visible    bool
	Window     Window
	Version    int
	Activities []Activity
}

// Activity, bir etkinliktir. Türe özgü alanlar sadece o türde doludur.
type Activity struct {
	ID          string
	TopicID     string
	Type        ActivityType
	Title       string
	Description string
	Position    int
	Visible     bool
	Window      Window
	Version     int
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Files       []FileRef // RESOURCE, FOLDER
	URL         string    // URL
	NewTab      bool      // URL
	Body        string    // PAGE (sadece ayrıntıda)
}

// Space, ders alanının görünümüdür: izleyenin rolüne göre süzülmüş bölümler ve etkinlikler.
type Space struct {
	Section
	Role        Role
	Summary     string
	Instructors []Instructor
	Topics      []Topic
}

// policy, ders içeriğindeki zengin metnin izin listesidir: biçimlendirme, başlıklar, listeler,
// tablolar, bağlantılar, görseller ve kod blokları. Betik, stil, olay öznitelikleri ve
// iframe atılır; bağlantılar yeni sekmede ve noopener ile açılır.
var policy = func() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.RequireNoReferrerOnLinks(true)
	p.AddTargetBlankToFullyQualifiedLinks(true)
	p.AllowAttrs("class").Matching(bluemonday.SpaceSeparatedTokens).OnElements("code", "pre", "span")
	return p
}()

// Sanitize, kullanıcının yazdığı HTML'i izin listesine göre temizler.
func Sanitize(html string) string {
	return policy.Sanitize(html)
}
