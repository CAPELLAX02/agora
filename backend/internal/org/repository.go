package org

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// Repository, org şemasına erişen veri katmanıdır.
type Repository struct {
	db *pgxpool.Pool
}

// NewRepository, verilen bağlantı havuzunu kullanan bir Repository oluşturur.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// scanner, pgx.Row ve pgx.Rows'un ortak Scan metodudur. Böylece tek satır ve
// çok satır sorguları aynı eşleme fonksiyonunu kullanabilir.
type scanner interface {
	Scan(dest ...any) error
}

// collect, sorgunun tüm satırlarını scan fonksiyonuyla okur ve kaynakları serbest bırakır.
func collect[T any](rows pgx.Rows, scan func(scanner) (T, error)) ([]T, error) {
	defer rows.Close()

	items := []T{}
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

// --- Birimler ----------------------------------------------------------------

const facultySelect = `
	SELECT f.id, f.code, f.name_tr, f.name_en, f.unit_type, f.is_active,
	       f.created_at, f.updated_at,
	       c.id, c.code, c.name
	FROM org.faculties f
	LEFT JOIN org.campuses c ON c.id = f.campus_id`

// ListFaculties, filtreye uyan birimleri Türkçe ada göre sıralı döndürür.
func (r *Repository) ListFaculties(ctx context.Context, filter FacultyFilter) ([]Faculty, error) {
	var w db.Where
	if !filter.IncludeInactive {
		w.Add("f.is_active")
	}
	if filter.UnitType != "" {
		w.Add("f.unit_type = $1", string(filter.UnitType))
	}
	if filter.Query != "" {
		w.Add("(f.code ILIKE $1 OR f.name_tr ILIKE $1 OR f.name_en ILIKE $1)", "%"+filter.Query+"%")
	}

	query := facultySelect + "\n\t" + w.SQL() +
		"\n\tORDER BY f.name_tr COLLATE \"tr-x-icu\", f.id"

	rows, err := r.db.Query(ctx, query, w.Args()...)
	if err != nil {
		return nil, fmt.Errorf("org: birimler listelenemedi: %w", err)
	}

	faculties, err := collect(rows, scanFaculty)
	if err != nil {
		return nil, fmt.Errorf("org: birimler okunamadı: %w", err)
	}
	return faculties, nil
}

// GetFaculty, kimliği verilen birimi döndürür. Yoksa ErrNotFound döner.
func (r *Repository) GetFaculty(ctx context.Context, id string) (Faculty, error) {
	row := r.db.QueryRow(ctx, facultySelect+"\n\tWHERE f.id = $1", id)

	f, err := scanFaculty(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Faculty{}, ErrNotFound
	}
	if err != nil {
		return Faculty{}, fmt.Errorf("org: birim okunamadı: %w", err)
	}
	return f, nil
}

func scanFaculty(s scanner) (Faculty, error) {
	var (
		f                              Faculty
		unitType                       string
		campusID, campusCode, campusNm *string
	)

	err := s.Scan(
		&f.ID, &f.Code, &f.NameTR, &f.NameEN, &unitType, &f.IsActive,
		&f.CreatedAt, &f.UpdatedAt,
		&campusID, &campusCode, &campusNm,
	)
	if err != nil {
		return Faculty{}, err
	}

	f.UnitType = UnitType(unitType)
	if campusID != nil {
		f.Campus = &CampusRef{ID: *campusID, Code: *campusCode, Name: *campusNm}
	}
	return f, nil
}

// --- Bölümler ----------------------------------------------------------------

const departmentSelect = `
	SELECT d.id, d.code, d.name_tr, d.name_en, d.is_active, d.created_at, d.updated_at,
	       f.id, f.code, f.name_tr
	FROM org.departments d
	JOIN org.faculties f ON f.id = d.faculty_id`

// ListDepartments, bir birime bağlı bölümleri Türkçe ada göre sıralı döndürür.
func (r *Repository) ListDepartments(ctx context.Context, facultyID string, includeInactive bool) ([]Department, error) {
	var w db.Where
	w.Add("d.faculty_id = $1", facultyID)
	if !includeInactive {
		w.Add("d.is_active")
	}

	query := departmentSelect + "\n\t" + w.SQL() +
		"\n\tORDER BY d.name_tr COLLATE \"tr-x-icu\", d.id"

	rows, err := r.db.Query(ctx, query, w.Args()...)
	if err != nil {
		return nil, fmt.Errorf("org: bölümler listelenemedi: %w", err)
	}

	departments, err := collect(rows, scanDepartment)
	if err != nil {
		return nil, fmt.Errorf("org: bölümler okunamadı: %w", err)
	}
	return departments, nil
}

// GetDepartment, kimliği verilen bölümü döndürür. Yoksa ErrNotFound döner.
func (r *Repository) GetDepartment(ctx context.Context, id string) (Department, error) {
	row := r.db.QueryRow(ctx, departmentSelect+"\n\tWHERE d.id = $1", id)

	d, err := scanDepartment(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Department{}, ErrNotFound
	}
	if err != nil {
		return Department{}, fmt.Errorf("org: bölüm okunamadı: %w", err)
	}
	return d, nil
}

func scanDepartment(s scanner) (Department, error) {
	var d Department
	err := s.Scan(
		&d.ID, &d.Code, &d.NameTR, &d.NameEN, &d.IsActive, &d.CreatedAt, &d.UpdatedAt,
		&d.Faculty.ID, &d.Faculty.Code, &d.Faculty.NameTR,
	)
	return d, err
}

// --- Programlar --------------------------------------------------------------

const programSelect = `
	SELECT p.id, p.code, p.yoksis_code, p.name_tr, p.name_en,
	       p.degree_level, p.language, p.education_type,
	       p.duration_semesters, p.max_duration_years, p.total_ects_required::float8,
	       p.has_prep_class, p.is_active, p.created_at, p.updated_at,
	       d.id, d.code, d.name_tr,
	       f.id, f.code, f.name_tr
	FROM org.programs p
	JOIN org.departments d ON d.id = p.department_id
	JOIN org.faculties f ON f.id = d.faculty_id`

// ListPrograms, filtreye uyan programları (name_tr, id) sırasıyla, en fazla
// filter.Limit kadar döndürür. hasMore, bu sayfadan sonra kayıt olup olmadığını söyler.
//
// Sayfalama keyset (cursor) yöntemiyle yapılır: OFFSET yerine "son görülen
// kaydın ardından" diye sorgulanır. Böylece sayfa numarası büyüdükçe sorgu
// yavaşlamaz ve sayfalar arasında kayıt eklense bile tekrar veya atlama olmaz.
func (r *Repository) ListPrograms(ctx context.Context, filter ProgramFilter) (programs []Program, hasMore bool, err error) {
	var w db.Where
	if !filter.IncludeInactive {
		w.Add("p.is_active")
	}
	if filter.FacultyID != "" {
		w.Add("d.faculty_id = $1", filter.FacultyID)
	}
	if filter.DepartmentID != "" {
		w.Add("p.department_id = $1", filter.DepartmentID)
	}
	if filter.DegreeLevel != "" {
		w.Add("p.degree_level = $1", string(filter.DegreeLevel))
	}
	if filter.Language != "" {
		w.Add("p.language = $1", string(filter.Language))
	}
	if filter.EducationType != "" {
		w.Add("p.education_type = $1", string(filter.EducationType))
	}
	if filter.Query != "" {
		w.Add("(p.code ILIKE $1 OR p.name_tr ILIKE $1 OR p.name_en ILIKE $1)", "%"+filter.Query+"%")
	}
	if filter.After != nil {
		w.Add(`(p.name_tr COLLATE "tr-x-icu", p.id) > ($1 COLLATE "tr-x-icu", $2::uuid)`,
			filter.After.NameTR, filter.After.ID)
	}

	// Bir fazlasını istiyoruz: gelirse sonraki sayfa var demektir.
	limit := w.Arg(filter.Limit + 1)

	query := programSelect + "\n\t" + w.SQL() +
		"\n\tORDER BY p.name_tr COLLATE \"tr-x-icu\", p.id" +
		"\n\tLIMIT " + limit

	rows, err := r.db.Query(ctx, query, w.Args()...)
	if err != nil {
		return nil, false, fmt.Errorf("org: programlar listelenemedi: %w", err)
	}

	programs, err = collect(rows, scanProgram)
	if err != nil {
		return nil, false, fmt.Errorf("org: programlar okunamadı: %w", err)
	}

	if len(programs) > filter.Limit {
		return programs[:filter.Limit], true, nil
	}
	return programs, false, nil
}

// GetProgram, kimliği verilen programı döndürür. Yoksa ErrNotFound döner.
func (r *Repository) GetProgram(ctx context.Context, id string) (Program, error) {
	row := r.db.QueryRow(ctx, programSelect+"\n\tWHERE p.id = $1", id)

	p, err := scanProgram(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Program{}, ErrNotFound
	}
	if err != nil {
		return Program{}, fmt.Errorf("org: program okunamadı: %w", err)
	}
	return p, nil
}

func scanProgram(s scanner) (Program, error) {
	var (
		p                                    Program
		degreeLevel, language, educationType string
	)

	err := s.Scan(
		&p.ID, &p.Code, &p.YoksisCode, &p.NameTR, &p.NameEN,
		&degreeLevel, &language, &educationType,
		&p.DurationSemesters, &p.MaxDurationYears, &p.TotalECTSRequired,
		&p.HasPrepClass, &p.IsActive, &p.CreatedAt, &p.UpdatedAt,
		&p.Department.ID, &p.Department.Code, &p.Department.NameTR,
		&p.Faculty.ID, &p.Faculty.Code, &p.Faculty.NameTR,
	)
	if err != nil {
		return Program{}, err
	}

	p.DegreeLevel = DegreeLevel(degreeLevel)
	p.Language = Language(language)
	p.EducationType = EducationType(educationType)
	return p, nil
}
