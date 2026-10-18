package grading

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/notify"
	"github.com/CAPELLAX02/agora/backend/internal/platform/outbox"
)

// Bildirimler: yayımlanan bileşen notu ve ilan edilen harf notu öğrenciye; not düzeltme talebi
// onaylayanlara, kararı talep edene. İşlemle aynı transaction'da yazılır.

// publishGrades, not ilanı olayını (FINAL, MAKEUP, CORRECTION) yazar ve notu ilan edilen
// öğrencilere bildirir.
func publishGrades(ctx context.Context, tx pgx.Tx, ev PublishedEvent) error {
	c, err := sectionCourse(ctx, tx, ev.SectionID)
	if err != nil {
		return err
	}
	users, err := userIDs(ctx, tx, `
		SELECT DISTINCT u.id FROM enrollment.enrollments e
		JOIN enrollment.student_programs sp ON sp.id = e.student_program_id
		JOIN people.students st ON st.id = sp.student_id
		JOIN iam.users u ON u.person_id = st.person_id
		WHERE e.id = ANY ($1::uuid[])`, ev.Enrollments)
	if err != nil {
		return err
	}
	if err := notify.Send(ctx, tx, notify.Notification{Type: notify.LettersPublished, UserIDs: users, Link: "/notlarim",
		Data: map[string]any{"course_code": c.code, "course_tr": c.nameTR, "course_en": c.nameEN, "phase": ev.Phase}}); err != nil {
		return err
	}
	return outbox.Publish(ctx, tx, "grades.published", "section", ev.SectionID, ev)
}

// notifyComponentPublished, yayımlanan bileşende notu olan (dersi bırakmamış) öğrencilere bildirir.
func notifyComponentPublished(ctx context.Context, tx pgx.Tx, componentID string) error {
	var sectionID, labelTR, labelEN string
	// Bileşenin adı: verilmişse kendi adı, yoksa türün adı (aynı türden birden fazlaysa sıra numarasıyla).
	err := tx.QueryRow(ctx, `
		SELECT ac.section_id,
		       coalesce(nullif(ac.name_tr, ''), t.name_tr || CASE WHEN k.n > 1 THEN ' ' || ac.sequence_no ELSE '' END),
		       coalesce(nullif(ac.name_en, ''), t.name_en || CASE WHEN k.n > 1 THEN ' ' || ac.sequence_no ELSE '' END)
		FROM grading.assessment_components ac
		JOIN grading.assessment_types t ON t.code = ac.type_code,
		LATERAL (SELECT count(*) AS n FROM grading.assessment_components x
		         WHERE x.section_id = ac.section_id AND x.type_code = ac.type_code) k
		WHERE ac.id = $1`, componentID).Scan(&sectionID, &labelTR, &labelEN)
	if err != nil {
		return fmt.Errorf("grading: bileşen okunamadı: %w", err)
	}
	c, err := sectionCourse(ctx, tx, sectionID)
	if err != nil {
		return err
	}
	users, err := userIDs(ctx, tx, `
		SELECT DISTINCT u.id FROM grading.assessment_scores sc
		JOIN enrollment.enrollments e ON e.id = sc.enrollment_id AND e.status <> 'DROPPED'
		JOIN enrollment.student_programs sp ON sp.id = e.student_program_id
		JOIN people.students st ON st.id = sp.student_id
		JOIN iam.users u ON u.person_id = st.person_id
		WHERE sc.component_id = $1`, componentID)
	if err != nil {
		return err
	}
	return notify.Send(ctx, tx, notify.Notification{Type: notify.ComponentPublished, UserIDs: users, Link: "/notlarim",
		Data: map[string]any{"course_code": c.code, "course_tr": c.nameTR, "course_en": c.nameEN,
			"component_tr": labelTR, "component_en": labelEN}})
}

// changeInfo, bir not düzeltme talebinin bildirimde kullanılan bilgileridir.
type changeInfo struct {
	courseCode, departmentID, student, requester, requestedBy string
}

