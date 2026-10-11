package registration

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/academic"
	"github.com/CAPELLAX02/agora/backend/internal/enrollment"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
	"github.com/CAPELLAX02/agora/backend/internal/registration/rules"
)

// Metrics, ders seçmenin iş metrikleridir. nil olabilir.
type Metrics interface {
	ItemAdded()
	Submitted()
	SectionFull()
}

// Service, ders seçme iş kurallarını uygular.
type Service struct {
	pool     *pgxpool.Pool
	calendar *academic.Repository
	now      func() time.Time
	metrics  Metrics

	// Seyrek değişen kural girdileri (bkz. ttlCache).
	lines   *ttlCache[[]Line]         // müfredat → satırlar
	catalog *ttlCache[*rules.Catalog] // müfredat → eşdeğerlikler ve havuzlar
	prereqs *ttlCache[map[string][]rules.Prerequisite]
	codes   *ttlCache[map[string]string]
	params  *ttlCache[rules.Params] // gün → yönetmelik parametreleri
}

// cacheTTL, kural girdilerinin önbellekte kalma süresidir.
const cacheTTL = 5 * time.Minute

// NewService, bir Service oluşturur. now, pencere kontrolleri için saattir.
func NewService(pool *pgxpool.Pool, now func() time.Time, metrics Metrics) *Service {
	return &Service{
		pool: pool, calendar: academic.NewRepository(pool), now: now, metrics: metrics,
		lines: newTTLCache[[]Line](cacheTTL), catalog: newTTLCache[*rules.Catalog](cacheTTL),
		prereqs: newTTLCache[map[string][]rules.Prerequisite](cacheTTL),
		codes:   newTTLCache[map[string]string](cacheTTL), params: newTTLCache[rules.Params](cacheTTL),
	}
}

// Pencere türleri.
const (
	WindowRegistration = "COURSE_REGISTRATION"
	WindowApproval     = "ADVISOR_APPROVAL"
)

// Window, kayıt için çözümlenmiş bir işlem penceresidir. Extended, pencere kapalıyken
// kayda tanınan ek sürenin işlemi açık tuttuğunu söyler.
type Window struct {
	academic.Window
	Extended bool
}

// Usable, işlemin şu an yapılabilir olup olmadığını söyler.
func (w Window) Usable() bool {
	return w.Open || w.Extended
}

// calendarWindow, program kaydının dönemdeki penceresini takvimden çözer. Transaction
// dışında çağrılmalıdır: takvim okuması havuzdan ayrı bir bağlantı alır. Transaction
// içinden çağrılsaydı, yük altında havuzun bütün bağlantıları ikinci bağlantıyı bekleyen
// transaction'larca tutulur ve hiçbiri ilerleyemezdi (havuz tükenmesiyle kilitlenme).
func (s *Service) calendarWindow(ctx context.Context, sp enrollment.StudentProgram, termID, typeCode string) (academic.Window, error) {
	w, err := s.calendar.Window(ctx, termID, typeCode,
		academic.WindowTarget{FacultyID: sp.Program.FacultyID, ProgramID: sp.Program.ID}, s.now())
	if err != nil && !errors.Is(err, academic.ErrUnknownEventType) {
		return academic.Window{}, err
	}
	return w, nil
}

// withExtension, takvim penceresine kayda özel ek süreyi ekler.
func (s *Service) withExtension(w academic.Window, ext *time.Time) Window {
	return Window{Window: w, Extended: !w.Open && ext != nil && s.now().Before(*ext)}
}

// window, pencereyi kayda özel ek süreyle birlikte çözer (transaction dışında).
func (s *Service) window(ctx context.Context, sp enrollment.StudentProgram, termID, typeCode string, ext *time.Time) (Window, error) {
	w, err := s.calendarWindow(ctx, sp, termID, typeCode)
	if err != nil {
		return Window{}, err
	}
	return s.withExtension(w, ext), nil
}

