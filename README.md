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
make worker        # e-postalar (parola sıfırlama, aktivasyon) için
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
| Worker metrikleri | http://localhost:9092/metrics                        | E-posta outbox'ı                                                    |
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
| `22290003`    | Can Polat         | Öğrenci, **geçici parola**: ilk girişte parolasını değiştirmek zorunda  |
| `P10001`      | Ayşe Demir        | Öğretim elemanı ve danışman (Bilgisayar Mühendisliği)                   |
| `P10002`      | Mehmet Yıldız     | Öğretim elemanı ve bölüm başkanı (Bilgisayar Mühendisliği)              |
| `P10003` … `P10007` | Elif Şahin, Burak Arslan, Selin Koç, Kerem Aydın, Zehra Kurt | Bilgisayar Mühendisliği öğretim elemanları (2026 güz derslerini verir) |
| `P10010`      | Ahmet Güneş       | Öğretim elemanı (Matematik; Matematik I)                                |
| `P10011`      | Gizem Erdoğan     | Öğretim elemanı (Fizik; Fizik I ve laboratuvarı)                        |
| `P20001`      | Fatma Çelik       | Fakülte öğrenci işleri (Mühendislik Fakültesi)                          |
| `P20002`      | Hasan Öztürk      | Merkezi öğrenci işleri (Daire Başkanlığı)                               |
| `P90001`      | Sistem Yöneticisi | Sistem yöneticisi                                                       |

**İki adımlı doğrulama:** hesap ve rol yönetimi, denetim kayıtları gibi hassas yetkiler sadece MFA ile açılmış oturumlarda kullanılabilir. `P90001` ile ilk girişte üstte çıkan banttan MFA kurun (Google/Microsoft Authenticator ya da benzeri bir uygulama). Telefon kaybolursa kurtarma kodları ya da başka bir yöneticinin "MFA'yı sıfırla" işlemi kullanılır. Geliştirme veritabanını sıfırlamak her şeyi başa döndürür.

**E-postalar:** parola sıfırlama ve aktivasyon bağlantıları worker çalışırken (`make worker`) Mailpit'e düşer.

### Akademik veri

- **Takvim:** 2026-2027 akademik yılı; güz dönemi içinde bulunulan dönemdir. Ders seçme, ekle-bırak, sınav ve not girişi pencereleri temsilî tarihlerle tanımlıdır. Mühendislik Fakültesi'nin ekle-bırak süresi fakülte düzeyinde uzatılmıştır: kapsam önceliği (program → fakülte → üniversite) `GET /api/v1/calendar/windows?faculty_id=...` ile görülebilir.
- **Ders planları:** Bilgisayar Mühendisliği (İngilizce) programının bölümün yayımladığı **gerçek** ders planları: 2022 (2018-2022 girişliler), 2023 (2023-2025) ve 2026 (2026 ve sonrası) sürümleri, 170 ders, teknik seçmeli havuzları, pedagojik formasyon grubu, eski ↔ yeni kod eşdeğerlikleri ve ön koşullar. Planlarda listelenmeyen üniversite alan dışı seçmeli havuzu için birkaç sentetik ders (`UNVG...`) eklenmiştir. Öğrenci kayıtları giriş yılına göre doğru sürüme bağlanır. Üretici: `infra/seed/dev/005_bm_curriculum.sql` başlığına bakın.
- **Not ölçeği ve yönetmelik:** Ankara Üniversitesi lisans not ölçeği ve tarihli yönetmelik parametreleri (AKTS üst sınırları, final barajı ...) migration'la gelir.
- **2026 güz ders açma:** Bilgisayar Mühendisliği'nin 38 dersi (her sınıfın zorunluları ve teknik seçmelileri), şubeleri, haftalık programı (derslik, şube ve öğretim elemanı çakışması olmadan), öğretim elemanları ve değerlendirme planları. `COM2501`'in kontenjanının bir kısmı Yapay Zekâ ve Veri Mühendisliğine ayrılmıştır; birkaç dersin değerlendirme planı kilitlidir.

**Hangi ekranı hangi hesapla denerim?**

| Ekran | Hesap | Not |
| ----- | ----- | --- |
| Akademik takvim | herkes | Birim olarak Mühendislik seçilince uzatılmış ekle-bırak penceresi uygulanır. Olay yönetimi `P20002` ile, MFA kurulduktan sonra |
| Ders planım | `22290002` | 2022 girişli: 2022 ders planı, seçmeli yuvaların havuzları açılır |
| Ders kataloğu, ders planları, haftalık program, not ölçeği | herkes | Ders planı taslağı açma ve yürürlüğe koyma `P20002` (merkezi öğrenci işleri) ya da `P20001` (fakülte) ile |
| Ders açma | `P10002` (bölüm başkanı) | Şube, kontenjan, öğretim elemanı ve oturum ekleme; çakışmada sunucunun mesajı gösterilir |
| Verdiğim dersler, değerlendirme planı | `P10001` … `P10007`, `P10010`, `P10011` | Planı öğretim elemanı düzenler ve kilitler; kilidi bölüm başkanı (`P10002`) gerekçeyle açar |

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

## Testler ve CI

| Katman        | Araç                                     | Kapsam                                                               |
| ------------- | ---------------------------------------- | -------------------------------------------------------------------- |
| Backend       | Go testleri + testcontainers + race      | Servisler, repository'ler ve HTTP uçları gerçek PostgreSQL/Redis ile |
| Web           | Vitest + Testing Library + MSW           | Bütün uygulama bellek içi router'la kullanıcı gibi gezilir           |
| Uçtan uca     | Playwright                               | Gerçek API, worker, veritabanı ve Mailpit ile kullanıcı senaryoları  |
| Sözleşme      | Redocly + üretilmiş istemci denetimi     | API ile web istemcisinin ayrışmaması                                 |

GitHub Actions'ta `backend` ve `web` (birim + uçtan uca) iş akışları her push'ta çalışır.
