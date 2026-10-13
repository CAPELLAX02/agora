package grading

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	"github.com/CAPELLAX02/agora/backend/internal/grading/gpa"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// RecomputeOptions, toplu yeniden hesabın ayarlarıdır.
type RecomputeOptions struct {
	Apply   bool // farkı olan öğrencilerin sonuçlarını yaz; false ise sadece karşılaştır
	Workers int  // aynı anda işlenen grup sayısı
	Batch   int  // bir grupta hesaplanan öğrenci sayısı
}

// RecomputeReport, toplu yeniden hesabın sonucudur.
type RecomputeReport struct {
	Students int // notu ya da kayıtlı dönem sonucu olan program kaydı
	Results  int // hesaplanan dönem sonucu
	// Drifted, kayıtlı dönem sonuçları ya da GANO önbelleği hesaplanandan farklı olan
	// öğrencilerdir; Apply ise bunların sonuçları yazıldı.
	Drifted      int
	RowsMissing  int // hesaplanıp kayıtlı olmayan dönem sonucu
	RowsExtra    int // kayıtlı olup artık notu olmayan dönem sonucu
	RowsChanged  int // değerleri farklı dönem sonucu
	CacheChanged int // GANO ya da kazanılan AKTS önbelleği farklı öğrenci
	// Orphans, notu olmadığı hâlde GANO önbelleği dolu kayıtlardır (eski sistemden geçmişi
	// aktarılmamış öğrenciler, sentetik veri). Bunlara dokunulmaz, sadece sayılır.
	Orphans  int
	Examples []string // farkı olan ilk öğrencilerin numaraları
	Applied  bool
	Duration time.Duration
}

const maxExamples = 10

type storedResult struct {
	attempted, earned, cumulativeECTS gpa.ECTS
	termGPA, cumulativeGPA            *gpa.Hundredths
	standing                          gpa.Standing
}

type storedProgram struct {
	studentNo string
	results   map[string]storedResult
	cacheGPA  *gpa.Hundredths
	cacheECTS gpa.ECTS
}