// itemTerm, sepet satırının dönemini (transaction öncesi pencere çözümü için) okur.
func (s *Service) itemTerm(ctx context.Context, studentProgramID, itemID string) (string, error) {
	var termID string
	err := s.pool.QueryRow(ctx, `
		SELECT r.term_id FROM enrollment.registration_items i JOIN enrollment.registrations r ON r.id = i.registration_id
		WHERE i.id = $1 AND r.student_program_id = $2 AND i.status = 'ACTIVE'`, itemID, studentProgramID).Scan(&termID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrItemNotFound
	}
	if err != nil {
		return "", fmt.Errorf("registration: sepet satırı okunamadı: %w", err)
	}
	return termID, nil
}

// bundle, bir program kaydının bir dönemdeki ders seçme bağlamıdır: kural girdileri ve
// dönemin ilgili şubeleri.
type bundle struct {
	sp       enrollment.StudentProgram
	termID   string
	lines    []Line
	history  []enrollment.CourseAttempt
	sections map[string][]Section // ders → dönemin açık şubeleri
	input    *rules.Input
}

var holdLabels = map[enrollment.HoldType]string{
	enrollment.HoldTuitionDebt:          "katkı payı borcu",
	enrollment.HoldDisciplineSuspension: "disiplin cezası",
	enrollment.HoldMissingDocument:      "eksik belge",
	enrollment.HoldLibraryDebt:          "kütüphane borcu",
}

var statusLabels = map[enrollment.Status]string{
	enrollment.StatusPrep: "hazırlık sınıfı", enrollment.StatusFrozen: "dondurulmuş",
	enrollment.StatusSuspended: "uzaklaştırılmış", enrollment.StatusGraduated: "mezun",
	enrollment.StatusWithdrawn: "kaydı silinmiş", enrollment.StatusDismissed: "ilişiği kesilmiş",
}

// load, kural girdilerini toplar. basket, sepetteki şubelerdir; dönemin kendi kayıtları
// (onaylanmış sepetin kesin kayıtları) geçmişten çıkarılır: onlar zaten sepettir. Şube
// ayrıntıları (öğretim elemanı, program) sadece alınabilir dersler listesinde gerekir; diğer
// işlemler için hangi derslerin açık olduğunu bilmek yeter.
func (s *Service) load(ctx context.Context, q db.Querier, sp enrollment.StudentProgram, termID string, basket []Item) (*bundle, error) {
	return s.loadWith(ctx, q, sp, termID, basket, false)
}

