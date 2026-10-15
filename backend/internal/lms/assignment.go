package lms

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/files"
	"github.com/CAPELLAX02/agora/backend/internal/grading"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// Teslim türleri.
const (
	SubmitFile = "FILE"
	SubmitText = "ONLINE_TEXT"
)

// Geç teslim politikaları: son tarihten sonra kesin kapanışa kadar kabul (geç işaretlenir), hiç
// kabul etmeme ya da kabul edip başlamış her gün için puandan yüzde düşme.
const (
	LateAllow   = "ALLOW"
	LateDeny    = "DENY"
	LatePenalty = "PENALTY"
)

// Teslim durumları.
const (
	StatusDraft     = "DRAFT"
	StatusSubmitted = "SUBMITTED"
)

// AssignmentSettings, ödevin ayarlarıdır. Puanlar ve yüzde yüzde birlik tamsayıdır
// (grading.Points: 10000 = 100,00).
type AssignmentSettings struct {
	OpensAt         *time.Time // teslimin başladığı an; boşsa hemen
	DueAt           *time.Time // son tarih; sonrası geç
	CutoffAt        *time.Time // kesin kapanış (geç teslim açıkken); boşsa sınırsız
	MaxAttempts     int
	SubmissionTypes []string
	MaxFiles        int
	MaxFileSize     int64
	Extensions      []string // ".pdf" biçiminde; boşsa platformun izin listesi
	MaxScore        grading.Points
	LatePolicy      string
	PenaltyPerDay   grading.Points // yüzde: 1000 = %10
	ReleasedAt      *time.Time     // puanların yayımlandığı an (salt okunur)
}

// Accepts, teslim türünün kabul edildiğini söyler.
func (a AssignmentSettings) Accepts(kind string) bool {
	return slices.Contains(a.SubmissionTypes, kind)
}

// normalize, uzantıları küçük harfe ve ".pdf" biçimine getirir, tekrarları ve teslim türlerinin
// tekrarlarını atar.
func (a *AssignmentSettings) normalize() {
	exts := make([]string, 0, len(a.Extensions))
	for _, e := range a.Extensions {
		e = strings.ToLower(strings.TrimSpace(e))
		if e == "" {
			continue
		}
		if !strings.HasPrefix(e, ".") {
			e = "." + e
		}
		if !slices.Contains(exts, e) {
			exts = append(exts, e)
		}
	}
	slices.Sort(exts)
	a.Extensions = exts
	// Türlerin hepsi tanınıyorsa sıralı ve tekrarsız yazılır; tanınmayan varsa doğrulama reddeder.
	types := make([]string, 0, 2)
	for _, t := range []string{SubmitFile, SubmitText} {
		if slices.Contains(a.SubmissionTypes, t) {
			types = append(types, t)
		}
	}
	if !slices.ContainsFunc(a.SubmissionTypes, func(t string) bool { return t != SubmitFile && t != SubmitText }) {
		a.SubmissionTypes = types
	}
}

