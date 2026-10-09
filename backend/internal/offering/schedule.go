package offering

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// Haftalık program değişiklikleri dönem başına sıraya girer: derslik çakışmasını
// veritabanı (EXCLUDE) engeller, ama öğretim elemanı çakışması birden fazla tabloya
// bakar ve serviste denetlenir. Kilit olmadan eşzamanlı iki atama ayrı ayrı denetimden
// geçip birlikte çakışma oluşturabilirdi.
func lockSchedule(ctx context.Context, tx pgx.Tx, termID string) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('offering.schedule'), hashtext($1))`, termID); err != nil {
		return fmt.Errorf("offering: program kilidi alınamadı: %w", err)
	}
	return nil
}

// sectionContext, şubenin dönemini, kontenjanını ve durumunu kilitleyerek okur.
type sectionContext struct {
	TermID     string
	OfferingID string
	Capacity   int
	Status     string
}

func lockSection(ctx context.Context, tx pgx.Tx, sectionID string) (sectionContext, error) {
	var sc sectionContext
	err := tx.QueryRow(ctx, `
		SELECT o.term_id, s.offering_id, s.capacity, s.status
		FROM offering.sections s JOIN offering.course_offerings o ON o.id = s.offering_id
		WHERE s.id = $1 FOR UPDATE OF s`, sectionID).Scan(&sc.TermID, &sc.OfferingID, &sc.Capacity, &sc.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return sectionContext{}, ErrNotFound
	}
	if err != nil {
		return sectionContext{}, fmt.Errorf("offering: şube okunamadı: %w", err)
	}
	return sc, nil
}

// --- Oturumlar -----------------------------------------------------------------

const slotSelect = `
	SELECT sl.id, sl.section_id, sl.day_of_week, to_char(lower(sl.time_span), 'HH24:MI'), to_char(upper(sl.time_span), 'HH24:MI'),
	       sl.session_type, cl.id, cl.code, cl.name, b.code, cl.capacity
	FROM offering.schedule_slots sl
	LEFT JOIN org.classrooms cl ON cl.id = sl.classroom_id
	LEFT JOIN org.buildings b ON b.id = cl.building_id`

func scanSlot(s scanner) (Slot, error) {
	var (
		sl                          Slot
		day                         int16
		clID, clCode, clName, bCode *string
		clCap                       *int
	)
	err := s.Scan(&sl.ID, &sl.SectionID, &day, &sl.Start, &sl.End, &sl.SessionType, &clID, &clCode, &clName, &bCode, &clCap)
	sl.DayOfWeek = int(day)
	if clID != nil {
		sl.Classroom = &ClassroomRef{ID: *clID, Code: *clCode, Name: *clName, BuildingCode: *bCode, Capacity: *clCap}
	}
	return sl, err
}

func (r *Repository) slots(ctx context.Context, q db.Querier, where string, args ...any) ([]Slot, error) {
	rows, err := q.Query(ctx, slotSelect+"\n"+where+"\nORDER BY sl.day_of_week, lower(sl.time_span)", args...)
	if err != nil {
		return nil, fmt.Errorf("offering: oturumlar okunamadı: %w", err)
	}
	slots, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Slot, error) { return scanSlot(row) })
	if err != nil {
		return nil, fmt.Errorf("offering: oturumlar okunamadı: %w", err)
	}
	return slots, nil
}

// Slot, oturumu döndürür. Yoksa ErrNotFound döner.
func (r *Repository) Slot(ctx context.Context, id string) (Slot, error) {
	slots, err := r.slots(ctx, r.db, `WHERE sl.id = $1`, id)
	if err != nil {
		return Slot{}, err
	}
	if len(slots) == 0 {
		return Slot{}, ErrNotFound
	}
	return slots[0], nil
}

// SlotInput, oturum ekleme ve güncelleme verisidir.
type SlotInput struct {
	DayOfWeek   int
	Start, End  string // HH:MM; bitiş hariç
	ClassroomID string // çevrim içi oturumda boş
	SessionType string
}

// AddSlot, şubeye oturum ekler. Derslik dolu, şubenin başka oturumuyla çakışıyor,
// öğretim elemanlarından birinin aynı saatte başka dersi var ya da teorik oturumun
// dersliği şube kontenjanını almıyorsa ilgili hata döner.
func (r *Repository) AddSlot(ctx context.Context, actorID, sectionID string, in SlotInput) (string, error) {
	var id string
	err := db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		sc, err := r.prepareSlot(ctx, tx, sectionID, "", in)
		if err != nil {
			return err
		}
		err = tx.QueryRow(ctx, `
			INSERT INTO offering.schedule_slots (section_id, term_id, day_of_week, time_span, classroom_id, session_type)
			VALUES ($1, $2, $3, offering.timerange($4::time, $5::time, '[)'), $6, $7) RETURNING id`,
			sectionID, sc.TermID, in.DayOfWeek, in.Start, in.End, nullable(in.ClassroomID), in.SessionType).Scan(&id)
		if err := slotWriteError(err); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "schedule.add_slot", EntityType: "section", EntityID: sectionID, After: in.audit(),
		})
	})
	return id, err
}

// UpdateSlot, oturumu değiştirir; AddSlot ile aynı denetimler yapılır.
func (r *Repository) UpdateSlot(ctx context.Context, actorID, slotID string, in SlotInput) error {
	return db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		var sectionID string
		err := tx.QueryRow(ctx, `SELECT section_id FROM offering.schedule_slots WHERE id = $1`, slotID).Scan(&sectionID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("offering: oturum okunamadı: %w", err)
		}
		if _, err := r.prepareSlot(ctx, tx, sectionID, slotID, in); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			UPDATE offering.schedule_slots
			SET day_of_week = $2, time_span = offering.timerange($3::time, $4::time, '[)'), classroom_id = $5, session_type = $6
			WHERE id = $1`, slotID, in.DayOfWeek, in.Start, in.End, nullable(in.ClassroomID), in.SessionType)
		if err := slotWriteError(err); err != nil {
			return err
		}
		after := in.audit()
		after["slot_id"] = slotID
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "schedule.update_slot", EntityType: "section", EntityID: sectionID, After: after,
		})
	})
}

