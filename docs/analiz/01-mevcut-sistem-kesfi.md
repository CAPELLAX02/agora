# 01 · Mevcut Sistem Keşfi (OBS + e-Kampüs)

> **Durum:** Taslak v0.1 · **Keşif tarihi:** 2026-10-04 · **Bakış açısı:** Öğrenci rolü
>
> Bu doküman, Ankara Üniversitesi'nin bugün kullandığı iki sistemin (OBS ve e-Kampüs) öğrenci gözünden ekran ekran envanteridir. Kişisel veri (numara, not, ortalama, isim vb.) bilerek **yazılmamıştır**; yalnızca ekranlar, alanlar, iş kuralları ve gözlemlenen sorunlar not edilmiştir.
>
> Akademik personel, danışman, bölüm başkanı ve öğrenci işleri ekranları doğrudan görülemedi. Bu rollerin yetenekleri; akademik takvim, ders seçme SSS'si ve ekranlardaki iş akışı ipuçlarından **çıkarım** yoluyla yazılmıştır (🔎 ile işaretli).

---

## 1. Genel Teknik Gözlemler

| Konu | OBS (`obs.ankara.edu.tr`) | e-Kampüs (`ekampus.ankara.edu.tr`) |
| --- | --- | --- |
| Teknoloji | ASP.NET MVC (IIS hata sayfaları, `/Account/...` rotaları) | Moodle 4.x (Açık ve Uzaktan Eğitim Fakültesi işletiyor) |
| Altyapı | Cloudflare arkasında, birden çok uygulama sunucusu (`OBSAPPW105`, `OBSAPPW110` … — sayfa altında "X ms'de yüklendi") | Ayrı sunucu, ayrı oturum |
| Giriş | Kullanıcı adı + şifre, "Şifremi Unuttum" | OBS içinden token ile tek yönlü SSO ("e-Kampüs OYS Öğrenci Giriş") |
| Oturum | 120 dk boşta kalma sayacı ("Kalan Süre") | Moodle oturumu, tarayıcı oturumları listesi |
| Dil | TR / EN | TR / EN |
| Mobil | Ayrı "Ankara Üniversitesi Mobil Uygulamaları" | Moodle mobil uygulaması (QR ile otomatik giriş, 10 dk geçerli) |
| Ölçek (OBS üst bandı) | **91.797 öğrenci · 10.120 akademik personel · 667 idari personel** | — |

> Bu ölçek bilgisi, yük testi senaryolarımızda (özellikle ders seçme açılışı) gerçekçi hedef olarak kullanılacak.

---

## 2. OBS — Ekran Envanteri

### 2.1 Menü ağacı (öğrenci)

```text
Öğrenci
├── İşlemler
│   ├── Akademik Takvim                         /student/academiccalendar
│   ├── Danışman Bilgileri                      /student/advisor
│   ├── E-imzalı Transcript/Öğrenci Belgesi     /TranscriptEbeyas/index
│   ├── Sanal Transkript                        /transcript/virtual
│   └── YÖKSİS E-Devlet Bilgi Güncelle          /transcript/sendyoksis
├── Raporlar
│   ├── Ders Planı                              /student/lessonplan
│   ├── Ders Programı                           /student/lessonprogram
│   ├── Transkript                              (PDF rapor)
│   ├── Sınav Sonuçları                         /student/lessongradebystudent
│   ├── Ders Bazlı Devam Raporu                 /student/studentattendancereport
│   ├── Kayıt Onay Raporu                       (PDF rapor)
│   └── Karne Raporu                            (PDF rapor)
└── Azami Sınav Durumu                          /student/AzamiSureBilgisi
Öğrenci BYS
├── Çift Anadal / Yandal Yerleştirme Onay      /student/ciftanadalyandalonay
└── Ders İşlemleri › Ders Seçme İşlemleri
    ├── Ders Seçme İşlemi                       /reregistration/syllabusselection
    └── Ders Seçme Raporları
Kampüs Kart Seç                                 (harici: kampuskart.ankara.edu.tr)
Kütüphane İşlemleri › Kütüphane Kayıt / Kütüphane Durum   (harici entegrasyon, modal)
Aydınlatma Metni (KVKK)                         /Account/PersonalDataProtectionLaw
e-Kampüs'e Giriş                                (SSO modalı)
ANKUZEF OYS'ye Giriş                            (uzaktan eğitim programları için ayrı LMS)
Hazırlık Kayıt                                  /Preparation/HazirlikDonemKayit
Yardım Masası                                   /HelpDesk
Aktif Bölüm: <Program> (N.Ö.) ▾                 (birden çok program kaydı arasında geçiş)
```

