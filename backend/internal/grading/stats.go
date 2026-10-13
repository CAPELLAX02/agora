package grading

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/grading/letter"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// ErrStatsNotAvailable: şubenin harf notları henüz ilan edilmedi, sınıf istatistiği yok.
var ErrStatsNotAvailable = errors.New("grading: sınıf istatistiği yok")

// Sample, sınıf istatistiğine giren bir öğrencinin notudur.
type Sample struct {
	Letter   string
	Weighted *Points
	Passing  bool
}

// Stats, sınıfın not istatistikleridir.
type Stats struct {
	Students int // harfi belli olan öğrenciler
	Scored   int // ders başarı puanı olanlar: ortalama, medyan ve standart sapma bunlardan
	Passed   int
	// Mean, Median, StdDev: Scored = 0 ise nil.
	Mean, Median, StdDev *Points
	Distribution         map[string]int
}

// PassRate, başarı oranıdır (0-1); öğrenci yoksa nil.
func (s Stats) PassRate() *float64 {
	if s.Students == 0 {
		return nil
	}
	r := float64(s.Passed) / float64(s.Students)
	return &r
}

// ComputeStats, sınıfın istatistiklerini hesaplar; harfi belli olmayan öğrenci sayılmaz.
// Ortalama ve medyan yüzde birliğe yarım yukarı yuvarlanır. Standart sapma sınıfın tamamı
// üzerinden (popülasyon) hesaplanır: varyans tamsayılarla tam bulunur, karekök tek kayan
// nokta adımıdır.
func ComputeStats(samples []Sample) Stats {
	st := Stats{Distribution: map[string]int{}}
	var scores []Points
	for _, s := range samples {
		if s.Letter == "" {
			continue
		}
		st.Students++
		st.Distribution[s.Letter]++
		if s.Passing {
			st.Passed++
		}
		if s.Weighted != nil {
			scores = append(scores, *s.Weighted)
		}
	}
	n := len(scores)
	st.Scored = n
	if n == 0 {
		return st
	}
	slices.Sort(scores)
	var sum, squares int64
	for _, p := range scores {
		sum += int64(p)
		squares += int64(p) * int64(p)
	}
	m := mean(Points(sum), n)
	med := scores[n/2]
	if n%2 == 0 {
		med = mean(scores[n/2-1]+scores[n/2], 2)
	}
	// σ = √(nΣx² − (Σx)²) / n
	sd := Points(math.Round(math.Sqrt(float64(int64(n)*squares-sum*sum)) / float64(n)))
	st.Mean, st.Median, st.StdDev = &m, &med, &sd
	return st
}

// Distribution, harf dağılımını ölçeğin sırasıyla verir: dersin notlandırmasına uygun bütün
// harfler (sıfır olanlar dahil), ayrıca başka bir harf verilmişse o da.
func Distribution(scale letter.Scale, passFail bool, counts map[string]int) []LetterCount {
	var out []LetterCount
	listed := map[string]bool{}
	for _, g := range ManualLetters(scale, passFail) {
		out = append(out, LetterCount{Letter: g.Letter, Count: counts[g.Letter]})
		listed[g.Letter] = true
	}
	for _, g := range scale {
		if !listed[g.Letter] && counts[g.Letter] > 0 {
			out = append(out, LetterCount{Letter: g.Letter, Count: counts[g.Letter]})
			listed[g.Letter] = true
		}
	}
	return out
}

// LetterCount, bir harfi alan öğrenci sayısıdır.
type LetterCount struct {
	Letter string
	Count  int
}

