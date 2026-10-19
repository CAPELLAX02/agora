package attendance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/notify"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// Oturum grupları: devamsızlık sınırı teori ve uygulama (uygulama ve laboratuvar) için ayrıdır.
const (
	GroupTheory   = "THEORY"
	GroupPractice = "PRACTICE"
)

// groupExpr, c oturumunun grubudur.
const groupExpr = `CASE c.session_type WHEN 'THEORY' THEN 'THEORY' ELSE 'PRACTICE' END`

// Usage, bir kesin kaydın bir oturum grubundaki devamsızlığıdır.
type Usage struct {
	Group    string
	Planned  int  // iptal edilmeyen oturumlar (telafi dahil)
	Held     int  // yoklaması alınmış oturumlar
	Absences int  // devamsız olduğu (iptal edilmemiş) oturumlar
	Pct      int  // yönetmelikteki en fazla devamsızlık yüzdesi; tanımlı değilse 0
	Limit    *int // planlanan oturumların Pct'si (aşağı yuvarlanır); yüzde tanımlı değilse nil
}

// Exceeded, devamsızlığın sınırı aşıp aşmadığıdır.
func (u Usage) Exceeded() bool { return u.Limit != nil && u.Absences > *u.Limit }

// Near, devamsızlığın sınırın %70'ine ulaşıp ulaşmadığıdır (sınır sıfırsa uyarı yok).
func (u Usage) Near() bool { return u.Limit != nil && *u.Limit > 0 && u.Absences*10 >= *u.Limit*7 }

// limits, terimin başladığı gün geçerli devamsızlık yüzdeleridir (grup başına).
func limits(ctx context.Context, q db.Querier, termID string) (map[string]int, error) {
	var raw []byte
	err := q.QueryRow(ctx, `
		SELECT p.value FROM curriculum.regulation_parameters p, academic.terms t
		WHERE t.id = $1 AND p.key = 'attendance_max_absence_pct'
		  AND p.effective_from <= t.starts_on AND (p.effective_to IS NULL OR p.effective_to > t.starts_on)`, termID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return map[string]int{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("attendance: devamsızlık sınırı okunamadı: %w", err)
	}
	var out map[string]int
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("attendance: devamsızlık sınırı çözümlenemedi: %w", err)
	}
	return out, nil
}

// usages, kesin kayıtların (aynı dönemden) grup başına devamsızlıklarıdır. Oturumu olmayan grup
// listelenmez.
func usages(ctx context.Context, q db.Querier, termID string, enrollmentIDs []string) (map[string][]Usage, error) {
	pct, err := limits(ctx, q, termID)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `
		SELECT e.id::text, `+groupExpr+`,
		       count(*) FILTER (WHERE c.status <> 'CANCELLED'),
		       count(*) FILTER (WHERE c.status = 'HELD'),
		       count(*) FILTER (WHERE c.status <> 'CANCELLED' AND r.status = 'ABSENT')
		FROM enrollment.enrollments e
		JOIN attendance.class_sessions c ON c.section_id = e.section_id
		LEFT JOIN attendance.attendance_records r ON r.term_id = c.term_id AND r.session_id = c.id AND r.enrollment_id = e.id
		WHERE e.id = ANY ($1::uuid[])
		GROUP BY 1, 2 ORDER BY 1, 2 DESC`, enrollmentIDs)
	if err != nil {
		return nil, fmt.Errorf("attendance: devamsızlık okunamadı: %w", err)
	}
	out := map[string][]Usage{}
	var (
		id string
		u  Usage
	)
	if _, err := pgx.ForEachRow(rows, []any{&id, &u.Group, &u.Planned, &u.Held, &u.Absences}, func() error {
		x := u
		if p, ok := pct[x.Group]; ok {
			limit := x.Planned * p / 100
			x.Pct, x.Limit = p, &limit
		}
		out[id] = append(out[id], x)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("attendance: devamsızlık okunamadı: %w", err)
	}
	return out, nil
}

