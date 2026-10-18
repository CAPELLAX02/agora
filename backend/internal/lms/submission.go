package lms

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/microcosm-cc/bluemonday"

	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/files"
	"github.com/CAPELLAX02/agora/backend/internal/grading"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
)

// ErrPreconditionRequired, var olan bir kaydın sürümsüz (If-Match'siz) güncellenmesidir.
var ErrPreconditionRequired = errors.New("lms: sürüm gerekli")

// ConflictError, isteğin kaydın durumuyla çeliştiğidir (ör. ödev kapandı, deneme hakkı bitti).
type ConflictError struct {
	Code   string
	Detail string
}

func (e *ConflictError) Error() string { return "lms: " + e.Code + ": " + e.Detail }

// Submission, öğrencinin bir denemesidir.
type Submission struct {
	ID          string
	AttemptNo   int
	Status      string
	Text        string
	Files       []FileRef
	FileCount   int
	SubmittedAt *time.Time
	Late        bool
	Version     int
	UpdatedAt   time.Time
}

// Grade, öğrencinin ödev puanı ve geri bildirimidir. Final, geç teslim kesintisi düşülmüş puandır.
type Grade struct {
	Score     *grading.Points
	Penalty   grading.Points
	Final     *grading.Points
	Feedback  string
	AttemptNo *int
	GradedBy  string
	GradedAt  time.Time
	Version   int
}

// Comment, bir gönderim yorumudur.
type Comment struct {
	ID         string
	AuthorID   string
	AuthorName string
	Body       string
	CreatedAt  time.Time
}

// Work, bir öğrencinin ödevdeki izidir: denemeler (eskiden yeniye), puan ve yorumlar.
type Work struct {
	Attempts []Submission
	Grade    *Grade
	Comments []Comment
}

// Latest, son denemedir; hiç deneme yoksa nil.
func (w Work) Latest() *Submission {
	if len(w.Attempts) == 0 {
		return nil
	}
	return &w.Attempts[len(w.Attempts)-1]
}

// plain, zengin metnin görünür yazısıdır (boş teslimi ayırt etmek için).
var plain = bluemonday.StrictPolicy()

func emptyHTML(html string) bool {
	return strings.TrimSpace(strings.ReplaceAll(plain.Sanitize(html), "&nbsp;", "")) == ""
}

// Assignment, ödevin ayarlarıdır.
func (s *Service) Assignment(ctx context.Context, q db.Querier, activityID string) (AssignmentSettings, error) {
	a, err := scanAssignment(q.QueryRow(ctx, `SELECT `+assignmentColumns+` FROM lms.assignments WHERE activity_id = $1`, activityID))
	if errors.Is(err, pgx.ErrNoRows) {
		return AssignmentSettings{}, ErrNotFound
	}
	if err != nil {
		return AssignmentSettings{}, fmt.Errorf("lms: ödev okunamadı: %w", err)
	}
	return a, nil
}

// lockAssignment, ödevin ayarlarını işlem boyunca değişmeyecek biçimde okur.
func lockAssignment(ctx context.Context, tx pgx.Tx, activityID string) (AssignmentSettings, error) {
	a, err := scanAssignment(tx.QueryRow(ctx, `SELECT `+assignmentColumns+` FROM lms.assignments WHERE activity_id = $1 FOR SHARE`, activityID))
	if errors.Is(err, pgx.ErrNoRows) {
		return AssignmentSettings{}, ErrNotFound
	}
	if err != nil {
		return AssignmentSettings{}, fmt.Errorf("lms: ödev okunamadı: %w", err)
	}
	return a, nil
}

