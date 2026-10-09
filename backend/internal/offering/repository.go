package offering

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// Repository, ders açma verisine erişir.
type Repository struct {
	pool *pgxpool.Pool
	db   db.Querier
}

// NewRepository, bir Repository oluşturur.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, db: pool}
}

type scanner interface{ Scan(dest ...any) error }

// --- Açılan dersler ------------------------------------------------------------

const offeringSelect = `
	SELECT o.id, o.term_id, t.code, o.status, coalesce(o.external_ref, ''), coalesce(o.note, ''), o.version,
	       c.id, c.code, c.name_tr, c.name_en, c.theory_hours, c.practice_hours, c.national_credit::float8, c.ects::float8, c.language,
	       d.id, d.code, d.name_tr, d.name_en, d.faculty_id,
	       (SELECT count(*) FROM offering.sections s WHERE s.offering_id = o.id AND s.status = 'ACTIVE'),
	       (SELECT coalesce(sum(s.capacity), 0) FROM offering.sections s WHERE s.offering_id = o.id AND s.status = 'ACTIVE'),
	       (SELECT coalesce(sum(s.enrolled_count), 0) FROM offering.sections s WHERE s.offering_id = o.id)
	FROM offering.course_offerings o
	JOIN academic.terms t ON t.id = o.term_id
	JOIN curriculum.courses c ON c.id = o.course_id
	JOIN org.departments d ON d.id = o.department_id`

func scanOffering(s scanner) (Offering, error) {
	var (
		o                Offering
		status           string
		theory, practice int16
	)
	err := s.Scan(&o.ID, &o.TermID, &o.TermCode, &status, &o.ExternalRef, &o.Note, &o.Version,
		&o.Course.ID, &o.Course.Code, &o.Course.NameTR, &o.Course.NameEN, &theory, &practice,
		&o.Course.NationalCredit, &o.Course.ECTS, &o.Course.Language,
		&o.Department.ID, &o.Department.Code, &o.Department.NameTR, &o.Department.NameEN, &o.FacultyID,
		&o.SectionCount, &o.TotalCapacity, &o.TotalEnrolled)
	o.Status, o.Course.TheoryHours, o.Course.PracticeHours = Status(status), int(theory), int(practice)
	return o, err
}

// OfferingCursor, kod sırasında bir sonraki sayfanın başlangıcıdır.
type OfferingCursor struct {
	Code string `json:"c"`
}

// OfferingFilter, dönemin açılan dersleri listesinin süzgecidir.
type OfferingFilter struct {
	TermID       string
	DepartmentID string
	Query        string // ders kodunda ya da adında geçen metin
	Status       Status
	After        *OfferingCursor
	Limit        int
}

// ListOfferings, dönemde açılan dersleri ders koduna göre sıralı döndürür.
func (r *Repository) ListOfferings(ctx context.Context, f OfferingFilter) ([]Offering, bool, error) {
	var w db.Where
	w.Add("o.term_id = $1", f.TermID)
	if f.DepartmentID != "" {
		w.Add("o.department_id = $1", f.DepartmentID)
	}
	if f.Query != "" {
		w.Add("(c.code || ' ' || c.name_tr || ' ' || c.name_en) ILIKE $1", "%"+f.Query+"%")
	}
	if f.Status != "" {
		w.Add("o.status = $1", string(f.Status))
	}
	if f.After != nil {
		w.Add("c.code > $1", f.After.Code)
	}
	limit := w.Arg(f.Limit + 1)
	rows, err := r.db.Query(ctx, offeringSelect+"\n"+w.SQL()+"\nORDER BY c.code LIMIT "+limit, w.Args()...)
	if err != nil {
		return nil, false, fmt.Errorf("offering: açılan dersler listelenemedi: %w", err)
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Offering, error) { return scanOffering(row) })
	if err != nil {
		return nil, false, fmt.Errorf("offering: açılan dersler okunamadı: %w", err)
	}
	if len(list) > f.Limit {
		return list[:f.Limit], true, nil
	}
	return list, false, nil
}

