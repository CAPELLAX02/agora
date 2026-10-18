package lms

import (
	"context"
	"fmt"
	"html"
	"io"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CAPELLAX02/agora/backend/internal/audit"
	"github.com/CAPELLAX02/agora/backend/internal/grading"
	"github.com/CAPELLAX02/agora/backend/internal/platform/db"
	"github.com/CAPELLAX02/agora/backend/internal/platform/sheet"
)

// Ödev puanlarının Excel şablonu ve toplu girişi (yol haritası §5 #18): öğretim elemanı puanları
// ve geri bildirimleri sarı hücrelere girer, dosyayı Agora'da yükler; önce satır satır önizleme,
// sonra hatasız dosya tek transaction'da yazılır. Şablon ödeve özeldir (gizli kimlik sayfası).

// turkey, Türkiye saatidir (yıl boyu UTC+3); tarihler şablonda bu saatle yazılır.
var turkey = time.FixedZone("TRT", 3*60*60)

// Şablonun kimlik sayfasındaki anahtarlar.
const (
	gsMetaKind     = "kind"
	gsKind         = "assignment_grades"
	gsMetaActivity = "activity_id"
	gsMetaLabel    = "assignment"
	gsMetaScore    = "score"    // puan sütununun harfi
	gsMetaFeedback = "feedback" // geri bildirim sütununun harfi
)

const (
	gsMaxRows     = 1000 // en fazla öğrenci satırı
	gsHeaderScan  = 20   // başlık satırının arandığı ilk dolu satır sayısı
	gsMaxFeedback = 5000 // geri bildirim hücresinin en uzun hâli
)

var (
	gsStudentHeaders  = []string{"ogrencino", "ogrencinumarasi", "ogrno", "numara", "no", "studentno", "studentnumber", "studentid"}
	gsScoreHeaders    = []string{"puan", "not", "score", "grade", "points"}
	gsFeedbackHeaders = []string{"geribildirim", "feedback", "yorum", "aciklama", "comment", "comments"}
)

// gsText, şablonun bir dildeki metinleridir.
type gsText struct {
	sheet, guide, title, subtitle, created, max                  string
	columns                                                      [9]string // sıra, no, soyadı, adı, durum, gönderim, dosya, puan, geri bildirim
	none, draft, submitted, late, latePenalty                    string
	scoreNote, feedbackNote                                      string
	legend                                                       []string
	guideTitle, guideIntro                                       string
	guideHeaders                                                 []string
	sections                                                     []sheet.GuideSection
	scoreErrTitle, scoreErr, scorePromptTitle, scorePrompt       string
	feedbackErrTitle, feedbackErr, feedbackPromptT, feedbackProm string
	subject, language                                            string
	policy                                                       map[string]string
	due, noDue                                                   string
}

