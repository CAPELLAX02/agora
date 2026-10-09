package curriculum

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// Müfredat hataları.
var (
	ErrNotDraft          = errors.New("curriculum: müfredat taslak değil")
	ErrNotActive         = errors.New("curriculum: müfredat yürürlükte değil")
	ErrCurriculumOverlap = errors.New("curriculum: giriş yılları yürürlükteki başka bir sürümle çakışıyor")
	ErrCurriculumInUse   = errors.New("curriculum: müfredata bağlı öğrenciler var")
	ErrEmptyCurriculum   = errors.New("curriculum: müfredatta ders yok")
	ErrSemesterRange     = errors.New("curriculum: yarıyıl programın süresini aşıyor")
)

// TotalMismatchError, yürürlüğe girecek müfredatın AKTS toplamı gerekenle tutmadığında döner.
type TotalMismatchError struct {
	Required, Actual float64
}

func (e *TotalMismatchError) Error() string {
	return fmt.Sprintf("curriculum: AKTS toplamı %.1f, gereken %.1f", e.Actual, e.Required)
}

// CurriculumStatus, müfredat sürümünün durumudur.
type CurriculumStatus string

// Müfredat durumları.
const (
	CurriculumDraft    CurriculumStatus = "DRAFT"
	CurriculumActive   CurriculumStatus = "ACTIVE"
	CurriculumArchived CurriculumStatus = "ARCHIVED"
)

// ItemType, müfredat satırının türüdür.
type ItemType string

// Satır türleri.
const (
	ItemCourse       ItemType = "COURSE"
	ItemElectiveSlot ItemType = "ELECTIVE_SLOT"
)

// ProgramRef, bir programın özetidir. Bölüm ve fakülte yetki kontrolü içindir.
type ProgramRef struct {
	ID                string
	Code              string
	NameTR            string
	NameEN            string
	DepartmentID      string
	FacultyID         string
	DurationSemesters int
}

// Curriculum, bir programın müfredat sürümüdür.
type Curriculum struct {
	ID                string
	Program           ProgramRef
	NameTR            string
	NameEN            string
	EffectiveFromYear int
	EffectiveToYear   *int
	TotalECTSRequired float64
	Status            CurriculumStatus
	DecisionRef       string
	CopiedFromID      string
	ActivatedAt       *time.Time
	ArchivedAt        *time.Time
	StudentCount      int
	Version           int
}

// CurriculumItem, müfredatın bir satırıdır: yarıyıla yerleşmiş bir ders ya da seçmeli
// ders yuvası. Saat, kredi ve AKTS ders satırında dersten, yuvada yuvanın kendisinden gelir.
type CurriculumItem struct {
	ID               string
	SemesterNo       int
	Type             ItemType
	Course           *CourseRef
	CourseKind       CourseKind
	HasPrerequisites bool
	Group            *GroupRef
	TheoryHours      int
	PracticeHours    int
	NationalCredit   float64
	ECTS             float64
	IsCompulsory     bool
	Position         int
}

// Code, satırın gösterilen kodudur: dersin ya da havuzun kodu.
func (it CurriculumItem) Code() string {
	if it.Course != nil {
		return it.Course.Code
	}
	if it.Group != nil {
		return it.Group.Code
	}
	return ""
}

// SemesterSummary, bir yarıyılın toplamlarıdır.
type SemesterSummary struct {
	SemesterNo     int
	ECTS           float64
	NationalCredit float64
	ItemCount      int
}

// KindSummary, bir seçmeli grup türünün toplam AKTS'sidir.
type KindSummary struct {
	Kind GroupKind
	ECTS float64
}

// CurriculumSummary, müfredatın mezuniyet açısından özetidir.
type CurriculumSummary struct {
	TotalECTS      float64
	TotalCredit    float64
	CompulsoryECTS float64
	ElectiveECTS   float64
	Semesters      []SemesterSummary
	ElectiveKinds  []KindSummary
}

