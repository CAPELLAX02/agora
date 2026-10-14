package grading

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

// CSV dosyaları öğretim elemanının Excel'de açıp düzenlemesi içindir. Türkçe Excel ondalık
// ayracı virgül olduğu için alan ayracı olarak `;` bekler; dosya UTF-8 olarak tanınsın diye
// başına BOM yazılır. İçe aktarma hoşgörülüdür: ayraç ilk satırdan sezilir, ondalık virgül
// ya da nokta olabilir, UTF-8 olmayan dosya Excel'in Türkçe "CSV" kaydı (Windows-1254) sayılır.

// maxImportRows, içe aktarılan dosyanın en fazla veri satırıdır (not yazmanın sınırıyla aynı).
const maxImportRows = 1000

// CSVError, dosyanın bütününe ait bir sorundur (okunamadı, sütun bulunamadı ...). Detail
// kullanıcıya gösterilir.
type CSVError struct {
	Detail string
}

func (e *CSVError) Error() string { return "grading: CSV: " + e.Detail }

// csvLocale, dışa aktarılan dosyanın dilidir: başlıklar, ayraçlar ve "girmedi" işareti.
type csvLocale struct {
	comma      rune
	decimal    string
	absent     string
	headers    [5]string // öğrenci no, soyadı, adı, program, devam durumu
	letter     string
	continuing string
	attFail    string
}

var (
	csvTurkish = csvLocale{
		comma: ';', decimal: ",", absent: "G",
		headers: [5]string{"Öğrenci No", "Soyadı", "Adı", "Program", "Devam Durumu"}, letter: "Harf Notu",
		continuing: "Devam ediyor", attFail: "Devamsız",
	}
	csvEnglish = csvLocale{
		comma: ',', decimal: ".", absent: "ABSENT",
		headers: [5]string{"Student No", "Last Name", "First Name", "Program", "Attendance"}, letter: "Letter Grade",
		continuing: "Continuing", attFail: "Failed (attendance)",
	}
)

// componentLabels, bileşenlerin not tablosundaki adlarıdır: aynı türden birden fazla bileşen
// varsa türün adı numaralanır (Ödev 1, Ödev 2).
func componentLabels(cs []Component, english bool) []string {
	perType := map[string]int{}
	for _, c := range cs {
		perType[c.Type.Code]++
	}
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Label(english, perType[c.Type.Code] > 1)
	}
	return out
}

// formatPoints, puanı gereksiz sıfırlar olmadan yazar: 7500 → "75", 7550 → "75,5".
func formatPoints(p Points, decimal string) string {
	s := strconv.FormatInt(int64(p/100), 10)
	if frac := p % 100; frac != 0 {
		s += decimal + strings.TrimRight(fmt.Sprintf("%02d", frac), "0")
	}
	return s
}

