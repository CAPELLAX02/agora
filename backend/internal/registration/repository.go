// Package registration, ders seçmeyi yönetir: alınabilir dersler, sepet, kural
// denetimi, danışman onayı ve ekle-bırak. Kararları saf kural motoru (rules) verir; bu
// paket girdileri veritabanından toplar, kontenjanı yarışsız düşer ve durumları yazar.
package registration

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/enrollment"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
	"github.com/CAPELLAX02/agora/backend/internal/registration/rules"
)

// Repository, ders seçme kurallarının girdilerini okur.
type Repository struct {
	db db.Querier
}

// NewRepository, verilen bağlantıyı (havuz ya da transaction) kullanan bir Repository oluşturur.
func NewRepository(q db.Querier) *Repository {
	return &Repository{db: q}
}

// Line, bir müfredat satırı ve gösterim bilgileridir.
type Line struct {
	rules.Line
	Position    int
	Code        string // ders ya da havuz kodu
	NameTR      string
	NameEN      string
	GroupKind   string // seçmeli yuvasında havuzun türü
	CourseKind  string // ders satırında dersin türü
	TheoryHours int
	Practice    int
	Credit      float64
}

// Lines, müfredat sürümünün satırlarını yarıyıl ve sıra düzeninde döndürür.
func (r *Repository) Lines(ctx context.Context, curriculumID string) ([]Line, error) {
	rows, err := r.db.Query(ctx, `
		SELECT i.id, i.semester_no, coalesce(i.course_id::text, ''), coalesce(i.elective_group_id::text, ''),
		       coalesce(i.slot_course_count, 1), i.is_compulsory, coalesce(c.ects, i.slot_ects)::float8, i.position,
		       coalesce(c.code, g.code), coalesce(c.name_tr, g.name_tr), coalesce(c.name_en, g.name_en),
		       coalesce(g.group_kind, ''), coalesce(c.course_kind, ''),
		       coalesce(c.theory_hours, i.slot_theory_hours), coalesce(c.practice_hours, i.slot_practice_hours),
		       coalesce(c.national_credit, i.slot_national_credit)::float8
		FROM curriculum.curriculum_items i
		LEFT JOIN curriculum.courses c ON c.id = i.course_id
		LEFT JOIN curriculum.elective_groups g ON g.id = i.elective_group_id
		WHERE i.curriculum_id = $1
		ORDER BY i.semester_no, i.position, coalesce(c.code, g.code), i.id`, curriculumID)
	if err != nil {
		return nil, fmt.Errorf("registration: müfredat satırları okunamadı: %w", err)
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Line, error) {
		var (
			l                                   Line
			semester, count, position, th, prac int16
		)
		err := row.Scan(&l.ID, &semester, &l.CourseID, &l.GroupID, &count, &l.Compulsory, &l.ECTS, &position,
			&l.Code, &l.NameTR, &l.NameEN, &l.GroupKind, &l.CourseKind, &th, &prac, &l.Credit)
		l.Semester, l.SlotCount, l.Position, l.TheoryHours, l.Practice = int(semester), int(count), int(position), int(th), int(prac)
		return l, err
	})
	if err != nil {
		return nil, fmt.Errorf("registration: müfredat satırları okunamadı: %w", err)
	}
	return list, nil
}

// Catalog, bütün eşdeğerlikleri ve müfredatın seçmeli havuzlarının derslerini yükler.
func (r *Repository) Catalog(ctx context.Context, curriculumID string) (*rules.Catalog, error) {
	rows, err := r.db.Query(ctx, `
		SELECT course_id, equivalent_course_id, is_bidirectional FROM curriculum.course_equivalences`)
	if err != nil {
		return nil, fmt.Errorf("registration: eşdeğerlikler okunamadı: %w", err)
	}
	eqs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (rules.Equivalence, error) {
		var e rules.Equivalence
		return e, row.Scan(&e.NewID, &e.OldID, &e.Bidirectional)
	})
	if err != nil {
		return nil, fmt.Errorf("registration: eşdeğerlikler okunamadı: %w", err)
	}

	rows, err = r.db.Query(ctx, `
		SELECT gc.elective_group_id, gc.course_id FROM curriculum.elective_group_courses gc
		WHERE gc.elective_group_id IN (
			SELECT elective_group_id FROM curriculum.curriculum_items
			WHERE curriculum_id = $1 AND elective_group_id IS NOT NULL)`, curriculumID)
	if err != nil {
		return nil, fmt.Errorf("registration: seçmeli havuzları okunamadı: %w", err)
	}
	groups := map[string][]string{}
	var g, c string
	_, err = pgx.ForEachRow(rows, []any{&g, &c}, func() error {
		groups[g] = append(groups[g], c)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("registration: seçmeli havuzları okunamadı: %w", err)
	}
	return rules.NewCatalog(eqs, groups), nil
}

// Attempts, ders geçmişini kural motorunun denemelerine çevirir. Notu verilmemiş
// (dönemi süren) kayıtlar "bu dönem alınıyor" sayılır.
func Attempts(history []enrollment.CourseAttempt) []rules.Attempt {
	out := make([]rules.Attempt, 0, len(history))
	for _, h := range history {
		out = append(out, rules.Attempt{
			CourseID:       h.CourseID,
			TermOrder:      h.TermOrder,
			Current:        h.Status == enrollment.EnrollmentEnrolled,
			Completed:      h.Status == enrollment.EnrollmentCompleted,
			Passing:        h.Status == enrollment.EnrollmentCompleted && h.Passing,
			AttendanceFail: h.AbsenceFail,
			Letter:         h.Letter,
			ECTS:           h.ECTS,
		})
	}
	return out
}

// RuleLines, satırların kural motoru karşılıklarıdır.
func RuleLines(lines []Line) []rules.Line {
	out := make([]rules.Line, len(lines))
	for i, l := range lines {
		out[i] = l.Line
	}
	return out
}
