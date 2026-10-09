package org

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// Targets, birimlerin yetki kapsamı hedeflerini çözer: bir program için bölümünü ve
// fakültesini, bir bölüm için fakültesini. Kapsamlı yetki kontrolleri (authz.Allows)
// hedefin bütün üst birimlerini ister. Transaction içinde de kullanılabilir.
type Targets struct {
	db db.Querier
}

// NewTargets, verilen bağlantıyı (havuz veya transaction) kullanan bir Targets oluşturur.
func NewTargets(q db.Querier) *Targets {
	return &Targets{db: q}
}

// Faculty, birimin hedefini döndürür. Birim yoksa ErrNotFound döner.
func (t *Targets) Faculty(ctx context.Context, facultyID string) (authz.Target, error) {
	var exists bool
	err := t.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM org.faculties WHERE id = $1)`, facultyID).Scan(&exists)
	if err != nil {
		return authz.Target{}, fmt.Errorf("org: birim okunamadı: %w", err)
	}
	if !exists {
		return authz.Target{}, ErrNotFound
	}
	return authz.Target{FacultyID: facultyID}, nil
}

// Department, bölümün hedefini (bölüm + fakülte) döndürür. Bölüm yoksa ErrNotFound döner.
func (t *Targets) Department(ctx context.Context, departmentID string) (authz.Target, error) {
	target := authz.Target{DepartmentID: departmentID}
	err := t.db.QueryRow(ctx, `SELECT faculty_id FROM org.departments WHERE id = $1`, departmentID).Scan(&target.FacultyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return authz.Target{}, ErrNotFound
	}
	if err != nil {
		return authz.Target{}, fmt.Errorf("org: bölüm okunamadı: %w", err)
	}
	return target, nil
}

// Program, programın hedefini (program + bölüm + fakülte) döndürür. Program yoksa
// ErrNotFound döner.
func (t *Targets) Program(ctx context.Context, programID string) (authz.Target, error) {
	target := authz.Target{ProgramID: programID}
	err := t.db.QueryRow(ctx, `
		SELECT d.faculty_id, d.id FROM org.programs p JOIN org.departments d ON d.id = p.department_id
		WHERE p.id = $1`, programID).Scan(&target.FacultyID, &target.DepartmentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return authz.Target{}, ErrNotFound
	}
	if err != nil {
		return authz.Target{}, fmt.Errorf("org: program okunamadı: %w", err)
	}
	return target, nil
}