Üst çubukta ayrıca **arama**, **yardım**, **oturumu kapat**, **Sık Kullanılanlar** (sayfayı favorilere ekle), **dil seçimi** ve **Ekran Ayarları** bulunuyor.

### 2.2 Giriş sayfası (oturum öncesi)

- Kullanıcı adı + şifre, "Şifremi Unuttum".
- **Öğrenci Bilgi Formu**: yeni kayıt olan öğrenci önce bu formu doldurur. Kullanıcı adı ve şifre forma girilen e-postaya gönderilir. Öğrenci ancak **aktif** duruma geçince giriş yapabilir.
- **YÖS Öğrenci Kayıt Formu** (yabancı uyruklu öğrenciler için).
- **Diploma İlişik Kesme Süreçleri**: mezuniyetteki ilişik kesme iş akışı.
- Duyuru: e-Devlet ile kayıt, aktiflik durumu, Kampüs Kart talebi, ders seçme takvimi.

### 2.3 Kontrol Paneli (dashboard)

- Widget'lar: **Son Mesajlar** (Kimden, Kime, Konu, Gönderilme Tarihi), **Akademik Takvim**, **Son Duyurular** (başlık + detay), **Anketler** (cevaplanacak aktif anketler).
- "Ekran Ayarları" ile kullanıcı hangi widget'ların görüneceğini seçebiliyor: Akademik Takvim, Anketler, Sisteme Son Giriş, Son Duyurular, Son İşlemler, Son Mesajlar, Tarihçe.
- Üst bantta aktif akademik yıl/dönem ve üniversite geneli sayaçlar.

### 2.4 Duyuru Detayı (`/Announce/Detail/{id}`)

Alanlar: **Duyuru #ID**, başlık, yayın tarihi, Durum (Aktif/Pasif), Başlangıç Tarihi, Bitiş Tarihi, Duyuru Türü, **Özet**, **İçerik**.

### 2.5 Akademik Takvim

- Seçim: Akademik yıl + dönem (Güz/Bahar).
- Kayıt yapısı: **Başlangıç · Bitiş · Aktivite**.
- Gözlemlenen aktivite türleri, sistemin **zaman penceresi** kurallarını belirliyor:

| Aktivite | Kim etkilenir |
| --- | --- |
| ÖSYS / YKS-Ek / DGS Ön Kayıt İşlemleri | Aday öğrenci, öğrenci işleri |
| Uyum Haftası | Yeni öğrenciler |
| Çift Anadal/Yandal Başvuru → Öğrenci Atama → Kayıt Onay | Öğrenci, bölüm |
| **Öğrenci Ders Seçme İşlemleri** | Öğrenci |
| **Danışman Ders Onay İşlemleri** | Danışman |
| Bölüm Başkanı Onay İşlemleri | Bölüm başkanı |
| Öğrencilerin danışmanlarla yüz yüze görüşme tarihleri | Öğrenci + danışman |
| **Ekle-Bırak** öğrenci → danışman onayı → bölüm başkanı onayı | Öğrenci, danışman, bölüm başkanı |
| Derslerin başlama ve bitiş tarihleri | Herkes |
| **Ara Sınav Not Girişleri** | Öğretim elemanı |
| **Final Sınav Tarihleri** / **Final Sınavı Not Giriş Tarihleri** | Öğretim elemanı |
| **Elle Harf Notu Atama** | Öğretim elemanı |
| **Bütünleme Sınav Tarihleri** / **Bütünleme Not Girişleri** | Öğretim elemanı |
| **3 Ders Sınav Tarihleri** (mezuniyet aşamasındakiler için ek sınav) | Öğrenci, öğretim elemanı |
| Resmi tatiller (29 Ekim, Yılbaşı …) | Herkes |

