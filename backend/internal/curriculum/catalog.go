package curriculum

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// Repository, katalog, müfredat ve not ölçeği verisine erişir.
type Repository struct {
	pool *pgxpool.Pool
	db   db.Querier
}

// NewRepository, bir Repository oluşturur.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, db: pool}
}

type scanner interface{ Scan(dest ...any) error }

// --- Dersler -------------------------------------------------------------------

const courseSelect = `
	SELECT c.id, c.code, c.name_tr, c.name_en, c.theory_hours, c.practice_hours, c.national_credit, c.ects,
	       c.language, c.course_kind, c.grading_mode, coalesce(c.description_tr, ''), coalesce(c.description_en, ''),
	       c.learning_outcomes, c.is_active, c.version,
	       d.id, d.code, d.name_tr, d.name_en, d.faculty_id
	FROM curriculum.courses c
	LEFT JOIN org.departments d ON d.id = c.owner_department_id`

func scanCourse(s scanner) (Course, error) {
	var (
		c                          Course
		kind, mode                 string
		dID, dCode, dTR, dEN, dFac *string
	)
	err := s.Scan(&c.ID, &c.Code, &c.NameTR, &c.NameEN, &c.TheoryHours, &c.PracticeHours, &c.NationalCredit, &c.ECTS,
		&c.Language, &kind, &mode, &c.DescriptionTR, &c.DescriptionEN, &c.LearningOutcomes, &c.IsActive, &c.Version,
		&dID, &dCode, &dTR, &dEN, &dFac)
	c.Kind, c.GradingMode = CourseKind(kind), GradingMode(mode)
	if dID != nil {
		c.OwnerDepartment = &DepartmentRef{ID: *dID, Code: *dCode, NameTR: *dTR, NameEN: *dEN, FacultyID: *dFac}
	}
	if c.LearningOutcomes == nil {
		c.LearningOutcomes = []string{}
	}
	return c, err
}

// CourseCursor, kod sırasında bir sonraki sayfanın başlangıcıdır.
type CourseCursor struct {
	Code string `json:"c"`
}

// CourseFilter, ders listesinin süzgecidir.
type CourseFilter struct {
	Query           string // kodda ya da adda geçen metin
	DepartmentID    string
	Kind            CourseKind
	IncludeInactive bool
	After           *CourseCursor
	Limit           int
}

// ListCourses, dersleri koda göre sıralı döndürür.
func (r *Repository) ListCourses(ctx context.Context, f CourseFilter) ([]Course, bool, error) {
	var w db.Where
	if f.Query != "" {
		w.Add("(c.code || ' ' || c.name_tr || ' ' || c.name_en) ILIKE $1", "%"+f.Query+"%")
	}
	if f.DepartmentID != "" {
		w.Add("c.owner_department_id = $1", f.DepartmentID)
	}
	if f.Kind != "" {
		w.Add("c.course_kind = $1", string(f.Kind))
	}
	if !f.IncludeInactive {
		w.Add("c.is_active")
	}
	if f.After != nil {
		w.Add("c.code > $1", f.After.Code)
	}
	limit := w.Arg(f.Limit + 1)
	rows, err := r.db.Query(ctx, courseSelect+"\n"+w.SQL()+"\nORDER BY c.code LIMIT "+limit, w.Args()...)
	if err != nil {
		return nil, false, fmt.Errorf("curriculum: dersler listelenemedi: %w", err)
	}
	courses, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Course, error) { return scanCourse(row) })
	if err != nil {
		return nil, false, fmt.Errorf("curriculum: dersler okunamadı: %w", err)
	}
	if len(courses) > f.Limit {
		return courses[:f.Limit], true, nil
	}
	return courses, false, nil
}

// Course, dersi döndürür. Yoksa ErrNotFound döner.
func (r *Repository) Course(ctx context.Context, id string) (Course, error) {
	return r.course(ctx, r.db, courseSelect+` WHERE c.id = $1`, id)
}

