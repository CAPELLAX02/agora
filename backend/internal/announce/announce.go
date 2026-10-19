// Package announce, kurumsal duyurulardır: birim kapsamında yayımlanan, hedef kitlesi (üniversite,
// fakülte, bölüm, program × herkes, öğrenciler, personel) olan, yayın penceresi ve okunma bilgisi
// tutulan duyurular. Yayın başlayınca hedef kitleye bir kez bildirilir (internal/notify).
//
// Ders duyuruları ders alanının Duyurular forumundadır (internal/lms).
package announce

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/richtext"
)

// Duyuru türleri.
const (
	TypeGeneral  = "GENERAL"
	TypeAcademic = "ACADEMIC"
	TypeExam     = "EXAM"
	TypeUrgent   = "URGENT"
	TypeEvent    = "EVENT"
)

// Durumlar.
const (
	StatusDraft     = "DRAFT"
	StatusPublished = "PUBLISHED"
	StatusArchived  = "ARCHIVED"
)

// Hedef kitlenin kapsamları ve kişileri.
const (
	ScopeUniversity = "UNIVERSITY"
	ScopeFaculty    = "FACULTY"
	ScopeDepartment = "DEPARTMENT"
	ScopeProgram    = "PROGRAM"

	PeopleAll      = "ALL"
	PeopleStudents = "STUDENTS"
	PeopleStaff    = "STAFF"
)

// Hatalar.
var (
	ErrNotFound        = errors.New("announce: duyuru bulunamadı")
	ErrVersionMismatch = errors.New("announce: duyuru başkası tarafından değiştirildi")
	// ErrInvalidState: işlem duyurunun durumuna uymuyor (ör. yayımlanmış duyuru silinmek istendi).
	ErrInvalidState = errors.New("announce: duyuru bu işlem için uygun durumda değil")
)

