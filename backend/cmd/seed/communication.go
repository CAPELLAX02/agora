package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CAPELLAX02/agora/backend/internal/announce"
	"github.com/CAPELLAX02/agora/backend/internal/messaging"
)

// İletişim demosu: kurumsal duyurular (üniversite, fakülte ve bölüm; biri acil, biri ileri tarihli,
// biri taslak) gerçek yayımlama yoluyla, yani hedef kitledeki hesaplara bildirimiyle; demo
// sınıfının yarısı sabitlenmiş duyuruları okumuştur. Konuşmalar: danışman P10001 ile öğrencisi
// 26290004 arasında birebir, COM1007-1 şube grubu ve bölüm başkanının kurduğu öğretim elemanları
// grubu; iletiler son iki güne yayılır, son iletiyi alan okumamıştır. Duyuru varsa duyurular,
// ileti varsa konuşmalar atlanır.

// seedCommunicationDemo, iletişim demosunu kurar.
func seedCommunicationDemo(ctx context.Context, pool *pgxpool.Pool) error {
	ids := map[string]string{}
	rows, err := pool.Query(ctx, `SELECT username, id FROM iam.users WHERE username = ANY($1)`,
		append([]string{"P10001", "P10002", "P10003", "P10004", "P10005", "P10006", "P10007", "P10008", "P20001", "P20002", "26290004"},
			cohortNumbers()...))
	if err != nil {
		return err
	}
	var name, id string
	if _, err := pgx.ForEachRow(rows, []any{&name, &id}, func() error { ids[name] = id; return nil }); err != nil {
		return err
	}
	for _, u := range []string{"P10001", "P10002", "P10007", "P20001", "P20002", "26290004"} {
		if ids[u] == "" {
			fmt.Println("ATLANDI    iletişim demosu: demo hesapları yok")
			return nil
		}
	}
	if err := seedAnnouncements(ctx, pool, ids); err != nil {
		return fmt.Errorf("duyurular: %w", err)
	}
	if err := seedConversations(ctx, pool, ids); err != nil {
		return fmt.Errorf("konuşmalar: %w", err)
	}
	return nil
}

// seedAnnouncements, demo duyurularını yayımlar.
func seedAnnouncements(ctx context.Context, pool *pgxpool.Pool, ids map[string]string) error {
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM communication.announcements)`).Scan(&exists); err != nil {
		return err
	}
	if exists {
		fmt.Println("ATLANDI    duyurular: zaten var")
		return nil
	}
	var muh, bil string
	if err := pool.QueryRow(ctx, `
		SELECT (SELECT id FROM org.faculties WHERE code = 'MUH'), (SELECT id FROM org.departments WHERE code = 'BIL')`).Scan(&muh, &bil); err != nil {
		return err
	}
	now := time.Now()
	ptr := func(t time.Time) *time.Time { return &t }
	svc := announce.NewService(pool, time.Now)
	type item struct {
		author  string
		publish bool
		in      announce.Input
	}
	items := []item{
		{"P20002", true, announce.Input{Type: announce.TypeAcademic, Pinned: true,
			Title:   "2026-2027 akademik takvimi yayımlandı",
			Summary: "Güz ve bahar dönemlerinin ders, sınav ve işlem tarihleri Akademik takvim sayfasında.",
			Body: `<p>2026-2027 eğitim-öğretim yılının akademik takvimi yayımlanmıştır. Ders seçme, ekle-bırak,
not girişi ve sınav dönemlerinin tarihlerini <strong>Akademik takvim</strong> sayfasından izleyebilirsiniz.</p>
<p>Fakültelerin kendi işlem tarihleri ayrıca duyurulur.</p>`,
			Audiences: []announce.Audience{{ScopeType: announce.ScopeUniversity, People: "ALL"}}}},
		{"P20002", true, announce.Input{Type: announce.TypeUrgent, PublishUntil: ptr(now.Add(3 * 24 * time.Hour)),
			Title:   "Merkez kütüphane hafta sonu kapalı",
			Summary: "Bakım çalışması nedeniyle merkez kütüphane cumartesi ve pazar günleri hizmet vermeyecek.",
			Body: `<p>Elektrik altyapısındaki bakım çalışması nedeniyle merkez kütüphane bu hafta sonu kapalıdır.