// Offering, açılan dersi döndürür. Yoksa ErrNotFound döner.
func (r *Repository) Offering(ctx context.Context, id string) (Offering, error) {
	return r.offering(ctx, r.db, offeringSelect+` WHERE o.id = $1`, id)
}

func (r *Repository) offering(ctx context.Context, q db.Querier, query string, args ...any) (Offering, error) {
	o, err := scanOffering(q.QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return Offering{}, ErrNotFound
	}
	if err != nil {
		return Offering{}, fmt.Errorf("offering: açılan ders okunamadı: %w", err)
	}
	return o, nil
}

// OfferingInput, ders açma verisidir.
type OfferingInput struct {
	CourseID     string
	DepartmentID string
	ExternalRef  string
	Note         string
}

// CreateOffering, dönemde bir ders açar. Ders bu dönemde zaten açılmışsa ErrConflict,
// dönem kapanmışsa ErrTermClosed döner.
func (r *Repository) CreateOffering(ctx context.Context, actorID, termID string, in OfferingInput) (string, error) {
	var id string
	err := db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		if err := termOpen(ctx, tx, termID); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `
			INSERT INTO offering.course_offerings (term_id, course_id, department_id, external_ref, note)
			VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			termID, in.CourseID, in.DepartmentID, nullable(in.ExternalRef), nullable(in.Note)).Scan(&id)
		switch {
		case db.IsConflict(err):
			return ErrConflict
		case db.IsForeignKeyViolation(err):
			return ErrUnknownReference
		case err != nil:
			return fmt.Errorf("offering: ders açılamadı: %w", err)
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "offering.create", EntityType: "course_offering", EntityID: id,
			After: map[string]any{"term_id": termID, "course_id": in.CourseID, "department_id": in.DepartmentID, "external_ref": in.ExternalRef},
		})
	})
	return id, err
}

// termOpen, dönemin var olduğunu ve kapanmadığını doğrular; dönem satırını işlem
// sonuna kadar paylaşımlı kilitler (bu arada kapatılamaz).
func termOpen(ctx context.Context, tx pgx.Tx, termID string) error {
	var status string
	err := tx.QueryRow(ctx, `SELECT status FROM academic.terms WHERE id = $1 FOR SHARE`, termID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("offering: dönem okunamadı: %w", err)
	}
	if status == "CLOSED" {
		return ErrTermClosed
	}
	return nil
}

// OfferingUpdate, açılan dersin değişebilen alanlarıdır.
type OfferingUpdate struct {
	Status      Status
	ExternalRef string
	Note        string
}

// UpdateOffering, açılan dersin durumunu ve notlarını günceller. İptal ya da planlamaya
// dönüş kayıtlı öğrenci varken yapılamaz; iptal edilen dersin şubeleri de iptal olur ve
// derslikleri boşalır.
func (r *Repository) UpdateOffering(ctx context.Context, actorID, id string, version int, in OfferingUpdate) error {
	return db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		before, err := r.offering(ctx, tx, offeringSelect+` WHERE o.id = $1 FOR UPDATE OF o`, id)
		if err != nil {
			return err
		}
		if before.Version != version {
			return ErrVersionMismatch
		}
		if !before.Status.CanBecome(in.Status) {
			return ErrInvalidTransition
		}
		if in.Status != before.Status && (in.Status == StatusCancelled || in.Status == StatusPlanned) && before.TotalEnrolled > 0 {
			return ErrHasEnrollments
		}
		if _, err := tx.Exec(ctx, `
			UPDATE offering.course_offerings
			SET status = $2, external_ref = $3, note = $4, version = version + 1, updated_at = now()
			WHERE id = $1`, id, string(in.Status), nullable(in.ExternalRef), nullable(in.Note)); err != nil {
			return fmt.Errorf("offering: açılan ders güncellenemedi: %w", err)
		}
		if in.Status == StatusCancelled && before.Status != StatusCancelled {
			if _, err := tx.Exec(ctx, `
				DELETE FROM offering.schedule_slots WHERE section_id IN (SELECT id FROM offering.sections WHERE offering_id = $1)`, id); err != nil {
				return fmt.Errorf("offering: oturumlar silinemedi: %w", err)
			}
			if _, err := tx.Exec(ctx, `
				UPDATE offering.sections SET status = 'CANCELLED', version = version + 1, updated_at = now()
				WHERE offering_id = $1 AND status <> 'CANCELLED'`, id); err != nil {
				return fmt.Errorf("offering: şubeler iptal edilemedi: %w", err)
			}
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "offering.update", EntityType: "course_offering", EntityID: id,
			Before: map[string]any{"status": before.Status, "external_ref": before.ExternalRef, "note": before.Note},
			After:  map[string]any{"status": in.Status, "external_ref": in.ExternalRef, "note": in.Note},
		})
	})
}

// DeleteOffering, planlama aşamasındaki ve kayıtlı öğrencisi olmayan bir açılışı şubeleriyle siler.
func (r *Repository) DeleteOffering(ctx context.Context, actorID, id string) error {
	return db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		before, err := r.offering(ctx, tx, offeringSelect+` WHERE o.id = $1 FOR UPDATE OF o`, id)
		if err != nil {
			return err
		}
		if before.Status != StatusPlanned {
			return ErrInvalidTransition
		}
		if before.TotalEnrolled > 0 {
			return ErrHasEnrollments
		}
		if _, err := tx.Exec(ctx, `DELETE FROM offering.course_offerings WHERE id = $1`, id); err != nil {
			return fmt.Errorf("offering: açılan ders silinemedi: %w", err)
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "offering.delete", EntityType: "course_offering", EntityID: id,
			Before: map[string]any{"term_id": before.TermID, "course_id": before.Course.ID, "department_id": before.Department.ID},
		})
	})
}

// --- Şubeler -------------------------------------------------------------------

const sectionSelect = `
	SELECT s.id, s.offering_id, s.section_code, s.capacity, s.enrolled_count, s.quota_mode, s.instruction_mode,
	       s.language, s.status, s.version
	FROM offering.sections s`

func scanSection(s scanner) (Section, error) {
	var sec Section
	err := s.Scan(&sec.ID, &sec.OfferingID, &sec.Code, &sec.Capacity, &sec.EnrolledCount, &sec.QuotaMode,
		&sec.InstructionMode, &sec.Language, &sec.Status, &sec.Version)
	sec.Instructors, sec.Quotas, sec.Slots = []Instructor{}, []Quota{}, []Slot{}
	return sec, err
}

// Sections, açılan dersin şubelerini öğretim elemanları, kontenjanları ve oturumlarıyla
// döndürür.
func (r *Repository) Sections(ctx context.Context, offeringID string) ([]Section, error) {
	rows, err := r.db.Query(ctx, sectionSelect+` WHERE s.offering_id = $1 ORDER BY length(s.section_code), s.section_code`, offeringID)
	if err != nil {
		return nil, fmt.Errorf("offering: şubeler okunamadı: %w", err)
	}
	sections, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Section, error) { return scanSection(row) })
	if err != nil {
		return nil, fmt.Errorf("offering: şubeler okunamadı: %w", err)
	}
	if err := r.fillSections(ctx, r.db, sections); err != nil {
		return nil, err
	}
	return sections, nil
}

// Section, şubeyi ayrıntılarıyla döndürür. Yoksa ErrNotFound döner.
func (r *Repository) Section(ctx context.Context, id string) (Section, error) {
	sec, err := scanSection(r.db.QueryRow(ctx, sectionSelect+` WHERE s.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Section{}, ErrNotFound
	}
	if err != nil {
		return Section{}, fmt.Errorf("offering: şube okunamadı: %w", err)
	}
	list := []Section{sec}
	if err := r.fillSections(ctx, r.db, list); err != nil {
		return Section{}, err
	}
	return list[0], nil
}

