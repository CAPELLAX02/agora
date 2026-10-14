package grading

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/CAPELLAX02/agora/backend/internal/platform/sheet"
)

func demoGradebook() Gradebook {
	end := time.Date(2026, 11, 4, 15, 0, 0, 0, time.UTC)
	next := time.Date(2026, 12, 28, 5, 0, 0, 0, time.UTC)
	published := time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)
	return Gradebook{
		SectionContext: SectionContext{
			SectionID: "s1", SectionCode: "1", CourseCode: "COM1007", CourseNameTR: "Bilgisayar Programlama I",
			CourseNameEN: "Computer Programming I", TermCode: "2026-FALL",
		},
		Components: []GradebookComponent{
			{Component: Component{ID: "mid", Type: AssessmentType{Code: "MIDTERM", NameTR: "Ara sınav", NameEN: "Midterm", Category: CategoryInTerm}, SequenceNo: 1, Weight: 25},
				PublishedAt: &published, Window: EntryWindow{Open: true, EndsAt: &end}},
			{Component: Component{ID: "lab", Type: AssessmentType{Code: "LAB", NameTR: "Laboratuvar", NameEN: "Laboratory", Category: CategoryInTerm}, SequenceNo: 1, Weight: 15},
				Window: EntryWindow{Open: true, EndsAt: &end}},
			{Component: Component{ID: "fin", Type: AssessmentType{Code: "FINAL", NameTR: "Final sınavı", NameEN: "Final exam", Category: CategoryFinal}, SequenceNo: 1, Weight: 60},
				Window: EntryWindow{NextStartsAt: &next}},
			{Component: Component{ID: "mak", Type: AssessmentType{Code: "MAKEUP", NameTR: "Bütünleme", NameEN: "Make-up exam", Category: CategoryMakeup}, SequenceNo: 1, Weight: 60}},
		},
		Students: []GradebookStudent{
			{StudentNo: "22290102", FirstName: "Ece", LastName: "Arslan", ProgramCode: "BIL-L", Attendance: AttendanceContinuing,
				Scores: map[string]Score{"mid": {Points: 7550}, "lab": {Absent: true}}},
			{StudentNo: "22290103", FirstName: "Can", LastName: "Çelik", ProgramCode: "BIL-L", Attendance: AttendanceFail,
				Scores: map[string]Score{"mid": {Points: 4000}}},
		},
	}
}

func TestWriteGradebookXLSX(t *testing.T) {
	var buf bytes.Buffer
	now := time.Date(2026, 10, 14, 11, 30, 0, 0, time.UTC)
	if err := WriteGradebookXLSX(&buf, demoGradebook(), false, map[string]bool{"mid": true, "lab": true}, now); err != nil {
		t.Fatal(err)
	}
	x, err := excelize.OpenReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(x.GetSheetList(), ","); got != "Not Listesi,Açıklamalar,_agora" {
		t.Errorf("sayfalar: %s", got)
	}
	title, _ := x.GetCellValue("Not Listesi", "A1")
	created, _ := x.GetCellValue("Not Listesi", "A4")
	if title != "COM1007-1 · Bilgisayar Programlama I" || created != "Oluşturulma: 14.10.2026 14:30 · Agora" {
		t.Errorf("başlık bloğu: %q, %q", title, created)
	}
	// Başlık 7. satırda (başlık bloğu, boş satır, notlar); sütunlar: 6 bilgi, 4 bileşen, harf.
	header, _ := x.GetRows("Not Listesi")
	if got := strings.Join(header[6], "|"); got != "Sıra|Öğrenci No|Soyadı|Adı|Program|Devam Durumu|Ara sınav|Laboratuvar|Final sınavı|Bütünleme|Harf Notu" {
		t.Errorf("başlık satırı: %s", got)
	}
	if got := strings.Join(header[5][6:], "|"); got != "%25 · Düzenlenebilir|%15 · Düzenlenebilir|%60 · Kapalı|%60 · Kapalı|İlan edilen" {
		t.Errorf("not satırı: %s", got)
	}
	if v, _ := x.GetCellValue("Not Listesi", "H8"); v != "G" {
		t.Errorf("girmedi hücresi: %q", v)
	}
	if v, _ := x.GetCellValue("Not Listesi", "F9"); v != "Devamsız" {
		t.Errorf("devam: %q", v)
	}
	dvs, _ := x.GetDataValidations("Not Listesi")
	var ranges []string
	for _, dv := range dvs {
		ranges = append(ranges, dv.Sqref)
	}
	if strings.Join(ranges, ",") != "G8:G9,H8:H9" {
		t.Errorf("doğrulama sadece düzenlenebilir sütunlarda: %v", ranges)
	}
	guide, _ := x.GetRows("Açıklamalar")
	if len(guide) < 8 || guide[0][0] != "Not listesi nasıl doldurulur?" || strings.Join(guide[5], "|") != "Laboratuvar|%15|Açık · bitiş 04.11.2026 18:00|Düzenlenebilir" ||
		strings.Join(guide[6], "|") != "Final sınavı|%60|Kapalı · açılış 28.12.2026 08:00|Salt okunur" {
		t.Errorf("açıklama sayfası: %q", guide)
	}

	// Geri okuma: bileşenin sütunu kimlik sayfasından bulunur (başlığı değiştirilse de).
	_ = x.SetCellValue("Not Listesi", "H7", "Lab (benim notlarım)")
	var edited bytes.Buffer
	if _, err := x.WriteTo(&edited); err != nil {
		t.Fatal(err)
	}
	f, err := sheet.Read(edited.Bytes(), sheet.MediaXLSX, 100)
	if err != nil {
		t.Fatal(err)
	}
	target := scoreTarget{sectionID: "s1", sectionLabel: "COM1007-1", componentID: "lab", labels: []string{"Laboratuvar", "Laboratory"}}
	file, err := readScoreFile(f, target)
	if err != nil || file.column != "Lab (benim notlarım)" || len(file.lines) != 2 || file.lines[0].cell != "G" || file.lines[0].line != 8 {
		t.Errorf("geri okuma: %+v %v", file, err)
	}
	target.componentID = "mid"
	if file, err := readScoreFile(f, target); err != nil || file.lines[0].cell != "75.5" || file.lines[1].cell != "40" {
		t.Errorf("ara sınav sütunu: %+v %v", file, err)
	}
	// Başka şubenin şablonu reddedilir.
	target.sectionID, target.sectionLabel = "s2", "COM1015-1"
	var se *sheet.Error
	if _, err := readScoreFile(f, target); !errors.As(err, &se) || !strings.Contains(se.Detail, "COM1007-1 şubesinin şablonu") {
		t.Errorf("başka şube: %v", err)
	}

	// İngilizce şablon.
	buf.Reset()
	if err := WriteGradebookXLSX(&buf, demoGradebook(), true, nil, now); err != nil {
		t.Fatal(err)
	}
	x, err = excelize.OpenReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(x.GetSheetList(), ","); got != "Grade List,Instructions,_agora" {
		t.Errorf("İngilizce sayfalar: %s", got)
	}
	if v, _ := x.GetCellValue("Grade List", "H8"); v != "ABSENT" {
		t.Errorf("İngilizce girmedi: %q", v)
	}
	if dvs, _ := x.GetDataValidations("Grade List"); len(dvs) != 0 {
		t.Errorf("düzenlenebilir bileşen yokken doğrulama olmamalı: %d", len(dvs))
	}
}
