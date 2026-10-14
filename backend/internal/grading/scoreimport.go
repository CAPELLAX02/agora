package grading

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// ImportStatus, içe aktarılan bir satırın sonucudur.
type ImportStatus string

// Satır sonuçları.
const (
	ImportChanged   ImportStatus = "CHANGED"
	ImportUnchanged ImportStatus = "UNCHANGED"
	ImportEmpty     ImportStatus = "EMPTY" // puan hücresi boş: not değişmez
	ImportError     ImportStatus = "ERROR"
)

// Satır hatalarının kodları.
const (
	ImportMissingStudentNo = "MISSING_STUDENT_NO"
	ImportNotInSection     = "NOT_IN_SECTION"
	ImportDuplicate        = "DUPLICATE_STUDENT"
	ImportInvalidScore     = "INVALID_SCORE"
	ImportNotEligible      = "NOT_ELIGIBLE"
)

// ImportRow, dosyanın bir satırının önizlemesidir. Current nil ise not girilmemiştir; Next
// boş hücrede ve hatalı satırda nil.
type ImportRow struct {
	Line      int
	StudentNo string
	Student   *GradebookStudent
	Status    ImportStatus
	Current   *Score
	Next      *Score
	ErrorCode string
	Error     string
}

// ScoreImport, bir bileşene CSV'den not aktarmanın satır satır önizlemesidir. Version,
// önizlemenin dayandığı not sürümüdür: uygularken iyimser kilit.
type ScoreImport struct {
	ComponentID string
	Version     int
	Column      string
	Rows        []ImportRow
	Missing     []GradebookStudent // şubenin dosyada olmayan öğrencileri
	Changed     int
	Unchanged   int
	Empty       int
	Errors      int
}

// ImportRowsError: dosyada hatalı satırlar var, hiçbir not yazılmadı.
type ImportRowsError struct {
	Rows []ImportRow
}

func (e *ImportRowsError) Error() string {
	return fmt.Sprintf("grading: CSV'de %d hatalı satır", len(e.Rows))
}

