package sheet

import (
	"fmt"
	"html"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

// Şablonların görünümü: web arayüzünün marka rengi (indigo) başlıkta, düzenlenecek hücreler
// açık sarı, salt okunur hücreler gri. Bütün Agora şablonları aynı görünür.
const (
	colorBrand    = "4F46E5"
	colorBrandInk = "1E1B4B"
	colorMuted    = "64748B"
	colorBorder   = "CBD5E1"
	colorZebra    = "F8FAFC"
	colorInput    = "FFFBEB"
	colorInputAlt = "FEF3C7"
	colorReadonly = "F1F5F9"
	colorNoteFill = "EEF2FF"
	fontFamily    = "Calibri"
)

// metaSheet, Agora'nın ürettiği şablonlardaki çok gizli (Excel arayüzünden açılamayan) kimlik
// sayfasıdır: içe aktarırken dosyanın hangi kayda ait olduğu ve sütunların anlamı buradan okunur.
const (
	metaSheet     = "_agora"
	metaDataSheet = "data_sheet"
)

// ColumnKind, bir sütunun şablondaki rolüdür.
type ColumnKind int

// Sütun türleri.
const (
	Text     ColumnKind = iota // salt okunur bilgi (öğrenci no, ad ...)
	Number                     // salt okunur sayı
	Input                      // kullanıcının dolduracağı hücreler: kilitsiz, doğrulamalı
	Readonly                   // bu şablonda doldurulamayan giriş sütunu (gri, kilitli)
)

// Validation, giriş sütununun Excel doğrulamasıdır. Formula, özel (custom) doğrulama
// formülüdür; {cell} sütunun ilk veri hücresiyle değiştirilir (Excel göreli uygular).
type Validation struct {
	Formula      string
	ErrorTitle   string
	ErrorMessage string
	PromptTitle  string
	Prompt       string
}

// Highlight, değeri Equals olan hücrelerin koşullu biçimidir (ör. "G" sarı).
type Highlight struct {
	Equals string
	Fill   string
	Font   string
}

// Column, tablonun bir sütunudur. Note, başlığın üstündeki küçük açıklama satırıdır (ör.
// ağırlık "%25" ya da "Düzenlenebilir").
type Column struct {
	Header     string
	Note       string
	Width      float64
	Kind       ColumnKind
	Validation *Validation
	Highlights []Highlight
}

// Table, bir veri sayfasıdır: başlık bloğu, sütun başlıkları, satırlar ve altta açıklamalar.
type Table struct {
	Title      string
	Subtitle   []string
	Columns    []Column
	Rows       [][]any
	Legend     []string
	FreezeCols int // kaydırırken sabit kalan sol sütun sayısı
}

// TableLayout, yazılan tablonun yerleşimidir: başlık satırı ve veri satırları (Excel satır
// numaraları).
type TableLayout struct {
	HeaderRow int
	FirstRow  int
	LastRow   int
}

// GuideSection, açıklama sayfasının bir bölümüdür.
type GuideSection struct {
	Heading string
	Lines   []string
}

// Guide, şablonun nasıl doldurulacağını anlatan sayfadır; isteğe bağlı bir özet tablosuyla.
type Guide struct {
	Title    string
	Intro    string
	Sections []GuideSection
	Headers  []string
	Rows     [][]string
}

// Props, çalışma kitabının belge özellikleridir.
type Props struct {
	Title    string
	Subject  string
	Language string // tr-TR, en-US
}

// Builder, Agora görünümünde bir Excel çalışma kitabı kurar. İlk hata saklanır, sonraki
// çağrılar bir şey yapmaz; Write hatayı döndürür.
type Builder struct {
	f      *excelize.File
	styles map[string]int
	sheets int
	err    error
}

// NewBuilder, boş bir çalışma kitabı oluşturur.
func NewBuilder(p Props) *Builder {
	b := &Builder{f: excelize.NewFile(), styles: map[string]int{}}
	b.check(b.f.SetDocProps(&excelize.DocProperties{
		Title: p.Title, Subject: p.Subject, Creator: "Agora", LastModifiedBy: "Agora",
		Language: p.Language, Created: time.Now().UTC().Format(time.RFC3339),
	}))
	return b
}

func (b *Builder) check(err error) {
	if b.err == nil && err != nil {
		b.err = err
	}
}

// sheet, yeni bir sayfa açar; ilk sayfa varsayılan "Sheet1"in yerini alır.
func (b *Builder) sheet(name string) {
	if b.err != nil {
		return
	}
	if b.sheets == 0 {
		b.check(b.f.SetSheetName("Sheet1", name))
	} else {
		_, err := b.f.NewSheet(name)
		b.check(err)
	}
	b.sheets++
	b.check(b.f.SetSheetView(name, 0, &excelize.ViewOptions{ShowGridLines: ptr(false), ZoomScale: ptr(110.0)}))
}

func ptr[T any](v T) *T { return &v }

func border(color string) []excelize.Border {
	return []excelize.Border{
		{Type: "left", Color: color, Style: 1}, {Type: "right", Color: color, Style: 1},
		{Type: "top", Color: color, Style: 1}, {Type: "bottom", Color: color, Style: 1},
	}
}

func solid(color string) excelize.Fill {
	return excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{color}}
}

