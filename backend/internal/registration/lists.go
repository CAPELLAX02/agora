package registration

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/platform/authz"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// Summary, danışman ve öğrenci işleri listelerinde bir öğrencinin dönemdeki ders seçme
// özetidir. Öğrencinin henüz sepeti yoksa durum NOT_STARTED'dır.
type Summary struct {
	StudentProgramID string
	StudentID        string
	StudentNo        string
	FirstName        string
	LastName         string
	ProgramCode      string
	Semester         int
	GPA              *float64
	RegistrationID   string
	Status           Status
	TotalECTS        float64
	ItemCount        int
	SubmittedAt      *time.Time
	Version          int
	Blocked          bool // ders seçmeyi kapatan aktif engeli var
}

const summarySelect = `
	SELECT sp.id, s.id, s.student_no, pe.first_name, pe.last_name, p.code, sp.current_semester, sp.gpa_cache::float8,
	       coalesce(r.id::text, ''), coalesce(r.status, 'NOT_STARTED'), coalesce(r.total_ects, 0)::float8,
	       (SELECT count(*) FROM enrollment.registration_items i WHERE i.registration_id = r.id AND i.status = 'ACTIVE'),
	       r.submitted_at, coalesce(r.version, 0),
	       EXISTS (SELECT 1 FROM enrollment.holds h WHERE h.student_program_id = sp.id
	               AND h.released_at IS NULL AND 'REGISTRATION' = ANY(h.blocks))
	FROM enrollment.student_programs sp
	JOIN people.students s ON s.id = sp.student_id
	JOIN people.persons pe ON pe.id = s.person_id
	JOIN org.programs p ON p.id = sp.program_id
	JOIN org.departments d ON d.id = p.department_id
	LEFT JOIN enrollment.registrations r ON r.student_program_id = sp.id AND r.term_id = $1`

func scanSummary(row pgx.CollectableRow) (Summary, error) {
	var (
		s      Summary
		status string
		sem    int16
	)
	err := row.Scan(&s.StudentProgramID, &s.StudentID, &s.StudentNo, &s.FirstName, &s.LastName, &s.ProgramCode, &sem,
		&s.GPA, &s.RegistrationID, &status, &s.TotalECTS, &s.ItemCount, &s.SubmittedAt, &s.Version, &s.Blocked)
	s.Status, s.Semester = Status(status), int(sem)
	return s, err
}

// statusOrder: onay bekleyenler önce.
const statusOrder = `CASE coalesce(r.status, 'NOT_STARTED') WHEN 'SUBMITTED' THEN 0 WHEN 'DRAFT' THEN 1
	WHEN 'REJECTED' THEN 2 WHEN 'NOT_STARTED' THEN 3 ELSE 4 END`

// AdviseeRegistrations, danışmanın aktif danışmanlıklarındaki süren kayıtların dönemdeki ders
// seçme özetlerini döndürür: onay bekleyenler önce, sonra öğrenci numarası sırasıyla.
func (r *Repository) AdviseeRegistrations(ctx context.Context, staffID, termID string, status Status) ([]Summary, error) {
	var w db.Where
	w.Arg(termID) // $1: dönem
	w.Add(`sp.id IN (SELECT student_program_id FROM enrollment.advisor_assignments
		WHERE advisor_staff_id = $1 AND valid_until IS NULL)`, staffID)
	w.Add("sp.status IN ('ACTIVE', 'FROZEN', 'SUSPENDED')")
	if status != "" {
		w.Add("coalesce(r.status, 'NOT_STARTED') = $1", string(status))
	}
	rows, err := r.db.Query(ctx, summarySelect+"\n\t"+w.SQL()+"\n\tORDER BY "+statusOrder+", s.student_no, sp.id", w.Args()...)
	if err != nil {
		return nil, fmt.Errorf("registration: danışmanlık listesi okunamadı: %w", err)
	}
	list, err := pgx.CollectRows(rows, scanSummary)
	if err != nil {
		return nil, fmt.Errorf("registration: danışmanlık listesi okunamadı: %w", err)
	}
	return list, nil
}

// SummaryCursor, öğrenci işleri listesinde bir sonraki sayfanın başlangıcıdır.
type SummaryCursor struct {
	StudentNo string `json:"n"`
	ID        string `json:"i"`
}

// ListFilter, öğrenci işleri listesinin filtreleridir.
type ListFilter struct {
	Scope        authz.ScopeSet
	TermID       string
	FacultyID    string
	DepartmentID string
	ProgramID    string
	Status       Status
	Query        string
	After        *SummaryCursor
	Limit        int
}

// ListRegistrations, kapsamdaki aktif öğrencilerin dönemdeki ders seçme özetlerini öğrenci
// numarasına göre sıralı döndürür. Kapsam boşsa hiçbir kayıt dönmez.
func (r *Repository) ListRegistrations(ctx context.Context, f ListFilter) (items []Summary, hasMore bool, err error) {
	if f.Scope.Empty() {
		return []Summary{}, false, nil
	}
	var w db.Where
	w.Arg(f.TermID)
	w.Add("sp.status = 'ACTIVE'")
	if !f.Scope.University {
		w.Add("(d.faculty_id = ANY($1::uuid[]) OR d.id = ANY($2::uuid[]) OR p.id = ANY($3::uuid[]))",
			f.Scope.FacultyIDs, f.Scope.DepartmentIDs, f.Scope.ProgramIDs)
	}
	if f.FacultyID != "" {
		w.Add("d.faculty_id = $1", f.FacultyID)
	}
	if f.DepartmentID != "" {
		w.Add("d.id = $1", f.DepartmentID)
	}
	if f.ProgramID != "" {
		w.Add("p.id = $1", f.ProgramID)
	}
	if f.Status != "" {
		w.Add("coalesce(r.status, 'NOT_STARTED') = $1", string(f.Status))
	}
	if f.Query != "" {
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
	rows, err := r.db.Query(ctx, summarySelect+"\n\t"+w.SQL()+"\n\tORDER BY s.student_no, sp.id\n\tLIMIT "+limit, w.Args()...)
	if err != nil {
		return nil, false, fmt.Errorf("registration: kayıtlar listelenemedi: %w", err)
	}
	items, err = pgx.CollectRows(rows, scanSummary)
	if err != nil {
		return nil, false, fmt.Errorf("registration: kayıtlar okunamadı: %w", err)
	}
	if len(items) > f.Limit {
		return items[:f.Limit], true, nil
	}
	return items, false, nil
}
