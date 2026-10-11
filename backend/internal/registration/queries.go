package registration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/registration/rules"
)

// sectionSelect, şubeyi dersi, kontenjanı, öğretim elemanları ve haftalık programıyla okur.
// $1: öğrencinin programı (ayrılmış kontenjanlı şubede programa kalan yer için).
const sectionSelect = `
	SELECT s.id, o.id, c.id, c.code, c.name_tr, c.name_en, c.ects::float8, c.national_credit::float8,
	       s.section_code, s.capacity, s.enrolled_count, s.quota_mode, s.language,
	       CASE WHEN s.quota_mode = 'RESERVED' THEN coalesce(q.quota - q.enrolled, 0) END,
	       coalesce((
	           SELECT array_agg(coalesce(t.name_tr || ' ', '') || pe.first_name || ' ' || pe.last_name
	                            ORDER BY si.role = 'PRIMARY' DESC, pe.last_name, pe.first_name)
	           FROM offering.section_instructors si
	           JOIN people.staff st ON st.id = si.staff_id
	           JOIN people.persons pe ON pe.id = st.person_id
	           LEFT JOIN people.academic_titles t ON t.code = st.academic_title_code
	           WHERE si.section_id = s.id AND si.role <> 'ASSISTANT'), '{}'),
	       coalesce((
	           SELECT json_agg(json_build_object(
	                      'day', sl.day_of_week, 'start', left(lower(sl.time_span)::text, 5),
	                      'end', left(upper(sl.time_span)::text, 5), 'kind', sl.session_type,
	                      'room', coalesce(b.code || '/' || cr.code, ''))
	                  ORDER BY sl.day_of_week, lower(sl.time_span))
	           FROM offering.schedule_slots sl
	           LEFT JOIN org.classrooms cr ON cr.id = sl.classroom_id
	           LEFT JOIN org.buildings b ON b.id = cr.building_id
	           WHERE sl.section_id = s.id), '[]')
	FROM offering.sections s
	JOIN offering.course_offerings o ON o.id = s.offering_id
	JOIN curriculum.courses c ON c.id = o.course_id
	LEFT JOIN offering.section_quotas q ON q.section_id = s.id AND q.program_id = $1`

func scanSection(row pgx.CollectableRow) (Section, error) {
	var (
		s     Section
		slots []byte
	)
	err := row.Scan(&s.ID, &s.OfferingID, &s.CourseID, &s.CourseCode, &s.NameTR, &s.NameEN, &s.ECTS, &s.Credit,
		&s.Code, &s.Capacity, &s.Enrolled, &s.QuotaMode, &s.Language, &s.QuotaLeft, &s.Instructors, &slots)
	if err != nil {
		return Section{}, err
	}
	var raw []struct {
		Day   int    `json:"day"`
		Start string `json:"start"`
		End   string `json:"end"`
		Kind  string `json:"kind"`
		Room  string `json:"room"`
	}
	if err := json.Unmarshal(slots, &raw); err != nil {
		return Section{}, err
	}
	for _, r := range raw {
		s.Slots = append(s.Slots, Slot{Day: r.Day, Start: r.Start, End: r.End, Kind: r.Kind, Classroom: r.Room})
	}
	return s, nil
}

// OpenSections, dönemde ders seçmeye açık (açılmış dersin aktif) şubelerini, verilen
// dersler için döndürür.
func (r *Repository) OpenSections(ctx context.Context, termID, programID string, courseIDs []string) ([]Section, error) {
	rows, err := r.db.Query(ctx, sectionSelect+`
		WHERE o.term_id = $2 AND o.status = 'OPEN' AND s.status = 'ACTIVE' AND o.course_id = ANY($3)
		ORDER BY c.code, s.section_code`, programID, termID, courseIDs)
	if err != nil {
		return nil, fmt.Errorf("registration: şubeler okunamadı: %w", err)
	}
	list, err := pgx.CollectRows(rows, scanSection)
	if err != nil {
		return nil, fmt.Errorf("registration: şubeler okunamadı: %w", err)
	}
	return list, nil
}