// style, adlandırılmış stilin kimliğidir (ilk kullanımda oluşturulur).
func (b *Builder) style(name string) int {
	if id, ok := b.styles[name]; ok {
		return id
	}
	font := func(size float64, bold bool, color string) *excelize.Font {
		return &excelize.Font{Family: fontFamily, Size: size, Bold: bold, Color: color}
	}
	locked := &excelize.Protection{Locked: true}
	var s excelize.Style
	switch name {
	case "title":
		s = excelize.Style{Font: font(16, true, colorBrandInk), Alignment: &excelize.Alignment{Vertical: "center"}}
	case "subtitle":
		s = excelize.Style{Font: font(10, false, colorMuted), Alignment: &excelize.Alignment{Vertical: "center"}}
	case "note":
		s = excelize.Style{Font: font(9, true, colorBrand), Fill: solid(colorNoteFill), Border: border(colorBorder),
			Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true}}
	case "header":
		s = excelize.Style{Font: font(11, true, "FFFFFF"), Fill: solid(colorBrand), Border: border(colorBrand),
			Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true}}
	case "text", "text-zebra":
		s = excelize.Style{Font: font(11, false, "0F172A"), Border: border(colorBorder), Protection: locked,
			Alignment: &excelize.Alignment{Vertical: "center"}}
	case "number", "number-zebra":
		s = excelize.Style{Font: font(11, false, "0F172A"), Border: border(colorBorder), Protection: locked,
			Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"}}
	case "input", "input-zebra":
		s = excelize.Style{Font: font(11, true, colorBrandInk), Border: border(colorBorder),
			Protection: &excelize.Protection{Locked: false},
			Alignment:  &excelize.Alignment{Horizontal: "center", Vertical: "center"}}
	case "readonly", "readonly-zebra":
		s = excelize.Style{Font: font(11, false, colorMuted), Border: border(colorBorder), Protection: locked,
			Fill: solid(colorReadonly), Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"}}
	case "legend":
		s = excelize.Style{Font: &excelize.Font{Family: fontFamily, Size: 9, Italic: true, Color: colorMuted}}
	case "guide-title":
		s = excelize.Style{Font: font(18, true, colorBrandInk)}
	case "guide-intro":
		s = excelize.Style{Font: font(11, false, colorMuted), Alignment: &excelize.Alignment{WrapText: true, Vertical: "top"}}
	case "guide-heading":
		s = excelize.Style{Font: font(12, true, colorBrand), Border: []excelize.Border{{Type: "bottom", Color: colorBrand, Style: 2}}}
	case "guide-line":
		s = excelize.Style{Font: font(11, false, "0F172A"), Alignment: &excelize.Alignment{WrapText: true, Vertical: "top", Indent: 1}}
	}
	if strings.HasSuffix(name, "-zebra") {
		switch {
		case strings.HasPrefix(name, "input"):
			s.Fill = solid(colorInputAlt)
		case strings.HasPrefix(name, "readonly"):
			s.Fill = solid("E2E8F0")
		default:
			s.Fill = solid(colorZebra)
		}
	} else if name == "input" {
		s.Fill = solid(colorInput)
	}
	id, err := b.f.NewStyle(&s)
	b.check(err)
	b.styles[name] = id
	return id
}