// DeleteSlot, oturumu kaldırır.
func (r *Repository) DeleteSlot(ctx context.Context, actorID, slotID string) error {
	return db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		var sectionID string
		err := tx.QueryRow(ctx, `DELETE FROM offering.schedule_slots WHERE id = $1 RETURNING section_id`, slotID).Scan(&sectionID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("offering: oturum silinemedi: %w", err)
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "schedule.remove_slot", EntityType: "section", EntityID: sectionID,
			Before: map[string]any{"slot_id": slotID},
		})
	})
}

func (in SlotInput) audit() map[string]any {
	return map[string]any{
		"day_of_week": in.DayOfWeek, "start": in.Start, "end": in.End, "classroom_id": in.ClassroomID, "session_type": in.SessionType,
	}
}

// prepareSlot, oturum yazmadan önceki ortak denetimlerdir. slotID güncellemede
// değişen oturumdur (kendisiyle çakışmaz).
func (r *Repository) prepareSlot(ctx context.Context, tx pgx.Tx, sectionID, slotID string, in SlotInput) (sectionContext, error) {
	sc, err := lockSection(ctx, tx, sectionID)
	if err != nil {
		return sectionContext{}, err
	}
	if sc.Status == "CANCELLED" {
		return sectionContext{}, ErrSectionCancelled
	}
	if err := lockSchedule(ctx, tx, sc.TermID); err != nil {
		return sectionContext{}, err
	}
	if in.ClassroomID != "" {
		var (
			code, name string
			capacity   int
			active     bool
			roomType   string
		)
		err := tx.QueryRow(ctx, `
			SELECT b.code || '-' || cl.code, cl.name, cl.capacity, cl.is_active AND b.is_active, cl.room_type
			FROM org.classrooms cl JOIN org.buildings b ON b.id = cl.building_id WHERE cl.id = $1`, in.ClassroomID).
			Scan(&code, &name, &capacity, &active, &roomType)
		if errors.Is(err, pgx.ErrNoRows) {
			return sectionContext{}, ErrUnknownReference
		}
		if err != nil {
			return sectionContext{}, fmt.Errorf("offering: derslik okunamadı: %w", err)
		}
		if !active {
			return sectionContext{}, ErrClassroomInactive
		}
		// Uygulama ve laboratuvar oturumları gruplar hâlinde yapılabildiği için kapasite
		// teorik oturumda aranır; çevrim içi "derslik"lerin kapasitesi anlamsızdır.
		if in.SessionType == "THEORY" && roomType != "ONLINE" && capacity < sc.Capacity {
			return sectionContext{}, &ClassroomTooSmallError{Classroom: code, Capacity: capacity, Needed: sc.Capacity}
		}
		with, found, err := conflictingSlot(ctx, tx, `
			sl.term_id = $1 AND sl.classroom_id = $2 AND sl.day_of_week = $3
			AND sl.time_span && offering.timerange($4::time, $5::time, '[)') AND sl.id::text <> $6`,
			sc.TermID, in.ClassroomID, in.DayOfWeek, in.Start, in.End, slotID)
		if err != nil {
			return sectionContext{}, err
		}
		if found {
			return sectionContext{}, &ClassroomConflictError{With: with}
		}
	}
	// Şubenin kendi oturumları: veritabanı da engeller, burada okunur bir hata için bakılır.
	if _, found, err := conflictingSlot(ctx, tx, `
		sl.section_id = $1 AND sl.day_of_week = $2 AND sl.time_span && offering.timerange($3::time, $4::time, '[)') AND sl.id::text <> $5`,
		sectionID, in.DayOfWeek, in.Start, in.End, slotID); err != nil {
		return sectionContext{}, err
	} else if found {
		return sectionContext{}, ErrSectionOverlap
	}
	// Öğretim elemanları: şubenin her öğretim elemanının başka şubelerdeki oturumları.
	rows, err := tx.Query(ctx, `
		SELECT si.staff_id, coalesce(t.name_tr || ' ', '') || pe.first_name || ' ' || pe.last_name
		FROM offering.section_instructors si
		JOIN people.staff st ON st.id = si.staff_id
		JOIN people.persons pe ON pe.id = st.person_id
		LEFT JOIN people.academic_titles t ON t.code = st.academic_title_code
		WHERE si.section_id = $1`, sectionID)
	if err != nil {
		return sectionContext{}, fmt.Errorf("offering: öğretim elemanları okunamadı: %w", err)
	}
	type staffName struct{ id, name string }
	staff, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (staffName, error) {
		var s staffName
		return s, row.Scan(&s.id, &s.name)
	})
	if err != nil {
		return sectionContext{}, fmt.Errorf("offering: öğretim elemanları okunamadı: %w", err)
	}
	for _, st := range staff {
		with, found, err := conflictingSlot(ctx, tx, `
			sl.term_id = $1 AND sl.section_id <> $2 AND sl.day_of_week = $3
			AND sl.time_span && offering.timerange($4::time, $5::time, '[)')
			AND EXISTS (SELECT 1 FROM offering.section_instructors si WHERE si.section_id = sl.section_id AND si.staff_id = $6)`,
			sc.TermID, sectionID, in.DayOfWeek, in.Start, in.End, st.id)
		if err != nil {
			return sectionContext{}, err
		}
		if found {
			return sectionContext{}, &InstructorConflictError{StaffName: st.name, With: with}
		}
	}
	return sc, nil
}

