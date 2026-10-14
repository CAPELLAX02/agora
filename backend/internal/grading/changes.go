package grading

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/grading/letter"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
	"github.com/CAPELLAX02/agora/backend/internal/platform/outbox"
)

// Not düzeltme hataları.
var (
	ErrGradesNotPublished    = errors.New("grading: harf notları ilan edilmedi")
	ErrNotCompleted          = errors.New("grading: kesin kaydın notu verilmedi")
	ErrMakeupNotPublishedYet = errors.New("grading: bütünleme sonuçları ilan edilmedi")
	ErrComponentNotInSection = errors.New("grading: bileşen bu şubenin planında değil")
	ErrIncompleteGrade       = errors.New("grading: düzeltilen notla harf notu hesaplanamıyor")
	ErrNoGradeChange         = errors.New("grading: talep notu değiştirmiyor")
	ErrGradeChangePending    = errors.New("grading: bu kayıtta bekleyen bir düzeltme talebi var")
	ErrGradeChangeNotPending = errors.New("grading: talep artık beklemede değil")
	ErrGradeChangeStale      = errors.New("grading: not talepten sonra değişti")
	ErrSelfApproval          = errors.New("grading: talep eden onaylayamaz")
)

// Not düzeltme türleri ve durumları.
const (
	ChangeScore  = "SCORE"  // bileşen notu düzeltilir, harf motorla yeniden hesaplanır
	ChangeLetter = "LETTER" // harf doğrudan değişir

	ChangePending   = "PENDING"
	ChangeApplied   = "APPLIED"
	ChangeRejected  = "REJECTED"
	ChangeWithdrawn = "WITHDRAWN"
)

// GradeValue, bir kesin kaydın düzeltmeden önceki ya da sonraki notudur.
type GradeValue struct {
	Letter   string  `json:"letter"`
	Weighted *Points `json:"weighted"`
	Reason   string  `json:"reason"`
	// Score ve Absent: SCORE türünde düzeltilen bileşenin notu.
	Score  *Points `json:"score,omitempty"`
	Absent bool    `json:"absent,omitempty"`
}

// GradeChangeInput, bir düzeltme talebidir. ComponentID boşsa harf doğrudan değişir (Letter).
type GradeChangeInput struct {
	EnrollmentID string
	ComponentID  string
	Score        *Points
	Absent       bool
	Letter       string
	Reason       string
}

// GradeChange, bir düzeltmenin önizlemesidir: kaydın şubesi ve notun önceki ve sonraki hali.
type GradeChange struct {
	Section          SectionContext
	StudentProgramID string
	Kind             string
	Component        *Component
	// ComponentNumbered, aynı türden birden fazla bileşen var: etiket numaralı.
	ComponentNumbered bool
	Old, New          GradeValue
}