// fillSections, şubelerin öğretim elemanlarını, kontenjanlarını ve oturumlarını üç
// sorguyla doldurur.
func (r *Repository) fillSections(ctx context.Context, q db.Querier, sections []Section) error {
	if len(sections) == 0 {
		return nil
	}
	index := make(map[string]int, len(sections))
	ids := make([]string, len(sections))
	for i, s := range sections {
		index[s.ID], ids[i] = i, s.ID
	}

	instructors, err := sectionInstructors(ctx, q, ids)
	if err != nil {
		return err
	}
	for sectionID, list := range instructors {
		sections[index[sectionID]].Instructors = list
	}

	rows, err := q.Query(ctx, `
		SELECT sq.section_id, p.id, p.code, p.name_tr, p.name_en, sq.quota, sq.enrolled
		FROM offering.section_quotas sq JOIN org.programs p ON p.id = sq.program_id
		WHERE sq.section_id = ANY($1::uuid[]) ORDER BY p.code`, ids)
	if err != nil {
		return fmt.Errorf("offering: kontenjanlar okunamadı: %w", err)
	}
	var (
		sectionID string
		qt        Quota
	)
	if _, err := pgx.ForEachRow(rows, []any{&sectionID, &qt.Program.ID, &qt.Program.Code, &qt.Program.NameTR, &qt.Program.NameEN, &qt.Quota, &qt.Enrolled}, func() error {
		s := &sections[index[sectionID]]
		s.Quotas = append(s.Quotas, qt)
		return nil
	}); err != nil {
		return fmt.Errorf("offering: kontenjanlar okunamadı: %w", err)
	}

	slots, err := r.slots(ctx, q, `WHERE sl.section_id = ANY($1::uuid[])`, ids)
	if err != nil {
		return err
	}
	for _, sl := range slots {
		s := &sections[index[sl.SectionID]]
		s.Slots = append(s.Slots, sl)
	}
	return nil
}