func (a AssignmentSettings) validate(maxUpload int64) error {
	switch {
	case a.MaxAttempts < 1 || a.MaxAttempts > 10:
		return &ValidationError{Field: "assignment.max_attempts", Message: "1 ile 10 arasında olmalı."}
	case len(a.SubmissionTypes) == 0 || len(a.SubmissionTypes) > 2:
		return &ValidationError{Field: "assignment.submission_types", Message: "FILE, ONLINE_TEXT ya da ikisi olmalı."}
	case a.MaxFiles < 1 || a.MaxFiles > 20:
		return &ValidationError{Field: "assignment.max_files", Message: "1 ile 20 arasında olmalı."}
	case a.MaxFileSize < 1 || a.MaxFileSize > maxUpload:
		return &ValidationError{Field: "assignment.max_file_size_bytes",
			Message: fmt.Sprintf("1 bayt ile %d MB arasında olmalı.", maxUpload>>20)}
	case a.MaxScore < 1 || a.MaxScore > 100_000:
		return &ValidationError{Field: "assignment.max_score", Message: "0,01 ile 1000 arasında olmalı."}
	case a.LatePolicy != LateAllow && a.LatePolicy != LateDeny && a.LatePolicy != LatePenalty:
		return &ValidationError{Field: "assignment.late_policy", Message: "ALLOW, DENY ya da PENALTY olmalı."}
	case a.LatePolicy == LatePenalty && (a.PenaltyPerDay < 1 || a.PenaltyPerDay > 10_000):
		return &ValidationError{Field: "assignment.late_penalty_pct_per_day", Message: "0,01 ile 100 arasında olmalı."}
	case a.LatePolicy == LateDeny && a.CutoffAt != nil:
		return &ValidationError{Field: "assignment.cutoff_at", Message: "Geç teslim kapalıyken kesin kapanış son tarihtir."}
	case a.LatePolicy != LateAllow && a.DueAt == nil:
		return &ValidationError{Field: "assignment.due_at", Message: "Geç teslim politikası için son tarih gerekli."}
	case a.OpensAt != nil && a.DueAt != nil && !a.OpensAt.Before(*a.DueAt):
		return &ValidationError{Field: "assignment.due_at", Message: "Açılıştan sonra olmalı."}
	case a.DueAt != nil && a.CutoffAt != nil && a.CutoffAt.Before(*a.DueAt):
		return &ValidationError{Field: "assignment.cutoff_at", Message: "Son tarihten önce olamaz."}
	case a.OpensAt != nil && a.CutoffAt != nil && !a.OpensAt.Before(*a.CutoffAt):
		return &ValidationError{Field: "assignment.cutoff_at", Message: "Açılıştan sonra olmalı."}
	}
	for _, t := range a.SubmissionTypes {
		if t != SubmitFile && t != SubmitText {
			return &ValidationError{Field: "assignment.submission_types", Message: "FILE, ONLINE_TEXT ya da ikisi olmalı."}
		}
	}
	allowed := files.Extensions()
	for _, e := range a.Extensions {
		if !slices.Contains(allowed, e) {
			return &ValidationError{Field: "assignment.allowed_extensions", Message: "İzin verilmeyen bir dosya türü: " + e}
		}
	}
	return nil
}

// deadline, teslimin kabul edildiği son andır: geç teslim kapalıysa son tarih, açıksa kesin
// kapanış (yoksa sınırsız).
func (a AssignmentSettings) deadline() *time.Time {
	if a.LatePolicy == LateDeny {
		return a.DueAt
	}
	return a.CutoffAt
}

// SubmissionState, verilen anda teslimin durumudur.
type SubmissionState string

// Teslim durumları: henüz açılmadı, açık, kapandı.
const (
	SubmissionNotOpen SubmissionState = "NOT_OPEN"
	SubmissionOpen    SubmissionState = "OPEN"
	SubmissionClosed  SubmissionState = "CLOSED"
)

// StateAt, verilen anda teslimin kabul edilip edilmediğidir.
func (a AssignmentSettings) StateAt(now time.Time) SubmissionState {
	if a.OpensAt != nil && now.Before(*a.OpensAt) {
		return SubmissionNotOpen
	}
	if d := a.deadline(); d != nil && now.After(*d) {
		return SubmissionClosed
	}
	return SubmissionOpen
}

// LateAt, verilen anda yapılan teslimin geç olduğunu söyler.
func (a AssignmentSettings) LateAt(at time.Time) bool {
	return a.DueAt != nil && at.After(*a.DueAt)
}

// Penalty, verilen anda gönderilen teslimin geç kesintisidir (yüzde, yüzde birlik): PENALTY
// politikasında son tarihten sonraki başlamış her gün için PenaltyPerDay, en çok %100.
func (a AssignmentSettings) Penalty(submittedAt *time.Time) grading.Points {
	if a.LatePolicy != LatePenalty || submittedAt == nil || !a.LateAt(*submittedAt) {
		return 0
	}
	late := submittedAt.Sub(*a.DueAt)
	days := grading.Points((late + 24*time.Hour - 1) / (24 * time.Hour))
	return min(days*a.PenaltyPerDay, 10_000)
}

// ApplyPenalty, puandan yüzde kesintiyi düşer (yüzde birliğe yarım yukarı yuvarlanır).
func ApplyPenalty(score, pct grading.Points) grading.Points {
	return (score*(10_000-pct) + 5_000) / 10_000
}

