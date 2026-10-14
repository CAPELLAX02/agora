package grading

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/CAPELLAX02/agora/backend/internal/platform/sheet"
)

// Not listesinin Excel şablonu: öğretim elemanı notları sarı hücrelere girer, dosyayı
// Agora'da bileşenin içe aktarma adımında yükler. Şablon şubeye özeldir (gizli kimlik
// sayfası), düzenlenemeyen sütunlar gri ve kilitlidir, puan hücreleri Excel'de de doğrulanır.

// turkey, Türkiye saatidir (2016'dan beri yıl boyu UTC+3); tarihler şablonda bu saatle yazılır.
var turkey = time.FixedZone("TRT", 3*60*60)

// xlsxText, şablonun bir dildeki metinleridir.
type xlsxText struct {
	sheet, guide                                string
	title                                       string
	termLine, weights, makeupNote, created      string
	columns                                     [6]string // sıra, öğrenci no, soyadı, adı, program, devam
	letter, letterNote                          string
	editable, closed, published                 string
	continuing, attFail, absent                 string
	legend                                      []string
	guideTitle, guideIntro                      string
	guideHeaders                                []string
	windowOpen, windowNext, windowNone          string
	statusEditable, statusReadonly              string
	sections                                    []sheet.GuideSection
	validationTitle, validationMsg, promptTitle string
	prompt, docSubject                          string
	language                                    string
}

var xlsxTurkish = xlsxText{
	sheet: "Not Listesi", guide: "Açıklamalar",
	title:    "%s-%s · %s",
	termLine: "%s dönemi not listesi · %d öğrenci", weights: "Değerlendirme: %s",
	makeupNote: "Bütünleme (finalin yerine)", created: "Oluşturulma: %s · Agora",
	columns: [6]string{"Sıra", "Öğrenci No", "Soyadı", "Adı", "Program", "Devam Durumu"},
	letter:  "Harf Notu", letterNote: "İlan edilen",
	editable: "%%%s · Düzenlenebilir", closed: "%%%s · Kapalı", published: "%%%s · İlan edildi",
	continuing: "Devam ediyor", attFail: "Devamsız", absent: "G",
	legend: []string{
		"G: sınava girmedi. Boş hücre notu değiştirmez, girilmiş bir notu silmez.",
		"Sarı sütunlar şu an düzenlenebilir; gri sütunların not girişi penceresi kapalı ya da notları ilan edildi.",
		"Dosyayı Agora'da not tablosunda bileşenin “Excel'den içe aktar” adımında yükleyin: her yüklemede bir bileşenin sütunu okunur.",
	},
	guideTitle:     "Not listesi nasıl doldurulur?",
	guideIntro:     "Bu şablon Agora'dan %s şubesi için %s tarihinde üretildi. Notları “Not Listesi” sayfasındaki sarı hücrelere girin, dosyayı kaydedip Agora'da ilgili bileşen için yükleyin. Önce satır satır önizleme gösterilir; onaylayınca notlar tek seferde yazılır.",
	guideHeaders:   []string{"Bileşen", "Ağırlık", "Not girişi penceresi", "Bu şablonda"},
	windowOpen:     "Açık · bitiş %s",
	windowNext:     "Kapalı · açılış %s",
	windowNone:     "Kapalı",
	statusEditable: "Düzenlenebilir", statusReadonly: "Salt okunur",
	sections: []sheet.GuideSection{
		{Heading: "Puanlar", Lines: []string{
			"0 ile 100 arasında, en fazla iki ondalıklı bir sayı girin (ör. 75 ya da 75,5).",
			"Sınava girmeyen öğrenci için G yazın.",
			"Boş bırakılan hücre notu değiştirmez; girilmiş bir notu silmez. Not silmek için Agora'daki not tablosunu kullanın.",
			"Bütünleme notu sadece bütünleme hakkı olan öğrencilere girilir.",
		}},
		{Heading: "Dikkat edilecekler", Lines: []string{
			"Öğrenci numaralarını ve sütun başlıklarını değiştirmeyin; satır eklemeyin, silmeyin.",
			"Sayfa kazara değişikliklere karşı korumalıdır (parolasız): yalnızca sarı hücreler düzenlenebilir.",
			"Dosya bu şubeye özeldir; başka bir şubenin bileşenine yüklenirse reddedilir.",
			"Dosyayı .xlsx olarak kaydedin. CSV olarak da yüklenebilir, ancak şube denetimi yalnızca .xlsx'te yapılır.",
		}},
		{Heading: "Yükleme", Lines: []string{
			"Agora → Verdiğim dersler → Not tablosu → bileşen → Excel'den içe aktar.",
			"Önizlemede hatalı satırlar ayrıca listelenir; hatalı satır varken hiçbir not yazılmaz.",
			"Önizlemeden sonra bu bileşenin notları başka biri tarafından değiştirilirse dosyayı yeniden yükleyin.",
		}},
	},
	validationTitle: "Geçersiz puan",
	validationMsg:   "0 ile 100 arasında, en fazla iki ondalıklı bir puan ya da sınava girmeyen için G yazın.",
	promptTitle:     "Puan",
	prompt:          "0-100 arası puan ya da G (girmedi). Boş bırakılan hücre notu değiştirmez.",
	docSubject:      "Agora not listesi şablonu",
	language:        "tr-TR",
}

