package lms

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/grading"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// Ödev ↔ değerlendirme bileşeni köprüsü: ödev şubenin dönem içi bir bileşenine ağırlıkla
// bağlanır; bileşenin notu, puanları yayımlanmış bağlı ödevlerin (geç kesintisi düşülmüş)
// puanlarının 100'lük ağırlıklı ortalamasıdır. Puanı olmayan öğrenci o ödevden 0 alır. Notlar
// not modülüne kaynak LMS olarak yazılır; harf notları ilan edilince donar.

// GradeSink, hesaplanan bileşen notlarını not modülüne yazar (grading.Service).
type GradeSink interface {
	ApplyLMSScores(ctx context.Context, actorID, componentID string, scores map[string]grading.Points) (int, error)
}

// Link, ödevin bağlı olduğu bileşen ve bileşen içindeki ağırlığıdır (yüzde birlik).
type Link struct {
	ComponentID string
	Weight      grading.Points
}

// LinkableComponent, ödevin bağlanabileceği bileşendir: şubenin dönem içi bileşenleri.
type LinkableComponent struct {
	ID      string
	LabelTR string
	LabelEN string
	Weight  float64 // bileşenin plandaki ağırlığı (%)
	Linked  int     // bileşene bağlı ödev sayısı
}

// LinkInfo, ödevin bağlantısı ve bağlanabileceği bileşenlerdir.
type LinkInfo struct {
	Link            *Link
	Components      []LinkableComponent
	PlanLocked      bool
	GradesPublished bool
}

// SyncState, bileşen notlarının eşitlenme sonucudur.
type SyncState string

// Eşitleme sonuçları: yazıldı, plan kilitlenmedi (notlar plan kilitlenince akar), harf notları
// ilan edildi (notlar dondu), bağlantı yok.
const (
	SyncDone          SyncState = "SYNCED"
	SyncPlanNotLocked SyncState = "PLAN_NOT_LOCKED"
	SyncFrozen        SyncState = "FROZEN"
	SyncNone          SyncState = "NOT_LINKED"
)

// LinkInfo, ödevin bağlantısını ve şubenin bağlanabilir bileşenlerini döndürür.
func (s *Service) LinkInfo(ctx context.Context, sectionID, activityID string) (LinkInfo, error) {
	var info LinkInfo
	var locked, published *time.Time
	if err := s.pool.QueryRow(ctx, `SELECT assessment_plan_locked_at, grades_published_at FROM offering.sections WHERE id = $1`,
		sectionID).Scan(&locked, &published); err != nil {
		return LinkInfo{}, fmt.Errorf("lms: şube okunamadı: %w", err)
	}
	info.PlanLocked, info.GradesPublished = locked != nil, published != nil
	var l Link
	var weight int64
	err := s.pool.QueryRow(ctx, `SELECT component_id, (weight * 100)::int8 FROM lms.assignment_links WHERE activity_id = $1`, activityID).
		Scan(&l.ComponentID, &weight)
	switch {
	case err == nil:
		l.Weight = grading.Points(weight)
		info.Link = &l
	case !errors.Is(err, pgx.ErrNoRows):
		return LinkInfo{}, fmt.Errorf("lms: bağlantı okunamadı: %w", err)
	}
	rows, err := s.pool.Query(ctx, `
		SELECT ac.id, ac.type_code, ac.sequence_no, coalesce(ac.name_tr, ''), coalesce(ac.name_en, ''), t.name_tr, t.name_en,
		       ac.weight::float8,
		       (SELECT count(*) FROM lms.assignment_links l JOIN lms.activities a ON a.id = l.activity_id
		        WHERE l.component_id = ac.id AND a.deleted_at IS NULL)
		FROM grading.assessment_components ac JOIN grading.assessment_types t ON t.code = ac.type_code
		WHERE ac.section_id = $1 AND t.category = 'IN_TERM'
		ORDER BY ac.position, ac.sequence_no`, sectionID)
	if err != nil {
		return LinkInfo{}, fmt.Errorf("lms: bileşenler okunamadı: %w", err)
	}
	type comp struct {
		c      grading.Component
		linked int
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (comp, error) {
		var c comp
		err := row.Scan(&c.c.ID, &c.c.Type.Code, &c.c.SequenceNo, &c.c.NameTR, &c.c.NameEN, &c.c.Type.NameTR, &c.c.Type.NameEN,
			&c.c.Weight, &c.linked)
		return c, err
	})
	if err != nil {
		return LinkInfo{}, fmt.Errorf("lms: bileşenler okunamadı: %w", err)
	}
	perType := map[string]int{}
	for _, c := range list {
		perType[c.c.Type.Code]++
	}
	for _, c := range list {
		numbered := perType[c.c.Type.Code] > 1
		info.Components = append(info.Components, LinkableComponent{ID: c.c.ID, LabelTR: c.c.Label(false, numbered),
			LabelEN: c.c.Label(true, numbered), Weight: c.c.Weight, Linked: c.linked})
	}
	return info, nil
}