func (s *Service) loadWith(ctx context.Context, q db.Querier, sp enrollment.StudentProgram, termID string, basket []Item, withSections bool) (*bundle, error) {
	repo, students := NewRepository(q), enrollment.NewRepository(q)
	b := &bundle{sp: sp, termID: termID, sections: map[string][]Section{}}

	history, err := students.CourseHistory(ctx, sp.ID)
	if err != nil {
		return nil, err
	}
	for _, h := range history {
		if h.TermID != termID {
			b.history = append(b.history, h)
		}
	}
	holds, err := students.Holds(ctx, sp.ID)
	if err != nil {
		return nil, err
	}
	day := s.now().Format(time.DateOnly)
	params, err := s.params.get(day, func() (rules.Params, error) { return repo.Params(ctx, s.now()) })
	if err != nil {
		return nil, err
	}

	catalog := rules.NewCatalog(nil, nil)
	if sp.Curriculum != nil {
		id := sp.Curriculum.ID
		if b.lines, err = s.lines.get(id, func() ([]Line, error) { return repo.Lines(ctx, id) }); err != nil {
			return nil, err
		}
		if catalog, err = s.catalog.get(id, func() (*rules.Catalog, error) { return repo.Catalog(ctx, id) }); err != nil {
			return nil, err
		}
	}

	relevant := map[string]bool{}
	for _, l := range b.lines {
		ids := catalog.Group(l.GroupID)
		if !l.IsSlot() {
			ids = catalog.Satisfiers(l.CourseID)
		}
		for _, id := range ids {
			relevant[id] = true
		}
	}
	for _, it := range basket {
		relevant[it.Section.CourseID] = true
	}
	ids := make([]string, 0, len(relevant))
	for id := range relevant {
		ids = append(ids, id)
	}

	offered := map[string]bool{}
	if withSections {
		sections, err := repo.OpenSections(ctx, termID, sp.Program.ID, ids)
		if err != nil {
			return nil, err
		}
		for _, sec := range sections {
			b.sections[sec.CourseID] = append(b.sections[sec.CourseID], sec)
			offered[sec.CourseID] = true
		}
	} else if offered, err = repo.OfferedCourses(ctx, termID, ids); err != nil {
		return nil, err
	}
	prereqs, err := s.prereqs.get("", func() (map[string][]rules.Prerequisite, error) { return repo.AllPrerequisites(ctx) })
	if err != nil {
		return nil, err
	}
	codes, err := s.codes.get("", func() (map[string]string, error) { return repo.AllCourseCodes(ctx) })
	if err != nil {
		return nil, err
	}

	student := rules.Student{
		Active:        sp.Status == enrollment.StatusActive,
		StatusLabel:   statusLabels[sp.Status],
		Semester:      sp.CurrentSemester,
		MaxSemesters:  sp.MaxSemesters(),
		GPA:           sp.GPA,
		HasCurriculum: sp.Curriculum != nil,
	}
	for _, h := range holds {
		if h.BlocksAction(enrollment.BlockRegistration) {
			student.BlockingHolds = append(student.BlockingHolds, holdLabels[h.Type])
		}
	}
	b.input = &rules.Input{
		Student: student, Params: params, Lines: RuleLines(b.lines), Attempts: Attempts(b.history),
		Catalog: catalog, Prereqs: prereqs, Offered: offered, Codes: codes,
	}
	for _, it := range basket {
		b.input.Basket = append(b.input.Basket, ruleItem(it.Section))
	}
	return b, nil
}

// ruleItem, şubenin kural motoru karşılığıdır.
func ruleItem(sec Section) rules.Item {
	it := rules.Item{CourseID: sec.CourseID, CourseCode: sec.CourseCode, SectionCode: sec.Label(), ECTS: sec.ECTS}
	for _, sl := range sec.Slots {
		it.Slots = append(it.Slots, rules.Slot{Day: sl.Day, Start: minutes(sl.Start), End: minutes(sl.End)})
	}
	return it
}

func minutes(hhmm string) int {
	h, m, _ := strings.Cut(hhmm, ":")
	hi, _ := strconv.Atoi(h)
	mi, _ := strconv.Atoi(m)
	return hi*60 + mi
}

func hasError(vs []rules.Violation) bool {
	return slices.ContainsFunc(vs, func(v rules.Violation) bool { return v.Severity == rules.Error })
}

// lockStudent, aynı öğrencinin işlemlerini sıraya koyar: iki sekmeden aynı anda eklenen
// iki ders AKTS sınırını birlikte aşamaz.
func lockStudent(ctx context.Context, tx pgx.Tx, studentProgramID string) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, studentProgramID); err != nil {
		return fmt.Errorf("registration: öğrenci kilidi alınamadı: %w", err)
	}
	return nil
}

// lockedRegistration, kaydın satırını kilitleyerek okur. Kayıt yoksa ok false döner.
func lockedRegistration(ctx context.Context, tx pgx.Tx, studentProgramID, termID string) (reg Registration, ok bool, err error) {
	var status string
	err = tx.QueryRow(ctx, `
		SELECT id, status, version, extension_until, total_ects::float8 FROM enrollment.registrations
		WHERE student_program_id = $1 AND term_id = $2 FOR UPDATE`, studentProgramID, termID,
	).Scan(&reg.ID, &status, &reg.Version, &reg.ExtensionUntil, &reg.TotalECTS)
	if errors.Is(err, pgx.ErrNoRows) {
		return Registration{StudentProgramID: studentProgramID, TermID: termID, Status: StatusNotStarted}, false, nil
	}
	if err != nil {
		return Registration{}, false, fmt.Errorf("registration: kayıt okunamadı: %w", err)
	}
	reg.StudentProgramID, reg.TermID, reg.Status = studentProgramID, termID, Status(status)
	return reg, true, nil
}

