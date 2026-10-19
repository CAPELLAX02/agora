package notify

import (
	"fmt"
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

	// Ders kaydı. Veri: student, student_no, term, courses, ects.
	RegistrationSubmitted: {
		TitleTR: "{{.student}} ders kaydını onayınıza gönderdi",
		TitleEN: "{{.student}} sent their course registration for your approval",
		BodyTR:  "{{.student}} ({{.student_no}}), {{.term}} dönemi ders kaydını onayınıza gönderdi: {{.courses}} ders, {{.ects}} AKTS.",
		BodyEN:  "{{.student}} ({{.student_no}}) sent their {{.term}} course registration for your approval: {{.courses}} courses, {{.ects}} ECTS.",
	},
	// Veri: decision (APPROVED, REJECTED, REVOKED), term, courses, ects, note.
	RegistrationDecision: {
		TitleTR: `{{if eq .decision "APPROVED"}}Ders kaydınız onaylandı{{else if eq .decision "REJECTED"}}Ders kaydınız düzeltme için geri gönderildi{{else}}Ders kaydınızın onayı geri alındı{{end}}`,
		TitleEN: `{{if eq .decision "APPROVED"}}Your course registration was approved{{else if eq .decision "REJECTED"}}Your course registration was sent back for changes{{else}}The approval of your course registration was revoked{{end}}`,
		BodyTR: `{{if eq .decision "APPROVED"}}{{.term}} dönemi ders kaydınız danışmanınız tarafından onaylandı: {{.courses}} ders, {{.ects}} AKTS.` +
			`{{else if eq .decision "REJECTED"}}{{.term}} dönemi ders kaydınız danışmanınız tarafından düzeltilmek üzere geri gönderildi. Ders seçme süresi içinde sepetinizi düzenleyip yeniden gönderebilirsiniz.` +
			`{{else}}{{.term}} dönemi ders kaydınızın onayı geri alındı. Ders seçme süresi içinde sepetinizi düzenleyip yeniden gönderebilirsiniz.{{end}}` +
			`{{if .note}} Danışman notu: {{.note}}{{end}}`,
		BodyEN: `{{if eq .decision "APPROVED"}}Your {{.term}} course registration was approved by your advisor: {{.courses}} courses, {{.ects}} ECTS.` +
			`{{else if eq .decision "REJECTED"}}Your {{.term}} course registration was sent back for changes by your advisor. You can edit and resubmit your basket while course registration is open.` +
			`{{else}}The approval of your {{.term}} course registration was revoked. You can edit and resubmit your basket while course registration is open.{{end}}` +
			`{{if .note}} Advisor's note: {{.note}}{{end}}`,
	},
	// Veri: student, student_no, term, stage (ADVISOR, DEPT_HEAD).
	AddDropPending: {
		TitleTR: "{{.student}} ekle-bırak talebi onayınızı bekliyor",
		TitleEN: "{{.student}}'s add/drop request awaits your approval",
		BodyTR:  `{{.student}} ({{.student_no}}), {{.term}} dönemi için ekle-bırak talebinde bulundu{{if eq .stage "DEPT_HEAD"}} ve danışmanı onayladı{{end}}. Talep onayınızı bekliyor.`,
		BodyEN:  `{{.student}} ({{.student_no}}) made an add/drop request for {{.term}}{{if eq .stage "DEPT_HEAD"}} and their advisor approved it{{end}}. The request awaits your approval.`,
	},
	// Veri: decision (APPLIED, REJECTED), term, note.
	AddDropDecision: {
		TitleTR: `{{if eq .decision "APPLIED"}}Ekle-bırak talebiniz onaylandı{{else}}Ekle-bırak talebiniz reddedildi{{end}}`,
		TitleEN: `{{if eq .decision "APPLIED"}}Your add/drop request was approved{{else}}Your add/drop request was rejected{{end}}`,
		BodyTR: `{{if eq .decision "APPLIED"}}{{.term}} dönemi ekle-bırak talebiniz onaylandı ve ders kaydınıza uygulandı.` +
			`{{else}}{{.term}} dönemi ekle-bırak talebiniz reddedildi.{{if .note}} Gerekçe: {{.note}}{{end}}{{end}}`,
		BodyEN: `{{if eq .decision "APPLIED"}}Your {{.term}} add/drop request was approved and applied to your registration.` +
			`{{else}}Your {{.term}} add/drop request was rejected.{{if .note}} Reason: {{.note}}{{end}}{{end}}`,
	},

	// Notlar. Veri: course_code, course_tr, course_en, component_tr, component_en.
	ComponentPublished: {
		TitleTR: "{{.course_code}} · {{.component_tr}} notunuz yayımlandı",
		TitleEN: "{{.course_code}} · your {{.component_en}} score was published",
		BodyTR:  "{{.course_tr}} dersinde {{.component_tr}} notları yayımlandı. Notunuzu ve sınıf ortalamasını Notlarım'da görebilirsiniz.",
		BodyEN:  "{{.component_en}} scores for {{.course_en}} were published. See your score and the class average in My grades.",
	},
	// Veri: course_code, course_tr, course_en, phase (FINAL, MAKEUP, CORRECTION).
	LettersPublished: {
		TitleTR: `{{.course_code}} {{if eq .phase "MAKEUP"}}bütünleme sonucunuz ilan edildi{{else if eq .phase "CORRECTION"}}notunuz düzeltildi{{else}}harf notunuz ilan edildi{{end}}`,
		TitleEN: `{{.course_code}} {{if eq .phase "MAKEUP"}}make-up result announced{{else if eq .phase "CORRECTION"}}grade corrected{{else}}letter grade announced{{end}}`,
		BodyTR: `{{if eq .phase "MAKEUP"}}{{.course_tr}} dersinin bütünleme sonuçları ilan edildi.` +
			`{{else if eq .phase "CORRECTION"}}{{.course_tr}} dersindeki notunuz onaylanan bir düzeltmeyle değişti.` +
			`{{else}}{{.course_tr}} dersinin harf notları ilan edildi.{{end}} Notunuzu Notlarım'da görebilirsiniz.`,
		BodyEN: `{{if eq .phase "MAKEUP"}}Make-up results for {{.course_en}} were announced.` +
			`{{else if eq .phase "CORRECTION"}}Your grade in {{.course_en}} changed with an approved correction.` +
			`{{else}}Letter grades for {{.course_en}} were announced.{{end}} See your grade in My grades.`,
	},
	// Veri: course_code, student, requester.
	ChangeRequested: {
		TitleTR: "{{.course_code}} için not düzeltme talebi",
		TitleEN: "Grade change request for {{.course_code}}",
		BodyTR:  "{{.requester}}, {{.course_code}} dersinde {{.student}} için not düzeltme talep etti. Talep onayınızı bekliyor.",
		BodyEN:  "{{.requester}} requested a grade change for {{.student}} in {{.course_code}}. The request awaits your approval.",
	},
	// Veri: course_code, student, decision (APPLIED, REJECTED), note.
	ChangeDecided: {
		TitleTR: `{{.course_code}} not düzeltme talebiniz {{if eq .decision "APPLIED"}}onaylandı{{else}}reddedildi{{end}}`,
		TitleEN: `Your grade change request for {{.course_code}} was {{if eq .decision "APPLIED"}}approved{{else}}rejected{{end}}`,
		BodyTR: `{{.student}} için not düzeltme talebiniz {{if eq .decision "APPLIED"}}onaylandı ve uygulandı.{{else}}reddedildi.{{end}}` +
			`{{if .note}} Not: {{.note}}{{end}}`,
		BodyEN: `Your grade change request for {{.student}} was {{if eq .decision "APPLIED"}}approved and applied.{{else}}rejected.{{end}}` +
			`{{if .note}} Note: {{.note}}{{end}}`,
	},
	// Ders alanı. Veri: course_code, title, due_tr, due_en (son tarih yoksa boş).
	AssignmentCreated: {
		TitleTR: "{{.course_code}} · yeni ödev: {{.title}}",
		TitleEN: "{{.course_code}} · new assignment: {{.title}}",
		BodyTR:  `“{{.title}}” ödevi yayımlandı.{{if .due_tr}} Son tarih: {{.due_tr}}.{{end}}`,
		BodyEN:  `The assignment “{{.title}}” was posted.{{if .due_en}} Due: {{.due_en}}.{{end}}`,
	},
	// Veri: course_code, title.
	AssignmentReleased: {
		TitleTR: "{{.course_code}} · {{.title}} puanınız yayımlandı",
		TitleEN: "{{.course_code}} · your {{.title}} score was released",
		BodyTR:  "“{{.title}}” ödevinin puanları ve geri bildirimleri yayımlandı.",
		BodyEN:  "Scores and feedback for “{{.title}}” were released.",
	},
	// Veri: course_code, title, author, excerpt (gönderinin düz metni, kısaltılmış).
	ForumAnnouncement: {
		TitleTR: "{{.course_code}} duyurusu: {{.title}}",
		TitleEN: "{{.course_code}} announcement: {{.title}}",
		BodyTR:  "{{.author}}: {{.excerpt}}",
		BodyEN:  "{{.author}}: {{.excerpt}}",
	},
	// Veri: course_code, thread, author.
	ForumReply: {
		TitleTR: "{{.author}}, “{{.thread}}” konusuna yanıt yazdı",
		TitleEN: "{{.author}} replied to “{{.thread}}”",
		BodyTR:  "{{.course_code}} forumunda yazdığınız “{{.thread}}” konusunda yeni bir yanıt var.",
		BodyEN:  "There is a new reply in “{{.thread}}”, a {{.course_code}} thread you posted in.",
	},
	// Veri: course_code, forum, thread, author.
	ForumThread: {
		TitleTR: "{{.course_code}} · {{.forum}}: {{.thread}}",
		TitleEN: "{{.course_code}} · {{.forum}}: {{.thread}}",
		BodyTR:  "{{.author}}, abone olduğunuz {{.forum}} forumunda “{{.thread}}” konusunu açtı.",
		BodyEN:  "{{.author}} started “{{.thread}}” in {{.forum}}, a forum you follow.",
	},
	// Kurumsal duyurular. Veri: title, summary (boş olabilir), author.
	AnnouncementPublished: {
		TitleTR: "Duyuru: {{.title}}",
		TitleEN: "Announcement: {{.title}}",
		BodyTR:  "{{if .summary}}{{.summary}}{{else}}{{.author}} yeni bir duyuru yayımladı.{{end}}",
		BodyEN:  "{{if .summary}}{{.summary}}{{else}}{{.author}} posted a new announcement.{{end}}",
	},
	AnnouncementUrgent: {
		TitleTR: "Acil duyuru: {{.title}}",
		TitleEN: "Urgent announcement: {{.title}}",
		BodyTR:  "{{if .summary}}{{.summary}}{{else}}{{.author}} acil bir duyuru yayımladı.{{end}}",
		BodyEN:  "{{if .summary}}{{.summary}}{{else}}{{.author}} posted an urgent announcement.{{end}}",
	},
	// Mesajlaşma. Veri: sender, conversation (grup adı; birebirde boş), preview.
	MessageReceived: {
		TitleTR: "{{.sender}} size ileti gönderdi",
		TitleEN: "{{.sender}} sent you a message",
		BodyTR:  "{{if .conversation}}{{.conversation}}: {{end}}{{.preview}}",
		BodyEN:  "{{if .conversation}}{{.conversation}}: {{end}}{{.preview}}",
	},
	// Hatırlatmalar. Veri: window_tr, window_en, at_tr, at_en (açılış ya da kapanış anı).
	WindowOpening: {
		TitleTR: "{{.window_tr}} yakında başlıyor",
		TitleEN: "{{.window_en}} starts soon",
		BodyTR:  "{{.window_tr}} {{.at_tr}} tarihinde başlıyor.",
		BodyEN:  "{{.window_en}} starts on {{.at_en}}.",
	},
	WindowClosing: {
		TitleTR: "{{.window_tr}} yakında sona eriyor",
		TitleEN: "{{.window_en}} ends soon",
		BodyTR:  "{{.window_tr}} {{.at_tr}} tarihinde sona eriyor; işlemlerinizi tamamlamayı unutmayın.",
		BodyEN:  "{{.window_en}} ends on {{.at_en}}; don't forget to complete your steps.",
	},
	// Veri: course_code, title, due_tr, due_en.
	AssignmentDue: {
		TitleTR: "{{.course_code}} · “{{.title}}” için son tarih yaklaşıyor",
		TitleEN: "{{.course_code}} · “{{.title}}” is due soon",
		BodyTR:  "Ödevinizi henüz göndermediniz. Son tarih: {{.due_tr}}.",
		BodyEN:  "You haven't submitted this assignment yet. Due: {{.due_en}}.",
	},
}

