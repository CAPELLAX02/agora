// Package sheet, toplu içe aktarmaların ortak dosya katmanıdır: Excel (XLSX) şablonlarını
// üretir, CSV ve XLSX dosyalarını satırlara çevirir. Hangi sütunun ne olduğu (başlık eşleme,
// doğrulama) kullanan paketin işidir; burası dosya biçimlerini ve hoşgörülü okumayı bilir.
package sheet

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
	"golang.org/x/text/encoding/charmap"
)

// Dosya türleri (istek gövdesinin Content-Type'ı).
const (
	MediaCSV  = "text/csv"
	MediaXLSX = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
)

// Error, dosyanın bütününe ait bir sorundur (okunamadı, boş, çok satırlı ...). Detail
// kullanıcıya gösterilir.
type Error struct {
	Detail string
}

func (e *Error) Error() string { return "sheet: " + e.Detail }

// Row, dosyanın dolu bir satırıdır. Line dosyadaki satır numarasıdır (Excel'de satır
// başlığı, CSV'de satır; ilk satır 1). Hücreler baştaki ve sondaki boşluklardan arındırılmıştır.
type Row struct {
	Line  int
	Cells []string
}

// Cell, satırın i. hücresidir; yoksa boş.
func (r Row) Cell(i int) string {
	if i >= 0 && i < len(r.Cells) {
		return r.Cells[i]
	}
	return ""
}

// File, okunmuş bir dosyadır. Meta, Agora'nın ürettiği şablonlardaki gizli kimlik sayfasının
// anahtar-değer çiftleridir (bkz. Builder.SetMeta); kullanıcının kendi dosyasında boştur.
type File struct {
	Rows []Row
	Meta map[string]string
}

// Read, gövdeyi türüne göre okur. maxRows, dolu satırların (başlık dahil) üst sınırıdır.
func Read(data []byte, mediaType string, maxRows int) (File, error) {
	if mediaType == MediaXLSX {
		return readXLSX(data, maxRows)
	}
	rows, err := readCSV(data, maxRows)
	return File{Rows: rows}, err
}

func tooMany(maxRows int) error {
	return &Error{Detail: fmt.Sprintf("Dosyada en fazla %d satır olabilir.", maxRows)}
}

// appendRow, tamamen boş olmayan satırı ekler; hücreleri kırpar.
func appendRow(rows []Row, line int, cells []string) []Row {
	blank := true
	for i := range cells {
		cells[i] = strings.TrimSpace(cells[i])
		blank = blank && cells[i] == ""
	}
	if blank {
		return rows
	}
	return append(rows, Row{Line: line, Cells: cells})
}

// readCSV, CSV dosyasını okur. Excel'in kayıtlarına hoşgörülüdür: UTF-8 BOM atılır, UTF-8
// olmayan dosya Türkçe Excel'in "CSV" kaydı (Windows-1254) sayılır, ayraç ilk dolu satırdan
// sezilir (`;`, sekme ya da `,`).
func readCSV(data []byte, maxRows int) ([]Row, error) {
	if !utf8.Valid(data) {
		decoded, err := charmap.Windows1254.NewDecoder().Bytes(data)
		if err != nil {
			return nil, &Error{Detail: "Dosyanın karakter kodlaması tanınmadı; UTF-8 olarak kaydedin."}
		}
		data = decoded
	}
	data = bytes.TrimPrefix(data, []byte("\ufeff"))

	r := csv.NewReader(bytes.NewReader(data))
	r.Comma, r.FieldsPerRecord, r.TrimLeadingSpace = sniffComma(data), -1, true
	var rows []Row
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		var pe *csv.ParseError
		if errors.As(err, &pe) {
			return nil, &Error{Detail: fmt.Sprintf("%d. satır okunamadı: tırnak işaretleri hatalı.", pe.Line)}
		}
		if err != nil {
			return nil, &Error{Detail: "Dosya okunamadı."}
		}
		// Satır numarası alanın konumundan: tırnak içinde satır sonu olabilir.
		line, _ := r.FieldPos(0)
		if rows = appendRow(rows, line, rec); len(rows) > maxRows {
			return nil, tooMany(maxRows)
		}
	}
	return rows, nil
}