// conflictingSlot, koşula uyan ilk etkin oturumu (ders kodu, şube, gün, saat) döndürür.
func conflictingSlot(ctx context.Context, tx pgx.Tx, cond string, args ...any) (SlotRef, bool, error) {
	var (
		ref SlotRef
		day int16
	)
	err := tx.QueryRow(ctx, `
		SELECT c.code, s.section_code, sl.day_of_week, to_char(lower(sl.time_span), 'HH24:MI'), to_char(upper(sl.time_span), 'HH24:MI')
		FROM offering.schedule_slots sl
		JOIN offering.sections s ON s.id = sl.section_id AND s.status = 'ACTIVE'
		JOIN offering.course_offerings o ON o.id = s.offering_id
		JOIN curriculum.courses c ON c.id = o.course_id
		WHERE `+cond+`
		ORDER BY sl.day_of_week, lower(sl.time_span) LIMIT 1`, args...).
		Scan(&ref.CourseCode, &ref.SectionCode, &day, &ref.Start, &ref.End)
	if errors.Is(err, pgx.ErrNoRows) {
		return SlotRef{}, false, nil
	}
	if err != nil {
		return SlotRef{}, false, fmt.Errorf("offering: çakışma denetlenemedi: %w", err)
	}
	ref.DayOfWeek = int(day)
	return ref, true, nil
}