### 2.6 Danışman Bilgileri

Alanlar: Ad, Soyad, Unvan, Ofis Adresi, E-posta, **Görüşme Saatleri** (Gün, Başlangıç, Bitiş). Görüşme saati çoğunlukla tanımlanmamış; randevu sistemi yok.

### 2.7 E-imzalı Transkript / Öğrenci Belgesi Talebi

- Form: **Rapor Dili** (TR/EN) · **Belge Türü** (Öğrenci Belgesi, Transkript) · **Belge içeriğini etkileyen sebep**: Resmi, Özel, Yabancı Ülke (İngilizce), Resmi + Disiplin Cezası Bilgisi, Resmi + Muhtemel Mezuniyet Bilgisi, Resmi + Yatay Geçiş Engeli Yoktur + Disiplin.
- "Belgeyi E-imza Sistemine Gönder": belge e-imza kuyruğuna girer. Hazırlanması **1–3 iş günü** sürer.
- Hazır belgeler **doğrulama kodu** ile aynı ekrandan indirilir. "İmzada Bekleyen Belgeler" listesi var.

### 2.8 Sanal Transkript (ne olur simülatörü)

- Üst bilgi: Öğrenci No, Ad Soyad, Program (Lisans), Bölüm, Genel Not Ortalaması.
- Dönem dönem ders listesi: **Ders Kodu · Ders Adı · AKTS · Başarı Notu**. Not hücresi **açılır liste**: öğrenci farklı harf notları seçip "Hesapla" ile YANO/GANO'yu simüle edebiliyor.
- Dönem altı özet: Alınan Kredi, Toplam Alınan Kredi, **YANO** (yarıyıl ortalaması), **GANO** (genel ortalama).
- Harf notu seçenekleri: `A, B1, B2, B3, C1, C2, C3, F1, F2`. Kredisiz dersler için `BŞR / BŞZ`. Muaf ya da transfer dersler sabit nottur ve değiştirilemez.
- "Resmiyeti bulunmamaktadır" uyarısı.

**Harf notu ölçeği** (Ankara Üniversitesi Ön Lisans ve Lisans Yönetmeliği; yönetmelik metninden doğrulanacak):

| Harf | Katsayı | Puan aralığı | Not |
| --- | --- | --- | --- |
| A | 4,00 | 90–100 | |
| B1 | 3,50 | 80–89 | |
| B2 | 3,25 | 75–79 | |
| B3 | 3,00 | 70–74 | |
| C1 | 2,75 | 65–69 | |
| C2 | 2,50 | 60–64 | |
| C3 | 2,25 | 50–59 | |
| F1 | 0,00 | — | Devamsızlık nedeniyle başarısız |
| F2 | 0,00 | 0–49 | Başarısız |
| BŞR / BŞZ | — | — | Kredisiz derslerde başarılı/başarısız, ortalamaya girmez |

### 2.9 YÖKSİS E-Devlet Bilgi Güncelle

- "YÖKSİS E-Devlete Gönder" butonu. **Günde 1 kez** yapılabilir.
- **Geçmiş İstekler**: İstek Tarihi + Durum metni (başarılı / hata). Sayfalı tablo (10/25/50/100).
- Hata mesajı tek tip ve detaysız ("öğrenci işleri ile görüşünüz").

### 2.10 Ders Planı (Müfredat)

