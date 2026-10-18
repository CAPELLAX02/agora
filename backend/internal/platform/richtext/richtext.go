// Package richtext, kullanıcının yazdığı zengin metni (HTML) izin listesine göre temizler ve düz
// yazıya çevirir: ders içeriği, ödev teslimi, forum gönderisi, duyuru.
package richtext

import (
	"html"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

// policy, zengin metnin izin listesidir: biçimlendirme, başlıklar, listeler, tablolar, bağlantılar,
// görseller ve kod blokları. Betik, stil, olay öznitelikleri ve iframe atılır; bağlantılar yeni
// sekmede ve noopener ile açılır.
var policy = func() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.RequireNoReferrerOnLinks(true)
	p.AddTargetBlankToFullyQualifiedLinks(true)
	p.AllowAttrs("class").Matching(bluemonday.SpaceSeparatedTokens).OnElements("code", "pre", "span")
	return p
}()

// strict, bütün etiketleri atar: görünür yazı.
var strict = bluemonday.StrictPolicy()

// Sanitize, HTML'i izin listesine göre temizler.
func Sanitize(h string) string {
	return policy.Sanitize(h)
}

// PlainText, zengin metnin düz yazısıdır: bloklar satır olur, etiketler atılır, boş satırlar
// ve fazla boşluklar silinir.
func PlainText(h string) string {
	for _, tag := range []string{"</p>", "<br>", "<br/>", "<br />", "</li>", "</h2>", "</h3>", "</blockquote>", "</pre>"} {
		h = strings.ReplaceAll(h, tag, tag+"\n")
	}
	lines := strings.Split(html.UnescapeString(strict.Sanitize(h)), "\n")
	out := lines[:0]
	for _, l := range lines {
		if l = strings.Join(strings.Fields(l), " "); l != "" {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// Empty, zengin metinde görünür yazı olmadığını söyler (ör. sadece boş paragraf).
func Empty(h string) bool {
	return strings.TrimSpace(strings.ReplaceAll(strict.Sanitize(h), "&nbsp;", "")) == ""
}
