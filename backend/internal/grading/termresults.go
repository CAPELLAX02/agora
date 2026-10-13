package grading

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/grading/gpa"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
	"github.com/CAPELLAX02/agora/backend/internal/platform/outbox"
)

// TermResultsUpdater, not ilanında (grades.published) öğrencilerin dönem sonuçlarını yeniden
// hesaplayan outbox tüketicisidir. Sonuçlar kesin kayıtlardan baştan üretildiği için olay
// birden fazla kez işlense de sonuç aynıdır.
type TermResultsUpdater struct {
	Now func() time.Time // yönetmelik parametrelerinin geçerlilik günü; nil ise time.Now
}

// Name, tüketicinin kalıcı adıdır.
func (TermResultsUpdater) Name() string { return "term-results" }

// Handles, not ilanı olayını işler.
func (TermResultsUpdater) Handles(eventType string) bool { return eventType == "grades.published" }

// Handle, olaydaki öğrencilerin dönem sonuçlarını yeniden hesaplar.
func (u TermResultsUpdater) Handle(ctx context.Context, tx pgx.Tx, e outbox.Event) error {
	var ev PublishedEvent
	if err := e.Decode(&ev); err != nil {
		return fmt.Errorf("grading: olay verisi çözülemedi: %w", err)
	}
	now := time.Now
	if u.Now != nil {
		now = u.Now
	}
	return RecomputeTermResults(ctx, tx, ev.StudentPrograms, now())
}

// loadGPARules, onur ve ders yükü parametreleridir (at gününde geçerli olanlar). Tanımlı
// değilse yönetmeliğin genel değerleri: 30 AKTS, onur 3,00, yüksek onur 3,50.
func loadGPARules(ctx context.Context, q db.Querier, at time.Time) (gpa.Rules, error) {
	r := gpa.Rules{FullLoad: 300, Honor: 300, HighHonor: 350}
	rows, err := q.Query(ctx, `
		SELECT key, value FROM curriculum.regulation_parameters
		WHERE key IN ('semester_ects_load', 'honor_gpa')
		  AND effective_from <= $1 AND (effective_to IS NULL OR effective_to > $1)`, at)
	if err != nil {
		return r, fmt.Errorf("grading: yönetmelik parametreleri okunamadı: %w", err)
	}
	var (
		key   string
		value []byte
	)
	_, err = pgx.ForEachRow(rows, []any{&key, &value}, func() error {
		switch key {
		case "semester_ects_load":
			var v float64
			if err := json.Unmarshal(value, &v); err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
			r.FullLoad = gpa.ECTS(cents(v) / 10)
		case "honor_gpa":
			var v struct {
				Honor     float64 `json:"HONOR"`
				HighHonor float64 `json:"HIGH_HONOR"`
			}
			if err := json.Unmarshal(value, &v); err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
			r.Honor, r.HighHonor = gpa.Hundredths(cents(v.Honor)), gpa.Hundredths(cents(v.HighHonor))
		}
		return nil
	})
	if err != nil {
		return r, fmt.Errorf("grading: yönetmelik parametreleri okunamadı: %w", err)
	}
	return r, nil
}