// prepareGradeChange, düzeltmeyi doğrular ve sonucunu hesaplar; bir şey yazmaz. Harf notları
// ilan edilmiş, kesin kaydın notu verilmiş olmalıdır. Bütünleme ilan edilmediyse girilmiş
// bütünleme notları hesaba katılmaz (o notlar bütünleme ilanıyla uygulanır).
func (s *Service) prepareGradeChange(ctx context.Context, q db.Querier, actorID string, in GradeChangeInput) (GradeChange, error) {
	var (
		sectionID, status, spID string
	)
	err := q.QueryRow(ctx, `SELECT section_id, status, student_program_id FROM enrollment.enrollments WHERE id = $1`,
		in.EnrollmentID).Scan(&sectionID, &status, &spID)
	if errors.Is(err, pgx.ErrNoRows) {
		return GradeChange{}, ErrNotFound
	}
	if err != nil {
		return GradeChange{}, fmt.Errorf("grading: kesin kayıt okunamadı: %w", err)
	}
	sc, err := s.repo.sectionContext(ctx, q, sectionID, actorID, false)
	if err != nil {
		return GradeChange{}, err
	}
	switch {
	case sc.GradesPublishedAt == nil:
		return GradeChange{}, ErrGradesNotPublished
	case status != "COMPLETED":
		return GradeChange{}, ErrNotCompleted
	}
	g, err := s.letterGrades(ctx, q, sc)
	if err != nil {
		return GradeChange{}, err
	}
	var row *LetterRow
	for i := range g.Rows {
		if g.Rows[i].EnrollmentID == in.EnrollmentID {
			row = &g.Rows[i]
		}
	}
	if row == nil {
		return GradeChange{}, ErrNotFound
	}
	ch := GradeChange{
		Section: sc, StudentProgramID: spID,
		Old: GradeValue{Letter: row.PublishedLetter, Weighted: row.PublishedWeighted, Reason: row.PublishedReason},
	}
	scale, passFail := g.Scales[row.ScaleID], sc.GradingMode == "PASS_FAIL"

	if in.ComponentID == "" {
		ch.Kind = ChangeLetter
		if _, ok := ManualLetters(scale, passFail).Find(in.Letter); !ok {
			return GradeChange{}, fmt.Errorf("%w: %s", ErrLetterNotAllowed, in.Letter)
		}
		if in.Letter == ch.Old.Letter {
			return GradeChange{}, ErrNoGradeChange
		}
		ch.New = GradeValue{Letter: in.Letter, Weighted: ch.Old.Weighted, Reason: "MANUAL"}
		return ch, nil
	}

	ch.Kind = ChangeScore
	for i := range g.Components {
		if g.Components[i].ID == in.ComponentID {
			ch.Component = &g.Components[i]
		}
	}
	if ch.Component == nil {
		return GradeChange{}, ErrComponentNotInSection
	}
	for _, c := range g.Components {
		if c.ID != ch.Component.ID && c.Type.Code == ch.Component.Type.Code {
			ch.ComponentNumbered = true
		}
	}
	scores := make(map[string]Score, len(row.Scores))
	for id, v := range row.Scores {
		scores[id] = v
	}
	if sc.MakeupPublishedAt == nil {
		if ch.Component.Type.Category == CategoryMakeup {
			return GradeChange{}, ErrMakeupNotPublishedYet
		}
		for _, c := range g.Components {
			if c.Type.Category == CategoryMakeup {
				delete(scores, c.ID)
			}
		}
	}
	if old, ok := scores[in.ComponentID]; ok {
		ch.Old.Absent = old.Absent
		if !old.Absent {
			p := old.Points
			ch.Old.Score = &p
		}
	}
	if !in.Absent && in.Score == nil {
		return GradeChange{}, ErrNoGradeChange
	}
	next := Score{Absent: in.Absent}
	if !in.Absent && in.Score != nil {
		next.Points = *in.Score
	}
	if ch.Old.Absent == in.Absent && (in.Absent || (ch.Old.Score != nil && in.Score != nil && *ch.Old.Score == *in.Score)) {
		return GradeChange{}, ErrNoGradeChange
	}
	scores[in.ComponentID] = next
	res, err := letter.Compute(scale, g.Params, letterInput(g.Components, scores, row.Attendance == AttendanceFail, passFail))
	if err != nil {
		return GradeChange{}, fmt.Errorf("grading: harf notu hesaplanamadı: %w", err)
	}
	if !res.Complete {
		return GradeChange{}, ErrIncompleteGrade
	}
	ch.New = GradeValue{Letter: res.Letter, Reason: string(res.Reason), Absent: in.Absent}
	if res.Weighted != nil {
		w := Points(*res.Weighted)
		ch.New.Weighted = &w
	}
	if !in.Absent {
		p := *in.Score
		ch.New.Score = &p
	}
	return ch, nil
}