// classroomsFit, şubenin teorik oturumlarının dersliklerinin verilen kontenjanı alıp
// almadığına bakar.
func classroomsFit(ctx context.Context, tx pgx.Tx, sectionID string, capacity int) error {
	var (
		code    string
		seating int
	)
	err := tx.QueryRow(ctx, `
		SELECT b.code || '-' || cl.code, cl.capacity
		FROM offering.schedule_slots sl
		JOIN org.classrooms cl ON cl.id = sl.classroom_id
		JOIN org.buildings b ON b.id = cl.building_id
		WHERE sl.section_id = $1 AND sl.session_type = 'THEORY' AND cl.room_type <> 'ONLINE' AND cl.capacity < $2
		ORDER BY cl.capacity LIMIT 1`, sectionID, capacity).Scan(&code, &seating)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("offering: derslik kapasitesi denetlenemedi: %w", err)
	}
	return &ClassroomTooSmallError{Classroom: code, Capacity: seating, Needed: capacity}
}

func slotWriteError(err error) error {
	switch {
	case err == nil:
		return nil
	case db.IsExclusionViolation(err) && db.ConstraintName(err) == "schedule_slots_section_no_overlap":
		return ErrSectionOverlap
	case db.IsExclusionViolation(err):
		return &ClassroomConflictError{}
	case db.IsForeignKeyViolation(err):
		return ErrUnknownReference
	}
	return fmt.Errorf("offering: oturum yazılamadı: %w", err)
}

// --- Öğretim elemanları --------------------------------------------------------

// InstructorInput, şubeye atanan öğretim elemanıdır.
type InstructorInput struct {
	StaffID string
	Role    string
}

