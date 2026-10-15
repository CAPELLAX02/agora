package lms

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/files"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// Service, ders alanı işlemleridir. sink, ödevlerden hesaplanan bileşen notlarını not
// modülüne yazar (nil ise köprü kapalı).
type Service struct {
	pool   *pgxpool.Pool
	files  *files.Service
	sink   GradeSink
	logger *slog.Logger
	now    func() time.Time
}

// NewService, bir Service oluşturur.
func NewService(pool *pgxpool.Pool, fs *files.Service, sink GradeSink, logger *slog.Logger, now func() time.Time) *Service {
	return &Service{pool: pool, files: fs, sink: sink, logger: logger, now: now}
}

const sectionColumns = `s.id, s.section_code, c.id, c.code, c.name_tr, c.name_en, t.id, t.code, o.department_id, d.faculty_id`

const sectionJoins = `
	FROM offering.sections s
	JOIN offering.course_offerings o ON o.id = s.offering_id
	JOIN curriculum.courses c ON c.id = o.course_id
	JOIN academic.terms t ON t.id = o.term_id
	JOIN org.departments d ON d.id = o.department_id`

func scanSection(row pgx.Row, extra ...any) (Section, error) {
	var s Section
	dest := append([]any{&s.ID, &s.Code, &s.CourseID, &s.CourseCode, &s.CourseNameTR, &s.CourseNameEN,
		&s.TermID, &s.TermCode, &s.DepartmentID, &s.FacultyID}, extra...)
	err := row.Scan(dest...)
	if errors.Is(err, pgx.ErrNoRows) {
		return Section{}, ErrNotFound
	}
	if err != nil {
		return Section{}, fmt.Errorf("lms: şube okunamadı: %w", err)
	}
	return s, nil
}

// roleExpr, $2 kullanıcısının s şubesindeki rolüdür: öğretim elemanı ataması, yoksa bırakmamış
// kesin kayıt (öğrenci), yoksa boş.
const roleExpr = `coalesce(
	(SELECT si.role FROM offering.section_instructors si
	 JOIN people.staff st ON st.id = si.staff_id JOIN iam.users u ON u.person_id = st.person_id
	 WHERE si.section_id = s.id AND u.id = $2),
	(SELECT 'STUDENT' FROM enrollment.enrollments e
	 JOIN enrollment.student_programs sp ON sp.id = e.student_program_id
	 JOIN people.students st ON st.id = sp.student_id JOIN iam.users u ON u.person_id = st.person_id
	 WHERE e.section_id = s.id AND e.status <> 'DROPPED' AND u.id = $2 LIMIT 1),
	'')`

// Membership, şubeyi ve kullanıcının oradaki rolünü döndürür. İptal edilen şube bulunamaz.
func (s *Service) Membership(ctx context.Context, q db.Querier, sectionID, userID string) (Section, Role, error) {
	var role string
	sec, err := scanSection(q.QueryRow(ctx, `SELECT `+sectionColumns+`, `+roleExpr+sectionJoins+`
		WHERE s.id = $1 AND s.status <> 'CANCELLED' AND o.status <> 'CANCELLED'`, sectionID, userID), &role)
	return sec, Role(role), err
}