// PreviewGradeChange, düzeltmenin sonucunu hesaplar; hiçbir şey kaydedilmez.
func (s *Service) PreviewGradeChange(ctx context.Context, actorID string, in GradeChangeInput) (GradeChange, error) {
	return s.prepareGradeChange(ctx, s.pool, actorID, in)
}

// RequestGradeChange, düzeltmeyi doğrulayıp talep olarak kaydeder ve kimliğini döndürür. Yetki
// (şubenin öğretim elemanı) çağıran tarafta denetlenir.
func (s *Service) RequestGradeChange(ctx context.Context, actorID string, in GradeChangeInput) (string, error) {
	var id string
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		ch, err := s.prepareGradeChange(ctx, tx, actorID, in)
		if err != nil {
			return err
		}
		oldValue, err := json.Marshal(ch.Old)
		if err != nil {
			return fmt.Errorf("grading: not kodlanamadı: %w", err)
		}
		newValue, err := json.Marshal(ch.New)
		if err != nil {
			return fmt.Errorf("grading: not kodlanamadı: %w", err)
		}
		var componentID *string
		if ch.Component != nil {
			componentID = &ch.Component.ID
		}
		err = tx.QueryRow(ctx, `
			INSERT INTO grading.grade_change_requests
				(enrollment_id, section_id, component_id, kind, old_value, new_value, reason, requested_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
			in.EnrollmentID, ch.Section.SectionID, componentID, ch.Kind, oldValue, newValue, in.Reason, actorID).Scan(&id)
		if db.IsConflict(err) {
			return ErrGradeChangePending
		}
		if err != nil {
			return fmt.Errorf("grading: talep kaydedilemedi: %w", err)
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "grade_change.request", EntityType: "grade_change_request", EntityID: id,
			After: map[string]any{"enrollment_id": in.EnrollmentID, "kind": ch.Kind, "from": ch.Old, "to": ch.New, "reason": in.Reason},
		})
	})
	return id, s.changeCounted("requested", err)
}

// changeCounted, işlem başarılıysa not düzeltme talebi olayını sayar.
func (s *Service) changeCounted(outcome string, err error) error {
	if err == nil {
		s.metrics.GradeChange(outcome)
	}
	return err
}

// GradeChangeRequest, bir düzeltme talebidir.
type GradeChangeRequest struct {
	ID                string
	Status            string
	Kind              string
	EnrollmentID      string
	StudentProgramID  string
	StudentID         string
	SectionID         string
	SectionCode       string
	DepartmentID      string
	FacultyID         string
	TermCode          string
	CourseCode        string
	CourseNameTR      string
	CourseNameEN      string
	StudentNo         string
	FirstName         string
	LastName          string
	Component         *Component
	ComponentNumbered bool
	Old, New          GradeValue
	Reason            string
	RequestedBy       string
	RequestedByName   string
	RequestedAt       time.Time
	ReviewedByName    string
	ReviewNote        string
	DecidedAt         *time.Time
}

// GradeChangeFilter, talep listesinin süzgecidir.
type GradeChangeFilter struct {
	ID          string   // tek talep
	RequestedBy string   // sadece bu kullanıcının talepleri
	Statuses    []string // boşsa hepsi
}

const gradeChangeSelect = `
	SELECT r.id, r.status, r.kind, r.enrollment_id, e.student_program_id, st.id, r.section_id, s.section_code,
	       o.department_id, d.faculty_id, t.code, c.code, c.name_tr, c.name_en, st.student_no, pe.first_name, pe.last_name,
	       ac.id, coalesce(at.code, ''), coalesce(at.name_tr, ''), coalesce(at.name_en, ''), coalesce(at.category, ''),
	       coalesce(ac.sequence_no, 0), coalesce(ac.name_tr, ''), coalesce(ac.name_en, ''), coalesce(ac.weight, 0)::float8,
	       coalesce((SELECT count(*) FROM grading.assessment_components x
	                 WHERE x.section_id = ac.section_id AND x.type_code = ac.type_code) > 1, false),
	       r.old_value, r.new_value, r.reason, r.requested_by, r.requested_at,
	       coalesce((SELECT p.first_name || ' ' || p.last_name FROM iam.users u JOIN people.persons p ON p.id = u.person_id WHERE u.id = r.requested_by), ''),
	       coalesce((SELECT p.first_name || ' ' || p.last_name FROM iam.users u JOIN people.persons p ON p.id = u.person_id WHERE u.id = r.reviewed_by), ''),
	       coalesce(r.review_note, ''), r.decided_at
	FROM grading.grade_change_requests r
	JOIN enrollment.enrollments e ON e.id = r.enrollment_id
	JOIN enrollment.student_programs sp ON sp.id = e.student_program_id
	JOIN people.students st ON st.id = sp.student_id
	JOIN people.persons pe ON pe.id = st.person_id
	JOIN offering.sections s ON s.id = r.section_id
	JOIN offering.course_offerings o ON o.id = s.offering_id
	JOIN org.departments d ON d.id = o.department_id
	JOIN academic.terms t ON t.id = o.term_id
	JOIN curriculum.courses c ON c.id = o.course_id
	LEFT JOIN grading.assessment_components ac ON ac.id = r.component_id
	LEFT JOIN grading.assessment_types at ON at.code = ac.type_code`

// GradeChangeRequests, düzeltme taleplerini en yeni önce listeler.
func (s *Service) GradeChangeRequests(ctx context.Context, f GradeChangeFilter) ([]GradeChangeRequest, error) {
	rows, err := s.pool.Query(ctx, gradeChangeSelect+`
		WHERE ($1 = '' OR r.id::text = $1) AND ($2 = '' OR r.requested_by::text = $2)
		  AND (coalesce(cardinality($3::text[]), 0) = 0 OR r.status = ANY($3))
		ORDER BY r.requested_at DESC
		LIMIT 500`, f.ID, f.RequestedBy, f.Statuses)
	if err != nil {
		return nil, fmt.Errorf("grading: düzeltme talepleri okunamadı: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (GradeChangeRequest, error) {
		var (
			r                  GradeChangeRequest
			componentID        *string
			c                  Component
			category           string
			seq                int16
			oldValue, newValue []byte
		)
		err := row.Scan(&r.ID, &r.Status, &r.Kind, &r.EnrollmentID, &r.StudentProgramID, &r.StudentID, &r.SectionID, &r.SectionCode,
			&r.DepartmentID, &r.FacultyID, &r.TermCode, &r.CourseCode, &r.CourseNameTR, &r.CourseNameEN,
			&r.StudentNo, &r.FirstName, &r.LastName, &componentID, &c.Type.Code, &c.Type.NameTR, &c.Type.NameEN,
			&category, &seq, &c.NameTR, &c.NameEN, &c.Weight, &r.ComponentNumbered, &oldValue, &newValue, &r.Reason,
			&r.RequestedBy, &r.RequestedAt, &r.RequestedByName, &r.ReviewedByName, &r.ReviewNote, &r.DecidedAt)
		if err != nil {
			return r, err
		}
		if componentID != nil {
			c.ID, c.Type.Category, c.SequenceNo = *componentID, Category(category), int(seq)
			r.Component = &c
		}
		if err := json.Unmarshal(oldValue, &r.Old); err != nil {
			return r, fmt.Errorf("old_value: %w", err)
		}
		if err := json.Unmarshal(newValue, &r.New); err != nil {
			return r, fmt.Errorf("new_value: %w", err)
		}
		return r, nil
	})
}

// GradeChangeRequestByID, tek bir düzeltme talebidir.
func (s *Service) GradeChangeRequestByID(ctx context.Context, id string) (GradeChangeRequest, error) {
	list, err := s.GradeChangeRequests(ctx, GradeChangeFilter{ID: id})
	if err != nil {
		return GradeChangeRequest{}, err
	}
	if len(list) == 0 {
		return GradeChangeRequest{}, ErrNotFound
	}
	return list[0], nil
}

// lockPendingChange, talebi kilitler ve beklemede olduğunu doğrular.
func lockPendingChange(ctx context.Context, tx pgx.Tx, id string) (requestedBy string, err error) {
	var status string
	err = tx.QueryRow(ctx, `SELECT status, requested_by FROM grading.grade_change_requests WHERE id = $1 FOR UPDATE`, id).
		Scan(&status, &requestedBy)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return "", ErrNotFound
	case err != nil:
		return "", fmt.Errorf("grading: talep kilitlenemedi: %w", err)
	case status != ChangePending:
		return "", ErrGradeChangeNotPending
	}
	return requestedBy, nil
}

func pointsText(p *Points) any {
	if p == nil {
		return nil
	}
	return fmt.Sprintf("%d.%02d", *p/100, *p%100)
}

// ApproveGradeChange, talebi onaylar ve uygular: bileşen notu (SCORE), kesin kaydın harfi ve
// puanı, not geçmişine CORRECTION satırı, sınıf istatistiği; grades.published olayı dönem
// sonuçlarını yeniden hesaplatır. Talep edildiğinden beri not değiştiyse uygulanmaz. Talep eden
// onaylayamaz; onaylayanın kapsamı çağıran tarafta denetlenir.
func (s *Service) ApproveGradeChange(ctx context.Context, actorID string, r GradeChangeRequest, note string) error {
	return s.changeCounted("applied", db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		requestedBy, err := lockPendingChange(ctx, tx, r.ID)
		if err != nil {
			return err
		}
		if requestedBy == actorID {
			return ErrSelfApproval
		}
		var (
			letterGrade, reason, termID, courseID string
			weighted                              *int64
		)
		if err := tx.QueryRow(ctx, `
			SELECT coalesce(letter_grade, ''), (weighted_score * 100)::int8, coalesce(letter_reason, ''), term_id, course_id
			FROM enrollment.enrollments WHERE id = $1 FOR UPDATE`, r.EnrollmentID).
			Scan(&letterGrade, &weighted, &reason, &termID, &courseID); err != nil {
			return fmt.Errorf("grading: kesin kayıt kilitlenemedi: %w", err)
		}
		same := letterGrade == r.Old.Letter && reason == r.Old.Reason &&
			((weighted == nil) == (r.Old.Weighted == nil)) && (weighted == nil || Points(*weighted) == *r.Old.Weighted)
		if r.Kind == ChangeScore {
			var (
				status string
				score  *int64
			)
			err := tx.QueryRow(ctx, `
				SELECT status, (score * 100)::int8 FROM grading.assessment_scores WHERE component_id = $1 AND enrollment_id = $2`,
				r.Component.ID, r.EnrollmentID).Scan(&status, &score)
			switch {
			case errors.Is(err, pgx.ErrNoRows):
				same = same && !r.Old.Absent && r.Old.Score == nil
			case err != nil:
				return fmt.Errorf("grading: bileşen notu okunamadı: %w", err)
			default:
				same = same && (status == "ABSENT") == r.Old.Absent &&
					((score == nil) == (r.Old.Score == nil)) && (score == nil || Points(*score) == *r.Old.Score)
			}
		}
		if !same {
			return ErrGradeChangeStale
		}

		if r.Kind == ChangeScore {
			status := "SCORED"
			if r.New.Absent {
				status = "ABSENT"
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO grading.assessment_scores (component_id, enrollment_id, status, score, entered_by)
				VALUES ($1, $2, $3, $4::numeric, $5)
				ON CONFLICT (component_id, enrollment_id) DO UPDATE
				SET status = excluded.status, score = excluded.score, source = 'MANUAL',
				    entered_by = excluded.entered_by, entered_at = now()`,
				r.Component.ID, r.EnrollmentID, status, pointsText(r.New.Score), nullable(actorID)); err != nil {
				return fmt.Errorf("grading: bileşen notu düzeltilemedi: %w", err)
			}
			if _, err := tx.Exec(ctx, `
				UPDATE grading.assessment_components SET scores_version = scores_version + 1 WHERE id = $1`, r.Component.ID); err != nil {
				return fmt.Errorf("grading: bileşen güncellenemedi: %w", err)
			}
		}
		if _, err := tx.Exec(ctx, `
			UPDATE enrollment.enrollments
			SET letter_grade = $2, weighted_score = $3::numeric, letter_reason = $4, version = version + 1, updated_at = now()
			WHERE id = $1`, r.EnrollmentID, r.New.Letter, pointsText(r.New.Weighted), r.New.Reason); err != nil {
			return fmt.Errorf("grading: düzeltme uygulanamadı: %w", err)
		}
		if err := recordHistory(ctx, tx, r.EnrollmentID, "CORRECTION", r.New.Letter, pointsText(r.New.Weighted), r.New.Reason, actorID); err != nil {
			return err
		}
		if err := RefreshStats(ctx, tx, []string{r.SectionID}); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE grading.grade_change_requests
			SET status = 'APPLIED', reviewed_by = $2, review_note = $3, decided_at = now()
			WHERE id = $1`, r.ID, actorID, nullable(note)); err != nil {
			return fmt.Errorf("grading: talep güncellenemedi: %w", err)
		}
		if err := audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "grade_change.apply", EntityType: "grade_change_request", EntityID: r.ID,
			Before: map[string]any{"letter": r.Old.Letter, "weighted": pointsText(r.Old.Weighted)},
			After:  map[string]any{"letter": r.New.Letter, "weighted": pointsText(r.New.Weighted), "enrollment_id": r.EnrollmentID},
		}); err != nil {
			return err
		}
		return outbox.Publish(ctx, tx, "grades.published", "section", r.SectionID, PublishedEvent{
			Phase: "CORRECTION", SectionID: r.SectionID, TermID: termID, CourseID: courseID,
			Enrollments: []string{r.EnrollmentID}, StudentPrograms: []string{r.StudentProgramID},
		})
	}))
}

// RejectGradeChange, talebi gerekçeyle reddeder.
func (s *Service) RejectGradeChange(ctx context.Context, actorID, id, note string) error {
	return s.changeCounted("rejected", db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		requestedBy, err := lockPendingChange(ctx, tx, id)
		if err != nil {
			return err
		}
		if requestedBy == actorID {
			return ErrSelfApproval
		}
		if _, err := tx.Exec(ctx, `
			UPDATE grading.grade_change_requests
			SET status = 'REJECTED', reviewed_by = $2, review_note = $3, decided_at = now()
			WHERE id = $1`, id, actorID, note); err != nil {
			return fmt.Errorf("grading: talep reddedilemedi: %w", err)
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "grade_change.reject", EntityType: "grade_change_request", EntityID: id,
			After: map[string]any{"note": note},
		})
	}))
}

// WithdrawGradeChange, talep edenin bekleyen talebini geri çekmesidir.
func (s *Service) WithdrawGradeChange(ctx context.Context, actorID, id string) error {
	return s.changeCounted("withdrawn", db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		requestedBy, err := lockPendingChange(ctx, tx, id)
		if err != nil {
			return err
		}
		if requestedBy != actorID {
			return ErrNotFound
		}
		if _, err := tx.Exec(ctx, `
			UPDATE grading.grade_change_requests SET status = 'WITHDRAWN', decided_at = now() WHERE id = $1`, id); err != nil {
			return fmt.Errorf("grading: talep geri çekilemedi: %w", err)
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "grade_change.withdraw", EntityType: "grade_change_request", EntityID: id,
		})
	}))
}