// RefreshStats, şubelerin sınıf istatistiklerini kesin kayıtlardan yeniden hesaplar. İlan
// edilmiş notu olmayan şubenin istatistiği silinir. İlanlarla aynı transaction'da çağrılır;
// seed de aktarılan geçmiş dönemler için kullanır.
func RefreshStats(ctx context.Context, q db.Querier, sectionIDs []string) error {
	if len(sectionIDs) == 0 {
		return nil
	}
	rows, err := q.Query(ctx, `
		SELECT e.section_id, e.letter_grade, (e.weighted_score * 100)::int8, gi.is_passing
		FROM enrollment.enrollments e
		JOIN curriculum.grade_scale_items gi ON gi.grade_scale_id = e.grade_scale_id AND gi.letter = e.letter_grade
		WHERE e.section_id = ANY($1) AND e.status = 'COMPLETED'`, sectionIDs)
	if err != nil {
		return fmt.Errorf("grading: sınıf notları okunamadı: %w", err)
	}
	samples := map[string][]Sample{}
	var (
		sectionID string
		s         Sample
		weighted  *int64
	)
	_, err = pgx.ForEachRow(rows, []any{&sectionID, &s.Letter, &weighted, &s.Passing}, func() error {
		s.Weighted = nil
		if weighted != nil {
			p := Points(*weighted)
			s.Weighted = &p
		}
		samples[sectionID] = append(samples[sectionID], s)
		return nil
	})
	if err != nil {
		return fmt.Errorf("grading: sınıf notları okunamadı: %w", err)
	}

	ids := make([]string, 0, len(samples)) // boş dizi (NULL değil): <> ALL boş dizide hep doğru
	var (
		students, scored, passed []int32
		means, medians, stddevs  []*int64
		distributions            []string
	)
	hundredths := func(p *Points) *int64 {
		if p == nil {
			return nil
		}
		v := int64(*p)
		return &v
	}
	for id, list := range samples {
		st := ComputeStats(list)
		if st.Students == 0 {
			continue
		}
		dist, err := json.Marshal(st.Distribution)
		if err != nil {
			return fmt.Errorf("grading: harf dağılımı kodlanamadı: %w", err)
		}
		ids = append(ids, id)
		students = append(students, int32(st.Students))
		scored = append(scored, int32(st.Scored))
		passed = append(passed, int32(st.Passed))
		means, medians, stddevs = append(means, hundredths(st.Mean)), append(medians, hundredths(st.Median)), append(stddevs, hundredths(st.StdDev))
		distributions = append(distributions, string(dist))
	}
	if _, err := q.Exec(ctx, `
		DELETE FROM grading.section_grade_stats WHERE section_id = ANY($1) AND section_id <> ALL($2)`,
		sectionIDs, ids); err != nil {
		return fmt.Errorf("grading: sınıf istatistikleri silinemedi: %w", err)
	}
	if len(ids) == 0 {
		return nil
	}
	if _, err := q.Exec(ctx, `
		INSERT INTO grading.section_grade_stats
			(section_id, student_count, scored_count, passed_count, mean, median, stddev, letter_distribution, computed_at)
		SELECT id, students, scored, passed, mean / 100.0, median / 100.0, stddev / 100.0, distribution::jsonb, now()
		FROM unnest($1::uuid[], $2::int[], $3::int[], $4::int[], $5::int8[], $6::int8[], $7::int8[], $8::text[])
			AS x (id, students, scored, passed, mean, median, stddev, distribution)
		ON CONFLICT (section_id) DO UPDATE SET
			student_count = excluded.student_count, scored_count = excluded.scored_count,
			passed_count = excluded.passed_count, mean = excluded.mean, median = excluded.median,
			stddev = excluded.stddev, letter_distribution = excluded.letter_distribution, computed_at = now()`,
		ids, students, scored, passed, means, medians, stddevs, distributions); err != nil {
		return fmt.Errorf("grading: sınıf istatistikleri yazılamadı: %w", err)
	}
	return nil
}

// SectionStats, bir şubenin kayıtlı sınıf istatistikleridir.
type SectionStats struct {
	Stats
	Scale      letter.Scale // şubenin (varsayılan) not ölçeği: dağılımın sırası için
	ComputedAt time.Time
}

// SectionStats, şubenin sınıf istatistiklerini okur. Harf notları ilan edilmemişse
// ErrStatsNotAvailable döner.
func (s *Service) SectionStats(ctx context.Context, sectionID string) (SectionStats, error) {
	var (
		out                  SectionStats
		mean, median, stddev *int64
		dist                 []byte
		scaleID              string
	)
	err := s.pool.QueryRow(ctx, `
		SELECT student_count, scored_count, passed_count, (mean * 100)::int8, (median * 100)::int8, (stddev * 100)::int8,
		       letter_distribution, computed_at, (SELECT id FROM curriculum.grade_scales WHERE is_default)
		FROM grading.section_grade_stats WHERE section_id = $1`, sectionID).Scan(
		&out.Students, &out.Scored, &out.Passed, &mean, &median, &stddev, &dist, &out.ComputedAt, &scaleID)
	if errors.Is(err, pgx.ErrNoRows) {
		return SectionStats{}, ErrStatsNotAvailable
	}
	if err != nil {
		return SectionStats{}, fmt.Errorf("grading: sınıf istatistikleri okunamadı: %w", err)
	}
	points := func(v *int64) *Points {
		if v == nil {
			return nil
		}
		p := Points(*v)
		return &p
	}
	out.Mean, out.Median, out.StdDev = points(mean), points(median), points(stddev)
	if err := json.Unmarshal(dist, &out.Distribution); err != nil {
		return SectionStats{}, fmt.Errorf("grading: harf dağılımı çözülemedi: %w", err)
	}
	if out.Scale, err = loadScale(ctx, s.pool, scaleID); err != nil {
		return SectionStats{}, err
	}
	return out, nil
}

// EnrolledIn, kullanıcının şubede (bırakılmamış) kesin kaydı olan öğrenci olduğunu söyler.
func (s *Service) EnrolledIn(ctx context.Context, sectionID, userID string) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM enrollment.enrollments e
			JOIN enrollment.student_programs sp ON sp.id = e.student_program_id
			JOIN people.students st ON st.id = sp.student_id
			JOIN iam.users u ON u.person_id = st.person_id
			WHERE e.section_id = $1 AND u.id::text = $2 AND e.status <> 'DROPPED')`, sectionID, userID).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("grading: kesin kayıt okunamadı: %w", err)
	}
	return ok, nil
}
