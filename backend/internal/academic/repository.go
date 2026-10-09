package academic

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// Repository, akademik takvim verisine erişir.
type Repository struct {
	pool *pgxpool.Pool
	db   db.Querier
}

// NewRepository, bir Repository oluşturur.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, db: pool}
}

// --- Yıllar ve dönemler --------------------------------------------------------

const termSelect = `
	SELECT t.id, t.academic_year_id, y.start_year, t.term_type, t.code, t.starts_on, t.ends_on,
	       t.status, t.is_current, t.version
	FROM academic.terms t
	JOIN academic.academic_years y ON y.id = t.academic_year_id`

type scanner interface{ Scan(dest ...any) error }

func scanTerm(s scanner) (Term, error) {
	var (
		t           Term
		typ, status string
	)
	err := s.Scan(&t.ID, &t.AcademicYearID, &t.StartYear, &typ, &t.Code, &t.StartsOn, &t.EndsOn, &status, &t.IsCurrent, &t.Version)
	t.Type, t.Status = TermType(typ), TermStatus(status)
	return t, err
}

// AcademicYears, yılları en yeniden eskiye döndürür.
func (r *Repository) AcademicYears(ctx context.Context) ([]AcademicYear, error) {
	rows, err := r.db.Query(ctx, `SELECT id, start_year, starts_on, ends_on FROM academic.academic_years ORDER BY start_year DESC`)
	if err != nil {
		return nil, fmt.Errorf("academic: yıllar okunamadı: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (AcademicYear, error) {
		var y AcademicYear
		err := row.Scan(&y.ID, &y.StartYear, &y.StartsOn, &y.EndsOn)
		return y, err
	})
}

// Terms, dönemleri en yeniden eskiye döndürür.
func (r *Repository) Terms(ctx context.Context) ([]Term, error) {
	rows, err := r.db.Query(ctx, termSelect+`
		ORDER BY y.start_year DESC, array_position(ARRAY['SUMMER','SPRING','FALL'], t.term_type)`)
	if err != nil {
		return nil, fmt.Errorf("academic: dönemler okunamadı: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Term, error) { return scanTerm(row) })
}

// Term, dönemi döndürür. Yoksa ErrNotFound döner.
func (r *Repository) Term(ctx context.Context, id string) (Term, error) {
	return r.term(ctx, r.db, termSelect+` WHERE t.id = $1`, id)
}

// CurrentTerm, aktif dönemi döndürür. Tanımlı değilse ErrNoCurrentTerm döner.
func (r *Repository) CurrentTerm(ctx context.Context) (Term, error) {
	t, err := r.term(ctx, r.db, termSelect+` WHERE t.is_current`)
	if errors.Is(err, ErrNotFound) {
		return Term{}, ErrNoCurrentTerm
	}
	return t, err
}

func (r *Repository) term(ctx context.Context, q db.Querier, query string, args ...any) (Term, error) {
	t, err := scanTerm(q.QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return Term{}, ErrNotFound
	}
	if err != nil {
		return Term{}, fmt.Errorf("academic: dönem okunamadı: %w", err)
	}
	return t, nil
}

// CreateAcademicYear, bir akademik yıl oluşturur. Aynı yıl varsa ErrConflict döner.
func (r *Repository) CreateAcademicYear(ctx context.Context, actorID string, y AcademicYear) (string, error) {
	var id string
	err := db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			INSERT INTO academic.academic_years (start_year, starts_on, ends_on) VALUES ($1, $2, $3) RETURNING id`,
			y.StartYear, y.StartsOn, y.EndsOn).Scan(&id)
		if db.IsConflict(err) {
			return ErrConflict
		}
		if err != nil {
			return fmt.Errorf("academic: yıl oluşturulamadı: %w", err)
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "academic_year.create", EntityType: "academic_year", EntityID: id,
			After: map[string]any{"start_year": y.StartYear, "starts_on": dateOnly(y.StartsOn), "ends_on": dateOnly(y.EndsOn)},
		})
	})
	return id, err
}

// TermInput, dönem oluşturma ve güncelleme verisidir.
type TermInput struct {
	Type     TermType // sadece oluşturmada
	StartsOn time.Time
	EndsOn   time.Time
	Status   TermStatus // sadece güncellemede
}

// CreateTerm, akademik yıla bir dönem ekler. Dönem yılın sınırları içinde olmalıdır.
func (r *Repository) CreateTerm(ctx context.Context, actorID, yearID string, in TermInput) (string, error) {
	var id string
	err := db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		var y AcademicYear
		err := tx.QueryRow(ctx, `SELECT id, start_year, starts_on, ends_on FROM academic.academic_years WHERE id = $1`, yearID).
			Scan(&y.ID, &y.StartYear, &y.StartsOn, &y.EndsOn)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("academic: yıl okunamadı: %w", err)
		}
		if in.StartsOn.Before(y.StartsOn) || in.EndsOn.After(y.EndsOn) {
			return ErrOutsideYear
		}
		code := TermCode(y.StartYear, in.Type)
		err = tx.QueryRow(ctx, `
			INSERT INTO academic.terms (academic_year_id, term_type, code, starts_on, ends_on)
			VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			yearID, string(in.Type), code, in.StartsOn, in.EndsOn).Scan(&id)
		if db.IsConflict(err) {
			return ErrConflict
		}
		if err != nil {
			return fmt.Errorf("academic: dönem oluşturulamadı: %w", err)
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "term.create", EntityType: "term", EntityID: id,
			After: map[string]any{"code": code, "starts_on": dateOnly(in.StartsOn), "ends_on": dateOnly(in.EndsOn)},
		})
	})
	return id, err
}