func (r *Repository) course(ctx context.Context, q db.Querier, query string, args ...any) (Course, error) {
	c, err := scanCourse(q.QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return Course{}, ErrNotFound
	}
	if err != nil {
		return Course{}, fmt.Errorf("curriculum: ders okunamadı: %w", err)
	}
	return c, nil
}

// CourseDetail, dersi ön koşulları, eşdeğerlikleri ve bulunduğu havuzlarla döndürür.
func (r *Repository) CourseDetail(ctx context.Context, id string) (CourseDetail, error) {
	c, err := r.Course(ctx, id)
	if err != nil {
		return CourseDetail{}, err
	}
	d := CourseDetail{Course: c}

	rows, err := r.db.Query(ctx, `
		SELECT p.id, p.code, p.name_tr, p.name_en, p.ects, cp.requirement, cp.group_no
		FROM curriculum.course_prerequisites cp JOIN curriculum.courses p ON p.id = cp.prerequisite_course_id
		WHERE cp.course_id = $1 ORDER BY cp.group_no, p.code`, id)
	if err != nil {
		return CourseDetail{}, fmt.Errorf("curriculum: ön koşullar okunamadı: %w", err)
	}
	d.Prerequisites, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Prerequisite, error) {
		var (
			p   Prerequisite
			req string
		)
		err := row.Scan(&p.Course.ID, &p.Course.Code, &p.Course.NameTR, &p.Course.NameEN, &p.Course.ECTS, &req, &p.GroupNo)
		p.Requirement = Requirement(req)
		return p, err
	})
	if err != nil {
		return CourseDetail{}, fmt.Errorf("curriculum: ön koşullar okunamadı: %w", err)
	}

	rows, err = r.db.Query(ctx, `
		SELECT c.id, c.code, c.name_tr, c.name_en, c.ects
		FROM curriculum.course_prerequisites cp JOIN curriculum.courses c ON c.id = cp.course_id
		WHERE cp.prerequisite_course_id = $1 ORDER BY c.code`, id)
	if err != nil {
		return CourseDetail{}, fmt.Errorf("curriculum: bağımlı dersler okunamadı: %w", err)
	}
	if d.RequiredBy, err = pgx.CollectRows(rows, scanCourseRef); err != nil {
		return CourseDetail{}, fmt.Errorf("curriculum: bağımlı dersler okunamadı: %w", err)
	}

	rows, err = r.db.Query(ctx, `
		SELECT e.id, 'REPLACES', o.id, o.code, o.name_tr, o.name_en, o.ects, e.is_bidirectional, e.valid_from_year, coalesce(e.note, '')
		FROM curriculum.course_equivalences e JOIN curriculum.courses o ON o.id = e.equivalent_course_id
		WHERE e.course_id = $1
		UNION ALL
		SELECT e.id, 'REPLACED_BY', o.id, o.code, o.name_tr, o.name_en, o.ects, e.is_bidirectional, e.valid_from_year, coalesce(e.note, '')
		FROM curriculum.course_equivalences e JOIN curriculum.courses o ON o.id = e.course_id
		WHERE e.equivalent_course_id = $1
		ORDER BY 4`, id)
	if err != nil {
		return CourseDetail{}, fmt.Errorf("curriculum: eşdeğerlikler okunamadı: %w", err)
	}
	d.Equivalences, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Equivalence, error) {
		var (
			e   Equivalence
			rel string
			y   *int16
		)
		err := row.Scan(&e.ID, &rel, &e.Course.ID, &e.Course.Code, &e.Course.NameTR, &e.Course.NameEN, &e.Course.ECTS,
			&e.IsBidirectional, &y, &e.Note)
		e.Relation = EquivalenceRelation(rel)
		if y != nil {
			v := int(*y)
			e.ValidFromYear = &v
		}
		return e, err
	})
	if err != nil {
		return CourseDetail{}, fmt.Errorf("curriculum: eşdeğerlikler okunamadı: %w", err)
	}

	rows, err = r.db.Query(ctx, `
		SELECT g.id, g.code, g.name_tr, g.name_en, g.group_kind
		FROM curriculum.elective_group_courses gc JOIN curriculum.elective_groups g ON g.id = gc.elective_group_id
		WHERE gc.course_id = $1 ORDER BY g.code`, id)
	if err != nil {
		return CourseDetail{}, fmt.Errorf("curriculum: gruplar okunamadı: %w", err)
	}
	d.ElectiveGroups, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (GroupRef, error) {
		var (
			g    GroupRef
			kind string
		)
		err := row.Scan(&g.ID, &g.Code, &g.NameTR, &g.NameEN, &kind)
		g.Kind = GroupKind(kind)
		return g, err
	})
	if err != nil {
		return CourseDetail{}, fmt.Errorf("curriculum: gruplar okunamadı: %w", err)
	}
	return d, nil
}

