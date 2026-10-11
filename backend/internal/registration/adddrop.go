package registration

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/enrollment"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
	"github.com/CAPELLAX02/agora/backend/internal/platform/outbox"
	"github.com/CAPELLAX02/agora/backend/internal/registration/rules"
)

// Ekle-bırak pencereleri.
const (
	WindowAddDropStudent = "ADD_DROP_STUDENT"
	WindowAddDropAdvisor = "ADD_DROP_ADVISOR"
	WindowAddDropHead    = "ADD_DROP_DEPT_HEAD"
)

// Ekle-bırak hataları.
var (
	ErrAddDropOpen      = errors.New("registration: açık bir ekle-bırak talebi var")
	ErrAddDropEmpty     = errors.New("registration: talepte ekleme ya da bırakma yok")
	ErrAddDropNotFound  = errors.New("registration: ekle-bırak talebi bulunamadı")
	ErrNotApproved      = errors.New("registration: ekle-bırak sadece onaylanmış kayıtta yapılır")
	ErrDropNotInBasket  = errors.New("registration: bırakılacak ders kayıtta yok")
	ErrAddAlreadyTaken  = errors.New("registration: eklenecek ders zaten kayıtta")
	ErrAddDropForbidden = errors.New("registration: bu talep için karar yetkiniz yok")
)

// AddDropStatus, ekle-bırak talebinin durumudur.
type AddDropStatus string

// Talep durumları.
const (
	AddDropPendingAdvisor AddDropStatus = "PENDING_ADVISOR"
	AddDropPendingHead    AddDropStatus = "PENDING_DEPT_HEAD"
	AddDropApproved       AddDropStatus = "APPROVED"
	AddDropRejected       AddDropStatus = "REJECTED"
	AddDropWithdrawn      AddDropStatus = "WITHDRAWN"
)

// Open, talebin karar beklediğini söyler.
func (s AddDropStatus) Open() bool {
	return s == AddDropPendingAdvisor || s == AddDropPendingHead
}

// AddDropItem, talepteki bir ekleme ya da bırakmadır.
type AddDropItem struct {
	ID                 string
	Action             string // ADD, DROP
	Section            Section
	ECTS               float64
	RegistrationItemID string // DROP: bırakılan sepet satırı
	QuotaProgramID     string // ADD: düşülen program kontenjanı
}

// AddDropRequest, bir ekle-bırak talebidir.
type AddDropRequest struct {
	ID               string
	RegistrationID   string
	StudentProgramID string
	TermID           string
	Status           AddDropStatus
	StudentNote      string
	AdvisorBy        string
	AdvisorAt        *time.Time
	AdvisorNote      string
	HeadBy           string
	HeadAt           *time.Time
	HeadNote         string
	Items            []AddDropItem
	Version          int
	CreatedAt        time.Time
}

// --- Okuma --------------------------------------------------------------------------------

const addDropSelect = `
	SELECT a.id, a.registration_id, r.student_program_id, r.term_id, a.status, coalesce(a.student_note, ''),
	       coalesce(ap.first_name || ' ' || ap.last_name, ''), a.advisor_decided_at, coalesce(a.advisor_note, ''),
	       coalesce(hp.first_name || ' ' || hp.last_name, ''), a.head_decided_at, coalesce(a.head_note, ''),
	       a.version, a.created_at
	FROM enrollment.add_drop_requests a
	JOIN enrollment.registrations r ON r.id = a.registration_id
	LEFT JOIN iam.users au ON au.id = a.advisor_decided_by
	LEFT JOIN people.persons ap ON ap.id = au.person_id
	LEFT JOIN iam.users hu ON hu.id = a.head_decided_by
	LEFT JOIN people.persons hp ON hp.id = hu.person_id`

func scanAddDrop(row pgx.CollectableRow) (AddDropRequest, error) {
	var (
		a      AddDropRequest
		status string
	)
	err := row.Scan(&a.ID, &a.RegistrationID, &a.StudentProgramID, &a.TermID, &status, &a.StudentNote,
		&a.AdvisorBy, &a.AdvisorAt, &a.AdvisorNote, &a.HeadBy, &a.HeadAt, &a.HeadNote, &a.Version, &a.CreatedAt)
	a.Status = AddDropStatus(status)
	return a, err
}