// SetLink, ödevi bir bileşene bağlar (componentID boşsa bağlantıyı kaldırır) ve etkilenen
// bileşenlerin notlarını eşitler. Bileşen şubenin dönem içi bileşeni olmalıdır.
func (s *Service) SetLink(ctx context.Context, actorID, sectionID, activityID, componentID string, weight grading.Points) (SyncState, error) {
	if componentID != "" && (weight < 1 || weight > 10_000) {
		return "", &ValidationError{Field: "weight", Message: "0,01 ile 100 arasında olmalı."}
	}
	var previous string
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT 1 FROM lms.assignments WHERE activity_id = $1 FOR UPDATE`, activityID); err != nil {
			return fmt.Errorf("lms: ödev kilitlenemedi: %w", err)
		}
		err := tx.QueryRow(ctx, `SELECT component_id FROM lms.assignment_links WHERE activity_id = $1`, activityID).Scan(&previous)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("lms: bağlantı okunamadı: %w", err)
		}
		if componentID == "" {
			if _, err := tx.Exec(ctx, `DELETE FROM lms.assignment_links WHERE activity_id = $1`, activityID); err != nil {
				return fmt.Errorf("lms: bağlantı kaldırılamadı: %w", err)
			}
		} else {
			var ok bool
			if err := tx.QueryRow(ctx, `
				SELECT EXISTS (SELECT 1 FROM grading.assessment_components ac JOIN grading.assessment_types t ON t.code = ac.type_code
				               WHERE ac.id = $1 AND ac.section_id = $2 AND t.category = 'IN_TERM')`, componentID, sectionID).Scan(&ok); err != nil {
				return fmt.Errorf("lms: bileşen okunamadı: %w", err)
			}
			if !ok {
				return &ValidationError{Field: "component_id", Message: "Şubenin dönem içi bileşenlerinden biri olmalı (final ve bütünleme ödevden beslenmez)."}
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO lms.assignment_links (activity_id, component_id, weight, created_by) VALUES ($1, $2, $3::numeric / 100, $4)
				ON CONFLICT (activity_id) DO UPDATE SET component_id = excluded.component_id, weight = excluded.weight, updated_at = now()`,
				activityID, componentID, int64(weight), actorID); err != nil {
				return fmt.Errorf("lms: bağlantı yazılamadı: %w", err)
			}
		}
		return audit.Record(ctx, tx, audit.Entry{ActorUserID: actorID, Action: "lms_link.update", EntityType: "lms_activity", EntityID: activityID,
			Before: map[string]any{"component_id": nullIfEmpty(previous)},
			After:  map[string]any{"component_id": nullIfEmpty(componentID), "weight": weight.Float()}})
	})
	if err != nil {
		return "", err
	}
	if previous != "" && previous != componentID {
		if _, err := s.syncComponent(ctx, actorID, sectionID, previous); err != nil {
			return "", err
		}
	}
	if componentID == "" {
		return SyncNone, nil
	}
	return s.syncComponent(ctx, actorID, sectionID, componentID)
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// linkedComponent, ödevin bağlı olduğu bileşendir; yoksa boş.
func (s *Service) linkedComponent(ctx context.Context, activityID string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `SELECT component_id FROM lms.assignment_links WHERE activity_id = $1`, activityID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("lms: bağlantı okunamadı: %w", err)
	}
	return id, nil
}

// syncActivity, ödev bir bileşene bağlıysa bileşenin notlarını eşitler. Ödev işlemi kaydedildikten
// sonra çağrılır: eşitleme hatası işlemi geri almaz, günlüğe yazılır (bağlantı kaydedilince
// yeniden eşitlenir).
func (s *Service) syncActivity(ctx context.Context, actorID, activityID string) {
	componentID, err := s.linkedComponent(ctx, activityID)
	if err == nil && componentID != "" {
		var sectionID string
		if err = s.pool.QueryRow(ctx, `SELECT section_id FROM lms.activities WHERE id = $1`, activityID).Scan(&sectionID); err == nil {
			_, err = s.syncComponent(ctx, actorID, sectionID, componentID)
		}
	}
	if err != nil && s.logger != nil {
		s.logger.Error("bileşen notları eşitlenemedi", "activity_id", activityID, "err", err)
	}
}

// linkedAssignment, bileşene bağlı bir ödevdir.
type linkedAssignment struct {
	activityID string
	weight     grading.Points
	settings   AssignmentSettings
}

