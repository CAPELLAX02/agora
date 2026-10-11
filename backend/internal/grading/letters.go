package grading

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/grading/letter"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
	"github.com/CAPELLAX02/agora/backend/internal/platform/outbox"
)

// Harf notu hataları.
var (
	ErrGradesPublished      = errors.New("grading: harf notları ilan edildi")
	ErrPublishWindowClosed  = errors.New("grading: ilan penceresi kapalı")
	ErrOverrideWindowClosed = errors.New("grading: elle harf notu atama penceresi kapalı")
	ErrLetterNotAllowed     = errors.New("grading: bu harf elle verilemez")
)

// IncompleteError: notu eksik (ve elle harf notu verilmemiş) öğrenciler varken ilan edilemez.
type IncompleteError struct {
	Students []string // "Ad Soyad (numara)"
}

func (e *IncompleteError) Error() string {
	return fmt.Sprintf("grading: %d öğrencinin notu eksik", len(e.Students))
}

// WindowManualLetter, elle harf notu atama penceresidir. İlan, final not girişi ya da bu
// pencere açıkken yapılır.
const WindowManualLetter = "MANUAL_LETTER_GRADE"

// ExemptionLetter, muafiyet notudur: ders içi değerlendirmeyle verilmez.
const ExemptionLetter = "MUAF"

// Override, elle verilmiş harf notudur.
type Override struct {
	Letter string
	Reason string
	SetBy  string
	SetAt  time.Time
}

// LetterRow, bir öğrencinin harf notu durumudur.
type LetterRow struct {
	GradebookStudent
	ScaleID  string
	Result   letter.Result
	Missing  []string // notu girilmemiş bileşenlerin kimlikleri
	Override *Override
	// Published*: ilan edilmiş not (ilandan sonra).
	PublishedLetter   string
	PublishedWeighted *Points
	PublishedReason   string
}

// Letter, geçerli harftir: ilan edilmişse ilan edilen, yoksa elle verilen, yoksa hesaplanan.
func (r LetterRow) Letter() string {
	switch {
	case r.PublishedLetter != "":
		return r.PublishedLetter
	case r.Override != nil:
		return r.Override.Letter
	case r.Result.Complete:
		return r.Result.Letter
	}
	return ""
}

// Weighted, geçerli ders başarı puanıdır.
func (r LetterRow) Weighted() *Points {
	if r.PublishedLetter != "" {
		return r.PublishedWeighted
	}
	if r.Result.Weighted == nil {
		return nil
	}
	p := Points(*r.Result.Weighted)
	return &p
}

// Reason, geçerli harfin kuralıdır.
func (r LetterRow) Reason() string {
	switch {
	case r.PublishedLetter != "":
		return r.PublishedReason
	case r.Override != nil:
		return "MANUAL"
	case r.Result.Complete:
		return string(r.Result.Reason)
	}
	return ""
}

// LetterGrades, şubenin harf notlarıdır (önizleme ya da ilan edilmiş).
type LetterGrades struct {
	SectionContext
	PublishedAt    *time.Time
	PublishedBy    string
	PublishWindow  EntryWindow
	OverrideWindow EntryWindow
	Params         letter.Params
	Scales         map[string]letter.Scale
	DefaultScaleID string
	Rows           []LetterRow
}

// Scale, şubenin (varsayılan) not ölçeğidir: öğrencilerin çoğu aynı ölçekle kayıtlıdır.
func (g LetterGrades) Scale() letter.Scale {
	return g.Scales[g.DefaultScaleID]
}

// Ready, bütün öğrencilerin harfi belli mi (hesaplanabilir ya da elle verilmiş)?
func (g LetterGrades) Ready() bool {
	return !slices.ContainsFunc(g.Rows, func(r LetterRow) bool { return r.Letter() == "" })
}

// publishWindow, ilanın yapılabildiği pencereler: final not girişi ya da elle harf notu atama.
func (s *Service) publishWindow(ctx context.Context, sc SectionContext) (EntryWindow, error) {
	w, err := s.window(ctx, sc, CategoryFinal)
	if err != nil || w.Open {
		return w, err
	}
	m, err := s.windowOf(ctx, sc, WindowManualLetter)
	if err != nil {
		return EntryWindow{}, err
	}
	if m.Open || (m.NextStartsAt != nil && (w.NextStartsAt == nil || m.NextStartsAt.Before(*w.NextStartsAt))) {
		return m, nil
	}
	return w, nil
}