func scanCourseRef(row pgx.CollectableRow) (CourseRef, error) {
	var c CourseRef
	err := row.Scan(&c.ID, &c.Code, &c.NameTR, &c.NameEN, &c.ECTS)
	return c, err
}

// CourseInput, ders oluşturma ve güncelleme verisidir. Kod sonradan değişmez: saat,
// kredi ya da AKTS değişen ders yeni bir kodla açılır ve eşdeğerlikle bağlanır.
type CourseInput struct {
	Code              string // sadece oluşturmada
	OwnerDepartmentID string
	NameTR            string
	NameEN            string
	TheoryHours       int
	PracticeHours     int
	NationalCredit    float64
	ECTS              float64
	Language          string
	Kind              CourseKind
	GradingMode       GradingMode
	DescriptionTR     string
	DescriptionEN     string
	LearningOutcomes  []string
	IsActive          bool
}

func (in CourseInput) audit() map[string]any {
	return map[string]any{
		"code": in.Code, "owner_department_id": in.OwnerDepartmentID, "name_tr": in.NameTR, "name_en": in.NameEN,
		"theory_hours": in.TheoryHours, "practice_hours": in.PracticeHours, "national_credit": in.NationalCredit,
		"ects": in.ECTS, "language": in.Language, "course_kind": in.Kind, "grading_mode": in.GradingMode,
		"is_active": in.IsActive,
	}
}

