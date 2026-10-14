package sheet

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
	"golang.org/x/text/encoding/charmap"
)

func TestKey(t *testing.T) {
	for in, want := range map[string]string{
		"Öğrenci No":      "ogrencino",
		"ÖĞRENCİ NO":      "ogrencino",
		"STUDENT NO":      "studentno",
		"Ödev 1":          "odev1",
		"Final sınavı":    "finalsinavi",
		"Ara Sınav (%40)": "arasinav40",
	} {
		if got := Key(in); got != want {
			t.Errorf("Key(%q) = %q, want %q", in, got, want)
		}
	}
}

func lines(rows []Row) []int {
	var out []int
	for _, r := range rows {
		out = append(out, r.Line)
	}
	return out
}

func TestReadCSV(t *testing.T) {
	tests := []struct {
		name  string
		data  string
		lines []int
		first []string
		err   string
	}{
		{"BOM, noktalı virgül, CRLF", "\ufeffNo;Puan\r\n1;75,5\r\n", []int{1, 2}, []string{"No", "Puan"}, ""},
		{"virgül ayraç", "No,Puan\n1,75.5\n", []int{1, 2}, []string{"No", "Puan"}, ""},
		{"sekme ayraç", "No\tPuan\n1\t70\n", []int{1, 2}, []string{"No", "Puan"}, ""},
		{"boş satırlar atlanır, satır numarası dosyadaki", "\nNo;Puan\n\n;;\n1;70\n", []int{2, 5}, []string{"No", "Puan"}, ""},
		{"tırnak içinde satır sonu", "Ad;Not\n\"Ece\nArslan\";70\n2;80\n", []int{1, 2, 4}, []string{"Ad", "Not"}, ""},
		{"bozuk tırnak", "No;Puan\n1;\"7\"0\n", nil, nil, "2. satır okunamadı"},
		{"çok satır", strings.Repeat("1;70\n", 4), nil, nil, "en fazla 3 satır"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := Read([]byte(tt.data), MediaCSV, 3)
			if tt.err != "" {
				var se *Error
				if !errors.As(err, &se) || !strings.Contains(se.Detail, tt.err) {
					t.Fatalf("hata = %v, %q içermeli", err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := lines(f.Rows); !equal(got, tt.lines) {
				t.Errorf("satırlar %v, want %v", got, tt.lines)
			}
			if got := f.Rows[0].Cells; strings.Join(got, "|") != strings.Join(tt.first, "|") {
				t.Errorf("ilk satır %q", got)
			}
		})
	}
}

func equal(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Türkçe Excel "CSV (virgülle ayrılmış)" kaydını Windows-1254 kodlamasıyla yazar.
func TestReadCSVWindows1254(t *testing.T) {
	data, err := charmap.Windows1254.NewEncoder().Bytes([]byte("Öğrenci No;Ara sınav\n22290102;70\n"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := Read(data, MediaCSV, 10)
	if err != nil || f.Rows[0].Cell(1) != "Ara sınav" {
		t.Fatalf("Windows-1254: %+v %v", f, err)
	}
}

func TestReadXLSX(t *testing.T) {
	// Kullanıcının kendi dosyası: arada boş satır, sayılar biçimli; satır numaraları Excel'deki.
	x := excelize.NewFile()
	_ = x.SetCellValue("Sheet1", "A1", "Öğrenci No")
	_ = x.SetCellValue("Sheet1", "B1", "Puan")
	_ = x.SetCellValue("Sheet1", "A2", "22290102")
	_ = x.SetCellValue("Sheet1", "B2", 75.5)
	_ = x.SetCellValue("Sheet1", "A4", 22290103)
	_ = x.SetCellValue("Sheet1", "B4", "G")
	style, _ := x.NewStyle(&excelize.Style{NumFmt: 2}) // 0.00
	_ = x.SetCellStyle("Sheet1", "B2", "B2", style)
	var buf bytes.Buffer
	if _, err := x.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	f, err := Read(buf.Bytes(), MediaXLSX, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got := lines(f.Rows); !equal(got, []int{1, 2, 4}) {
		t.Errorf("satırlar %v", got)
	}
	if f.Rows[1].Cell(1) != "75.5" || f.Rows[2].Cell(0) != "22290103" || len(f.Meta) != 0 {
		t.Errorf("değerler: %+v, meta %v", f.Rows, f.Meta)
	}

	if _, err := Read([]byte("PK bozuk"), MediaXLSX, 10); err == nil || !strings.Contains(err.Error(), ".xlsx") {
		t.Errorf("bozuk dosya: %v", err)
	}
}

func TestBuilderRoundTrip(t *testing.T) {
	b := NewBuilder(Props{Title: "Deneme", Language: "tr-TR"})
	layout := b.AddTable("Liste", Table{
		Title: "COM1007-1", Subtitle: []string{"2026-FALL"},
		Columns: []Column{
			{Header: "Öğrenci No", Kind: Text},
			{Header: "Ara sınav", Note: "%25", Kind: Input, Validation: &Validation{Formula: "AND(ISNUMBER({cell}),{cell}<=100)", ErrorMessage: "0-100"}},
			{Header: "Final", Kind: Readonly},
		},
		Rows:   [][]any{{"22290102", 70.5, nil}, {"22290103", "G", 40}},
		Legend: []string{"G: girmedi"},
	})
	b.AddGuide("Açıklamalar", Guide{Title: "Nasıl doldurulur?", Headers: []string{"Bileşen", "Ağırlık"}, Rows: [][]string{{"Ara sınav", "%25"}},
		Sections: []GuideSection{{Heading: "Puanlar", Lines: []string{"0-100"}}}})
	b.SetMeta("Liste", map[string]string{"section_id": "s1"})
	var buf bytes.Buffer
	if err := b.Write(&buf); err != nil {
		t.Fatal(err)
	}
	if layout.HeaderRow != 5 || layout.FirstRow != 6 || layout.LastRow != 7 {
		t.Errorf("yerleşim: %+v", layout)
	}

	x, err := excelize.OpenReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if got := x.GetSheetList(); strings.Join(got, ",") != "Liste,Açıklamalar,_agora" {
		t.Errorf("sayfalar: %v", got)
	}
	if visible, _ := x.GetSheetVisible("_agora"); visible {
		t.Error("kimlik sayfası gizli olmalı")
	}
	dvs, _ := x.GetDataValidations("Liste")
	if len(dvs) != 1 || dvs[0].Sqref != "B6:B7" || !strings.Contains(dvs[0].Formula1, "B6&lt;=100") && !strings.Contains(dvs[0].Formula1, "B6<=100") {
		t.Errorf("doğrulama: %+v", dvs)
	}
	locked := func(c string) bool {
		id, _ := x.GetCellStyle("Liste", c)
		s, _ := x.GetStyle(id)
		return s.Protection == nil || s.Protection.Locked
	}
	if locked("B6") || !locked("A6") || !locked("C6") {
		t.Errorf("kilitler: giriş açık, diğerleri kilitli olmalı")
	}

	f, err := Read(buf.Bytes(), MediaXLSX, 50)
	if err != nil {
		t.Fatal(err)
	}
	if f.Meta["section_id"] != "s1" || f.Meta[metaDataSheet] != "Liste" {
		t.Errorf("meta: %v", f.Meta)
	}
	h := FindHeader(f.Rows, []string{"ogrencino"}, 10)
	if h < 0 || f.Rows[h].Line != 5 || f.Rows[h+1].Cell(1) != "70.5" || f.Rows[h+2].Cell(1) != "G" {
		t.Errorf("başlık %d, satırlar %+v", h, f.Rows)
	}
}
