package enrollment

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
)

// EnrollmentStatus, kesinleşmiş ders kaydının durumudur.
type EnrollmentStatus string

// Ders kaydı durumları.
const (
	EnrollmentEnrolled  EnrollmentStatus = "ENROLLED"  // dönem sürüyor
	EnrollmentDropped   EnrollmentStatus = "DROPPED"   // ekle-bırakta bırakıldı
	EnrollmentCompleted EnrollmentStatus = "COMPLETED" // notu verildi
)

// CourseAttempt, öğrencinin bir dersi bir dönemde alışıdır (ders geçmişinin bir satırı).
type CourseAttempt struct {
	ID           string
	TermID       string
	TermCode     string
	TermOrder    int // dönem sırası: büyük olan daha yeni
	CourseID     string
	CourseCode   string
	CourseNameTR string
	CourseNameEN string
	SectionCode  string
	AttemptNo    int
	Status       EnrollmentStatus
	Letter       string   // notu yoksa boş
	Coefficient  *float64 // katsayısız harflerde (BŞR, MUAF) ya da notu yoksa nil
	Passing      bool
	CountsInGPA  bool
	EarnsECTS    bool
	AbsenceFail  bool // devamsızlıktan kaldı
	ECTS         float64
	Credit       float64
	Source       string
	Superseded   bool // yerini sonraki bir deneme aldı (son not geçerli)
}

// TermOrderSQL, dönemin sırasını veren SQL ifadesidir (t: academic.terms, ay: academic_years).
// Güz, aynı yılın baharından; bahar, sonraki yılın güzünden öncedir.
const TermOrderSQL = `(ay.start_year * 3 + CASE t.term_type WHEN 'FALL' THEN 0 WHEN 'SPRING' THEN 1 ELSE 2 END)`

// CourseHistory, program kaydının bırakılmamış bütün ders kayıtlarını dönem sırasıyla döndürür.
func (r *Repository) CourseHistory(ctx context.Context, studentProgramID string) ([]CourseAttempt, error) {
	rows, err := r.db.Query(ctx, `
		SELECT e.id, t.id, t.code, `+TermOrderSQL+`, c.id, c.code, c.name_tr, c.name_en, s.section_code,
		       e.attempt_no, e.status, coalesce(e.letter_grade, ''), gi.coefficient::float8,
		       coalesce(gi.is_passing, false), coalesce(gi.counts_in_gpa, false), coalesce(gi.earns_ects, false),
		       e.attendance_status = 'ATTENDANCE_FAIL' OR coalesce(gi.is_attendance_fail, false),
		       e.ects_snapshot::float8, e.credit_snapshot::float8, e.source, e.superseded_by_id IS NOT NULL
		FROM enrollment.enrollments e
		JOIN academic.terms t           ON t.id = e.term_id
		JOIN academic.academic_years ay ON ay.id = t.academic_year_id
		JOIN curriculum.courses c       ON c.id = e.course_id
		JOIN offering.sections s        ON s.id = e.section_id
		LEFT JOIN curriculum.grade_scale_items gi
		       ON gi.grade_scale_id = e.grade_scale_id AND gi.letter = e.letter_grade
		WHERE e.student_program_id = $1 AND e.status <> 'DROPPED'
		ORDER BY 4, c.code, e.id`, studentProgramID)
	if err != nil {
		return nil, fmt.Errorf("enrollment: ders geçmişi okunamadı: %w", err)
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (CourseAttempt, error) {
		var (
			a      CourseAttempt
			status string
		)
		err := row.Scan(&a.ID, &a.TermID, &a.TermCode, &a.TermOrder, &a.CourseID, &a.CourseCode, &a.CourseNameTR,
			&a.CourseNameEN, &a.SectionCode, &a.AttemptNo, &status, &a.Letter, &a.Coefficient,
			&a.Passing, &a.CountsInGPA, &a.EarnsECTS, &a.AbsenceFail, &a.ECTS, &a.Credit, &a.Source, &a.Superseded)
		a.Status = EnrollmentStatus(status)
		return a, err
	})
	if err != nil {
		return nil, fmt.Errorf("enrollment: ders geçmişi okunamadı: %w", err)
	}
	return list, nil
}

// CanReadProgram, kullanıcının program kaydını (ders geçmişi, ders durumu, ders seçimi)
// görebilip göremeyeceğine karar verir: öğrenciyi görebilen herkes (bkz. CanReadStudent).
func (s *Service) CanReadProgram(ctx context.Context, perms *authz.Permissions, userID string, sp StudentProgram) (bool, error) {
	st, err := NewRepository(s.pool).Student(ctx, sp.StudentID)
	if err != nil {
		return false, err
	}
	return s.CanReadStudent(ctx, perms, userID, st)
}