// CreateCourse, kataloğa ders ekler. Aynı kodla ders varsa ErrConflict döner.
func (r *Repository) CreateCourse(ctx context.Context, actorID string, in CourseInput) (string, error) {
	var id string
	err := db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			INSERT INTO curriculum.courses
			    (code, owner_department_id, name_tr, name_en, theory_hours, practice_hours, national_credit, ects,
			     language, course_kind, grading_mode, description_tr, description_en, learning_outcomes)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
			RETURNING id`,
			in.Code, nullable(in.OwnerDepartmentID), in.NameTR, in.NameEN, in.TheoryHours, in.PracticeHours,
			in.NationalCredit, in.ECTS, in.Language, string(in.Kind), string(in.GradingMode),
			nullable(in.DescriptionTR), nullable(in.DescriptionEN), outcomes(in.LearningOutcomes)).Scan(&id)
		if err := writeError(err, "ders oluşturulamadı"); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "course.create", EntityType: "course", EntityID: id, After: in.audit(),
		})
	})
	return id, err
}

// UpdateCourse, dersin kod dışındaki bilgilerini günceller.
func (r *Repository) UpdateCourse(ctx context.Context, actorID, id string, version int, in CourseInput) error {
	return db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		before, err := r.course(ctx, tx, courseSelect+` WHERE c.id = $1 FOR UPDATE OF c`, id)
		if err != nil {
			return err
		}
		if before.Version != version {
			return ErrVersionMismatch
		}
		_, err = tx.Exec(ctx, `
			UPDATE curriculum.courses
			SET owner_department_id = $2, name_tr = $3, name_en = $4, theory_hours = $5, practice_hours = $6,
			    national_credit = $7, ects = $8, language = $9, course_kind = $10, grading_mode = $11,
			    description_tr = $12, description_en = $13, learning_outcomes = $14, is_active = $15,
			    version = version + 1, updated_at = now()
			WHERE id = $1`,
			id, nullable(in.OwnerDepartmentID), in.NameTR, in.NameEN, in.TheoryHours, in.PracticeHours,
			in.NationalCredit, in.ECTS, in.Language, string(in.Kind), string(in.GradingMode),
			nullable(in.DescriptionTR), nullable(in.DescriptionEN), outcomes(in.LearningOutcomes), in.IsActive)
		if err := writeError(err, "ders güncellenemedi"); err != nil {
			return err
		}
		prev := CourseInput{
			Code: before.Code, NameTR: before.NameTR, NameEN: before.NameEN, TheoryHours: before.TheoryHours,
			PracticeHours: before.PracticeHours, NationalCredit: before.NationalCredit, ECTS: before.ECTS,
			Language: before.Language, Kind: before.Kind, GradingMode: before.GradingMode, IsActive: before.IsActive,
		}
		if before.OwnerDepartment != nil {
			prev.OwnerDepartmentID = before.OwnerDepartment.ID
		}
		in.Code = before.Code
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "course.update", EntityType: "course", EntityID: id,
			Before: prev.audit(), After: in.audit(),
		})
	})
}

// PrerequisiteInput, bir ön koşul satırıdır.
type PrerequisiteInput struct {
	CourseID    string
	Requirement Requirement
	GroupNo     int
}

// SetPrerequisites, dersin ön koşullarını verilen kümeyle değiştirir. Yeni küme bir
// döngü oluşturursa (A, B'yi; B de A'yı isterse) ErrPrerequisiteCycle döner.
func (r *Repository) SetPrerequisites(ctx context.Context, actorID, courseID string, items []PrerequisiteInput) error {
	return db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		// Ön koşul değişiklikleri sıraya girer: eşzamanlı iki değişiklik (A→B ve B→A)
		// ayrı ayrı döngü kontrolünden geçip birlikte döngü oluşturamaz.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('curriculum.course_prerequisites'))`); err != nil {
			return fmt.Errorf("curriculum: kilit alınamadı: %w", err)
		}
		if _, err := r.course(ctx, tx, courseSelect+` WHERE c.id = $1 FOR UPDATE OF c`, courseID); err != nil {
			return err
		}

		var before []string
		rows, err := tx.Query(ctx, `
			SELECT p.code FROM curriculum.course_prerequisites cp JOIN curriculum.courses p ON p.id = cp.prerequisite_course_id
			WHERE cp.course_id = $1 ORDER BY p.code`, courseID)
		if err != nil {
			return fmt.Errorf("curriculum: ön koşullar okunamadı: %w", err)
		}
		if before, err = pgx.CollectRows(rows, pgx.RowTo[string]); err != nil {
			return fmt.Errorf("curriculum: ön koşullar okunamadı: %w", err)
		}

		if _, err := tx.Exec(ctx, `DELETE FROM curriculum.course_prerequisites WHERE course_id = $1`, courseID); err != nil {
			return fmt.Errorf("curriculum: ön koşullar silinemedi: %w", err)
		}
		ids := make([]string, 0, len(items))
		for _, it := range items {
			_, err := tx.Exec(ctx, `
				INSERT INTO curriculum.course_prerequisites (course_id, prerequisite_course_id, requirement, group_no)
				VALUES ($1, $2, $3, $4)`, courseID, it.CourseID, string(it.Requirement), it.GroupNo)
			switch {
			case db.IsForeignKeyViolation(err):
				return ErrUnknownReference
			case db.IsConflict(err):
				return ErrDuplicate
			case db.IsCheckViolation(err):
				return ErrPrerequisiteCycle // ders kendisini isteyemez
			case err != nil:
				return fmt.Errorf("curriculum: ön koşul eklenemedi: %w", err)
			}
			ids = append(ids, it.CourseID)
		}

		// Döngü: yeni ön koşullardan geriye doğru ilerlerken ders kendisine ulaşıyor mu?
		var cycle bool
		err = tx.QueryRow(ctx, `
			WITH RECURSIVE reach(id) AS (
				SELECT unnest($2::uuid[])
				UNION
				SELECT cp.prerequisite_course_id FROM curriculum.course_prerequisites cp JOIN reach r ON cp.course_id = r.id
			)
			SELECT EXISTS (SELECT 1 FROM reach WHERE id = $1)`, courseID, ids).Scan(&cycle)
		if err != nil {
			return fmt.Errorf("curriculum: döngü denetlenemedi: %w", err)
		}
		if cycle {
			return ErrPrerequisiteCycle
		}

		var after []string
		rows, err = tx.Query(ctx, `
			SELECT p.code FROM curriculum.course_prerequisites cp JOIN curriculum.courses p ON p.id = cp.prerequisite_course_id
			WHERE cp.course_id = $1 ORDER BY p.code`, courseID)
		if err != nil {
			return fmt.Errorf("curriculum: ön koşullar okunamadı: %w", err)
		}
		if after, err = pgx.CollectRows(rows, pgx.RowTo[string]); err != nil {
			return fmt.Errorf("curriculum: ön koşullar okunamadı: %w", err)
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "course.set_prerequisites", EntityType: "course", EntityID: courseID,
			Before: map[string]any{"prerequisites": before}, After: map[string]any{"prerequisites": after},
		})
	})
}