// AddDropRequests, kaydın ekle-bırak taleplerini en yeniden eskiye, kalemleriyle döndürür.
func (r *Repository) AddDropRequests(ctx context.Context, registrationID, programID string) ([]AddDropRequest, error) {
	rows, err := r.db.Query(ctx, addDropSelect+` WHERE a.registration_id = $1 ORDER BY a.created_at DESC, a.id DESC`, registrationID)
	if err != nil {
		return nil, fmt.Errorf("registration: ekle-bırak talepleri okunamadı: %w", err)
	}
	list, err := pgx.CollectRows(rows, scanAddDrop)
	if err != nil {
		return nil, fmt.Errorf("registration: ekle-bırak talepleri okunamadı: %w", err)
	}
	for i := range list {
		if list[i].Items, err = r.addDropItems(ctx, list[i].ID, programID); err != nil {
			return nil, err
		}
	}
	return list, nil
}

// AddDropRequest, talebi kalemleriyle döndürür. Yoksa ErrAddDropNotFound döner.
func (r *Repository) AddDropRequest(ctx context.Context, id, programID string) (AddDropRequest, error) {
	rows, err := r.db.Query(ctx, addDropSelect+` WHERE a.id = $1`, id)
	if err != nil {
		return AddDropRequest{}, fmt.Errorf("registration: ekle-bırak talebi okunamadı: %w", err)
	}
	a, err := pgx.CollectExactlyOneRow(rows, scanAddDrop)
	if errors.Is(err, pgx.ErrNoRows) {
		return AddDropRequest{}, ErrAddDropNotFound
	}
	if err != nil {
		return AddDropRequest{}, fmt.Errorf("registration: ekle-bırak talebi okunamadı: %w", err)
	}
	a.Items, err = r.addDropItems(ctx, a.ID, programID)
	return a, err
}

