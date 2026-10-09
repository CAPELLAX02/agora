// Package grading, şubelerin değerlendirme planlarını (ve sonraki fazlarda notları)
// yönetir.
package grading

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// Hatalar.
var (
	ErrNotFound        = errors.New("grading: kayıt bulunamadı")
	ErrVersionMismatch = errors.New("grading: plan bu arada değişti")
	ErrPlanLocked      = errors.New("grading: değerlendirme planı kilitli")
	ErrPlanIncomplete  = errors.New("grading: değerlendirme planı tamamlanmamış")
	ErrSectionInactive = errors.New("grading: şube iptal edilmiş")
)

// Category, değerlendirme türünün plandaki yeridir.
type Category string

// Kategoriler.
const (
	CategoryInTerm Category = "IN_TERM"
	CategoryFinal  Category = "FINAL"
	CategoryMakeup Category = "MAKEUP"
)

// AssessmentType, bir değerlendirme türüdür (ara sınav, ödev ...).
type AssessmentType struct {
	Code     string
	NameTR   string
	NameEN   string
	Category Category
}

// Component, planın bir bileşenidir.
type Component struct {
	ID          string
	Type        AssessmentType
	SequenceNo  int
	NameTR      string // boşsa türün adı kullanılır
	NameEN      string
	Weight      float64
	ScheduledOn *time.Time
}

// Label, bileşenin gösterilen adıdır: verilmişse kendi adı, yoksa türün adı ve (birden
// fazla aynı türde bileşen olabildiği için) sıra numarası.
func (c Component) Label(english bool, numbered bool) string {
	name, typeName := c.NameTR, c.Type.NameTR
	if english {
		name, typeName = c.NameEN, c.Type.NameEN
	}
	if name != "" {
		return name
	}
	if numbered {
		return typeName + " " + strconv.Itoa(c.SequenceNo)
	}
	return typeName
}

// SectionContext, plan işlemlerinin yetki ve doğrulama için ihtiyaç duyduğu şube bilgisidir.
type SectionContext struct {
	SectionID      string
	SectionCode    string
	SectionStatus  string
	CourseCode     string
	DepartmentID   string
	FacultyID      string
	TermStartsOn   time.Time
	TermEndsOn     time.Time
	Version        int
	LockedAt       *time.Time
	LockedByName   string
	InstructorRole string // görüntüleyen kullanıcının şubedeki rolü; öğretim elemanı değilse boş
}

// Plan, şubenin değerlendirme planıdır.
type Plan struct {
	SectionContext
	Components []Component
}

// Repository, değerlendirme verisine erişir.
type Repository struct {
	pool *pgxpool.Pool
	db   db.Querier
}

// NewRepository, bir Repository oluşturur.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, db: pool}
}

// AssessmentTypes, değerlendirme türlerini sıralı döndürür.
func (r *Repository) AssessmentTypes(ctx context.Context) ([]AssessmentType, error) {
	rows, err := r.db.Query(ctx, `SELECT code, name_tr, name_en, category FROM grading.assessment_types ORDER BY sort_order`)
	if err != nil {
		return nil, fmt.Errorf("grading: değerlendirme türleri okunamadı: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (AssessmentType, error) {
		var (
			t   AssessmentType
			cat string
		)
		err := row.Scan(&t.Code, &t.NameTR, &t.NameEN, &cat)
		t.Category = Category(cat)
		return t, err
	})
}

const contextSelect = `
	SELECT s.id, s.section_code, s.status, c.code, o.department_id, d.faculty_id, t.starts_on, t.ends_on,
	       s.assessment_plan_version, s.assessment_plan_locked_at,
	       coalesce((SELECT pe.first_name || ' ' || pe.last_name FROM iam.users u JOIN people.persons pe ON pe.id = u.person_id
	                 WHERE u.id = s.assessment_plan_locked_by), ''),
	       coalesce((SELECT si.role FROM offering.section_instructors si
	                 JOIN people.staff st ON st.id = si.staff_id
	                 JOIN iam.users u ON u.person_id = st.person_id
	                 WHERE si.section_id = s.id AND u.id::text = $2), '')
	FROM offering.sections s
	JOIN offering.course_offerings o ON o.id = s.offering_id
	JOIN curriculum.courses c ON c.id = o.course_id
	JOIN org.departments d ON d.id = o.department_id
	JOIN academic.terms t ON t.id = o.term_id
	WHERE s.id = $1`

