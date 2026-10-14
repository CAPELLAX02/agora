package grading

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/CAPELLAX02/agora/backend/internal/platform/sheet"
)

// readCSVScores, CSV metnini okuyup bileşen sütununu çıkarır (kimlik sayfası olmayan dosya).
func readCSVScores(data string, labels []string) (importFile, error) {
	f, err := sheet.Read([]byte(data), sheet.MediaCSV, maxImportRows+headerScan+20)
	if err != nil {
		return importFile{}, err
	}
	return readScoreFile(f, scoreTarget{sectionID: "s1", sectionLabel: "COM1007-1", componentID: "c1", labels: labels})
}

func TestParseScoreCell(t *testing.T) {
	tests := []struct {
		in    string
		want  Score
		empty bool
		ok    bool
	}{
		{"", Score{}, true, true},
		{"  ", Score{}, true, true},
		{"75", Score{Points: 7500}, false, true},
		{"75,5", Score{Points: 7550}, false, true},
		{"75.25", Score{Points: 7525}, false, true},
		{"0", Score{}, false, true},
		{"100", Score{Points: 10000}, false, true},
		{"100,00", Score{Points: 10000}, false, true},
		{"G", Score{Absent: true}, false, true},
		{"gr", Score{Absent: true}, false, true},
		{"GİRMEDİ", Score{Absent: true}, false, true},
		{"Girmedi", Score{Absent: true}, false, true},
		{"ABSENT", Score{Absent: true}, false, true},
		{"100,5", Score{}, false, false},
		{"75,125", Score{}, false, false},
		{"-5", Score{}, false, false},
		{"1000", Score{}, false, false},
		{"75,", Score{}, false, false},
		{",5", Score{}, false, false},
		{"7 5", Score{}, false, false},
		{"=1+1", Score{}, false, false},
		{"75.49999999999999", Score{Points: 7550}, false, true}, // Excel sayı hücresi
		{"75.125", Score{}, false, false},
		{"1e2", Score{}, false, false},
	}
	for _, tt := range tests {
		got, empty, ok := parseScoreCell(tt.in)
		if got != tt.want || empty != tt.empty || ok != tt.ok {
			t.Errorf("parseScoreCell(%q) = %+v %v %v, want %+v %v %v", tt.in, got, empty, ok, tt.want, tt.empty, tt.ok)
		}
	}
}