// EquivalenceInput, eşdeğerlik ekleme verisidir. CourseID yeni, EquivalentCourseID eski koddur.
type EquivalenceInput struct {
	EquivalentCourseID string
	IsBidirectional    bool
	ValidFromYear      *int
	Note               string
}

// AddEquivalence, iki ders arasında eşdeğerlik tanımlar. Ters yönde de olsa aynı çift
// zaten bağlıysa ErrDuplicate döner.
func (r *Repository) AddEquivalence(ctx context.Context, actorID, courseID string, in EquivalenceInput) (string, error) {
	var id string
	err := db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM curriculum.course_equivalences
			               WHERE (course_id = $1 AND equivalent_course_id = $2) OR (course_id = $2 AND equivalent_course_id = $1))`,
			courseID, in.EquivalentCourseID).Scan(&exists); err != nil {
			return fmt.Errorf("curriculum: eşdeğerlik okunamadı: %w", err)
		}
		if exists {
			return ErrDuplicate
		}
		err := tx.QueryRow(ctx, `
			INSERT INTO curriculum.course_equivalences (course_id, equivalent_course_id, is_bidirectional, valid_from_year, note)
			VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			courseID, in.EquivalentCourseID, in.IsBidirectional, in.ValidFromYear, nullable(in.Note)).Scan(&id)
		switch {
		case db.IsForeignKeyViolation(err):
			return ErrUnknownReference
		case db.IsCheckViolation(err):
			return ErrDuplicate // ders kendisine eşdeğer olamaz
		case err != nil:
			return fmt.Errorf("curriculum: eşdeğerlik eklenemedi: %w", err)
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "course.add_equivalence", EntityType: "course", EntityID: courseID,
			After: map[string]any{"equivalent_course_id": in.EquivalentCourseID, "is_bidirectional": in.IsBidirectional, "valid_from_year": in.ValidFromYear},
		})
	})
	return id, err
}

// DeleteEquivalence, dersin bir eşdeğerliğini kaldırır. Eşdeğerlik bu derse ait değilse
// ErrNotFound döner.
func (r *Repository) DeleteEquivalence(ctx context.Context, actorID, courseID, equivalenceID string) error {
	return db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		var a, b string
		err := tx.QueryRow(ctx, `
			DELETE FROM curriculum.course_equivalences
			WHERE id = $1 AND (course_id = $2 OR equivalent_course_id = $2)
			RETURNING course_id, equivalent_course_id`, equivalenceID, courseID).Scan(&a, &b)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("curriculum: eşdeğerlik silinemedi: %w", err)
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "course.remove_equivalence", EntityType: "course", EntityID: courseID,
			Before: map[string]any{"course_id": a, "equivalent_course_id": b},
		})
	})
}

// --- Seçmeli gruplar -----------------------------------------------------------