Fakülte kütüphaneleri ve e-kaynaklar erişime açıktır.</p>`,
			Audiences: []announce.Audience{{ScopeType: announce.ScopeUniversity, People: "STUDENTS"}}}},
		{"P20001", true, announce.Input{Type: announce.TypeEvent,
			Title:   "Bitirme projesi tanıtım günleri",
			Summary: "Mühendislik Fakültesi bölümlerinin bitirme projesi konuları öğrencilere tanıtılıyor.",
			Body: `<p>Son sınıf öğrencilerimiz için bitirme projesi tanıtım günleri gelecek hafta fakülte konferans
salonunda yapılacaktır. Öğretim üyelerimiz proje konularını ve beklentilerini anlatacaktır.</p>
<ul><li>Bilgisayar Mühendisliği: Salı 13.30</li><li>Diğer bölümler: Çarşamba 13.30</li></ul>`,
			Audiences: []announce.Audience{{ScopeType: announce.ScopeFaculty, ScopeID: muh, People: "STUDENTS"}}}},
		{"P10002", true, announce.Input{Type: announce.TypeGeneral, Pinned: true,
			Title:   "Bölüm tanışma toplantısı",
			Summary: "Bilgisayar Mühendisliği öğrencileri ve öğretim elemanları perşembe 15.00'te A101'de buluşuyor.",
			Body: `<p>Yeni dönemin başında bölümümüzün öğrencileri ve öğretim elemanlarıyla tanışma toplantısı
düzenliyoruz. Danışmanlık, staj ve laboratuvar olanakları hakkında bilgi verilecektir.</p>
<p><strong>Yer:</strong> A101 · <strong>Saat:</strong> Perşembe 15.00</p>`,
			Audiences: []announce.Audience{{ScopeType: announce.ScopeDepartment, ScopeID: bil, People: "ALL"}}}},
		{"P10002", true, announce.Input{Type: announce.TypeExam, PublishFrom: ptr(now.Add(24 * time.Hour)),
			Title:   "Ara sınav programı",
			Summary: "Bölüm derslerinin ara sınav tarihleri ve salonları.",
			Body: `<p>Bilgisayar Mühendisliği derslerinin ara sınav programı ektedir. Sınavlara öğrenci
