# 07 · Yol Haritası ve Geliştirme Sırası

> **Durum:** v0.4 · Kararlar §5'te · İlerleme ve "Bitti" kontrol listesi §6'da.

## 1. Kritik Karar: Önce Backend mi, Frontend mi?

**Kısıtlar**

- Backend kodunu **sen** yazıyorsun (Go öğrenme projesi). Bu yolun **kritik yolu** (critical path) ve en yavaş ilerleyecek kısım burası. Bu bir sorun değil, projenin amacı zaten bu.
  - *Güncelleme (2026-10-08):* Go temelleri Ders 1–17'de atıldı. Bundan sonra kodun tamamını Claude yazıyor, Ahmet her dilimde inceliyor (bkz. §4 ve §5 #10).
- Web ve mobil frontend'i ajan olarak **ben** hızlı üretebilirim.
- Mobil en sona kalacak (karar verildi).

### Seçenekler

| | A) Önce backend (tamamen) | B) Önce frontend (tamamen) | **C) Sözleşme önce, dikey dilimler** |
| --- | --- | --- | --- |
| Nasıl | Tüm API biter, sonra UI | Tüm UI mock'larla biter, sonra API | Her modül için: birlikte veri modeli + OpenAPI sözleşmesi → ben UI'ı mock'la, sen API'yi gerçekle yazarsın → birleştir |
| Görünür ilerleme | Aylarca yok | Çok hızlı | Her dilimde |
| Yeniden iş riski | Düşük (UI'ın ihtiyacı geç fark edilir) | **Yüksek**: hayali API'ye göre UI, sonra uyumsuzluk | Düşük: sözleşme tek kaynak |
| Senin Go temposuna etkisi | Baskı yok ama motivasyon düşebilir | Backend'i UI'a uydurma baskısı | UI bir dilim önde gider, backend kendi temposunda yetişir |
| Tasarım kalitesi | API "veri odaklı" kalır, UX geç düşünülür | UX iyi, API tutarsız olabilir | UX ve API **birlikte** tasarlanır |
| Entegrasyon | Big-bang | Big-bang | Dilim dilim, küçük |

### Öneri: **C — Sözleşme önce, dikey dilimler**

```mermaid
flowchart LR
  A["1 · Birlikte tasarım<br/>(veri modeli + kullanıcı akışı<br/>+ OpenAPI sözleşmesi)"] --> B["2a · Ben: Web UI<br/>(MSW mock'ları sözleşmeden)"]
  A --> C["2b · Sen: Go API<br/>(migration, repo, servis, handler, test)<br/>— ben chat'te rehberlik + code review"]
  B --> D["3 · Entegrasyon<br/>mock → gerçek API, e2e test"]
  C --> D
  D --> E["4 · Commit + kısa retro<br/>sonraki dilim"]
```

- **Faz 0**'da iki taraf paralel temel atar. Sen: backend iskeleti. Ben: web tasarım sistemi ve uygulama kabuğu.
- Sonra her dilimde UI genellikle **bir dilim önde** olur. Sen bir modülün backend'ini yazarken ben bir sonrakinin UI'ını mock'la hazırlarım. Senin hızın kimseyi bekletmez, kimse de seni sıkıştırmaz.
- **OpenAPI sözleşmesi** (`contracts/openapi.yaml`) Go kodu değil, ortak tasarım belgesidir. Her dilimde ben taslağını çıkarırım, sen incelersin ve birlikte kesinleştiririz. İstersen sözleşmeyi de tamamen sen yazabilirsin, bu bir tercih.
- Her dilimin **"Bitti" tanımı (DoD)**: migration + repository + servis + handler + birim ve entegrasyon testleri + OpenAPI ile uyum + web ekranları + e2e senaryosu + metrikler + doküman güncellemesi + commit.

## 2. Fazlar

> Süreler kaba tahmindir. Bitirme projesinin iki döneme yayıldığı varsayılmıştır (Güz: Graduation Project I, Bahar: II). Kesin teslim tarihlerini öğrendiğimizde takvimi sabitleyeceğiz.

### Faz 0 — Temel (≈ 2 hafta)
| Sen (backend) | Ben (web + ortak) |
| --- | --- |
| `go mod init`, klasör yapısı, `cmd/api` ile `/healthz` | Vite + React + TS + Tailwind + shadcn kurulumu |
| Config struct'ı, `slog`, graceful shutdown | Tasarım sistemi: renkler, tipografi, açık/koyu tema, temel bileşenler |
| `docker compose`: PostgreSQL, Redis, MinIO, Mailpit | Uygulama kabuğu: layout, sidebar, üst bar, program bağlamı seçici, i18n |
| İlk migration + `pgxpool` + `/readyz` | Redux store, RTK Query iskeleti, MSW kurulumu |
| `/metrics` + Prometheus + Grafana (compose'a eklenir) | ESLint/Prettier/Vitest, CI (GitHub Actions) |

**Go konuları:** modüller, paketler, `net/http`, `http.ServeMux` desenleri, middleware, `context`, `log/slog`, `os/signal`, pgx havuzu, migration'lar, `testing` temelleri.

### Faz 1 — IAM + Organizasyon + Seed (≈ 3–4 hafta)
- Giriş, refresh rotasyonu, çıkış, şifre sıfırlama, oturumlar, rate limit, rol/yetki çözümleme, politika iskeleti, denetim izi.
- Organizasyon (fakülte → program, derslik), kişiler, danışman ataması.
- **Seed üretici** (`cmd/seed`): ~92 bin öğrenci, ~10 bin akademisyen, fakülteler, Bilgisayar Mühendisliği müfredatı (gerçek), sentetik diğer programlar.
- Web: giriş, şifre sıfırlama, oturumlarım, profil, rol bazlı menü, kontrol paneli iskeleti.

**Go konuları:** `crypto/ed25519`, `crypto/rand`, `crypto/subtle`, `crypto/hmac`, argon2, **JWT'yi elle yazma**, `encoding/json`/`base64`, Redis, Lua ile rate limit, tablo güdümlü testler, testcontainers.

### Faz 2 — Takvim + Müfredat + Ders Açma (≈ 3 hafta)
- Akademik yıl/dönem, takvim olayları ve **pencere motoru**, müfredat versiyonları, seçmeli gruplar, ön koşullar, not ölçeği, ders açma, şube, kontenjan, haftalık program ve çakışma kontrolü, değerlendirme planı.
- Web: akademik takvim, ders planı (müfredat), ders kataloğu, haftalık program, bölüm başkanı için ders açma ekranları.

**Go konuları:** repository deseni, transaction yardımcısı, cursor sayfalama (generics ile), PostgreSQL range tipleri ve `EXCLUDE` kısıtları, doğrulama.

### Faz 3 — Ders Seçme ★ (≈ 4 hafta, projenin kalbi)
- Program kaydı, alınabilir dersler, sepet, **kural motoru** (AKTS/GANO, 1. sınıf zorunluluğu, alttan ders önceliği, ön koşul, çakışma, kontenjan, engel, azami süre), danışmana gönder/onay/red, outbox → (ileride LMS üyeliği), kayıt onay raporu.
- Danışman ekranı: öğrenci listesi, toplu inceleme, onay/red.
- **İlk k6 senaryosu: `registration-rush`** → kontenjan aşımı = 0 kanıtı.

**Go konuları:** goroutine/channel, `sync`, `errgroup`, **race detector**, advisory lock, koşullu atomik UPDATE, iyimser kilit, idempotentlik, outbox deseni, `pprof`.

### Faz 4 — Değerlendirme ve Not (≈ 3 hafta)
- Bileşen notu girişi (pencere + eğitmen kontrolü), harf notu hesaplama, ilan, sınıf istatistikleri, bütünleme, YANO/GANO, transkript, sanal transkript ("ne olur" simülatörü), not düzeltme talebi.

**Go konuları:** ondalık aritmetik (float yok: tamsayı yüzdelik ya da `pgtype.Numeric`), kural motoru tasarımı, özellik tabanlı (property-based) test fikri, `text/template` ile rapor.

> **Güz dönemi sonu hedefi (Graduation Project I):** Faz 0–4 → giriş yapan öğrenci ders seçer, danışman onaylar, eğitmen not girer, öğrenci transkriptini görür. Çalışan bir dikey MVP + ilk yük testi sonuçları.

### Faz 5 — LMS Çekirdeği (≈ 4 hafta)
- Ders alanı (kayıt onayında otomatik üyelik), bölümler, dosya/klasör/link/sayfa, ödev + teslim + puanlama + geri bildirim, **not öğesi ↔ değerlendirme bileşeni köprüsü**, forum, zaman çizelgesi, ilerleme.

**Go konuları:** multipart upload'u stream etme, `io.Reader`/`Writer` kompozisyonu, S3/MinIO, imzalı URL, HTML temizleme, dosya tipi tespiti.

### Faz 6 — İletişim (≈ 2–3 hafta)
- Duyurular (hedef kitle), bildirim merkezi + tercih matrisi, **SSE** ile canlı bildirim, e-posta (outbox + worker), mesajlaşma.

**Go konuları:** SSE (`http.Flusher`), Redis pub/sub, worker pool, `net/smtp`, `html/template`, üstel geri çekilme.

### Faz 7 — Yoklama, Sınav (quiz), Anket, Belgeler (≈ 4 hafta)
- Yoklama oturumları ve girişi, quiz ve soru bankası, anonim ders değerlendirme anketi + anket kapısı, belge üretimi (PDF) + imza + doğrulama kodu/QR + halka açık doğrulama sayfası.

**Go konuları:** zaman ve takvim hesapları, JSONB ile esnek modeller, PDF üretimi, imzalama, QR üretimi.

### Faz 8 — Analitik / OLAP (≈ 3 hafta)
- `agora_dw`, boyutlar ve olgular, ETL worker'ı, sentetik geçmiş dönem verisi, Grafana iş panelleri, uygulama içi analitik, risk skoru ve danışman risk paneli.

**Go konuları:** batch işleme, `COPY` protokolü, zamanlanmış işler, idempotent upsert, iki veritabanıyla çalışma.

### Faz 9 — Sertleştirme ve Performans (≈ 3 hafta)
- Tüm k6 senaryoları, darboğaz analizi (`pg_stat_statements`, pprof), önbellekleme, index iyileştirme, güvenlik taraması (ZAP, gosec, govulncheck, Trivy), ASVS kontrol listesi, (opsiyonel) RLS, (opsiyonel) OpenTelemetry tracing, SLO panosu.

**Go konuları:** benchmark'lar, kaçış analizi (escape analysis), `sync.Pool`, profil okuma, bellek ve GC ayarı.

### Faz 10 — Mobil (≈ 4 hafta)
- Expo uygulaması: giriş (secure store), kontrol paneli, ders programı, notlar/transkript, LMS içerik ve ödev teslimi, bildirimler (push), QR yoklama (P2).

### Faz 11 — Dağıtım, Dokümantasyon, Teslim (≈ 2 hafta)
- VPS + Docker Compose + Caddy, demo verisi, kullanım kılavuzu, mimari doküman güncellemesi, bitirme raporu, sunum.

## 3. İlk Somut Adımlar (karar sonrası)

1. Sen kararları verirsin (§5).
2. Ben: `docs/` dokümanlarını kararlarına göre güncellerim. Kökte `docker-compose.yml` iskeleti ve `contracts/openapi.yaml` iskeleti hazırlarım. Backend klasörüne **dokunmam**.
3. Sen: Faz 0 backend'ine başlarsın. İlk dersimiz Go modül yapısı, `net/http` ve Spring'deki karşılıkları. Kodu ben chat'te adım adım açıklarım, sen yazarsın.
4. Ben paralelde: web iskeleti + tasarım sistemi + uygulama kabuğu.

## 4. Çalışma Kuralları (özet)

- Her küçük başarıdan sonra Türkçe, önek kullanmayan commit, yazar CAPELLAX02. Her commit'ten önce `make check` (ve web'de karşılığı) yeşil olmalı.
- Ders 17'ye kadar `backend/` kodunu Ahmet yazdı. Ders 17'den sonra bütün kodu Claude yazar. Ahmet her dilim sonunda inceler, "devam" demeden sonraki dilime geçilmez.
- **Bir faz tamamen bitmeden sonrakine geçilmez:** önce o fazın backend'i eksiksiz (sözleşme, migration, servis, uç, test), sonra web ekranları, sonra uçtan uca testler ve doküman. §6'daki kontrol listesinin bütün maddeleri işaretlenince faz kapanır.
- Her dilimde sözleşme koddan **önce** güncellenir. Doküman ve kontrol listesi aynı commit dizisinde güncellenir.

## 5. Kararlar (2026-10-05)

| # | Konu | Karar |
| --- | --- | --- |
| 1 | Geliştirme sırası | **C — sözleşme önce, dikey dilimler**. Başlangıçta Go temelleri ağırdan alınır, backend temeli önce atılır |
| 2 | OpenAPI sözleşmesi | Taslak Claude, inceleme ve son karar Ahmet |
| 3 | Ortak paketler (`packages/api-types`, `packages/i18n`) | Evet, pnpm workspaces ile (ihtiyaç doğduğunda) |
| 4 | Migration aracı | goose (düz SQL) |
| 5 | JWT | Stdlib Ed25519 ile elle + kapsamlı testler |
| 6 | SQL erişimi | Elle yazılmış SQL + pgx. sqlc Faz 9'da karşılaştırma olarak denenebilir |
| 7 | Kampüs yaşamı özellikleri | Opsiyonel P2 |
| 8 | Teslim takvimi | **Açık**. Tarihler öğrenilince fazlar sabitlenecek |
| 9 | Seed müfredatı | Bilgisayar Mühendisliği'nin halka açık gerçek ders planı kullanılacak |
| 10 | Kodu kim yazar (2026-10-08) | Ders 17'ye kadar backend'i Ahmet yazdı. Bundan sonra bütün kodu Claude yazar, Ahmet dilim sonlarında inceler |
| 11 | Faz kapanışı (2026-10-08) | Bir faz, §6'daki bütün maddeleri (backend + web + e2e + doküman) bitmeden kapanmaz. Backend'i bitmemiş bir fazın web'ine geçilmez |
| 12 | Geçmiş ders sonuçları (2026-10-11) | Kural motoru (GANO, alttan ders, ön koşul) geçmiş dönem sonuçlarına dayanır. Bu sonuçlar Faz 3'te eski sistemden aktarılmış kayıtlar olarak (`source = IMPORT`) seed'lenir; not girişi, harf notu hesabı ve resmî YANO/GANO Faz 4'te gelir |
| 13 | Ders seçmenin demosu (2026-10-11) | Gerçek takvimde 2026 güz ders seçmesi geçmişte kalır. Geliştirme seed'i, Bilgisayar Mühendisliği programı için kayıt pencerelerini seed'in çalıştığı ana göre açan **program kapsamlı** takvim olayları ekler (açıklamasında demo olduğu yazar). Üniversite takvimi değişmez; pencere motoru en dar kapsamı uyguladığı için sadece bu program etkilenir. Not girişi demosu (2026-10-14) aynı yaklaşımla **fakülte kapsamlıdır** (Mühendislik): not girişi pencereleri şubenin fakültesinde çözülür |
| 14 | Puan aritmetiği (2026-10-11) | Puan ve ağırlıklar veritabanında `numeric`, Go'da yüzde birlik tamsayı (`int64`); kayan nokta kullanılmaz. Ders başarı puanı tam sayıya yarım yukarı yuvarlanır ve ölçeğin aralığına bakılır (yönetmelik metninden doğrulanacak) |
| 15 | Not girişi ve ilanında MFA (2026-10-11) | Yetki kataloğunda `score:enter` ve `grade:finalize` MFA ister: öğretim elemanı not girmeden önce iki adımlı doğrulamayı kurar. Demo ve e2e bunu da gösterir |
| 16 | Bütünleme hakkı (2026-10-11) | Hangi harf notlarının bütünleme hakkı verdiği yönetmelik parametresidir (`makeup_eligible_letters`, varsayılan `["F2", "BŞZ"]`: harf notlu derste F2, başarılı/başarısız notlu derste BŞZ); devamsız öğrenci (F1 ya da devamsızlıktan BŞZ) bütünlemeye giremez. Bütünleme notu, puan olarak girilmişse final notunun yerine geçer; bütünlemeye girmeyenin notu değişmez. Bütünleme sonuçları bir kez ilan edilir, önceki harf not geçmişinde kalır |
| 17 | Transkript belgesi (2026-10-11) | Faz 4'te ekran ve yazdırılabilir görünüm. İmzalı PDF, doğrulama kodu ve belge talebi Faz 7'nin belge altyapısıyla |

## 6. İlerleme ve "Bitti" Kontrol Listesi

> Her commit'te ilgili madde işaretlenir. ✅ bitti · 🔄 sürüyor · ⬜ başlanmadı · ⏭️ bilinçli olarak sonraki faza taşındı (gerekçesiyle).

### Faz 0 — Temel

| Durum | Madde | Not |
| --- | --- | --- |
| ✅ | Go modülü, klasör yapısı, `/healthz`, config, `slog`, graceful shutdown | Ders 1–5 |
| ✅ | PostgreSQL + `pgxpool` + `/readyz`, goose migration'ları | Ders 6–9 |
| ✅ | `/metrics`, Prometheus, Grafana (panolar kod olarak) | Ders 11–12 |
| ✅ | Backend CI (gofmt, vet, staticcheck, govulncheck, race testleri) | |
| ✅ | Redis | Ders 16 |
| ✅ | Mailpit (geliştirme SMTP) | `http://localhost:8025` |
| ⏭️ | MinIO | Faz 5'te dosya yüklemeyle birlikte eklenecek: kullanılmayan servis compose'da durmasın |
| ✅ | Web: Vite + React + TS + Tailwind + shadcn, ESLint/Prettier/Vitest | pnpm (corepack), TypeScript strict, tip bilgili ESLint |
| ✅ | Web: tasarım sistemi, açık/koyu tema, uygulama kabuğu, i18n | Radix tabanlı bileşenler, OKLCH renk belirteçleri, TR/EN (anahtarlar tip kontrollü, iki dilin eşliği testli) |
| ✅ | Web: Redux store, OpenAPI'den RTK Query istemcisi, MSW | Access token sadece bellekte, 401'de tek uçuşlu sessiz yenileme, MSW ile bütünleşik testler |
| ✅ | Web CI | Biçim, lint, tip, test, build; üretilmiş istemcinin sözleşmeyle uyumu |

### Faz 1 — IAM + Organizasyon + Seed

**Backend**

| Durum | Madde | Not |
| --- | --- | --- |
| ✅ | Parola hash'leme (argon2id) ve parola politikası | Ders 10 |
| ✅ | JWT (Ed25519) elle | Ders 12 |
| ✅ | Giriş, refresh rotasyonu ve yeniden kullanım tespiti, çıkış | Ders 14–15 |
| ✅ | Hesap kilitleme, giriş hız sınırı, anında oturum iptali | Ders 14, 16 |
| ✅ | Rol/yetki çözümleme, politikalı router (varsayılan ret), sürümlü yetki önbelleği | Ders 17 |
| ✅ | Güvenlik başlıkları ve CORS | Sıkı CSP, nosniff, HSTS (HTTPS ortamlarında), origin listesiyle CORS |
| ✅ | Denetim izi (`audit.audit_log`) ve güvenlik olayları (`audit.security_events`), aylık partition | Giriş, çıkış, kilit ve yeniden kullanım olayları; `GET /audit/log`, `GET /audit/security-events`. Saklama süresine göre arşivleme worker'la, `INSERT`-only DB rolü Faz 9'da |
| ✅ | Parola değiştirme, `must_change_password` zorunluluğu | `POST /me/password`, `SelfService` politikası, diğer oturumlar kapanır, yanlış mevcut parola kilit sayacını artırır |
| ✅ | E-posta outbox'ı ve `cmd/worker` (SMTP, üstel geri çekilme) | `SKIP LOCKED` + kira ile çoklu worker, gönderimden sonra gizli veri silinir, `agora_outbox_*` metrikleri, günlük bakım işi |
| ✅ | Şifre sıfırlama (30 dk, tek kullanımlık token, bütün oturumlar iptal) | `forgot` her zaman 202, hesap başına 2 dk kısma, IP hız sınırı, token URL parçasında (#), `FOR UPDATE` ile tek kullanım, PENDING hesap etkinleşir |
| ✅ | Oturumlarım: listeleme, tek oturumu kapatma, diğer oturumları kapatma | Cihaz bilgisi User-Agent'tan, kapatılan oturumların access token'ları anında geçersiz, `GET /me/security-events` ile giriş geçmişi |
| ✅ | Kullanıcı yönetimi: listeleme, oluşturma (aktivasyon e-postasıyla), askıya alma | Hesap PENDING başlar, parolayı kullanıcı belirler; askıya almada oturumlar anında kapanır; kendi hesabında işlem yasak; denetim kaydı |
| ✅ | Rol kataloğu, rol atama ve sonlandırma (görevler ayrılığı, denetim izi) | Kendine rol atanamaz, kapsam doğrulaması, ileri tarihli atama, son sistem yöneticisi korunur (advisory lock + `clock_timestamp()`), değişiklik anında etkili |
| ✅ | Organizasyon: binalar ve derslikler | Kapsamlı yetki (binanın birimi), iyimser kilit (ETag/If-Match, 412/428), denetim kaydı, geliştirme seed'i |
| ✅ | Program kaydı (`enrollment.student_programs`) ve danışman ataması | Yetkili kümeden listeleme, danışman rolü ilişkiye dayalı (bölümü kapsamaz), erişim yoksa 404, danışman geçmişi. Müfredat/dönem bağlantıları Faz 2'de |
| ✅ | İş metrikleri (`agora_login_failures_total`, `agora_refresh_reuse_detected_total` …) | Girişler, başarısız girişler (sebebe göre), kilitlenmeler, token yeniden kullanımı, parola sıfırlama, hız sınırı; Grafana'da "Kimlik doğrulama ve güvenlik" ve "Worker" satırları |
| ✅ | Seed üretici: ~92 bin öğrenci, ~10 bin personel, sentetik programlar, danışmanlar | `make seed-synthetic` (~6 sn, COPY + tek transaction, belirlenimci tohum); 68 programlık katalog; arama için trigram index'leri (75 ms → 0,7 ms). Gerçek BM müfredatı Faz 2'de müfredat modeliyle |
| ✅ | MFA (TOTP) ve kurtarma kodları | RFC 6238 (resmi test vektörleriyle), sır AES-256-GCM ile şifreli ve kullanıcıya bağlı, tekrar oynatma koruması, iki adımlı giriş (5 dk, 5 deneme, ortak kilit sayacı), 10 kurtarma kodu (HMAC), hassas yetkiler sadece MFA'lı oturumda (`403 MFA_REQUIRED`), yönetici sıfırlaması |

**Web**

| Durum | Madde |
| --- | --- |
| ✅ | Giriş, oturum geri yükleme (sessiz refresh), çıkış |
| ✅ | Parola değiştirme (ilk girişte zorunlu), şifre sıfırlama ve hesap aktivasyonu |
| ✅ | İki adımlı doğrulama: girişin ikinci adımı, kurulum (QR), kurtarma kodları, MFA öneri bandı |
| ✅ | Oturumlarım, güvenlik geçmişi, profil |
| ✅ | Rol bazlı menü, kontrol paneli iskeleti |
| ✅ | Yönetim: kullanıcılar, hesap işlemleri, rol atama, MFA sıfırlama, denetim kayıtları |

**Kapanış**

| Durum | Madde |
| --- | --- |
| ✅ | Playwright e2e: giriş, ilk girişte parola değiştirme, şifre sıfırlama, MFA kurulumu ve iki adımlı giriş, rol atamasının anında etkisi, hesap aktivasyonu (gerçek yığınla, CI'da da) |
| ✅ | Sözleşme ve dokümanlar güncel, CI yeşil. Go 1.27.2'ye geçildi (net/http HTTP/2 açıkları GO-2026-6611/6612/6613/6617); staticcheck ve govulncheck `backend/tools` modülünde sabitlendi |

### Faz 2 — Takvim + Müfredat + Ders Açma

**Backend**

| Durum | Madde | Not |
| --- | --- | --- |
| ✅ | Akademik yıl ve dönemler, tek aktif dönem | `academic.academic_years`, `academic.terms`, kısmi benzersiz index ile tek aktif dönem; yıl ve dönem tanımı üniversite geneli yetki ister |
| ✅ | Takvim olayları ve pencere motoru | Tipli zaman pencereleri, kapsam (üniversite → fakülte → program) geçersiz kılma, aynı kapsamda çakışma yok (`EXCLUDE`), taslak olaylar, `GET /calendar/windows` |
| ✅ | Ders kataloğu, ön koşullar (VE/VEYA grupları), eşdeğerlikler | `curriculum.courses`; kod sabit (değişen ders yeni kodla açılıp eşdeğerlikle bağlanır), ön koşul kümesi tek seferde değişir, döngü özyinelemeli sorguyla engellenir (`PREREQUISITE_CYCLE`), eski ↔ yeni kod eşdeğerliği; bölüm kapsamlı yetki |
| ✅ | Seçmeli gruplar ve ders havuzları | Teknik seçmeli, üniversite alan dışı, pedagojik formasyon, genel sosyal; havuz üyeliği idempotent `PUT` |
| ✅ | Versiyonlu müfredat ve öğrenci program kaydına bağlantı | Taslak → yürürlükte → arşiv; yürürlükteki sürümlerin giriş yılları çakışamaz (`EXCLUDE`); yürürlüğe girerken AKTS toplamı ve yarıyıl sınırı doğrulanır, aralıktaki öğrenci kayıtları bağlanır; ders satırı ve seçmeli yuva; `GET /me/curricula` |
| ✅ | Not ölçeği ve yönetmelik parametreleri (veri olarak) | Ankara Üniversitesi lisans ölçeği (A … F2, F1 devamsızlık, BŞR/BŞZ, MUAF); puan aralıkları 0-100'ü boşluksuz kapsar (`EXCLUDE` + doğrulama), tek varsayılan ölçek; parametreler tarihli (`daterange` çakışmasız), yeni değer eskisini kapatır, JSON türü korunur |
| ✅ | Ders açma, şube, kontenjan ve program bazlı alt kontenjan, öğretim elemanı ataması | Bir ders dönemde bir kez açılır, öğrenci grupları şubelerle; durum geçişleri (planlama → açık → kapalı, iptal); kontenjan kayıtlı sayının ve program kontenjanları toplamının altına inemez (`CHECK`); tek sorumlu öğretim elemanı (kısmi benzersiz index), sadece görevdeki akademik personel; bölüm kapsamlı yetki |
| ✅ | Haftalık program ve çakışma kontrolü | `timerange` tipi; derslik ve şube içi çakışma veritabanında (`EXCLUDE`), öğretim elemanı çakışması serviste (dönem başına advisory lock ile yarışsız); teorik oturumda derslik kapasitesi; çakışma mesajı dersi ve saati söyler; bölüm, derslik ve öğretim elemanı programı, `GET /me/teaching` |
| ✅ | Değerlendirme planı | `grading` şeması; bileşenler ve ağırlıklar (dönem içi + final = 100, tek final), bütünleme finalden türetilir; bileşen kimlikleri yeniden yazmada korunur (notlar bağlanacak); düzenleme ilişkiye dayalı (şubenin öğretim elemanı) ya da bölüm; kilidi öğretim elemanı koyar, gerekçeyle bölüm açar |
| ✅ | Seed: gerçek BM (İngilizce) müfredatı, 2026-2027 takvimi, not ölçeği, BM güz dönemi ders açma ve programı | Müfredat bölümün yayımladığı formlardan (2022, 2023, 2026 sürümleri; 170 ders, 42 eşdeğerlik); takvimde fakülte kapsamlı uzatma örneği; 38 ders açma repository'ler üzerinden (çakışma ve kapasite denetimli), seed testi CI'da gerçek SQL seed'leriyle çalışır; öğrenciler giriş yılına göre müfredata bağlanır |

**Web**

| Durum | Madde |
| --- | --- |
| ✅ | Akademik takvim: dönem seçici, açık/kapalı pencereler, olay yönetimi |
| ✅ | Ders kataloğu ve ders detayı (ön koşul, eşdeğerlik, havuzlar) |
| ✅ | Ders planı (müfredat): öğrenci için kendi programı (program bağlamı seçici), personel için program ve sürüm gezgini, taslak düzenleme |
| ✅ | Haftalık program görünümü (öğretim elemanı, bölüm, derslik) |
| ✅ | Ders açma ekranları (bölüm başkanı): ders açma, şube, kontenjan, öğretim elemanı, program yerleşimi ve çakışma geri bildirimi |
| ✅ | Değerlendirme planı (öğretim elemanı) ve not ölçeği |

**Kapanış**

| Durum | Madde |
| --- | --- |
| ✅ | Playwright e2e: takvim penceresi, müfredat görüntüleme, ders açma ve çakışma senaryoları (seed'in gerçek BM verisiyle, tarihten bağımsız; derslik ve öğretim elemanı çakışma mesajları) |
| ✅ | Sözleşme ve dokümanlar güncel (veri modeli, yetkilendirme politikaları, README'de akademik veri ve demo hesap rehberi), CI yeşil (backend, web kontrolü ve Playwright) |

### Faz 3 — Ders Seçme ★

**Backend**

| Durum | Madde | Not |
| --- | --- | --- |
| ✅ | Kayıt engelleri | `enrollment.holds`: katkı payı borcu, disiplin, eksik belge, kütüphane borcu; engelin neyi kapattığı (ders seçme, belge, mezuniyet); koyma ve gerekçeyle kaldırma (`hold:manage`, mali işler üniversite, fakülte öğrenci işleri fakülte kapsamı); öğrenci kendi engellerini görür |
| ✅ | Program kaydı durumu ve süreler | Durum geçişleri (dondurma, uzaklaştırma, ilişik kesme, mezuniyet) gerekçe ve karar numarasıyla, geçmişiyle; yarıyıl, normal süre ve azami süre (programın `max_duration_years` değeri) hesabı; dönem geçişinde aktif öğrencilerin yarıyılı outbox olayıyla bir kez ilerler (dondurulan dönem sayılmaz) |
| ✅ | Ders geçmişi (kesinleşmiş kayıtlar) | `enrollment.enrollments`: deneme numarası, tekrar alınan derste son not geçerli (`superseded_by`), harf notu not ölçeğinden; eski sistemden aktarılan geçmiş (`source = IMPORT`); müfredata göre ders durumu (geçti, kaldı, devam ediyor, alınmadı) eşdeğerlikler ve seçmeli havuzlarla |
| ✅ | Alınabilir dersler | Öğrencinin müfredatı ve dönemin açılan dersleri: alttan, dönem, seçmeli yuva, üst yarıyıl ve not yükseltme kategorileri; eski koddan kalan ders yeni eşdeğeriyle alınır; şubeler, programa göre kalan kontenjan, haftalık program, ön koşul durumu |
| ✅ | Kural motoru | Saf Go paketi, tablo güdümlü testler. Engel, program kaydı durumu, AKTS üst sınırı (GANO'ya göre, yönetmelik parametresi), 1. sınıf zorunlu dersleri, alttan ders önceliği, ön koşul (VE/VEYA, geçti/devam etti), haftalık program çakışması (devam şartını önceden sağlamış tekrar derste uyarı), müfredat dışı ders, azami süre (en çok 5 ders), not yükseltme ve seçmeli yuva aşımı uyarıları. Hata ve uyarı ayrımı; ekleme anında ve gönderimde ayrı kural kümeleri |
| ✅ | Sepet ve yarışa dayanıklı kontenjan | `registrations` (dönem başına tek), `registration_items`, `registration_events`; ekle, çıkar, şube değiştir. Öğrenci başına advisory lock; kontenjan sepete eklerken koşullu atomik `UPDATE` ile düşer (program alt kontenjanı dahil), çıkarınca serbest kalır; aynı dersin iki şubesi kısmi benzersiz index'le engellenir. Eşzamanlılık testi: yüzlerce goroutine aynı şubeye, kontenjan aşımı 0 (race detector açık) |
| ✅ | Danışman onayı | Gönder, geri çek, onayla, gerekçeyle reddet, onayı geri al (durum makinesi, `If-Match`); ders seçme ve danışman onayı pencereleri; onayda kesin kayıtlar oluşur; danışman için dönem ve durum filtreli öğrenci listesi, toplu onay; öğrenci işleri pencere dışında gerekçeyle müdahale eder (`registration:override`, denetime işaretli) |
| ✅ | Outbox | `platform.outbox_events` aynı transaction'da yazılır, worker `SKIP LOCKED` ile dağıtır, tüketiciler idempotent. İlk tüketici: öğrenciye onay ve red e-postası (LMS üyeliği Faz 5'te aynı olaya bağlanacak) |
| ✅ | Süresi dolan taslaklar | Ders seçme penceresi kapanınca gönderilmemiş taslakların ayırdığı kontenjan worker işiyle serbest bırakılır, olay günlüğüne yazılır |
| ✅ | İdempotentlik, hız sınırı ve iş metrikleri | `Idempotency-Key` başlığı (`platform.idempotency_keys`); ders seçme uçları için kullanıcı başına ayrı hız sınırı; `agora_registration_items_added_total`, `agora_registration_submitted_total`, `agora_section_full_rejections_total` ve Grafana panosu |
| ✅ | Ekle-bırak | Onaylı kayıtta öğrenci talebi (ekle-bırak penceresi) → danışman → bölüm başkanı onayı → uygulama; eklenecek şubenin kontenjanı talepte ayrılır, red ya da geri çekmede serbest kalır |
| ✅ | Seed: geçmiş dönemler ve demo senaryoları | 2020-2026 akademik yılları; BM öğrencilerine giriş yılına ve müfredatlarına göre deterministik ders geçmişi (başarısız ve tekrar alınan dersler dahil), GANO ve AKTS önbellekleri; demo öğrencileri (alttan ve eşdeğer ders, 1. sınıf), P10001'in danışmanlığında onay bekleyen kayıtlarıyla bir demo grubu, bir kayıt engeli; güz kayıtları servis üzerinden (kurallar ve kontenjan denetimli); BM için program kapsamlı demo pencereleri (§5 #13) |
| ✅ | k6 `registration-rush` | Eşzamanlı giriş ve popüler şubelere ders ekleme; kontenjan aşımı = 0 kanıtı (SQL doğrulaması), gecikme ve hata oranı sonuçları dokümanda; `make loadtest-registration` |

**Web**

| Durum | Madde |
| --- | --- |
| ✅ | Ders seçme (öğrenci): alınabilir dersler ve kategoriler, şube ve kontenjan, sepet ve AKTS sayacı, anlık kural geri bildirimi, haftalık program önizlemesi ve çakışma, danışmana gönder ve geri çek, durum ve danışman notu |
| ✅ | Danışman: danışmanlığımdaki öğrencilerin kayıt durumları, kayıt inceleme (ders geçmişi, kurallar, program), onay, red, onayı geri alma, toplu onay |
| ✅ | Kayıt onay belgesi (yazdırılabilir; imzalı PDF ve doğrulama kodu Faz 7'nin belge altyapısıyla) |
| ✅ | Ekle-bırak: öğrenci talebi, danışman ve bölüm başkanı onay ekranları |
| ✅ | Kayıt engelleri yönetimi; öğrenci detayında program durumu ve ders geçmişi |
| ✅ | Ders planım: derslerin durumu (geçti, kaldı, devam ediyor) ve mezuniyet ilerlemesi |
| ✅ | Ana sayfa: öğrenci için kayıt durumu, danışman için onay bekleyenler |

**Kapanış**

| Durum | Madde |
| --- | --- |
| ✅ | Playwright e2e: ders seçme (alttan ders ve eşdeğerlik, çakışma uyarısı ve hatası, AKTS sayacı, kayıt engeli), gönder → geri gönder → düzelt → onay, toplu onay, ekle-bırak (danışman ve bölüm başkanı), onay belgesi |
| ✅ | Sözleşme ve dokümanlar güncel (veri modeli, [kural kataloğu](09-ders-secme-kurallari.md), yetkilendirme politikaları, [yük testi sonuçları](08-yuk-testleri.md), README'de ders seçme demo rehberi), CI yeşil (backend, web kontrolü ve Playwright) |

### Faz 4 — Değerlendirme ve Not

**Backend**

| Durum | Madde | Not |
| --- | --- | --- |
| ✅ | Bileşen notu girişi | `grading.assessment_scores`: puan (0-100) ya da "girmedi"; öğrencinin devam durumu (devamsız → F1). Not girişi pencereleri bileşenin türüne göre (ara sınav ve dönem içi → `MIDTERM_GRADE_ENTRY`, final → `FINAL_GRADE_ENTRY`, bütünleme → `MAKEUP_GRADE_ENTRY`), şubenin birim kapsamında. Şubenin sorumlu ve ortak öğretim elemanı girer (ilişki), plan kilitli olmalı, `score:enter` MFA ister. Bileşen bazında toplu yazma, iyimser kilit (`If-Match`), denetim izi |
| ✅ | Bileşen notlarının yayımlanması | Bileşen notları öğretim elemanı yayımlayınca öğrenciye görünür (`published_at`); bileşen ortalaması |
| ✅ | CSV ile toplu not girişi | `GET /sections/{id}/gradebook.csv`: öğrenci listesi, her bileşen için bir sütun, devam durumu ve harf notu; Türkçe Excel'e uygun (`;` ayraç, ondalık virgül, UTF-8 BOM), İngilizcesi `,` ve nokta; formül enjeksiyonuna karşı hücreler etkisizleştirilir. Bir bileşene içe aktarma: önce satır satır doğrulamalı önizleme (öğrenci şubede mi, tekrar, puan biçimi, bütünleme hakkı; dosyada olmayanlar), sonra hatasız dosya tek transaction'da (`If-Match` önizlemenin not sürümü, kaynak `CSV`, denetim izi). Hoşgörülü okuma (`encoding/csv`): ayraç sezilir, puan sütunu bileşenin adından (dışa aktarılan dosya şablondur), başlıksız iki sütun, ondalık virgül ya da nokta, girmedi için `G`, Windows-1254 (Türkçe Excel'in CSV kaydı). Boş hücre notu silmez |
| ✅ | Harf notu motoru | Saf Go paketi, tamsayı aritmetik (§5 #14). Kurallar: devamsız → F1; final ya da bütünleme barajı (`final_min_score`) altı ya da girmedi → F2; bütünleme finalin yerine geçer; ağırlıklı ortalama yuvarlanıp ölçeğin aralığına bakılır; başarılı/başarısız (BŞR/BŞZ) notlu dersler; eksik bileşen → hesaplanamaz. Tablo güdümlü ve özellik tabanlı testler (ör. puan artınca harf notu düşmez) |
| ✅ | Harf notu önizleme ve ilan | Sorumlu öğretim elemanı önizler ve ilan eder (`grade:finalize`, MFA): kesin kayıt `COMPLETED`, ağırlıklı puan ve harf notu, ilan tarihi; tekrar alınan derste önceki deneme `superseded_by` ile kapanır. Elle harf notu (`MANUAL_LETTER_GRADE` penceresi, gerekçe zorunlu, denetimde). İlandan sonra değişiklik sadece not düzeltme talebiyle. `grades.published` olayı outbox'a |
| ✅ | Bütünleme | İlan sonrası bütünleme hakkı olanlar (§5 #16), bütünleme notu girişi ve yeniden ilan; bütünlemeye girmeyenin notu değişmez |
| ✅ | Güncel yönetmelik (2023) | Ankara Üniversitesi yönetmeliğinin 2022 ve 2023 değişiklikleri tarihli veri olarak: yeni ölçek AU-LISANS-2023 (C3 60-64 2,00; F1 50-59 1,50 başarısız; F3 finale girmedi; F4 devamsızlık), geçme puanı 60, bütünleme hakkı F1-F3, final ağırlığı %60 (plan doğrulaması ve kilit), yüksek onur 3,70. Eski ölçek ve parametre değerleri eski notlar ve geçmiş dönemler için kalır; onur kararı dönemin kuralıyla. Seed geçmiş notları dönemin ölçeğiyle üretir |
| ✅ | Sınıf istatistikleri | `grading.section_grade_stats`: öğrenci sayısı, ortalama, medyan, standart sapma, başarı oranı, harf dağılımı; harf notu ve bütünleme ilanıyla aynı transaction'da güncellenir (önizleme özeti de aynı hesaptan). `GET /sections/{id}/grade-stats`: şubenin öğrencisi, öğretim elemanları ve kapsamdaki birimler görür; öğrenci kendi notunun yanında sınıf ortalamasını "Öğrencinin notları"nda görür. Aktarılan geçmiş dönem şubeleri için seed hesaplar |
| ✅ | YANO/GANO ve dönem sonuçları | `enrollment.term_results`: alınan ve kazanılan AKTS, YANO, GANO, onur/yüksek onur (`honor_gpa`, tam yük, finalde başarı, disiplin cezası yok). Saf Go hesap paketi (`grading/gpa`, tamsayı aritmetik), son not geçerli (eşdeğer ders ve kalınan seçmelinin yerine alınan ders dahil), AKTS ağırlıklı, iki haneye yuvarlama. `grades.published` tüketicisi idempotent yeniden hesaplar ve ders seçmenin kullandığı `gpa_cache`'i besler; aktarılan geçmiş dönemler için seed aynı kodla üretir. Kurallar [veri modelinde](04-veri-modeli-oltp.md) |
| ✅ | Toplu yeniden hesap ve doğrulama | `cmd/termresults`: notu olan bütün öğrencilerin dönem sonuçlarını kesin kayıtlardan yeniden hesaplar, kayıtlı sonuçlar ve GANO önbelleğiyle farkı raporlar (fark varsa çıkış kodu 1), `-apply` ile sadece farkı olanları yazar; `errgroup` ile paralel gruplar (`make term-results-verify`, `make term-results-recompute`). Worker günde bir doğrular, `agora_term_results_drift` metriği. 92 bin öğrenci ve 3,7 milyon kesin kayıtta doğrulama 8 sn, ilk hesap 12 sn ([ölçüm](08-yuk-testleri.md)) |
| ✅ | Transkript | `GET /student-programs/{id}/transcript`: dönem dönem dersler, AKTS, kredi, harf notu ve katsayı, tekrar ve yerini sonraki denemeye bırakmış satır işaretleri, dönem sonuçları (YANO, GANO, AKTS, onur) ve genel özet (GANO, kazanılan / gereken AKTS, onur dönemleri). Öğrenci kendisininkini (`transcript:read_own`), danışman danışmanlığındakileri, kapsamdaki birimler (`transcript:read_scoped`) görür; görme yetkisi olmayana 404 |
| ✅ | Sanal transkript ("ne olur") | `POST /student-programs/{id}/transcript/simulate`: varsayımsal harflerle YANO/GANO, hiçbir şey kaydedilmez; dönem sonuçlarıyla aynı hesap (`grading/gpa`). Süren derse harf verilince ilandaki gibi yerini aldığı denemeler düşer (tek kaynak: ilan ve simülasyon aynı "son not geçerli" sorgusunu kullanır). Harfler dersin not ölçeğinden (`letter_options`) |
| ✅ | Öğrencinin notları | `GET /student-programs/{id}/grades?term_id=`: dönem seçimli (varsayılan içinde bulunulan dönem); dersler, bileşenler ve ağırlıkları, yayımlanan bileşende not ve sınıfın bileşen ortalaması (not tablosuyla aynı hesap), harf notu ve ilan tarihi, devam durumu, sınıf istatistiği. Yayımlanmamış notlar görünmez, bütünleme sadece notu olana. Öğrenci kendisininkini (`grade:read_own`), danışmanı ve kapsamdaki birimler (`grade:read_scoped`) görür |
| ✅ | Not düzeltme talebi | İlandan sonra şubenin öğretim elemanı gerekçeyle talep eder (`grade_change:request`): bileşen notu (harf motorla yeniden hesaplanır, talepten önce önizlenir) ya da doğrudan harf; bir kesin kayıtta tek bekleyen talep. Bölüm başkanı ya da fakülte öğrenci işleri onaylar ya da gerekçeyle reddeder (`grade_change:approve`, MFA; talep eden onaylayamaz, veritabanı da denetler). Onay notu uygular: bileşen notu, harf, not geçmişi (CORRECTION), sınıf istatistiği, olayla dönem sonuçları; talepten sonra değişen not uygulanmaz. Talep geri çekilebilir, denetim izi |
| ✅ | İş metrikleri | `agora_scores_entered_total` (kaynağa göre: elle, CSV), `agora_grades_published_total` (ilan ve bütünlemeyle değişen), `agora_grade_changes_total` (talep, uygulama, ret, geri çekme); olaylar transaction başarıyla bitince sayılır, bilinen etiketler sıfırla başlar. Grafana'da "Değerlendirme ve not" satırı: son bir saatte girilen not, günlük ilan ve düzeltme talebi, kaynağa ve sonuca göre seriler, günlük doğrulamanın dönem sonucu farkı (`agora_term_results_drift`, 0'dan büyükse kırmızı) |
| ✅ | Seed: not girişi demosu | Mühendislik Fakültesi için seed anına göre üç hafta açık fakülte kapsamlı not girişi pencereleri (ara sınav, final, elle harf notu, bütünleme; §5 #13 gibi, açıklamasında demo yazar). 20 kişilik kurgusal 1. sınıf grubu ders seçme servisiyle sepetini kurar, kendi danışmanı (`P10008`) onaylar: P10001'in COM1007-1 şubesinde gerçek akıştan gelen kesin kayıtlar. Ara sınav notları not girişi servisiyle girilip yayımlanır (biri girmedi), laboratuvar girilir ama yayımlanmaz, final boş; seed testi bu durumu ve P10001'in danışman kuyruğunun değişmediğini doğrular |

**Web**

| Durum | Madde |
| --- | --- |
| ⬜ | Öğretim elemanı: not girişi tablosu (öğrenciler × bileşenler, klavyeyle gezinme, girmedi ve devamsız işaretleri, kaydedilmemiş değişiklik uyarısı, pencere durumu), bileşen yayımlama, CSV içe ve dışa aktarma |
| ⬜ | Harf notu önizleme (istatistikler ve dağılım grafiği), elle harf notu, ilan; bütünleme listesi ve girişi |
| ✅ | Öğrenci: notlarım (dönem seçici, ders kartlarında harf notu ve sınıf ortalaması, bileşen puan çubuğunda sınıf ortalaması işareti), transkript (yazdırılabilir, mobilde sadeleşen tablo), sanal transkript ("Ne olur?" modu: derslere harf seçimi, gerçek → sanal YANO/GANO) |
| ✅ | Danışman ve öğrenci işleri: öğrenci detayında transkript ve dönem sonuçları, yazdırılabilir görünüm |
| ✅ | Not düzeltme talepleri: öğretim elemanının talepleri ve yeni talep (dönem, şube, öğrenci, bileşen ya da harf, canlı önizleme), onay kuyruğu (onayla ve uygula, gerekçeli ret, son kararlar) |
| ⬜ | Ana sayfa: öğrenci için son ilan edilen notlar, öğretim elemanı için not girişi bekleyen şubeler |

**Kapanış**

| Durum | Madde |
| --- | --- |
| ⬜ | Playwright e2e: öğretim elemanı MFA kurup not girer ve yayımlar → harf notlarını ilan eder → öğrenci notunu, sınıf ortalamasını ve transkriptini görür; bütünleme; not düzeltme talebi ve onayı; sanal transkript |
| ⬜ | Sözleşme ve dokümanlar güncel (veri modeli, harf notu ve GANO kuralları, yetkilendirme politikaları, README'de demo rehberi), CI yeşil |

### Faz 5 ve sonrası

Her faz başlarken §2'deki kapsamdan aynı biçimde ayrıntılı bir liste çıkarılır.