// reserve, şubede (ayrılmış kontenjanlıysa öğrencinin programının kontenjanında) bir yer
// ayırır. Koşullu atomik UPDATE: satır kilidi eşzamanlı istekleri sıraya koyar, kilidi
// bekleyen istek koşulu güncel değerle yeniden değerlendirir; kontenjan asla aşılmaz.
// Dönen değer, kontenjanı düşülen programdır (ayrılmış kontenjan yoksa boş).
func reserve(ctx context.Context, tx pgx.Tx, sec Section, programID string) (string, error) {
	tag, err := tx.Exec(ctx, `
		UPDATE offering.sections SET enrolled_count = enrolled_count + 1
		WHERE id = $1 AND enrolled_count < capacity`, sec.ID)
	if err != nil {
		return "", fmt.Errorf("registration: kontenjan ayrılamadı: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return "", ErrSectionFull
	}
	if sec.QuotaMode != "RESERVED" {
		return "", nil
	}
	tag, err = tx.Exec(ctx, `
		UPDATE offering.section_quotas SET enrolled = enrolled + 1
		WHERE section_id = $1 AND program_id = $2 AND enrolled < quota`, sec.ID, programID)
	if err != nil {
		return "", fmt.Errorf("registration: program kontenjanı ayrılamadı: %w", err)
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM offering.section_quotas WHERE section_id = $1 AND program_id = $2)`,
			sec.ID, programID).Scan(&exists); err != nil {
			return "", fmt.Errorf("registration: program kontenjanı okunamadı: %w", err)
		}
		if exists {
			return "", ErrQuotaFull
		}
		return "", ErrNotForProgram
	}
	return programID, nil
}

// release, ayrılan yeri iade eder.
func release(ctx context.Context, tx pgx.Tx, sectionID, quotaProgramID string) error {
	if _, err := tx.Exec(ctx, `
		UPDATE offering.sections SET enrolled_count = enrolled_count - 1 WHERE id = $1 AND enrolled_count > 0`, sectionID); err != nil {
		return fmt.Errorf("registration: kontenjan iade edilemedi: %w", err)
	}
	if quotaProgramID == "" {
		return nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE offering.section_quotas SET enrolled = enrolled - 1
		WHERE section_id = $1 AND program_id = $2 AND enrolled > 0`, sectionID, quotaProgramID); err != nil {
		return fmt.Errorf("registration: program kontenjanı iade edilemedi: %w", err)
	}
	return nil
}