const groupSelect = `
	SELECT g.id, g.code, g.name_tr, g.name_en, g.group_kind, g.is_active, g.version,
	       (SELECT count(*) FROM curriculum.elective_group_courses gc WHERE gc.elective_group_id = g.id),
	       d.id, d.code, d.name_tr, d.name_en, d.faculty_id
	FROM curriculum.elective_groups g
	LEFT JOIN org.departments d ON d.id = g.owner_department_id`

func scanGroup(s scanner) (ElectiveGroup, error) {
	var (
		g                          ElectiveGroup
		kind                       string
		dID, dCode, dTR, dEN, dFac *string
	)
	err := s.Scan(&g.ID, &g.Code, &g.NameTR, &g.NameEN, &kind, &g.IsActive, &g.Version, &g.CourseCount,
		&dID, &dCode, &dTR, &dEN, &dFac)
	g.Kind = GroupKind(kind)
	if dID != nil {
		g.OwnerDepartment = &DepartmentRef{ID: *dID, Code: *dCode, NameTR: *dTR, NameEN: *dEN, FacultyID: *dFac}
	}
	return g, err
}

// GroupFilter, seçmeli grup listesinin süzgecidir.
type GroupFilter struct {
	Query        string
	Kind         GroupKind
	DepartmentID string
}

// ListGroups, seçmeli grupları koda göre sıralı döndürür.
func (r *Repository) ListGroups(ctx context.Context, f GroupFilter) ([]ElectiveGroup, error) {
	var w db.Where
	if f.Query != "" {
		w.Add("(g.code || ' ' || g.name_tr || ' ' || g.name_en) ILIKE $1", "%"+f.Query+"%")
	}
	if f.Kind != "" {
		w.Add("g.group_kind = $1", string(f.Kind))
	}
	if f.DepartmentID != "" {
		w.Add("g.owner_department_id = $1", f.DepartmentID)
	}
	rows, err := r.db.Query(ctx, groupSelect+"\n"+w.SQL()+"\nORDER BY g.code", w.Args()...)
	if err != nil {
		return nil, fmt.Errorf("curriculum: gruplar listelenemedi: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (ElectiveGroup, error) { return scanGroup(row) })
}

// Group, seçmeli grubu döndürür. Yoksa ErrNotFound döner.
func (r *Repository) Group(ctx context.Context, id string) (ElectiveGroup, error) {
	return r.group(ctx, r.db, groupSelect+` WHERE g.id = $1`, id)
}

func (r *Repository) group(ctx context.Context, q db.Querier, query string, args ...any) (ElectiveGroup, error) {
	g, err := scanGroup(q.QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return ElectiveGroup{}, ErrNotFound
	}
	if err != nil {
		return ElectiveGroup{}, fmt.Errorf("curriculum: grup okunamadı: %w", err)
	}
	return g, nil
}

// GroupCourses, grubun derslerini koda göre sıralı döndürür.
func (r *Repository) GroupCourses(ctx context.Context, groupID string) ([]Course, error) {
	rows, err := r.db.Query(ctx, courseSelect+`
		JOIN curriculum.elective_group_courses gc ON gc.course_id = c.id
		WHERE gc.elective_group_id = $1 ORDER BY c.code`, groupID)
	if err != nil {
		return nil, fmt.Errorf("curriculum: grup dersleri okunamadı: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Course, error) { return scanCourse(row) })
}

// GroupInput, seçmeli grup oluşturma ve güncelleme verisidir.
type GroupInput struct {
	Code              string // sadece oluşturmada
	NameTR            string
	NameEN            string
	OwnerDepartmentID string
	Kind              GroupKind
	IsActive          bool
}

func (in GroupInput) audit() map[string]any {
	return map[string]any{
		"code": in.Code, "name_tr": in.NameTR, "name_en": in.NameEN,
		"owner_department_id": in.OwnerDepartmentID, "group_kind": in.Kind, "is_active": in.IsActive,
	}
}

// CreateGroup, bir seçmeli grup oluşturur.
func (r *Repository) CreateGroup(ctx context.Context, actorID string, in GroupInput) (string, error) {
	var id string
	err := db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			INSERT INTO curriculum.elective_groups (code, name_tr, name_en, owner_department_id, group_kind)
			VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			in.Code, in.NameTR, in.NameEN, nullable(in.OwnerDepartmentID), string(in.Kind)).Scan(&id)
		if err := writeError(err, "grup oluşturulamadı"); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "elective_group.create", EntityType: "elective_group", EntityID: id, After: in.audit(),
		})
	})
	return id, err
}

