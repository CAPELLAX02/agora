package grading

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Not tablosunun CSV dışa aktarması. CSV öğretim elemanının Excel'de açıp düzenlemesi içindir:
// Türkçe Excel ondalık ayracı virgül olduğu için alan ayracı olarak `;` bekler; dosya UTF-8
// olarak tanınsın diye başına BOM yazılır. Biçimli şablon için bkz. xlsx.go; içe aktarma her
// iki biçimi de okur (scorefile.go).

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