func cell(col, row int) string {
	name, _ := excelize.CoordinatesToCellName(col, row)
	return name
}

// colName, 1'den başlayan sütun numarasının harfidir (1 → A).
func colName(col int) string {
	name, _ := excelize.ColumnNumberToName(col)
	return name
}

// ColumnName, tablonun i. sütununun (0'dan) Excel harfidir.
func ColumnName(i int) string { return colName(i + 1) }

// AddTable, sayfaya bir veri tablosu yazar: başlık bloğu, sütun notları, marka renkli başlık
// satırı, satır satır zebra, giriş sütunlarında kilitsiz hücre ve doğrulama, koşullu biçim,
// sabit başlık ve sütunlar, filtre, yazdırma ayarları (yatay A4, sayfaya sığdır, başlık her
// sayfada). Giriş sütunu varsa sayfa korunur: sadece giriş hücreleri değiştirilebilir (parola
// yok; Excel'den "Sayfa Korumasını Kaldır" ile açılabilir, amaç kazara değişikliği önlemek).
func (b *Builder) AddTable(name string, t Table) TableLayout {
	b.sheet(name)
	if b.err != nil {
		return TableLayout{}
	}
	f, n := b.f, len(t.Columns)
	last := colName(n)
	row := 1
	b.check(f.SetCellValue(name, "A1", t.Title))
	b.check(f.MergeCell(name, "A1", last+"1"))
	b.check(f.SetCellStyle(name, "A1", last+"1", b.style("title")))
	b.check(f.SetRowHeight(name, 1, 28))
	for _, s := range t.Subtitle {
		row++
		b.check(f.SetCellValue(name, cell(1, row), s))
		b.check(f.MergeCell(name, cell(1, row), cell(n, row)))
		b.check(f.SetCellStyle(name, cell(1, row), cell(n, row), b.style("subtitle")))
	}
	row += 2

	hasNotes := slices.ContainsFunc(t.Columns, func(c Column) bool { return c.Note != "" })
	if hasNotes {
		for i, c := range t.Columns {
			if c.Note != "" {
				b.check(f.SetCellValue(name, cell(i+1, row), c.Note))
				b.check(f.SetCellStyle(name, cell(i+1, row), cell(i+1, row), b.style("note")))
			}
		}
		row++
	}
	layout := TableLayout{HeaderRow: row, FirstRow: row + 1, LastRow: row + len(t.Rows)}
	for i, c := range t.Columns {
		b.check(f.SetCellValue(name, cell(i+1, row), c.Header))
		width := c.Width
		if width == 0 {
			width = 14
		}
		b.check(f.SetColWidth(name, colName(i+1), colName(i+1), width))
	}
	b.check(f.SetCellStyle(name, cell(1, row), cell(n, row), b.style("header")))
	b.check(f.SetRowHeight(name, row, 32))

	for r, values := range t.Rows {
		y := layout.FirstRow + r
		for i, c := range t.Columns {
			if i < len(values) && values[i] != nil && values[i] != "" {
				b.check(f.SetCellValue(name, cell(i+1, y), values[i]))
			}
			style := map[ColumnKind]string{Text: "text", Number: "number", Input: "input", Readonly: "readonly"}[c.Kind]
			if r%2 == 1 {
				style += "-zebra"
			}
			b.check(f.SetCellStyle(name, cell(i+1, y), cell(i+1, y), b.style(style)))
		}
		b.check(f.SetRowHeight(name, y, 20))
	}

	hasInput := false
	if len(t.Rows) > 0 {
		for i, c := range t.Columns {
			first, rng := cell(i+1, layout.FirstRow), cell(i+1, layout.FirstRow)+":"+cell(i+1, layout.LastRow)
			if c.Kind == Input {
				hasInput = true
			}
			if c.Kind == Input && c.Validation != nil {
				v := c.Validation
				dv := excelize.NewDataValidation(true)
				dv.Sqref, dv.Type = rng, "custom"
				dv.Formula1 = html.EscapeString(strings.ReplaceAll(v.Formula, "{cell}", first))
				dv.SetError(excelize.DataValidationErrorStyleStop, v.ErrorTitle, v.ErrorMessage)
				if v.Prompt != "" {
					dv.SetInput(v.PromptTitle, v.Prompt)
				}
				b.check(f.AddDataValidation(name, dv))
			}
			for _, h := range c.Highlights {
				id, err := f.NewConditionalStyle(&excelize.Style{
					Fill: solid(h.Fill), Font: &excelize.Font{Color: h.Font, Bold: true},
				})
				b.check(err)
				b.check(f.SetConditionalFormat(name, rng, []excelize.ConditionalFormatOptions{{
					Type: "cell", Criteria: "==", Format: &id, Value: `"` + h.Equals + `"`,
				}}))
			}
		}
	}

	y := layout.LastRow + 2
	for _, l := range t.Legend {
		b.check(f.SetCellValue(name, cell(1, y), l))
		b.check(f.SetCellStyle(name, cell(1, y), cell(1, y), b.style("legend")))
		y++
	}

	b.check(f.SetPanes(name, &excelize.Panes{
		Freeze: true, XSplit: t.FreezeCols, YSplit: layout.HeaderRow,
		TopLeftCell: cell(t.FreezeCols+1, layout.HeaderRow+1), ActivePane: "bottomRight",
	}))
	if len(t.Rows) > 0 {
		b.check(f.AutoFilter(name, cell(1, layout.HeaderRow)+":"+cell(n, layout.LastRow), nil))
	}
	b.check(f.SetPageLayout(name, &excelize.PageLayoutOptions{
		Orientation: ptr("landscape"), Size: ptr(9), FitToWidth: ptr(1), FitToHeight: ptr(0),
	}))
	b.check(f.SetSheetProps(name, &excelize.SheetPropsOptions{FitToPage: ptr(true)}))
	b.check(f.SetHeaderFooter(name, &excelize.HeaderFooterOptions{
		OddFooter: "&L&8Agora&C&8&P / &N&R&8&D",
	}))
	b.check(f.SetDefinedName(&excelize.DefinedName{
		Name: "_xlnm.Print_Titles", Scope: name,
		RefersTo: fmt.Sprintf("'%s'!$%d:$%d", name, layout.HeaderRow, layout.HeaderRow),
	}))
	if hasInput {
		b.check(f.ProtectSheet(name, &excelize.SheetProtectionOptions{
			SelectLockedCells: true, SelectUnlockedCells: true, AutoFilter: true, FormatColumns: true, FormatRows: true,
		}))
	}
	return layout
}

