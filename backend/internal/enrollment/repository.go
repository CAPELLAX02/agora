package enrollment

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// Repository, enrollment şemasına erişen veri katmanıdır.
type Repository struct {
	db db.Querier
}

// NewRepository, verilen bağlantıyı (havuz ya da transaction) kullanan bir Repository oluşturur.
func NewRepository(q db.Querier) *Repository {
	return &Repository{db: q}
}

const studentProgramSelect = `
	SELECT sp.id, s.id, s.student_no, pe.first_name, pe.last_name,
	       p.id, p.code, p.name_tr, d.id, d.name_tr, f.id, f.name_tr,
	       sp.enrollment_kind, sp.admission_type, sp.admission_year, sp.admitted_on, sp.status,
	       sp.class_level, sp.current_semester, sp.gpa_cache::float8, sp.earned_ects_cache::float8, sp.version,
	       aa.id, st.id, st.staff_no, t.name_tr, ape.first_name, ape.last_name, aa.valid_from
	FROM enrollment.student_programs sp
	JOIN people.students s     ON s.id = sp.student_id
	JOIN people.persons pe     ON pe.id = s.person_id
	JOIN org.programs p        ON p.id = sp.program_id
	JOIN org.departments d     ON d.id = p.department_id
	JOIN org.faculties f       ON f.id = d.faculty_id
	LEFT JOIN enrollment.advisor_assignments aa ON aa.student_program_id = sp.id AND aa.valid_until IS NULL
	LEFT JOIN people.staff st  ON st.id = aa.advisor_staff_id
	LEFT JOIN people.persons ape ON ape.id = st.person_id
	LEFT JOIN people.academic_titles t ON t.code = st.academic_title_code`

func scanStudentProgram(row pgx.CollectableRow) (StudentProgram, error) {
	var (
		sp                             StudentProgram
		kind, admission, status        string
		assignmentID, staffID, staffNo *string
		title, advFirst, advLast       *string
		since                          *time.Time
	)
	err := row.Scan(&sp.ID, &sp.StudentID, &sp.StudentNo, &sp.FirstName, &sp.LastName,
		&sp.Program.ID, &sp.Program.Code, &sp.Program.NameTR, &sp.Program.DepartmentID, &sp.Program.DepartmentName,
		&sp.Program.FacultyID, &sp.Program.FacultyName,
		&kind, &admission, &sp.AdmissionYear, &sp.AdmittedOn, &status,
		&sp.ClassLevel, &sp.CurrentSemester, &sp.GPA, &sp.EarnedECTS, &sp.Version,
		&assignmentID, &staffID, &staffNo, &title, &advFirst, &advLast, &since)
	if err != nil {
		return StudentProgram{}, err
	}
	sp.Kind, sp.AdmissionType, sp.Status = Kind(kind), AdmissionType(admission), Status(status)
	if assignmentID != nil {
		sp.Advisor = &AdvisorRef{
			AssignmentID: *assignmentID, StaffID: *staffID, StaffNo: *staffNo,
			Title: deref(title), FirstName: *advFirst, LastName: *advLast, Since: *since,
		}
	}
	return sp, nil
}

// StudentCursor, öğrenci listesinde bir sonraki sayfanın başlangıcıdır.
type StudentCursor struct {
	StudentNo string `json:"n"`
	ID        string `json:"i"` // program kaydı kimliği
}

// StudentFilter, program kaydı listesinin filtreleridir.
type StudentFilter struct {
	Scope        authz.ScopeSet // sadece bu kapsamdaki programların kayıtları listelenir
	FacultyID    string
	DepartmentID string
	ProgramID    string
	Status       Status
	ClassLevel   int // 0 ve altı filtre yok demek değil: hazırlık 0'dır, bu yüzden -1 "hepsi"
	Query        string
	After        *StudentCursor
	Limit        int
}