// Summarize, satırlardan müfredat özetini hesaplar.
func Summarize(items []CurriculumItem) CurriculumSummary {
	var s CurriculumSummary
	semesters := map[int]*SemesterSummary{}
	kinds := map[GroupKind]float64{}
	for _, it := range items {
		s.TotalECTS += it.ECTS
		s.TotalCredit += it.NationalCredit
		if it.IsCompulsory {
			s.CompulsoryECTS += it.ECTS
		} else {
			s.ElectiveECTS += it.ECTS
		}
		if it.Group != nil {
			kinds[it.Group.Kind] += it.ECTS
		}
		sem := semesters[it.SemesterNo]
		if sem == nil {
			sem = &SemesterSummary{SemesterNo: it.SemesterNo}
			semesters[it.SemesterNo] = sem
		}
		sem.ECTS += it.ECTS
		sem.NationalCredit += it.NationalCredit
		sem.ItemCount++
	}
	for _, sem := range semesters {
		s.Semesters = append(s.Semesters, *sem)
	}
	slices.SortFunc(s.Semesters, func(a, b SemesterSummary) int { return cmp.Compare(a.SemesterNo, b.SemesterNo) })
	for kind, ects := range kinds {
		s.ElectiveKinds = append(s.ElectiveKinds, KindSummary{Kind: kind, ECTS: ects})
	}
	slices.SortFunc(s.ElectiveKinds, func(a, b KindSummary) int { return cmp.Compare(a.Kind, b.Kind) })
	return s
}

// ProgramCurriculumRef, öğrencinin bir program kaydı ve izlediği müfredattır.
type ProgramCurriculumRef struct {
	StudentProgramID string
	Program          ProgramRef
	EnrollmentKind   string
	AdmissionYear    int
	CurriculumID     string // bağlı değilse boş
}

// --- Okuma ---------------------------------------------------------------------

const curriculumSelect = `
	SELECT c.id, c.name_tr, c.name_en, c.effective_from_year, c.effective_to_year, c.total_ects_required, c.status,
	       coalesce(c.decision_ref, ''), coalesce(c.copied_from_id::text, ''), c.activated_at, c.archived_at, c.version,
	       (SELECT count(*) FROM enrollment.student_programs sp WHERE sp.curriculum_id = c.id),
	       p.id, p.code, p.name_tr, p.name_en, p.department_id, d.faculty_id, p.duration_semesters
	FROM curriculum.curricula c
	JOIN org.programs p ON p.id = c.program_id
	JOIN org.departments d ON d.id = p.department_id`

func scanCurriculum(s scanner) (Curriculum, error) {
	var (
		c        Curriculum
		from     int16
		to       *int16
		status   string
		duration int16
	)
	err := s.Scan(&c.ID, &c.NameTR, &c.NameEN, &from, &to, &c.TotalECTSRequired, &status,
		&c.DecisionRef, &c.CopiedFromID, &c.ActivatedAt, &c.ArchivedAt, &c.Version, &c.StudentCount,
		&c.Program.ID, &c.Program.Code, &c.Program.NameTR, &c.Program.NameEN, &c.Program.DepartmentID, &c.Program.FacultyID, &duration)
	c.EffectiveFromYear, c.Status, c.Program.DurationSemesters = int(from), CurriculumStatus(status), int(duration)
	if to != nil {
		v := int(*to)
		c.EffectiveToYear = &v
	}
	return c, err
}