func (r *Repository) addDropItems(ctx context.Context, requestID, programID string) ([]AddDropItem, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, action, section_id, ects::float8, coalesce(registration_item_id::text, ''), coalesce(quota_program_id::text, '')
		FROM enrollment.add_drop_items WHERE request_id = $1 ORDER BY action DESC, id`, requestID)
	if err != nil {
		return nil, fmt.Errorf("registration: talep kalemleri okunamadı: %w", err)
	}
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (AddDropItem, error) {
		var it AddDropItem
		return it, row.Scan(&it.ID, &it.Action, &it.Section.ID, &it.ECTS, &it.RegistrationItemID, &it.QuotaProgramID)
	})
	if err != nil {
		return nil, fmt.Errorf("registration: talep kalemleri okunamadı: %w", err)
	}
	if len(items) == 0 {
		return []AddDropItem{}, nil
	}
	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = it.Section.ID
	}
	sections, err := r.sectionsByID(ctx, programID, ids)
	if err != nil {
		return nil, err
	}
	for i := range items {
		for _, s := range sections {
			if s.ID == items[i].Section.ID {
				items[i].Section = s
			}
		}
	}
	return items, nil
}

// --- Yazma --------------------------------------------------------------------------------

// AddDropInput, öğrencinin ekle-bırak talebidir: eklenecek şubeler ve bırakılacak sepet satırları.
type AddDropInput struct {
	Adds  []string
	Drops []string
	Note  string
}

// RequestAddDrop, onaylanmış kayıtta ekle-bırak talebi açar. Ekle-bırak (öğrenci) penceresi
// açık olmalı, kayıtta açık başka talep olmamalı. Talep sonrası oluşacak ders listesi
// bütün kurallarla denetlenir (1. sınıf ve alttan dersleri bırakmak, çakışma, AKTS sınırı...).
// Eklenecek şubelerin kontenjanı hemen ayrılır.
func (s *Service) RequestAddDrop(ctx context.Context, actorID string, sp enrollment.StudentProgram, termID string, in AddDropInput) (string, error) {
	if len(in.Adds)+len(in.Drops) == 0 {
		return "", ErrAddDropEmpty
	}
	cw, err := s.calendarWindow(ctx, sp, termID, WindowAddDropStudent)
	if err != nil {
		return "", err
	}
	var id string
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := lockStudent(ctx, tx, sp.ID); err != nil {
			return err
		}
		reg, ok, err := lockedRegistration(ctx, tx, sp.ID, termID)
		if err != nil {
			return err
		}
		if !ok || reg.Status != StatusApproved {
			return ErrNotApproved
		}
		if !s.withExtension(cw, reg.ExtensionUntil).Usable() {
			return ErrWindowClosed
		}
		var open bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM enrollment.add_drop_requests
			WHERE registration_id = $1 AND status IN ('PENDING_ADVISOR', 'PENDING_DEPT_HEAD'))`, reg.ID).Scan(&open); err != nil {
			return fmt.Errorf("registration: açık talep denetlenemedi: %w", err)
		}
		if open {
			return ErrAddDropOpen
		}

		repo := NewRepository(tx)
		current, err := repo.items(ctx, reg.ID, sp.Program.ID)
		if err != nil {
			return err
		}
		var drops []Item
		for _, itemID := range in.Drops {
			i := slices.IndexFunc(current, func(it Item) bool { return it.ID == itemID })
			if i < 0 {
				return ErrDropNotInBasket
			}
			drops = append(drops, current[i])
		}
		result := slices.DeleteFunc(slices.Clone(current), func(it Item) bool { return slices.Contains(in.Drops, it.ID) })
		var adds []Section
		for _, sectionID := range in.Adds {
			sec, err := repo.SectionForRegistration(ctx, sectionID, sp.Program.ID)
			if err != nil {
				return err
			}
			if sec.OfferingStatus != "OPEN" || sec.SectionStatus != "ACTIVE" || sec.TermID != termID {
				return ErrSectionNotOpen
			}
			if slices.ContainsFunc(result, func(it Item) bool { return it.Section.CourseID == sec.CourseID }) {
				return ErrAddAlreadyTaken
			}
			adds = append(adds, sec.Section)
			result = append(result, Item{Section: sec.Section, ECTS: sec.ECTS})
		}

		b, err := s.load(ctx, tx, sp, termID, result)
		if err != nil {
			return err
		}
		if ev := rules.Evaluate(b.input); ev.Blocking() {
			return &RuleError{Violations: ev.Violations}
		}

		if err := tx.QueryRow(ctx, `
			INSERT INTO enrollment.add_drop_requests (registration_id, student_note) VALUES ($1, $2) RETURNING id`,
			reg.ID, nullable(in.Note)).Scan(&id); err != nil {
			return fmt.Errorf("registration: ekle-bırak talebi açılamadı: %w", err)
		}
		for _, sec := range adds {
			quota, err := reserve(ctx, tx, sec, sp.Program.ID)
			if errors.Is(err, ErrSectionFull) && s.metrics != nil {
				s.metrics.SectionFull()
			}
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO enrollment.add_drop_items (request_id, action, section_id, course_id, ects, quota_program_id)
				VALUES ($1, 'ADD', $2, $3, $4, $5)`, id, sec.ID, sec.CourseID, sec.ECTS, nullable(quota)); err != nil {
				return fmt.Errorf("registration: talep kalemi yazılamadı: %w", err)
			}
		}
		for _, it := range drops {
			if _, err := tx.Exec(ctx, `
				INSERT INTO enrollment.add_drop_items (request_id, action, section_id, course_id, ects, registration_item_id)
				VALUES ($1, 'DROP', $2, $3, $4, $5)`, id, it.Section.ID, it.Section.CourseID, it.ECTS, it.ID); err != nil {
				return fmt.Errorf("registration: talep kalemi yazılamadı: %w", err)
			}
		}
		if err := event(ctx, tx, reg.ID, "ADD_DROP_REQUESTED", actorID, map[string]any{
			"request_id": id, "adds": len(adds), "drops": len(drops),
		}); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "add_drop.request", EntityType: "add_drop_request", EntityID: id,
			After: map[string]any{"registration_id": reg.ID, "adds": in.Adds, "drops": in.Drops},
		})
	})
	return id, err
}

// lockedAddDrop, talebi kilitleyerek okur ve öğrenciye ait olduğunu doğrular.
func lockedAddDrop(ctx context.Context, tx pgx.Tx, studentProgramID, requestID string) (a AddDropRequest, err error) {
	var status string
	err = tx.QueryRow(ctx, `
		SELECT a.id, a.registration_id, r.term_id, a.status, a.version FROM enrollment.add_drop_requests a
		JOIN enrollment.registrations r ON r.id = a.registration_id
		WHERE a.id = $1 AND r.student_program_id = $2 FOR UPDATE OF a`, requestID, studentProgramID,
	).Scan(&a.ID, &a.RegistrationID, &a.TermID, &status, &a.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return AddDropRequest{}, ErrAddDropNotFound
	}
	if err != nil {
		return AddDropRequest{}, fmt.Errorf("registration: ekle-bırak talebi okunamadı: %w", err)
	}
	a.StudentProgramID, a.Status = studentProgramID, AddDropStatus(status)
	return a, nil
}

// releaseAdds, talebin eklemeleri için ayrılan kontenjanı iade eder.
func releaseAdds(ctx context.Context, tx pgx.Tx, requestID string) error {
	rows, err := tx.Query(ctx, `
		SELECT section_id, coalesce(quota_program_id::text, '') FROM enrollment.add_drop_items
		WHERE request_id = $1 AND action = 'ADD'`, requestID)
	if err != nil {
		return fmt.Errorf("registration: talep kalemleri okunamadı: %w", err)
	}
	type held struct{ Section, Quota string }
	list, err := pgx.CollectRows(rows, pgx.RowToStructByPos[held])
	if err != nil {
		return fmt.Errorf("registration: talep kalemleri okunamadı: %w", err)
	}
	for _, h := range list {
		if err := release(ctx, tx, h.Section, h.Quota); err != nil {
			return err
		}
	}
	return nil
}

// WithdrawAddDrop, öğrencinin karar bekleyen talebini geri çekmesidir; ayrılan kontenjan
// iade edilir.
func (s *Service) WithdrawAddDrop(ctx context.Context, actorID string, sp enrollment.StudentProgram, requestID string, version int) error {
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := lockStudent(ctx, tx, sp.ID); err != nil {
			return err
		}
		a, err := lockedAddDrop(ctx, tx, sp.ID, requestID)
		if err != nil {
			return err
		}
		if a.Version != version {
			return ErrVersionMismatch
		}
		if !a.Status.Open() {
			return ErrInvalidState
		}
		if err := releaseAdds(ctx, tx, requestID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE enrollment.add_drop_requests SET status = 'WITHDRAWN', version = version + 1, updated_at = now()
			WHERE id = $1`, requestID); err != nil {
			return fmt.Errorf("registration: talep geri çekilemedi: %w", err)
		}
		return event(ctx, tx, a.RegistrationID, "ADD_DROP_WITHDRAWN", actorID, map[string]any{"request_id": requestID})
	})
}