// Work, öğrencinin ödevdeki izidir. withGrade false ise puan okunmaz (yayımlanmamış).
func (s *Service) Work(ctx context.Context, st AssignmentSettings, activityID, userID string, withGrade bool) (Work, error) {
	var w Work
	rows, err := s.pool.Query(ctx, `
		SELECT id, attempt_no, status, online_text_html, submitted_at, is_late, version, updated_at
		FROM lms.assignment_submissions WHERE activity_id = $1 AND user_id = $2 ORDER BY attempt_no`, activityID, userID)
	if err != nil {
		return Work{}, fmt.Errorf("lms: teslimler okunamadı: %w", err)
	}
	w.Attempts, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Submission, error) {
		var sub Submission
		err := row.Scan(&sub.ID, &sub.AttemptNo, &sub.Status, &sub.Text, &sub.SubmittedAt, &sub.Late, &sub.Version, &sub.UpdatedAt)
		return sub, err
	})
	if err != nil {
		return Work{}, fmt.Errorf("lms: teslimler okunamadı: %w", err)
	}
	if len(w.Attempts) > 0 {
		ids := make([]string, len(w.Attempts))
		index := map[string]int{}
		for i, sub := range w.Attempts {
			ids[i], index[sub.ID] = sub.ID, i
		}
		rows, err := s.pool.Query(ctx, `
			SELECT sf.submission_id, f.id, f.original_name, f.mime_type, f.size_bytes
			FROM lms.submission_files sf JOIN platform.files f ON f.id = sf.file_id
			WHERE sf.submission_id = ANY($1) AND f.deleted_at IS NULL ORDER BY sf.position`, ids)
		if err != nil {
			return Work{}, fmt.Errorf("lms: teslim dosyaları okunamadı: %w", err)
		}
		var (
			subID string
			f     FileRef
		)
		_, err = pgx.ForEachRow(rows, []any{&subID, &f.ID, &f.Name, &f.MimeType, &f.Size}, func() error {
			i := index[subID]
			w.Attempts[i].Files = append(w.Attempts[i].Files, f)
			w.Attempts[i].FileCount++
			return nil
		})
		if err != nil {
			return Work{}, fmt.Errorf("lms: teslim dosyaları okunamadı: %w", err)
		}
	}
	if withGrade {
		grades, err := s.grades(ctx, st, activityID, []string{userID})
		if err != nil {
			return Work{}, err
		}
		if g, ok := grades[userID]; ok {
			w.Grade = &g
		}
	}
	w.Comments, err = s.comments(ctx, activityID, userID)
	return w, err
}

// grades, öğrencilerin puanlarıdır; geç kesintisi puanlanan denemenin teslim anından hesaplanır.
func (s *Service) grades(ctx context.Context, st AssignmentSettings, activityID string, userIDs []string) (map[string]Grade, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT g.user_id, (g.score * 100)::int8, g.feedback_html, g.attempt_no, p.first_name || ' ' || p.last_name,
		       g.graded_at, g.version, sub.submitted_at
		FROM lms.assignment_grades g
		JOIN iam.users u ON u.id = g.graded_by JOIN people.persons p ON p.id = u.person_id
		LEFT JOIN lms.assignment_submissions sub
		       ON sub.activity_id = g.activity_id AND sub.user_id = g.user_id AND sub.attempt_no = g.attempt_no
		WHERE g.activity_id = $1 AND g.user_id = ANY($2)`, activityID, userIDs)
	if err != nil {
		return nil, fmt.Errorf("lms: puanlar okunamadı: %w", err)
	}
	out := map[string]Grade{}
	var (
		userID      string
		score       *int64
		attempt     *int16
		g           Grade
		submittedAt *time.Time
	)
	_, err = pgx.ForEachRow(rows, []any{&userID, &score, &g.Feedback, &attempt, &g.GradedBy, &g.GradedAt, &g.Version, &submittedAt}, func() error {
		gr := g
		if attempt != nil {
			n := int(*attempt)
			gr.AttemptNo = &n
		}
		if score != nil {
			sc := grading.Points(*score)
			gr.Score = &sc
			gr.Penalty = st.Penalty(submittedAt)
			final := ApplyPenalty(sc, gr.Penalty)
			gr.Final = &final
		}
		out[userID] = gr
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("lms: puanlar okunamadı: %w", err)
	}
	return out, nil
}

// comments, öğrencinin teslimi üzerindeki yorumlardır (eskiden yeniye).
func (s *Service) comments(ctx context.Context, activityID, studentID string) ([]Comment, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT c.id, c.author_id, p.first_name || ' ' || p.last_name, c.body, c.created_at
		FROM lms.submission_comments c
		JOIN iam.users u ON u.id = c.author_id JOIN people.persons p ON p.id = u.person_id
		WHERE c.activity_id = $1 AND c.student_id = $2 ORDER BY c.created_at, c.id`, activityID, studentID)
	if err != nil {
		return nil, fmt.Errorf("lms: yorumlar okunamadı: %w", err)
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Comment, error) {
		var c Comment
		err := row.Scan(&c.ID, &c.AuthorID, &c.AuthorName, &c.Body, &c.CreatedAt)
		return c, err
	})
	if err != nil {
		return nil, fmt.Errorf("lms: yorumlar okunamadı: %w", err)
	}
	return list, nil
}