// ProgramCurricula, programın müfredat sürümlerini en yeniden eskiye döndürür. Taslaklar
// sadece includeDrafts doğruysa döner.
func (r *Repository) ProgramCurricula(ctx context.Context, programID string, includeDrafts bool) ([]Curriculum, error) {
	rows, err := r.db.Query(ctx, curriculumSelect+`
		WHERE c.program_id = $1 AND ($2 OR c.status <> 'DRAFT')
		ORDER BY c.effective_from_year DESC, c.created_at DESC`, programID, includeDrafts)
	if err != nil {
		return nil, fmt.Errorf("curriculum: müfredatlar listelenemedi: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Curriculum, error) { return scanCurriculum(row) })
}

// Curriculum, müfredat sürümünü döndürür. Yoksa ErrNotFound döner.
func (r *Repository) Curriculum(ctx context.Context, id string) (Curriculum, error) {
	return r.curriculum(ctx, r.db, curriculumSelect+` WHERE c.id = $1`, id)
}

func (r *Repository) curriculum(ctx context.Context, q db.Querier, query string, args ...any) (Curriculum, error) {
	c, err := scanCurriculum(q.QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return Curriculum{}, ErrNotFound
	}
	if err != nil {
		return Curriculum{}, fmt.Errorf("curriculum: müfredat okunamadı: %w", err)
	}
	return c, nil
}

// lockCurriculum, müfredat satırını transaction sonuna kadar kilitler.
func (r *Repository) lockCurriculum(ctx context.Context, tx pgx.Tx, id string) (Curriculum, error) {
	return r.curriculum(ctx, tx, curriculumSelect+` WHERE c.id = $1 FOR UPDATE OF c`, id)
}

// CurriculumItems, müfredatın satırlarını yarıyıl ve sıraya göre döndürür.
func (r *Repository) CurriculumItems(ctx context.Context, curriculumID string) ([]CurriculumItem, error) {
	return r.curriculumItems(ctx, r.db, curriculumID)
}

func (r *Repository) curriculumItems(ctx context.Context, q db.Querier, curriculumID string) ([]CurriculumItem, error) {
	rows, err := q.Query(ctx, `
		SELECT i.id, i.semester_no, i.item_type, i.is_compulsory, i.position,
		       c.id, c.code, c.name_tr, c.name_en, c.course_kind,
		       EXISTS (SELECT 1 FROM curriculum.course_prerequisites cp WHERE cp.course_id = c.id),
		       g.id, g.code, g.name_tr, g.name_en, g.group_kind,
		       coalesce(c.theory_hours, i.slot_theory_hours), coalesce(c.practice_hours, i.slot_practice_hours),
		       coalesce(c.national_credit, i.slot_national_credit), coalesce(c.ects, i.slot_ects)
		FROM curriculum.curriculum_items i
		LEFT JOIN curriculum.courses c ON c.id = i.course_id
		LEFT JOIN curriculum.elective_groups g ON g.id = i.elective_group_id
		WHERE i.curriculum_id = $1
		ORDER BY i.semester_no, i.position, coalesce(c.code, g.code), i.id`, curriculumID)
	if err != nil {
		return nil, fmt.Errorf("curriculum: müfredat satırları okunamadı: %w", err)
	}
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (CurriculumItem, error) {
		var (
			it                                   CurriculumItem
			semester, position, theory, practice int16
			itemType                             string
			cID, cCode, cTR, cEN, cKind          *string
			hasPre                               bool
			gID, gCode, gTR, gEN, gKind          *string
		)
		err := row.Scan(&it.ID, &semester, &itemType, &it.IsCompulsory, &position,
			&cID, &cCode, &cTR, &cEN, &cKind, &hasPre,
			&gID, &gCode, &gTR, &gEN, &gKind,
			&theory, &practice, &it.NationalCredit, &it.ECTS)
		it.SemesterNo, it.Position, it.Type = int(semester), int(position), ItemType(itemType)
		it.TheoryHours, it.PracticeHours = int(theory), int(practice)
		if cID != nil {
			it.Course = &CourseRef{ID: *cID, Code: *cCode, NameTR: *cTR, NameEN: *cEN, ECTS: it.ECTS}
			it.CourseKind, it.HasPrerequisites = CourseKind(*cKind), hasPre
		}
		if gID != nil {
			it.Group = &GroupRef{ID: *gID, Code: *gCode, NameTR: *gTR, NameEN: *gEN, Kind: GroupKind(*gKind)}
		}
		return it, err
	})
	if err != nil {
		return nil, fmt.Errorf("curriculum: müfredat satırları okunamadı: %w", err)
	}
	return items, nil
}

// StudentCurricula, kullanıcının öğrenci olarak program kayıtlarını ve izlediği müfredat
// sürümlerini döndürür. Kullanıcı öğrenci değilse boş liste döner.
func (r *Repository) StudentCurricula(ctx context.Context, userID string) ([]ProgramCurriculumRef, error) {
	rows, err := r.db.Query(ctx, `
		SELECT sp.id, sp.enrollment_kind, sp.admission_year, coalesce(sp.curriculum_id::text, ''),
		       p.id, p.code, p.name_tr, p.name_en, p.department_id, d.faculty_id, p.duration_semesters
		FROM iam.users u
		JOIN people.students s ON s.person_id = u.person_id
		JOIN enrollment.student_programs sp ON sp.student_id = s.id
		JOIN org.programs p ON p.id = sp.program_id
		JOIN org.departments d ON d.id = p.department_id
		WHERE u.id = $1
		ORDER BY sp.enrollment_kind = 'MAJOR' DESC, sp.admitted_on`, userID)
	if err != nil {
		return nil, fmt.Errorf("curriculum: öğrencinin programları okunamadı: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (ProgramCurriculumRef, error) {
		var (
			ref            ProgramCurriculumRef
			year, duration int16
		)
		err := row.Scan(&ref.StudentProgramID, &ref.EnrollmentKind, &year, &ref.CurriculumID,
			&ref.Program.ID, &ref.Program.Code, &ref.Program.NameTR, &ref.Program.NameEN,
			&ref.Program.DepartmentID, &ref.Program.FacultyID, &duration)
		ref.AdmissionYear, ref.Program.DurationSemesters = int(year), int(duration)
		return ref, err
	})
}

// --- Sürüm yaşam döngüsü -------------------------------------------------------

// CurriculumInput, müfredat sürümü oluşturma ve güncelleme verisidir.
type CurriculumInput struct {
	NameTR            string
	NameEN            string
	EffectiveFromYear int
	EffectiveToYear   *int
	TotalECTSRequired float64
	DecisionRef       string
}

func (in CurriculumInput) audit() map[string]any {
	return map[string]any{
		"name_tr": in.NameTR, "name_en": in.NameEN, "effective_from_year": in.EffectiveFromYear,
		"effective_to_year": in.EffectiveToYear, "total_ects_required": in.TotalECTSRequired, "decision_ref": in.DecisionRef,
	}
}

func (c Curriculum) input() CurriculumInput {
	return CurriculumInput{
		NameTR: c.NameTR, NameEN: c.NameEN, EffectiveFromYear: c.EffectiveFromYear, EffectiveToYear: c.EffectiveToYear,
		TotalECTSRequired: c.TotalECTSRequired, DecisionRef: c.DecisionRef,
	}
}

// CreateCurriculum, programa taslak bir müfredat sürümü ekler. copyFromID verilirse o
// sürümün satırları kopyalanır (yeni sürüm genellikle bir öncekinden türetilir).
func (r *Repository) CreateCurriculum(ctx context.Context, actorID, programID string, in CurriculumInput, copyFromID string) (string, error) {
	var id string
	err := db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			INSERT INTO curriculum.curricula (program_id, name_tr, name_en, effective_from_year, effective_to_year,
			                                  total_ects_required, decision_ref, copied_from_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
			programID, in.NameTR, in.NameEN, in.EffectiveFromYear, in.EffectiveToYear, in.TotalECTSRequired,
			nullable(in.DecisionRef), nullable(copyFromID)).Scan(&id)
		if err := writeError(err, "müfredat oluşturulamadı"); err != nil {
			return err
		}
		copied := 0
		if copyFromID != "" {
			tag, err := tx.Exec(ctx, `
				INSERT INTO curriculum.curriculum_items
				    (curriculum_id, semester_no, item_type, course_id, elective_group_id, slot_theory_hours, slot_practice_hours,
				     slot_national_credit, slot_ects, is_compulsory, position)
				SELECT $1, semester_no, item_type, course_id, elective_group_id, slot_theory_hours, slot_practice_hours,
				       slot_national_credit, slot_ects, is_compulsory, position
				FROM curriculum.curriculum_items WHERE curriculum_id = $2`, id, copyFromID)
			if err != nil {
				return fmt.Errorf("curriculum: satırlar kopyalanamadı: %w", err)
			}
			copied = int(tag.RowsAffected())
		}
		after := in.audit()
		after["program_id"], after["copied_from_id"], after["copied_items"] = programID, copyFromID, copied
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "curriculum.create", EntityType: "curriculum", EntityID: id, After: after,
		})
	})
	return id, err
}