kimlik kartınızla gelmeniz gerekmektedir.</p>`,
			Audiences: []announce.Audience{{ScopeType: announce.ScopeDepartment, ScopeID: bil, People: "STUDENTS"}}}},
		{"P10002", false, announce.Input{Type: announce.TypeAcademic,
			Title:     "Yaz stajı başvuruları",
			Summary:   "Staj başvuru takvimi ve gerekli belgeler.",
			Body:      `<p>Yaz stajı başvuruları için gerekli belgeler ve takvim yakında duyurulacaktır.</p>`,
			Audiences: []announce.Audience{{ScopeType: announce.ScopeDepartment, ScopeID: bil, People: "STUDENTS"}}}},
	}
	var pinned []string
	for _, it := range items {
		id, err := svc.Create(ctx, ids[it.author], it.in, it.publish)
		if err != nil {
			return fmt.Errorf("%s: %w", it.in.Title, err)
		}
		if it.in.Pinned {
			pinned = append(pinned, id)
		}
	}
	// Demo sınıfının yarısı sabitlenmiş duyuruları okumuştur (yönetim ekranında okunma oranı).
	cohort := cohortNumbers()
	for _, no := range cohort[:len(cohort)/2] {
		for _, id := range pinned {
			if err := svc.MarkRead(ctx, id, ids[no]); err != nil {
				return err
			}
		}
	}
	fmt.Printf("OLUŞTURULDU duyurular: %d (biri ileri tarihli, biri taslak)\n", len(items))
	return nil
}

// demoMessage, bir demo iletisidir: gönderen, metin ve ne kadar önce gönderildiği.
type demoMessage struct {
	from string
	body string
	ago  time.Duration
}

// seedConversations, demo konuşmalarını kurar.
func seedConversations(ctx context.Context, pool *pgxpool.Pool, ids map[string]string) error {
	var exists bool
	if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM communication.messages)`).Scan(&exists); err != nil {
		return err
	}
	if exists {
		fmt.Println("ATLANDI    konuşmalar: zaten var")
		return nil
	}
	svc := messaging.NewService(pool, nil, time.Now)
	type sent struct {
		id  string
		ago time.Duration
	}
	var all []sent
	post := func(conversation string, msgs []demoMessage) error {
		for _, m := range msgs {
			id, err := svc.Send(ctx, ids[m.from], conversation, m.body, "", nil)
			if err != nil {
				return err
			}
			all = append(all, sent{id, m.ago})
		}
		return nil
	}
	h := time.Hour

	direct, err := svc.Direct(ctx, ids["P10001"], ids["26290004"])
	if err != nil {
		return err
	}
	if err := post(direct, []demoMessage{
		{"26290004", "Merhaba hocam, ders seçimimle ilgili kısa bir görüşme yapabilir miyiz?", 26 * h},
		{"P10001", "Merhaba Mert, tabii. Salı 14.00–16.00 arası ofis saatim, B-312'ye uğrayabilirsin.", 25 * h},
		{"26290004", "Teşekkürler hocam, salı uğrarım.", 24*h + 30*time.Minute},
		{"P10001", "Gelmeden önce sepetindeki dersleri bir gözden geçir; ön koşulları birlikte konuşalım.", 3 * h},
	}); err != nil {
		return err
	}

	// Şube grubu, öğretim elemanının konuşmaları ilk listelendiğinde açılır.
	var section string
	if err := pool.QueryRow(ctx, `
		SELECT s.id FROM offering.sections s
		JOIN offering.course_offerings o ON o.id = s.offering_id
		JOIN academic.terms t ON t.id = o.term_id AND t.is_current
		JOIN curriculum.courses c ON c.id = o.course_id
		WHERE c.code = $1 AND s.section_code = $2`, demoClassCourse, demoClassSection).Scan(&section); err != nil {
		return fmt.Errorf("demo sınıfı okunamadı: %w", err)
	}
	list, err := svc.Conversations(ctx, ids["P10001"])
	if err != nil {
		return err
	}
	group := ""
	for _, c := range list {
		if c.Kind == messaging.KindSection && c.SectionID == section {
			group = c.ID
		}
	}
	cohort := cohortNumbers()
	if group != "" && len(cohort) >= 2 {
		if err := post(group, []demoMessage{
			{"P10001", "Arkadaşlar merhaba, bu grup COM1007-1 şubesinin ortak konuşması. Ders ve laboratuvarla ilgili kısa duyuruları buradan da paylaşacağım.", 40 * h},
			{"P10007", "İlk laboratuvarda Go kurulumunu birlikte yapacağız; dizüstü bilgisayarlarınızı getirin.", 39 * h},
			{cohort[0], "Hocam, laboratuvardan önce bir şey kurmamız gerekiyor mu?", 20 * h},
			{"P10007", "Gerek yok, ders alanındaki kurulum sayfasını laboratuvarda birlikte izleyeceğiz.", 19 * h},
			{cohort[1], "Ödev 1'i tek dosya olarak mı yükleyeceğiz?", 2 * h},
		}); err != nil {
			return err
		}
	}

	staff := []string{}
	for _, no := range []string{"P10001", "P10003", "P10004", "P10005", "P10006", "P10007", "P10008"} {
		if ids[no] != "" {
			staff = append(staff, ids[no])
		}
	}
	board, err := svc.CreateGroup(ctx, ids["P10002"], "BIL öğretim elemanları", staff)
	if err != nil {
		return err
	}
	boardMsgs := []demoMessage{
		{"P10002", "Değerli hocalarım, bölüm kurulu perşembe 10.00'da toplanacak. Gündem önerilerinizi buraya yazabilirsiniz.", 30 * h},
		{"P10001", "Ders alanının ilk haftalardaki kullanımıyla ilgili kısa bir değerlendirme sunabilirim.", 28 * h},
	}
	if ids["P10005"] != "" {
		boardMsgs = append(boardMsgs, demoMessage{"P10005", "Laboratuvar bilgisayarlarının yenilenmesini de gündeme almayı öneririm.", 27 * h})
	}
	if err := post(board, boardMsgs); err != nil {
		return err
	}

	// İletiler son iki güne yayılır (sıra kimlikle korunur: aynı sırayla gönderildiler).
	for _, m := range all {
		if _, err := pool.Exec(ctx, `UPDATE communication.messages SET created_at = now() - $2::interval WHERE id = $1`,
			m.id, fmt.Sprintf("%d seconds", int(m.ago.Seconds()))); err != nil {
			return err
		}
	}
	if _, err := pool.Exec(ctx, `
		UPDATE communication.conversations c SET last_message_at = m.at
		FROM (SELECT conversation_id, max(created_at) AS at FROM communication.messages GROUP BY conversation_id) m
		WHERE m.conversation_id = c.id`); err != nil {
		return err
	}
	fmt.Printf("OLUŞTURULDU konuşmalar: birebir, şube grubu ve öğretim elemanları grubu (%d ileti)\n", len(all))
	return nil
}