// ListStudentPrograms, filtreye ve kapsama uyan program kayıtlarını öğrenci
// numarasına göre sıralı döndürür. Kapsam boşsa hiçbir kayıt dönmez.
func (r *Repository) ListStudentPrograms(ctx context.Context, f StudentFilter) (items []StudentProgram, hasMore bool, err error) {
	if f.Scope.Empty() {
		return []StudentProgram{}, false, nil
	}

	var w db.Where
	if !f.Scope.University {
		w.Add("(f.id = ANY($1::uuid[]) OR d.id = ANY($2::uuid[]) OR p.id = ANY($3::uuid[]))",
			f.Scope.FacultyIDs, f.Scope.DepartmentIDs, f.Scope.ProgramIDs)
	}
	if f.FacultyID != "" {
		w.Add("f.id = $1", f.FacultyID)
	}
	if f.DepartmentID != "" {
		w.Add("d.id = $1", f.DepartmentID)
	}
	if f.ProgramID != "" {
		w.Add("p.id = $1", f.ProgramID)
	}
	if f.Status != "" {
		w.Add("sp.status = $1", string(f.Status))
	}
	if f.ClassLevel >= 0 {
		w.Add("sp.class_level = $1", f.ClassLevel)
	}
	if f.Query != "" {
		// Her dal kendi trigram index'ini kullanır (00014). Bkz. iam.ListUsers.
		w.Add(`s.id IN (
			SELECT id FROM people.students WHERE student_no ILIKE $1
			UNION
			SELECT s2.id FROM people.persons p2 JOIN people.students s2 ON s2.person_id = p2.id
			WHERE (p2.first_name || ' ' || p2.last_name) ILIKE $1)`, "%"+f.Query+"%")
	}
	if f.After != nil {
		w.Add("(s.student_no, sp.id) > ($1, $2::uuid)", f.After.StudentNo, f.After.ID)
	}
	limit := w.Arg(f.Limit + 1)

	rows, err := r.db.Query(ctx, studentProgramSelect+"\n\t"+w.SQL()+"\n\tORDER BY s.student_no, sp.id\n\tLIMIT "+limit, w.Args()...)
	if err != nil {
		return nil, false, fmt.Errorf("enrollment: öğrenciler listelenemedi: %w", err)
	}
	items, err = pgx.CollectRows(rows, scanStudentProgram)
	if err != nil {
		return nil, false, fmt.Errorf("enrollment: öğrenciler okunamadı: %w", err)
	}
	if len(items) > f.Limit {
		return items[:f.Limit], true, nil
	}
	return items, false, nil
}

// Student, öğrenciyi bütün program kayıtlarıyla döndürür. Yoksa ErrNotFound döner.
func (r *Repository) Student(ctx context.Context, studentID string) (Student, error) {
	var s Student
	var userID, email *string
	err := r.db.QueryRow(ctx, `
		SELECT s.id, s.person_id, s.student_no, pe.first_name, pe.last_name, u.id, u.email
		FROM people.students s
		JOIN people.persons pe ON pe.id = s.person_id
		LEFT JOIN iam.users u ON u.person_id = s.person_id
		WHERE s.id = $1`, studentID,
	).Scan(&s.ID, &s.PersonID, &s.StudentNo, &s.FirstName, &s.LastName, &userID, &email)
	if errors.Is(err, pgx.ErrNoRows) {
		return Student{}, ErrNotFound
	}
	if err != nil {
		return Student{}, fmt.Errorf("enrollment: öğrenci okunamadı: %w", err)
	}
	s.UserID, s.Email = deref(userID), deref(email)

	rows, err := r.db.Query(ctx, studentProgramSelect+`
		WHERE sp.student_id = $1
		ORDER BY sp.admitted_on, sp.id`, studentID)
	if err != nil {
		return Student{}, fmt.Errorf("enrollment: program kayıtları okunamadı: %w", err)
	}
	if s.Programs, err = pgx.CollectRows(rows, scanStudentProgram); err != nil {
		return Student{}, fmt.Errorf("enrollment: program kayıtları okunamadı: %w", err)
	}
	return s, nil
}

// StudentProgram, program kaydını döndürür. Yoksa ErrNotFound döner.
func (r *Repository) StudentProgram(ctx context.Context, id string) (StudentProgram, error) {
	rows, err := r.db.Query(ctx, studentProgramSelect+"\n\tWHERE sp.id = $1", id)
	if err != nil {
		return StudentProgram{}, fmt.Errorf("enrollment: program kaydı okunamadı: %w", err)
	}
	sp, err := pgx.CollectExactlyOneRow(rows, scanStudentProgram)
	if errors.Is(err, pgx.ErrNoRows) {
		return StudentProgram{}, ErrNotFound
	}
	if err != nil {
		return StudentProgram{}, fmt.Errorf("enrollment: program kaydı okunamadı: %w", err)
	}
	return sp, nil
}