// UpdateCurriculum, sürümün bilgilerini günceller. Taslakta her alan değişebilir.
// Yürürlükteki sürümde giriş yılı başlangıcı ve AKTS toplamı donmuştur: sadece adlar,
// karar bilgisi ve yeni girişlere kapanış yılı değişir. Arşivlenmiş sürüm değişmez.
func (r *Repository) UpdateCurriculum(ctx context.Context, actorID, id string, version int, in CurriculumInput) error {
	return db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		before, err := r.lockCurriculum(ctx, tx, id)
		if err != nil {
			return err
		}
		if before.Version != version {
			return ErrVersionMismatch
		}
		switch before.Status {
		case CurriculumArchived:
			return ErrNotDraft
		case CurriculumActive:
			if in.EffectiveFromYear != before.EffectiveFromYear || in.TotalECTSRequired != before.TotalECTSRequired {
				return ErrNotDraft
			}
			// Kapanış yılı öne çekilirse bağlı öğrenciler aralık dışında kalmamalı.
			if in.EffectiveToYear != nil {
				var outside bool
				if err := tx.QueryRow(ctx, `
					SELECT EXISTS (SELECT 1 FROM enrollment.student_programs WHERE curriculum_id = $1 AND admission_year > $2)`,
					id, *in.EffectiveToYear).Scan(&outside); err != nil {
					return fmt.Errorf("curriculum: bağlı öğrenciler okunamadı: %w", err)
				}
				if outside {
					return ErrCurriculumInUse
				}
			}
		}
		_, err = tx.Exec(ctx, `
			UPDATE curriculum.curricula
			SET name_tr = $2, name_en = $3, effective_from_year = $4, effective_to_year = $5, total_ects_required = $6,
			    decision_ref = $7, version = version + 1, updated_at = now()
			WHERE id = $1`, id, in.NameTR, in.NameEN, in.EffectiveFromYear, in.EffectiveToYear, in.TotalECTSRequired, nullable(in.DecisionRef))
		if db.IsExclusionViolation(err) {
			return ErrCurriculumOverlap
		}
		if err != nil {
			return fmt.Errorf("curriculum: müfredat güncellenemedi: %w", err)
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "curriculum.update", EntityType: "curriculum", EntityID: id,
			Before: before.input().audit(), After: in.audit(),
		})
	})
}

