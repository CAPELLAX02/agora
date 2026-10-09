package curriculum

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// Not ölçeği ve yönetmelik hataları.
var (
	ErrDefaultRequired   = errors.New("curriculum: varsayılan not ölçeği kaldırılamaz, başka bir ölçek varsayılan yapılmalı")
	ErrScoreRangeOverlap = errors.New("curriculum: puan aralıkları çakışıyor")
	ErrUnknownRegulation = errors.New("curriculum: yönetmelik parametresi tanımlı değil")
	ErrNotAfterCurrent   = errors.New("curriculum: yeni değer yürürlükteki değerden sonra başlamalı")
	ErrValueTypeMismatch = errors.New("curriculum: parametre değerinin JSON türü değişemez")
)

// GradeScale, bir not ölçeğidir.
type GradeScale struct {
	ID                string
	Code              string
	NameTR            string
	NameEN            string
	EffectiveFromYear int
	IsDefault         bool
	Version           int
	Items             []GradeItem
}

// GradeItem, ölçekteki bir harf notudur. Puan aralığı olmayan harfler puanla değil
// durumla verilir (F1 devamsızlık, BŞR/BŞZ, MUAF).
type GradeItem struct {
	Letter           string
	Coefficient      *float64
	MinScore         *float64
	MaxScore         *float64
	IsPassing        bool
	CountsInGPA      bool
	EarnsECTS        bool
	IsAttendanceFail bool
}

// RegulationParameter, bir yönetmelik parametresinin bir dönemdeki değeridir.
type RegulationParameter struct {
	Key           string
	Value         json.RawMessage
	EffectiveFrom time.Time
	EffectiveTo   *time.Time // hariç; nil ise hâlâ geçerli
	DescriptionTR string
	Note          string
}

// --- Not ölçekleri -------------------------------------------------------------

// GradeScales, not ölçeklerini harfleriyle döndürür: önce varsayılan, sonra yeniden eskiye.
func (r *Repository) GradeScales(ctx context.Context) ([]GradeScale, error) {
	return r.gradeScales(ctx, r.db, "")
}

// GradeScale, not ölçeğini döndürür. Yoksa ErrNotFound döner.
func (r *Repository) GradeScale(ctx context.Context, id string) (GradeScale, error) {
	scales, err := r.gradeScales(ctx, r.db, id)
	if err != nil {
		return GradeScale{}, err
	}
	if len(scales) == 0 {
		return GradeScale{}, ErrNotFound
	}
	return scales[0], nil
}

// gradeScales, id boşsa bütün ölçekleri, doluysa sadece o ölçeği harfleriyle okur.
func (r *Repository) gradeScales(ctx context.Context, q db.Querier, id string) ([]GradeScale, error) {
	rows, err := q.Query(ctx, `
		SELECT id, code, name_tr, name_en, effective_from_year, is_default, version
		FROM curriculum.grade_scales
		WHERE $1 = '' OR id::text = $1
		ORDER BY is_default DESC, effective_from_year DESC, code`, id)
	if err != nil {
		return nil, fmt.Errorf("curriculum: not ölçekleri okunamadı: %w", err)
	}
	scales, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (GradeScale, error) {
		var (
			g    GradeScale
			year int16
		)
		err := row.Scan(&g.ID, &g.Code, &g.NameTR, &g.NameEN, &year, &g.IsDefault, &g.Version)
		g.EffectiveFromYear, g.Items = int(year), []GradeItem{}
		return g, err
	})
	if err != nil {
		return nil, fmt.Errorf("curriculum: not ölçekleri okunamadı: %w", err)
	}
	if len(scales) == 0 {
		return scales, nil
	}

	index := make(map[string]int, len(scales))
	ids := make([]string, len(scales))
	for i, g := range scales {
		index[g.ID], ids[i] = i, g.ID
	}
	rows, err = q.Query(ctx, `
		SELECT grade_scale_id, letter, coefficient::float8, min_score::float8, max_score::float8,
		       is_passing, counts_in_gpa, earns_ects, is_attendance_fail
		FROM curriculum.grade_scale_items
		WHERE grade_scale_id = ANY($1::uuid[])
		ORDER BY grade_scale_id, sort_order`, ids)
	if err != nil {
		return nil, fmt.Errorf("curriculum: harf notları okunamadı: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			scaleID string
			it      GradeItem
		)
		if err := rows.Scan(&scaleID, &it.Letter, &it.Coefficient, &it.MinScore, &it.MaxScore,
			&it.IsPassing, &it.CountsInGPA, &it.EarnsECTS, &it.IsAttendanceFail); err != nil {
			return nil, fmt.Errorf("curriculum: harf notları okunamadı: %w", err)
		}
		g := &scales[index[scaleID]]
		g.Items = append(g.Items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("curriculum: harf notları okunamadı: %w", err)
	}
	return scales, nil
}