func (r *Repository) sectionContext(ctx context.Context, q db.Querier, sectionID, viewerID string, lock bool) (SectionContext, error) {
	query := contextSelect
	if lock {
		query += ` FOR UPDATE OF s`
	}
	var sc SectionContext
	err := q.QueryRow(ctx, query, sectionID, viewerID).Scan(&sc.SectionID, &sc.SectionCode, &sc.SectionStatus, &sc.CourseCode,
		&sc.DepartmentID, &sc.FacultyID, &sc.TermStartsOn, &sc.TermEndsOn, &sc.Version, &sc.LockedAt, &sc.LockedByName, &sc.InstructorRole)
	if errors.Is(err, pgx.ErrNoRows) {
		return SectionContext{}, ErrNotFound
	}
	if err != nil {
		return SectionContext{}, fmt.Errorf("grading: şube okunamadı: %w", err)
	}
	return sc, nil
}

// Plan, şubenin değerlendirme planını döndürür. viewerID, kullanıcının şubedeki
// rolünü (InstructorRole) çözmek içindir.
func (r *Repository) Plan(ctx context.Context, sectionID, viewerID string) (Plan, error) {
	sc, err := r.sectionContext(ctx, r.db, sectionID, viewerID, false)
	if err != nil {
		return Plan{}, err
	}
	components, err := r.components(ctx, r.db, sectionID)
	if err != nil {
		return Plan{}, err
	}
	return Plan{SectionContext: sc, Components: components}, nil
}

func (r *Repository) components(ctx context.Context, q db.Querier, sectionID string) ([]Component, error) {
	rows, err := q.Query(ctx, `
		SELECT ac.id, t.code, t.name_tr, t.name_en, t.category, ac.sequence_no, coalesce(ac.name_tr, ''), coalesce(ac.name_en, ''),
		       ac.weight::float8, ac.scheduled_on
		FROM grading.assessment_components ac JOIN grading.assessment_types t ON t.code = ac.type_code
		WHERE ac.section_id = $1
		ORDER BY t.category = 'MAKEUP', t.category = 'FINAL', ac.position, t.sort_order, ac.sequence_no`, sectionID)
	if err != nil {
		return nil, fmt.Errorf("grading: plan okunamadı: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Component, error) {
		var (
			c   Component
			cat string
			seq int16
		)
		err := row.Scan(&c.ID, &c.Type.Code, &c.Type.NameTR, &c.Type.NameEN, &cat, &seq, &c.NameTR, &c.NameEN, &c.Weight, &c.ScheduledOn)
		c.Type.Category, c.SequenceNo = Category(cat), int(seq)
		return c, err
	})
}

// ComponentInput, plana yazılacak bir bileşendir. Bütünleme bileşeni istemciden gelmez:
// finalden türetilir.
type ComponentInput struct {
	TypeCode    string
	SequenceNo  int
	NameTR      string
	NameEN      string
	Weight      float64
	ScheduledOn *time.Time
}

func (in ComponentInput) key() string { return in.TypeCode + "#" + strconv.Itoa(in.SequenceNo) }