// SetInstructors, şubenin öğretim elemanlarını verilen kümeyle değiştirir. Küme boş
// değilse tam bir sorumlu (PRIMARY) olmalı; atananlar görevdeki akademik personel
// olmalı ve şubenin oturum saatlerinde başka dersleri olmamalıdır.
func (r *Repository) SetInstructors(ctx context.Context, actorID, sectionID string, items []InstructorInput) error {
	return db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		sc, err := lockSection(ctx, tx, sectionID)
		if err != nil {
			return err
		}
		if sc.Status == "CANCELLED" {
			return ErrSectionCancelled
		}
		if err := lockSchedule(ctx, tx, sc.TermID); err != nil {
			return err
		}
		primaries := 0
		ids := make([]string, 0, len(items))
		for _, it := range items {
			if it.Role == "PRIMARY" {
				primaries++
			}
			ids = append(ids, it.StaffID)
		}
		if len(items) > 0 && primaries != 1 {
			return ErrPrimaryRequired
		}

		// Atananlar görevdeki akademik personel olmalı.
		var eligible int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM people.staff
			WHERE id = ANY($1::uuid[]) AND staff_type = 'ACADEMIC' AND employment_status = 'ACTIVE'`, ids).Scan(&eligible); err != nil {
			return fmt.Errorf("offering: personel okunamadı: %w", err)
		}
		if eligible != len(ids) {
			return ErrNotInstructor
		}

		// Şubenin her oturumu için, atanan her öğretim elemanının başka şubedeki oturumları.
		var (
			name, code, section string
			day                 int16
			start, end          string
		)
		err = tx.QueryRow(ctx, `
			SELECT coalesce(t.name_tr || ' ', '') || pe.first_name || ' ' || pe.last_name,
			       c.code, os.section_code, other.day_of_week,
			       to_char(lower(other.time_span), 'HH24:MI'), to_char(upper(other.time_span), 'HH24:MI')
			FROM offering.schedule_slots mine
			JOIN offering.schedule_slots other
			  ON other.term_id = mine.term_id AND other.day_of_week = mine.day_of_week
			 AND other.time_span && mine.time_span AND other.section_id <> mine.section_id
			JOIN offering.sections os ON os.id = other.section_id AND os.status = 'ACTIVE'
			JOIN offering.course_offerings o ON o.id = os.offering_id
			JOIN curriculum.courses c ON c.id = o.course_id
			JOIN offering.section_instructors si ON si.section_id = other.section_id AND si.staff_id = ANY($2::uuid[])
			JOIN people.staff st ON st.id = si.staff_id
			JOIN people.persons pe ON pe.id = st.person_id
			LEFT JOIN people.academic_titles t ON t.code = st.academic_title_code
			WHERE mine.section_id = $1
			ORDER BY other.day_of_week, lower(other.time_span) LIMIT 1`, sectionID, ids).
			Scan(&name, &code, &section, &day, &start, &end)
		switch {
		case err == nil:
			return &InstructorConflictError{StaffName: name, With: SlotRef{CourseCode: code, SectionCode: section, DayOfWeek: int(day), Start: start, End: end}}
		case !errors.Is(err, pgx.ErrNoRows):
			return fmt.Errorf("offering: çakışma denetlenemedi: %w", err)
		}

		if _, err := tx.Exec(ctx, `DELETE FROM offering.section_instructors WHERE section_id = $1`, sectionID); err != nil {
			return fmt.Errorf("offering: öğretim elemanları silinemedi: %w", err)
		}
		after := make([]map[string]any, 0, len(items))
		for _, it := range items {
			_, err := tx.Exec(ctx, `INSERT INTO offering.section_instructors (section_id, staff_id, role) VALUES ($1, $2, $3)`,
				sectionID, it.StaffID, it.Role)
			if db.IsConflict(err) {
				return ErrConflict
			}
			if err != nil {
				return fmt.Errorf("offering: öğretim elemanı atanamadı: %w", err)
			}
			after = append(after, map[string]any{"staff_id": it.StaffID, "role": it.Role})
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "section.set_instructors", EntityType: "section", EntityID: sectionID,
			After: map[string]any{"instructors": after},
		})
	})
}

// --- Görünümler ----------------------------------------------------------------

// ScheduleFilter, dönemin haftalık programının süzgecidir: bölümün açtığı dersler,
// bir derslik ya da bir öğretim elemanı.
type ScheduleFilter struct {
	TermID       string
	DepartmentID string
	ClassroomID  string
	StaffID      string
}

// Schedule, süzgece uyan etkin şubelerin oturumlarını gün ve saate göre döndürür.
func (r *Repository) Schedule(ctx context.Context, f ScheduleFilter) ([]ScheduleEntry, error) {
	var w db.Where
	w.Add("sl.term_id = $1", f.TermID)
	w.Add("s.status = 'ACTIVE'")
	if f.DepartmentID != "" {
		w.Add("o.department_id = $1", f.DepartmentID)
	}
	if f.ClassroomID != "" {
		w.Add("sl.classroom_id = $1", f.ClassroomID)
	}
	if f.StaffID != "" {
		w.Add("EXISTS (SELECT 1 FROM offering.section_instructors si WHERE si.section_id = s.id AND si.staff_id = $1)", f.StaffID)
	}
	return r.schedule(ctx, w)
}

func (r *Repository) schedule(ctx context.Context, w db.Where) ([]ScheduleEntry, error) {
	rows, err := r.db.Query(ctx, `
		SELECT sl.id, sl.section_id, sl.day_of_week, to_char(lower(sl.time_span), 'HH24:MI'), to_char(upper(sl.time_span), 'HH24:MI'),
		       sl.session_type, cl.id, cl.code, cl.name, b.code, cl.capacity,
		       sl.term_id, o.id, c.id, c.code, c.name_tr, c.name_en, s.section_code
		FROM offering.schedule_slots sl
		JOIN offering.sections s ON s.id = sl.section_id
		JOIN offering.course_offerings o ON o.id = s.offering_id
		JOIN curriculum.courses c ON c.id = o.course_id
		LEFT JOIN org.classrooms cl ON cl.id = sl.classroom_id
		LEFT JOIN org.buildings b ON b.id = cl.building_id
		`+w.SQL()+`
		ORDER BY sl.day_of_week, lower(sl.time_span), c.code, s.section_code`, w.Args()...)
	if err != nil {
		return nil, fmt.Errorf("offering: program okunamadı: %w", err)
	}
	entries, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (ScheduleEntry, error) {
		var (
			e                           ScheduleEntry
			day                         int16
			clID, clCode, clName, bCode *string
			clCap                       *int
		)
		err := row.Scan(&e.ID, &e.SectionID, &day, &e.Start, &e.End, &e.SessionType, &clID, &clCode, &clName, &bCode, &clCap,
			&e.TermID, &e.OfferingID, &e.Course.ID, &e.Course.Code, &e.Course.NameTR, &e.Course.NameEN, &e.SectionCode)
		e.DayOfWeek = int(day)
		if clID != nil {
			e.Classroom = &ClassroomRef{ID: *clID, Code: *clCode, Name: *clName, BuildingCode: *bCode, Capacity: *clCap}
		}
		e.Instructors = []Instructor{}
		return e, err
	})
	if err != nil {
		return nil, fmt.Errorf("offering: program okunamadı: %w", err)
	}
	if len(entries) == 0 {
		return entries, nil
	}
	sectionIDs := make([]string, 0, len(entries))
	for _, e := range entries {
		sectionIDs = append(sectionIDs, e.SectionID)
	}
	instructors, err := sectionInstructors(ctx, r.db, sectionIDs)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		if list, ok := instructors[entries[i].SectionID]; ok {
			entries[i].Instructors = list
		}
	}
	return entries, nil
}

// StaffByUser, kullanıcının personel kaydını döndürür. Personel değilse ErrNotFound döner.
func (r *Repository) StaffByUser(ctx context.Context, userID string) (string, error) {
	var id string
	err := r.db.QueryRow(ctx, `
		SELECT st.id FROM iam.users u JOIN people.staff st ON st.person_id = u.person_id WHERE u.id = $1`, userID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("offering: personel okunamadı: %w", err)
	}
	return id, nil
}

// CurrentTermID, aktif dönemin kimliğini döndürür. Aktif dönem yoksa ErrNotFound döner.
func (r *Repository) CurrentTermID(ctx context.Context) (string, error) {
	var id string
	err := r.db.QueryRow(ctx, `SELECT id FROM academic.terms WHERE is_current`).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("offering: aktif dönem okunamadı: %w", err)
	}
	return id, nil
}

// TermExists, dönemin var olup olmadığını söyler.
func (r *Repository) TermExists(ctx context.Context, id string) (bool, error) {
	var ok bool
	if err := r.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM academic.terms WHERE id = $1)`, id).Scan(&ok); err != nil {
		return false, fmt.Errorf("offering: dönem okunamadı: %w", err)
	}
	return ok, nil
}