// GradeScaleInput, not ölçeği oluşturma ve güncelleme verisidir. Harflerin sırası
// gösterim sırasıdır.
type GradeScaleInput struct {
	Code              string // sadece oluşturmada
	NameTR            string
	NameEN            string
	EffectiveFromYear int
	IsDefault         bool
	Items             []GradeItem
}

func (in GradeScaleInput) audit() map[string]any {
	letters := make([]map[string]any, 0, len(in.Items))
	for _, it := range in.Items {
		letters = append(letters, map[string]any{
			"letter": it.Letter, "coefficient": it.Coefficient, "min_score": it.MinScore, "max_score": it.MaxScore,
			"is_passing": it.IsPassing, "counts_in_gpa": it.CountsInGPA, "earns_ects": it.EarnsECTS,
		})
	}
	return map[string]any{
		"code": in.Code, "name_tr": in.NameTR, "name_en": in.NameEN, "effective_from_year": in.EffectiveFromYear,
		"is_default": in.IsDefault, "items": letters,
	}
}

func (g GradeScale) input() GradeScaleInput {
	return GradeScaleInput{
		Code: g.Code, NameTR: g.NameTR, NameEN: g.NameEN, EffectiveFromYear: g.EffectiveFromYear,
		IsDefault: g.IsDefault, Items: g.Items,
	}
}

// CreateGradeScale, not ölçeği oluşturur. Varsayılan yapılırsa önceki varsayılan ölçek
// varsayılanlıktan çıkar.
func (r *Repository) CreateGradeScale(ctx context.Context, actorID string, in GradeScaleInput) (string, error) {
	var id string
	err := db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		if in.IsDefault {
			if err := clearDefaultScale(ctx, tx); err != nil {
				return err
			}
		}
		err := tx.QueryRow(ctx, `
			INSERT INTO curriculum.grade_scales (code, name_tr, name_en, effective_from_year, is_default)
			VALUES ($1, $2, $3, $4, $5) RETURNING id`,
			in.Code, in.NameTR, in.NameEN, in.EffectiveFromYear, in.IsDefault).Scan(&id)
		if err := writeError(err, "not ölçeği oluşturulamadı"); err != nil {
			return err
		}
		if err := insertGradeItems(ctx, tx, id, in.Items); err != nil {
			return err
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "grade_scale.create", EntityType: "grade_scale", EntityID: id, After: in.audit(),
		})
	})
	return id, err
}

// UpdateGradeScale, ölçeğin bilgilerini ve harflerini (tümüyle) değiştirir. Kod değişmez.
// Varsayılan ölçek doğrudan varsayılanlıktan çıkarılamaz: başka bir ölçek varsayılan yapılır.
func (r *Repository) UpdateGradeScale(ctx context.Context, actorID, id string, version int, in GradeScaleInput) error {
	return db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		var locked string
		err := tx.QueryRow(ctx, `SELECT id FROM curriculum.grade_scales WHERE id = $1 FOR UPDATE`, id).Scan(&locked)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("curriculum: not ölçeği okunamadı: %w", err)
		}
		scales, err := r.gradeScales(ctx, tx, id)
		if err != nil {
			return err
		}
		before := scales[0]
		if before.Version != version {
			return ErrVersionMismatch
		}
		if before.IsDefault && !in.IsDefault {
			return ErrDefaultRequired
		}
		if in.IsDefault && !before.IsDefault {
			if err := clearDefaultScale(ctx, tx); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `
			UPDATE curriculum.grade_scales
			SET name_tr = $2, name_en = $3, effective_from_year = $4, is_default = $5, version = version + 1, updated_at = now()
			WHERE id = $1`, id, in.NameTR, in.NameEN, in.EffectiveFromYear, in.IsDefault); err != nil {
			return fmt.Errorf("curriculum: not ölçeği güncellenemedi: %w", err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM curriculum.grade_scale_items WHERE grade_scale_id = $1`, id); err != nil {
			return fmt.Errorf("curriculum: harf notları silinemedi: %w", err)
		}
		if err := insertGradeItems(ctx, tx, id, in.Items); err != nil {
			return err
		}
		in.Code = before.Code
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "grade_scale.update", EntityType: "grade_scale", EntityID: id,
			Before: before.input().audit(), After: in.audit(),
		})
	})
}

func clearDefaultScale(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `
		UPDATE curriculum.grade_scales SET is_default = false, version = version + 1, updated_at = now() WHERE is_default`)
	if err != nil {
		return fmt.Errorf("curriculum: varsayılan ölçek değiştirilemedi: %w", err)
	}
	return nil
}

func insertGradeItems(ctx context.Context, tx pgx.Tx, scaleID string, items []GradeItem) error {
	for i, it := range items {
		_, err := tx.Exec(ctx, `
			INSERT INTO curriculum.grade_scale_items
			    (grade_scale_id, letter, coefficient, min_score, max_score, is_passing, counts_in_gpa, earns_ects,
			     is_attendance_fail, sort_order)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			scaleID, it.Letter, it.Coefficient, it.MinScore, it.MaxScore, it.IsPassing, it.CountsInGPA, it.EarnsECTS,
			it.IsAttendanceFail, i+1)
		switch {
		case db.IsExclusionViolation(err):
			return ErrScoreRangeOverlap
		case db.IsConflict(err):
			return ErrDuplicate
		case err != nil:
			return fmt.Errorf("curriculum: harf notu eklenemedi: %w", err)
		}
	}
	return nil
}