// latestAttempt, öğrencinin son denemesini kilitleyerek okur; yoksa nil.
func latestAttempt(ctx context.Context, tx pgx.Tx, activityID, userID string) (*Submission, []string, error) {
	var sub Submission
	err := tx.QueryRow(ctx, `
		SELECT id, attempt_no, status, online_text_html, submitted_at, is_late, version, updated_at
		FROM lms.assignment_submissions WHERE activity_id = $1 AND user_id = $2
		ORDER BY attempt_no DESC LIMIT 1 FOR UPDATE`, activityID, userID).
		Scan(&sub.ID, &sub.AttemptNo, &sub.Status, &sub.Text, &sub.SubmittedAt, &sub.Late, &sub.Version, &sub.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("lms: teslim okunamadı: %w", err)
	}
	rows, err := tx.Query(ctx, `SELECT file_id FROM lms.submission_files WHERE submission_id = $1 ORDER BY position`, sub.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("lms: teslim dosyaları okunamadı: %w", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, nil, fmt.Errorf("lms: teslim dosyaları okunamadı: %w", err)
	}
	return &sub, ids, nil
}

// requireOpen, teslimin şu an kabul edildiğini denetler.
func requireOpen(st AssignmentSettings, now time.Time) error {
	switch st.StateAt(now) {
	case SubmissionNotOpen:
		return &ConflictError{Code: "SUBMISSION_NOT_OPEN", Detail: "Bu ödev henüz teslime açılmadı."}
	case SubmissionClosed:
		return &ConflictError{Code: "SUBMISSION_CLOSED", Detail: "Bu ödevin teslim süresi doldu."}
	}
	return nil
}

// DraftInput, teslim taslağının içeriğidir.
type DraftInput struct {
	Text    string
	FileIDs []string
}

// SaveDraft, öğrencinin taslağını yazar. Hiç deneme yoksa ilk deneme oluşur (version 0); varsa
// son deneme taslak olmalı ve sürümü tutmalıdır. Yeni dosyalar taslağa bağlanır (yükleyen
// öğrenci olmalı; önceki denemelerinin dosyaları yeniden eklenebilir), çıkarılanlar hiçbir
// teslimde kalmadıysa serbest bırakılır.
func (s *Service) SaveDraft(ctx context.Context, userID, activityID string, version int, in DraftInput) error {
	if len(in.Text) > 200_000 {
		return &ValidationError{Field: "online_text_html", Message: "Metin çok uzun."}
	}
	for i, f := range in.FileIDs {
		if slices.Contains(in.FileIDs[:i], f) {
			return &ValidationError{Field: "file_ids", Message: "Aynı dosya iki kez eklenemez."}
		}
	}
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		st, err := lockAssignment(ctx, tx, activityID)
		if err != nil {
			return err
		}
		if err := requireOpen(st, s.now()); err != nil {
			return err
		}
		text := Sanitize(in.Text)
		if emptyHTML(text) {
			text = ""
		}
		switch {
		case text != "" && !st.Accepts(SubmitText):
			return &ValidationError{Field: "online_text_html", Message: "Bu ödev çevrimiçi metin kabul etmiyor."}
		case len(in.FileIDs) > 0 && !st.Accepts(SubmitFile):
			return &ValidationError{Field: "file_ids", Message: "Bu ödev dosya kabul etmiyor."}
		case len(in.FileIDs) > st.MaxFiles:
			return &ValidationError{Field: "file_ids", Message: fmt.Sprintf("En çok %d dosya yüklenebilir.", st.MaxFiles)}
		}
		latest, current, err := latestAttempt(ctx, tx, activityID, userID)
		if err != nil {
			return err
		}
		var subID string
		switch {
		case latest == nil && version != 0:
			return ErrVersionMismatch
		case latest == nil:
			err := tx.QueryRow(ctx, `
				INSERT INTO lms.assignment_submissions (activity_id, user_id, attempt_no, online_text_html)
				VALUES ($1, $2, 1, $3) ON CONFLICT DO NOTHING RETURNING id`, activityID, userID, text).Scan(&subID)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrVersionMismatch // aynı anda başka bir istek oluşturdu
			}
			if err != nil {
				return fmt.Errorf("lms: taslak oluşturulamadı: %w", err)
			}
		case latest.Status == StatusSubmitted:
			return &ConflictError{Code: "ALREADY_SUBMITTED", Detail: "Bu deneme gönderildi; değiştirmek için yeni deneme başlatın."}
		case version == 0:
			return ErrPreconditionRequired
		case latest.Version != version:
			return ErrVersionMismatch
		default:
			subID = latest.ID
			if _, err := tx.Exec(ctx, `
				UPDATE lms.assignment_submissions SET online_text_html = $2, version = version + 1, updated_at = now()
				WHERE id = $1`, subID, text); err != nil {
				return fmt.Errorf("lms: taslak yazılamadı: %w", err)
			}
		}
		return s.syncSubmissionFiles(ctx, tx, st, activityID, userID, subID, current, in.FileIDs)
	})
}