// compiled, şablonların derlenmiş hâlidir: [tür][alan]. Bozuk bir şablon uygulamanın hiç
// başlamamasına yol açar.
var compiled = func() map[Type][4]*template.Template {
	out := make(map[Type][4]*template.Template, len(texts))
	for typ, t := range texts {
		out[typ] = compile(typ, t)
	}
	return out
}()

func compile(typ Type, t Text) [4]*template.Template {
	var c [4]*template.Template
	for i, src := range []string{t.TitleTR, t.TitleEN, t.BodyTR, t.BodyEN} {
		c[i] = template.Must(template.New(string(typ)).Option("missingkey=error").Parse(src))
	}
	return c
}

// Render, bildirimin metnini üretir. Şablon verisi eksik ya da bozuksa (eski bir kaydın
// verisi şablonla uyuşmuyorsa) metin yerine türün adı (fallback) kullanılır.
func Render(typ Type, data map[string]any, fallback Text) Text {
	t, err := render(typ, data)
	if err != nil {
		return fallback
	}
	return t
}

func render(typ Type, data map[string]any) (Text, error) {
	c, ok := compiled[typ]
	if !ok {
		return Text{}, fmt.Errorf("notify: bilinmeyen bildirim türü %q", typ)
	}
	var out [4]string
	for i, tpl := range c {
		var b strings.Builder
		if err := tpl.Execute(&b, data); err != nil {
			return Text{}, fmt.Errorf("notify: %s şablonu işlenemedi: %w", typ, err)
		}
		out[i] = b.String()
	}
	return Text{TitleTR: out[0], TitleEN: out[1], BodyTR: out[2], BodyEN: out[3]}, nil
}