// MySpaces, kullanıcının dönemdeki ders alanlarıdır: öğretim elemanı olduğu ve bırakmamış
// kesin kaydı olduğu şubeler.
func (s *Service) MySpaces(ctx context.Context, userID, termID string) ([]SpaceSummary, error) {
	rows, err := s.pool.Query(ctx, `
		WITH mine AS (
			SELECT si.section_id, si.role FROM offering.section_instructors si
			JOIN people.staff st ON st.id = si.staff_id JOIN iam.users u ON u.person_id = st.person_id
			WHERE u.id = $1
			UNION
			SELECT e.section_id, 'STUDENT' FROM enrollment.enrollments e
			JOIN enrollment.student_programs sp ON sp.id = e.student_program_id
			JOIN people.students st ON st.id = sp.student_id JOIN iam.users u ON u.person_id = st.person_id
			WHERE u.id = $1 AND e.status <> 'DROPPED'
		)
		SELECT `+sectionColumns+`, m.role,
		       (SELECT count(*) FROM lms.activities a JOIN lms.topics tp ON tp.id = a.topic_id
		        WHERE a.section_id = s.id AND a.deleted_at IS NULL AND a.is_visible AND tp.is_visible AND tp.deleted_at IS NULL)
		FROM mine m JOIN offering.sections s ON s.id = m.section_id`+
		` JOIN offering.course_offerings o ON o.id = s.offering_id
		JOIN curriculum.courses c ON c.id = o.course_id
		JOIN academic.terms t ON t.id = o.term_id
		JOIN org.departments d ON d.id = o.department_id
		WHERE o.term_id = $2 AND s.status <> 'CANCELLED' AND o.status <> 'CANCELLED'
		ORDER BY c.code, s.section_code`, userID, termID)
	if err != nil {
		return nil, fmt.Errorf("lms: ders alanları okunamadı: %w", err)
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (SpaceSummary, error) {
		var (
			sum  SpaceSummary
			role string
		)
		sec, err := scanSection(row, &role, &sum.Activities)
		sum.Section, sum.Role = sec, Role(role)
		return sum, err
	})
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(list))
	for i, sp := range list {
		ids[i] = sp.ID
	}
	instructors, err := s.instructors(ctx, s.pool, ids)
	if err != nil {
		return nil, err
	}
	for i := range list {
		list[i].Instructors = instructors[list[i].ID]
	}
	return list, nil
}

// instructors, şubelerin öğretim elemanlarıdır (sorumlu, ortak, asistan sırasıyla).
func (s *Service) instructors(ctx context.Context, q db.Querier, sectionIDs []string) (map[string][]Instructor, error) {
	rows, err := q.Query(ctx, `
		SELECT si.section_id, u.id, coalesce(at.name_tr, ''), p.first_name, p.last_name, u.email, si.role
		FROM offering.section_instructors si
		JOIN people.staff st ON st.id = si.staff_id
		JOIN people.persons p ON p.id = st.person_id
		JOIN iam.users u ON u.person_id = p.id
		LEFT JOIN people.academic_titles at ON at.code = st.academic_title_code
		WHERE si.section_id = ANY($1)
		ORDER BY array_position(ARRAY['PRIMARY', 'CO_INSTRUCTOR', 'ASSISTANT'], si.role), p.last_name, p.first_name`, sectionIDs)
	if err != nil {
		return nil, fmt.Errorf("lms: öğretim elemanları okunamadı: %w", err)
	}
	out := map[string][]Instructor{}
	var (
		sectionID, role string
		in              Instructor
	)
	_, err = pgx.ForEachRow(rows, []any{&sectionID, &in.UserID, &in.Title, &in.FirstName, &in.LastName, &in.Email, &role}, func() error {
		in.Role = Role(role)
		out[sectionID] = append(out[sectionID], in)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("lms: öğretim elemanları okunamadı: %w", err)
	}
	return out, nil
}

// Space, ders alanını izleyenin rolüne göre döndürür. Düzenleyen her şeyi görür; öğrenci
// gizli bölüm ve etkinlikleri görmez, henüz açılmamış ya da kapanmış olanların sadece adını ve
// tarihlerini görür (içerik yok).
func (s *Service) Space(ctx context.Context, sec Section, role Role) (Space, error) {
	sp := Space{Section: sec, Role: role}
	err := s.pool.QueryRow(ctx, `SELECT summary_html FROM lms.course_spaces WHERE section_id = $1`, sec.ID).Scan(&sp.Summary)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Space{}, fmt.Errorf("lms: ders alanı okunamadı: %w", err)
	}
	instructors, err := s.instructors(ctx, s.pool, []string{sec.ID})
	if err != nil {
		return Space{}, err
	}
	sp.Instructors = instructors[sec.ID]
	if sp.Topics, err = s.topics(ctx, s.pool, sec.ID); err != nil {
		return Space{}, err
	}
	if !role.CanEdit() {
		sp.Topics = forLearners(sp.Topics, s.now())
	}
	return sp, nil
}

