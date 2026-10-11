package grading

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/academic"
	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// Not girişi hataları.
var (
	ErrPlanNotLocked      = errors.New("grading: değerlendirme planı kilitlenmemiş")
	ErrEntryWindowClosed  = errors.New("grading: not girişi penceresi kapalı")
	ErrMakeupNotAvailable = errors.New("grading: bütünleme notları harf notları ilan edildikten sonra girilir")
	ErrNoScores           = errors.New("grading: yayımlanacak not yok")
)

// UnknownEnrollmentsError: istekte şubenin kesin kayıtlı öğrencisi olmayan kayıtlar var.
type UnknownEnrollmentsError struct {
	IDs []string
}

func (e *UnknownEnrollmentsError) Error() string {
	return fmt.Sprintf("grading: şubede olmayan %d kayıt", len(e.IDs))
}

// Not girişi pencereleri (academic.calendar_event_types).
const (
	WindowMidtermEntry = "MIDTERM_GRADE_ENTRY"
	WindowFinalEntry   = "FINAL_GRADE_ENTRY"
	WindowMakeupEntry  = "MAKEUP_GRADE_ENTRY"
)

// entryWindows, bir bileşen kategorisinin notunun girilebildiği pencerelerdir. Dönem içi
// bileşenler (ödev, proje ...) çoğu zaman dönem sonunda değerlendirildiği için final not
// girişi penceresinde de girilebilir.
func entryWindows(c Category) []string {
	switch c {
	case CategoryInTerm:
		return []string{WindowMidtermEntry, WindowFinalEntry}
	case CategoryFinal:
		return []string{WindowFinalEntry}
	case CategoryMakeup:
		return []string{WindowMakeupEntry}
	}
	return nil
}

// Points, yüzde birlik tamsayı puandır: 7550 = 75,50. Puanlar kayan noktaya hiç
// çevrilmeden toplanır ve karşılaştırılır (yol haritası §5 #14).
type Points int64

// Float, puanı yanıt için ondalık sayıya çevirir.
func (p Points) Float() float64 { return float64(p) / 100 }

// mean, puanların ortalamasıdır; yüzde birliğe yarım yukarı yuvarlanır.
func mean(sum Points, n int) Points {
	if n == 0 {
		return 0
	}
	return (2*sum + Points(n)) / (2 * Points(n))
}

// Attendance, öğrencinin derse devam durumudur.
type Attendance string

// Devam durumları.
const (
	AttendanceContinuing Attendance = "CONTINUING"
	AttendanceFail       Attendance = "ATTENDANCE_FAIL" // devamsız: harf notu F1
)

// Score, öğrencinin bir bileşendeki notudur: puan ya da "girmedi".
type Score struct {
	Points Points
	Absent bool
}

// EntryWindow, bir bileşenin not girişi penceresidir.
type EntryWindow struct {
	Open         bool
	StartsAt     *time.Time
	EndsAt       *time.Time
	NextStartsAt *time.Time
}

// GradebookComponent, not tablosundaki bir bileşendir.
type GradebookComponent struct {
	Component
	Version     int
	PublishedAt *time.Time
	Window      EntryWindow
	Scored      int
	Absent      int
	Average     *Points // girilen puanların ortalaması (girmedi hariç)
}

// GradebookStudent, not tablosundaki bir öğrencidir (kesin kaydı).
type GradebookStudent struct {
	EnrollmentID  string
	StudentID     string
	StudentNo     string
	FirstName     string
	LastName      string
	ProgramCode   string
	ProgramNameTR string
	ProgramNameEN string
	AttemptNo     int
	Attendance    Attendance
	Scores        map[string]Score // bileşen kimliğine göre
}

// Gradebook, şubenin not tablosudur.
type Gradebook struct {
	SectionContext
	Components []GradebookComponent
	Students   []GradebookStudent
}

// Calendar, takvim pencerelerini çözer. *academic.Repository bunu sağlar.
type Calendar interface {
	Window(ctx context.Context, termID, typeCode string, target academic.WindowTarget, at time.Time) (academic.Window, error)
}

// Service, not girişi işlemleridir.
type Service struct {
	repo     *Repository
	pool     *pgxpool.Pool
	calendar Calendar
	now      func() time.Time
}

// NewService, bir Service oluşturur.
func NewService(pool *pgxpool.Pool, calendar Calendar, now func() time.Time) *Service {
	return &Service{repo: NewRepository(pool), pool: pool, calendar: calendar, now: now}
}