// LetterGrades, şubenin harf notlarını hesaplar. Transaction dışında çağrılır.
func (s *Service) LetterGrades(ctx context.Context, sectionID, viewerID string) (LetterGrades, error) {
	sc, err := s.repo.sectionContext(ctx, s.pool, sectionID, viewerID, false)
	if err != nil {
		return LetterGrades{}, err
	}
	g, err := s.letterGrades(ctx, s.pool, sc)
	if err != nil {
		return LetterGrades{}, err
	}
	if g.PublishWindow, err = s.publishWindow(ctx, sc); err != nil {
		return LetterGrades{}, err
	}
	if g.OverrideWindow, err = s.windowOf(ctx, sc, WindowManualLetter); err != nil {
		return LetterGrades{}, err
	}
	return g, nil
}

// letterGrades, q üzerinden (transaction içinde ya da dışında) harf notlarını hesaplar.
func (s *Service) letterGrades(ctx context.Context, q db.Querier, sc SectionContext) (LetterGrades, error) {
	g := LetterGrades{SectionContext: sc, Scales: map[string]letter.Scale{}}
	if err := q.QueryRow(ctx, `
		SELECT s.grades_published_at,
		       coalesce((SELECT pe.first_name || ' ' || pe.last_name FROM iam.users u JOIN people.persons pe ON pe.id = u.person_id
		                 WHERE u.id = s.grades_published_by), ''),
		       (SELECT id FROM curriculum.grade_scales WHERE is_default)
		FROM offering.sections s WHERE s.id = $1`, sc.SectionID).Scan(&g.PublishedAt, &g.PublishedBy, &g.DefaultScaleID); err != nil {
		return LetterGrades{}, fmt.Errorf("grading: şube okunamadı: %w", err)
	}
	var err error
	if g.Params, err = loadParams(ctx, q, s.now()); err != nil {
		return LetterGrades{}, err
	}
	components, err := s.repo.components(ctx, q, sc.SectionID)
	if err != nil {
		return LetterGrades{}, err
	}
	students, err := s.gradebookStudentsQ(ctx, q, sc.SectionID)
	if err != nil {
		return LetterGrades{}, err
	}

	rows, err := q.Query(ctx, `
		SELECT e.id, coalesce(e.grade_scale_id, $2), coalesce(e.letter_grade, ''),
		       (e.weighted_score * 100)::int8, coalesce(e.letter_reason, ''),
		       o.letter, o.reason, o.set_at,
		       coalesce((SELECT pe.first_name || ' ' || pe.last_name FROM iam.users u JOIN people.persons pe ON pe.id = u.person_id
		                 WHERE u.id = o.set_by), '')
		FROM enrollment.enrollments e
		LEFT JOIN grading.letter_overrides o ON o.enrollment_id = e.id
		WHERE e.section_id = $1 AND e.status <> 'DROPPED'`, sc.SectionID, g.DefaultScaleID)
	if err != nil {
		return LetterGrades{}, fmt.Errorf("grading: harf notları okunamadı: %w", err)
	}
	type extra struct {
		scaleID, published, reason string
		weighted                   *int64
		override                   *Override
	}
	extras := map[string]extra{}
	for rows.Next() {
		var (
			id          string
			x           extra
			oL, oR, oBy *string
			oAt         *time.Time
		)
		if err := rows.Scan(&id, &x.scaleID, &x.published, &x.weighted, &x.reason, &oL, &oR, &oAt, &oBy); err != nil {
			rows.Close()
			return LetterGrades{}, fmt.Errorf("grading: harf notları okunamadı: %w", err)
		}
		if oL != nil {
			x.override = &Override{Letter: *oL, Reason: *oR, SetAt: *oAt, SetBy: *oBy}
		}
		extras[id] = x
	}
	if err := rows.Err(); err != nil {
		return LetterGrades{}, fmt.Errorf("grading: harf notları okunamadı: %w", err)
	}

	for _, st := range students {
		x := extras[st.EnrollmentID]
		if _, ok := g.Scales[x.scaleID]; !ok {
			if g.Scales[x.scaleID], err = loadScale(ctx, q, x.scaleID); err != nil {
				return LetterGrades{}, err
			}
		}
		row := LetterRow{GradebookStudent: st, ScaleID: x.scaleID, Override: x.override, PublishedLetter: x.published, PublishedReason: x.reason}
		if x.weighted != nil {
			p := Points(*x.weighted)
			row.PublishedWeighted = &p
		}
		in := letter.Input{AttendanceFail: st.Attendance == AttendanceFail, PassFail: sc.GradingMode == "PASS_FAIL"}
		for _, c := range components {
			comp := letter.Component{Kind: kindOf(c.Type.Category), Weight: letter.Points(cents(c.Weight))}
			if score, ok := st.Scores[c.ID]; ok {
				if score.Absent {
					comp.Absent = true
				} else {
					p := letter.Points(score.Points)
					comp.Score = &p
				}
			}
			in.Components = append(in.Components, comp)
		}
		res, err := letter.Compute(g.Scales[x.scaleID], g.Params, in)
		if err != nil {
			return LetterGrades{}, fmt.Errorf("grading: %s harf notu hesaplanamadı: %w", st.StudentNo, err)
		}
		row.Result = res
		for _, i := range res.Missing {
			row.Missing = append(row.Missing, components[i].ID)
		}
		g.Rows = append(g.Rows, row)
	}
	return g, nil
}