// safeCell, Excel'in formül olarak çalıştıracağı metin hücrelerini etkisizleştirir (CSV
// enjeksiyonu): =, +, -, @ ya da sekmeyle başlayan hücrenin başına ' eklenir.
func safeCell(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

// WriteGradebookCSV, not tablosunu CSV olarak yazar: öğrenci bilgileri, her bileşen için bir
// sütun (başlığı bileşenin adı) ve ilan edilmişse harf notu.
func WriteGradebookCSV(w io.Writer, g Gradebook, english bool) error {
	loc := csvTurkish
	if english {
		loc = csvEnglish
	}
	components := make([]Component, len(g.Components))
	for i, c := range g.Components {
		components[i] = c.Component
	}
	if _, err := io.WriteString(w, "\ufeff"); err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	cw.Comma, cw.UseCRLF = loc.comma, true

	header := append(loc.headers[:], componentLabels(components, english)...)
	if err := cw.Write(append(header, loc.letter)); err != nil {
		return err
	}
	for _, st := range g.Students {
		attendance := loc.continuing
		if st.Attendance == AttendanceFail {
			attendance = loc.attFail
		}
		row := []string{st.StudentNo, safeCell(st.LastName), safeCell(st.FirstName), safeCell(st.ProgramCode), attendance}
		for _, c := range g.Components {
			cell := ""
			if sc, ok := st.Scores[c.ID]; ok {
				cell = loc.absent
				if !sc.Absent {
					cell = formatPoints(sc.Points, loc.decimal)
				}
			}
			row = append(row, cell)
		}
		if err := cw.Write(append(row, st.Letter)); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// --- İçe aktarma ---------------------------------------------------------------

// importLine, dosyanın bir veri satırıdır: öğrenci numarası ve puan hücresi olduğu gibi.
type importLine struct {
	line      int
	studentNo string
	cell      string
}

// importFile, okunmuş dosyadır. column, puanların okunduğu sütunun başlığıdır (başlıksız
// dosyada boş).
type importFile struct {
	column string
	lines  []importLine
}

// headerKey, başlığı karşılaştırma için sadeleştirir: küçük harf, Türkçe harfler ASCII
// karşılığına, harf ve rakam dışındakiler atılır. "Öğrenci No" → "ogrencino".
func headerKey(s string) string {
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

var (
	studentNoHeaders = []string{"ogrencino", "ogrencinumarasi", "ogrno", "numara", "no", "studentno", "studentnumber", "studentid"}
	scoreHeaders     = []string{"puan", "not", "score", "grade", "points"}
	absentMarks      = []string{"g", "gr", "girmedi", "absent", "abs"}
)

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

// readScoreCSV, bir bileşenin notlarını içeren dosyayı okur. labels, bileşenin adlarıdır
// (Türkçe ve İngilizce): bu başlıklı sütun puan sütunudur. Yoksa "Puan" başlıklı sütun, o da
// yoksa iki sütunlu dosyada öğrenci numarası dışındaki sütun. İlk hücresi sadece rakam olan
// dosya başlıksız sayılır (öğrenci no, puan).
func readScoreCSV(data []byte, labels []string) (importFile, error) {
	if !utf8.Valid(data) {
		decoded, err := charmap.Windows1254.NewDecoder().Bytes(data)
		if err != nil {
			return importFile{}, &CSVError{Detail: "Dosyanın karakter kodlaması tanınmadı; UTF-8 olarak kaydedin."}
		}
		data = decoded
	}
	data = bytes.TrimPrefix(data, []byte("\ufeff"))

	r := csv.NewReader(bytes.NewReader(data))
	r.Comma, r.FieldsPerRecord, r.TrimLeadingSpace = sniffComma(data), -1, true
	// Tamamen boş satırlar atlanır; satır numarası dosyadakidir (tırnak içinde satır sonu
	// olabileceği için alanın konumundan okunur).
	type record struct {
		line   int
		fields []string
	}
	var rows []record
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		var pe *csv.ParseError
		if errors.As(err, &pe) {
			return importFile{}, &CSVError{Detail: fmt.Sprintf("%d. satır okunamadı: tırnak işaretleri hatalı.", pe.Line)}
		}
		if err != nil {
			return importFile{}, &CSVError{Detail: "Dosya okunamadı."}
		}
		line, _ := r.FieldPos(0)
		blank := true
		for j := range rec {
			rec[j] = strings.TrimSpace(rec[j])
			blank = blank && rec[j] == ""
		}
		if !blank {
			rows = append(rows, record{line: line, fields: rec})
		}
		if len(rows) > maxImportRows+1 {
			return importFile{}, &CSVError{Detail: fmt.Sprintf("Dosyada en fazla %d öğrenci satırı olabilir.", maxImportRows)}
		}
	}
	if len(rows) == 0 {
		return importFile{}, &CSVError{Detail: "Dosya boş."}
	}

	studentCol, scoreCol, out := -1, -1, importFile{}
	header := rows[0].fields
	if isDigits(header[0]) {
		filled := 0
		for _, f := range header {
			if f != "" {
				filled++
			}
		}
		if filled > 2 {
			return importFile{}, &CSVError{Detail: "Başlıksız dosya iki sütunlu olmalı: öğrenci numarası ve puan."}
		}
		studentCol, scoreCol = 0, 1
	} else {
		keys := make([]string, len(header))
		for i, h := range header {
			keys[i] = headerKey(h)
		}
		studentCol = slices.IndexFunc(keys, func(k string) bool { return slices.Contains(studentNoHeaders, k) })
		if studentCol < 0 {
			return importFile{}, &CSVError{Detail: "Öğrenci numarası sütunu bulunamadı: ilk satırda “Öğrenci No” başlıklı bir sütun olmalı."}
		}
		for _, l := range labels {
			if i := slices.Index(keys, headerKey(l)); i >= 0 && i != studentCol {
				scoreCol = i
				break
			}
		}
		if scoreCol < 0 {
			scoreCol = slices.IndexFunc(keys, func(k string) bool { return slices.Contains(scoreHeaders, k) })
		}
		if scoreCol < 0 {
			var filled []int
			for i, k := range keys {
				if k != "" {
					filled = append(filled, i)
				}
			}
			if len(filled) == 2 {
				scoreCol = filled[0] + filled[1] - studentCol
			}
		}
		if scoreCol < 0 {
			return importFile{}, &CSVError{Detail: fmt.Sprintf(
				"“%s” sütunu bulunamadı. Dışa aktarılan dosyayı ya da “Öğrenci No” ve “Puan” sütunlu bir dosya kullanın.", labels[0])}
		}
		out.column = header[scoreCol]
		rows = rows[1:]
	}
	if len(rows) > maxImportRows {
		return importFile{}, &CSVError{Detail: fmt.Sprintf("Dosyada en fazla %d öğrenci satırı olabilir.", maxImportRows)}
	}
	cell := func(fields []string, i int) string {
		if i < len(fields) {
			return fields[i]
		}
		return ""
	}
	for _, row := range rows {
		out.lines = append(out.lines, importLine{line: row.line, studentNo: cell(row.fields, studentCol), cell: cell(row.fields, scoreCol)})
	}
	return out, nil
}

func isDigits(s string) bool {
	return s != "" && strings.Trim(s, "0123456789") == ""
}

// parseScoreCell, puan hücresini okur. Boş hücrede ok true ve empty true döner; geçersiz
// değerde ok false. Puan 0-100 arası, en fazla iki ondalık; ondalık ayracı virgül ya da nokta.
func parseScoreCell(s string) (score Score, empty, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Score{}, true, true
	}
	if slices.Contains(absentMarks, headerKey(s)) {
		return Score{Absent: true}, false, true
	}
	whole, frac, hasFrac := strings.Cut(strings.Replace(s, ",", ".", 1), ".")
	if !isDigits(whole) || len(whole) > 3 || (hasFrac && (!isDigits(frac) || len(frac) > 2)) {
		return Score{}, false, false
	}
	w, _ := strconv.Atoi(whole)
	f := 0
	if hasFrac {
		f, _ = strconv.Atoi(frac)
		if len(frac) == 1 {
			f *= 10
		}
	}
	p := Points(w*100 + f)
	if p > 10000 {
		return Score{}, false, false
	}
	return Score{Points: p}, false, true
}