// AddGuide, şablonun nasıl doldurulacağını anlatan bir sayfa ekler.
func (b *Builder) AddGuide(name string, g Guide) {
	b.sheet(name)
	if b.err != nil {
		return
	}
	f := b.f
	width := max(len(g.Headers), 1)
	last := colName(max(width, 4))
	b.check(f.SetColWidth(name, "A", "A", 34))
	b.check(f.SetColWidth(name, "B", colName(max(width, 4)), 22))
	b.check(f.SetCellValue(name, "A1", g.Title))
	b.check(f.MergeCell(name, "A1", last+"1"))
	b.check(f.SetCellStyle(name, "A1", "A1", b.style("guide-title")))
	b.check(f.SetRowHeight(name, 1, 30))
	row := 2
	if g.Intro != "" {
		b.check(f.SetCellValue(name, "A2", g.Intro))
		b.check(f.MergeCell(name, "A2", last+"2"))
		b.check(f.SetCellStyle(name, "A2", "A2", b.style("guide-intro")))
		b.check(f.SetRowHeight(name, 2, 36))
		row = 3
	}
	if len(g.Headers) > 0 {
		row++
		for i, h := range g.Headers {
			b.check(f.SetCellValue(name, cell(i+1, row), h))
		}
		b.check(f.SetCellStyle(name, cell(1, row), cell(len(g.Headers), row), b.style("header")))
		b.check(f.SetRowHeight(name, row, 24))
		for r, values := range g.Rows {
			row++
			style := "text"
			if r%2 == 1 {
				style = "text-zebra"
			}
			for i, v := range values {
				b.check(f.SetCellValue(name, cell(i+1, row), v))
			}
			b.check(f.SetCellStyle(name, cell(1, row), cell(len(g.Headers), row), b.style(style)))
		}
	}
	for _, s := range g.Sections {
		row += 2
		b.check(f.SetCellValue(name, cell(1, row), s.Heading))
		b.check(f.MergeCell(name, cell(1, row), last+fmt.Sprint(row)))
		b.check(f.SetCellStyle(name, cell(1, row), last+fmt.Sprint(row), b.style("guide-heading")))
		for _, l := range s.Lines {
			row++
			b.check(f.SetCellValue(name, cell(1, row), "•  "+l))
			b.check(f.MergeCell(name, cell(1, row), last+fmt.Sprint(row)))
			b.check(f.SetCellStyle(name, cell(1, row), last+fmt.Sprint(row), b.style("guide-line")))
			// Uzun satırlar sarılır: birleştirilmiş hücrede Excel yüksekliği kendisi ayarlamaz.
			b.check(f.SetRowHeight(name, row, float64(15*(1+len([]rune(l))/110))+3))
		}
	}
}