// syncSubmissionFiles, taslağın dosyalarını want'a eşitler.
func (s *Service) syncSubmissionFiles(ctx context.Context, tx pgx.Tx, st AssignmentSettings, activityID, userID, subID string, current, want []string) error {
	var removed []string
	for _, f := range current {
		if !slices.Contains(want, f) {
			removed = append(removed, f)
		}
	}
	if len(removed) > 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM lms.submission_files WHERE submission_id = $1 AND file_id = ANY($2)`, subID, removed); err != nil {
			return fmt.Errorf("lms: dosyalar çıkarılamadı: %w", err)
		}
		if err := releaseFiles(ctx, tx, `SELECT id FROM unnest($1::uuid[]) id
			WHERE NOT EXISTS (SELECT 1 FROM lms.submission_files sf WHERE sf.file_id = id)`, removed); err != nil {
			return err
		}
	}
	for i, id := range want {
		if !slices.Contains(current, id) {
			f, err := s.claimSubmissionFile(ctx, tx, activityID, userID, id)
			if err != nil {
				return err
			}
			if msg := st.AllowsFile(f.Name, f.Size); msg != "" {
				return &ValidationError{Field: "file_ids", Message: msg}
			}
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO lms.submission_files (submission_id, file_id, position) VALUES ($1, $2, $3)
			ON CONFLICT (submission_id, file_id) DO UPDATE SET position = excluded.position`, subID, id, i); err != nil {
			return fmt.Errorf("lms: dosya eklenemedi: %w", err)
		}
	}
	return nil
}

// claimSubmissionFile, taslağa eklenen dosyayı döndürür: öğrencinin önceki bir denemesindeyse
// olduğu gibi, değilse öğrencinin yüklediği bağlanmamış dosya bağlanarak.
func (s *Service) claimSubmissionFile(ctx context.Context, tx pgx.Tx, activityID, userID, id string) (files.File, error) {
	var mine bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM lms.submission_files sf JOIN lms.assignment_submissions sub ON sub.id = sf.submission_id
		               WHERE sf.file_id = $1 AND sub.activity_id = $2 AND sub.user_id = $3)`, id, activityID, userID).Scan(&mine)
	if err != nil {
		return files.File{}, fmt.Errorf("lms: dosya okunamadı: %w", err)
	}
	if mine {
		return s.files.Get(ctx, tx, id)
	}
	f, err := s.files.Attach(ctx, tx, id, userID)
	if errors.Is(err, files.ErrNotFound) || errors.Is(err, files.ErrAttached) {
		return files.File{}, &ValidationError{Field: "file_ids", Message: "Dosya bulunamadı ya da başka bir kayda bağlı: " + id}
	}
	return f, err
}

// hasContent, denemenin ödevin istediği bir içeriği olduğunu söyler.
func hasContent(st AssignmentSettings, text string, fileCount int) bool {
	return (st.Accepts(SubmitText) && !emptyHTML(text)) || (st.Accepts(SubmitFile) && fileCount > 0)
}

// Submit, son taslağı gönderir: deneme kilitlenir, son tarihten sonraysa geç işaretlenir;
// ödev öğrenci için tamamlanmış olur.
func (s *Service) Submit(ctx context.Context, userID, activityID string, version int) error {
	var late, completed bool
	err := db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		st, err := lockAssignment(ctx, tx, activityID)
		if err != nil {
			return err
		}
		latest, fileIDs, err := latestAttempt(ctx, tx, activityID, userID)
		if err != nil {
			return err
		}
		switch {
		case latest == nil:
			return &ConflictError{Code: "EMPTY_SUBMISSION", Detail: "Göndermeden önce dosya ya da metin ekleyin."}
		case latest.Status == StatusSubmitted:
			return &ConflictError{Code: "ALREADY_SUBMITTED", Detail: "Bu deneme zaten gönderildi."}
		case latest.Version != version:
			return ErrVersionMismatch
		}
		now := s.now()
		if err := requireOpen(st, now); err != nil {
			return err
		}
		if !hasContent(st, latest.Text, len(fileIDs)) {
			return &ConflictError{Code: "EMPTY_SUBMISSION", Detail: "Göndermeden önce dosya ya da metin ekleyin."}
		}
		late = st.LateAt(now)
		if _, err := tx.Exec(ctx, `
			UPDATE lms.assignment_submissions SET status = 'SUBMITTED', submitted_at = $2, is_late = $3,
			       version = version + 1, updated_at = now()
			WHERE id = $1`, latest.ID, now, late); err != nil {
			return fmt.Errorf("lms: teslim gönderilemedi: %w", err)
		}
		tag, err := tx.Exec(ctx, `
			INSERT INTO lms.activity_completions (activity_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			activityID, userID)
		if err != nil {
			return fmt.Errorf("lms: tamamlama yazılamadı: %w", err)
		}
		completed = tag.RowsAffected() > 0
		return audit.Record(ctx, tx, audit.Entry{ActorUserID: userID, Action: "lms_submission.submit", EntityType: "lms_submission",
			EntityID: latest.ID, After: map[string]any{"activity_id": activityID, "attempt_no": latest.AttemptNo, "late": late, "files": len(fileIDs)}})
	})
	if err != nil {
		return err
	}
	s.metrics.Submitted(late)
	if completed {
		s.metrics.Completed("assignment")
	}
	return nil
}

