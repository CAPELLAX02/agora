package main

import (
	"cmp"
	"context"
	"fmt"
	"hash/fnv"
	"math"
	"math/rand/v2"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/grading"
)

// Geçmiş dönem ders sonuçları. Ders seçme kuralları (GANO'ya göre AKTS sınırı, alttan
// dersler, ön koşullar) öğrencinin geçmişine dayanır. Gerçek sistemde bu geçmiş eski öğrenci
// bilgi sisteminden aktarılır; seed de aynısını yapar: müfredata bağlı her öğrenciye giriş
// yılına ve yarıyılına göre geçmiş dönemlerde aldığı dersleri ve notlarını üretir
// (source = IMPORT). Üretim deterministiktir: aynı tohum ve aynı öğrenci aynı geçmişi verir.
//
// Kurallar: her yarıyıl müfredatın o yarıyılındaki dersleri alınır, seçmeli yuvaları
// havuzdan rastgele doldurulur. Kalınan ders bir sonraki aynı dönemde (güz dersi güzde)
// tekrar alınır. Normal süresini aşmış öğrencilerin birkaç dersi hiç geçilememiştir:
// uzamalarının sebebi budur. Demo öğrencilerinin belirli notları sabittir (demo.go).
//
// Her dönemin notu o dönemde geçerli ölçekle verilir: yönetmeliğin 2023 değişikliğinden önceki
// dönemlerde AU-LISANS (50-59 C3, devamsızlık F1), sonrakilerde AU-LISANS-2023 (50-59 F1
// başarısız, devamsızlık F4). Puan dağılımı ölçeğin geçme puanına göre kayar: demo
// senaryolarının geçilen ve kalınan dersleri ölçekten bağımsızdır, harfler ve ortalamalar değişir.

type histTerm struct {
	id      string
	code    string
	typ     string
	order   int
	endedOn time.Time
	year    int // akademik yılın başlangıç yılı
}

type histCourse struct {
	id, code, language, department string
	ects, credit                   float64
	passFail                       bool
}

type histLine struct {
	semester  int
	courseID  string
	groupID   string
	slotCount int
}

type histStudent struct {
	spID, no, curriculumID string
	semester, normal       int
}

type histGrade struct {
	letter   string
	min, max float64
}

// histScale, bir not ölçeğidir.
type histScale struct {
	id       string
	fromYear int
	grades   []histGrade // puanla verilen harfler, yüksekten düşüğe
	absence  string      // devamsızlık harfi
	passing  map[string]bool
	passMin  float64 // geçen en düşük puan
}

func (s *histScale) letterFor(score float64) string {
	for _, g := range s.grades {
		if score >= g.min && score <= g.max {
			return g.letter
		}
	}
	return s.grades[len(s.grades)-1].letter
}

// scoreFor, sabitlenmiş bir harf notuna uyan bir puandır (aralığın ortası).
func (s *histScale) scoreFor(letter string) float64 {
	for _, g := range s.grades {
		if g.letter == letter {
			return math.Round((g.min + g.max) / 2)
		}
	}
	return 0
}

type histRecord struct {
	spID, termID, courseID string
	attempt                int
	scaleID, letter        string
	score                  *float64
	absent                 bool
}

type historyData struct {
	terms      []histTerm  // güz ve bahar dönemleri, eskiden yeniye; sonuncusu içinde bulunulan dönem
	scales     []histScale // başlangıç yılına göre sıralı
	courses    map[string]histCourse
	codeToID   map[string]string
	lines      map[string][]histLine // müfredat → satırlar
	groups     map[string][]string   // havuz → dersler
	students   []histStudent
	fallbackDp string // dersin sahibi bölüm yoksa açan bölüm
}

