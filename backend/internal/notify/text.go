package notify

import (
	"strings"
	"text/template"
)

// Text, bir bildirimin iki dildeki başlığı ve metnidir.
type Text struct {
	TitleTR, TitleEN string
	BodyTR, BodyEN   string
}

// texts, türlerin şablonlarıdır (text/template, veri bildirimin şablon verisi). Kataloğa
// (migration) eklenen her türün burada şablonu olmalı; test ikisini karşılaştırır.
var texts = map[Type]Text{
	PasswordChanged: {
		TitleTR: "Parolanız değiştirildi",
		TitleEN: "Your password was changed",
		BodyTR:  "Hesabınızın parolası değiştirildi ve diğer oturumlarınız kapatıldı. Bu işlemi siz yapmadıysanız hemen parolanızı sıfırlayın.",
		BodyEN:  "Your account password was changed and your other sessions were signed out. If this wasn't you, reset your password right away.",
	},
	MFAEnabled: {
		TitleTR: "İki adımlı doğrulama açıldı",
		TitleEN: "Two-step verification turned on",
		BodyTR:  "Hesabınızda iki adımlı doğrulama açıldı. Kurtarma kodlarınızı güvenli bir yerde saklayın.",
		BodyEN:  "Two-step verification is now on for your account. Keep your recovery codes somewhere safe.",
	},
	MFADisabled: {
		TitleTR: "İki adımlı doğrulama kapatıldı",
		TitleEN: "Two-step verification turned off",
		BodyTR: `{{if eq .by "admin"}}Hesabınızdaki iki adımlı doğrulama bir yönetici tarafından sıfırlandı; güvenlik sayfasından yeniden kurabilirsiniz.` +
			`{{else}}Hesabınızda iki adımlı doğrulama kapatıldı. Bu işlemi siz yapmadıysanız hemen parolanızı değiştirin.{{end}}`,
		BodyEN: `{{if eq .by "admin"}}An administrator reset two-step verification for your account; you can set it up again on the security page.` +
			`{{else}}Two-step verification was turned off for your account. If this wasn't you, change your password right away.{{end}}`,
	},
}

// compiled, şablonların derlenmiş hâlidir: [tür][alan].
var compiled = func() map[Type][4]*template.Template {
	out := make(map[Type][4]*template.Template, len(texts))
	for typ, t := range texts {
		var c [4]*template.Template
		for i, src := range []string{t.TitleTR, t.TitleEN, t.BodyTR, t.BodyEN} {
			c[i] = template.Must(template.New(string(typ)).Option("missingkey=error").Parse(src))
		}
		out[typ] = c
	}
	return out
}()

// Render, bildirimin metnini üretir. Şablon verisi eksik ya da bozuksa (eski bir kaydın
// verisi şablonla uyuşmuyorsa) metin yerine türün adı (fallback) kullanılır.
func Render(typ Type, data map[string]any, fallback Text) Text {
	c, ok := compiled[typ]
	if !ok {
		return fallback
	}
	var out [4]string
	for i, tpl := range c {
		var b strings.Builder
		if err := tpl.Execute(&b, data); err != nil {
			return fallback
		}
		out[i] = b.String()
	}
	return Text{TitleTR: out[0], TitleEN: out[1], BodyTR: out[2], BodyEN: out[3]}
}