// sniffComma, alan ayracını ilk dolu satırdan seçer: `;` varsa o, yoksa sekme, yoksa virgül.
func sniffComma(data []byte) rune {
	first := data
	for len(first) > 0 {
		line, rest, _ := bytes.Cut(first, []byte("\n"))
		if len(bytes.TrimSpace(line)) > 0 {
			first = line
			break
		}
		first = rest
	}
	switch {
	case bytes.ContainsRune(first, ';'):
		return ';'
	case bytes.ContainsRune(first, '\t'):
		return '\t'
	}
	return ','
}

// XLSX bir zip arşividir: 1 MiB'lık gövde açılınca çok büyüyebilir (zip bombası). Açılmış
// boyut sınırlanır.
const (
	unzipLimit    = 64 << 20
	unzipXMLLimit = 32 << 20
)

// readXLSX, çalışma kitabının veri sayfasını okur: Agora şablonunda gizli kimlik sayfasının
// gösterdiği sayfa, değilse ilk görünür sayfa. Hücreler biçimlenmemiş değerleriyle okunur
// (75,5 yazılmış sayı "75.5" gelir).
func readXLSX(data []byte, maxRows int) (File, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{
		RawCellValue: true, UnzipSizeLimit: unzipLimit, UnzipXMLSizeLimit: unzipXMLLimit,
	})
	if err != nil {
		return File{}, &Error{Detail: "Excel dosyası açılamadı: .xlsx biçiminde kaydedin (eski .xls desteklenmez)."}
	}
	defer f.Close()

	out := File{Meta: readMeta(f)}
	sheetName := out.Meta[metaDataSheet]
	if sheetName == "" || !slices.Contains(f.GetSheetList(), sheetName) {
		sheetName = ""
		for _, name := range f.GetSheetList() {
			if visible, _ := f.GetSheetVisible(name); visible {
				sheetName = name
				break
			}
		}
	}
	if sheetName == "" {
		return File{}, &Error{Detail: "Excel dosyasında görünür bir sayfa yok."}
	}
	rows, err := f.Rows(sheetName)
	if err != nil {
		return File{}, &Error{Detail: "Excel sayfası okunamadı."}
	}
	defer rows.Close()
	for line := 1; rows.Next(); line++ {
		cells, err := rows.Columns(excelize.Options{RawCellValue: true})
		if err != nil {
			return File{}, &Error{Detail: fmt.Sprintf("%d. satır okunamadı.", line)}
		}
		if out.Rows = appendRow(out.Rows, line, cells); len(out.Rows) > maxRows {
			return File{}, tooMany(maxRows)
		}
	}
	if err := rows.Error(); err != nil {
		return File{}, &Error{Detail: "Excel sayfası okunamadı."}
	}
	return out, nil
}

// Key, başlığı karşılaştırma için sadeleştirir: küçük harf, Türkçe harfler ASCII karşılığına,
// harf ve rakam dışındakiler atılır. "Öğrenci No" → "ogrencino", "STUDENT NO" → "studentno".
func Key(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLowerSpecial(unicode.TurkishCase, s) {
		switch r {
		case 'ı', 'î':
			r = 'i'
		case 'ö':
			r = 'o'
		case 'ü', 'û':
			r = 'u'
		case 'ş':
			r = 's'
		case 'ç':
			r = 'c'
		case 'ğ':
			r = 'g'
		case 'â':
			r = 'a'
		}
		if r < utf8.RuneSelf && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// FindHeader, başlık satırını arar: ilk maxScan dolu satırdan, sadeleştirilmiş hücrelerinden
// biri keys'ten olan ilk satır. Şablonların başlık satırından önce başlık bloğu (ders, dönem
// ...) olabilir. Bulunamazsa -1.
func FindHeader(rows []Row, keys []string, maxScan int) int {
	for i, r := range rows {
		if i >= maxScan {
			break
		}
		for _, c := range r.Cells {
			if slices.Contains(keys, Key(c)) {
				return i
			}
		}
	}
	return -1
}