// forLearners, öğrencinin gördüğüdür: gizliler atılır, açık olmayanların içeriği boşaltılır.
func forLearners(topics []Topic, now time.Time) []Topic {
	out := make([]Topic, 0, len(topics))
	for _, t := range topics {
		if !t.Visible {
			continue
		}
		if t.Window.At(now) != Open {
			t.Summary, t.Activities = "", nil
			out = append(out, t)
			continue
		}
		acts := make([]Activity, 0, len(t.Activities))
		for _, a := range t.Activities {
			if !a.Visible {
				continue
			}
			if a.Window.At(now) != Open {
				a.Description, a.Files, a.URL = "", nil, ""
			}
			acts = append(acts, a)
		}
		t.Activities = acts
		out = append(out, t)
	}
	return out
}

// topics, şubenin silinmemiş bölümleri ve etkinlikleridir (sıralı).
func (s *Service) topics(ctx context.Context, q db.Querier, sectionID string) ([]Topic, error) {
	rows, err := q.Query(ctx, `
		SELECT id, title, summary_html, position, is_visible, available_from, available_until, version
		FROM lms.topics WHERE section_id = $1 AND deleted_at IS NULL ORDER BY position, created_at`, sectionID)
	if err != nil {
		return nil, fmt.Errorf("lms: bölümler okunamadı: %w", err)
	}
	topics, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Topic, error) {
		var t Topic
		err := row.Scan(&t.ID, &t.Title, &t.Summary, &t.Position, &t.Visible, &t.Window.From, &t.Window.Until, &t.Version)
		return t, err
	})
	if err != nil {
		return nil, fmt.Errorf("lms: bölümler okunamadı: %w", err)
	}
	acts, err := s.activities(ctx, q, `a.section_id = $1`, sectionID)
	if err != nil {
		return nil, err
	}
	index := map[string]int{}
	for i, t := range topics {
		index[t.ID] = i
	}
	for _, a := range acts {
		if i, ok := index[a.TopicID]; ok {
			topics[i].Activities = append(topics[i].Activities, a)
		}
	}
	return topics, nil
}

// activities, koşula uyan silinmemiş etkinliklerdir; dosya ve bağlantı bilgileriyle.
func (s *Service) activities(ctx context.Context, q db.Querier, where string, args ...any) ([]Activity, error) {
	rows, err := q.Query(ctx, `
		SELECT a.id, a.topic_id, a.activity_type, a.title, a.description_html, a.position, a.is_visible,
		       a.available_from, a.available_until, a.version, a.created_at, a.updated_at,
		       coalesce(u.url, ''), coalesce(u.open_in_new_tab, false)
		FROM lms.activities a LEFT JOIN lms.urls u ON u.activity_id = a.id
		WHERE a.deleted_at IS NULL AND `+where+` ORDER BY a.position, a.created_at`, args...)
	if err != nil {
		return nil, fmt.Errorf("lms: etkinlikler okunamadı: %w", err)
	}
	acts, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Activity, error) {
		var (
			a   Activity
			typ string
		)
		err := row.Scan(&a.ID, &a.TopicID, &typ, &a.Title, &a.Description, &a.Position, &a.Visible,
			&a.Window.From, &a.Window.Until, &a.Version, &a.CreatedAt, &a.UpdatedAt, &a.URL, &a.NewTab)
		a.Type = ActivityType(typ)
		return a, err
	})
	if err != nil {
		return nil, fmt.Errorf("lms: etkinlikler okunamadı: %w", err)
	}
	if len(acts) == 0 {
		return acts, nil
	}
	ids := make([]string, len(acts))
	index := map[string]int{}
	for i, a := range acts {
		ids[i], index[a.ID] = a.ID, i
	}
	rows, err = q.Query(ctx, `
		SELECT af.activity_id, f.id, f.original_name, f.mime_type, f.size_bytes
		FROM lms.activity_files af JOIN platform.files f ON f.id = af.file_id
		WHERE af.activity_id = ANY($1) AND f.deleted_at IS NULL
		ORDER BY af.position`, ids)
	if err != nil {
		return nil, fmt.Errorf("lms: dosyalar okunamadı: %w", err)
	}
	var (
		activityID string
		f          FileRef
	)
	_, err = pgx.ForEachRow(rows, []any{&activityID, &f.ID, &f.Name, &f.MimeType, &f.Size}, func() error {
		i := index[activityID]
		acts[i].Files = append(acts[i].Files, f)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("lms: dosyalar okunamadı: %w", err)
	}
	var assignIDs []string
	for _, a := range acts {
		if a.Type == TypeAssignment {
			assignIDs = append(assignIDs, a.ID)
		}
	}
	if len(assignIDs) > 0 {
		settings, err := assignments(ctx, q, assignIDs)
		if err != nil {
			return nil, err
		}
		for id, st := range settings {
			acts[index[id]].Assignment = &st
		}
	}
	return acts, nil
}