// --- Yönetmelik parametreleri --------------------------------------------------

const regulationSelect = `
	SELECT key, value, effective_from, effective_to, description_tr, coalesce(note, '')
	FROM curriculum.regulation_parameters`

func scanRegulation(row pgx.CollectableRow) (RegulationParameter, error) {
	var p RegulationParameter
	err := row.Scan(&p.Key, &p.Value, &p.EffectiveFrom, &p.EffectiveTo, &p.DescriptionTR, &p.Note)
	return p, err
}

// RegulationParameters, verilen günde geçerli olan yönetmelik parametrelerini döndürür.
func (r *Repository) RegulationParameters(ctx context.Context, at time.Time) ([]RegulationParameter, error) {
	rows, err := r.db.Query(ctx, regulationSelect+`
		WHERE effective_from <= $1 AND (effective_to IS NULL OR effective_to > $1)
		ORDER BY key`, at)
	if err != nil {
		return nil, fmt.Errorf("curriculum: yönetmelik parametreleri okunamadı: %w", err)
	}
	return pgx.CollectRows(rows, scanRegulation)
}

// RegulationHistory, parametrenin bütün değerlerini yeniden eskiye döndürür. Parametre
// tanımlı değilse ErrUnknownRegulation döner.
func (r *Repository) RegulationHistory(ctx context.Context, key string) ([]RegulationParameter, error) {
	rows, err := r.db.Query(ctx, regulationSelect+` WHERE key = $1 ORDER BY effective_from DESC`, key)
	if err != nil {
		return nil, fmt.Errorf("curriculum: yönetmelik parametresi okunamadı: %w", err)
	}
	history, err := pgx.CollectRows(rows, scanRegulation)
	if err != nil {
		return nil, fmt.Errorf("curriculum: yönetmelik parametresi okunamadı: %w", err)
	}
	if len(history) == 0 {
		return nil, ErrUnknownRegulation
	}
	return history, nil
}

// SetRegulationParameter, parametreye verilen günden itibaren geçerli yeni bir değer
// ekler; son değer o gün kapanır. Yeni parametre tanımlanamaz (kodu okuyan kural da
// yazılmalı), değerin JSON türü (sayı, nesne, dizi ...) değişemez.
func (r *Repository) SetRegulationParameter(ctx context.Context, actorID, key string, value json.RawMessage, from time.Time, note string) error {
	return db.InTx(ctx, r.pool, func(tx pgx.Tx) error {
		var (
			lastFrom time.Time
			lastVal  json.RawMessage
			sameType bool
		)
		err := tx.QueryRow(ctx, `
			SELECT effective_from, value, jsonb_typeof(value) = jsonb_typeof($2::jsonb)
			FROM curriculum.regulation_parameters WHERE key = $1
			ORDER BY effective_from DESC LIMIT 1 FOR UPDATE`, key, value).Scan(&lastFrom, &lastVal, &sameType)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUnknownRegulation
		}
		if err != nil {
			return fmt.Errorf("curriculum: yönetmelik parametresi okunamadı: %w", err)
		}
		if !from.After(lastFrom) {
			return ErrNotAfterCurrent
		}
		if !sameType {
			return ErrValueTypeMismatch
		}
		if _, err := tx.Exec(ctx, `
			UPDATE curriculum.regulation_parameters SET effective_to = $3 WHERE key = $1 AND effective_from = $2`,
			key, lastFrom, from); err != nil {
			return fmt.Errorf("curriculum: önceki değer kapatılamadı: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO curriculum.regulation_parameters (key, value, effective_from, description_tr, note)
			SELECT key, $2, $3, description_tr, $4 FROM curriculum.regulation_parameters WHERE key = $1 AND effective_from = $5`,
			key, value, from, nullable(note), lastFrom); err != nil {
			return fmt.Errorf("curriculum: yeni değer eklenemedi: %w", err)
		}
		return audit.Record(ctx, tx, audit.Entry{
			ActorUserID: actorID, Action: "regulation.set", EntityType: "regulation_parameter", EntityID: key,
			Before: map[string]any{"value": lastVal, "effective_from": lastFrom.Format(time.DateOnly)},
			After:  map[string]any{"value": value, "effective_from": from.Format(time.DateOnly), "note": note},
		})
	})
}
