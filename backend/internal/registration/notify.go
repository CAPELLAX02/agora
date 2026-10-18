package registration

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/notify"
)

// DecisionEvent, danışman kararı olaylarının (registration.approved, registration.rejected,
// registration.approval_revoked) verisidir.
type DecisionEvent struct {
	RegistrationID   string   `json:"registration_id"`
	StudentProgramID string   `json:"student_program_id"`
	TermID           string   `json:"term_id"`
	Status           Status   `json:"status"`
	Note             string   `json:"note"`
	Override         bool     `json:"override"`
	SectionIDs       []string `json:"section_ids"`
}

// notifyEvent, kayıt olayından ilgililere bildirim üretir (olayla aynı transaction'da):
// gönderilen kayıt danışmana, danışman kararı öğrenciye; ekle-bırak talebi önce danışmana,
// danışman onaylayınca programı kapsayan onaylayanlara (bölüm başkanı), sonuç öğrenciye.
func notifyEvent(ctx context.Context, tx pgx.Tx, registrationID, typ string, payload map[string]any) error {
	switch typ {
	case "SUBMITTED", "APPROVED", "REJECTED", "APPROVAL_REVOKED",
		"ADD_DROP_REQUESTED", "ADD_DROP_APPROVED", "ADD_DROP_REJECTED", "ADD_DROP_APPLIED":
	default:
		return nil
	}
	var (
		programID, term, student, studentNo string
		studentUser, advisorUser            string
		ects                                float64
		courses                             int
	)
	err := tx.QueryRow(ctx, `
		SELECT sp.program_id, t.code, pe.first_name || ' ' || pe.last_name, st.student_no,
		       coalesce(su.id::text, ''),
		       coalesce((SELECT au.id::text FROM enrollment.advisor_assignments aa
		                 JOIN people.staff s ON s.id = aa.advisor_staff_id
		                 JOIN iam.users au ON au.person_id = s.person_id AND au.status = 'ACTIVE'
		                 WHERE aa.student_program_id = sp.id AND aa.valid_until IS NULL
		                 LIMIT 1), ''),
		       r.total_ects::float8,
		       (SELECT count(*) FROM enrollment.registration_items i WHERE i.registration_id = r.id AND i.status = 'ACTIVE')
		FROM enrollment.registrations r
		JOIN academic.terms t ON t.id = r.term_id
		JOIN enrollment.student_programs sp ON sp.id = r.student_program_id
		JOIN people.students st ON st.id = sp.student_id
		JOIN people.persons pe ON pe.id = st.person_id
		LEFT JOIN iam.users su ON su.person_id = st.person_id
		WHERE r.id = $1`, registrationID).Scan(&programID, &term, &student, &studentNo, &studentUser, &advisorUser, &ects, &courses)
	if err != nil {
		return fmt.Errorf("registration: bildirim alıcıları okunamadı: %w", err)
	}
	note, _ := payload["note"].(string)
	send := func(typ notify.Type, to string, data map[string]any, link string) error {
		if to == "" {
			return nil // hesabı ya da danışmanı yok: bildirilecek kimse yok
		}
		return notify.Send(ctx, tx, notify.Notification{Type: typ, UserIDs: []string{to}, Data: data, Link: link})
	}
	who := map[string]any{"student": student, "student_no": studentNo, "term": term}

	switch typ {
	case "SUBMITTED":
		return send(notify.RegistrationSubmitted, advisorUser,
			map[string]any{"student": student, "student_no": studentNo, "term": term, "courses": courses, "ects": ects},
			"/danismanlik/kayitlar/"+registrationID)
	case "APPROVED", "REJECTED", "APPROVAL_REVOKED":
		decision := map[string]string{"APPROVED": "APPROVED", "REJECTED": "REJECTED", "APPROVAL_REVOKED": "REVOKED"}[typ]
		return send(notify.RegistrationDecision, studentUser,
			map[string]any{"decision": decision, "term": term, "courses": courses, "ects": ects, "note": note}, "/ders-secimi")
	case "ADD_DROP_REQUESTED":
		who["stage"] = "ADVISOR"
		return send(notify.AddDropPending, advisorUser, who, "/ekle-birak-onaylari")
	case "ADD_DROP_APPROVED": // danışman onayladı: talep bölüm başkanı aşamasında
		heads, err := notify.PermissionHolders(ctx, tx, "registration:approve_add_drop", notify.Target{ProgramID: programID})
		if err != nil {
			return err
		}
		who["stage"] = "DEPT_HEAD"
		return notify.Send(ctx, tx, notify.Notification{Type: notify.AddDropPending, UserIDs: heads, Data: who, Link: "/ekle-birak-onaylari"})
	case "ADD_DROP_REJECTED":
		return send(notify.AddDropDecision, studentUser, map[string]any{"decision": "REJECTED", "term": term, "note": note}, "/ders-secimi")
	default: // ADD_DROP_APPLIED
		return send(notify.AddDropDecision, studentUser, map[string]any{"decision": "APPLIED", "term": term, "note": note}, "/ders-secimi")
	}
}