// SetPlan, şubenin planını verilen bileşenlerle değiştirir. Aynı tür ve sıra numaralı
// bileşenler güncellenir (kimlikleri korunur: notlar bileşene bağlıdır), olmayanlar
// eklenir, kalanlar silinir. Bütünleme, finalin ağırlığıyla otomatik yazılır.
func (r *Repository) SetPlan(ctx context.Context, actorID, sectionID string, version int, items []ComponentInput) error {
	return db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		sc, err := r.sectionContext(ctx, tx, sectionID, "", true)
		if err != nil {
			return err
		}
		if sc.SectionStatus != "ACTIVE" {
			return ErrSectionInactive
		}
		if sc.Version != version {
			return ErrVersionMismatch
		}
		if sc.LockedAt != nil {
			return ErrPlanLocked
		}
		before, err := r.components(ctx, tx, sectionID)
		if err != nil {
			return err
		}

		for _, it := range items {
			if it.TypeCode == "FINAL" {
				items = append(items, ComponentInput{TypeCode: "MAKEUP", SequenceNo: 1, Weight: it.Weight})
				break
			}
		}
		keep := make([]string, 0, len(items))
		for i, it := range items {
			keep = append(keep, it.key())
			if _, err := tx.Exec(ctx, `
				INSERT INTO grading.assessment_components (section_id, type_code, sequence_no, name_tr, name_en, weight, scheduled_on, position)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
				ON CONFLICT (section_id, type_code, sequence_no) DO UPDATE
				SET name_tr = excluded.name_tr, name_en = excluded.name_en, weight = excluded.weight,
				    scheduled_on = excluded.scheduled_on, position = excluded.position`,
				sectionID, it.TypeCode, it.SequenceNo, nullable(it.NameTR), nullable(it.NameEN), it.Weight, it.ScheduledOn, i); err != nil {
				return fmt.Errorf("grading: bileşen yazılamadı: %w", err)
			}
		}
		if _, err := tx.Exec(ctx, `
			DELETE FROM grading.assessment_components
			WHERE section_id = $1 AND NOT (type_code || '#' || sequence_no::text = ANY($2::text[]))`, sectionID, keep); err != nil {
			return fmt.Errorf("grading: eski bileşenler silinemedi: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE offering.sections SET assessment_plan_version = assessment_plan_version + 1 WHERE id = $1`, sectionID); err != nil {
			return fmt.Errorf("grading: plan sürümü artırılamadı: %w", err)
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "assessment_plan.set", EntityType: "section", EntityID: sectionID,
			Before: map[string]any{"components": componentAudit(before)}, After: map[string]any{"components": inputAudit(items)},
		})
	})
}

// Lock, planı kilitler: notlar bu plana göre girilecektir. Planda final yoksa
// ErrPlanIncomplete döner; zaten kilitliyse bir şey değişmez.
func (r *Repository) Lock(ctx context.Context, actorID, sectionID string) error {
	return db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		sc, err := r.sectionContext(ctx, tx, sectionID, "", true)
		if err != nil {
			return err
		}
		if sc.LockedAt != nil {
			return nil
		}
		if sc.SectionStatus != "ACTIVE" {
			return ErrSectionInactive
		}
		var total float64
		var finals int
		if err := tx.QueryRow(ctx, `
			SELECT coalesce(sum(ac.weight) FILTER (WHERE t.category <> 'MAKEUP'), 0)::float8, count(*) FILTER (WHERE t.category = 'FINAL')
			FROM grading.assessment_components ac JOIN grading.assessment_types t ON t.code = ac.type_code
			WHERE ac.section_id = $1`, sectionID).Scan(&total, &finals); err != nil {
			return fmt.Errorf("grading: plan okunamadı: %w", err)
		}
		if finals != 1 || total != 100 {
			return ErrPlanIncomplete
		}
		if _, err := tx.Exec(ctx, `
			UPDATE offering.sections
			SET assessment_plan_locked_at = now(), assessment_plan_locked_by = $2, assessment_plan_version = assessment_plan_version + 1
			WHERE id = $1`, sectionID, nullable(actorID)); err != nil {
			return fmt.Errorf("grading: plan kilitlenemedi: %w", err)
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "assessment_plan.lock", EntityType: "section", EntityID: sectionID,
		})
	})
}

// Unlock, kilitli planı yeniden düzenlemeye açar. Kilitli değilse bir şey değişmez.
func (r *Repository) Unlock(ctx context.Context, actorID, sectionID, reason string) error {
	return db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		sc, err := r.sectionContext(ctx, tx, sectionID, "", true)
		if err != nil {
			return err
		}
		if sc.LockedAt == nil {
			return nil
		}
		if _, err := tx.Exec(ctx, `
			UPDATE offering.sections
			SET assessment_plan_locked_at = NULL, assessment_plan_locked_by = NULL, assessment_plan_version = assessment_plan_version + 1
			WHERE id = $1`, sectionID); err != nil {
			return fmt.Errorf("grading: plan kilidi açılamadı: %w", err)
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "assessment_plan.unlock", EntityType: "section", EntityID: sectionID,
			Before: map[string]any{"locked_at": sc.LockedAt}, After: map[string]any{"reason": reason},
		})
	})
}

func componentAudit(list []Component) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for _, c := range list {
		out = append(out, map[string]any{"type": c.Type.Code, "sequence_no": c.SequenceNo, "weight": c.Weight})
	}
	return out
}

func inputAudit(list []ComponentInput) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for _, c := range list {
		out = append(out, map[string]any{"type": c.TypeCode, "sequence_no": c.SequenceNo, "weight": c.Weight})
	}
	return out
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