// UpdateTerm, dönemin tarihlerini ve durumunu günceller. Kapanan dönem aktif dönem
// olmaktan çıkar.
func (r *Repository) UpdateTerm(ctx context.Context, actorID, id string, version int, in TermInput) error {
	return db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		before, err := r.term(ctx, tx, termSelect+` WHERE t.id = $1 FOR UPDATE OF t`, id)
		if err != nil {
			return err
		}
		if before.Version != version {
			return ErrVersionMismatch
		}
		var yStart, yEnd time.Time
		if err := tx.QueryRow(ctx, `SELECT starts_on, ends_on FROM academic.academic_years WHERE id = $1`, before.AcademicYearID).
			Scan(&yStart, &yEnd); err != nil {
			return fmt.Errorf("academic: yıl okunamadı: %w", err)
		}
		if in.StartsOn.Before(yStart) || in.EndsOn.After(yEnd) {
			return ErrOutsideYear
		}
		_, err = tx.Exec(ctx, `
			UPDATE academic.terms
			SET starts_on = $2, ends_on = $3, status = $4,
			    is_current = is_current AND $4 <> 'CLOSED',
			    version = version + 1, updated_at = now()
			WHERE id = $1`, id, in.StartsOn, in.EndsOn, string(in.Status))
		if err != nil {
			return fmt.Errorf("academic: dönem güncellenemedi: %w", err)
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "term.update", EntityType: "term", EntityID: id,
			Before: map[string]any{"starts_on": dateOnly(before.StartsOn), "ends_on": dateOnly(before.EndsOn), "status": before.Status},
			After:  map[string]any{"starts_on": dateOnly(in.StartsOn), "ends_on": dateOnly(in.EndsOn), "status": in.Status},
		})
	})
}