// Activity, bir etkinliği (sayfa gövdesiyle) ve ait olduğu şubeyi döndürür. Erişimi çağıran
// denetler (Accessible).
func (s *Service) Activity(ctx context.Context, id string) (Activity, string, error) {
	var sectionID string
	err := s.pool.QueryRow(ctx, `SELECT section_id FROM lms.activities WHERE id = $1 AND deleted_at IS NULL`, id).Scan(&sectionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Activity{}, "", ErrNotFound
	}
	if err != nil {
		return Activity{}, "", fmt.Errorf("lms: etkinlik okunamadı: %w", err)
	}
	acts, err := s.activities(ctx, s.pool, `a.id = $1`, id)
	if err != nil {
		return Activity{}, "", err
	}
	a := acts[0]
	if a.Type == TypePage {
		if err := s.pool.QueryRow(ctx, `SELECT body_html FROM lms.pages WHERE activity_id = $1`, id).Scan(&a.Body); err != nil &&
			!errors.Is(err, pgx.ErrNoRows) {
			return Activity{}, "", fmt.Errorf("lms: sayfa okunamadı: %w", err)
		}
	}
	return a, sectionID, nil
}

// Accessible, etkinliğin role açık olduğunu denetler: düzenleyen her zaman; öğrenci ve
// izleyici sadece görünür bölümdeki görünür ve açık etkinliğe.
func (s *Service) Accessible(ctx context.Context, a Activity, role Role) error {
	return s.access(ctx, a, role, false)
}

// Reviewable, Accessible gibidir ama kapanmış etkinliğe de izin verir: öğrenci, süresi geçen
// ödevdeki teslimini, puanını ve yorumları görmeye devam eder.
func (s *Service) Reviewable(ctx context.Context, a Activity, role Role) error {
	return s.access(ctx, a, role, true)
}

func (s *Service) access(ctx context.Context, a Activity, role Role, allowClosed bool) error {
	if role.CanEdit() {
		return nil
	}
	if role == RoleNone {
		return ErrNotFound
	}
	var (
		visible bool
		w       Window
	)
	err := s.pool.QueryRow(ctx, `SELECT is_visible, available_from, available_until FROM lms.topics WHERE id = $1`, a.TopicID).
		Scan(&visible, &w.From, &w.Until)
	if err != nil {
		return fmt.Errorf("lms: bölüm okunamadı: %w", err)
	}
	if !visible || !a.Visible {
		return ErrNotFound
	}
	now := s.now()
	ok := func(av Availability) bool { return av == Open || (allowClosed && av == Closed) }
	if !ok(w.At(now)) || !ok(a.Window.At(now)) {
		return ErrNotAvailable
	}
	return nil
}

// FileURL, etkinliğin dosyasının imzalı indirme URL'sidir (erişim Accessible ile denetlenmiş olmalı).
func (s *Service) FileURL(ctx context.Context, a Activity, fileID string) (string, time.Time, error) {
	for _, f := range a.Files {
		if f.ID == fileID {
			file, err := s.files.Get(ctx, s.pool, fileID)
			if err != nil {
				return "", time.Time{}, err
			}
			return s.files.DownloadURL(ctx, file)
		}
	}
	return "", time.Time{}, ErrNotFound
}