// PreviewImport, dosyadaki notların bileşene nasıl yazılacağını gösterir; hiçbir şey kaydedilmez.
// Not girme koşulları (aşama, kilitli plan, açık pencere) yazmadakiyle aynıdır.
func (s *Service) PreviewImport(ctx context.Context, ref ComponentRef, data []byte) (ScoreImport, error) {
	if err := s.checkEntry(ctx, ref); err != nil {
		return ScoreImport{}, err
	}
	components, err := s.repo.components(ctx, s.pool, ref.Section.SectionID)
	if err != nil {
		return ScoreImport{}, err
	}
	i := slices.IndexFunc(components, func(c Component) bool { return c.ID == ref.ID })
	if i < 0 {
		return ScoreImport{}, ErrNotFound
	}
	file, err := readScoreCSV(data, []string{componentLabels(components, false)[i], componentLabels(components, true)[i]})
	if err != nil {
		return ScoreImport{}, err
	}

	res := ScoreImport{ComponentID: ref.ID, Column: file.column}
	if err := s.pool.QueryRow(ctx, `SELECT scores_version FROM grading.assessment_components WHERE id = $1`,
		ref.ID).Scan(&res.Version); errors.Is(err, pgx.ErrNoRows) {
		return ScoreImport{}, ErrNotFound
	} else if err != nil {
		return ScoreImport{}, fmt.Errorf("grading: bileşen okunamadı: %w", err)
	}
	students, err := s.gradebookStudents(ctx, ref.Section.SectionID)
	if err != nil {
		return ScoreImport{}, err
	}
	var allowed []string
	if ref.Category == CategoryMakeup {
		if allowed, err = loadMakeupLetters(ctx, s.pool, s.now()); err != nil {
			return ScoreImport{}, err
		}
	}

	byNo := make(map[string]int, len(students))
	for i, st := range students {
		byNo[st.StudentNo] = i
	}
	lines := map[string][]int{} // öğrenci no → puanı dolu satırları (tekrar denetimi)
	for _, l := range file.lines {
		if strings.TrimSpace(l.cell) != "" {
			lines[l.studentNo] = append(lines[l.studentNo], l.line)
		}
	}
	seen := make([]bool, len(students))
	for _, l := range file.lines {
		row := ImportRow{Line: l.line, StudentNo: l.studentNo}
		idx, inSection := byNo[l.studentNo]
		if inSection {
			st := students[idx]
			row.Student, seen[idx] = &st, true
			if sc, ok := st.Scores[ref.ID]; ok {
				row.Current = &sc
			}
		}
		// Boş hücre hiçbir şeyi değiştirmez: satırın geri kalanı denetlenmez.
		score, empty, valid := parseScoreCell(l.cell)
		switch {
		case empty:
			row.Status = ImportEmpty
		case l.studentNo == "":
			row.ErrorCode, row.Error = ImportMissingStudentNo, "Öğrenci numarası boş."
		case !inSection:
			row.ErrorCode, row.Error = ImportNotInSection, "Bu numarada şubenin kesin kayıtlı öğrencisi yok."
		case len(lines[l.studentNo]) > 1:
			row.ErrorCode, row.Error = ImportDuplicate, "Öğrenci dosyada birden fazla kez geçiyor (satır "+joinInts(lines[l.studentNo])+")."
		case !valid:
			row.ErrorCode, row.Error = ImportInvalidScore, "Puan 0 ile 100 arasında, en fazla iki ondalıklı olmalı; girmedi için G yazın."
		case ref.Category == CategoryMakeup && !makeupEligible(row.Student.Letter, row.Student.Attendance, allowed):
			row.ErrorCode, row.Error = ImportNotEligible, "Öğrencinin bütünleme hakkı yok."
		case row.Current != nil && *row.Current == score:
			row.Status, row.Next = ImportUnchanged, &score
		default:
			row.Status, row.Next = ImportChanged, &score
		}
		if row.ErrorCode != "" {
			row.Status = ImportError
		}
		switch row.Status {
		case ImportChanged:
			res.Changed++
		case ImportUnchanged:
			res.Unchanged++
		case ImportEmpty:
			res.Empty++
		case ImportError:
			res.Errors++
		}
		res.Rows = append(res.Rows, row)
	}
	for i, st := range students {
		if !seen[i] {
			res.Missing = append(res.Missing, st)
		}
	}
	return res, nil
}

// ImportScores, dosyadaki notları bileşene yazar: önce önizlemedeki gibi doğrulanır, hatalı
// satır varsa hiçbir not yazılmaz (*ImportRowsError). Değişen notlar tek transaction'da
// yazılır (kaynak CSV); version önizlemenin not sürümüdür.
func (s *Service) ImportScores(ctx context.Context, actorID string, ref ComponentRef, version int, data []byte) (ScoreImport, error) {
	res, err := s.PreviewImport(ctx, ref, data)
	if err != nil {
		return ScoreImport{}, err
	}
	if res.Errors > 0 {
		var bad []ImportRow
		for _, r := range res.Rows {
			if r.Status == ImportError {
				bad = append(bad, r)
			}
		}
		return ScoreImport{}, &ImportRowsError{Rows: bad}
	}
	var items []ScoreInput
	for _, r := range res.Rows {
		if r.Status != ImportChanged {
			continue
		}
		in := ScoreInput{EnrollmentID: r.Student.EnrollmentID, Absent: r.Next.Absent}
		if !r.Next.Absent {
			p := r.Next.Points
			in.Points = &p
		}
		items = append(items, in)
	}
	if len(items) == 0 {
		if version != res.Version {
			return ScoreImport{}, ErrVersionMismatch
		}
		return res, nil
	}
	if _, err := s.writeScores(ctx, actorID, ref, version, items, sourceCSV); err != nil {
		return ScoreImport{}, err
	}
	return res, nil
}

func joinInts(ns []int) string {
	parts := make([]string, len(ns))
	for i, n := range ns {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ", ")
}
