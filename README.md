# Agora - Üniversite Kampüs Platformu ve Bilgi Sistemi

Ankara Üniversitesi'nin Öğrenci Bilgi Sistemi (OBS) ve e-Kampüs sistemlerini tek bir modern, bütünleşik ve güvenli platformda birleştiren bitirme projesi.

> Analiz, mimari ve yol haritası dokümanları için: [`docs/`](docs/README.md). İlerleme durumu: [yol haritası, bölüm 6](docs/mimari/07-yol-haritasi.md).

## Monorepo Yapısı

| Dizin                                  | Açıklama                                                                  |
| -------------------------------------- | ------------------------------------------------------------------------- |
| [`backend/`](backend/)                 | Go ile yazılan REST API ve worker (PostgreSQL, Redis)                     |
| [`web/`](web/README.md)                | Vite + React + TypeScript + shadcn/ui + Tailwind CSS + Redux Toolkit      |
| [`mobile/`](mobile/)                   | React Native + Expo (file-based routing) + TypeScript + NativeWind        |
| [`contracts/`](contracts/openapi.yaml) | OpenAPI 3.1 sözleşmesi: API önce burada tasarlanır (`make contract-lint`) |
| [`infra/`](infra/)                     | Prometheus, Grafana panoları (kod olarak), PostgreSQL ve seed betikleri   |

## Hızlı Başlangıç