// RecomputeAll, notu olan bütün öğrencilerin dönem sonuçlarını kesin kayıtlardan yeniden
// hesaplar ve kayıtlı sonuçlarla, GANO önbelleğiyle karşılaştırır. Öğrenciler gruplara bölünür,
// gruplar paralel işlenir; her grup kendi transaction'ındadır. Apply ise sadece farkı olan
// öğrencilerin sonuçları yazılır: tutarlı veride tekrar çalıştırmak bir şey değiştirmez.
func RecomputeAll(ctx context.Context, pool *pgxpool.Pool, opts RecomputeOptions) (RecomputeReport, error) {
	start := time.Now()
	opts.Workers, opts.Batch = max(opts.Workers, 1), max(opts.Batch, 1)
	report := RecomputeReport{Applied: opts.Apply}

	rows, err := pool.Query(ctx, `
		SELECT sp.id FROM enrollment.student_programs sp
		WHERE EXISTS (SELECT 1 FROM enrollment.enrollments e WHERE e.student_program_id = sp.id AND e.status = 'COMPLETED')
		   OR EXISTS (SELECT 1 FROM enrollment.term_results r WHERE r.student_program_id = sp.id)
		ORDER BY sp.id`)
	if err != nil {
		return report, fmt.Errorf("grading: öğrenciler okunamadı: %w", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return report, fmt.Errorf("grading: öğrenciler okunamadı: %w", err)
	}
	report.Students = len(ids)
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM enrollment.student_programs sp
		WHERE sp.gpa_cache IS NOT NULL
		  AND NOT EXISTS (SELECT 1 FROM enrollment.enrollments e WHERE e.student_program_id = sp.id AND e.status = 'COMPLETED')`).
		Scan(&report.Orphans); err != nil {
		return report, fmt.Errorf("grading: önbellekler okunamadı: %w", err)
	}

	var mu sync.Mutex
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(opts.Workers)
	for batch := range slices.Chunk(ids, opts.Batch) {
		g.Go(func() error {
			part, err := recomputeBatch(ctx, pool, batch, opts.Apply)
			if err != nil {
				return err
			}
			mu.Lock()
			defer mu.Unlock()
			report.merge(part)
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return report, err
	}
	slices.Sort(report.Examples)
	report.Examples = report.Examples[:min(len(report.Examples), maxExamples)]
	report.Duration = time.Since(start)
	return report, nil
}

func (r *RecomputeReport) merge(p RecomputeReport) {
	r.Results += p.Results
	r.Drifted += p.Drifted
	r.RowsMissing += p.RowsMissing
	r.RowsExtra += p.RowsExtra
	r.RowsChanged += p.RowsChanged
	r.CacheChanged += p.CacheChanged
	r.Examples = append(r.Examples, p.Examples...)
}

// recomputeBatch, bir grup öğrencinin sonuçlarını hesaplar, kayıtlılarla karşılaştırır ve
// apply ise farkı olanları yazar. Karşılaştırma salt okunur transaction'dadır.
func recomputeBatch(ctx context.Context, pool *pgxpool.Pool, ids []string, apply bool) (RecomputeReport, error) {
	var report RecomputeReport
	mode := pgx.ReadOnly
	if apply {
		mode = pgx.ReadWrite
	}
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: mode}, func(tx pgx.Tx) error {
		computed, err := computeTermResults(ctx, tx, ids)
		if err != nil {
			return err
		}
		stored, err := loadStoredTermResults(ctx, tx, ids)
		if err != nil {
			return err
		}
		var drifted []string
		for _, id := range ids {
			report.Results += len(computed[id])
			if compareProgram(computed[id], stored[id], &report) {
				drifted = append(drifted, id)
				if len(report.Examples) < maxExamples {
					report.Examples = append(report.Examples, stored[id].studentNo)
				}
			}
		}
		report.Drifted = len(drifted)
		if !apply || len(drifted) == 0 {
			return nil
		}
		return writeTermResults(ctx, tx, drifted, computed)
	})
	return report, err
}

// compareProgram, bir öğrencinin hesaplanan ve kayıtlı sonuçlarını karşılaştırır, farkları
// rapora ekler; fark varsa true döner.
func compareProgram(computed []gpa.Result, stored storedProgram, report *RecomputeReport) bool {
	drift := false
	seen := map[string]bool{}
	for _, r := range computed {
		seen[r.TermID] = true
		s, ok := stored.results[r.TermID]
		switch {
		case !ok:
			report.RowsMissing++
			drift = true
		case s.attempted != r.AttemptedECTS || s.earned != r.EarnedECTS || s.cumulativeECTS != r.CumulativeECTS ||
			!sameHundredths(s.termGPA, r.TermGPA) || !sameHundredths(s.cumulativeGPA, r.CumulativeGPA) || s.standing != r.Standing:
			report.RowsChanged++
			drift = true
		}
	}
	for term := range stored.results {
		if !seen[term] {
			report.RowsExtra++
			drift = true
		}
	}
	last, _ := gpa.Latest(computed)
	if !sameHundredths(stored.cacheGPA, last.CumulativeGPA) || stored.cacheECTS != last.CumulativeECTS {
		report.CacheChanged++
		drift = true
	}
	return drift
}

func sameHundredths(a, b *gpa.Hundredths) bool {
	return (a == nil) == (b == nil) && (a == nil || *a == *b)
}

// loadStoredTermResults, öğrencilerin kayıtlı dönem sonuçlarını ve önbelleklerini okur.
func loadStoredTermResults(ctx context.Context, q db.Querier, ids []string) (map[string]storedProgram, error) {
	out := make(map[string]storedProgram, len(ids))
	rows, err := q.Query(ctx, `
		SELECT sp.id, st.student_no, (sp.gpa_cache * 100)::int8, (sp.earned_ects_cache * 10)::int8
		FROM enrollment.student_programs sp JOIN people.students st ON st.id = sp.student_id
		WHERE sp.id = ANY($1)`, ids)
	if err != nil {
		return nil, fmt.Errorf("grading: önbellekler okunamadı: %w", err)
	}
	var (
		id, no string
		gpaV   *int64
		ects   int64
	)
	_, err = pgx.ForEachRow(rows, []any{&id, &no, &gpaV, &ects}, func() error {
		p := storedProgram{studentNo: no, results: map[string]storedResult{}, cacheECTS: gpa.ECTS(ects)}
		if gpaV != nil {
			v := gpa.Hundredths(*gpaV)
			p.cacheGPA = &v
		}
		out[id] = p
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("grading: önbellekler okunamadı: %w", err)
	}

	rows, err = q.Query(ctx, `
		SELECT student_program_id, term_id, (attempted_ects * 10)::int8, (earned_ects * 10)::int8,
		       (term_gpa * 100)::int8, (cumulative_ects * 10)::int8, (cumulative_gpa * 100)::int8, standing
		FROM enrollment.term_results WHERE student_program_id = ANY($1)`, ids)
	if err != nil {
		return nil, fmt.Errorf("grading: dönem sonuçları okunamadı: %w", err)
	}
	var (
		term, standing                string
		attempted, earned, cumulative int64
		termGPA, cumulativeGPA        *int64
	)
	hundredths := func(v *int64) *gpa.Hundredths {
		if v == nil {
			return nil
		}
		h := gpa.Hundredths(*v)
		return &h
	}
	_, err = pgx.ForEachRow(rows, []any{&id, &term, &attempted, &earned, &termGPA, &cumulative, &cumulativeGPA, &standing}, func() error {
		out[id].results[term] = storedResult{
			attempted: gpa.ECTS(attempted), earned: gpa.ECTS(earned), cumulativeECTS: gpa.ECTS(cumulative),
			termGPA: hundredths(termGPA), cumulativeGPA: hundredths(cumulativeGPA), standing: gpa.Standing(standing),
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("grading: dönem sonuçları okunamadı: %w", err)
	}
	return out, nil
}