// window, bileşen kategorisinin şubenin birimindeki not girişi penceresini çözer: açık olan
// pencere, yoksa en yakın açılacak olan. Transaction dışında çağrılmalıdır.
func (s *Service) window(ctx context.Context, sc SectionContext, c Category) (EntryWindow, error) {
	var res EntryWindow
	for _, code := range entryWindows(c) {
		w, err := s.windowOf(ctx, sc, code)
		if err != nil {
			return EntryWindow{}, err
		}
		if w.Open {
			return w, nil
		}
		if w.NextStartsAt != nil && (res.NextStartsAt == nil || w.NextStartsAt.Before(*res.NextStartsAt)) {
			res.NextStartsAt = w.NextStartsAt
		}
	}
	return res, nil
}

// windowOf, bir takvim penceresinin şubenin birimindeki durumudur. Tanımsız tür kapalı sayılır.
func (s *Service) windowOf(ctx context.Context, sc SectionContext, code string) (EntryWindow, error) {
	w, err := s.calendar.Window(ctx, sc.TermID, code, academic.WindowTarget{FacultyID: sc.FacultyID}, s.now())
	if errors.Is(err, academic.ErrUnknownEventType) {
		return EntryWindow{}, nil
	}
	if err != nil {
		return EntryWindow{}, fmt.Errorf("grading: %s penceresi okunamadı: %w", code, err)
	}
	var res EntryWindow
	if w.Open && w.Current != nil {
		res = EntryWindow{Open: true, StartsAt: &w.Current.StartsAt, EndsAt: &w.Current.EndsAt}
	}
	if !w.Open && w.Next != nil {
		res.NextStartsAt = &w.Next.StartsAt
	}
	return res, nil
}

// Gradebook, şubenin not tablosunu döndürür. viewerID, kullanıcının şubedeki rolünü çözer.
func (s *Service) Gradebook(ctx context.Context, sectionID, viewerID string) (Gradebook, error) {
	sc, err := s.repo.sectionContext(ctx, s.pool, sectionID, viewerID, false)
	if err != nil {
		return Gradebook{}, err
	}
	g := Gradebook{SectionContext: sc}
	if g.Components, err = s.gradebookComponents(ctx, sc); err != nil {
		return Gradebook{}, err
	}
	if g.Students, err = s.gradebookStudents(ctx, sectionID); err != nil {
		return Gradebook{}, err
	}
	return g, nil
}