var gsTurkish = gsText{
	sheet: "Puanlar", guide: "Açıklamalar", title: "%s-%s · %s", subtitle: "%s · %d öğrenci · en yüksek puan %s",
	created: "Oluşturulma: %s · Agora", max: "En çok %s",
	columns: [9]string{"Sıra", "Öğrenci No", "Soyadı", "Adı", "Teslim Durumu", "Gönderim", "Dosya", "Puan", "Geri Bildirim"},
	none:    "Teslim edilmedi", draft: "Taslak (gönderilmedi)", submitted: "Gönderildi", late: "Geç gönderildi",
	latePenalty: "Geç gönderildi (%%%s kesinti)",
	scoreNote:   "Düzenlenebilir · en çok %s", feedbackNote: "Düzenlenebilir · düz metin",
	legend: []string{
		"Boş hücre puanı ve geri bildirimi değiştirmez; girilmiş bir puanı silmez.",
		"Geç teslim kesintisi Agora'da otomatik düşülür: buraya kesintisiz puanı yazın.",
		"Dosyayı Agora'da ödevin teslim listesinde “Excel'den içe aktar” ile yükleyin.",
	},
	guideTitle:   "Puan şablonu nasıl doldurulur?",
	guideIntro:   "Bu şablon Agora'dan “%s” ödevi için %s tarihinde üretildi. Puanları ve isterseniz geri bildirimleri “Puanlar” sayfasındaki sarı hücrelere girin, dosyayı kaydedip Agora'da yükleyin. Önce satır satır önizleme gösterilir; onaylayınca puanlar tek seferde yazılır.",
	guideHeaders: []string{"Ayar", "Değer"},
	sections: []sheet.GuideSection{
		{Heading: "Puanlar", Lines: []string{
			"0 ile ödevin en yüksek puanı arasında, en fazla iki ondalıklı bir sayı girin (ör. 45 ya da 45,5).",
			"Geç teslim kesintisi Agora'da hesaplanır; kesintisiz puanı yazın.",
			"Boş bırakılan hücre puanı değiştirmez; puan silmek için Agora'daki puanlama panelini kullanın.",
			"Teslim etmeyen öğrenciye de puan verilebilir.",
		}},
		{Heading: "Geri bildirim", Lines: []string{
			"Düz metin yazın; her satır ayrı paragraf olur. Boş hücre mevcut geri bildirimi korur.",
			"Agora'da biçimlendirilmiş (kalın, liste ...) bir geri bildirim şablonda düz metin görünür; değiştirmezseniz biçimi korunur.",
		}},
		{Heading: "Dikkat edilecekler", Lines: []string{
			"Öğrenci numaralarını ve sütun başlıklarını değiştirmeyin; satır eklemeyin, silmeyin.",
			"Sayfa kazara değişikliklere karşı korumalıdır (parolasız): yalnızca sarı hücreler düzenlenebilir.",
			"Dosya bu ödeve özeldir; başka bir ödeve yüklenirse reddedilir.",
			"Önizlemeden sonra puanlar başka biri tarafından değiştirilirse dosyayı yeniden yükleyin.",
		}},
	},
	scoreErrTitle: "Geçersiz puan", scoreErr: "0 ile %s arasında, en fazla iki ondalıklı bir puan girin.",
	scorePromptTitle: "Puan", scorePrompt: "0-%s arası puan. Boş hücre puanı değiştirmez.",
	feedbackErrTitle: "Geri bildirim çok uzun", feedbackErr: "Geri bildirim en çok 5000 karakter olabilir.",
	feedbackPromptT: "Geri bildirim", feedbackProm: "Düz metin; boş hücre mevcut geri bildirimi korur.",
	subject: "Agora ödev puan şablonu", language: "tr-TR",
	policy: map[string]string{LateAllow: "Geç teslim serbest (geç işaretlenir)", LatePenalty: "Geç teslim kesintili: günlük %%%s", LateDeny: "Geç teslim kabul edilmez"},
	due:    "Son tarih", noDue: "Yok",
}