// DeleteCurriculum, taslak bir sürümü satırlarıyla siler.
func (r *Repository) DeleteCurriculum(ctx context.Context, actorID, id string) error {
	return db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		before, err := r.lockCurriculum(ctx, tx, id)
		if err != nil {
			return err
		}
		if before.Status != CurriculumDraft {
			return ErrNotDraft
		}
		// Bu taslaktan kopyalanmış sürümlerin bağlantısı ON DELETE SET NULL ile düşer.
		if _, err := tx.Exec(ctx, `DELETE FROM curriculum.curricula WHERE id = $1`, id); err != nil {
			return fmt.Errorf("curriculum: müfredat silinemedi: %w", err)
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "curriculum.delete", EntityType: "curriculum", EntityID: id,
			Before: before.input().audit(),
		})
	})
}

// Activate, taslak sürümü yürürlüğe koyar. Satırların AKTS toplamı gerekenle tutmalı,
// yarıyıllar programın süresini aşmamalı ve giriş yılları yürürlükteki başka bir sürümle
// çakışmamalıdır. Giriş yılı aralığındaki bağlantısız öğrenci kayıtları bu sürüme
// bağlanır; bağlanan kayıt sayısı döner.
func (r *Repository) Activate(ctx context.Context, actorID, id string) (int, error) {
	var linked int
	err := db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		c, err := r.lockCurriculum(ctx, tx, id)
		if err != nil {
			return err
		}
		if c.Status != CurriculumDraft {
			return ErrNotDraft
		}
		items, err := r.curriculumItems(ctx, tx, id)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return ErrEmptyCurriculum
		}
		var total float64
		for _, it := range items {
			if it.SemesterNo > c.Program.DurationSemesters {
				return ErrSemesterRange
			}
			total += it.ECTS
		}
		// AKTS değerleri tek ondalıklı: karşılaştırma onda birlik tam sayılarla yapılır.
		if roundTenth(total) != roundTenth(c.TotalECTSRequired) {
			return &TotalMismatchError{Required: c.TotalECTSRequired, Actual: total}
		}

		_, err = tx.Exec(ctx, `
			UPDATE curriculum.curricula SET status = 'ACTIVE', activated_at = now(), version = version + 1, updated_at = now()
			WHERE id = $1`, id)
		if db.IsExclusionViolation(err) {
			return ErrCurriculumOverlap
		}
		if err != nil {
			return fmt.Errorf("curriculum: müfredat yürürlüğe konamadı: %w", err)
		}
		tag, err := tx.Exec(ctx, `
			UPDATE enrollment.student_programs
			SET curriculum_id = $1, version = version + 1, updated_at = now()
			WHERE program_id = $2 AND curriculum_id IS NULL
			  AND admission_year >= $3 AND ($4::smallint IS NULL OR admission_year <= $4)`,
			id, c.Program.ID, c.EffectiveFromYear, c.EffectiveToYear)
		if err != nil {
			return fmt.Errorf("curriculum: öğrenciler bağlanamadı: %w", err)
		}
		linked = int(tag.RowsAffected())
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "curriculum.activate", EntityType: "curriculum", EntityID: id,
			Before: map[string]any{"status": c.Status},
			After:  map[string]any{"status": CurriculumActive, "total_ects": total, "linked_students": linked},
		})
	})
	return linked, err
}