// UpdateGroup, grubun kod dışındaki bilgilerini günceller.
func (r *Repository) UpdateGroup(ctx context.Context, actorID, id string, version int, in GroupInput) error {
	return db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		before, err := r.group(ctx, tx, groupSelect+` WHERE g.id = $1 FOR UPDATE OF g`, id)
		if err != nil {
			return err
		}
		if before.Version != version {
			return ErrVersionMismatch
		}
		_, err = tx.Exec(ctx, `
			UPDATE curriculum.elective_groups
			SET name_tr = $2, name_en = $3, owner_department_id = $4, group_kind = $5, is_active = $6,
			    version = version + 1, updated_at = now()
			WHERE id = $1`, id, in.NameTR, in.NameEN, nullable(in.OwnerDepartmentID), string(in.Kind), in.IsActive)
		if err := writeError(err, "grup güncellenemedi"); err != nil {
			return err
		}
		prev := GroupInput{Code: before.Code, NameTR: before.NameTR, NameEN: before.NameEN, Kind: before.Kind, IsActive: before.IsActive}
		if before.OwnerDepartment != nil {
			prev.OwnerDepartmentID = before.OwnerDepartment.ID
		}
		in.Code = before.Code
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "elective_group.update", EntityType: "elective_group", EntityID: id,
			Before: prev.audit(), After: in.audit(),
		})
	})
}

// AddGroupCourse, havuza ders ekler. Ders zaten havuzdaysa bir şey yapmaz (idempotent).
func (r *Repository) AddGroupCourse(ctx context.Context, actorID, groupID, courseID string) error {
	return db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			INSERT INTO curriculum.elective_group_courses (elective_group_id, course_id) VALUES ($1, $2)
			ON CONFLICT DO NOTHING`, groupID, courseID)
		switch {
		case db.IsForeignKeyViolation(err):
			return ErrUnknownReference
		case err != nil:
			return fmt.Errorf("curriculum: havuza ders eklenemedi: %w", err)
		case tag.RowsAffected() == 0:
			return nil
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "elective_group.add_course", EntityType: "elective_group", EntityID: groupID,
			After: map[string]any{"course_id": courseID},
		})
	})
}

// RemoveGroupCourse, dersi havuzdan çıkarır. Ders havuzda değilse ErrNotFound döner.
func (r *Repository) RemoveGroupCourse(ctx context.Context, actorID, groupID, courseID string) error {
	return db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM curriculum.elective_group_courses WHERE elective_group_id = $1 AND course_id = $2`, groupID, courseID)
		if err != nil {
			return fmt.Errorf("curriculum: havuzdan ders çıkarılamadı: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "elective_group.remove_course", EntityType: "elective_group", EntityID: groupID,
			Before: map[string]any{"course_id": courseID},
		})
	})
}

// --- Ortak ---------------------------------------------------------------------

func writeError(err error, msg string) error {
	switch {
	case err == nil:
		return nil
	case db.IsConflict(err):
		return ErrConflict
	case db.IsForeignKeyViolation(err):
		return ErrUnknownReference
	}
	return fmt.Errorf("curriculum: %s: %w", msg, err)
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// outcomes, öğrenme çıktılarını boş satırlardan arındırır. nil yerine boş dizi döner:
// sütun NOT NULL ve JSON dizi olmalı.
func outcomes(list []string) []string {
	out := slices.DeleteFunc(slices.Clone(list), func(s string) bool { return s == "" })
	if out == nil {
		return []string{}
	}
	return out
}