func (s *Service) gradebookComponents(ctx context.Context, sc SectionContext) ([]GradebookComponent, error) {
	components, err := s.repo.components(ctx, s.pool, sc.SectionID)
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT ac.id, ac.scores_version, ac.published_at,
		       count(sc.id) FILTER (WHERE sc.status = 'SCORED'),
		       count(sc.id) FILTER (WHERE sc.status = 'ABSENT'),
		       coalesce(sum((sc.score * 100)::int8), 0)
		FROM grading.assessment_components ac
		LEFT JOIN (grading.assessment_scores sc
		           JOIN enrollment.enrollments e ON e.id = sc.enrollment_id AND e.status <> 'DROPPED')
		       ON sc.component_id = ac.id
		WHERE ac.section_id = $1
		GROUP BY ac.id`, sc.SectionID)
	if err != nil {
		return nil, fmt.Errorf("grading: bileşen notları okunamadı: %w", err)
	}
	type stat struct {
		version        int
		published      *time.Time
		scored, absent int
		sum            Points
	}
	stats := map[string]stat{}
	for rows.Next() {
		var (
			id  string
			st  stat
			sum int64
		)
		if err := rows.Scan(&id, &st.version, &st.published, &st.scored, &st.absent, &sum); err != nil {
			rows.Close()
			return nil, fmt.Errorf("grading: bileşen notları okunamadı: %w", err)
		}
		st.sum = Points(sum)
		stats[id] = st
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("grading: bileşen notları okunamadı: %w", err)
	}

	windows := map[Category]EntryWindow{}
	out := make([]GradebookComponent, 0, len(components))
	for _, c := range components {
		w, ok := windows[c.Type.Category]
		if !ok {
			if w, err = s.window(ctx, sc, c.Type.Category); err != nil {
				return nil, err
			}
			windows[c.Type.Category] = w
		}
		st := stats[c.ID]
		gc := GradebookComponent{Component: c, Version: st.version, PublishedAt: st.published, Window: w, Scored: st.scored, Absent: st.absent}
		if st.scored > 0 {
			avg := mean(st.sum, st.scored)
			gc.Average = &avg
		}
		out = append(out, gc)
	}
	return out, nil
}

func (s *Service) gradebookStudents(ctx context.Context, sectionID string) ([]GradebookStudent, error) {
	return s.gradebookStudentsQ(ctx, s.pool, sectionID)
}

func (s *Service) gradebookStudentsQ(ctx context.Context, q db.Querier, sectionID string) ([]GradebookStudent, error) {
	rows, err := q.Query(ctx, `
		SELECT e.id, st.id, st.student_no, pe.first_name, pe.last_name, p.code, p.name_tr, p.name_en,
		       e.attempt_no, e.attendance_status
		FROM enrollment.enrollments e
		JOIN enrollment.student_programs sp ON sp.id = e.student_program_id
		JOIN people.students st ON st.id = sp.student_id
		JOIN people.persons pe ON pe.id = st.person_id
		JOIN org.programs p ON p.id = sp.program_id
		WHERE e.section_id = $1 AND e.status <> 'DROPPED'
		ORDER BY pe.last_name COLLATE "tr-x-icu", pe.first_name COLLATE "tr-x-icu", st.student_no`, sectionID)
	if err != nil {
		return nil, fmt.Errorf("grading: şube listesi okunamadı: %w", err)
	}
	students, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (GradebookStudent, error) {
		var (
			st      GradebookStudent
			attempt int16
			att     string
		)
		err := row.Scan(&st.EnrollmentID, &st.StudentID, &st.StudentNo, &st.FirstName, &st.LastName,
			&st.ProgramCode, &st.ProgramNameTR, &st.ProgramNameEN, &attempt, &att)
		st.AttemptNo, st.Attendance, st.Scores = int(attempt), Attendance(att), map[string]Score{}
		return st, err
	})
	if err != nil {
		return nil, fmt.Errorf("grading: şube listesi okunamadı: %w", err)
	}
	index := make(map[string]int, len(students))
	for i, st := range students {
		index[st.EnrollmentID] = i
	}
	rows, err = q.Query(ctx, `
		SELECT sc.enrollment_id, sc.component_id, sc.status, coalesce((sc.score * 100)::int8, 0)
		FROM grading.assessment_scores sc JOIN grading.assessment_components ac ON ac.id = sc.component_id
		WHERE ac.section_id = $1`, sectionID)
	if err != nil {
		return nil, fmt.Errorf("grading: notlar okunamadı: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			enrollmentID, componentID, status string
			points                            int64
		)
		if err := rows.Scan(&enrollmentID, &componentID, &status, &points); err != nil {
			return nil, fmt.Errorf("grading: notlar okunamadı: %w", err)
		}
		if i, ok := index[enrollmentID]; ok {
			students[i].Scores[componentID] = Score{Points: Points(points), Absent: status == "ABSENT"}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("grading: notlar okunamadı: %w", err)
	}
	return students, nil
}

// ComponentRef, bir bileşenin hangi şubeye ait olduğu ve kategorisidir.
type ComponentRef struct {
	Section  SectionContext
	ID       string
	Category Category
}

// ComponentRef, bileşeni ve şubesinin bağlamını (viewerID'nin rolüyle) okur.
func (s *Service) ComponentRef(ctx context.Context, componentID, viewerID string) (ComponentRef, error) {
	var (
		sectionID, cat string
	)
	err := s.pool.QueryRow(ctx, `
		SELECT ac.section_id, t.category FROM grading.assessment_components ac
		JOIN grading.assessment_types t ON t.code = ac.type_code WHERE ac.id = $1`, componentID).Scan(&sectionID, &cat)
	if errors.Is(err, pgx.ErrNoRows) {
		return ComponentRef{}, ErrNotFound
	}
	if err != nil {
		return ComponentRef{}, fmt.Errorf("grading: bileşen okunamadı: %w", err)
	}
	sc, err := s.repo.sectionContext(ctx, s.pool, sectionID, viewerID, false)
	if err != nil {
		return ComponentRef{}, err
	}
	return ComponentRef{Section: sc, ID: componentID, Category: Category(cat)}, nil
}

// ScoreInput, bir öğrencinin bileşen notudur. Points nil ve Absent false ise not silinir.
type ScoreInput struct {
	EnrollmentID string
	Points       *Points
	Absent       bool
}

// SetScores, bileşenin notlarını yazar: sadece verilen öğrencilerin notu değişir. Plan
// kilitli ve bileşenin not girişi penceresi açık olmalıdır; version bileşenin not sürümüdür
// (iyimser kilit). Değişen not yoksa sürüm artmaz: aynı isteğin tekrarı zararsızdır.
// Değişen not sayısını döndürür.
func (s *Service) SetScores(ctx context.Context, actorID string, ref ComponentRef, version int, items []ScoreInput) (int, error) {
	if ref.Section.GradesPublishedAt != nil {
		return 0, ErrGradesPublished
	}
	if ref.Category == CategoryMakeup {
		return 0, ErrMakeupNotAvailable
	}
	if ref.Section.LockedAt == nil {
		return 0, ErrPlanNotLocked
	}
	w, err := s.window(ctx, ref.Section, ref.Category)
	if err != nil {
		return 0, err
	}
	if !w.Open {
		return 0, ErrEntryWindowClosed
	}

	changed := 0
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := lockSection(ctx, tx, ref.Section.SectionID, false); err != nil {
			return err
		}
		var (
			current int
			locked  *time.Time
		)
		err := tx.QueryRow(ctx, `
			SELECT ac.scores_version, s.assessment_plan_locked_at
			FROM grading.assessment_components ac JOIN offering.sections s ON s.id = ac.section_id
			WHERE ac.id = $1 FOR UPDATE OF ac`, ref.ID).Scan(&current, &locked)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("grading: bileşen okunamadı: %w", err)
		}
		if current != version {
			return ErrVersionMismatch
		}
		if locked == nil {
			return ErrPlanNotLocked
		}

		existing, err := s.componentScores(ctx, tx, ref)
		if err != nil {
			return err
		}
		var unknown []string
		for _, it := range items {
			if _, ok := existing[it.EnrollmentID]; !ok {
				unknown = append(unknown, it.EnrollmentID)
			}
		}
		if len(unknown) > 0 {
			return &UnknownEnrollmentsError{IDs: unknown}
		}

		var before, after []map[string]any
		for _, it := range items {
			old := existing[it.EnrollmentID]
			next := scoreState{set: it.Points != nil || it.Absent, score: Score{Absent: it.Absent}}
			if it.Points != nil {
				next.score.Points = *it.Points
			}
			if old == next {
				continue
			}
			changed++
			before = append(before, old.audit(it.EnrollmentID))
			after = append(after, next.audit(it.EnrollmentID))
			if !next.set {
				if _, err := tx.Exec(ctx, `DELETE FROM grading.assessment_scores WHERE component_id = $1 AND enrollment_id = $2`,
					ref.ID, it.EnrollmentID); err != nil {
					return fmt.Errorf("grading: not silinemedi: %w", err)
				}
				continue
			}
			status, score := "SCORED", any(fmt.Sprintf("%d.%02d", next.score.Points/100, next.score.Points%100))
			if next.score.Absent {
				status, score = "ABSENT", nil
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO grading.assessment_scores (component_id, enrollment_id, status, score, entered_by)
				VALUES ($1, $2, $3, $4::numeric, $5)
				ON CONFLICT (component_id, enrollment_id) DO UPDATE
				SET status = excluded.status, score = excluded.score, source = 'MANUAL',
				    entered_by = excluded.entered_by, entered_at = now()`,
				ref.ID, it.EnrollmentID, status, score, nullable(actorID)); err != nil {
				return fmt.Errorf("grading: not yazılamadı: %w", err)
			}
		}
		if changed == 0 {
			return nil
		}
		if _, err := tx.Exec(ctx, `UPDATE grading.assessment_components SET scores_version = scores_version + 1 WHERE id = $1`, ref.ID); err != nil {
			return fmt.Errorf("grading: not sürümü artırılamadı: %w", err)
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "assessment_scores.set", EntityType: "assessment_component", EntityID: ref.ID,
			Before: map[string]any{"scores": before}, After: map[string]any{"scores": after},
		})
	})
	if err != nil {
		return 0, err
	}
	return changed, nil
}