// SearchInstructors, görevdeki akademik personeli adına ya da sicil numarasına göre arar.
func (r *Repository) SearchInstructors(ctx context.Context, query, departmentID string, limit int) ([]StaffSummary, error) {
	var w db.Where
	w.Add("st.staff_type = 'ACADEMIC'")
	w.Add("st.employment_status = 'ACTIVE'")
	if query != "" {
		w.Add("(pe.first_name || ' ' || pe.last_name || ' ' || st.staff_no) ILIKE $1", "%"+query+"%")
	}
	if departmentID != "" {
		w.Add("st.primary_department_id = $1", departmentID)
	}
	lim := w.Arg(limit)
	rows, err := r.db.Query(ctx, `
		SELECT st.id, st.staff_no, coalesce(t.name_tr, ''), pe.first_name, pe.last_name, d.id, d.code, d.name_tr, d.name_en
		FROM people.staff st
		JOIN people.persons pe ON pe.id = st.person_id
		LEFT JOIN people.academic_titles t ON t.code = st.academic_title_code
		LEFT JOIN org.departments d ON d.id = st.primary_department_id
		`+w.SQL()+`
		ORDER BY t.rank NULLS LAST, pe.last_name, pe.first_name LIMIT `+lim, w.Args()...)
	if err != nil {
		return nil, fmt.Errorf("offering: personel aranamadı: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (StaffSummary, error) {
		var (
			s                    StaffSummary
			dID, dCode, dTR, dEN *string
		)
		err := row.Scan(&s.StaffID, &s.StaffNo, &s.Title, &s.FirstName, &s.LastName, &dID, &dCode, &dTR, &dEN)
		if dID != nil {
			s.Department = &Ref{ID: *dID, Code: *dCode, NameTR: *dTR, NameEN: *dEN}
		}
		return s, err
	})
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