var xlsxEnglish = xlsxText{
	sheet: "Grade List", guide: "Instructions",
	title:    "%s-%s · %s",
	termLine: "%s grade list · %d students", weights: "Assessment: %s",
	makeupNote: "Make-up (replaces the final)", created: "Generated: %s · Agora",
	columns: [6]string{"No.", "Student No", "Last Name", "First Name", "Program", "Attendance"},
	letter:  "Letter Grade", letterNote: "Published",
	editable: "%%%s · Editable", closed: "%%%s · Closed", published: "%%%s · Published",
	continuing: "Continuing", attFail: "Failed (attendance)", absent: "ABSENT",
	legend: []string{
		"ABSENT: did not sit the exam. An empty cell keeps the current score; it never deletes a score.",
		"Yellow columns are editable now; grey columns are closed for entry or already published.",
		"Upload this file in Agora with “Import from Excel” on the component: each upload reads one component's column.",
	},
	guideTitle:     "How to fill in the grade list",
	guideIntro:     "Agora generated this template for section %s on %s. Enter scores in the yellow cells of the “Grade List” sheet, save the file and upload it in Agora for the component. A row-by-row preview comes first; on confirmation all scores are written at once.",
	guideHeaders:   []string{"Component", "Weight", "Grade entry window", "In this template"},
	windowOpen:     "Open · until %s",
	windowNext:     "Closed · opens %s",
	windowNone:     "Closed",
	statusEditable: "Editable", statusReadonly: "Read-only",
	sections: []sheet.GuideSection{
		{Heading: "Scores", Lines: []string{
			"Enter a number between 0 and 100 with at most two decimals (e.g. 75 or 75.5).",
			"Write ABSENT (or G) for a student who did not sit the exam.",
			"An empty cell keeps the current score; it never deletes one. Use the gradebook in Agora to delete a score.",
			"Make-up scores are entered only for students eligible for the make-up exam.",
		}},
		{Heading: "Please note", Lines: []string{
			"Do not change student numbers or column headers; do not add or delete rows.",
			"The sheet is protected against accidental edits (no password): only yellow cells can be edited.",
			"The file belongs to this section; uploading it to another section's component is rejected.",
			"Save as .xlsx. CSV uploads also work, but the section check is only possible with .xlsx.",
		}},
		{Heading: "Uploading", Lines: []string{
			"Agora → My courses → Gradebook → component → Import from Excel.",
			"The preview lists invalid rows; no score is written while any row is invalid.",
			"If someone changes this component's scores after your preview, upload the file again.",
		}},
	},
	validationTitle: "Invalid score",
	validationMsg:   "Enter a score between 0 and 100 with at most two decimals, or ABSENT for a student who did not sit the exam.",
	promptTitle:     "Score",
	prompt:          "Score 0-100 or ABSENT. An empty cell keeps the current score.",
	docSubject:      "Agora grade list template",
	language:        "en-US",
}

// scoreValidation, puan hücrelerinin Excel doğrulamasıdır: 0-100 arası en fazla iki ondalıklı
// sayı ya da girmedi işareti.
const scoreValidation = `OR(AND(ISNUMBER({cell}),{cell}>=0,{cell}<=100,ROUND({cell}*100,0)={cell}*100),` +
	`UPPER({cell})="G",UPPER({cell})="GR",UPPER({cell})="ABSENT")`

func formatWeight(w float64) string {
	return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.2f", w), "0"), ".")
}