// scoreState, bir öğrencinin bileşendeki not durumudur; set false ise not girilmemiştir.
type scoreState struct {
	set   bool
	score Score
}

func (st scoreState) audit(enrollmentID string) map[string]any {
	m := map[string]any{"enrollment_id": enrollmentID}
	switch {
	case !st.set:
		m["score"] = nil
	case st.score.Absent:
		m["absent"] = true
	default:
		m["score"] = st.score.Points.Float()
	}
	return m
}

// componentScores, şubenin bırakmamış her öğrencisinin bu bileşendeki not durumudur.
func (s *Service) componentScores(ctx context.Context, q db.Querier, ref ComponentRef) (map[string]scoreState, error) {
	rows, err := q.Query(ctx, `
		SELECT e.id, sc.status, coalesce((sc.score * 100)::int8, 0)
		FROM enrollment.enrollments e
		LEFT JOIN grading.assessment_scores sc ON sc.enrollment_id = e.id AND sc.component_id = $2
		WHERE e.section_id = $1 AND e.status <> 'DROPPED'`, ref.Section.SectionID, ref.ID)
	if err != nil {
		return nil, fmt.Errorf("grading: notlar okunamadı: %w", err)
	}
	defer rows.Close()
	out := map[string]scoreState{}
	for rows.Next() {
		var (
			id     string
			status *string
			points int64
		)
		if err := rows.Scan(&id, &status, &points); err != nil {
			return nil, fmt.Errorf("grading: notlar okunamadı: %w", err)
		}
		st := scoreState{}
		if status != nil {
			st = scoreState{set: true, score: Score{Points: Points(points), Absent: *status == "ABSENT"}}
		}
		out[id] = st
	}
	return out, rows.Err()
}