func event(ctx context.Context, tx pgx.Tx, registrationID, typ, actorID string, payload map[string]any) error {
	if payload == nil {
		payload = map[string]any{}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO enrollment.registration_events (registration_id, event_type, actor_user_id, payload)
		VALUES ($1, $2, $3, $4)`, registrationID, typ, nullable(actorID), payload); err != nil {
		return fmt.Errorf("registration: olay yazılamadı: %w", err)
	}
	return nil
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// ensureEditable, sepeti düzenlemeye hazırlar: kayıt yoksa oluşturur, reddedilmişse
// yeniden taslak yapar. Gönderilmiş ya da onaylanmış kayıtta ErrLocked döner.
func ensureEditable(ctx context.Context, tx pgx.Tx, reg *Registration, actorID string) error {
	switch reg.Status {
	case StatusNotStarted:
		err := tx.QueryRow(ctx, `
			INSERT INTO enrollment.registrations (student_program_id, term_id) VALUES ($1, $2) RETURNING id, version`,
			reg.StudentProgramID, reg.TermID).Scan(&reg.ID, &reg.Version)
		if err != nil {
			return fmt.Errorf("registration: kayıt oluşturulamadı: %w", err)
		}
		reg.Status = StatusDraft
		return event(ctx, tx, reg.ID, "CREATED", actorID, nil)
	case StatusRejected:
		if _, err := tx.Exec(ctx, `UPDATE enrollment.registrations SET status = 'DRAFT' WHERE id = $1`, reg.ID); err != nil {
			return fmt.Errorf("registration: kayıt taslağa dönemedi: %w", err)
		}
		reg.Status = StatusDraft
		return event(ctx, tx, reg.ID, "RETURNED_TO_DRAFT", actorID, nil)
	case StatusDraft:
		return nil
	}
	return ErrLocked
}

// AddItem, şubeyi öğrencinin sepetine ekler: ders seçme penceresi açık olmalı, kurallar
// eklemeye izin vermeli ve şubede yer olmalı. Kural hatası *RuleError döner; dönen
// ihlaller (varsa) eklemeye engel olmayan uyarılardır.
func (s *Service) AddItem(ctx context.Context, actorID string, sp enrollment.StudentProgram, sectionID string) ([]rules.Violation, error) {
	pre, err := NewRepository(s.pool).SectionForRegistration(ctx, sectionID, sp.Program.ID)
	if err != nil {
		return nil, err
	}
	cw, err := s.calendarWindow(ctx, sp, pre.TermID, WindowRegistration)
	if err != nil {
		return nil, err
	}
	var warnings []rules.Violation
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := lockStudent(ctx, tx, sp.ID); err != nil {
			return err
		}
		repo := NewRepository(tx)
		sec, err := repo.SectionForRegistration(ctx, sectionID, sp.Program.ID)
		if err != nil {
			return err
		}
		if sec.OfferingStatus != "OPEN" || sec.SectionStatus != "ACTIVE" {
			return ErrSectionNotOpen
		}
		reg, _, err := lockedRegistration(ctx, tx, sp.ID, sec.TermID)
		if err != nil {
			return err
		}
		if !reg.Status.Editable() {
			return ErrLocked
		}
		if !s.withExtension(cw, reg.ExtensionUntil).Usable() {
			return ErrWindowClosed
		}

		basket := []Item{}
		if reg.ID != "" {
			if basket, err = repo.items(ctx, reg.ID, sp.Program.ID); err != nil {
				return err
			}
		}
		b, err := s.load(ctx, tx, sp, sec.TermID, basket)
		if err != nil {
			return err
		}
		vs := rules.CheckAdd(b.input, ruleItem(sec.Section))
		if hasError(vs) {
			return &RuleError{Violations: vs}
		}
		warnings = vs
		line, kind, _ := rules.Classify(b.input, sec.CourseID)

		quotaProgram, err := reserve(ctx, tx, sec.Section, sp.Program.ID)
		if errors.Is(err, ErrSectionFull) && s.metrics != nil {
			s.metrics.SectionFull()
		}
		if err != nil {
			return err
		}
		if err := ensureEditable(ctx, tx, &reg, actorID); err != nil {
			return err
		}
		var itemID string
		err = tx.QueryRow(ctx, `
			INSERT INTO enrollment.registration_items
				(registration_id, section_id, course_id, item_kind, curriculum_item_id, ects, quota_program_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
			reg.ID, sec.ID, sec.CourseID, string(kind), nullable(line.ID), sec.ECTS, nullable(quotaProgram)).Scan(&itemID)
		if db.IsConflict(err) {
			return &RuleError{Violations: []rules.Violation{{Code: rules.CodeDuplicateCourse, Severity: rules.Error,
				Message: sec.CourseCode + " dersinin bir şubesi zaten sepetinizde."}}}
		}
		if err != nil {
			return fmt.Errorf("registration: ders sepete eklenemedi: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE enrollment.registrations SET total_ects = total_ects + $2, version = version + 1, updated_at = now()
			WHERE id = $1`, reg.ID, sec.ECTS); err != nil {
			return fmt.Errorf("registration: kayıt güncellenemedi: %w", err)
		}
		return event(ctx, tx, reg.ID, "ITEM_ADDED", actorID, map[string]any{
			"item_id": itemID, "section_id": sec.ID, "section": sec.Label(), "kind": kind,
		})
	})
	if err == nil && s.metrics != nil {
		s.metrics.ItemAdded()
	}
	return warnings, err
}

// lockedItem, sepet satırını kaydıyla birlikte kilitler ve öğrenciye ait olduğunu doğrular.
func lockedItem(ctx context.Context, tx pgx.Tx, studentProgramID, itemID string) (reg Registration, sectionID, quotaProgram string, ects float64, err error) {
	var (
		status string
		quota  *string
	)
	err = tx.QueryRow(ctx, `
		SELECT r.id, r.term_id, r.status, r.version, r.extension_until, i.section_id, i.quota_program_id, i.ects::float8
		FROM enrollment.registration_items i
		JOIN enrollment.registrations r ON r.id = i.registration_id
		WHERE i.id = $1 AND r.student_program_id = $2 AND i.status = 'ACTIVE'
		FOR UPDATE OF r, i`, itemID, studentProgramID,
	).Scan(&reg.ID, &reg.TermID, &status, &reg.Version, &reg.ExtensionUntil, &sectionID, &quota, &ects)
	if errors.Is(err, pgx.ErrNoRows) {
		return Registration{}, "", "", 0, ErrItemNotFound
	}
	if err != nil {
		return Registration{}, "", "", 0, fmt.Errorf("registration: sepet satırı okunamadı: %w", err)
	}
	reg.StudentProgramID, reg.Status = studentProgramID, Status(status)
	if quota != nil {
		quotaProgram = *quota
	}
	return reg, sectionID, quotaProgram, ects, nil
}

// RemoveItem, dersi sepetten çıkarır ve ayrılan yeri iade eder.
func (s *Service) RemoveItem(ctx context.Context, actorID string, sp enrollment.StudentProgram, itemID string) error {
	termID, err := s.itemTerm(ctx, sp.ID, itemID)
	if err != nil {
		return err
	}
	cw, err := s.calendarWindow(ctx, sp, termID, WindowRegistration)
	if err != nil {
		return err
	}
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := lockStudent(ctx, tx, sp.ID); err != nil {
			return err
		}
		reg, sectionID, quotaProgram, ects, err := lockedItem(ctx, tx, sp.ID, itemID)
		if err != nil {
			return err
		}
		if !reg.Status.Editable() {
			return ErrLocked
		}
		if !s.withExtension(cw, reg.ExtensionUntil).Usable() {
			return ErrWindowClosed
		}
		if err := ensureEditable(ctx, tx, &reg, actorID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM enrollment.registration_items WHERE id = $1`, itemID); err != nil {
			return fmt.Errorf("registration: ders sepetten çıkarılamadı: %w", err)
		}
		if err := release(ctx, tx, sectionID, quotaProgram); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE enrollment.registrations SET total_ects = greatest(total_ects - $2, 0), version = version + 1, updated_at = now()
			WHERE id = $1`, reg.ID, ects); err != nil {
			return fmt.Errorf("registration: kayıt güncellenemedi: %w", err)
		}
		return event(ctx, tx, reg.ID, "ITEM_REMOVED", actorID, map[string]any{"item_id": itemID, "section_id": sectionID})
	})
}

// ChangeSection, sepetteki dersin şubesini değiştirir: yeni şubede yer ayrılır, eskisi
// iade edilir. Yeni şubenin programı sepetin geri kalanıyla yeniden denetlenir.
func (s *Service) ChangeSection(ctx context.Context, actorID string, sp enrollment.StudentProgram, itemID, sectionID string) ([]rules.Violation, error) {
	termID, err := s.itemTerm(ctx, sp.ID, itemID)
	if err != nil {
		return nil, err
	}
	cw, err := s.calendarWindow(ctx, sp, termID, WindowRegistration)
	if err != nil {
		return nil, err
	}
	var warnings []rules.Violation
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := lockStudent(ctx, tx, sp.ID); err != nil {
			return err
		}
		reg, oldSection, oldQuota, _, err := lockedItem(ctx, tx, sp.ID, itemID)
		if err != nil {
			return err
		}
		if !reg.Status.Editable() {
			return ErrLocked
		}
		if oldSection == sectionID {
			return ErrSameSection
		}
		repo := NewRepository(tx)
		sec, err := repo.SectionForRegistration(ctx, sectionID, sp.Program.ID)
		if err != nil {
			return err
		}
		if sec.OfferingStatus != "OPEN" || sec.SectionStatus != "ACTIVE" || sec.TermID != reg.TermID {
			return ErrSectionNotOpen
		}
		if !s.withExtension(cw, reg.ExtensionUntil).Usable() {
			return ErrWindowClosed
		}

		all, err := repo.items(ctx, reg.ID, sp.Program.ID)
		if err != nil {
			return err
		}
		var rest []Item
		for _, it := range all {
			if it.ID == itemID {
				if it.Section.CourseID != sec.CourseID {
					return ErrCourseMismatch
				}
				continue
			}
			rest = append(rest, it)
		}
		b, err := s.load(ctx, tx, sp, reg.TermID, rest)
		if err != nil {
			return err
		}
		vs := rules.CheckAdd(b.input, ruleItem(sec.Section))
		if hasError(vs) {
			return &RuleError{Violations: vs}
		}
		warnings = vs

		quotaProgram, err := reserve(ctx, tx, sec.Section, sp.Program.ID)
		if errors.Is(err, ErrSectionFull) && s.metrics != nil {
			s.metrics.SectionFull()
		}
		if err != nil {
			return err
		}
		if err := release(ctx, tx, oldSection, oldQuota); err != nil {
			return err
		}
		if err := ensureEditable(ctx, tx, &reg, actorID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE enrollment.registration_items SET section_id = $2, quota_program_id = $3 WHERE id = $1`,
			itemID, sec.ID, nullable(quotaProgram)); err != nil {
			return fmt.Errorf("registration: şube değiştirilemedi: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE enrollment.registrations SET version = version + 1, updated_at = now() WHERE id = $1`, reg.ID); err != nil {
			return fmt.Errorf("registration: kayıt güncellenemedi: %w", err)
		}
		return event(ctx, tx, reg.ID, "SECTION_CHANGED", actorID, map[string]any{
			"item_id": itemID, "from_section_id": oldSection, "section_id": sec.ID, "section": sec.Label(),
		})
	})
	return warnings, err
}

