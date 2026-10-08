package org

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository, org şemasına erişen veri katmanıdır.
type Repository struct {
	db *pgxpool.Pool
}

// NewRepository, verilen bağlantı havuzunu kullanan bir Repository oluşturur.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

const facultySelect = `
	SELECT f.id, f.code, f.name_tr, f.name_en, f.unit_type, f.is_active,
	       f.created_at, f.updated_at,
	       c.id, c.code, c.name
	FROM org.faculties f
	LEFT JOIN org.campuses c ON c.id = f.campus_id`

// ListFaculties, filtreye uyan birimleri Türkçe ada göre sıralı döndürür.
func (r *Repository) ListFaculties(ctx context.Context, filter FacultyFilter) ([]Faculty, error) {
	var (
		conds []string
		args  []any
	)

	if !filter.IncludeInactive {
		conds = append(conds, "f.is_active")
	}
	if filter.UnitType != "" {
		args = append(args, string(filter.UnitType))
		conds = append(conds, fmt.Sprintf("f.unit_type = $%d", len(args)))
	}
	if filter.Query != "" {
		args = append(args, "%"+filter.Query+"%")
		conds = append(conds, fmt.Sprintf(
			"(f.code ILIKE $%[1]d OR f.name_tr ILIKE $%[1]d OR f.name_en ILIKE $%[1]d)", len(args)))
	}

	query := facultySelect
	if len(conds) > 0 {
		query += "\n\tWHERE " + strings.Join(conds, " AND ")
	}
	query += "\n\tORDER BY f.name_tr COLLATE \"tr-x-icu\""

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("org: birimler listelenemedi: %w", err)
	}
	defer rows.Close()

	faculties := []Faculty{}
	for rows.Next() {
		f, err := scanFaculty(rows)
		if err != nil {
			return nil, fmt.Errorf("org: birim okunamadı: %w", err)
		}
		faculties = append(faculties, f)
	}
	if err := rows.Err(); err != nil {
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

// scanner, pgx.Row ve pgx.Rows'un ortak Scan metodudur. Böylece tek satır ve
// çok satır sorguları aynı eşleme fonksiyonunu kullanabilir.
type scanner interface {
	Scan(dest ...any) error
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
