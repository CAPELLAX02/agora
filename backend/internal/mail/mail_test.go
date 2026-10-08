package mail

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"testing"
	"time"
)

func newRenderer(t *testing.T) *Renderer {
	t.Helper()
	r, err := NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

var resetData = map[string]any{
	"name":            "Ayşe Yılmaz",
	"link":            "http://localhost:5173/sifre-sifirla#token=abc",
	"expires_minutes": 30,
}

func TestRenderAllTemplates(t *testing.T) {
	r := newRenderer(t)
	data := map[string]map[string]any{
		TemplatePasswordReset: resetData,
		TemplateAccountActivation: {
			"name": "Ayşe Yılmaz", "username": "P10003",
			"link": "http://localhost:5173/sifre-sifirla#token=abc", "expires_hours": 72,
		},
	}

	for code, d := range data {
		for _, locale := range locales {
			t.Run(code+"."+locale, func(t *testing.T) {
				m, err := r.Render(code, locale, "ayse@agora.test", d)
				if err != nil {
					t.Fatal(err)
				}
				if m.Subject == "" || strings.Contains(m.Subject, "\n") {
					t.Errorf("konu = %q", m.Subject)
				}
				for _, body := range []string{m.Text, m.HTML} {
					if !strings.Contains(body, "Ayşe Yılmaz") || !strings.Contains(body, "#token=abc") {
						t.Errorf("gövdede ad ya da bağlantı yok:\n%s", body)
					}
				}
				if !strings.Contains(m.HTML, `<html lang="`+locale+`">`) {
					t.Error("HTML yerleşimi uygulanmadı")
				}
			})
		}
	}
}

func TestRenderErrors(t *testing.T) {
	r := newRenderer(t)

	if _, err := r.Render("olmayan", "tr", "a@b.test", nil); err == nil {
		t.Error("bilinmeyen şablon hata vermeli")
	}
	if _, err := r.Render(TemplatePasswordReset, "tr", "a@b.test", map[string]any{"name": "x"}); err == nil {
		t.Error("eksik alan (link) sessizce boş bırakılmamalı")
	}

	m, err := r.Render(TemplatePasswordReset, "de", "a@b.test", resetData)
	if err != nil || !strings.Contains(m.Subject, "parola") {
		t.Errorf("desteklenmeyen dil Türkçeye düşmeli: %q, %v", m.Subject, err)
	}
}

// TestRenderEscapesHTML, kullanıcıdan gelen değerlerin HTML'e kaçışlanarak girdiğini ve
// tehlikeli bağlantıların etkisizleştirildiğini doğrular.
func TestRenderEscapesHTML(t *testing.T) {
	r := newRenderer(t)
	m, err := r.Render(TemplatePasswordReset, "tr", "a@b.test", map[string]any{
		"name": `<script>alert(1)</script>`, "link": "javascript:alert(1)", "expires_minutes": 30,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(m.HTML, "<script>") {
		t.Error("ad kaçışlanmadı")
	}
	if strings.Contains(m.HTML, `href="javascript:`) {
		t.Error("javascript: bağlantısı etkisizleştirilmedi")
	}
}

func TestBuild(t *testing.T) {
	m := Message{
		To:      "ayse@agora.test",
		Subject: "Agora parola sıfırlama\r\nBcc: kotu@example.com",
		Text:    "Merhaba Ayşe, şifreniz güçlü olsun.\n",
		HTML:    "<p>Merhaba Ayşe</p>",
	}
	date := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)

	raw, err := m.Build("Agora <no-reply@agora.test>", date, "abc@agora.test")
	if err != nil {
		t.Fatal(err)
	}

	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("üretilen mesaj çözülemedi: %v\n%s", err, raw)
	}
	if msg.Header.Get("Bcc") != "" {
		t.Fatal("konu üzerinden başlık enjekte edildi")
	}

	subject, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	if err != nil || !strings.HasPrefix(subject, "Agora parola sıfırlama") {
		t.Errorf("konu = %q, %v", subject, err)
	}
	if msg.Header.Get("Message-Id") != "<abc@agora.test>" || msg.Header.Get("Mime-Version") != "1.0" {
		t.Errorf("başlıklar = %v", msg.Header)
	}

	_, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	mr := multipart.NewReader(msg.Body, params["boundary"])
	var parts []string
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(quotedprintable.NewReader(p))
		if err != nil {
			t.Fatal(err)
		}
		parts = append(parts, p.Header.Get("Content-Type")+"|"+string(body))
	}
	if len(parts) != 2 || !strings.HasPrefix(parts[0], "text/plain") || !strings.Contains(parts[0], "şifreniz güçlü") ||
		!strings.HasPrefix(parts[1], "text/html") {
		t.Errorf("parçalar = %q", parts)
	}
}

func TestBackoff(t *testing.T) {
	base, max := 30*time.Second, time.Hour
	for attempt, want := range map[int]time.Duration{1: 30 * time.Second, 2: time.Minute, 3: 2 * time.Minute, 20: time.Hour} {
		got := backoff(attempt, base, max)
		if got < want || got > want+want/5 {
			t.Errorf("backoff(%d) = %v, [%v, %v] aralığında olmalı", attempt, got, want, want+want/5)
		}
	}
}