// Archive, yürürlükteki sürümü arşivler. Sürümü izleyen ve öğrenimi süren (mezun,
// kaydı silinmiş ya da ilişiği kesilmiş olmayan) öğrenci varsa ErrCurriculumInUse döner.
func (r *Repository) Archive(ctx context.Context, actorID, id string) error {
	return db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		c, err := r.lockCurriculum(ctx, tx, id)
		if err != nil {
			return err
		}
		if c.Status != CurriculumActive {
			return ErrNotActive
		}
		var inUse bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM enrollment.student_programs
			               WHERE curriculum_id = $1 AND status NOT IN ('GRADUATED', 'WITHDRAWN', 'DISMISSED'))`, id).Scan(&inUse); err != nil {
			return fmt.Errorf("curriculum: bağlı öğrenciler okunamadı: %w", err)
		}
		if inUse {
			return ErrCurriculumInUse
		}
		if _, err := tx.Exec(ctx, `
			UPDATE curriculum.curricula SET status = 'ARCHIVED', archived_at = now(), version = version + 1, updated_at = now()
			WHERE id = $1`, id); err != nil {
			return fmt.Errorf("curriculum: müfredat arşivlenemedi: %w", err)
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "curriculum.archive", EntityType: "curriculum", EntityID: id,
			Before: map[string]any{"status": c.Status}, After: map[string]any{"status": CurriculumArchived},
		})
	})
}

func roundTenth(v float64) int64 {
	if v < 0 {
		return int64(v*10 - 0.5)
	}
	return int64(v*10 + 0.5)
}

// --- Satırlar ------------------------------------------------------------------

// ItemInput, müfredat satırı ekleme ve güncelleme verisidir. Ders satırında CourseID,
// yuvada GroupID ve yuvanın saat, kredi ve AKTS değerleri dolu olur.
type ItemInput struct {
	SemesterNo     int
	Type           ItemType
	CourseID       string
	GroupID        string
	TheoryHours    int
	PracticeHours  int
	NationalCredit float64
	ECTS           float64
	IsCompulsory   bool
	Position       int
}

func (in ItemInput) args() []any {
	if in.Type == ItemCourse {
		return []any{in.SemesterNo, string(in.Type), in.CourseID, nil, nil, nil, nil, nil, in.IsCompulsory, in.Position}
	}
	return []any{in.SemesterNo, string(in.Type), nil, in.GroupID, in.TheoryHours, in.PracticeHours, in.NationalCredit, in.ECTS, false, in.Position}
}

func (in ItemInput) audit() map[string]any {
	m := map[string]any{"semester_no": in.SemesterNo, "item_type": in.Type, "position": in.Position}
	if in.Type == ItemCourse {
		m["course_id"], m["is_compulsory"] = in.CourseID, in.IsCompulsory
	} else {
		m["elective_group_id"], m["ects"] = in.GroupID, in.ECTS
	}
	return m
}

// AddItem, taslak müfredata satır ekler. Ders zaten müfredattaysa ErrDuplicate döner.
func (r *Repository) AddItem(ctx context.Context, actorID, curriculumID string, in ItemInput) (string, error) {
	var id string
	err := r.editDraft(ctx, curriculumID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			INSERT INTO curriculum.curriculum_items
			    (curriculum_id, semester_no, item_type, course_id, elective_group_id, slot_theory_hours, slot_practice_hours,
			     slot_national_credit, slot_ects, is_compulsory, position)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING id`,
			append([]any{curriculumID}, in.args()...)...).Scan(&id)
		if err := itemWriteError(err); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "curriculum.add_item", EntityType: "curriculum", EntityID: curriculumID, After: in.audit(),
		})
	})
	return id, err
}