func kindOf(c Category) letter.Kind {
	switch c {
	case CategoryFinal:
		return letter.Final
	case CategoryMakeup:
		return letter.Makeup
	}
	return letter.InTerm
}

// loadParams, harf notu hesabının yönetmelik parametreleridir (at gününde geçerli olanlar).
// Tanımlı değilse yönetmeliğin genel değeri: final barajı ve geçme puanı 50.
func loadParams(ctx context.Context, q db.Querier, at time.Time) (letter.Params, error) {
	p := letter.Params{FinalMinScore: 5000, PassMinScore: 5000}
	rows, err := q.Query(ctx, `
		SELECT key, value FROM curriculum.regulation_parameters
		WHERE key IN ('final_min_score', 'pass_min_score')
		  AND effective_from <= $1 AND (effective_to IS NULL OR effective_to > $1)`, at)
	if err != nil {
		return p, fmt.Errorf("grading: yönetmelik parametreleri okunamadı: %w", err)
	}
	var (
		key   string
		value []byte
	)
	_, err = pgx.ForEachRow(rows, []any{&key, &value}, func() error {
		var v float64
		if err := json.Unmarshal(value, &v); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		switch key {
		case "final_min_score":
			p.FinalMinScore = letter.Points(cents(v))
		case "pass_min_score":
			p.PassMinScore = letter.Points(cents(v))
		}
		return nil
	})
	if err != nil {
		return p, fmt.Errorf("grading: yönetmelik parametreleri okunamadı: %w", err)
	}
	return p, nil
}

// loadScale, not ölçeğini başarıdan başarısızlığa sıralı okur.
func loadScale(ctx context.Context, q db.Querier, scaleID string) (letter.Scale, error) {
	rows, err := q.Query(ctx, `
		SELECT letter, (coefficient * 100)::int8, (min_score * 100)::int8, (max_score * 100)::int8,
		       is_passing, counts_in_gpa, earns_ects, is_attendance_fail
		FROM curriculum.grade_scale_items WHERE grade_scale_id = $1 ORDER BY sort_order`, scaleID)
	if err != nil {
		return nil, fmt.Errorf("grading: not ölçeği okunamadı: %w", err)
	}
	ptr := func(v *int64) *letter.Points {
		if v == nil {
			return nil
		}
		p := letter.Points(*v)
		return &p
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (letter.Grade, error) {
		var (
			g            letter.Grade
			coef, lo, hi *int64
		)
		err := row.Scan(&g.Letter, &coef, &lo, &hi, &g.Passing, &g.CountsInGPA, &g.EarnsECTS, &g.AttendanceFail)
		g.Coefficient, g.Min, g.Max = ptr(coef), ptr(lo), ptr(hi)
		return g, err
	})
}

// ManualLetters, derse elle verilebilecek harflerdir: harf notlu derste puanla verilen harfler
// ve devamsızlık harfi, başarılı/başarısız notlu derste geçti/kaldı harfleri. Muafiyet ders
// içi değerlendirmeyle verilmez.
func ManualLetters(scale letter.Scale, passFail bool) letter.Scale {
	var out letter.Scale
	for _, g := range scale {
		switch {
		case g.Letter == ExemptionLetter:
		case passFail && g.Min == nil && !g.CountsInGPA && !g.AttendanceFail:
			out = append(out, g)
		case !passFail && (g.Min != nil || g.AttendanceFail):
			out = append(out, g)
		}
	}
	return out
}

// OverrideInput, bir öğrenciye elle verilecek harftir; Letter boşsa elle verilen not kalkar.
type OverrideInput struct {
	EnrollmentID string
	Letter       string
	Reason       string
}