// WriteGradebookXLSX, not tablosunun Excel şablonunu yazar. editable, isteği yapanın şu an not
// girebildiği bileşenlerdir (sarı ve kilitsiz); diğerleri gri ve kilitlidir.
func WriteGradebookXLSX(w io.Writer, g Gradebook, english bool, editable map[string]bool, now time.Time) error {
	tx := xlsxTurkish
	if english {
		tx = xlsxEnglish
	}
	components := make([]Component, len(g.Components))
	for i, c := range g.Components {
		components[i] = c.Component
	}
	labels := componentLabels(components, english)
	courseName := g.CourseNameTR
	if english {
		courseName = g.CourseNameEN
	}
	section := g.CourseCode + "-" + g.SectionCode
	stamp := now.In(turkey).Format("02.01.2006 15:04")

	var weights []string
	for i, c := range g.Components {
		if c.Type.Category == CategoryMakeup {
			weights = append(weights, tx.makeupNote)
			continue
		}
		weights = append(weights, fmt.Sprintf("%s %%%s", labels[i], formatWeight(c.Weight)))
	}

	cols := []sheet.Column{
		{Header: tx.columns[0], Width: 7, Kind: sheet.Number},
		{Header: tx.columns[1], Width: 14, Kind: sheet.Text},
		{Header: tx.columns[2], Width: 18, Kind: sheet.Text},
		{Header: tx.columns[3], Width: 18, Kind: sheet.Text},
		{Header: tx.columns[4], Width: 13, Kind: sheet.Text},
		{Header: tx.columns[5], Width: 16, Kind: sheet.Text},
	}
	meta := map[string]string{metaKind: metaKindBook, metaSection: g.SectionID, metaLabel: section, "term": g.TermCode}
	for i, c := range g.Components {
		col := sheet.Column{Header: labels[i], Width: 15, Kind: sheet.Readonly}
		weight := formatWeight(c.Weight)
		switch {
		case editable[c.ID]:
			col.Kind, col.Note = sheet.Input, fmt.Sprintf(tx.editable, weight)
			col.Validation = &sheet.Validation{
				Formula: scoreValidation, ErrorTitle: tx.validationTitle, ErrorMessage: tx.validationMsg,
				PromptTitle: tx.promptTitle, Prompt: tx.prompt,
			}
		case c.PublishedAt != nil:
			col.Note = fmt.Sprintf(tx.published, weight)
		default:
			col.Note = fmt.Sprintf(tx.closed, weight)
		}
		col.Highlights = []sheet.Highlight{{Equals: tx.absent, Fill: "FEF3C7", Font: "92400E"}}
		if tx.absent != "G" {
			col.Highlights = append(col.Highlights, sheet.Highlight{Equals: "G", Fill: "FEF3C7", Font: "92400E"})
		}
		meta[metaComponent+c.ID] = sheet.ColumnName(len(cols))
		cols = append(cols, col)
	}
	cols = append(cols, sheet.Column{Header: tx.letter, Note: tx.letterNote, Width: 12, Kind: sheet.Number})

	rows := make([][]any, 0, len(g.Students))
	for n, st := range g.Students {
		attendance := tx.continuing
		if st.Attendance == AttendanceFail {
			attendance = tx.attFail
		}
		row := []any{n + 1, st.StudentNo, safeCell(st.LastName), safeCell(st.FirstName), safeCell(st.ProgramCode), attendance}
		for _, c := range g.Components {
			var v any
			if sc, ok := st.Scores[c.ID]; ok {
				v = tx.absent
				if !sc.Absent {
					v = sc.Points.Float()
				}
			}
			row = append(row, v)
		}
		rows = append(rows, append(row, st.Letter))
	}

	b := sheet.NewBuilder(sheet.Props{
		Title: fmt.Sprintf(tx.title, g.CourseCode, g.SectionCode, courseName), Subject: tx.docSubject, Language: tx.language,
	})
	b.AddTable(tx.sheet, sheet.Table{
		Title: fmt.Sprintf(tx.title, g.CourseCode, g.SectionCode, courseName),
		Subtitle: []string{
			fmt.Sprintf(tx.termLine, g.TermCode, len(g.Students)),
			fmt.Sprintf(tx.weights, strings.Join(weights, " · ")),
			fmt.Sprintf(tx.created, stamp),
		},
		Columns: cols, Rows: rows, Legend: tx.legend, FreezeCols: 4,
	})

	guideRows := make([][]string, 0, len(g.Components))
	for i, c := range g.Components {
		window := tx.windowNone
		switch {
		case c.Window.Open && c.Window.EndsAt != nil:
			window = fmt.Sprintf(tx.windowOpen, c.Window.EndsAt.In(turkey).Format("02.01.2006 15:04"))
		case c.Window.NextStartsAt != nil:
			window = fmt.Sprintf(tx.windowNext, c.Window.NextStartsAt.In(turkey).Format("02.01.2006 15:04"))
		}
		status := tx.statusReadonly
		if editable[c.ID] {
			status = tx.statusEditable
		}
		weight := "%" + formatWeight(c.Weight)
		if c.Type.Category == CategoryMakeup {
			weight = tx.makeupNote
		}
		guideRows = append(guideRows, []string{labels[i], weight, window, status})
	}
	b.AddGuide(tx.guide, sheet.Guide{
		Title: tx.guideTitle, Intro: fmt.Sprintf(tx.guideIntro, section, stamp),
		Headers: tx.guideHeaders, Rows: guideRows, Sections: tx.sections,
	})
	b.SetMeta(tx.sheet, meta)
	return b.Write(w)
}