Gerekenler: Docker, Go (sürüm `backend/go.mod`'da), Node.js 22 (pnpm ayrıca kurulmaz, corepack ile gelir).

```bash
make up            # PostgreSQL, Redis, Mailpit, Prometheus, Grafana
make migrate-up    # şema
make seed          # organizasyon, takvim, ders planları, ders açma ve demo hesaplar (tekrar çalıştırılabilir)
make api           # API: http://localhost:8080
make worker        # e-postalar ve olaylar (parola sıfırlama, ders kaydı kararları, taslak süresi)
make web-install   # ilk seferde
make web-dev       # web: http://localhost:5173
```

`.env` dosyası gerekmez: development ortamında bütün ayarların yerel servislere uyan varsayılanları vardır (bkz. [`.env.example`](.env.example)).

## Servisler ve Adresler

| Servis       | Adres                                                     | Not                                                                 |
| ------------ | --------------------------------------------------------- | ------------------------------------------------------------------- |
| Web          | http://localhost:5173                                     | `/api` isteklerini API'ye aktarır (aynı origin, production'daki gibi) |
| API          | http://localhost:8080                                     | `/healthz`, `/readyz`, `/api/v1/...`                                |
| API metrikleri | http://localhost:9091/metrics                           | Aynı adreste `/debug/pprof/`                                        |
| Worker metrikleri | http://localhost:9092/metrics                        | E-posta ve olay outbox'ı                                            |
| Mailpit      | http://localhost:8025                                     | Gönderilen bütün e-postalar burada görünür, dışarı çıkmaz           |
| Grafana      | http://localhost:3000                                     | Giriş gerekmez (salt okuma). Yönetici: `admin` / `agora_dev_password` |
| Prometheus   | http://localhost:9090                                     |                                                                     |
| PostgreSQL   | `localhost:5432`                                          | `agora` / `agora_dev_password`, `make psql`                         |
| Redis        | `localhost:6379`                                          | `make redis`                                                        |

## Demo Hesaplar

`make seed` aşağıdaki kurgusal hesapları oluşturur. Hepsinin parolası **`agora-dev-parola`** (değiştirmek için `AGORA_SEED_PASSWORD`).

| Kullanıcı adı | Kişi              | Roller                                                                  |
| ------------- | ----------------- | ----------------------------------------------------------------------- |
| `22290001`    | Deniz Aksoy       | Öğrenci, Bilgisayar Mühendisliği (danışmanı `P10001`)                   |
| `22290002`    | Ece Kaya          | Öğrenci, Bilgisayar Mühendisliği (danışmanı `P10001`)                   |
| `26290004` … `26290017` | 8 öğrenci | Ders seçme demo grubu, danışmanı `P10001` (bkz. [Ders seçme demosu](#ders-seçme-demosu)) |
| `26290101` … `26290120` | 20 öğrenci | 1. sınıf not girişi demo grubu, danışmanı `P10008` (bkz. [Not girişi demosu](#not-girişi-demosu)) |
| `22290003`    | Can Polat         | Öğrenci, **geçici parola**: ilk girişte parolasını değiştirmek zorunda  |
| `P10001`      | Ayşe Demir        | Öğretim elemanı ve danışman (Bilgisayar Mühendisliği)                   |
| `P10002`      | Mehmet Yıldız     | Öğretim elemanı ve bölüm başkanı (Bilgisayar Mühendisliği)              |
| `P10003` … `P10007` | Elif Şahin, Burak Arslan, Selin Koç, Kerem Aydın, Zehra Kurt | Bilgisayar Mühendisliği öğretim elemanları (2026 güz derslerini verir) |
| `P10008`      | Cem Aksu          | Öğretim elemanı ve 1. sınıf demo grubunun danışmanı (Bilgisayar Mühendisliği) |
| `P10010`      | Ahmet Güneş       | Öğretim elemanı (Matematik; Matematik I)                                |
| `P10011`      | Gizem Erdoğan     | Öğretim elemanı (Fizik; Fizik I ve laboratuvarı)                        |
| `P20001`      | Fatma Çelik       | Fakülte öğrenci işleri (Mühendislik Fakültesi)                          |
| `P20002`      | Hasan Öztürk      | Merkezi öğrenci işleri (Daire Başkanlığı)                               |
| `P90001`      | Sistem Yöneticisi | Sistem yöneticisi                                                       |

**İki adımlı doğrulama:** hesap ve rol yönetimi, denetim kayıtları gibi hassas yetkiler sadece MFA ile açılmış oturumlarda kullanılabilir. `P90001` ile ilk girişte üstte çıkan banttan MFA kurun (Google/Microsoft Authenticator ya da benzeri bir uygulama). Telefon kaybolursa kurtarma kodları ya da başka bir yöneticinin "MFA'yı sıfırla" işlemi kullanılır. Geliştirme veritabanını sıfırlamak her şeyi başa döndürür.

**E-postalar:** parola sıfırlama ve aktivasyon bağlantıları ile danışman kararları (onay, geri gönderme, onayı geri alma) worker çalışırken (`make worker`) Mailpit'e düşer.

### Akademik veri

- **Takvim:** 2026-2027 akademik yılı; güz dönemi içinde bulunulan dönemdir. Ders seçme, ekle-bırak, sınav ve not girişi pencereleri temsilî tarihlerle tanımlıdır. Mühendislik Fakültesi'nin ekle-bırak süresi fakülte düzeyinde uzatılmıştır: kapsam önceliği (program → fakülte → üniversite) `GET /api/v1/calendar/windows?faculty_id=...` ile görülebilir.
- **Ders planları:** Bilgisayar Mühendisliği (İngilizce) programının bölümün yayımladığı **gerçek** ders planları: 2022 (2018-2022 girişliler), 2023 (2023-2025) ve 2026 (2026 ve sonrası) sürümleri, 170 ders, teknik seçmeli havuzları, pedagojik formasyon grubu, eski ↔ yeni kod eşdeğerlikleri ve ön koşullar. Planlarda listelenmeyen üniversite alan dışı seçmeli havuzu için birkaç sentetik ders (`UNVG...`) eklenmiştir. Öğrenci kayıtları giriş yılına göre doğru sürüme bağlanır. Üretici: `infra/seed/dev/005_bm_curriculum.sql` başlığına bakın.
- **Geçmiş dönemler:** 2020-2025 akademik yılları. Bilgisayar Mühendisliği öğrencilerine giriş yılına ve ders planlarına göre deterministik bir not geçmişi üretilir (eski sistemden aktarılmış gibi, `source = IMPORT`): başarısız ve tekrar alınan dersler, GANO ve kazanılan AKTS dahil.
- **Not ölçeği ve yönetmelik:** Ankara Üniversitesi lisans not ölçeği ve tarihli yönetmelik parametreleri (AKTS üst sınırları, final barajı ...) migration'la gelir.
- **2026 güz ders açma:** Bilgisayar Mühendisliği'nin 38 dersi (her sınıfın zorunluları ve teknik seçmelileri), şubeleri, haftalık programı (derslik, şube ve öğretim elemanı çakışması olmadan), öğretim elemanları ve değerlendirme planları. `COM2501`'in kontenjanının bir kısmı Yapay Zekâ ve Veri Mühendisliğine ayrılmıştır; birkaç dersin değerlendirme planı kilitlidir.

**Hangi ekranı hangi hesapla denerim?**

| Ekran | Hesap | Not |
| ----- | ----- | --- |
| Akademik takvim | herkes | Birim olarak Mühendislik seçilince uzatılmış ekle-bırak penceresi uygulanır. Olay yönetimi `P20002` ile, MFA kurulduktan sonra |
| Ders planım | `22290002` | 2022 girişli: 2022 ders planı, seçmeli yuvaların havuzları açılır |
| Notlarım | `22290002` | Dönem seçici; geçmiş dönemlerde harf notu, ilan tarihi ve sınıf ortalaması (2025-2026 Bahar'da devamsızlıktan F4). Bileşen notları öğretim elemanı yayımlayınca görünür |
| Transkript | `22290002`; danışman `P10001` öğrenci detayında | Dönem dönem notlar, tekrar ve geçersiz deneme işaretleri (Fizik I iki kez kalındı, yeni koduyla tekrar alınıyor), YANO/GANO, onur; yazdırılabilir görünüm. "Ne olur?" ile sanal transkript: Fizik I'e (PHY0105) B1 verilince eski F2 GANO'dan düşer |
| Ders kataloğu, ders planları, haftalık program, not ölçeği | herkes | Ders planı taslağı açma ve yürürlüğe koyma `P20002` (merkezi öğrenci işleri) ya da `P20001` (fakülte) ile |
| Ders açma | `P10002` (bölüm başkanı) | Şube, kontenjan, öğretim elemanı ve oturum ekleme; çakışmada sunucunun mesajı gösterilir |
| Verdiğim dersler, değerlendirme planı | `P10001` … `P10007`, `P10010`, `P10011` | Planı öğretim elemanı düzenler ve kilitler; kilidi bölüm başkanı (`P10002`) gerekçeyle açar |

### Ders seçme demosu

Gerçek takvimde 2026 güz ders seçmesi Eylül'de biter. Ekranlar her gün denenebilsin diye `make seed`, Bilgisayar Mühendisliği programına **seed'in çalıştığı andan itibaren üç hafta** açık kalan program kapsamlı pencereler açar: ders seçme, danışman onayı ve ekle-bırak (öğrenci, danışman, bölüm başkanı). Süre azalınca `make seed` pencereleri yeniler; takvim ekranında "(demo)" diye görünürler. Kurallar ve akış: [09 · Ders Seçme](docs/mimari/09-ders-secme-kurallari.md).

| Hesap | Öğrenci | Durum | Ne denenir |
| ----- | ------- | ----- | ---------- |
| `22290002` | Ece Kaya, 7. yarıyıl | Kayıt yok | Alttan ders ve eşdeğerlik (Fizik I'in yeni kodu PHY0105), devam şartı sağlanmış derste çakışma uyarısı, şubeler arası çakışma hatası, AKTS sayacı, danışmana gönderme |
| `25290011` | Zeynep Arslan, 3. yarıyıl | Kayıt yok | Ön koşul: devamsızlıktan kaldığı dersin devamı alınamaz |
| `24290012` | Emre Yılmaz, 5. yarıyıl | Taslak | Düşük GANO: 30 AKTS sınırı, alttan ders önceliği |
| `24290014` | Onur Kılıç, 5. yarıyıl | Kayıt yok | Katkı payı borcu: ders seçemez, nedeni sepette yazar |
| `26290004` | Mert Şimşek, 1. yarıyıl | Kayıt yok | 1. sınıf: dönemin bütün zorunlu dersleri alınmalı |
| `23290013`, `25290016`, `26290017` | Buse Demirtaş, Kaan Tekin, İrem Çakır | Onay bekliyor | `P10001`'in kuyruğu: inceleme, onay, gerekçeyle geri gönderme, toplu onay |
| `23290015` | Derya Uçar, 7. yarıyıl | Onaylı | Ekle-bırak talebi, kayıt onay belgesi |

| Ekran | Hesap | Not |
| ----- | ----- | --- |
| Ders seçimi, kayıt onay belgesi | yukarıdaki öğrenciler | Alınabilir dersler kategorilerle (alttan, dönem, seçmeli, üst yarıyıl, not yükseltme), sepet, haftalık program önizlemesi, kural geri bildirimi |
| Danışmanlık | `P10001` | Danışmanlığındaki öğrencilerin kayıtları; kayıt incelemede ders geçmişi, kurallar ve program; onay, geri gönderme, onayı geri alma |
| Ekle-bırak onayları | `P10001` (danışman), sonra `P10002` (bölüm başkanı) | Talep iki onaydan sonra kayda uygulanır |
| Öğrenciler, kayıt engelleri | `P20001` (fakülte öğrenci işleri) | Öğrenci detayında program durumu ve ders geçmişi; engel koyma ve gerekçeyle kaldırma |
| Pencere dışı müdahale, ek süre | `P20001`, `P20002` | `registration:override` MFA ister: önce iki adımlı doğrulamayı kurun |

`make seed` kaydı olan öğrenciye dokunmaz. Demo kayıtlarını baştan kurmak için geliştirme veritabanını sıfırlayın (`docker compose down -v`, ardından hızlı başlangıç adımları).

### Not girişi demosu

Gerçek takvimde 2026 güz not girişi Kasım'da başlar. `make seed`, Mühendislik Fakültesi'ne **seed'in çalıştığı andan itibaren üç hafta** açık kalan fakülte kapsamlı not girişi pencereleri açar: ara sınav, final, elle harf notu ve bütünleme (bütün aşamalar aynı anda açık; takvimde "(demo)").

Demo sınıfı `P10001`'in 1. sınıf dersi **COM1007-1** (Bilgisayar Programlama, ortak öğretim üyesi `P10007` asistan; plan kilitli: ara sınav %25, laboratuvar %15, final %60). 1. sınıf demo grubu (`26290101` … `26290120`) ders seçme servisiyle sepetini kurmuş, danışmanı `P10008` onaylamıştır; kesin kayıtlar gerçek akıştan gelir. Ara sınav notları girilip yayımlanmıştır (`26290108` sınava girmedi), laboratuvar notları girilmiş ama yayımlanmamıştır (iki öğrencinin notu eksik), final girilmemiştir.

| Ne denenir | Hesap | Not |
| ---------- | ----- | --- |
| Not tablosu, not girişi, Excel şablonu ve CSV ile içe ve dışa aktarma | `P10001` | Verdiğim dersler → COM1007-1 → Not tablosu. Not girişi `score:enter` ister ve MFA'lı oturumda açılır: önce iki adımlı doğrulamayı kurun (MFA'sız tablo salt okunur) |
| Ara sınav notu ve sınıf ortalaması | `26290101` … `26290120` | "Notlarım"da yayımlanan ara sınav görünür, laboratuvar görünmez |

`make seed` ara sınavına not girilmiş sınıfa dokunmaz.

**Yük testi:** `make loadtest-registration` ders seçme açılışında eşzamanlı giriş yapıp popüler şubelere ders ekleyen sentetik öğrencileri k6 ile canlandırır ve ardından kontenjan aşımının 0 olduğunu SQL ile doğrular (`make seed-synthetic` gerekir). Sonuçlar ve darboğaz analizi: [08 · Yük Testleri](docs/mimari/08-yuk-testleri.md).

### Büyük veri seti

```bash
make seed-synthetic   # ~92 bin öğrenci, ~10 bin akademik ve idari personel (~6-10 sn)
```

Sentetik hesapların parolası da aynıdır. Öğrenciler `18062001` ... `26067322` (8 hane), akademisyenler `A000001` ..., fakülte öğrenci işleri `I000001` ... biçimindedir. Her bölümün başkanı, her fakültenin dekanı ve öğrenci işleri, her öğrencinin danışmanı vardır. Veri belirlenimcidir: aynı tohum (`-seed`) aynı kişileri üretir.

## Komutlar

`make help` bütün komutları listeler. En sık kullanılanlar:

| Komut              | Açıklama                                                                 |
| ------------------ | ------------------------------------------------------------------------ |
| `make check`       | Backend: biçim, go.mod, vet, staticcheck, govulncheck, testler (CI ile aynı) |
| `make test`        | Backend testleri (gerçek PostgreSQL ve Redis, testcontainers ile)        |
| `make web-check`   | Web: biçim, lint, tip kontrolü, testler, build (CI ile aynı)             |
| `make web-e2e`     | Uçtan uca testler (Playwright), ayrı bir veritabanında                   |
| `make contract-lint` | OpenAPI sözleşmesini doğrular                                          |
| `make web-codegen` | Sözleşmeden web API istemcisini yeniden üretir                           |
| `make migration name=...` | Yeni migration dosyası                                            |
| `make term-results-verify` | Dönem sonuçlarını (YANO/GANO) kesin kayıtlardan yeniden hesaplar ve kayıtlılarla karşılaştırır; `make term-results-recompute` farkları yazar |

## Testler ve CI

| Katman        | Araç                                     | Kapsam                                                               |
| ------------- | ---------------------------------------- | -------------------------------------------------------------------- |
| Backend       | Go testleri + testcontainers + race      | Servisler, repository'ler ve HTTP uçları gerçek PostgreSQL/Redis ile |
| Web           | Vitest + Testing Library + MSW           | Bütün uygulama bellek içi router'la kullanıcı gibi gezilir           |
| Uçtan uca     | Playwright                               | Gerçek API, worker, veritabanı ve Mailpit ile kullanıcı senaryoları  |
| Sözleşme      | Redocly + üretilmiş istemci denetimi     | API ile web istemcisinin ayrışmaması                                 |

GitHub Actions'ta `backend` ve `web` (birim + uçtan uca) iş akışları her push'ta çalışır.