var gsEnglish = gsText{
	sheet: "Grades", guide: "Instructions", title: "%s-%s · %s", subtitle: "%s · %d students · maximum grade %s",
	created: "Generated: %s · Agora", max: "Up to %s",
	columns: [9]string{"No.", "Student No", "Last Name", "First Name", "Submission", "Submitted", "Files", "Grade", "Feedback"},
	none:    "Not submitted", draft: "Draft (not submitted)", submitted: "Submitted", late: "Submitted late",
	latePenalty: "Submitted late (%s%% penalty)",
	scoreNote:   "Editable · up to %s", feedbackNote: "Editable · plain text",
	legend: []string{
		"An empty cell keeps the current grade and feedback; it never deletes a grade.",
		"Agora deducts the late penalty automatically: enter the grade before the penalty.",
		"Upload this file in Agora with “Import from Excel” on the assignment's submission list.",
	},
	guideTitle:   "How to fill in the grade sheet",
	guideIntro:   "Agora generated this template for the assignment “%s” on %s. Enter grades and, optionally, feedback in the yellow cells of the “Grades” sheet, save the file and upload it in Agora. A row-by-row preview comes first; on confirmation all grades are written at once.",
	guideHeaders: []string{"Setting", "Value"},
	sections: []sheet.GuideSection{
		{Heading: "Grades", Lines: []string{
			"Enter a number between 0 and the maximum grade with at most two decimals (e.g. 45 or 45.5).",
			"Agora computes the late penalty; enter the grade before the penalty.",
			"An empty cell keeps the current grade; use the grading panel in Agora to delete a grade.",
			"Students who did not submit can be graded too.",
		}},
		{Heading: "Feedback", Lines: []string{
			"Write plain text; each line becomes a paragraph. An empty cell keeps the current feedback.",
			"Formatted feedback (bold, lists ...) appears as plain text here; it keeps its formatting unless you change it.",
		}},
		{Heading: "Please note", Lines: []string{
			"Do not change student numbers or column headers; do not add or delete rows.",
			"The sheet is protected against accidental edits (no password): only yellow cells can be edited.",
			"The file belongs to this assignment; uploading it to another assignment is rejected.",
			"If someone changes the grades after your preview, upload the file again.",
		}},
	},
	scoreErrTitle: "Invalid grade", scoreErr: "Enter a grade between 0 and %s with at most two decimals.",
	scorePromptTitle: "Grade", scorePrompt: "Grade 0-%s. An empty cell keeps the current grade.",
	feedbackErrTitle: "Feedback too long", feedbackErr: "Feedback can be at most 5000 characters.",
	feedbackPromptT: "Feedback", feedbackProm: "Plain text; an empty cell keeps the current feedback.",
	subject: "Agora assignment grade template", language: "en-US",
	policy: map[string]string{LateAllow: "Late submissions allowed (marked late)", LatePenalty: "Late penalty: %s%% per day", LateDeny: "Late submissions not accepted"},
	due:    "Due date", noDue: "None",
}

// formatDot, puanı Excel formülü için noktalı yazar (45.5).
func formatDot(p grading.Points) string {
	return strings.Replace(formatPoints(p), ",", ".", 1)
}