func TestReadScoreFile(t *testing.T) {
	labels := []string{"Ara sınav", "Midterm"}
	type line struct {
		no, cell string
		at       int
	}
	tests := []struct {
		name   string
		data   string
		column string
		want   []line
		err    string // CSVError'da geçmeli
	}{
		{
			name:   "dışa aktarılan dosya (BOM, ;, CRLF): bileşenin sütunu",
			data:   "\ufeffÖğrenci No;Soyadı;Adı;Program;Devam Durumu;Ara sınav;Final sınavı;Harf Notu\r\n22290102;Arslan;Ece;BIL-L;Devam ediyor;75,5;;\r\n22290103;Çelik;Can;BIL-L;Devam ediyor;G;;\r\n",
			column: "Ara sınav",
			want:   []line{{"22290102", "75,5", 2}, {"22290103", "G", 3}},
		},
		{
			name:   "İngilizce dosya (virgül ayraç, İngilizce ad)",
			data:   "Student No,Last Name,First Name,Midterm,Final exam\n22290102,Arslan,Ece,75.5,\n",
			column: "Midterm",
			want:   []line{{"22290102", "75.5", 2}},
		},
		{
			name:   "Puan başlığı, boş satırlar atlanır, satır numaraları dosyadaki",
			data:   "numara;ad;puan\n\n22290102;Ece;70\n;;\n22290103;Can;80\n",
			column: "puan",
			want:   []line{{"22290102", "70", 3}, {"22290103", "80", 5}},
		},
		{
			name:   "iki sütun: öğrenci no dışındaki puandır",
			data:   "Öğrenci No\tSonuç\n22290102\t64\n",
			column: "Sonuç",
			want:   []line{{"22290102", "64", 2}},
		},
		{
			name: "başlıksız iki sütun",
			data: "22290102;70\n22290103;80\n",
			want: []line{{"22290102", "70", 1}, {"22290103", "80", 2}},
		},
		{
			name:   "eksik hücre boş sayılır",
			data:   "Öğrenci No;Ad;Ara sınav\n22290102;Ece\n",
			column: "Ara sınav",
			want:   []line{{"22290102", "", 2}},
		},
		{
			name:   "başlık bloğu ve altta açıklama (şablon gibi)",
			data:   "COM1007-1 · Bilgisayar Programlama I;;\n2026-FALL;;\n\nSıra;Öğrenci No;Ara sınav\n1;22290102;70\n\nG: sınava girmedi;;\n",
			column: "Ara sınav",
			want:   []line{{"22290102", "70", 5}},
		},
		{name: "boş dosya", data: "\ufeff\n\n", err: "boş"},
		{name: "öğrenci no sütunu yok", data: "Ad;Ara sınav\nEce;70\n", err: "Öğrenci numarası sütunu"},
		{name: "puan sütunu yok", data: "Öğrenci No;Ad;Final sınavı\n22290102;Ece;70\n", err: "“Ara sınav” sütunu bulunamadı"},
		{name: "başlıksız çok sütun", data: "22290102;Ece;70\n", err: "iki sütunlu"},
		{name: "bozuk tırnak", data: "Öğrenci No;Ara sınav\n22290102;\"7\"0\n", err: "2. satır okunamadı"},
		{name: "çok satır", data: "Öğrenci No;Puan\n" + strings.Repeat("22290102;70\n", maxImportRows+1), err: "en fazla"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := readCSVScores(tt.data, labels)
			if tt.err != "" {
				var se *sheet.Error
				if !errors.As(err, &se) || !strings.Contains(se.Detail, tt.err) {
					t.Fatalf("hata = %v, %q içermeli", err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if f.column != tt.column {
				t.Errorf("sütun = %q, want %q", f.column, tt.column)
			}
			var got []line
			for _, l := range f.lines {
				got = append(got, line{l.studentNo, l.cell, l.line})
			}
			if len(got) != len(tt.want) {
				t.Fatalf("satırlar = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("satır %d = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestWriteGradebookCSV(t *testing.T) {
	midterm := GradebookComponent{Component: Component{ID: "m", Type: AssessmentType{Code: "MIDTERM", NameTR: "Ara sınav", NameEN: "Midterm"}, SequenceNo: 1}}
	hw1 := GradebookComponent{Component: Component{ID: "h1", Type: AssessmentType{Code: "HOMEWORK", NameTR: "Ödev", NameEN: "Homework"}, SequenceNo: 1}}
	hw2 := GradebookComponent{Component: Component{ID: "h2", Type: AssessmentType{Code: "HOMEWORK", NameTR: "Ödev", NameEN: "Homework"}, SequenceNo: 2}}
	g := Gradebook{
		Components: []GradebookComponent{midterm, hw1, hw2},
		Students: []GradebookStudent{
			{StudentNo: "22290102", FirstName: "Ece", LastName: "Arslan", ProgramCode: "BIL-L", Attendance: AttendanceContinuing,
				Scores: map[string]Score{"m": {Points: 7550}, "h1": {Absent: true}}, Letter: "CB"},
			{StudentNo: "22290103", FirstName: "=HYPERLINK(1)", LastName: "Çelik", ProgramCode: "BIL-L", Attendance: AttendanceFail,
				Scores: map[string]Score{"m": {Points: 6000}}},
		},
	}
	var buf bytes.Buffer
	if err := WriteGradebookCSV(&buf, g, false); err != nil {
		t.Fatal(err)
	}
	want := "\ufeffÖğrenci No;Soyadı;Adı;Program;Devam Durumu;Ara sınav;Ödev 1;Ödev 2;Harf Notu\r\n" +
		"22290102;Arslan;Ece;BIL-L;Devam ediyor;75,5;G;;CB\r\n" +
		"22290103;Çelik;'=HYPERLINK(1);BIL-L;Devamsız;60;;;\r\n"
	if buf.String() != want {
		t.Errorf("Türkçe CSV:\n%q\nwant\n%q", buf.String(), want)
	}

	buf.Reset()
	if err := WriteGradebookCSV(&buf, g, true); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(buf.String(), "\ufeffStudent No,Last Name,First Name,Program,Attendance,Midterm,Homework 1,Homework 2,Letter Grade\r\n22290102,Arslan,Ece,BIL-L,Continuing,75.5,ABSENT,,CB\r\n") {
		t.Errorf("İngilizce CSV:\n%q", buf.String())
	}

	// Dışa aktarılan dosya her bileşen için şablondur.
	f, err := readCSVScores(buf.String(), []string{"Ödev 1", "Homework 1"})
	if err != nil || f.column != "Homework 1" || f.lines[0].cell != "ABSENT" {
		t.Errorf("dışa aktarılan dosya geri okunamadı: %+v %v", f, err)
	}
}