// MakeCurrent, dönemi aktif (içinde bulunulan) dönem yapar ve durumunu ACTIVE'e çeker.
// Önceki aktif dönem aktif olmaktan çıkar, durumu değişmez.
func (r *Repository) MakeCurrent(ctx context.Context, actorID, id string) error {
	return db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		t, err := r.term(ctx, tx, termSelect+` WHERE t.id = $1 FOR UPDATE OF t`, id)
		if err != nil {
			return err
		}
		if t.Status == TermClosed {
			return ErrTermClosed
		}
		if t.IsCurrent {
			return nil
		}
		var previous *string
		if err := tx.QueryRow(ctx, `
			UPDATE academic.terms SET is_current = false, version = version + 1, updated_at = now()
			WHERE is_current RETURNING code`).Scan(&previous); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("academic: aktif dönem değiştirilemedi: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE academic.terms SET is_current = true, status = 'ACTIVE', version = version + 1, updated_at = now()
			WHERE id = $1`, id); err != nil {
			return fmt.Errorf("academic: aktif dönem değiştirilemedi: %w", err)
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "term.make_current", EntityType: "term", EntityID: id,
			Before: map[string]any{"current": previous}, After: map[string]any{"current": t.Code},
		})
	})
}

// --- Olay türleri ve olaylar ---------------------------------------------------

// EventTypes, olay türlerini sırasıyla döndürür.
func (r *Repository) EventTypes(ctx context.Context) ([]EventType, error) {
	rows, err := r.db.Query(ctx, `
		SELECT code, name_tr, name_en, category, is_action_window, sort_order
		FROM academic.calendar_event_types ORDER BY sort_order`)
	if err != nil {
		return nil, fmt.Errorf("academic: olay türleri okunamadı: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (EventType, error) {
		var t EventType
		err := row.Scan(&t.Code, &t.NameTR, &t.NameEN, &t.Category, &t.IsActionWindow, &t.SortOrder)
		return t, err
	})
}

const eventSelect = `
	SELECT e.id, e.term_id, t.code, t.name_tr, t.name_en, t.category, t.is_action_window, t.sort_order,
	       coalesce(e.title_tr, ''), coalesce(e.title_en, ''), lower(e.period), upper(e.period),
	       e.scope_type, coalesce(e.scope_id::text, ''),
	       coalesce(f.name_tr, p.name_tr, ''),
	       e.is_published, coalesce(e.note, ''), e.version
	FROM academic.calendar_events e
	JOIN academic.calendar_event_types t ON t.code = e.event_type_code
	LEFT JOIN org.faculties f ON e.scope_type = 'FACULTY' AND f.id = e.scope_id
	LEFT JOIN org.programs p  ON e.scope_type = 'PROGRAM' AND p.id = e.scope_id`

func scanEvent(s scanner) (Event, error) {
	var (
		e     Event
		scope string
	)
	err := s.Scan(&e.ID, &e.TermID, &e.Type.Code, &e.Type.NameTR, &e.Type.NameEN, &e.Type.Category,
		&e.Type.IsActionWindow, &e.Type.SortOrder, &e.TitleTR, &e.TitleEN, &e.StartsAt, &e.EndsAt,
		&scope, &e.ScopeID, &e.ScopeName, &e.IsPublished, &e.Note, &e.Version)
	e.ScopeType = ScopeType(scope)
	return e, err
}

// EventFilter, dönem olayları listesinin süzgecidir.
type EventFilter struct {
	TermID             string
	TypeCode           string
	Target             *WindowTarget // doluysa sadece bu hedefe uygulanan kapsamlar (üniversite + birim + program)
	IncludeUnpublished bool
}

// Events, dönemin olaylarını başlangıç zamanına göre sıralı döndürür.
func (r *Repository) Events(ctx context.Context, f EventFilter) ([]Event, error) {
	var w db.Where
	w.Add("e.term_id = $1", f.TermID)
	if f.TypeCode != "" {
		w.Add("e.event_type_code = $1", f.TypeCode)
	}
	if !f.IncludeUnpublished {
		w.Add("e.is_published")
	}
	if f.Target != nil {
		w.Add(`(e.scope_type = 'UNIVERSITY'
		        OR (e.scope_type = 'FACULTY' AND e.scope_id = $1::uuid)
		        OR (e.scope_type = 'PROGRAM' AND e.scope_id = $2::uuid))`,
			nullable(f.Target.FacultyID), nullable(f.Target.ProgramID))
	}
	rows, err := r.db.Query(ctx, eventSelect+"\n"+w.SQL()+"\nORDER BY lower(e.period), t.sort_order", w.Args()...)
	if err != nil {
		return nil, fmt.Errorf("academic: olaylar okunamadı: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Event, error) { return scanEvent(row) })
}

// Event, olayı döndürür. Yoksa ErrNotFound döner.
func (r *Repository) Event(ctx context.Context, id string) (Event, error) {
	e, err := scanEvent(r.db.QueryRow(ctx, eventSelect+` WHERE e.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Event{}, ErrNotFound
	}
	if err != nil {
		return Event{}, fmt.Errorf("academic: olay okunamadı: %w", err)
	}
	return e, nil
}