// SectionState, şubenin ders seçmeye uygunluğu için gereken durumudur.
type SectionState struct {
	Section
	TermID         string
	OfferingStatus string
	SectionStatus  string
}

// SectionForRegistration, şubeyi dönemi ve durumlarıyla döndürür. Yoksa ErrSectionNotFound döner.
func (r *Repository) SectionForRegistration(ctx context.Context, sectionID, programID string) (SectionState, error) {
	var st SectionState
	err := r.db.QueryRow(ctx, `
		SELECT o.term_id, o.status, s.status FROM offering.sections s
		JOIN offering.course_offerings o ON o.id = s.offering_id WHERE s.id = $1`, sectionID,
	).Scan(&st.TermID, &st.OfferingStatus, &st.SectionStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return SectionState{}, ErrSectionNotFound
	}
	if err != nil {
		return SectionState{}, fmt.Errorf("registration: şube okunamadı: %w", err)
	}
	list, err := r.sectionsByID(ctx, programID, []string{sectionID})
	if err != nil {
		return SectionState{}, err
	}
	st.Section = list[0]
	return st, nil
}

func (r *Repository) sectionsByID(ctx context.Context, programID string, ids []string) ([]Section, error) {
	rows, err := r.db.Query(ctx, sectionSelect+`
		WHERE s.id = ANY($2) ORDER BY c.code, s.section_code`, programID, ids)
	if err != nil {
		return nil, fmt.Errorf("registration: şubeler okunamadı: %w", err)
	}
	list, err := pgx.CollectRows(rows, scanSection)
	if err != nil {
		return nil, fmt.Errorf("registration: şubeler okunamadı: %w", err)
	}
	return list, nil
}

// CurrentTerm, içinde bulunulan dönemin kimliğini ve kodunu döndürür. Yoksa ErrNoTerm döner.
func (r *Repository) CurrentTerm(ctx context.Context) (id, code string, err error) {
	err = r.db.QueryRow(ctx, `SELECT id, code FROM academic.terms WHERE is_current`).Scan(&id, &code)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNoTerm
	}
	if err != nil {
		return "", "", fmt.Errorf("registration: aktif dönem okunamadı: %w", err)
	}
	return id, code, nil
}

// TermCode, dönemin kodunu döndürür. Yoksa ErrNoTerm döner.
func (r *Repository) TermCode(ctx context.Context, termID string) (string, error) {
	var code string
	err := r.db.QueryRow(ctx, `SELECT code FROM academic.terms WHERE id = $1`, termID).Scan(&code)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNoTerm
	}
	if err != nil {
		return "", fmt.Errorf("registration: dönem okunamadı: %w", err)
	}
	return code, nil
}

// OfferedCourses, verilen derslerden dönemde ders seçmeye açık (aktif şubesi olan) olanları döndürür.
func (r *Repository) OfferedCourses(ctx context.Context, termID string, courseIDs []string) (map[string]bool, error) {
	rows, err := r.db.Query(ctx, `
		SELECT DISTINCT o.course_id FROM offering.course_offerings o
		JOIN offering.sections s ON s.offering_id = o.id AND s.status = 'ACTIVE'
		WHERE o.term_id = $1 AND o.status = 'OPEN' AND o.course_id = ANY($2)`, termID, courseIDs)
	if err != nil {
		return nil, fmt.Errorf("registration: açık dersler okunamadı: %w", err)
	}
	out := map[string]bool{}
	var id string
	if _, err := pgx.ForEachRow(rows, []any{&id}, func() error { out[id] = true; return nil }); err != nil {
		return nil, fmt.Errorf("registration: açık dersler okunamadı: %w", err)
	}
	return out, nil
}

