// Package mail, e-postaları şablondan üretir, SMTP ile gönderir ve outbox üzerinden
// güvenilir biçimde kuyruğa alır.
//
// Akış: iş kodu e-postayı kendi transaction'ında outbox'a yazar (Enqueue), worker
// süreci outbox'ı okur, şablonu işler ve gönderir (Dispatcher). Böylece e-posta
// sadece iş verisi commit edilirse gönderilir ve SMTP sunucusu geçici olarak
// erişilemezse kaybolmaz, üstel geri çekilmeyle yeniden denenir.
package mail

import (
	"bytes"
	"embed"
	"fmt"
	htmltemplate "html/template"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"slices"
	"strings"
	texttemplate "text/template"
	"time"
)

// Şablon kodları.
const (
	TemplatePasswordReset     = "password_reset"
	TemplateAccountActivation = "account_activation"
)

// Desteklenen diller. İlki varsayılandır.
var locales = []string{"tr", "en"}

//go:embed templates
var templateFS embed.FS

// Message, gönderilmeye hazır bir e-postadır.
type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
}

// Renderer, gömülü şablonlardan e-posta üretir. Bütün şablonlar açılışta derlenir:
// bozuk bir şablon uygulamanın hiç başlamamasına yol açar, ilk e-postada değil.
type Renderer struct {
	text map[string]*texttemplate.Template // "kod.dil" → şablon
	html map[string]*htmltemplate.Template
}

// NewRenderer, şablonları derler.
func NewRenderer() (*Renderer, error) {
	r := &Renderer{text: map[string]*texttemplate.Template{}, html: map[string]*htmltemplate.Template{}}

	for _, code := range []string{TemplatePasswordReset, TemplateAccountActivation} {
		for _, locale := range locales {
			key := code + "." + locale

			t, err := texttemplate.New(key).Option("missingkey=error").ParseFS(templateFS, "templates/"+key+".txt")
			if err != nil {
				return nil, fmt.Errorf("mail: %s metin şablonu derlenemedi: %w", key, err)
			}
			r.text[key] = t

			h, err := htmltemplate.New(key).Option("missingkey=error").ParseFS(templateFS, "templates/layout.html", "templates/"+key+".html")
			if err != nil {
				return nil, fmt.Errorf("mail: %s HTML şablonu derlenemedi: %w", key, err)
			}
			r.html[key] = h
		}
	}
	return r, nil
}

// Render, şablonu verilen dil ve veriyle işler. Dil desteklenmiyorsa varsayılan
// dil (Türkçe) kullanılır. Şablondaki bir alan veride yoksa hata döner.
func (r *Renderer) Render(code, locale, to string, data map[string]any) (Message, error) {
	if !slices.Contains(locales, locale) {
		locale = locales[0]
	}
	key := code + "." + locale

	t, ok := r.text[key]
	if !ok {
		return Message{}, fmt.Errorf("mail: bilinmeyen şablon %q", code)
	}

	var subject, text, html bytes.Buffer
	if err := t.ExecuteTemplate(&subject, "subject", data); err != nil {
		return Message{}, fmt.Errorf("mail: %s konusu işlenemedi: %w", key, err)
	}
	if err := t.ExecuteTemplate(&text, "text", data); err != nil {
		return Message{}, fmt.Errorf("mail: %s metni işlenemedi: %w", key, err)
	}

	layoutData := map[string]any{"Locale": locale, "Subject": subject.String(), "Data": data}
	if err := r.html[key].ExecuteTemplate(&html, "layout", layoutData); err != nil {
		return Message{}, fmt.Errorf("mail: %s HTML'i işlenemedi: %w", key, err)
	}

	return Message{
		To:      to,
		Subject: strings.TrimSpace(subject.String()),
		Text:    strings.TrimSpace(text.String()) + "\n",
		HTML:    html.String(),
	}, nil
}

// Build, mesajı RFC 5322 biçiminde, düz metin ve HTML alternatifleriyle birlikte
// (multipart/alternative) üretir. Gövdeler quoted-printable ile kodlanır: Türkçe
// karakterler 7-bit güvenli taşınır.
func (m Message) Build(from string, date time.Time, messageID string) ([]byte, error) {
	fromAddr, err := mail.ParseAddress(from)
	if err != nil {
		return nil, fmt.Errorf("mail: gönderen adresi geçersiz: %w", err)
	}
	toAddr, err := mail.ParseAddress(m.To)
	if err != nil {
		return nil, fmt.Errorf("mail: alıcı adresi geçersiz: %w", err)
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)

	for _, part := range []struct{ contentType, content string }{
		{"text/plain; charset=utf-8", m.Text},
		{"text/html; charset=utf-8", m.HTML},
	} {
		w, err := mw.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {part.contentType},
			"Content-Transfer-Encoding": {"quoted-printable"},
		})
		if err != nil {
			return nil, err
		}
		qp := quotedprintable.NewWriter(w)
		if _, err := qp.Write([]byte(part.content)); err != nil {
			return nil, err
		}
		if err := qp.Close(); err != nil {
			return nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}

	var out bytes.Buffer
	header := func(k, v string) {
		// Başlık değerlerinde satır sonu olamaz: aksi halde başlık enjeksiyonu olurdu.
		v = strings.NewReplacer("\r", "", "\n", "").Replace(v)
		fmt.Fprintf(&out, "%s: %s\r\n", k, v)
	}
	header("From", fromAddr.String())
	header("To", toAddr.String())
	header("Subject", mime.QEncoding.Encode("utf-8", m.Subject))
	header("Date", date.Format(time.RFC1123Z))
	header("Message-ID", "<"+messageID+">")
	header("MIME-Version", "1.0")
	header("Content-Type", `multipart/alternative; boundary="`+mw.Boundary()+`"`)
	out.WriteString("\r\n")
	out.Write(body.Bytes())

	return out.Bytes(), nil
}