// NewAttempt, gönderilmiş son denemenin içeriğiyle yeni bir taslak deneme başlatır (deneme hakkı
// kaldıysa ve teslim açıksa).
func (s *Service) NewAttempt(ctx context.Context, userID, activityID string) error {
	return db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		st, err := lockAssignment(ctx, tx, activityID)
		if err != nil {
			return err
		}
		latest, fileIDs, err := latestAttempt(ctx, tx, activityID, userID)
		if err != nil {
			return err
		}
		switch {
		case latest == nil || latest.Status != StatusSubmitted:
			return &ConflictError{Code: "NOT_SUBMITTED", Detail: "Yeni deneme, önceki deneme gönderildikten sonra başlatılır."}
		case latest.AttemptNo >= st.MaxAttempts:
			return &ConflictError{Code: "NO_ATTEMPTS_LEFT", Detail: fmt.Sprintf("Deneme hakkınız doldu (%d).", st.MaxAttempts)}
		}
		if err := requireOpen(st, s.now()); err != nil {
			return err
		}
		var id string
		if err := tx.QueryRow(ctx, `
			INSERT INTO lms.assignment_submissions (activity_id, user_id, attempt_no, online_text_html)
			VALUES ($1, $2, $3, $4) RETURNING id`, activityID, userID, latest.AttemptNo+1, latest.Text).Scan(&id); err != nil {
			return fmt.Errorf("lms: deneme başlatılamadı: %w", err)
		}
		for i, f := range fileIDs {
			if _, err := tx.Exec(ctx, `INSERT INTO lms.submission_files (submission_id, file_id, position) VALUES ($1, $2, $3)`, id, f, i); err != nil {
				return fmt.Errorf("lms: dosyalar kopyalanamadı: %w", err)
			}
		}
		return audit.Record(ctx, tx, audit.Entry{ActorUserID: userID, Action: "lms_submission.new_attempt", EntityType: "lms_submission",
			EntityID: id, After: map[string]any{"activity_id": activityID, "attempt_no": latest.AttemptNo + 1}})
	})
}

// AddComment, öğrencinin teslimine bir yorum ekler (öğrencinin kendisi ya da öğretim elemanı).
func (s *Service) AddComment(ctx context.Context, authorID, activityID, studentID, body string) error {
	body = strings.TrimSpace(body)
	if n := len([]rune(body)); n < 1 || n > 2000 {
		return &ValidationError{Field: "body", Message: "1 ile 2000 karakter arasında olmalı."}
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO lms.submission_comments (activity_id, student_id, author_id, body) VALUES ($1, $2, $3, $4)`,
		activityID, studentID, authorID, body)
	if err != nil {
		return fmt.Errorf("lms: yorum eklenemedi: %w", err)
	}
	return nil
}

// SubmissionFileURL, öğrencinin bu ödevdeki bir teslim dosyasının imzalı indirme URL'sidir.
func (s *Service) SubmissionFileURL(ctx context.Context, activityID, studentID, fileID string) (string, time.Time, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM lms.submission_files sf JOIN lms.assignment_submissions sub ON sub.id = sf.submission_id
		               WHERE sf.file_id = $1 AND sub.activity_id = $2 AND sub.user_id = $3)`, fileID, activityID, studentID).Scan(&ok)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("lms: dosya okunamadı: %w", err)
	}
	if !ok {
		return "", time.Time{}, ErrNotFound
	}
	f, err := s.files.Get(ctx, s.pool, fileID)
	if err != nil {
		return "", time.Time{}, err
	}
	return s.files.DownloadURL(ctx, f)
}
