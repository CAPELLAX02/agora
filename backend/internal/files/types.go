package files

import (
	"bytes"
	"net/http"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Dosya türleri bir izin listesidir: uzantı yüklenen dosyanın türünü seçer, içeriğin ilk
// baytları o türle uyuşmalıdır. İstemcinin bildirdiği Content-Type'a güvenilmez. Kaydedilen
// tür (indirmede Content-Type) bu listedendir; tarayıcıda çalışabilecek metinler (HTML, XML)
// düz metin olarak saklanır ve indirme her zaman ek (attachment) olarak verilir.

// family, içerikten tespit edilen dosya ailesidir.
type family int

const (
	famPDF family = iota
	famZip
	famOLE // eski Office biçimleri (doc, xls, ppt)
	famRar
	fam7z
	famGzip
	famText
	famImage
	famVideo
	famAudio
)

// kind, izin verilen bir dosya türüdür: kaydedilen tür ve içerikten beklenen aile.
type kind struct {
	mime   string
	family family
}

var kinds = func() map[string]kind {
	k := map[string]kind{
		".pdf":  {"application/pdf", famPDF},
		".doc":  {"application/msword", famOLE},
		".xls":  {"application/vnd.ms-excel", famOLE},
		".ppt":  {"application/vnd.ms-powerpoint", famOLE},
		".docx": {"application/vnd.openxmlformats-officedocument.wordprocessingml.document", famZip},
		".xlsx": {"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", famZip},
		".pptx": {"application/vnd.openxmlformats-officedocument.presentationml.presentation", famZip},
		".odt":  {"application/vnd.oasis.opendocument.text", famZip},
		".ods":  {"application/vnd.oasis.opendocument.spreadsheet", famZip},
		".odp":  {"application/vnd.oasis.opendocument.presentation", famZip},
		".zip":  {"application/zip", famZip},
		".rar":  {"application/vnd.rar", famRar},
		".7z":   {"application/x-7z-compressed", fam7z},
		".gz":   {"application/gzip", famGzip},
		".tgz":  {"application/gzip", famGzip},
		".png":  {"image/png", famImage},
		".jpg":  {"image/jpeg", famImage},
		".jpeg": {"image/jpeg", famImage},
		".gif":  {"image/gif", famImage},
		".webp": {"image/webp", famImage},
		".mp4":  {"video/mp4", famVideo},
		".webm": {"video/webm", famVideo},
		".mp3":  {"audio/mpeg", famAudio},
		".wav":  {"audio/wav", famAudio},
		".csv":  {"text/csv; charset=utf-8", famText},
	}
	// Metin ve kaynak kod: düz metin olarak saklanır (HTML ve XML dahil, çalıştırılmasın diye).
	for _, ext := range []string{
		".txt", ".md", ".rtf", ".tex", ".json", ".xml", ".html", ".htm", ".css", ".js", ".ts", ".java",
		".py", ".ipynb", ".c", ".h", ".cpp", ".hpp", ".cs", ".go", ".rs", ".sql", ".m", ".r", ".kt",
		".swift", ".php", ".rb", ".sh", ".yaml", ".yml",
	} {
		k[ext] = kind{"text/plain; charset=utf-8", famText}
	}
	return k
}()

// Extensions, izin verilen uzantılardır (sözleşme ve arayüz için).
func Extensions() []string {
	out := make([]string, 0, len(kinds))
	for ext := range kinds {
		out = append(out, ext)
	}
	return out
}

// matches, dosyanın ilk baytlarının türün ailesiyle uyuştuğunu söyler.
func (k kind) matches(head []byte) bool {
	sniffed := http.DetectContentType(head)
	switch k.family {
	case famPDF:
		return sniffed == "application/pdf"
	case famZip:
		return sniffed == "application/zip"
	case famOLE:
		return bytes.HasPrefix(head, []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1})
	case famRar:
		return sniffed == "application/x-rar-compressed"
	case fam7z:
		return bytes.HasPrefix(head, []byte{'7', 'z', 0xBC, 0xAF, 0x27, 0x1C})
	case famGzip:
		return sniffed == "application/x-gzip"
	case famImage:
		return sniffed == strings.SplitN(k.mime, ";", 2)[0]
	case famVideo:
		return sniffed == "video/mp4" || sniffed == "video/webm"
	case famAudio:
		return sniffed == "audio/mpeg" || sniffed == "audio/wave"
	case famText:
		// Metin dosyası ikili içerik taşımamalı: HTML gibi tespit edilenler de metindir.
		return strings.HasPrefix(sniffed, "text/") && !bytes.ContainsRune(head, 0)
	}
	return false
}

// cleanName, istemcinin gönderdiği dosya adını güvenli hale getirir: yol bileşenleri ve
// kontrol karakterleri atılır, en fazla 255 karakter. Uzantı küçük harfle döner.
func cleanName(name string) (clean, ext string) {
	name = strings.ReplaceAll(name, "\\", "/")
	name = path.Base(strings.TrimSpace(name))
	if name == "." || name == "/" {
		return "", ""
	}
	var b strings.Builder
	for _, r := range name {
		if r == utf8.RuneError || unicode.IsControl(r) || r == '"' {
			continue
		}
		b.WriteRune(r)
	}
	clean = strings.TrimSpace(b.String())
	if runes := []rune(clean); len(runes) > 255 {
		ext := path.Ext(clean)
		clean = string(runes[:255-len([]rune(ext))]) + ext
	}
	return clean, strings.ToLower(path.Ext(clean))
}