// UpdateItem, taslak müfredatın bir satırını değiştirir.
func (r *Repository) UpdateItem(ctx context.Context, actorID, curriculumID, itemID string, in ItemInput) error {
	return r.editDraft(ctx, curriculumID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE curriculum.curriculum_items
			SET semester_no = $3, item_type = $4, course_id = $5, elective_group_id = $6, slot_theory_hours = $7,
			    slot_practice_hours = $8, slot_national_credit = $9, slot_ects = $10, is_compulsory = $11, position = $12
			WHERE curriculum_id = $1 AND id = $2`,
			append([]any{curriculumID, itemID}, in.args()...)...)
		if err := itemWriteError(err); err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		after := in.audit()
		after["item_id"] = itemID
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "curriculum.update_item", EntityType: "curriculum", EntityID: curriculumID, After: after,
		})
	})
}

// DeleteItem, taslak müfredattan bir satırı kaldırır.
func (r *Repository) DeleteItem(ctx context.Context, actorID, curriculumID, itemID string) error {
	return r.editDraft(ctx, curriculumID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM curriculum.curriculum_items WHERE curriculum_id = $1 AND id = $2`, curriculumID, itemID)
		if err != nil {
			return fmt.Errorf("curriculum: satır silinemedi: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "curriculum.remove_item", EntityType: "curriculum", EntityID: curriculumID,
			Before: map[string]any{"item_id": itemID},
		})
	})
}

// editDraft, taslak müfredatın satırlarını değiştiren işlemleri sarar: sürümü kilitler,
// taslak olduğunu doğrular ve değişiklikten sonra sürüm numarasını artırır (ETag değişir).
func (r *Repository) editDraft(ctx context.Context, curriculumID string, fn func(tx pgx.Tx) error) error {
	return db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		c, err := r.lockCurriculum(ctx, tx, curriculumID)
		if err != nil {
			return err
		}
		if c.Status != CurriculumDraft {
			return ErrNotDraft
		}
		if err := fn(tx); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE curriculum.curricula SET version = version + 1, updated_at = now() WHERE id = $1`, curriculumID); err != nil {
			return fmt.Errorf("curriculum: müfredat sürümü artırılamadı: %w", err)
		}
		return nil
	})
}

func itemWriteError(err error) error {
	switch {
	case err == nil:
		return nil
	case db.IsConflict(err):
		return ErrDuplicate
	case db.IsForeignKeyViolation(err):
		return ErrUnknownReference
	}
	return fmt.Errorf("curriculum: müfredat satırı yazılamadı: %w", err)
}