// View, kaydın kurallara göre değerlendirilmiş halidir.
type View struct {
	Registration
	Evaluation     rules.Evaluation
	GPA            *float64
	RegWindow      Window
	ApprovalWindow Window
}

// Evaluate, program kaydının dönemdeki ders seçimini kurallara göre değerlendirir.
func (s *Service) Evaluate(ctx context.Context, sp enrollment.StudentProgram, termID string) (View, error) {
	reg, err := NewRepository(s.pool).Registration(ctx, sp.ID, sp.Program.ID, termID)
	if err != nil {
		return View{}, err
	}
	return s.view(ctx, sp, reg)
}

// EvaluateByID, kaydı kimliğiyle okuyup değerlendirir (danışman incelemesi).
func (s *Service) EvaluateByID(ctx context.Context, sp enrollment.StudentProgram, registrationID string) (View, error) {
	reg, err := NewRepository(s.pool).RegistrationByID(ctx, registrationID, sp.Program.ID)
	if err != nil {
		return View{}, err
	}
	return s.view(ctx, sp, reg)
}

func (s *Service) view(ctx context.Context, sp enrollment.StudentProgram, reg Registration) (View, error) {
	b, err := s.load(ctx, s.pool, sp, reg.TermID, reg.Items)
	if err != nil {
		return View{}, err
	}
	v := View{Registration: reg, Evaluation: rules.Evaluate(b.input), GPA: sp.GPA}
	if v.RegWindow, err = s.window(ctx, sp, reg.TermID, WindowRegistration, reg.ExtensionUntil); err != nil {
		return View{}, err
	}
	if v.ApprovalWindow, err = s.window(ctx, sp, reg.TermID, WindowApproval, reg.ExtensionUntil); err != nil {
		return View{}, err
	}
	return v, nil
}