// AllowsFile, dosyanın ödevin tür ve boyut sınırlarına uyduğunu söyler; uymuyorsa nedenini yazar.
func (a AssignmentSettings) AllowsFile(name string, size int64) string {
	if size > a.MaxFileSize {
		return fmt.Sprintf("%s: dosya en çok %s olabilir.", name, humanBytes(a.MaxFileSize))
	}
	if len(a.Extensions) > 0 && !slices.Contains(a.Extensions, strings.ToLower(path.Ext(name))) {
		return fmt.Sprintf("%s: bu ödevde kabul edilen türler %s.", name, strings.Join(a.Extensions, ", "))
	}
	return ""
}

func humanBytes(n int64) string {
	switch {
	case n >= 1<<20 && n%(1<<20) == 0:
		return fmt.Sprintf("%d MB", n>>20)
	case n >= 1<<10:
		return fmt.Sprintf("%d KB", n>>10)
	}
	return fmt.Sprintf("%d bayt", n)
}

// writeAssignment, ödevin ayarlarını yazar.
func writeAssignment(ctx context.Context, tx pgx.Tx, id string, a AssignmentSettings) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO lms.assignments (activity_id, opens_at, due_at, cutoff_at, max_attempts, submission_types, max_files,
		                             max_file_size_bytes, allowed_extensions, max_score, late_policy, late_penalty_pct_per_day)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::numeric / 100, $11, $12::numeric / 100)
		ON CONFLICT (activity_id) DO UPDATE SET
			opens_at = excluded.opens_at, due_at = excluded.due_at, cutoff_at = excluded.cutoff_at,
			max_attempts = excluded.max_attempts, submission_types = excluded.submission_types, max_files = excluded.max_files,
			max_file_size_bytes = excluded.max_file_size_bytes, allowed_extensions = excluded.allowed_extensions,
			max_score = excluded.max_score, late_policy = excluded.late_policy,
			late_penalty_pct_per_day = excluded.late_penalty_pct_per_day`,
		id, a.OpensAt, a.DueAt, a.CutoffAt, a.MaxAttempts, a.SubmissionTypes, a.MaxFiles, a.MaxFileSize, a.Extensions,
		int64(a.MaxScore), a.LatePolicy, int64(a.PenaltyPerDay))
	if err != nil {
		return fmt.Errorf("lms: ödev yazılamadı: %w", err)
	}
	return nil
}

const assignmentColumns = `opens_at, due_at, cutoff_at, max_attempts, submission_types, max_files, max_file_size_bytes,
	allowed_extensions, (max_score * 100)::int8, late_policy, (late_penalty_pct_per_day * 100)::int8, grades_released_at`

// scanAssignment, assignmentColumns'u okur; prefix, sorguda onlardan önce gelen sütunlardır.
func scanAssignment(row pgx.Row, prefix ...any) (AssignmentSettings, error) {
	var (
		a            AssignmentSettings
		score, pct   int64
		attempts, mf int16
	)
	dest := append(prefix, &a.OpensAt, &a.DueAt, &a.CutoffAt, &attempts, &a.SubmissionTypes, &mf, &a.MaxFileSize,
		&a.Extensions, &score, &a.LatePolicy, &pct, &a.ReleasedAt)
	err := row.Scan(dest...)
	a.MaxAttempts, a.MaxFiles, a.MaxScore, a.PenaltyPerDay = int(attempts), int(mf), grading.Points(score), grading.Points(pct)
	return a, err
}

// assignments, etkinliklerin ödev ayarlarıdır.
func assignments(ctx context.Context, q db.Querier, ids []string) (map[string]AssignmentSettings, error) {
	rows, err := q.Query(ctx, `SELECT activity_id, `+assignmentColumns+` FROM lms.assignments WHERE activity_id = ANY($1)`, ids)
	if err != nil {
		return nil, fmt.Errorf("lms: ödevler okunamadı: %w", err)
	}
	defer rows.Close()
	out := map[string]AssignmentSettings{}
	for rows.Next() {
		var id string
		a, err := scanAssignment(rows, &id)
		if err != nil {
			return nil, fmt.Errorf("lms: ödevler okunamadı: %w", err)
		}
		out[id] = a
	}
	return out, rows.Err()
}