// AllPrerequisites, bütün ön koşulları ders başına döndürür (küçük bir tablo; önbelleğe alınır).
func (r *Repository) AllPrerequisites(ctx context.Context) (map[string][]rules.Prerequisite, error) {
	rows, err := r.db.Query(ctx, `
		SELECT course_id, prerequisite_course_id, requirement, group_no FROM curriculum.course_prerequisites`)
	if err != nil {
		return nil, fmt.Errorf("registration: ön koşullar okunamadı: %w", err)
	}
	out := map[string][]rules.Prerequisite{}
	var (
		course, pre, req string
		group            int16
	)
	_, err = pgx.ForEachRow(rows, []any{&course, &pre, &req, &group}, func() error {
		out[course] = append(out[course], rules.Prerequisite{CourseID: pre, Requirement: rules.Requirement(req), Group: int(group)})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("registration: ön koşullar okunamadı: %w", err)
	}
	return out, nil
}

// AllCourseCodes, bütün derslerin kodlarını döndürür (kural mesajları için; önbelleğe alınır).
func (r *Repository) AllCourseCodes(ctx context.Context) (map[string]string, error) {
	rows, err := r.db.Query(ctx, `SELECT id, code FROM curriculum.courses`)
	if err != nil {
		return nil, fmt.Errorf("registration: ders kodları okunamadı: %w", err)
	}
	out := map[string]string{}
	var id, code string
	if _, err := pgx.ForEachRow(rows, []any{&id, &code}, func() error { out[id] = code; return nil }); err != nil {
		return nil, fmt.Errorf("registration: ders kodları okunamadı: %w", err)
	}
	return out, nil
}

// Params, at gününde geçerli yönetmelik parametrelerini okur. Tanımlı olmayan parametre
// için yönetmeliğin genel değeri kullanılır (30 AKTS dönem yükü, azami sürede 5 ders).
func (r *Repository) Params(ctx context.Context, at time.Time) (rules.Params, error) {
	p := rules.Params{DefaultECTS: 30, MaxCoursesAfterMaxDuration: 5}
	rows, err := r.db.Query(ctx, `
		SELECT key, value FROM curriculum.regulation_parameters
		WHERE key IN ('ects_limit_by_gpa', 'semester_ects_load', 'max_courses_after_max_duration')
		  AND effective_from <= $1 AND (effective_to IS NULL OR effective_to > $1)`, at)
	if err != nil {
		return p, fmt.Errorf("registration: yönetmelik parametreleri okunamadı: %w", err)
	}
	var (
		key   string
		value []byte
	)
	_, err = pgx.ForEachRow(rows, []any{&key, &value}, func() error {
		switch key {
		case "ects_limit_by_gpa":
			var steps []struct {
				MaxGPA float64 `json:"max_gpa"`
				ECTS   float64 `json:"ects"`
			}
			if err := json.Unmarshal(value, &steps); err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
			for _, s := range steps {
				p.ECTSSteps = append(p.ECTSSteps, rules.ECTSStep{MaxGPA: s.MaxGPA, ECTS: s.ECTS})
			}
		case "semester_ects_load":
			return json.Unmarshal(value, &p.DefaultECTS)
		case "max_courses_after_max_duration":
			return json.Unmarshal(value, &p.MaxCoursesAfterMaxDuration)
		}
		return nil
	})
	if err != nil {
		return p, fmt.Errorf("registration: yönetmelik parametreleri okunamadı: %w", err)
	}
	return p, nil
}

const registrationSelect = `
	SELECT r.id, r.student_program_id, r.term_id, t.code, r.status, r.total_ects::float8,
	       r.max_ects_allowed::float8, r.gpa_snapshot::float8, r.submitted_at, r.decided_at,
	       coalesce(pe.first_name || ' ' || pe.last_name, ''), coalesce(r.decision_note, ''),
	       r.extension_until, coalesce(r.extension_reason, ''), r.version
	FROM enrollment.registrations r
	JOIN academic.terms t ON t.id = r.term_id
	LEFT JOIN iam.users u ON u.id = r.decided_by
	LEFT JOIN people.persons pe ON pe.id = u.person_id`

func scanRegistration(row pgx.Row) (Registration, error) {
	var (
		reg    Registration
		status string
	)
	err := row.Scan(&reg.ID, &reg.StudentProgramID, &reg.TermID, &reg.TermCode, &status, &reg.TotalECTS,
		&reg.MaxECTSAllowed, &reg.GPASnapshot, &reg.SubmittedAt, &reg.DecidedAt, &reg.DecidedBy, &reg.DecisionNote,
		&reg.ExtensionUntil, &reg.ExtensionReason, &reg.Version)
	reg.Status = Status(status)
	return reg, err
}

// Registration, program kaydının dönemdeki ders seçme kaydını sepetiyle döndürür. Kayıt
// yoksa durumu NOT_STARTED olan boş bir kayıt döner.
func (r *Repository) Registration(ctx context.Context, studentProgramID, programID, termID string) (Registration, error) {
	reg, err := scanRegistration(r.db.QueryRow(ctx, registrationSelect+`
		WHERE r.student_program_id = $1 AND r.term_id = $2`, studentProgramID, termID))
	if errors.Is(err, pgx.ErrNoRows) {
		code, err := r.TermCode(ctx, termID)
		if err != nil {
			return Registration{}, err
		}
		return Registration{StudentProgramID: studentProgramID, TermID: termID, TermCode: code, Status: StatusNotStarted}, nil
	}
	if err != nil {
		return Registration{}, fmt.Errorf("registration: kayıt okunamadı: %w", err)
	}
	reg.Items, err = r.items(ctx, reg.ID, programID)
	return reg, err
}

// RegistrationByID, kaydı sepetiyle döndürür. Yoksa ErrNotFound döner.
func (r *Repository) RegistrationByID(ctx context.Context, id, programID string) (Registration, error) {
	reg, err := scanRegistration(r.db.QueryRow(ctx, registrationSelect+` WHERE r.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Registration{}, ErrNotFound
	}
	if err != nil {
		return Registration{}, fmt.Errorf("registration: kayıt okunamadı: %w", err)
	}
	reg.Items, err = r.items(ctx, reg.ID, programID)
	return reg, err
}

func (r *Repository) items(ctx context.Context, registrationID, programID string) ([]Item, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, section_id, item_kind, coalesce(curriculum_item_id::text, ''), ects::float8, source, status, added_at
		FROM enrollment.registration_items WHERE registration_id = $1 AND status = 'ACTIVE'
		ORDER BY added_at, id`, registrationID)
	if err != nil {
		return nil, fmt.Errorf("registration: sepet okunamadı: %w", err)
	}
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Item, error) {
		var it Item
		var kind string
		err := row.Scan(&it.ID, &it.Section.ID, &kind, &it.CurriculumItemID, &it.ECTS, &it.Source, &it.Status, &it.AddedAt)
		it.Kind = ItemKind(kind)
		return it, err
	})
	if err != nil {
		return nil, fmt.Errorf("registration: sepet okunamadı: %w", err)
	}
	if len(items) == 0 {
		return []Item{}, nil
	}
	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = it.Section.ID
	}
	sections, err := r.sectionsByID(ctx, programID, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]Section, len(sections))
	for _, s := range sections {
		byID[s.ID] = s
	}
	for i := range items {
		items[i].Section = byID[items[i].Section.ID]
	}
	return items, nil
}

// Events, kaydın olay günlüğünü eskiden yeniye döndürür.
func (r *Repository) Events(ctx context.Context, registrationID string) ([]Event, error) {
	rows, err := r.db.Query(ctx, `
		SELECT e.id, e.event_type, coalesce(pe.first_name || ' ' || pe.last_name, ''), e.payload, e.occurred_at
		FROM enrollment.registration_events e
		LEFT JOIN iam.users u ON u.id = e.actor_user_id
		LEFT JOIN people.persons pe ON pe.id = u.person_id
		WHERE e.registration_id = $1 ORDER BY e.id`, registrationID)
	if err != nil {
		return nil, fmt.Errorf("registration: olaylar okunamadı: %w", err)
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Event, error) {
		var e Event
		return e, row.Scan(&e.ID, &e.Type, &e.Actor, &e.Payload, &e.OccurredAt)
	})
	if err != nil {
		return nil, fmt.Errorf("registration: olaylar okunamadı: %w", err)
	}
	return list, nil
}