// RecomputeTermResults, program kayıtlarının dönem sonuçlarını kesin kayıtlardan yeniden
// üretir, artık notu kalmayan dönemlerin sonucunu siler ve GANO ile kazanılan AKTS
// önbelleklerini (son dönemin sonucu) günceller. Önbellek sadece değiştiyse yazılır: kaydın
// sürümü boş yere artmaz. at, yönetmelik parametrelerinin geçerlilik günüdür.
func RecomputeTermResults(ctx context.Context, q db.Querier, studentProgramIDs []string, at time.Time) error {
	if len(studentProgramIDs) == 0 {
		return nil
	}
	rules, err := loadGPARules(ctx, q, at)
	if err != nil {
		return err
	}

	// Dönemler: dönem sırası başlangıç gününden; disiplin cezası, dönem içinde konmuş
	// uzaklaştırma engelidir.
	terms := map[string]map[string]gpa.Term{}
	rows, err := q.Query(ctx, `
		SELECT DISTINCT e.student_program_id, t.id, t.starts_on - date '2000-01-01', t.term_type IN ('FALL', 'SPRING'),
		       EXISTS (SELECT 1 FROM enrollment.holds h
		               WHERE h.student_program_id = e.student_program_id AND h.hold_type = 'DISCIPLINE_SUSPENSION'
		                 AND h.placed_at::date BETWEEN t.starts_on AND t.ends_on)
		FROM enrollment.enrollments e JOIN academic.terms t ON t.id = e.term_id
		WHERE e.student_program_id = ANY($1) AND e.status <> 'DROPPED'`, studentProgramIDs)
	if err != nil {
		return fmt.Errorf("grading: dönemler okunamadı: %w", err)
	}
	var (
		spID string
		t    gpa.Term
	)
	_, err = pgx.ForEachRow(rows, []any{&spID, &t.ID, &t.Order, &t.Regular, &t.Discipline}, func() error {
		if terms[spID] == nil {
			terms[spID] = map[string]gpa.Term{}
		}
		terms[spID][t.ID] = t
		return nil
	})
	if err != nil {
		return fmt.Errorf("grading: dönemler okunamadı: %w", err)
	}

	attempts := map[string][]gpa.Attempt{}
	rows, err = q.Query(ctx, `
		SELECT e.student_program_id, e.term_id, e.status = 'COMPLETED', (e.ects_snapshot * 10)::int8,
		       coalesce((gi.coefficient * 100)::int8, 0), coalesce(gi.counts_in_gpa, false),
		       coalesce(gi.earns_ects, false), coalesce(gi.is_passing, false),
		       EXISTS (SELECT 1 FROM grading.grade_history gh WHERE gh.enrollment_id = e.id AND gh.kind = 'MAKEUP'),
		       coalesce(n.term_id::text, '')
		FROM enrollment.enrollments e
		LEFT JOIN curriculum.grade_scale_items gi ON gi.grade_scale_id = e.grade_scale_id AND gi.letter = e.letter_grade
		LEFT JOIN enrollment.enrollments n ON n.id = e.superseded_by_id
		WHERE e.student_program_id = ANY($1) AND e.status <> 'DROPPED'`, studentProgramIDs)
	if err != nil {
		return fmt.Errorf("grading: ders geçmişi okunamadı: %w", err)
	}
	var (
		a          gpa.Attempt
		ects, coef int64
	)
	_, err = pgx.ForEachRow(rows, []any{&spID, &a.TermID, &a.Completed, &ects, &coef, &a.CountsInGPA,
		&a.EarnsECTS, &a.Passing, &a.ViaMakeup, &a.SupersededIn}, func() error {
		a.ECTS, a.Coefficient = gpa.ECTS(ects), gpa.Hundredths(coef)
		attempts[spID] = append(attempts[spID], a)
		return nil
	})
	if err != nil {
		return fmt.Errorf("grading: ders geçmişi okunamadı: %w", err)
	}

	tenths := func(v gpa.ECTS) string { return fmt.Sprintf("%d.%d", v/10, v%10) }
	hundredths := func(v *gpa.Hundredths) *string {
		if v == nil {
			return nil
		}
		s := fmt.Sprintf("%d.%02d", *v/100, *v%100)
		return &s
	}
	var (
		sps, termIDs, standings           []string
		attempted, earned, cumulativeECTS []string
		termGPAs, cumulativeGPAs          []*string
		cacheSPs, cacheECTS               []string
		cacheGPAs                         []*string
	)
	for _, id := range studentProgramIDs {
		list := make([]gpa.Term, 0, len(terms[id]))
		for _, t := range terms[id] {
			list = append(list, t)
		}
		results := gpa.Compute(list, attempts[id], rules)
		for _, r := range results {
			sps, termIDs, standings = append(sps, id), append(termIDs, r.TermID), append(standings, string(r.Standing))
			attempted, earned = append(attempted, tenths(r.AttemptedECTS)), append(earned, tenths(r.EarnedECTS))
			cumulativeECTS = append(cumulativeECTS, tenths(r.CumulativeECTS))
			termGPAs, cumulativeGPAs = append(termGPAs, hundredths(r.TermGPA)), append(cumulativeGPAs, hundredths(r.CumulativeGPA))
		}
		last, _ := gpa.Latest(results)
		cacheSPs = append(cacheSPs, id)
		cacheECTS = append(cacheECTS, tenths(last.CumulativeECTS))
		cacheGPAs = append(cacheGPAs, hundredths(last.CumulativeGPA))
	}

	if _, err := q.Exec(ctx, `
		DELETE FROM enrollment.term_results r
		WHERE r.student_program_id = ANY($1)
		  AND NOT EXISTS (SELECT 1 FROM unnest($2::uuid[], $3::uuid[]) AS k (sp, term)
		                  WHERE k.sp = r.student_program_id AND k.term = r.term_id)`,
		studentProgramIDs, nonNil(sps), nonNil(termIDs)); err != nil {
		return fmt.Errorf("grading: eski dönem sonuçları silinemedi: %w", err)
	}
	if len(sps) > 0 {
		if _, err := q.Exec(ctx, `
			INSERT INTO enrollment.term_results (student_program_id, term_id, attempted_ects, earned_ects, term_gpa,
				cumulative_ects, cumulative_gpa, standing, computed_at)
			SELECT sp, term, attempted::numeric, earned::numeric, term_gpa::numeric, cum_ects::numeric, cum_gpa::numeric,
			       standing, now()
			FROM unnest($1::uuid[], $2::uuid[], $3::text[], $4::text[], $5::text[], $6::text[], $7::text[], $8::text[])
				AS x (sp, term, attempted, earned, term_gpa, cum_ects, cum_gpa, standing)
			ON CONFLICT (student_program_id, term_id) DO UPDATE SET
				attempted_ects = excluded.attempted_ects, earned_ects = excluded.earned_ects, term_gpa = excluded.term_gpa,
				cumulative_ects = excluded.cumulative_ects, cumulative_gpa = excluded.cumulative_gpa,
				standing = excluded.standing, computed_at = now()`,
			sps, termIDs, attempted, earned, termGPAs, cumulativeECTS, cumulativeGPAs, standings); err != nil {
			return fmt.Errorf("grading: dönem sonuçları yazılamadı: %w", err)
		}
	}
	if _, err := q.Exec(ctx, `
		UPDATE enrollment.student_programs sp
		SET gpa_cache = x.gpa::numeric, earned_ects_cache = x.ects::numeric, version = sp.version + 1, updated_at = now()
		FROM unnest($1::uuid[], $2::text[], $3::text[]) AS x (id, gpa, ects)
		WHERE sp.id = x.id
		  AND (sp.gpa_cache IS DISTINCT FROM x.gpa::numeric OR sp.earned_ects_cache <> x.ects::numeric)`,
		cacheSPs, cacheGPAs, cacheECTS); err != nil {
		return fmt.Errorf("grading: GANO önbelleği güncellenemedi: %w", err)
	}
	return nil
}

// nonNil, pgx'in boş dilimi NULL değil boş dizi olarak göndermesi içindir.
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