// PublishComponent, bileşenin notlarını öğrencilere yayımlar. Zaten yayımlanmışsa bir şey değişmez.
func (s *Service) PublishComponent(ctx context.Context, actorID string, ref ComponentRef) error {
	if ref.Section.LockedAt == nil {
		return ErrPlanNotLocked
	}
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var (
			published *time.Time
			scores    int
		)
		err := tx.QueryRow(ctx, `
			SELECT ac.published_at, (SELECT count(*) FROM grading.assessment_scores sc WHERE sc.component_id = ac.id)
			FROM grading.assessment_components ac WHERE ac.id = $1 FOR UPDATE`, ref.ID).Scan(&published, &scores)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("grading: bileşen okunamadı: %w", err)
		}
		if published != nil {
			return nil
		}
		if scores == 0 {
			return ErrNoScores
		}
		if _, err := tx.Exec(ctx, `
			UPDATE grading.assessment_components SET published_at = now(), published_by = $2, scores_version = scores_version + 1
			WHERE id = $1`, ref.ID, nullable(actorID)); err != nil {
			return fmt.Errorf("grading: bileşen yayımlanamadı: %w", err)
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "assessment_scores.publish", EntityType: "assessment_component", EntityID: ref.ID,
			After: map[string]any{"scores": scores},
		})
	})
}

// AttendanceInput, bir öğrencinin devam durumudur.
type AttendanceInput struct {
	EnrollmentID string
	Status       Attendance
}

// SetAttendance, öğrencilerin devam durumunu yazar. Değişen kayıt sayısını döndürür.
func (s *Service) SetAttendance(ctx context.Context, actorID string, sc SectionContext, items []AttendanceInput) (int, error) {
	changed := 0
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := lockSection(ctx, tx, sc.SectionID, false); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT id, attendance_status FROM enrollment.enrollments
			WHERE section_id = $1 AND status <> 'DROPPED' FOR UPDATE`, sc.SectionID)
		if err != nil {
			return fmt.Errorf("grading: şube listesi okunamadı: %w", err)
		}
		current := map[string]Attendance{}
		for rows.Next() {
			var id, st string
			if err := rows.Scan(&id, &st); err != nil {
				rows.Close()
				return fmt.Errorf("grading: şube listesi okunamadı: %w", err)
			}
			current[id] = Attendance(st)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("grading: şube listesi okunamadı: %w", err)
		}
		var unknown []string
		for _, it := range items {
			if _, ok := current[it.EnrollmentID]; !ok {
				unknown = append(unknown, it.EnrollmentID)
			}
		}
		if len(unknown) > 0 {
			return &UnknownEnrollmentsError{IDs: unknown}
		}
		var changes []map[string]any
		for _, it := range items {
			if current[it.EnrollmentID] == it.Status {
				continue
			}
			changed++
			changes = append(changes, map[string]any{"enrollment_id": it.EnrollmentID, "from": current[it.EnrollmentID], "to": it.Status})
			if _, err := tx.Exec(ctx, `
				UPDATE enrollment.enrollments SET attendance_status = $2, version = version + 1, updated_at = now() WHERE id = $1`,
				it.EnrollmentID, string(it.Status)); err != nil {
				return fmt.Errorf("grading: devam durumu yazılamadı: %w", err)
			}
		}
		if changed == 0 {
			return nil
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "attendance.set", EntityType: "section", EntityID: sc.SectionID,
			After: map[string]any{"changes": changes},
		})
	})
	if err != nil {
		return 0, err
	}
	return changed, nil
}