// EventInput, olay oluşturma ve güncelleme verisidir. Tür ve dönem sonradan değişmez.
type EventInput struct {
	TypeCode    string // sadece oluşturmada
	TitleTR     string
	TitleEN     string
	StartsAt    time.Time
	EndsAt      time.Time
	ScopeType   ScopeType
	ScopeID     string
	IsPublished bool
	Note        string
}

func (in EventInput) audit() map[string]any {
	return map[string]any{
		"type": in.TypeCode, "starts_at": in.StartsAt, "ends_at": in.EndsAt,
		"scope_type": in.ScopeType, "scope_id": in.ScopeID, "is_published": in.IsPublished,
	}
}

// CreateEvent, döneme bir olay ekler. Aynı türden aynı kapsamda çakışan bir olay varsa
// ErrEventOverlap döner.
func (r *Repository) CreateEvent(ctx context.Context, actorID, termID string, in EventInput) (string, error) {
	var id string
	err := db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		if err := checkScope(ctx, tx, in.ScopeType, in.ScopeID); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `
			INSERT INTO academic.calendar_events
			    (term_id, event_type_code, title_tr, title_en, period, scope_type, scope_id, is_published, note)
			VALUES ($1, $2, $3, $4, tstzrange($5, $6, '[)'), $7, $8, $9, $10)
			RETURNING id`,
			termID, in.TypeCode, nullable(in.TitleTR), nullable(in.TitleEN), in.StartsAt, in.EndsAt,
			string(in.ScopeType), nullable(in.ScopeID), in.IsPublished, nullable(in.Note)).Scan(&id)
		if err := eventWriteError(err, "olay oluşturulamadı"); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "calendar_event.create", EntityType: "calendar_event", EntityID: id,
			After: in.audit(),
		})
	})
	return id, err
}

// UpdateEvent, olayın zamanını, kapsamını ve başlığını günceller.
func (r *Repository) UpdateEvent(ctx context.Context, actorID, id string, version int, in EventInput) error {
	return db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		before, err := scanEvent(tx.QueryRow(ctx, eventSelect+` WHERE e.id = $1 FOR UPDATE OF e`, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("academic: olay okunamadı: %w", err)
		}
		if before.Version != version {
			return ErrVersionMismatch
		}
		if err := checkScope(ctx, tx, in.ScopeType, in.ScopeID); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			UPDATE academic.calendar_events
			SET title_tr = $2, title_en = $3, period = tstzrange($4, $5, '[)'), scope_type = $6, scope_id = $7,
			    is_published = $8, note = $9, version = version + 1, updated_at = now()
			WHERE id = $1`,
			id, nullable(in.TitleTR), nullable(in.TitleEN), in.StartsAt, in.EndsAt, string(in.ScopeType),
			nullable(in.ScopeID), in.IsPublished, nullable(in.Note))
		if err := eventWriteError(err, "olay güncellenemedi"); err != nil {
			return err
		}
		in.TypeCode = before.Type.Code
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "calendar_event.update", EntityType: "calendar_event", EntityID: id,
			Before: EventInput{
				TypeCode: before.Type.Code, StartsAt: before.StartsAt, EndsAt: before.EndsAt,
				ScopeType: before.ScopeType, ScopeID: before.ScopeID, IsPublished: before.IsPublished,
			}.audit(),
			After: in.audit(),
		})
	})
}

// DeleteEvent, olayı siler. Takvim olayları akademik kayıt değildir: yanlış girilen bir
// pencere silinebilir, silme denetim izine yazılır.
func (r *Repository) DeleteEvent(ctx context.Context, actorID, id string) error {
	return db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		before, err := scanEvent(tx.QueryRow(ctx, eventSelect+` WHERE e.id = $1 FOR UPDATE OF e`, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("academic: olay okunamadı: %w", err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM academic.calendar_events WHERE id = $1`, id); err != nil {
			return fmt.Errorf("academic: olay silinemedi: %w", err)
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "calendar_event.delete", EntityType: "calendar_event", EntityID: id,
			Before: EventInput{
				TypeCode: before.Type.Code, StartsAt: before.StartsAt, EndsAt: before.EndsAt,
				ScopeType: before.ScopeType, ScopeID: before.ScopeID, IsPublished: before.IsPublished,
			}.audit(),
		})
	})
}