// AddDropStage, talebin karar aşamasıdır: önce danışman, sonra bölüm başkanı.
func AddDropStage(status AddDropStatus) string {
	if status == AddDropPendingHead {
		return "DEPT_HEAD"
	}
	return "ADVISOR"
}

// DecideAddDrop, talebin bulunduğu aşamadaki kararı uygular. Danışman onayı talebi bölüm
// başkanına iletir; bölüm başkanının onayı talebi uygular: bırakılan dersler kayıttan ve
// kesin kayıtlardan düşer (yerleri açılır), eklenen dersler kayda ve kesin kayıtlara girer.
// Red, ayrılan kontenjanı iade eder; gerekçe zorunludur. Aşamanın penceresi kapalıysa ancak
// öğrenci işleri (Override) gerekçeyle karar verebilir.
func (s *Service) DecideAddDrop(ctx context.Context, d Decider, sp enrollment.StudentProgram, requestID string, version int, approve bool, note string) error {
	if (!approve || d.Override) && note == "" {
		return ErrReasonRequired
	}
	pre, err := NewRepository(s.pool).AddDropRequest(ctx, requestID, sp.Program.ID)
	if err != nil {
		return err
	}
	window := WindowAddDropAdvisor
	if pre.Status == AddDropPendingHead {
		window = WindowAddDropHead
	}
	cw, err := s.calendarWindow(ctx, sp, pre.TermID, window)
	if err != nil {
		return err
	}
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := lockStudent(ctx, tx, sp.ID); err != nil {
			return err
		}
		a, err := lockedAddDrop(ctx, tx, sp.ID, requestID)
		if err != nil {
			return err
		}
		if a.Version != version {
			return ErrVersionMismatch
		}
		if a.Status != pre.Status || !a.Status.Open() {
			return ErrInvalidState
		}
		if !d.Override && !cw.Open {
			return ErrWindowClosed
		}
		stage := AddDropStage(a.Status)
		decided := "advisor_decided_by = $2, advisor_decided_at = now(), advisor_note = $3"
		if stage == "DEPT_HEAD" {
			decided = "head_decided_by = $2, head_decided_at = now(), head_note = $3"
		}

		next, eventType := AddDropRejected, "ADD_DROP_REJECTED"
		switch {
		case !approve:
			if err := releaseAdds(ctx, tx, requestID); err != nil {
				return err
			}
		case stage == "ADVISOR":
			next, eventType = AddDropPendingHead, "ADD_DROP_APPROVED"
		default:
			if err := s.applyAddDrop(ctx, tx, sp, a); err != nil {
				return err
			}
			next, eventType = AddDropApproved, "ADD_DROP_APPLIED"
		}
		if _, err := tx.Exec(ctx, `
			UPDATE enrollment.add_drop_requests SET status = $4, `+decided+`, version = version + 1, updated_at = now()
			WHERE id = $1`, requestID, nullable(d.UserID), nullable(note), string(next)); err != nil {
			return fmt.Errorf("registration: karar kaydedilemedi: %w", err)
		}
		payload := map[string]any{"request_id": requestID, "stage": stage, "note": note}
		if d.Override {
			payload["override"] = true
		}
		if err := event(ctx, tx, a.RegistrationID, eventType, d.UserID, payload); err != nil {
			return err
		}
		if next == AddDropApproved {
			if err := outbox.Publish(ctx, tx, "registration.add_drop_applied", "registration", a.RegistrationID, map[string]any{
				"registration_id": a.RegistrationID, "request_id": requestID, "student_program_id": sp.ID, "term_id": a.TermID,
			}); err != nil {
				return err
			}
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: d.UserID, Action: "add_drop.decide", EntityType: "add_drop_request", EntityID: requestID,
			Before: map[string]any{"status": a.Status},
			After:  map[string]any{"status": next, "stage": stage, "note": note, "override": d.Override},
		})
	})
}