// ValidationError, girdideki bir alanın hatasıdır.
type ValidationError struct {
	Field, Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

// Audience, duyurunun bir hedefidir: birim ve kişiler.
type Audience struct {
	ScopeType string
	ScopeID   string // üniversitede boş
	ScopeName string // okunurken doldurulur (TR)
	People    string
}

// Target, hedefin yetki denetimindeki birimidir (birimin üst birimleri çözülmüş olarak).
type Target = authz.Target

// Announcement, bir duyurudur.
type Announcement struct {
	ID           string
	Title        string
	Summary      string
	Body         string
	Type         string
	PublishFrom  time.Time
	PublishUntil *time.Time
	Pinned       bool
	Status       string
	AuthorID     string
	AuthorName   string
	Audiences    []Audience
	NotifiedAt   *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Version      int
	ReadAt       *time.Time // izleyicinin okuduğu an (akışta)
}

// Input, oluşturulan ya da düzenlenen duyurudur.
type Input struct {
	Title        string
	Summary      string
	Body         string
	Type         string
	PublishFrom  *time.Time // boşsa hemen
	PublishUntil *time.Time
	Pinned       bool
	Audiences    []Audience
}

func (in *Input) validate() error {
	switch n := len([]rune(in.Title)); {
	case n < 1 || n > 200:
		return &ValidationError{"title", "1 ile 200 karakter arasında olmalı."}
	}
	if len([]rune(in.Summary)) > 500 {
		return &ValidationError{"summary", "En çok 500 karakter olmalı."}
	}
	switch in.Type {
	case TypeGeneral, TypeAcademic, TypeExam, TypeUrgent, TypeEvent:
	default:
		return &ValidationError{"type", "Geçersiz duyuru türü."}
	}
	in.Body = richtext.Sanitize(in.Body)
	if in.PublishFrom != nil && in.PublishUntil != nil && !in.PublishUntil.After(*in.PublishFrom) {
		return &ValidationError{"publish_until", "Yayın bitişi başlangıçtan sonra olmalı."}
	}
	if len(in.Audiences) == 0 || len(in.Audiences) > 20 {
		return &ValidationError{"audiences", "1 ile 20 arasında hedef seçin."}
	}
	seen := map[Audience]bool{}
	for i, a := range in.Audiences {
		a.ScopeName = ""
		switch {
		case a.People != PeopleAll && a.People != PeopleStudents && a.People != PeopleStaff:
			return &ValidationError{"audiences", "Geçersiz hedef kişiler."}
		case a.ScopeType == ScopeUniversity && a.ScopeID != "":
			return &ValidationError{"audiences", "Üniversite hedefinde birim seçilmez."}
		case a.ScopeType == ScopeFaculty || a.ScopeType == ScopeDepartment:
			if a.ScopeID == "" {
				return &ValidationError{"audiences", "Hedef birimi seçin."}
			}
		case a.ScopeType == ScopeProgram:
			if a.ScopeID == "" {
				return &ValidationError{"audiences", "Hedef programı seçin."}
			}
			if a.People == PeopleStaff {
				return &ValidationError{"audiences", "Program hedefi sadece öğrencileri kapsar."}
			}
		case a.ScopeType != ScopeUniversity:
			return &ValidationError{"audiences", "Geçersiz hedef kapsamı."}
		}
		if seen[a] {
			return &ValidationError{"audiences", "Aynı hedef iki kez seçildi."}
		}
		seen[a] = true
		in.Audiences[i] = a
	}
	return nil
}

// Service, duyuruları yönetir.
type Service struct {
	pool    *pgxpool.Pool
	now     func() time.Time
	metrics Metrics
}

// Metrics, duyuruların iş metrikleridir: yayımlanan duyurular ve ilk okumalar. İşlem başarıyla
// bittikten sonra sayılır.
type Metrics interface {
	AnnouncementPublished()
	AnnouncementRead()
}

type noopMetrics struct{}

func (noopMetrics) AnnouncementPublished() {}
func (noopMetrics) AnnouncementRead()      {}

// NewService, bir Service oluşturur.
func NewService(pool *pgxpool.Pool, now func() time.Time) *Service {
	return &Service{pool: pool, now: now, metrics: noopMetrics{}}
}

// SetMetrics, duyuru sayaçlarını bağlar. Verilmezse sayılmaz.
func (s *Service) SetMetrics(m Metrics) { s.metrics = m }

// Targets, hedeflerin yetki denetimi için birimlerini çözer (bölümün fakültesi, programın bölümü
// ve fakültesi). Olmayan birim ValidationError döner.
func (s *Service) Targets(ctx context.Context, audiences []Audience) ([]Target, error) {
	out := make([]Target, 0, len(audiences))
	for _, a := range audiences {
		var t Target
		var err error
		switch a.ScopeType {
		case ScopeUniversity:
		case ScopeFaculty:
			t.FacultyID = a.ScopeID
			var ok bool
			err = s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM org.faculties WHERE id = $1)`, a.ScopeID).Scan(&ok)
			if err == nil && !ok {
				err = pgx.ErrNoRows
			}
		case ScopeDepartment:
			t.DepartmentID = a.ScopeID
			err = s.pool.QueryRow(ctx, `SELECT faculty_id FROM org.departments WHERE id = $1`, a.ScopeID).Scan(&t.FacultyID)
		case ScopeProgram:
			t.ProgramID = a.ScopeID
			err = s.pool.QueryRow(ctx, `
				SELECT d.id, d.faculty_id FROM org.programs p JOIN org.departments d ON d.id = p.department_id
				WHERE p.id = $1`, a.ScopeID).Scan(&t.DepartmentID, &t.FacultyID)
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &ValidationError{"audiences", "Hedef birim bulunamadı."}
		}
		if err != nil {
			return nil, fmt.Errorf("announce: hedef birim okunamadı: %w", err)
		}
		out = append(out, t)
	}
	return out, nil
}

// Allowed, yetkinin bütün hedefleri kapsadığını söyler: üniversite hedefi üniversite kapsamı ister.
func Allowed(perms *authz.Permissions, audiences []Audience, targets []Target) bool {
	for i, a := range audiences {
		if a.ScopeType == ScopeUniversity {
			if !perms.ScopesOf("announcement:publish").University {
				return false
			}
			continue
		}
		if !perms.Allows("announcement:publish", targets[i]) {
			return false
		}
	}
	return len(audiences) > 0
}