// lockSection, şubeyi kilitler ve harf notlarının ilan edilmediğini doğrular. Not ve devam
// yazmaları paylaşımlı (FOR SHARE), ilan özel (FOR UPDATE) kilit alır: ilanla eşzamanlı bir
// not yazımı ya ilandan önce biter ya da ilanı görüp reddedilir.
func lockSection(ctx context.Context, tx pgx.Tx, sectionID string, exclusive bool) error {
	query := `SELECT grades_published_at FROM offering.sections WHERE id = $1 FOR SHARE`
	if exclusive {
		query = `SELECT grades_published_at FROM offering.sections WHERE id = $1 FOR UPDATE`
	}
	var published *time.Time
	if err := tx.QueryRow(ctx, query, sectionID).Scan(&published); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("grading: şube kilitlenemedi: %w", err)
	}
	if published != nil {
		return ErrGradesPublished
	}
	return nil
}

// SetOverrides, elle harf notlarını yazar ya da kaldırır. Elle harf notu atama penceresi açık
// ve notlar ilan edilmemiş olmalıdır.
func (s *Service) SetOverrides(ctx context.Context, actorID string, sc SectionContext, items []OverrideInput) error {
	w, err := s.windowOf(ctx, sc, WindowManualLetter)
	if err != nil {
		return err
	}
	if !w.Open {
		return ErrOverrideWindowClosed
	}
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := lockSection(ctx, tx, sc.SectionID, false); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT e.id, coalesce(e.grade_scale_id, (SELECT id FROM curriculum.grade_scales WHERE is_default)), o.letter
			FROM enrollment.enrollments e LEFT JOIN grading.letter_overrides o ON o.enrollment_id = e.id
			WHERE e.section_id = $1 AND e.status <> 'DROPPED'`, sc.SectionID)
		if err != nil {
			return fmt.Errorf("grading: şube listesi okunamadı: %w", err)
		}
		type current struct {
			scaleID string
			letter  *string
		}
		known := map[string]current{}
		for rows.Next() {
			var (
				id string
				c  current
			)
			if err := rows.Scan(&id, &c.scaleID, &c.letter); err != nil {
				rows.Close()
				return fmt.Errorf("grading: şube listesi okunamadı: %w", err)
			}
			known[id] = c
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("grading: şube listesi okunamadı: %w", err)
		}
		var unknown []string
		for _, it := range items {
			if _, ok := known[it.EnrollmentID]; !ok {
				unknown = append(unknown, it.EnrollmentID)
			}
		}
		if len(unknown) > 0 {
			return &UnknownEnrollmentsError{IDs: unknown}
		}

		scales := map[string]letter.Scale{}
		var changes []map[string]any
		for _, it := range items {
			c := known[it.EnrollmentID]
			if it.Letter == "" {
				if c.letter == nil {
					continue
				}
				if _, err := tx.Exec(ctx, `DELETE FROM grading.letter_overrides WHERE enrollment_id = $1`, it.EnrollmentID); err != nil {
					return fmt.Errorf("grading: elle verilen not kaldırılamadı: %w", err)
				}
				changes = append(changes, map[string]any{"enrollment_id": it.EnrollmentID, "from": *c.letter, "to": nil})
				continue
			}
			scale, ok := scales[c.scaleID]
			if !ok {
				if scale, err = loadScale(ctx, tx, c.scaleID); err != nil {
					return err
				}
				scales[c.scaleID] = scale
			}
			if _, ok := ManualLetters(scale, sc.GradingMode == "PASS_FAIL").Find(it.Letter); !ok {
				return fmt.Errorf("%w: %s", ErrLetterNotAllowed, it.Letter)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO grading.letter_overrides (enrollment_id, letter, reason, set_by) VALUES ($1, $2, $3, $4)
				ON CONFLICT (enrollment_id) DO UPDATE
				SET letter = excluded.letter, reason = excluded.reason, set_by = excluded.set_by, set_at = now()`,
				it.EnrollmentID, it.Letter, it.Reason, nullable(actorID)); err != nil {
				return fmt.Errorf("grading: elle harf notu yazılamadı: %w", err)
			}
			change := map[string]any{"enrollment_id": it.EnrollmentID, "to": it.Letter, "reason": it.Reason}
			if c.letter != nil {
				change["from"] = *c.letter
			}
			changes = append(changes, change)
		}
		if len(changes) == 0 {
			return nil
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "letter_override.set", EntityType: "section", EntityID: sc.SectionID,
			After: map[string]any{"changes": changes},
		})
	})
}