// StudentByUser, kullanıcının öğrenci kaydının kimliğini döndürür. Kullanıcı öğrenci
// değilse ErrNotFound döner.
func (r *Repository) StudentByUser(ctx context.Context, userID string) (string, error) {
	return r.idByUser(ctx, `
		SELECT s.id FROM iam.users u JOIN people.students s ON s.person_id = u.person_id WHERE u.id = $1`, userID)
}

// StaffByUser, kullanıcının personel kaydının kimliğini döndürür. Kullanıcı personel
// değilse ErrNotFound döner.
func (r *Repository) StaffByUser(ctx context.Context, userID string) (string, error) {
	return r.idByUser(ctx, `
		SELECT st.id FROM iam.users u JOIN people.staff st ON st.person_id = u.person_id WHERE u.id = $1`, userID)
}

func (r *Repository) idByUser(ctx context.Context, q, userID string) (string, error) {
	var id string
	err := r.db.QueryRow(ctx, q, userID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("enrollment: kullanıcının kaydı okunamadı: %w", err)
	}
	return id, nil
}

// IsCurrentAdvisor, personelin öğrencinin herhangi bir program kaydında aktif
// danışmanı olup olmadığını söyler.
func (r *Repository) IsCurrentAdvisor(ctx context.Context, staffID, studentID string) (bool, error) {
	var ok bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM enrollment.advisor_assignments aa
			JOIN enrollment.student_programs sp ON sp.id = aa.student_program_id
			WHERE aa.advisor_staff_id = $1 AND sp.student_id = $2 AND aa.valid_until IS NULL
		)`, staffID, studentID).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("enrollment: danışmanlık denetlenemedi: %w", err)
	}
	return ok, nil
}

// Advisees, danışmanın aktif danışmanlık yaptığı program kayıtlarını öğrenci
// numarasına göre sıralı döndürür.
func (r *Repository) Advisees(ctx context.Context, staffID string) ([]StudentProgram, error) {
	rows, err := r.db.Query(ctx, studentProgramSelect+`
		WHERE aa.advisor_staff_id = $1
		ORDER BY s.student_no, sp.id`, staffID)
	if err != nil {
		return nil, fmt.Errorf("enrollment: danışmanlıklar okunamadı: %w", err)
	}
	list, err := pgx.CollectRows(rows, scanStudentProgram)
	if err != nil {
		return nil, fmt.Errorf("enrollment: danışmanlıklar okunamadı: %w", err)
	}
	return list, nil
}

// AdvisorHistory, program kaydının bütün danışman atamalarını en yeniden eskiye döndürür.
func (r *Repository) AdvisorHistory(ctx context.Context, studentProgramID string) ([]AdvisorHistory, error) {
	rows, err := r.db.Query(ctx, `
		SELECT aa.id, st.id, st.staff_no, coalesce(t.name_tr, ''), pe.first_name, pe.last_name,
		       aa.valid_from, aa.valid_until, coalesce(aa.assigned_by::text, ''), coalesce(aa.reason, '')
		FROM enrollment.advisor_assignments aa
		JOIN people.staff st ON st.id = aa.advisor_staff_id
		JOIN people.persons pe ON pe.id = st.person_id
		LEFT JOIN people.academic_titles t ON t.code = st.academic_title_code
		WHERE aa.student_program_id = $1
		ORDER BY aa.valid_from DESC, aa.id DESC`, studentProgramID)
	if err != nil {
		return nil, fmt.Errorf("enrollment: danışman geçmişi okunamadı: %w", err)
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (AdvisorHistory, error) {
		var h AdvisorHistory
		err := row.Scan(&h.AssignmentID, &h.StaffID, &h.StaffNo, &h.Title, &h.FirstName, &h.LastName,
			&h.Since, &h.Until, &h.AssignedBy, &h.Reason)
		return h, err
	})
	if err != nil {
		return nil, fmt.Errorf("enrollment: danışman geçmişi okunamadı: %w", err)
	}
	return list, nil
}

// EligibleAdvisors, bölümde aktif danışman rolü olan, çalışan akademik personeli
// mevcut danışmanlık sayılarıyla birlikte döndürür.
func (r *Repository) EligibleAdvisors(ctx context.Context, departmentID string) ([]EligibleAdvisor, error) {
	rows, err := r.db.Query(ctx, eligibleAdvisorsSelect+`
		ORDER BY pe.last_name COLLATE "tr-x-icu", pe.first_name COLLATE "tr-x-icu", st.id`, departmentID)
	if err != nil {
		return nil, fmt.Errorf("enrollment: danışmanlar okunamadı: %w", err)
	}
	list, err := pgx.CollectRows(rows, scanEligibleAdvisor)
	if err != nil {
		return nil, fmt.Errorf("enrollment: danışmanlar okunamadı: %w", err)
	}
	return list, nil
}

// isEligibleAdvisor, personelin bölümde danışman olarak atanabilir olup olmadığını söyler.
func (r *Repository) isEligibleAdvisor(ctx context.Context, departmentID, staffID string) (bool, error) {
	rows, err := r.db.Query(ctx, eligibleAdvisorsSelect+" AND st.id = $2", departmentID, staffID)
	if err != nil {
		return false, fmt.Errorf("enrollment: danışman denetlenemedi: %w", err)
	}
	list, err := pgx.CollectRows(rows, scanEligibleAdvisor)
	if err != nil {
		return false, fmt.Errorf("enrollment: danışman denetlenemedi: %w", err)
	}
	return len(list) == 1, nil
}

// eligibleAdvisorsSelect: bölüm kapsamında aktif ADVISOR rolü, hesabı aktif ve hâlâ
// görevde olan akademik personel. Rol ataması veritabanı saatiyle değerlendirilir.
const eligibleAdvisorsSelect = `
	SELECT st.id, st.staff_no, coalesce(t.name_tr, ''), pe.first_name, pe.last_name,
	       (SELECT count(*) FROM enrollment.advisor_assignments aa
	        WHERE aa.advisor_staff_id = st.id AND aa.valid_until IS NULL)
	FROM people.staff st
	JOIN people.persons pe ON pe.id = st.person_id
	JOIN iam.users u ON u.person_id = st.person_id AND u.status = 'ACTIVE'
	LEFT JOIN people.academic_titles t ON t.code = st.academic_title_code
	WHERE st.staff_type = 'ACADEMIC' AND st.employment_status = 'ACTIVE'
	  AND EXISTS (
		SELECT 1 FROM iam.role_assignments ra
		JOIN iam.roles ro ON ro.id = ra.role_id
		WHERE ra.user_id = u.id AND ro.code = 'ADVISOR'
		  AND ra.scope_type = 'DEPARTMENT' AND ra.scope_id = $1
		  AND ra.valid_from <= now() AND (ra.valid_until IS NULL OR ra.valid_until > now())
	  )`

func scanEligibleAdvisor(row pgx.CollectableRow) (EligibleAdvisor, error) {
	var a EligibleAdvisor
	err := row.Scan(&a.StaffID, &a.StaffNo, &a.Title, &a.FirstName, &a.LastName, &a.ActiveCount)
	return a, err
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ProgramTarget, programın yetki kapsamı hedefini döndürür. Yoksa ErrUnknownProgram döner.
func (r *Repository) ProgramTarget(ctx context.Context, programID string) (authz.Target, error) {
	t := authz.Target{ProgramID: programID}
	err := r.db.QueryRow(ctx, `
		SELECT d.faculty_id, d.id FROM org.programs p JOIN org.departments d ON d.id = p.department_id
		WHERE p.id = $1`, programID).Scan(&t.FacultyID, &t.DepartmentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return authz.Target{}, ErrUnknownProgram
	}
	if err != nil {
		return authz.Target{}, fmt.Errorf("enrollment: program okunamadı: %w", err)
	}
	return t, nil
}

// DepartmentTarget, bölümün yetki kapsamı hedefini döndürür. Yoksa ErrNotFound döner.
func (r *Repository) DepartmentTarget(ctx context.Context, departmentID string) (authz.Target, error) {
	t := authz.Target{DepartmentID: departmentID}
	err := r.db.QueryRow(ctx, `SELECT faculty_id FROM org.departments WHERE id = $1`, departmentID).Scan(&t.FacultyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return authz.Target{}, ErrNotFound
	}
	if err != nil {
		return authz.Target{}, fmt.Errorf("enrollment: bölüm okunamadı: %w", err)
	}
	return t, nil
}