// applyAddDrop, onaylanan talebi kayda uygular. Sonuç ders listesi yeniden denetlenir:
// talepten sonra engel konmuş ya da bir şube kapanmış olabilir.
func (s *Service) applyAddDrop(ctx context.Context, tx pgx.Tx, sp enrollment.StudentProgram, a AddDropRequest) error {
	repo := NewRepository(tx)
	items, err := repo.addDropItems(ctx, a.ID, sp.Program.ID)
	if err != nil {
		return err
	}
	current, err := repo.items(ctx, a.RegistrationID, sp.Program.ID)
	if err != nil {
		return err
	}
	result := current
	for _, it := range items {
		if it.Action == "DROP" {
			result = slices.DeleteFunc(result, func(c Item) bool { return c.ID == it.RegistrationItemID })
		}
	}
	for _, it := range items {
		if it.Action == "ADD" {
			result = append(result, Item{Section: it.Section, ECTS: it.ECTS})
		}
	}
	b, err := s.load(ctx, tx, sp, a.TermID, result)
	if err != nil {
		return err
	}
	if ev := rules.Evaluate(b.input); ev.Blocking() {
		return &RuleError{Violations: ev.Violations}
	}

	for _, it := range items {
		if it.Action != "DROP" {
			continue
		}
		var quota *string
		if err := tx.QueryRow(ctx, `
			UPDATE enrollment.registration_items SET status = 'DROPPED', dropped_at = now()
			WHERE id = $1 AND status = 'ACTIVE' RETURNING quota_program_id::text`, it.RegistrationItemID).Scan(&quota); err != nil {
			return fmt.Errorf("registration: ders bırakılamadı: %w", err)
		}
		q := ""
		if quota != nil {
			q = *quota
		}
		if err := release(ctx, tx, it.Section.ID, q); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE enrollment.enrollments SET status = 'DROPPED', version = version + 1, updated_at = now()
			WHERE registration_item_id = $1 AND status = 'ENROLLED'`, it.RegistrationItemID); err != nil {
			return fmt.Errorf("registration: kesin kayıt bırakılamadı: %w", err)
		}
	}
	var added []Item
	for _, it := range items {
		if it.Action != "ADD" {
			continue
		}
		line, kind, _ := rules.Classify(b.input, it.Section.CourseID)
		var itemID string
		if err := tx.QueryRow(ctx, `
			INSERT INTO enrollment.registration_items
				(registration_id, section_id, course_id, item_kind, curriculum_item_id, ects, quota_program_id, source)
			VALUES ($1, $2, $3, $4, $5, $6, $7, 'ADD_DROP') RETURNING id`,
			a.RegistrationID, it.Section.ID, it.Section.CourseID, string(kind), nullable(line.ID), it.ECTS,
			nullable(it.QuotaProgramID)).Scan(&itemID); err != nil {
			return fmt.Errorf("registration: ders eklenemedi: %w", err)
		}
		added = append(added, Item{ID: itemID, Section: it.Section, ECTS: it.ECTS})
	}
	if err := createEnrollments(ctx, tx, sp, a.TermID, added, b, "ADD_DROP"); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		UPDATE enrollment.registrations SET version = version + 1, updated_at = now(),
			total_ects = (SELECT coalesce(sum(ects), 0) FROM enrollment.registration_items WHERE registration_id = $1 AND status = 'ACTIVE')
		WHERE id = $1`, a.RegistrationID)
	if err != nil {
		return fmt.Errorf("registration: kayıt güncellenemedi: %w", err)
	}
	return nil
}