// PublishedEvent, grades.published olayının verisidir: dönem sonuçları ve sınıf
// istatistikleri bu olayla yeniden hesaplanır.
type PublishedEvent struct {
	SectionID       string   `json:"section_id"`
	TermID          string   `json:"term_id"`
	CourseID        string   `json:"course_id"`
	Enrollments     []string `json:"enrollments"`
	StudentPrograms []string `json:"student_programs"`
}

// PublishGrades, harf notlarını ilan eder: kesin kayıtlar tamamlanır, tekrar alınan derste önceki
// deneme kapanır, grades.published olayı yayımlanır. İlan edilen notların sayısını döndürür.
func (s *Service) PublishGrades(ctx context.Context, actorID string, sc SectionContext) (int, error) {
	if sc.LockedAt == nil {
		return 0, ErrPlanNotLocked
	}
	w, err := s.publishWindow(ctx, sc)
	if err != nil {
		return 0, err
	}
	if !w.Open {
		return 0, ErrPublishWindowClosed
	}
	count := 0
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := lockSection(ctx, tx, sc.SectionID, true); err != nil {
			return err
		}
		g, err := s.letterGrades(ctx, tx, sc)
		if err != nil {
			return err
		}
		var incomplete []string
		for _, r := range g.Rows {
			if r.Letter() == "" {
				incomplete = append(incomplete, fmt.Sprintf("%s %s (%s)", r.FirstName, r.LastName, r.StudentNo))
			}
		}
		if len(incomplete) > 0 {
			return &IncompleteError{Students: incomplete}
		}

		ev := PublishedEvent{SectionID: sc.SectionID, TermID: sc.TermID, CourseID: sc.CourseID}
		distribution := map[string]int{}
		for _, r := range g.Rows {
			var weighted any
			if w := r.Weighted(); w != nil {
				weighted = fmt.Sprintf("%d.%02d", *w/100, *w%100)
			}
			var spID string
			if err := tx.QueryRow(ctx, `
				UPDATE enrollment.enrollments
				SET status = 'COMPLETED', letter_grade = $2, grade_scale_id = $3, weighted_score = $4::numeric,
				    letter_reason = $5, grade_published_at = now(), version = version + 1, updated_at = now()
				WHERE id = $1 AND status = 'ENROLLED'
				RETURNING student_program_id`,
				r.EnrollmentID, r.Letter(), r.ScaleID, weighted, r.Reason()).Scan(&spID); err != nil {
				return fmt.Errorf("grading: %s notu yazılamadı: %w", r.StudentNo, err)
			}
			// Son not geçerli: aynı dersin ya da eşdeğerinin önceki dönemlerdeki denemesi kapanır.
			if _, err := tx.Exec(ctx, `
				UPDATE enrollment.enrollments old
				SET superseded_by_id = $1, version = old.version + 1, updated_at = now()
				FROM academic.terms ot, academic.terms nt
				WHERE old.student_program_id = $2 AND old.id <> $1 AND old.status = 'COMPLETED'
				  AND old.superseded_by_id IS NULL
				  AND ot.id = old.term_id AND nt.id = $3 AND ot.starts_on < nt.starts_on
				  AND (old.course_id = $4 OR old.course_id IN (
				        -- course_id yeni, equivalent_course_id eski ders: yeni ders eskinin yerine
				        -- her zaman, eski ders yeninin yerine sadece çift yönlü eşdeğerlikte sayılır.
				        SELECT equivalent_course_id FROM curriculum.course_equivalences WHERE course_id = $4
				        UNION SELECT course_id FROM curriculum.course_equivalences
				        WHERE equivalent_course_id = $4 AND is_bidirectional))`,
				r.EnrollmentID, spID, sc.TermID, sc.CourseID); err != nil {
				return fmt.Errorf("grading: önceki deneme kapatılamadı: %w", err)
			}
			ev.Enrollments = append(ev.Enrollments, r.EnrollmentID)
			ev.StudentPrograms = append(ev.StudentPrograms, spID)
			distribution[r.Letter()]++
		}
		if _, err := tx.Exec(ctx, `
			UPDATE offering.sections SET grades_published_at = now(), grades_published_by = $2 WHERE id = $1`,
			sc.SectionID, nullable(actorID)); err != nil {
			return fmt.Errorf("grading: ilan kaydedilemedi: %w", err)
		}
		count = len(g.Rows)
		sort.Strings(ev.StudentPrograms)
		if err := audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "grades.publish", EntityType: "section", EntityID: sc.SectionID,
			After: map[string]any{"students": count, "distribution": distribution},
		}); err != nil {
			return err
		}
		return outbox.Publish(ctx, tx, "grades.published", "section", sc.SectionID, ev)
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}