func sectionInstructors(ctx context.Context, q db.Querier, sectionIDs []string) (map[string][]Instructor, error) {
	rows, err := q.Query(ctx, `
		SELECT si.section_id, st.id, st.staff_no, coalesce(t.name_tr, ''), pe.first_name, pe.last_name, si.role
		FROM offering.section_instructors si
		JOIN people.staff st ON st.id = si.staff_id
		JOIN people.persons pe ON pe.id = st.person_id
		LEFT JOIN people.academic_titles t ON t.code = st.academic_title_code
		WHERE si.section_id = ANY($1::uuid[])
		ORDER BY si.role = 'PRIMARY' DESC, si.role, pe.last_name, pe.first_name`, sectionIDs)
	if err != nil {
		return nil, fmt.Errorf("offering: öğretim elemanları okunamadı: %w", err)
	}
	out := map[string][]Instructor{}
	var (
		sectionID string
		in        Instructor
	)
	if _, err := pgx.ForEachRow(rows, []any{&sectionID, &in.StaffID, &in.StaffNo, &in.Title, &in.FirstName, &in.LastName, &in.Role}, func() error {
		out[sectionID] = append(out[sectionID], in)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("offering: öğretim elemanları okunamadı: %w", err)
	}
	return out, nil
}

// SectionInput, şube oluşturma ve güncelleme verisidir.
type SectionInput struct {
	Code            string // sadece oluşturmada
	Capacity        int
	QuotaMode       string
	InstructionMode string
	Language        string
	Status          string // sadece güncellemede: ACTIVE ya da CANCELLED
}

// CreateSection, açılan derse şube ekler. Aynı kodla şube varsa ErrConflict döner.
func (r *Repository) CreateSection(ctx context.Context, actorID, offeringID string, in SectionInput) (string, error) {
	var id string
	err := db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		o, err := r.offering(ctx, tx, offeringSelect+` WHERE o.id = $1 FOR UPDATE OF o`, offeringID)
		if err != nil {
			return err
		}
		if o.Status == StatusCancelled || o.Status == StatusClosed {
			return ErrInvalidTransition
		}
		if err := termOpen(ctx, tx, o.TermID); err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `
			INSERT INTO offering.sections (offering_id, section_code, capacity, quota_mode, instruction_mode, language)
			VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
			offeringID, in.Code, in.Capacity, in.QuotaMode, in.InstructionMode, in.Language).Scan(&id)
		if db.IsConflict(err) {
			return ErrConflict
		}
		if err != nil {
			return fmt.Errorf("offering: şube oluşturulamadı: %w", err)
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "section.create", EntityType: "section", EntityID: id,
			After: map[string]any{"offering_id": offeringID, "section_code": in.Code, "capacity": in.Capacity, "quota_mode": in.QuotaMode},
		})
	})
	return id, err
}

// UpdateSection, şubenin kontenjanını ve öğretim biçimini günceller. Kontenjan kayıtlı
// öğrencinin ve program kontenjanları toplamının altına inemez; teorik oturumların
// derslikleri yeni kontenjanı almalıdır. İptal edilen şubenin oturumları silinir.
func (r *Repository) UpdateSection(ctx context.Context, actorID, id string, version int, in SectionInput) error {
	return db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		before, err := scanSection(tx.QueryRow(ctx, sectionSelect+` WHERE s.id = $1 FOR UPDATE`, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("offering: şube okunamadı: %w", err)
		}
		if before.Version != version {
			return ErrVersionMismatch
		}
		if before.Status == "CANCELLED" && in.Status != "CANCELLED" {
			return ErrSectionCancelled
		}
		if in.Status == "CANCELLED" && before.EnrolledCount > 0 {
			return ErrHasEnrollments
		}
		if in.Capacity < before.EnrolledCount {
			return ErrBelowEnrolled
		}
		var reserved int
		if err := tx.QueryRow(ctx, `SELECT coalesce(sum(quota), 0) FROM offering.section_quotas WHERE section_id = $1`, id).Scan(&reserved); err != nil {
			return fmt.Errorf("offering: kontenjanlar okunamadı: %w", err)
		}
		if reserved > in.Capacity {
			return ErrQuotaExceeds
		}
		if in.Status != "CANCELLED" {
			if err := classroomsFit(ctx, tx, id, in.Capacity); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `
			UPDATE offering.sections
			SET capacity = $2, quota_mode = $3, instruction_mode = $4, language = $5, status = $6,
			    version = version + 1, updated_at = now()
			WHERE id = $1`, id, in.Capacity, in.QuotaMode, in.InstructionMode, in.Language, in.Status); err != nil {
			return fmt.Errorf("offering: şube güncellenemedi: %w", err)
		}
		if in.Status == "CANCELLED" && before.Status != "CANCELLED" {
			if _, err := tx.Exec(ctx, `DELETE FROM offering.schedule_slots WHERE section_id = $1`, id); err != nil {
				return fmt.Errorf("offering: oturumlar silinemedi: %w", err)
			}
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "section.update", EntityType: "section", EntityID: id,
			Before: map[string]any{"capacity": before.Capacity, "quota_mode": before.QuotaMode, "instruction_mode": before.InstructionMode, "status": before.Status},
			After:  map[string]any{"capacity": in.Capacity, "quota_mode": in.QuotaMode, "instruction_mode": in.InstructionMode, "status": in.Status},
		})
	})
}

// DeleteSection, kayıtlı öğrencisi olmayan şubeyi siler.
func (r *Repository) DeleteSection(ctx context.Context, actorID, id string) error {
	return db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		var offeringID, code string
		var enrolled int
		err := tx.QueryRow(ctx, `SELECT offering_id, section_code, enrolled_count FROM offering.sections WHERE id = $1 FOR UPDATE`, id).
			Scan(&offeringID, &code, &enrolled)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("offering: şube okunamadı: %w", err)
		}
		if enrolled > 0 {
			return ErrHasEnrollments
		}
		if _, err := tx.Exec(ctx, `DELETE FROM offering.sections WHERE id = $1`, id); err != nil {
			return fmt.Errorf("offering: şube silinemedi: %w", err)
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "section.delete", EntityType: "section", EntityID: id,
			Before: map[string]any{"offering_id": offeringID, "section_code": code},
		})
	})
}

// QuotaInput, bir programa ayrılan kontenjandır.
type QuotaInput struct {
	ProgramID string
	Quota     int
}

// SetQuotas, şubenin program kontenjanlarını verilen kümeyle değiştirir. Toplam şube
// kontenjanını aşamaz; bir programın kontenjanı o programdan kayıtlı öğrencinin altına inemez.
func (r *Repository) SetQuotas(ctx context.Context, actorID, sectionID string, items []QuotaInput) error {
	return db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		var capacity int
		var status string
		err := tx.QueryRow(ctx, `SELECT capacity, status FROM offering.sections WHERE id = $1 FOR UPDATE`, sectionID).Scan(&capacity, &status)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("offering: şube okunamadı: %w", err)
		}
		if status == "CANCELLED" {
			return ErrSectionCancelled
		}
		total := 0
		for _, it := range items {
			total += it.Quota
		}
		if total > capacity {
			return ErrQuotaExceeds
		}
		// Ders seçmeyle dolan sayılar korunur: öğrencisi olan programın kontenjanı ne
		// kaldırılabilir ne de kayıtlı sayının altına indirilebilir.
		enrolled := map[string]int{}
		rows, err := tx.Query(ctx, `SELECT program_id, enrolled FROM offering.section_quotas WHERE section_id = $1`, sectionID)
		if err != nil {
			return fmt.Errorf("offering: kontenjanlar okunamadı: %w", err)
		}
		var (
			programID string
			n         int
		)
		if _, err := pgx.ForEachRow(rows, []any{&programID, &n}, func() error { enrolled[programID] = n; return nil }); err != nil {
			return fmt.Errorf("offering: kontenjanlar okunamadı: %w", err)
		}
		kept := map[string]bool{}
		for _, it := range items {
			if it.Quota < enrolled[it.ProgramID] {
				return ErrBelowEnrolled
			}
			kept[it.ProgramID] = true
		}
		for programID, n := range enrolled {
			if n > 0 && !kept[programID] {
				return ErrBelowEnrolled
			}
		}

		if _, err := tx.Exec(ctx, `DELETE FROM offering.section_quotas WHERE section_id = $1`, sectionID); err != nil {
			return fmt.Errorf("offering: kontenjanlar silinemedi: %w", err)
		}
		after := make([]map[string]any, 0, len(items))
		for _, it := range items {
			_, err := tx.Exec(ctx, `
				INSERT INTO offering.section_quotas (section_id, program_id, quota, enrolled)
				VALUES ($1, $2, $3, $4)`, sectionID, it.ProgramID, it.Quota, enrolled[it.ProgramID])
			switch {
			case db.IsForeignKeyViolation(err):
				return ErrUnknownReference
			case db.IsConflict(err):
				return ErrConflict
			case err != nil:
				return fmt.Errorf("offering: kontenjan eklenemedi: %w", err)
			}
			after = append(after, map[string]any{"program_id": it.ProgramID, "quota": it.Quota})
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "section.set_quotas", EntityType: "section", EntityID: sectionID,
			After: map[string]any{"quotas": after},
		})
	})
}