// syncComponent, bileşenin notlarını bağlı ödevlerden hesaplayıp not modülüne yazar.
func (s *Service) syncComponent(ctx context.Context, actorID, sectionID, componentID string) (SyncState, error) {
	if s.sink == nil {
		return SyncNone, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT l.activity_id, (l.weight * 100)::int8, `+assignmentColumns+`
		FROM lms.assignment_links l
		JOIN lms.assignments asg ON asg.activity_id = l.activity_id
		JOIN lms.activities a ON a.id = l.activity_id
		WHERE l.component_id = $1 AND a.deleted_at IS NULL`, componentID)
	if err != nil {
		return "", fmt.Errorf("lms: bağlı ödevler okunamadı: %w", err)
	}
	links, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (linkedAssignment, error) {
		var (
			l      linkedAssignment
			weight int64
		)
		st, err := scanAssignment(row, &l.activityID, &weight)
		l.weight, l.settings = grading.Points(weight), st
		return l, err
	})
	if err != nil {
		return "", fmt.Errorf("lms: bağlı ödevler okunamadı: %w", err)
	}
	rows, err = s.pool.Query(ctx, `
		SELECT e.id, u.id FROM enrollment.enrollments e
		JOIN enrollment.student_programs sp ON sp.id = e.student_program_id
		JOIN people.students st ON st.id = sp.student_id
		JOIN iam.users u ON u.person_id = st.person_id
		WHERE e.section_id = $1 AND e.status <> 'DROPPED'`, sectionID)
	if err != nil {
		return "", fmt.Errorf("lms: öğrenciler okunamadı: %w", err)
	}
	type enrolled struct{ enrollmentID, userID string }
	students, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (enrolled, error) {
		var e enrolled
		err := row.Scan(&e.enrollmentID, &e.userID)
		return e, err
	})
	if err != nil {
		return "", fmt.Errorf("lms: öğrenciler okunamadı: %w", err)
	}
	userIDs := make([]string, len(students))
	for i, st := range students {
		userIDs[i] = st.userID
	}
	var inputs []ComponentInput
	for _, l := range links {
		if l.settings.ReleasedAt == nil {
			continue // yayımlanmamış ödev bileşen notuna girmez
		}
		grades, err := s.grades(ctx, l.settings, l.activityID, userIDs)
		if err != nil {
			return "", err
		}
		finals := make(map[string]grading.Points, len(grades))
		for userID, g := range grades {
			if g.Final != nil {
				finals[userID] = *g.Final
			}
		}
		inputs = append(inputs, ComponentInput{Weight: l.weight, MaxScore: l.settings.MaxScore, Finals: finals})
	}
	scores := map[string]grading.Points{}
	if len(inputs) > 0 {
		for _, st := range students {
			scores[st.enrollmentID] = ComponentScore(inputs, st.userID)
		}
	}
	_, err = s.sink.ApplyLMSScores(ctx, actorID, componentID, scores)
	switch {
	case errors.Is(err, grading.ErrGradesPublished):
		return SyncFrozen, nil
	case errors.Is(err, grading.ErrPlanNotLocked):
		return SyncPlanNotLocked, nil
	case err != nil:
		return "", err
	}
	return SyncDone, nil
}

// ComponentInput, bileşen notuna giren yayımlanmış bir ödevdir: ağırlığı, en yüksek puanı ve
// öğrencilerin (geç kesintisi düşülmüş) puanları.
type ComponentInput struct {
	Weight   grading.Points
	MaxScore grading.Points
	Finals   map[string]grading.Points // kullanıcı → puan; olmayan 0 sayılır
}

// ComponentScore, öğrencinin bileşen notudur: ödev puanlarının 100'lük ağırlıklı ortalaması,
// tam kesirle hesaplanıp yüzde birliğe yarım yukarı yuvarlanır.
func ComponentScore(inputs []ComponentInput, userID string) grading.Points {
	sum, weights := new(big.Rat), new(big.Rat)
	for _, in := range inputs {
		if in.MaxScore <= 0 || in.Weight <= 0 {
			continue
		}
		// puan / en yüksek × 10000 (yüzde birlik 100'lük) × ağırlık
		part := big.NewRat(int64(in.Finals[userID])*10_000, int64(in.MaxScore))
		part.Mul(part, big.NewRat(int64(in.Weight), 1))
		sum.Add(sum, part)
		weights.Add(weights, big.NewRat(int64(in.Weight), 1))
	}
	if weights.Sign() == 0 {
		return 0
	}
	avg := sum.Quo(sum, weights)
	// Yarım yukarı: ⌊avg + 1/2⌋ (değer negatif olmaz).
	avg.Add(avg, big.NewRat(1, 2))
	q := new(big.Int).Quo(avg.Num(), avg.Denom())
	return min(grading.Points(q.Int64()), 10_000)
}