// SetMeta, çok gizli kimlik sayfasını yazar: veri sayfasının adı ve kullanan paketin
// anahtarları (kayıt kimliği, sütunların anlamı ...). İçe aktarırken File.Meta olarak okunur.
func (b *Builder) SetMeta(dataSheet string, kv map[string]string) {
	if b.err != nil {
		return
	}
	_, err := b.f.NewSheet(metaSheet)
	b.check(err)
	keys := make([]string, 0, len(kv))
	for k := range kv {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	b.check(b.f.SetCellValue(metaSheet, "A1", metaDataSheet))
	b.check(b.f.SetCellValue(metaSheet, "B1", dataSheet))
	for i, k := range keys {
		b.check(b.f.SetCellValue(metaSheet, cell(1, i+2), k))
		b.check(b.f.SetCellValue(metaSheet, cell(2, i+2), kv[k]))
	}
	b.check(b.f.SetSheetVisible(metaSheet, false, true))
}

// Write, çalışma kitabını yazar; ilk sayfa açık gelir.
func (b *Builder) Write(w io.Writer) error {
	if b.err != nil {
		return b.err
	}
	b.f.SetActiveSheet(0)
	_, err := b.f.WriteTo(w)
	return err
}

// readMeta, kimlik sayfasını okur; yoksa boş.
func readMeta(f *excelize.File) map[string]string {
	if !slices.Contains(f.GetSheetList(), metaSheet) {
		return map[string]string{}
	}
	rows, err := f.GetRows(metaSheet, excelize.Options{RawCellValue: true})
	if err != nil {
		return map[string]string{}
	}
	out := map[string]string{}
	for _, r := range rows {
		if len(r) >= 2 && r[0] != "" {
			out[r[0]] = r[1]
		}
	}
	return out
}