func readChangeInfo(ctx context.Context, tx pgx.Tx, requestID string) (changeInfo, error) {
	var ci changeInfo
	err := tx.QueryRow(ctx, `
		SELECT c.code, o.department_id, pe.first_name || ' ' || pe.last_name,
		       coalesce(rp.first_name || ' ' || rp.last_name, ''), coalesce(r.requested_by::text, '')
		FROM grading.grade_change_requests r
		JOIN offering.sections s ON s.id = r.section_id
		JOIN offering.course_offerings o ON o.id = s.offering_id
		JOIN curriculum.courses c ON c.id = o.course_id
		JOIN enrollment.enrollments e ON e.id = r.enrollment_id
		JOIN enrollment.student_programs sp ON sp.id = e.student_program_id
		JOIN people.students st ON st.id = sp.student_id
		JOIN people.persons pe ON pe.id = st.person_id
		LEFT JOIN iam.users ru ON ru.id = r.requested_by
		LEFT JOIN people.persons rp ON rp.id = ru.person_id
		WHERE r.id = $1`, requestID).Scan(&ci.courseCode, &ci.departmentID, &ci.student, &ci.requester, &ci.requestedBy)
	if err != nil {
		return ci, fmt.Errorf("grading: düzeltme talebi okunamadı: %w", err)
	}
	return ci, nil
}

// notifyChangeRequested, talebi şubeyi kapsayan grade_change:approve sahiplerine (talep eden hariç)
// bildirir.
func notifyChangeRequested(ctx context.Context, tx pgx.Tx, requestID string) error {
	ci, err := readChangeInfo(ctx, tx, requestID)
	if err != nil {
		return err
	}
	approvers, err := notify.PermissionHolders(ctx, tx, permChangeApprove, notify.Target{DepartmentID: ci.departmentID})
	if err != nil {
		return err
	}
	approvers = slices.DeleteFunc(approvers, func(id string) bool { return id == ci.requestedBy })
	return notify.Send(ctx, tx, notify.Notification{Type: notify.ChangeRequested, UserIDs: approvers, Link: "/not-duzeltme-onaylari",
		Data: map[string]any{"course_code": ci.courseCode, "student": ci.student, "requester": ci.requester}})
}

// notifyChangeDecided, kararı (APPLIED, REJECTED) talep edene bildirir.
func notifyChangeDecided(ctx context.Context, tx pgx.Tx, requestID, decision, note string) error {
	ci, err := readChangeInfo(ctx, tx, requestID)
	if err != nil {
		return err
	}
	if ci.requestedBy == "" {
		return nil
	}
	return notify.Send(ctx, tx, notify.Notification{Type: notify.ChangeDecided, UserIDs: []string{ci.requestedBy}, Link: "/not-duzeltme",
		Data: map[string]any{"course_code": ci.courseCode, "student": ci.student, "decision": decision, "note": note}})
}

// courseRef, şubenin dersidir.
type courseRef struct {
	code, nameTR, nameEN string
}

func sectionCourse(ctx context.Context, tx pgx.Tx, sectionID string) (courseRef, error) {
	var c courseRef
	if err := tx.QueryRow(ctx, `
		SELECT c.code, c.name_tr, c.name_en
		FROM offering.sections s
		JOIN offering.course_offerings o ON o.id = s.offering_id
		JOIN curriculum.courses c ON c.id = o.course_id
		WHERE s.id = $1`, sectionID).Scan(&c.code, &c.nameTR, &c.nameEN); err != nil {
		return c, fmt.Errorf("grading: şubenin dersi okunamadı: %w", err)
	}
	return c, nil
}

func userIDs(ctx context.Context, tx pgx.Tx, sql string, args ...any) ([]string, error) {
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("grading: bildirim alıcıları okunamadı: %w", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("grading: bildirim alıcıları okunamadı: %w", err)
	}
	return ids, nil
}
