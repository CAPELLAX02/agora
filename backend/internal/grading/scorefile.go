package grading

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/CAPELLAX02/agora/backend/internal/platform/sheet"
)

// Bir bileşenin notlarını içeren dosyanın (CSV ya da XLSX) okunması. Dosya hoşgörülü okunur:
// başlık satırı ilk satırlarda aranır (Agora şablonunun üstünde başlık bloğu vardır), puan
// sütunu bileşenin adından bulunur, başlıksız iki sütunlu dosya da kabul edilir. Agora'nın
// ürettiği Excel şablonunda gizli kimlik sayfası vardır: dosyanın başka bir şubeye ait olması
// reddedilir, bileşenin sütunu adından değil kimliğinden bulunur.

// maxImportRows, içe aktarılan dosyanın en fazla öğrenci satırıdır (not yazmanın sınırıyla aynı).
const maxImportRows = 1000

// headerScan, başlık satırının arandığı ilk dolu satır sayısıdır.
const headerScan = 20

// Şablonun kimlik sayfasındaki anahtarlar.
const (
	metaKind      = "kind"
	metaKindBook  = "gradebook"
	metaSection   = "section_id"
	metaLabel     = "section"
	metaComponent = "component:" // + bileşen kimliği → sütun harfi
)

var (
	studentNoHeaders = []string{"ogrencino", "ogrencinumarasi", "ogrno", "numara", "no", "studentno", "studentnumber", "studentid"}
	scoreHeaders     = []string{"puan", "not", "score", "grade", "points"}
	absentMarks      = []string{"g", "gr", "girmedi", "absent", "abs"}
)

// importLine, dosyanın bir öğrenci satırıdır: öğrenci numarası ve puan hücresi olduğu gibi.
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

// scoreTarget, dosyanın yazılacağı bileşendir.
type scoreTarget struct {
	sectionID    string
	sectionLabel string // COM1007-1
	componentID  string
	labels       []string // bileşenin Türkçe ve İngilizce adı
}

// readScoreFile, okunmuş dosyadan bileşenin notlarını çıkarır.
func readScoreFile(f sheet.File, t scoreTarget) (importFile, error) {
	if f.Meta[metaKind] == metaKindBook && f.Meta[metaSection] != t.sectionID {
		return importFile{}, &sheet.Error{Detail: fmt.Sprintf(
			"Bu dosya %s şubesinin şablonu; %s şubesinin bileşenine yüklenemez. Bu şubenin şablonunu indirin.",
			cmpOr(f.Meta[metaLabel], "başka bir"), t.sectionLabel)}
	}
	rows := f.Rows
	if len(rows) == 0 {
		return importFile{}, &sheet.Error{Detail: "Dosya boş."}
	}

	out := importFile{}
	studentCol, scoreCol := -1, -1
	h := sheet.FindHeader(rows, studentNoHeaders, headerScan)
	switch {
	case h >= 0:
		header := rows[h].Cells
		keys := make([]string, len(header))
		for i, c := range header {
			keys[i] = sheet.Key(c)
		}
		studentCol = slices.IndexFunc(keys, func(k string) bool { return slices.Contains(studentNoHeaders, k) })
		if letter := f.Meta[metaComponent+t.componentID]; letter != "" {
			scoreCol = columnIndex(letter)
		}
		for _, l := range t.labels {
			if scoreCol >= 0 {
				break
			}
			if i := slices.Index(keys, sheet.Key(l)); i >= 0 && i != studentCol {
				scoreCol = i
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
		if scoreCol < 0 || scoreCol == studentCol {
			return importFile{}, &sheet.Error{Detail: fmt.Sprintf(
				"“%s” sütunu bulunamadı. Agora'nın şablonunu ya da “Öğrenci No” ve “Puan” sütunlu bir dosya kullanın.", t.labels[0])}
		}
		out.column = rows[h].Cell(scoreCol)
		rows = rows[h+1:]
	case isDigits(rows[0].Cell(0)):
		filled := 0
		for _, c := range rows[0].Cells {
			if c != "" {
				filled++
			}
		}
		if filled > 2 {
			return importFile{}, &sheet.Error{Detail: "Başlıksız dosya iki sütunlu olmalı: öğrenci numarası ve puan."}
		}
		studentCol, scoreCol = 0, 1
	default:
		return importFile{}, &sheet.Error{Detail: "Öğrenci numarası sütunu bulunamadı: başlık satırında “Öğrenci No” başlıklı bir sütun olmalı."}
	}

	for _, r := range rows {
		l := importLine{line: r.Line, studentNo: r.Cell(studentCol), cell: r.Cell(scoreCol)}
		// Öğrenci numarası ve puanı boş satır (şablonun altındaki açıklamalar) öğrenci satırı değildir.
		if l.studentNo == "" && l.cell == "" {
			continue
		}
		out.lines = append(out.lines, l)
	}
	if len(out.lines) > maxImportRows {
		return importFile{}, &sheet.Error{Detail: fmt.Sprintf("Dosyada en fazla %d öğrenci satırı olabilir.", maxImportRows)}
	}
	return out, nil
}

func cmpOr(s, fallback string) string {
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

func isDigits(s string) bool {
	return s != "" && strings.Trim(s, "0123456789") == ""
}

// parseScoreCell, puan hücresini okur. Boş hücrede ok true ve empty true döner; geçersiz
// değerde ok false. Puan 0-100 arası, en fazla iki ondalık; ondalık ayracı virgül ya da nokta.
// Excel'in sayı hücresi kayan nokta kalıntısı taşıyabilir (75.49999999999999): iki ondalığa
// çok yakın değer kabul edilir.
func parseScoreCell(s string) (score Score, empty, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Score{}, true, true
	}
	if slices.Contains(absentMarks, sheet.Key(s)) {
		return Score{Absent: true}, false, true
	}
	s = strings.Replace(s, ",", ".", 1)
	whole, frac, hasFrac := strings.Cut(s, ".")
	if isDigits(whole) && len(whole) <= 3 && (!hasFrac || (isDigits(frac) && len(frac) <= 2)) {
		w, _ := strconv.Atoi(whole)
		f := 0
		if hasFrac {
			f, _ = strconv.Atoi(frac)
			if len(frac) == 1 {
				f *= 10
			}
		}
		if p := Points(w*100 + f); p <= 10000 {
			return Score{Points: p}, false, true
		}
		return Score{}, false, false
	}
	if hasFrac && isDigits(whole) && isDigits(frac) && len(frac) > 2 {
		v, _ := strconv.ParseFloat(s, 64)
		c := math.Round(v * 100)
		if math.Abs(v*100-c) < 1e-6 && c >= 0 && c <= 10000 {
			return Score{Points: Points(c)}, false, true
		}
	}
	return Score{}, false, false
}