- Yarıyıl bazında (1–8) tablo: **Ders Tipi · Kod · Ad · Dil · T (teori saat) · U (uygulama saat) · K (ulusal kredi) · A (AKTS)** ve yarıyıl **TOPLAM**ı.
- **Seçmeli grup** kavramı: Müfredatta tek bir satır gibi duran grup kodu (ör. `COMTE02` "Technical Electives 2nd Year", `COMTE03`, `COMTE04`) altında seçilebilecek ders havuzu listeleniyor.
- **Üniversite Alan Dışı Seçmeli** havuzu (`UNVGOFECG`): `AAUE`, `EDUE`, `SEUE`, `SHUE`, `SSUE`, `YBUS` önekli, fakülteler arası yüzlerce ders.
- **Pedagojik Formasyon** grubu (`PFESECGnYY`): isteğe bağlı paralel program dersleri.
- Aynı ders adı farklı kodlarla farklı AKTS'lerde tekrar edebiliyor (eski/yeni müfredat versiyonu, ör. `COM4519` 6 AKTS ↔ `COM4569` 4 AKTS). Bu, **müfredat versiyonlama** ve **ders eşdeğerliği** ihtiyacını gösteriyor.
- Ders dili: İngilizce / Türkçe.

### 2.11 Ders Programı (haftalık)

- Takvim ızgarası (Pzt–Paz, 08:00–23:00). Her blokta: **saat aralığı, Ders Kodu-Adı, Derslik, Öğretim Elemanı, Oturum Tipi (Teori/Uygulama)**.
- Derslik değerleri: "Derslik 3", "Öğretim Üyesi Odası". Bitirme projesi gibi dersler öğretim üyesi odasında.
- "Rapor" ile PDF çıktı.

### 2.12 Sınav Sonuçları