// safeCell, hücrenin formül olarak çalışmasını önler (formül enjeksiyonu).
func safeCell(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

// plainText, zengin metnin düz yazısıdır: bloklar satır olur, etiketler atılır.
func plainText(h string) string {
	for _, tag := range []string{"</p>", "<br>", "<br/>", "<br />", "</li>", "</h2>", "</h3>", "</blockquote>", "</pre>"} {
		h = strings.ReplaceAll(h, tag, tag+"\n")
	}
	lines := strings.Split(html.UnescapeString(plain.Sanitize(h)), "\n")
	out := lines[:0]
	for _, l := range lines {
		if l = strings.Join(strings.Fields(l), " "); l != "" {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// textHTML, düz metni paragraflara çevirir (her dolu satır bir paragraf).
func textHTML(s string) string {
	var b strings.Builder
	for _, l := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			b.WriteString("<p>" + html.EscapeString(l) + "</p>")
		}
	}
	return b.String()
}

// sameText, iki metnin boşluklar dışında aynı olduğunu söyler.
func sameText(a, b string) bool {
	return strings.Join(strings.Fields(a), " ") == strings.Join(strings.Fields(b), " ")
}

// WriteGradeSheet, ödevin puan şablonunu yazar.
func WriteGradeSheet(w io.Writer, sec Section, activityID, title string, st AssignmentSettings, rows []RosterRow, english bool, now time.Time) error {
	tx := gsTurkish
	course := sec.CourseNameTR
	if english {
		tx, course = gsEnglish, sec.CourseNameEN
	}
	stamp := now.In(turkey).Format("02.01.2006 15:04")
	maxText := formatPoints(st.MaxScore)
	if english {
		maxText = formatDot(st.MaxScore)
	}
	cols := []sheet.Column{
		{Header: tx.columns[0], Width: 7, Kind: sheet.Number},
		{Header: tx.columns[1], Width: 14, Kind: sheet.Text},
		{Header: tx.columns[2], Width: 18, Kind: sheet.Text},
		{Header: tx.columns[3], Width: 18, Kind: sheet.Text},
		{Header: tx.columns[4], Width: 26, Kind: sheet.Text},
		{Header: tx.columns[5], Width: 17, Kind: sheet.Text},
		{Header: tx.columns[6], Width: 8, Kind: sheet.Number},
		{Header: tx.columns[7], Width: 12, Kind: sheet.Input, Note: fmt.Sprintf(tx.scoreNote, maxText), Validation: &sheet.Validation{
			Formula:    `AND(ISNUMBER({cell}),{cell}>=0,{cell}<=` + formatDot(st.MaxScore) + `,ROUND({cell}*100,0)={cell}*100)`,
			ErrorTitle: tx.scoreErrTitle, ErrorMessage: fmt.Sprintf(tx.scoreErr, maxText),
			PromptTitle: tx.scorePromptTitle, Prompt: fmt.Sprintf(tx.scorePrompt, maxText),
		}},
		{Header: tx.columns[8], Width: 60, Kind: sheet.Input, Note: tx.feedbackNote, Validation: &sheet.Validation{
			Formula:    `LEN({cell})<=` + strconv.Itoa(gsMaxFeedback),
			ErrorTitle: tx.feedbackErrTitle, ErrorMessage: tx.feedbackErr, PromptTitle: tx.feedbackPromptT, Prompt: tx.feedbackProm,
		}},
	}
	data := make([][]any, 0, len(rows))
	for n, r := range rows {
		status, submitted, fileCount := tx.none, "", any(nil)
		if r.Latest != nil {
			fileCount = r.Latest.FileCount
			switch {
			case r.Latest.Status == StatusDraft:
				status = tx.draft
			case r.Latest.Late && st.Penalty(r.Latest.SubmittedAt) > 0:
				pct := formatPoints(st.Penalty(r.Latest.SubmittedAt))
				if english {
					pct = formatDot(st.Penalty(r.Latest.SubmittedAt))
				}
				status = fmt.Sprintf(tx.latePenalty, pct)
			case r.Latest.Late:
				status = tx.late
			default:
				status = tx.submitted
			}
			if r.Latest.SubmittedAt != nil {
				submitted = r.Latest.SubmittedAt.In(turkey).Format("02.01.2006 15:04")
			}
		}
		var score any
		feedback := ""
		if r.Grade != nil {
			if r.Grade.Score != nil {
				score = r.Grade.Score.Float()
			}
			feedback = safeCell(plainText(r.Grade.Feedback))
		}
		data = append(data, []any{n + 1, r.StudentNo, safeCell(r.LastName), safeCell(r.FirstName), status, submitted, fileCount, score, feedback})
	}

	label := sec.CourseCode + "-" + sec.Code
	b := sheet.NewBuilder(sheet.Props{Title: fmt.Sprintf(tx.title, sec.CourseCode, sec.Code, title), Subject: tx.subject, Language: tx.language})
	b.AddTable(tx.sheet, sheet.Table{
		Title: fmt.Sprintf(tx.title, sec.CourseCode, sec.Code, title),
		Subtitle: []string{
			fmt.Sprintf(tx.subtitle, course, len(rows), maxText),
			fmt.Sprintf(tx.created, stamp),
		},
		Columns: cols, Rows: data, Legend: tx.legend, FreezeCols: 4,
	})
	due := tx.noDue
	if st.DueAt != nil {
		due = st.DueAt.In(turkey).Format("02.01.2006 15:04")
	}
	pct := formatPoints(st.PenaltyPerDay)
	if english {
		pct = formatDot(st.PenaltyPerDay)
	}
	policy := tx.policy[st.LatePolicy]
	if st.LatePolicy == LatePenalty {
		policy = fmt.Sprintf(policy, pct)
	}
	b.AddGuide(tx.guide, sheet.Guide{
		Title: tx.guideTitle, Intro: fmt.Sprintf(tx.guideIntro, title, stamp),
		Headers: tx.guideHeaders,
		Rows: [][]string{
			{tx.columns[7], fmt.Sprintf(tx.max, maxText)},
			{tx.due, due},
			{tx.columns[4], policy},
		},
		Sections: tx.sections,
	})
	b.SetMeta(tx.sheet, map[string]string{
		gsMetaKind: gsKind, gsMetaActivity: activityID, gsMetaLabel: label + " · " + title,
		gsMetaScore: sheet.ColumnName(7), gsMetaFeedback: sheet.ColumnName(8),
	})
	return b.Write(w)
}

// gradeLine, dosyanın bir öğrenci satırıdır: hücreler olduğu gibi.
type gradeLine struct {
	line      int
	studentNo string
	score     string
	feedback  string
}

// gradeFile, okunmuş puan dosyasıdır. Sütun başlıkları önizlemede gösterilir.
type gradeFile struct {
	scoreHeader, feedbackHeader string
	hasFeedback                 bool
	lines                       []gradeLine
}

// readGradeSheet, okunmuş dosyadan puanları çıkarır. Agora şablonunda sütunlar kimlik
// sayfasından, değilse başlıklardan bulunur; başlıksız iki sütunlu (numara, puan) dosya da olur.
func readGradeSheet(f sheet.File, activityID, label string) (gradeFile, error) {
	if f.Meta[gsMetaKind] == gsKind && f.Meta[gsMetaActivity] != activityID {
		return gradeFile{}, &sheet.Error{Detail: fmt.Sprintf(
			"Bu dosya “%s” ödevinin şablonu; “%s” ödevine yüklenemez. Bu ödevin şablonunu indirin.",
			cmp(f.Meta[gsMetaLabel], "başka bir"), label)}
	}
	rows := f.Rows
	if len(rows) == 0 {
		return gradeFile{}, &sheet.Error{Detail: "Dosya boş."}
	}
	var out gradeFile
	studentCol, scoreCol, feedbackCol := -1, -1, -1
	h := sheet.FindHeader(rows, gsStudentHeaders, gsHeaderScan)
	switch {
	case h >= 0:
		keys := make([]string, len(rows[h].Cells))
		for i, c := range rows[h].Cells {
			keys[i] = sheet.Key(c)
		}
		find := func(meta string, headers []string) int {
			if letter := f.Meta[meta]; letter != "" {
				return columnIndex(letter)
			}
			return slices.IndexFunc(keys, func(k string) bool { return slices.Contains(headers, k) })
		}
		studentCol = slices.IndexFunc(keys, func(k string) bool { return slices.Contains(gsStudentHeaders, k) })
		scoreCol, feedbackCol = find(gsMetaScore, gsScoreHeaders), find(gsMetaFeedback, gsFeedbackHeaders)
		if scoreCol < 0 && feedbackCol < 0 {
			return gradeFile{}, &sheet.Error{Detail: "“Puan” sütunu bulunamadı. Agora'nın şablonunu ya da “Öğrenci No” ve “Puan” sütunlu bir dosya kullanın."}
		}
		if scoreCol >= 0 {
			out.scoreHeader = rows[h].Cell(scoreCol)
		}
		if feedbackCol >= 0 {
			out.feedbackHeader, out.hasFeedback = rows[h].Cell(feedbackCol), true
		}
		rows = rows[h+1:]
	case strings.Trim(rows[0].Cell(0), "0123456789") == "" && rows[0].Cell(0) != "":
		studentCol, scoreCol = 0, 1
	default:
		return gradeFile{}, &sheet.Error{Detail: "Öğrenci numarası sütunu bulunamadı: başlık satırında “Öğrenci No” başlıklı bir sütun olmalı."}
	}
	cell := func(r sheet.Row, i int) string {
		if i < 0 {
			return ""
		}
		return r.Cell(i)
	}
	for _, r := range rows {
		l := gradeLine{line: r.Line, studentNo: cell(r, studentCol), score: cell(r, scoreCol), feedback: cell(r, feedbackCol)}
		// Numarası, puanı ve geri bildirimi boş satır (şablonun altındaki açıklamalar) öğrenci satırı değildir.
		if l.studentNo == "" && l.score == "" && l.feedback == "" {
			continue
		}
		out.lines = append(out.lines, l)
	}
	if len(out.lines) > gsMaxRows {
		return gradeFile{}, &sheet.Error{Detail: fmt.Sprintf("Dosyada en fazla %d öğrenci satırı olabilir.", gsMaxRows)}
	}
	return out, nil
}

func cmp(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// columnIndex, Excel sütun harfinin 0'dan başlayan sırasıdır (A → 0); geçersizse -1.
func columnIndex(letter string) int {
	n := 0
	for _, r := range strings.ToUpper(letter) {
		if r < 'A' || r > 'Z' {
			return -1
		}
		n = n*26 + int(r-'A'+1)
	}
	return n - 1
}

// parsePoints, puan hücresini okur (0..max, en fazla iki ondalık, virgül ya da nokta). Excel'in
// kayan nokta kalıntısı (45.49999999999999) iki ondalığa çok yakınsa kabul edilir.
func parsePoints(s string, max grading.Points) (grading.Points, bool) {
	s = strings.Replace(strings.TrimSpace(s), ",", ".", 1)
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || strings.ContainsAny(s, "eE+-") {
		return 0, false
	}
	c := math.Round(v * 100)
	if math.Abs(v*100-c) > 1e-6 || c < 0 || grading.Points(c) > max {
		return 0, false
	}
	return grading.Points(c), true
}

// Satır sonuçları ve hata kodları.
const (
	ImportChanged   = "CHANGED"
	ImportUnchanged = "UNCHANGED"
	ImportEmpty     = "EMPTY"
	ImportError     = "ERROR"

	ImportMissingStudentNo = "MISSING_STUDENT_NO"
	ImportNotInSection     = "NOT_IN_SECTION"
	ImportDuplicate        = "DUPLICATE_STUDENT"
	ImportInvalidScore     = "INVALID_SCORE"
	ImportFeedbackTooLong  = "FEEDBACK_TOO_LONG"
)

// GradeImportRow, dosyanın bir satırının önizlemesidir. Next, satırdan sonraki puandır (puan
// hücresi boşsa şimdiki); NextFeedback geri bildirim değişiyorsa yeni HTML'idir.
type GradeImportRow struct {
	Line            int
	StudentNo       string
	Student         *Student
	Status          string
	Current         *grading.Points
	Next            *grading.Points
	FeedbackChanged bool
	NextFeedback    string
	ErrorCode       string
	Error           string
}

// GradeImport, puan dosyasının önizlemesidir. Version, önizlemenin dayandığı puan sürümüdür
// (uygularken iyimser kilit).
type GradeImport struct {
	Version        int
	ScoreColumn    string
	FeedbackColumn string
	Rows           []GradeImportRow
	Missing        []Student
	Changed        int
	Unchanged      int
	Empty          int
	Errors         int
}

// GradeRowsError: dosyada hatalı satırlar var, hiçbir puan yazılmadı.
type GradeRowsError struct {
	Rows []GradeImportRow
}

func (e *GradeRowsError) Error() string {
	return fmt.Sprintf("lms: dosyada %d hatalı satır", len(e.Rows))
}

// gradesVersion, ödevin puanlarının sürümüdür: her puan yazımı bir satırın sürümünü artırır,
// toplam kesin artar.
func gradesVersion(ctx context.Context, q db.Querier, activityID string) (int, error) {
	var v int
	err := q.QueryRow(ctx, `SELECT coalesce(sum(version), 0) FROM lms.assignment_grades WHERE activity_id = $1`, activityID).Scan(&v)
	if err != nil {
		return 0, fmt.Errorf("lms: puan sürümü okunamadı: %w", err)
	}
	return v, nil
}

// PreviewGradeImport, dosyadaki puanların ödeve nasıl yazılacağını gösterir; hiçbir şey kaydedilmez.
func (s *Service) PreviewGradeImport(ctx context.Context, sec Section, activityID, title string, st AssignmentSettings, data []byte, mediaType string) (GradeImport, error) {
	f, err := sheet.Read(data, mediaType, gsMaxRows+gsHeaderScan+20)
	if err != nil {
		return GradeImport{}, err
	}
	file, err := readGradeSheet(f, activityID, sec.CourseCode+"-"+sec.Code+" · "+title)
	if err != nil {
		return GradeImport{}, err
	}
	res := GradeImport{ScoreColumn: file.scoreHeader, FeedbackColumn: file.feedbackHeader}
	if res.Version, err = gradesVersion(ctx, s.pool, activityID); err != nil {
		return GradeImport{}, err
	}
	roster, err := s.Roster(ctx, st, sec.ID, activityID)
	if err != nil {
		return GradeImport{}, err
	}
	byNo := make(map[string]int, len(roster))
	for i, r := range roster {
		byNo[r.StudentNo] = i
	}
	filled := map[string][]int{} // öğrenci no → dolu satırları (tekrar denetimi)
	for _, l := range file.lines {
		if strings.TrimSpace(l.score) != "" || strings.TrimSpace(l.feedback) != "" {
			filled[l.studentNo] = append(filled[l.studentNo], l.line)
		}
	}
	seen := make([]bool, len(roster))
	for _, l := range file.lines {
		row := GradeImportRow{Line: l.line, StudentNo: l.studentNo}
		i, inSection := byNo[l.studentNo]
		var current *Grade
		if inSection {
			student := roster[i].Student
			row.Student, seen[i], current = &student, true, roster[i].Grade
			if current != nil && current.Score != nil {
				sc := *current.Score
				row.Current = &sc
			}
		}
		scoreCell, feedbackCell := strings.TrimSpace(l.score), strings.TrimSpace(l.feedback)
		score, validScore := parsePoints(scoreCell, st.MaxScore)
		switch {
		case scoreCell == "" && feedbackCell == "":
			row.Status = ImportEmpty
		case l.studentNo == "":
			row.ErrorCode, row.Error = ImportMissingStudentNo, "Öğrenci numarası boş."
		case !inSection:
			row.ErrorCode, row.Error = ImportNotInSection, "Bu numarada şubenin kesin kayıtlı öğrencisi yok."
		case len(filled[l.studentNo]) > 1:
			row.ErrorCode, row.Error = ImportDuplicate, "Öğrenci dosyada birden fazla kez geçiyor (satır "+joinInts(filled[l.studentNo])+")."
		case scoreCell != "" && !validScore:
			row.ErrorCode, row.Error = ImportInvalidScore, fmt.Sprintf("Puan 0 ile %s arasında, en fazla iki ondalıklı olmalı.", formatPoints(st.MaxScore))
		case len([]rune(feedbackCell)) > gsMaxFeedback:
			row.ErrorCode, row.Error = ImportFeedbackTooLong, fmt.Sprintf("Geri bildirim en çok %d karakter olabilir.", gsMaxFeedback)
		default:
			row.Next = row.Current
			if scoreCell != "" {
				row.Next = &score
			}
			currentFeedback := ""
			if current != nil {
				currentFeedback = current.Feedback
			}
			if feedbackCell != "" && !sameText(feedbackCell, plainText(currentFeedback)) {
				row.FeedbackChanged, row.NextFeedback = true, textHTML(feedbackCell)
			}
			scoreChanged := scoreCell != "" && (row.Current == nil || *row.Current != score)
			row.Status = ImportUnchanged
			if scoreChanged || row.FeedbackChanged {
				row.Status = ImportChanged
			}
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
	for i, r := range roster {
		if !seen[i] {
			res.Missing = append(res.Missing, r.Student)
		}
	}
	return res, nil
}

// ImportGrades, dosyadaki puanları ve geri bildirimleri yazar: hatalı satır varsa hiçbir şey
// yazılmaz (*GradeRowsError); version önizlemenin puan sürümüdür. Değişen satırlar tek
// transaction'da, o anki son denemeye yazılır.
func (s *Service) ImportGrades(ctx context.Context, actorID string, sec Section, activityID, title string, version int, data []byte, mediaType string) (GradeImport, error) {
	st, err := s.Assignment(ctx, s.pool, activityID)
	if err != nil {
		return GradeImport{}, err
	}
	res, err := s.PreviewGradeImport(ctx, sec, activityID, title, st, data, mediaType)
	if err != nil {
		return GradeImport{}, err
	}
	if res.Errors > 0 {
		var bad []GradeImportRow
		for _, r := range res.Rows {
			if r.Status == ImportError {
				bad = append(bad, r)
			}
		}
		return GradeImport{}, &GradeRowsError{Rows: bad}
	}
	if version != res.Version {
		return GradeImport{}, ErrVersionMismatch
	}
	if res.Changed == 0 {
		return res, nil
	}
	source := "CSV"
	if mediaType == sheet.MediaXLSX {
		source = "XLSX"
	}
	err = db.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		// Tek tek puanlama (FOR SHARE) bu işlem bitene kadar bekler.
		if _, err := tx.Exec(ctx, `SELECT 1 FROM lms.assignments WHERE activity_id = $1 FOR UPDATE`, activityID); err != nil {
			return fmt.Errorf("lms: ödev kilitlenemedi: %w", err)
		}
		current, err := gradesVersion(ctx, tx, activityID)
		if err != nil {
			return err
		}
		if current != version {
			return ErrVersionMismatch
		}
		for _, r := range res.Rows {
			if r.Status != ImportChanged {
				continue
			}
			var score *int64
			if r.Next != nil {
				v := int64(*r.Next)
				score = &v
			}
			// Geri bildirim değişmiyorsa NULL: mevcut korunur (yeni satırda boş).
			var feedback *string
			if r.FeedbackChanged {
				feedback = &r.NextFeedback
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO lms.assignment_grades (activity_id, user_id, score, feedback_html, attempt_no, graded_by)
				VALUES ($1, $2, $3::numeric / 100, coalesce($4, ''),
				        (SELECT max(attempt_no) FROM lms.assignment_submissions WHERE activity_id = $1 AND user_id = $2), $5)
				ON CONFLICT (activity_id, user_id) DO UPDATE SET
					score = excluded.score, feedback_html = coalesce($4, lms.assignment_grades.feedback_html),
					attempt_no = excluded.attempt_no, graded_by = excluded.graded_by, graded_at = now(),
					version = lms.assignment_grades.version + 1`,
				activityID, r.Student.UserID, score, feedback, actorID); err != nil {
				return fmt.Errorf("lms: puan yazılamadı: %w", err)
			}
		}
		return audit.Record(ctx, tx, audit.Entry{ActorUserID: actorID, Action: "lms_grade.import", EntityType: "lms_activity",
			EntityID: activityID, After: map[string]any{"changed": res.Changed, "source": source}})
	})
	if err != nil {
		return GradeImport{}, err
	}
	s.metrics.Graded(strings.ToLower(source), res.Changed)
	if st.ReleasedAt != nil {
		s.syncActivity(ctx, actorID, activityID)
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