// checkScope, kapsam biriminin var olduğunu doğrular: scope_id çok biçimli olduğu için
// yabancı anahtarla korunamaz.
func checkScope(ctx context.Context, q db.Querier, scope ScopeType, id string) error {
	var table string
	switch scope {
	case ScopeUniversity:
		return nil
	case ScopeFaculty:
		table = "org.faculties"
	case ScopeProgram:
		table = "org.programs"
	}
	var exists bool
	if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM `+table+` WHERE id = $1)`, id).Scan(&exists); err != nil {
		return fmt.Errorf("academic: kapsam okunamadı: %w", err)
	}
	if !exists {
		return ErrUnknownScope
	}
	return nil
}

func eventWriteError(err error, msg string) error {
	switch {
	case err == nil:
		return nil
	case db.IsExclusionViolation(err):
		return ErrEventOverlap
	case db.IsForeignKeyViolation(err):
		if db.ConstraintName(err) == "calendar_events_event_type_code_fkey" {
			return ErrUnknownEventType
		}
		return ErrNotFound // dönem yok
	}
	return fmt.Errorf("academic: %s: %w", msg, err)
}

// --- Pencere motoru ------------------------------------------------------------

// Windows, dönemin işlem pencerelerini hedefe göre çözer. Her tür için en dar kapsamda
// tanımlı olaylar uygulanır: hedefin programı için olay varsa o, yoksa biriminin, o da
// yoksa üniversitenin olayları. Pencere, uygulanan olaylardan biri at anını içeriyorsa açıktır.
func (r *Repository) Windows(ctx context.Context, termID string, target WindowTarget, at time.Time) ([]Window, error) {
	types, err := r.EventTypes(ctx)
	if err != nil {
		return nil, err
	}
	events, err := r.Events(ctx, EventFilter{TermID: termID, Target: &target})
	if err != nil {
		return nil, err
	}

	rank := map[ScopeType]int{ScopeUniversity: 1, ScopeFaculty: 2, ScopeProgram: 3}
	best := map[string]int{}
	for _, e := range events {
		best[e.Type.Code] = max(best[e.Type.Code], rank[e.ScopeType])
	}

	var out []Window
	for _, t := range types {
		if !t.IsActionWindow {
			continue
		}
		w := Window{Type: t}
		for _, e := range events {
			if e.Type.Code != t.Code || rank[e.ScopeType] != best[t.Code] {
				continue
			}
			w.Scope = e.ScopeType
			w.Events = append(w.Events, e)
			switch {
			case e.Contains(at):
				w.Open = true
				w.Current = &e
			case e.StartsAt.After(at) && w.Next == nil:
				w.Next = &e
			}
		}
		out = append(out, w)
	}
	return out, nil
}

// Window, tek bir türün penceresini çözer. Kayıt ve not kuralları bunu kullanır:
// "şu an bu program için ders seçme açık mı?"
func (r *Repository) Window(ctx context.Context, termID, typeCode string, target WindowTarget, at time.Time) (Window, error) {
	all, err := r.Windows(ctx, termID, target, at)
	if err != nil {
		return Window{}, err
	}
	for _, w := range all {
		if w.Type.Code == typeCode {
			return w, nil
		}
	}
	return Window{}, ErrUnknownEventType
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func dateOnly(t time.Time) string {
	return t.Format(time.DateOnly)
}