- Filtre: Dönem (1950'den bugüne, Güz/Bahar/**Yaz Okulu** — gereksiz uzun liste).
- Tablo: **Ders · Akademik Personel · Anket · Harf Notu İlan Tarihi · Sınıfın Ortalaması · Öğrenci Ortalaması · Harf Notu · Durum (Devamlı/Devamsız) · Aksiyon**.
- Satır açılınca **değerlendirme bileşenleri**: Çalışma Türü · Not · Yüzde Etkisi · İlan Tarihi.
- Gözlemlenen bileşen tipleri:
  - `Yarıyıl İçi (Ara Sınav)` → Vize, Ödev, Proje, Project Homework, Laboratuvar, Lab Exam, Lab Attendance …
  - `Yarıyıl Sonu (Final)` → Final (genelde %50–%60)
  - `Bütünleme` → finalin yerini **aynı ağırlıkla** alır
- Ağırlıklar şube bazında öğretim elemanınca belirleniyor ve toplamı %100.
- **"Anket" sütunu**: ders değerlendirme anketi doldurulmadan notun görülemediği bilinen bir kural (🔎).

### 2.13 Ders Bazlı Devam(sızlık) Raporu

- Dönem filtresi → ders listesi (Kod, Ad, Akademik Personel) → ders detayında yoklama kayıtları. Bu dönem için kayıt yok. Yoklama dijital olarak sistematik tutulmuyor.

### 2.14 Azami Süre Durumu

- Alanlar: Birim, Bölüm, Sınıf, Yarıyıl, Dönem, **Azami Süre Dönemi**.
- Uzaktan yapılan bazı sınavların (ATA, TDİ, YDİ, ENG …) saatleri için e-Kampüs'e yönlendiren duyuru.

### 2.15 Çift Anadal / Yandal Yerleştirme Onay

- Aktif döneme ait ÇAP/Yandal başvurusu varsa yerleştirme sonucunu öğrenci onaylar. Akış: Başvuru → Atama → Öğrenci Onayı → Kayıt Onayı.

### 2.16 Ders Seçme İşlemi (kayıt yenileme) — **en kritik iş akışı**

Ekran bileşenleri:

- Öğrenci Bilgileri paneli, **Ders Listesi** (müfredata göre alınabilecek dersler; renk kodu: **Başarısız / Başarılı / Hiç alınmamış**).
- **Takvim Görünümü**: seçilen şubelerin haftalık programı (çakışma görselleştirme).
- **Kayıt Onay Raporu**, **Sıkça Sorulan Sorular**, "Kredi Güncelle".
- Durum: "2026-2027 Güz Dönemi ders seçme durumunuz: **Danışman Onaylı**".
- Takvim dışında giriş: "Akademik takvime göre kayıt yenileme aralığında olmadığınız için ders seçemezsiniz."

**Ders seçme kuralları** (ekranda yayınlanan yönetmelik özeti):

1. Hazırlık öğrencileri, **kayıt dondurmuş** öğrenciler ve ders seçme tarihlerinde **uzaklaştırma cezası** olan öğrenciler ders seçemez.
2. Bir yarıyıl ders yükü **30 AKTS**, yıllık **60 AKTS**.
3. 1. ve 2. yarıyıl öğrencileri bulundukları yarıyılın **tüm derslerini** almak zorunda (güzde hazırlığı bitirenler hariç).
4. 3. yarıyıldan itibaren **GANO'ya göre** ek yük (danışman onayıyla):
   - GANO ≤ 1,99 → en fazla **30 AKTS**
   - 2,00 ≤ GANO ≤ 2,99 → en fazla **40 AKTS**
   - GANO ≥ 3,00 → en fazla **45 AKTS**
5. Öğrenci önce alt yarıyıllardan **başarısız olduğu ya da hiç almadığı** dersleri almak zorunda. 30/60 AKTS'yi dolduran derslerin hesap sırası: alttan kalan → alttan alınmamış → bulunulan yarıyıldan kalan → bulunulan yarıyıldan alınmamış → üstten kalan → üstten alınmamış.
6. **Azami süre**: 2 yıllık önlisans → 4 yıl, 4 yıllık lisans → 7 yıl, 5 yıllık → 8 yıl, 6 yıllık (tıp) → 9 yıl. Süre aşılınca katkı payı/öğrenim ücreti ödenerek devam edilir. **Azami süre sonu derse devam hakkı** verilenler Güz+Bahar toplamında en fazla **5 ders** seçebilir.
7. Programdan çıkarılan zorunlu dersin yerine **eşdeğer ders** alınır. Eşdeğer yoksa AKTS açığı seçmelilerle kapatılır.
8. Başarısız seçmeli dersin yerine **başka seçmeli** alınabilir.
9. **Geçmiş dersi tekrar alma** (not yükseltme) mümkün. **Son alınan not geçerlidir.**
10. Geçici madde: 2016-2017 ve öncesi müfredatlı, mezuniyet aşamasındaki öğrenciler için **39 saat/yarıyıl, 78 saat/yıl** kuralı (koşullu: 3. sınıftan en az 1 zorunlu ders + 120 kazanılan AKTS vb.).
11. **Üniversite Alan Dışı Seçmeli**: 2023-2024 ve sonrası girişliler için zorunlu, öncesi için değil. Yani kural **giriş yılına/müfredat versiyonuna** bağlı.
12. AKTS açığı, birim izniyle **edX / Coursera** kurslarıyla kapatılabilir (dış kredi tanıma).

**SSS'den çıkan iş akışı kuralları:**

- Ders listede yoksa ya bölüm dersi **açmamıştır** ya da ders **müfredata işlenmemiştir**.
- **Kontenjanları bölüm belirler**. Kontenjan **bölüm/program bazında** ayrılabiliyor ("öğrencinin bölümüne kontenjan verilmemiş").
- Ders seçimi **danışman onayına gönderilince kilitlenir**. Değişiklik için danışmanın **reddetmesi** gerekir. Onaylanmış seçim de danışman tarafından reddedilip açılabilir.
- **Onaylanmamış / gönderilmemiş / reddedilmiş seçimler öğrencinin üzerine yansımaz.**
- **Danışman öğrenci adına ders ekleyip silemez**, sadece onaylar veya reddeder.
- **Katkı payı borcu** varsa danışman onayına gönderilemez (başka üniversitede kayıt veya süre aşımı borç doğurabilir).
- ÇAP/Yandal öğrencisi, sağ üstteki **Aktif Bölüm** seçiciyle diğer programı için ayrı ders seçer.
- Ders açma işlemleri ders seçme penceresinde yapılamaz, **ders seçme ile ekle-bırak arasında** yapılır.
- Danışman ekranı: "Öğrenci Seç" ile danışmanlığındaki öğrencileri listeler, onaylar veya reddeder (🔎).
- Öğrenci işleri: "Öğrenci Ders Yönetimi" ile **müfredat eşleştirmesi** ve **ders bağlantısı** (tekrar alınan dersin eskisine bağlanması) düzeltilir (🔎).

### 2.17 Diğer

- **Hazırlık Kayıt**: hazırlık sınıfı interaktif kayıt dönemi (ayrı takvim penceresi).
- **Aktif Bölüm** açılır menüsü: Bölüm, **Kayıt Tipi** (ÖSYS / DGS / YÖS / Yatay Geçiş …), **Kayıt Tarihi**, **Durum** (Aktif …).
- **Kampüs Kart**: harici sistem. Öğrenci kimlik kartı talebi.
- **Kütüphane**: kayıt ve durum sorgulama, harici entegrasyon.
- **Aydınlatma Metni**: KVKK (6698) metni, veri kategorileri, işleme amaçları, aktarım.

---

## 3. e-Kampüs (Moodle) — Ekran Envanteri

### 3.1 Kontrol paneli

- "Hoş geldin" kartı: bugünün tarihi, son giriş, **son erişilen ders**, "Öğrenmeye devam et".
- Ders sayaçları: Tüm kurslar · Geliştiriliyor (devam eden) · Past (geçmiş — 0, yani geçmiş dönem dersleri kaldırılıyor ya da arşivleniyor).
- **Zaman çizelgesi**: yaklaşan ve eylem gerektiren etkinlikler (önümüzdeki 7 gün / 30 gün …, tarihe veya derse göre sıralama, arama).
- **Takvim** (ay görünümü, ders filtresi, "Yeni olay", iCal içe/dışa aktarım).
- Düzenleme modu (öğretim elemanı için).

### 3.2 Derslerim

- Filtre: Tümü / Devam eden / Gelecek / Geçmiş / Yıldızlı / Gizli. Sıralama: ada göre, son erişime göre. Görünüm: Kart / Liste / Özet.
- **Ders adlandırması**: `[760535](COM4573) ARTIFICIAL NEURAL NETWORKS [B]` → `[OBS ders açma ID'si](Ders Kodu) Ders Adı [Şube]`. Yani **her LMS dersi bir OBS şubesine 1:1 karşılık geliyor**.
- OBS'deki modal: "Danışmanınız tarafından onaylanan dersler e-Kampüs'e **belirli aralıklarla** aktarılmaktadır". Senkronizasyon gerçek zamanlı değil, batch.

### 3.3 Ders sayfası

- **Bölümler** (konu/hafta: "Genel", "Yeni Bölüm" …) ve **alt bölümler**.
- Sağ panel: **İlerleme %** (etkinlik tamamlama), hızlı linkler: Notlar · Forumlar · Kaynaklar · Ödevler, "Course content", **Ders bilgisi** (ders özeti + eğitmen kartları: unvan, ad, e-posta, "Mesaj gönder").
- Her derste varsayılan **Announcements (Duyurular) forumu**.
- Gözlemlenen etkinlik türleri: `forum`, `resource` (dosya: PDF/RAR, boyut, yüklenme tarihi), `folder` (satır içi klasör), `url` (dış link), `assign` (ödev), `subsection`.
- Moodle'da bulunan ama bu derslerde görülmeyenler: `quiz` (sınav — uzaktan sınavlar için kullanıldığı duyurudan anlaşılıyor), `page`, `label`, `bigbluebutton/zoom`, `attendance`, `feedback/survey`, `lesson`, `h5p` vb.

### 3.4 Ödev (assign)

- **Açıldı / Son tarih** (ör. açılış ile son teslim arası ~3 ay), açıklama (zengin metin, şablon dokümanlar), **Tamamlama Gereklilikleri**.
- **Gönderim durumu** tablosu: Deneme numarası ("1 / 2 deneme izinli"), Gönderim durumu, **Puan durumu**, **Kalan süre**, Son düzenleme, **Gönderim yorumları**.
- Önceki/Sonraki etkinlik gezinmesi.

### 3.5 Forum

- Forum açıklaması (öğretim elemanları **canlı ders linklerini** — Google Meet — buraya yazıyor; saat dilimi Europe/Istanbul).
- Forumda ara · **Yeni tartışma konusu ekle** · **Foruma abone ol**.
- **Canlı ders entegrasyonu yok**, dış link paylaşılıyor.

### 3.6 Notlar (Gradebook)

- **Genel bakış**: Aldığım kurslar → ders başına not.
- **Kullanıcı raporu**: Not öğesi (tür: Ödev …) · **Hesaplanan ağırlık** · Not · **Aralık** (0–100) · Yüzde · **Geri bildirim** · **Kurs toplamına katkısı** · Kurs toplamı (İcmal).
- ⚠️ **Bu not defteri OBS'deki "Sınav Sonuçları" bileşenlerinden tamamen bağımsız.** Öğretim elemanı aynı notu iki sisteme ayrı ayrı girmek zorunda (🔎).

### 3.7 Bildirimler

- Bildirim listesi + detay paneli. Türler: "yeni içerik", "içerik değişikliği", forum gönderisi, ders bilgi formu vb.
- **Bildirim tercihleri matrisi** (olay türü × kanal: **Web**, **E-posta**):
  - Ödev: ödev bildirimleri, teslim tarihi yaklaşan, gecikme, 7 gün içinde teslim edilecekler
  - Geri bildirim: bildirim + hatırlatıcı
  - Forum: abone olunan forum mesajları / özetleri
  - Ders: yazılı değerlendirme bildirimi
  - Sınav: sınav yakında açılıyor
  - Sistem: kurs tamamlandı, kurs içeriği değişiklikleri, rozet, öğrenme planı/yetkinlik yorumları, **not bildirimleri**, yeni kurs kaydı hoş geldin mesajı, kayıt süresi bitimi, veri gizliliği talepleri
- Toplu "Bildirimleri devre dışı bırak".

### 3.8 Mesajlar

- Kategoriler: **Yıldızlı · Grup · Özel**. Kişiler, arama, mesaj gizlilik ayarları.
- Yerelleştirme hatası: "`$a toplam görüşme`".

### 3.9 Profil ve Tercihler

- Profil: e-posta (görünürlük notu), şehir, zaman dilimi, ders profilleri, forum mesajları/tartışmaları, blog, **öğrenme planları**, **tarayıcı oturumları**, not genel bakış, **giriş etkinliği** (ilk/son erişim), **mobil uygulama QR girişi**, gizlilik ve politikalar, **veri saklama özeti**.
- Tercihler: dil, forum, editör, takvim, içerik bankası, ileti, bildirim, depolar, blog, **rozetler**.
- Kullanıcı raporunda yerelleştirme hatası: "`Son ({$a->last})`".

---

## 4. Tespit Edilen Sorunlar ve Agora Fırsatları

| # | Sorun (bugün) | Agora'daki karşılığı |
| --- | --- | --- |
| 1 | İki ayrı sistem, iki ayrı arayüz, iki oturum; e-Kampüs'e geçiş modal + SSO ile | **Tek uygulama, tek oturum**. OBS ve LMS aynı alan modelinin parçası |
| 2 | Onaylı dersler LMS'e **batch** ile aktarılıyor (gecikmeli) | Kayıt onaylandığı anda **olay tabanlı** olarak ders alanına üyelik |
| 3 | **İki ayrı not sistemi** (e-Kampüs not defteri ↔ OBS değerlendirme bileşenleri) | LMS not öğeleri doğrudan **değerlendirme bileşenine bağlı**. Tek kaynak |
| 4 | Eski, responsive olmayan arayüz. 1950'den başlayan dönem listesi | Modern, mobil uyumlu, bağlama duyarlı UI (aktif dönem varsayılan) |
| 5 | Bozuk sayfalar: `/home/help` 500 hatası, boş Yardım Masası, kütüphane "Kullanıcı bulunamadı", yerelleştirme hataları | Gözlemlenebilirlik (Prometheus/Grafana/log), e2e testler, i18n anahtar kontrolü |
| 6 | Bildirim ve mesajlar iki sistemde dağınık | **Tek bildirim merkezi** (web + e-posta + mobil push), tercih matrisi |
| 7 | Akademik takvim sadece tablo | Takvim **kural motoru** (zaman pencereleri) + proaktif hatırlatmalar ("ders seçme yarın başlıyor") |
| 8 | Ders seçimi kuralları metin olarak yazılı, hata sonradan çıkıyor | Kurallar **çalışan kod**: anlık doğrulama (AKTS limiti, ön koşul, çakışma, kontenjan) ve anlaşılır hata mesajları |
| 9 | Danışmanın elinde erken uyarı yok | **Risk paneli** (düşük GANO, devamsızlık, teslim edilmemiş ödev) |
| 10 | Yoklama dijital ve sistematik değil | Şube oturumu bazlı yoklama (manuel / QR), F1 kuralına bağlı uyarılar |
| 11 | Belge talebi 1–3 iş günü sürüyor | Otomatik PDF üretimi + imza + **doğrulama kodu / QR doğrulama sayfası** |
| 12 | YÖKSİS hataları detaysız | Entegrasyon log'u, hata kodları, yeniden deneme (outbox) |
| 13 | Canlı ders linkleri foruma metin olarak yazılıyor | **Canlı oturum** varlığı (sağlayıcı linki + şube programından otomatik takvim) |
| 14 | Danışman görüşme saati yok, randevu yok | Görüşme saatleri + **randevu** (stretch) |
| 15 | Anket notu görmeye engel, ama anonimlik belirsiz | Anonimliği **veri modelinde** garanti eden anket tasarımı (yanıt ↔ kimlik ayrık) |
| 16 | Mobil uygulamalar dağınık | Tek Expo uygulaması (web ile aynı API) |
| 17 | Analitik yok (öğrenciye "sanal transkript" dışında) | **OLAP katmanı**: başarı dağılımları, kontenjan doluluğu, kayıt hunisi, LMS etkileşimi ↔ başarı |

---

## 5. Agora için Gerçekçi Seed Veri Kaynakları

- **Ölçek**: ~92 bin öğrenci, ~10 bin akademik, ~700 idari personel (OBS üst bandı).
- **Müfredat**: Bilgisayar Mühendisliği (İngilizce) ders planı. 8 yarıyıl, zorunlu dersler, `COMTE02/03/04` teknik seçmeli havuzları, üniversite alan dışı seçmeli havuzu, pedagojik formasyon grubu. Halka açık Bologna verisi olduğu için seed'de referans alınabilir.
- **Akademik takvim**: 2026-2027 Güz aktivite listesi (Bölüm 2.5) doğrudan seed'e dönüşebilir.
- **Değerlendirme şablonları**: Vize %25-40, Ödev/Proje/Lab %5-20, Final %50-60, Bütünleme = Final ağırlığı.
- **Harf notu ölçeği**: Bölüm 2.8.
