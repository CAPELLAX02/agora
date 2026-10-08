// Package people, gerçek kişileri ve onların öğrenci/personel kimliklerini yönetir.
package people

import (
	"context"
	"errors"
	"fmt"

	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// ErrConflict, aynı numaraya sahip bir kaydın zaten bulunduğunu bildirir.
var ErrConflict = errors.New("people: kayıt zaten var")

// StaffType, personelin türüdür.
type StaffType string

// Personel türleri.
const (
	StaffAcademic       StaffType = "ACADEMIC"
	StaffAdministrative StaffType = "ADMINISTRATIVE"
)

// NewPerson, oluşturulacak kişinin bilgileridir.
type NewPerson struct {
	FirstName string
	LastName  string
}

// NewStaff, oluşturulacak personel kaydıdır. AcademicTitle ve DepartmentID boş olabilir.
type NewStaff struct {
	PersonID      string
	StaffNo       string
	Type          StaffType
	AcademicTitle string // "PROF", "ASSOC_PROF"... Sadece akademik personelde.
	DepartmentID  string
}

// Repository, people şemasına erişen veri katmanıdır.
type Repository struct {
	db db.Querier
}

// NewRepository, verilen bağlantıyı (havuz veya transaction) kullanan bir Repository oluşturur.
func NewRepository(q db.Querier) *Repository {
	return &Repository{db: q}
}

// CreatePerson, yeni bir kişi oluşturur ve kimliğini döndürür.
func (r *Repository) CreatePerson(ctx context.Context, p NewPerson) (string, error) {
	var id string
	err := r.db.QueryRow(ctx,
		`INSERT INTO people.persons (first_name, last_name) VALUES ($1, $2) RETURNING id`,
		p.FirstName, p.LastName,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("people: kişi oluşturulamadı: %w", err)
	}
	return id, nil
}

// CreateStudent, bir kişiye öğrenci numarası atar.
func (r *Repository) CreateStudent(ctx context.Context, personID, studentNo string) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO people.students (person_id, student_no) VALUES ($1, $2)`,
		personID, studentNo,
	)
	if db.IsConflict(err) {
		return ErrConflict
	}
	if err != nil {
		return fmt.Errorf("people: öğrenci oluşturulamadı: %w", err)
	}
	return nil
}

// CreateStaff, bir kişiyi personel olarak kaydeder.
func (r *Repository) CreateStaff(ctx context.Context, s NewStaff) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO people.staff (person_id, staff_no, staff_type, academic_title_code, primary_department_id)
		 VALUES ($1, $2, $3, $4, $5)`,
		s.PersonID, s.StaffNo, string(s.Type), nullable(s.AcademicTitle), nullable(s.DepartmentID),
	)
	if db.IsConflict(err) {
		return ErrConflict
	}
	if err != nil {
		return fmt.Errorf("people: personel oluşturulamadı: %w", err)
	}
	return nil
}

// nullable, boş metni SQL NULL'a çevirir. pgx, nil değeri NULL olarak gönderir.
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