// seedHistory, müfredata bağlı ve henüz geçmişi olmayan öğrencilere geçmiş dönem ders
// sonuçlarını üretir, geçmiş dönemlerin ders açmalarını ve şubelerini oluşturur, dönem
// sonuçlarını (YANO/GANO) ve önbellekleri not ilanının kullandığı hesapla üretir. Geçmişi olan
// öğrenciye dokunmaz.
func seedHistory(ctx context.Context, pool *pgxpool.Pool, seed uint64) error {
	d, err := loadHistoryData(ctx, pool)
	if err != nil {
		return err
	}
	if d == nil || len(d.students) == 0 {
		return nil
	}

	var records []histRecord
	for _, st := range d.students {
		records = append(records, d.generate(st, seed)...)
	}
	if len(records) == 0 {
		return nil
	}

	sections, err := ensurePastSections(ctx, pool, d, records)
	if err != nil {
		return err
	}
	termEnd := map[string]time.Time{}
	for _, t := range d.terms {
		termEnd[t.id] = t.endedOn
	}

	err = pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		rows := make([][]any, 0, len(records))
		for _, r := range records {
			c := d.courses[r.courseID]
			attendance := "CONTINUING"
			if r.absent {
				attendance = "ATTENDANCE_FAIL"
			}
			rows = append(rows, []any{
				r.spID, sections[sectionKey{r.termID, r.courseID}], r.termID, r.courseID, r.attempt,
				"COMPLETED", attendance, r.score, r.scaleID, r.letter, c.ects, c.credit, "IMPORT", termEnd[r.termID],
			})
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"enrollment", "enrollments"},
			[]string{"student_program_id", "section_id", "term_id", "course_id", "attempt_no", "status",
				"attendance_status", "weighted_score", "grade_scale_id", "letter_grade", "ects_snapshot",
				"credit_snapshot", "source", "grade_published_at"},
			pgx.CopyFromRows(rows)); err != nil {
			return fmt.Errorf("ders geçmişi yazılamadı: %w", err)
		}

		spIDs := make([]string, len(d.students))
		for i, st := range d.students {
			spIDs[i] = st.spID
		}
		// Tekrar alınan derste son not geçerli: eski deneme, yerini alan denemeyi gösterir.
		if _, err := tx.Exec(ctx, `
			UPDATE enrollment.enrollments e SET superseded_by_id = x.next_id
			FROM (
				SELECT e2.id, lead(e2.id) OVER (PARTITION BY e2.student_program_id, e2.course_id ORDER BY t.starts_on) AS next_id
				FROM enrollment.enrollments e2 JOIN academic.terms t ON t.id = e2.term_id
				WHERE e2.student_program_id = ANY($1) AND e2.status <> 'DROPPED'
			) x
			WHERE e.id = x.id AND x.next_id IS NOT NULL`, spIDs); err != nil {
			return fmt.Errorf("tekrar alınan dersler bağlanamadı: %w", err)
		}
		sectionIDs := make([]string, 0, len(sections))
		for _, id := range sections {
			sectionIDs = append(sectionIDs, id)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE offering.sections s SET capacity = greatest(s.capacity, c.n), enrolled_count = c.n, updated_at = now()
			FROM (SELECT section_id, count(*)::int AS n FROM enrollment.enrollments
			      WHERE section_id = ANY($1) AND status <> 'DROPPED' GROUP BY section_id) c
			WHERE s.id = c.section_id`, sectionIDs); err != nil {
			return fmt.Errorf("şube mevcutları güncellenemedi: %w", err)
		}
		return grading.RecomputeTermResults(ctx, tx, spIDs)
	})
	if err != nil {
		return err
	}
	fmt.Printf("GEÇMİŞ     %d öğrenciye %d geçmiş ders sonucu aktarıldı (%d şube)\n", len(d.students), len(records), len(sections))
	return nil
}

// seedGradeStats, notları verilmiş ama sınıf istatistiği olmayan şubelerin (aktarılan geçmiş
// dönemler) istatistiklerini hesaplar. Gerçek sistemde ilan bunu kendisi yapar; aktarılan
// notlar ilan edilmeden gelir. Daha önce aktarılmış bir geçmiş için de çalışır.
func seedGradeStats(ctx context.Context, pool *pgxpool.Pool) error {
	rows, err := pool.Query(ctx, `
		SELECT DISTINCT e.section_id FROM enrollment.enrollments e
		WHERE e.status = 'COMPLETED'
		  AND NOT EXISTS (SELECT 1 FROM grading.section_grade_stats g WHERE g.section_id = e.section_id)`)
	if err != nil {
		return fmt.Errorf("şubeler okunamadı: %w", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return fmt.Errorf("şubeler okunamadı: %w", err)
	}
	if len(ids) == 0 {
		return nil
	}
	if err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error { return grading.RefreshStats(ctx, tx, ids) }); err != nil {
		return err
	}
	fmt.Printf("İSTATİSTİK %d şubenin sınıf istatistiği hesaplandı\n", len(ids))
	return nil
}

// seedTermResults, notu verilmiş dersi olup dönem sonucu olmayan öğrencilerin (önceden
// aktarılmış geçmiş) dönem sonuçlarını, GANO ve kazanılan AKTS önbelleklerini hesaplar.
func seedTermResults(ctx context.Context, pool *pgxpool.Pool) error {
	rows, err := pool.Query(ctx, `
		SELECT DISTINCT e.student_program_id FROM enrollment.enrollments e
		WHERE e.status = 'COMPLETED'
		  AND NOT EXISTS (SELECT 1 FROM enrollment.term_results r WHERE r.student_program_id = e.student_program_id)`)
	if err != nil {
		return fmt.Errorf("öğrenciler okunamadı: %w", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return fmt.Errorf("öğrenciler okunamadı: %w", err)
	}
	if len(ids) == 0 {
		return nil
	}
	if err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		return grading.RecomputeTermResults(ctx, tx, ids)
	}); err != nil {
		return err
	}
	fmt.Printf("DÖNEM      %d öğrencinin dönem sonuçları (YANO/GANO) hesaplandı\n", len(ids))
	return nil
}

// generate, bir öğrencinin geçmiş dönem ders sonuçlarını üretir.
func (d *historyData) generate(st histStudent, seed uint64) []histRecord {
	h := fnv.New64a()
	_, _ = h.Write([]byte(st.no))
	rng := rand.New(rand.NewPCG(seed, h.Sum64()))

	demo, isDemo := demoByNo(st.no)
	ability := rng.NormFloat64() * 0.8
	if isDemo {
		ability = demo.ability
	}

	current := len(d.terms) - 1
	termOf := func(semester int) int { return current - (st.semester - semester) }

	lines := d.lines[st.curriculumID]
	// Normal süresini aşmış öğrencinin birkaç zorunlu dersi hiç geçilememiştir.
	persistent := map[string]bool{}
	if !isDemo && st.semester > st.normal {
		var pool []string
		for _, l := range lines {
			if l.courseID != "" && l.semester < st.semester && !d.courses[l.courseID].passFail {
				pool = append(pool, l.courseID)
			}
		}
		for range 1 + rng.IntN(3) {
			if len(pool) > 0 {
				persistent[pool[rng.IntN(len(pool))]] = true
			}
		}
	}

	type state struct {
		attempts int
		passed   bool
		lastTerm int
		stop     bool // demo: sabit notlar bitti, tekrar alınmadı
	}
	taken := map[string]*state{}
	var out []histRecord

	take := func(courseID string, term int) {
		s := taken[courseID]
		if s == nil {
			s = &state{}
			taken[courseID] = s
		}
		s.attempts++
		s.lastTerm = term
		c := d.courses[courseID]
		sc := d.scaleFor(term)

		var letter string
		var score *float64
		absent := false
		fixed, hasFixed := []string(nil), false
		if isDemo {
			fixed, hasFixed = demo.letters[c.code]
		}
		switch {
		case hasFixed && s.attempts <= len(fixed):
			letter = fixed[s.attempts-1]
			if letter == absentLetter {
				letter, absent = sc.absence, true
			} else if !c.passFail {
				v := sc.scoreFor(letter)
				score = &v
			}
			if s.attempts == len(fixed) {
				s.stop = true
			}
		case persistent[courseID]:
			v := float64(30 + rng.IntN(18))
			letter, score = sc.letterFor(v), &v
		default:
			letter, score, absent = d.grade(rng, c, sc, ability, isDemo && demo.clean)
		}
		s.passed = sc.passing[letter]
		out = append(out, histRecord{
			spID: st.spID, termID: d.terms[term].id, courseID: courseID, attempt: s.attempts,
			scaleID: sc.id, letter: letter, score: score, absent: absent,
		})
	}

	for sem := 1; sem < st.semester; sem++ {
		term := termOf(sem)
		if term < 0 {
			continue // aktarılan geçmiş bu dönemden eski değil
		}
		// Önce alttan dersler: geçen yıl aynı dönemde kalınanlar.
		var retakes []string
		for id, s := range taken {
			if !s.passed && !s.stop && s.lastTerm == term-2 {
				retakes = append(retakes, id)
			}
		}
		// Map sırası rastgeledir; kimlik (UUID) de veritabanına göre değişir: koda göre sırala.
		slices.SortFunc(retakes, func(a, b string) int { return cmp.Compare(d.courses[a].code, d.courses[b].code) })
		for _, id := range retakes {
			take(id, term)
		}
		for _, l := range lines {
			if l.semester != sem {
				continue
			}
			if l.courseID != "" {
				if taken[l.courseID] == nil {
					take(l.courseID, term)
				}
				continue
			}
			var candidates []string
			for _, id := range d.groups[l.groupID] {
				if taken[id] == nil {
					candidates = append(candidates, id)
				}
			}
			rng.Shuffle(len(candidates), func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })
			for _, id := range candidates[:min(l.slotCount, len(candidates))] {
				take(id, term)
			}
		}
	}
	return out
}

// scaleFor, dönemde geçerli ölçektir: başlangıç yılı dönemin akademik yılını geçmeyen en yeni
// ölçek.
func (d *historyData) scaleFor(term int) *histScale {
	sc := &d.scales[0]
	for i := range d.scales {
		if d.scales[i].fromYear <= d.terms[term].year {
			sc = &d.scales[i]
		}
	}
	return sc
}

// grade, bir dersin notunu üretir: başarı düzeyi, dersin zorluğu ve rastlantı.
func (d *historyData) grade(rng *rand.Rand, c histCourse, sc *histScale, ability float64, clean bool) (string, *float64, bool) {
	if c.passFail {
		if !clean && rng.Float64() < 0.03 {
			return "BŞZ", nil, false
		}
		return "BŞR", nil, false
	}
	if !clean && rng.Float64() < 0.015+math.Max(0, -ability)*0.02 {
		return sc.absence, nil, true
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(c.code))
	difficulty := float64(h.Sum32()%9) - 2
	// Dağılım ölçeğin geçme puanına göre: aynı öğrenci aynı dersten her ölçekte geçer ya da kalır.
	v := math.Round(sc.passMin + 20 + 10*ability + rng.NormFloat64()*9 - difficulty)
	if clean && v < sc.passMin {
		v = sc.passMin + float64(rng.IntN(15))
	}
	v = math.Max(0, math.Min(100, v))
	return sc.letterFor(v), &v, false
}

type sectionKey struct{ termID, courseID string }

// ensurePastSections, geçmiş dönemlerin ders açmalarını ve tek şubelerini oluşturur (varsa
// olanları kullanır). Kontenjan, aktarılan kayıt sayısına göre sonradan güncellenir.
func ensurePastSections(ctx context.Context, pool *pgxpool.Pool, d *historyData, records []histRecord) (map[sectionKey]string, error) {
	keys := map[sectionKey]bool{}
	for _, r := range records {
		keys[sectionKey{r.termID, r.courseID}] = true
	}
	list := make([]sectionKey, 0, len(keys))
	for k := range keys {
		list = append(list, k)
	}
	slices.SortFunc(list, func(a, b sectionKey) int {
		return cmp.Or(cmp.Compare(a.termID, b.termID), cmp.Compare(a.courseID, b.courseID))
	})

	out := make(map[sectionKey]string, len(list))
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		batch := &pgx.Batch{}
		for _, k := range list {
			c := d.courses[k.courseID]
			dept := c.department
			if dept == "" {
				dept = d.fallbackDp
			}
			batch.Queue(`
				WITH o AS (
					INSERT INTO offering.course_offerings (term_id, course_id, department_id, status, note)
					VALUES ($1, $2, $3, 'CLOSED', 'Eski sistemden aktarıldı')
					ON CONFLICT (term_id, course_id) DO UPDATE SET term_id = EXCLUDED.term_id
					RETURNING id
				)
				INSERT INTO offering.sections (offering_id, section_code, capacity, language)
				SELECT id, '1', 0, $4 FROM o
				ON CONFLICT (offering_id, section_code) DO UPDATE SET section_code = EXCLUDED.section_code
				RETURNING id`, k.termID, k.courseID, dept, c.language)
		}
		results := tx.SendBatch(ctx, batch)
		for _, k := range list {
			var id string
			if err := results.QueryRow().Scan(&id); err != nil {
				_ = results.Close()
				return fmt.Errorf("geçmiş şube oluşturulamadı: %w", err)
			}
			out[k] = id
		}
		return results.Close()
	})
	return out, err
}

// loadHistScales, not ölçeklerini başlangıç yılına göre okur (aynı yılda varsayılan sonda).
func loadHistScales(ctx context.Context, pool *pgxpool.Pool) ([]histScale, error) {
	rows, err := pool.Query(ctx, `
		SELECT s.id, s.effective_from_year, i.letter, coalesce(i.min_score::float8, -1), coalesce(i.max_score::float8, -1),
		       i.is_passing, i.is_attendance_fail
		FROM curriculum.grade_scales s JOIN curriculum.grade_scale_items i ON i.grade_scale_id = s.id
		ORDER BY s.effective_from_year, s.is_default, s.code, i.min_score DESC NULLS LAST`)
	if err != nil {
		return nil, fmt.Errorf("not ölçekleri okunamadı: %w", err)
	}
	var (
		scales            []histScale
		id, letter        string
		year              int
		lo, hi            float64
		passing, absentee bool
	)
	_, err = pgx.ForEachRow(rows, []any{&id, &year, &letter, &lo, &hi, &passing, &absentee}, func() error {
		if len(scales) == 0 || scales[len(scales)-1].id != id {
			scales = append(scales, histScale{id: id, fromYear: year, passing: map[string]bool{}, passMin: 101})
		}
		sc := &scales[len(scales)-1]
		sc.passing[letter] = passing
		if absentee {
			sc.absence = letter
		}
		if lo >= 0 {
			sc.grades = append(sc.grades, histGrade{letter: letter, min: lo, max: hi})
			if passing {
				sc.passMin = math.Min(sc.passMin, lo)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("not ölçekleri okunamadı: %w", err)
	}
	scales = slices.DeleteFunc(scales, func(s histScale) bool { return len(s.grades) == 0 || s.absence == "" })
	if len(scales) == 0 {
		return nil, fmt.Errorf("puan aralıklı ve devamsızlık harfli bir not ölçeği yok")
	}
	return scales, nil
}

func loadHistoryData(ctx context.Context, pool *pgxpool.Pool) (*historyData, error) {
	d := &historyData{courses: map[string]histCourse{}, codeToID: map[string]string{},
		lines: map[string][]histLine{}, groups: map[string][]string{}}

	rows, err := pool.Query(ctx, `
		SELECT t.id, t.code, t.term_type, ay.start_year * 3 + CASE t.term_type WHEN 'FALL' THEN 0 ELSE 1 END, t.ends_on,
		       ay.start_year
		FROM academic.terms t JOIN academic.academic_years ay ON ay.id = t.academic_year_id
		WHERE t.term_type IN ('FALL', 'SPRING')
		  AND t.starts_on <= (SELECT starts_on FROM academic.terms WHERE is_current)
		ORDER BY 4`)
	if err != nil {
		return nil, fmt.Errorf("dönemler okunamadı: %w", err)
	}
	d.terms, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (histTerm, error) {
		var t histTerm
		return t, row.Scan(&t.id, &t.code, &t.typ, &t.order, &t.endedOn, &t.year)
	})
	if err != nil {
		return nil, fmt.Errorf("dönemler okunamadı: %w", err)
	}
	if len(d.terms) < 2 {
		fmt.Println("ATLANDI    ders geçmişi: geçmiş dönemler yok (önce SQL seed'leri yükleyin)")
		return nil, nil
	}

	if d.scales, err = loadHistScales(ctx, pool); err != nil {
		return nil, err
	}

	rows, err = pool.Query(ctx, `
		SELECT id, code, language, coalesce(owner_department_id::text, ''), ects::float8, national_credit::float8,
		       grading_mode = 'PASS_FAIL'
		FROM curriculum.courses`)
	if err != nil {
		return nil, fmt.Errorf("dersler okunamadı: %w", err)
	}
	var c histCourse
	if _, err := pgx.ForEachRow(rows, []any{&c.id, &c.code, &c.language, &c.department, &c.ects, &c.credit, &c.passFail}, func() error {
		d.courses[c.id] = c
		d.codeToID[c.code] = c.id
		return nil
	}); err != nil {
		return nil, fmt.Errorf("dersler okunamadı: %w", err)
	}

	// Sıralamalar kodlara dayanır, kimliklere (UUID) değil: aynı tohum her veritabanında
	// (geliştirme, e2e, test) aynı geçmişi üretsin.
	rows, err = pool.Query(ctx, `
		SELECT i.curriculum_id, i.semester_no, coalesce(i.course_id::text, ''), coalesce(i.elective_group_id::text, ''),
		       coalesce(i.slot_course_count, 1)
		FROM curriculum.curriculum_items i
		LEFT JOIN curriculum.courses c ON c.id = i.course_id
		LEFT JOIN curriculum.elective_groups g ON g.id = i.elective_group_id
		ORDER BY i.curriculum_id, i.semester_no, i.position, coalesce(c.code, g.code)`)
	if err != nil {
		return nil, fmt.Errorf("müfredatlar okunamadı: %w", err)
	}
	var (
		curID string
		l     histLine
	)
	if _, err := pgx.ForEachRow(rows, []any{&curID, &l.semester, &l.courseID, &l.groupID, &l.slotCount}, func() error {
		d.lines[curID] = append(d.lines[curID], l)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("müfredatlar okunamadı: %w", err)
	}

	rows, err = pool.Query(ctx, `
		SELECT gc.elective_group_id, gc.course_id FROM curriculum.elective_group_courses gc
		JOIN curriculum.courses c ON c.id = gc.course_id ORDER BY gc.elective_group_id, c.code`)
	if err != nil {
		return nil, fmt.Errorf("seçmeli havuzları okunamadı: %w", err)
	}
	var g, cid string
	if _, err := pgx.ForEachRow(rows, []any{&g, &cid}, func() error {
		d.groups[g] = append(d.groups[g], cid)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("seçmeli havuzları okunamadı: %w", err)
	}

	if err := pool.QueryRow(ctx, `SELECT id FROM org.departments WHERE code = 'BIL'`).Scan(&d.fallbackDp); err != nil {
		return nil, fmt.Errorf("BIL bölümü okunamadı: %w", err)
	}

	rows, err = pool.Query(ctx, `
		SELECT sp.id, s.student_no, sp.curriculum_id, sp.current_semester, p.duration_semesters
		FROM enrollment.student_programs sp
		JOIN people.students s ON s.id = sp.student_id
		JOIN org.programs p ON p.id = sp.program_id
		WHERE sp.curriculum_id IS NOT NULL AND sp.current_semester > 1
		  AND sp.status IN ('ACTIVE', 'FROZEN', 'SUSPENDED')
		  AND NOT EXISTS (SELECT 1 FROM enrollment.enrollments e WHERE e.student_program_id = sp.id)
		ORDER BY s.student_no`)
	if err != nil {
		return nil, fmt.Errorf("öğrenciler okunamadı: %w", err)
	}
	d.students, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (histStudent, error) {
		var st histStudent
		var sem, normal int16
		err := row.Scan(&st.spID, &st.no, &st.curriculumID, &sem, &normal)
		st.semester, st.normal = int(sem), int(normal)
		return st, err
	})
	if err != nil {
		return nil, fmt.Errorf("öğrenciler okunamadı: %w", err)
	}
	return d, nil
}