// StudentSession, öğrencinin raporunda bir oturumdur: kendi yoklama durumuyla (alınmamışsa boş).
type StudentSession struct {
	ID, Type, Kind, Status string
	StartsAt, EndsAt       time.Time
	Topic                  string
	Record                 string
}

// CourseAttendance, öğrencinin bir dersteki devamsızlığıdır.
type CourseAttendance struct {
	EnrollmentID, SectionID, SectionCode   string
	CourseCode, CourseNameTR, CourseNameEN string
	AttendanceStatus                       string // CONTINUING ya da ATTENDANCE_FAIL (öğretim elemanının kararı)
	Usage                                  []Usage
	Sessions                               []StudentSession
}

// Mine, kullanıcının dönemdeki kesin kayıtlarının devamsızlığıdır (bırakılanlar hariç), ders
// koduna göre sıralı.
func (s *Service) Mine(ctx context.Context, userID, termID string) ([]CourseAttendance, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT e.id, e.section_id, sec.section_code, c.code, c.name_tr, c.name_en, e.attendance_status
		FROM enrollment.enrollments e
		JOIN enrollment.student_programs sp ON sp.id = e.student_program_id
		JOIN people.students st ON st.id = sp.student_id
		JOIN iam.users u ON u.person_id = st.person_id
		JOIN offering.sections sec ON sec.id = e.section_id
		JOIN curriculum.courses c ON c.id = e.course_id
		WHERE u.id = $1 AND e.term_id = $2 AND e.status <> 'DROPPED'
		ORDER BY c.code`, userID, termID)
	if err != nil {
		return nil, fmt.Errorf("attendance: dersler okunamadı: %w", err)
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (CourseAttendance, error) {
		var c CourseAttendance
		return c, row.Scan(&c.EnrollmentID, &c.SectionID, &c.SectionCode, &c.CourseCode, &c.CourseNameTR, &c.CourseNameEN, &c.AttendanceStatus)
	})
	if err != nil {
		return nil, fmt.Errorf("attendance: dersler okunamadı: %w", err)
	}
	if len(list) == 0 {
		return list, nil
	}
	ids := make([]string, len(list))
	for i, c := range list {
		ids[i] = c.EnrollmentID
	}
	use, err := usages(ctx, s.pool, termID, ids)
	if err != nil {
		return nil, err
	}
	rows, err = s.pool.Query(ctx, `
		SELECT e.id::text, c.id, c.session_type, c.kind, c.status, c.starts_at, c.ends_at, coalesce(c.topic, ''), coalesce(r.status, '')
		FROM enrollment.enrollments e
		JOIN attendance.class_sessions c ON c.section_id = e.section_id
		LEFT JOIN attendance.attendance_records r ON r.term_id = c.term_id AND r.session_id = c.id AND r.enrollment_id = e.id
		WHERE e.id = ANY ($1::uuid[])
		ORDER BY c.starts_at`, ids)
	if err != nil {
		return nil, fmt.Errorf("attendance: oturumlar okunamadı: %w", err)
	}
	sessions := map[string][]StudentSession{}
	var (
		id string
		x  StudentSession
	)
	if _, err := pgx.ForEachRow(rows, []any{&id, &x.ID, &x.Type, &x.Kind, &x.Status, &x.StartsAt, &x.EndsAt, &x.Topic, &x.Record}, func() error {
		sessions[id] = append(sessions[id], x)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("attendance: oturumlar okunamadı: %w", err)
	}
	for i := range list {
		list[i].Usage = use[list[i].EnrollmentID]
		list[i].Sessions = sessions[list[i].EnrollmentID]
	}
	return list, nil
}

// StudentUsage, şube özetinde bir öğrencidir.
type StudentUsage struct {
	EnrollmentID, StudentNo, FirstName, LastName string
	AttendanceStatus                             string
	Usage                                        []Usage
}

// Summary, şubenin öğrenci başına devamsızlığıdır (numara sırasıyla).
func (s *Service) Summary(ctx context.Context, sectionID string) ([]StudentUsage, error) {
	var termID string
	if err := s.pool.QueryRow(ctx, `
		SELECT o.term_id FROM offering.sections s JOIN offering.course_offerings o ON o.id = s.offering_id WHERE s.id = $1`,
		sectionID).Scan(&termID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("attendance: şube okunamadı: %w", err)
	}
	rows, err := s.pool.Query(ctx, `
		SELECT e.id, st.student_no, p.first_name, p.last_name, e.attendance_status
		FROM enrollment.enrollments e
		JOIN enrollment.student_programs sp ON sp.id = e.student_program_id
		JOIN people.students st ON st.id = sp.student_id
		JOIN people.persons p ON p.id = st.person_id
		WHERE e.section_id = $1 AND e.status <> 'DROPPED'
		ORDER BY st.student_no`, sectionID)
	if err != nil {
		return nil, fmt.Errorf("attendance: öğrenciler okunamadı: %w", err)
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (StudentUsage, error) {
		var x StudentUsage
		return x, row.Scan(&x.EnrollmentID, &x.StudentNo, &x.FirstName, &x.LastName, &x.AttendanceStatus)
	})
	if err != nil || len(list) == 0 {
		if err != nil {
			err = fmt.Errorf("attendance: öğrenciler okunamadı: %w", err)
		}
		return list, err
	}
	ids := make([]string, len(list))
	for i, x := range list {
		ids[i] = x.EnrollmentID
	}
	use, err := usages(ctx, s.pool, termID, ids)
	if err != nil {
		return nil, err
	}
	for i := range list {
		list[i].Usage = use[list[i].EnrollmentID]
	}
	return list, nil
}

// groupNames, bildirimlerde oturum gruplarının adlarıdır.
var groupNames = map[string][2]string{GroupTheory: {"Teori", "theory"}, GroupPractice: {"Uygulama", "practice"}}

// warn, kesin kayıtların grubundaki devamsızlık sınırın %70'ine ulaşmış ya da sınırı aşmışsa
// o düzeyin uyarısını (henüz gönderilmemişse) bildirir. Yoklamayla aynı transaction'dadır.
func warn(ctx context.Context, tx pgx.Tx, termID, group string, enrollmentIDs []string) error {
	use, err := usages(ctx, tx, termID, enrollmentIDs)
	if err != nil {
		return err
	}
	for id, list := range use {
		for _, u := range list {
			if u.Group != group || !u.Near() && !u.Exceeded() {
				continue
			}
			level := "NEAR"
			if u.Exceeded() {
				level = "OVER"
			}
			var userID, courseCode, sectionID string
			err := tx.QueryRow(ctx, `
				WITH w AS (INSERT INTO attendance.warnings_sent (enrollment_id, session_group, level) VALUES ($1, $2, $3)
				           ON CONFLICT DO NOTHING RETURNING enrollment_id)
				SELECT u.id, c.code, e.section_id FROM w
				JOIN enrollment.enrollments e ON e.id = w.enrollment_id
				JOIN enrollment.student_programs sp ON sp.id = e.student_program_id
				JOIN people.students st ON st.id = sp.student_id
				JOIN iam.users u ON u.person_id = st.person_id
				JOIN curriculum.courses c ON c.id = e.course_id`, id, group, level).Scan(&userID, &courseCode, &sectionID)
			if errors.Is(err, pgx.ErrNoRows) {
				continue // bu düzeyin uyarısı gönderilmiş
			}
			if err != nil {
				return fmt.Errorf("attendance: uyarı kaydedilemedi: %w", err)
			}
			names := groupNames[group]
			if err := notify.Send(ctx, tx, notify.Notification{Type: notify.AttendanceWarning, UserIDs: []string{userID},
				Link: "/devamsizligim",
				Data: map[string]any{"course_code": courseCode, "level": level, "group_tr": names[0], "group_en": names[1],
					"absences": u.Absences, "limit": *u.Limit}}); err != nil {
				return err
			}
		}
	}
	return nil
}
