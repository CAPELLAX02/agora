# 06 · Sistem Mimarisi ve Teknoloji Seçimleri

> **Durum:** Taslak v0.1 · Sürümler kurulum anında **en güncel kararlı** sürüm olarak sabitlenecek.

## 1. Yüksek Seviye Mimari

```mermaid
flowchart TB
  subgraph Clients[İstemciler]
    WEB[Web · Vite + React + TS]
    MOB[Mobil · Expo + RN + TS]
    PUB[Halka açık<br/>belge doğrulama]
  end
  RP[Ters vekil · Caddy<br/>TLS, sıkıştırma, statik dosya]
  subgraph App[Go uygulaması · tek repo, iki ikili dosya]
    API[cmd/api<br/>REST + SSE]
    WRK[cmd/worker<br/>outbox, e-posta, push, ETL,<br/>zamanlanmış işler, PDF]
  end
  PG[(PostgreSQL 18<br/>agora · OLTP)]
  DW[(PostgreSQL 18<br/>agora_dw · OLAP)]
  RD[(Redis<br/>önbellek, rate limit,<br/>oturum iptali, pub/sub)]
  S3[(MinIO · S3<br/>dosyalar)]
  MAIL[Mailpit<br/>geliştirme SMTP]
  subgraph Obs[Gözlemlenebilirlik]
    PROM[Prometheus]
    GRAF[Grafana]
    LOKI[Loki]
    TEMPO[Tempo · opsiyonel]
  end
  SW[Swagger UI<br/>OpenAPI 3.1]
  K6[k6 yük testleri]

  WEB & MOB & PUB --> RP --> API
  API --> PG & RD & S3
  WRK --> PG & DW & RD & S3 & MAIL
  API & WRK -- /metrics --> PROM --> GRAF
  API & WRK -- JSON log --> LOKI --> GRAF
  DW --> GRAF
  K6 -- remote write --> PROM
  SW -. contracts/openapi.yaml .- API
```

### 1.1 Neden mikroservis değil, **modüler monolit**?

| Kriter | Modüler monolit | Mikroservis |
| --- | --- | --- |
| Transaction tutarlılığı (kayıt + kontenjan + outbox) | Tek DB transaction'ı | Saga/telafi gerekir |
| Operasyonel yük | 2 ikili dosya | N servis, servis keşfi, ağ hataları |
| Go öğrenme odağı | Dil, stdlib, SQL, eşzamanlılık | Altyapıya kayar |
| Ölçekleme | API yatayda çoğaltılır (durumsuz). Worker ayrı ölçeklenir | Servis bazında |
| Gelecekte bölme | Modül sınırları net olduğu için mümkün | — |

**Kural:** Modüller birbirinin tablosuna **doğrudan** sorgu atmaz. Diğer modülün servis arayüzünü çağırır ya da olay dinler. Böylece sınırlar korunur ve ileride servis ayırmak mümkün kalır.

## 2. Backend (Go) — "Sihir yok, her şey açık"

### 2.1 Spring → Go zihinsel eşleme

| Spring Boot'ta | Go'da (Agora'da) | Not |
| --- | --- | --- |
| `@SpringBootApplication`, otomatik konfigürasyon | `main.go` içinde **elle kurulum** (config oku → DB havuzu → repository → service → handler → router) | Bağımlılık grafiği tek dosyada, okunabilir |
| DI container, `@Autowired` | **Constructor fonksiyonları** (`NewRegistrationService(repo, clock, policy)`) | Interface'i *kullanan* tarafta tanımla (küçük interface) |
| `@RestController`, `@GetMapping` | `http.Handler` / `http.HandlerFunc`, `mux.HandleFunc("GET /api/v1/terms/{id}", h.getTerm)` (Go 1.22+ ServeMux) | Framework yok, stdlib router |
| Filter chain, `HandlerInterceptor` | **Middleware**: `func(http.Handler) http.Handler` zinciri | AuthN, rate limit, log, recover, metrics |
| Spring Security | Kendi AuthN middleware'imiz + politika fonksiyonları | Bkz. doküman 03 |
| `@Valid`, Bean Validation | Açık `Validate() error` metotları, alan bazlı hata listesi | Reflection yok |
| Checked/unchecked exception, `@ControllerAdvice` | **Error değerleri** (`errors.Is/As`, sentinel ve tipli hatalar) + tek bir hata → HTTP eşleyici (RFC 9457) | Hata akışı görünür |
| JPA / Hibernate, `@Entity`, lazy loading | **pgx v5** + elle yazılmış SQL, açık `Scan` | N+1 ve gizli sorgu yok |
| `@Transactional` | `db.InTx(ctx, func(tx pgx.Tx) error { … })` yardımcı fonksiyonu | Transaction sınırı kodda görünür |
| Flyway / Liquibase | **goose** veya **golang-migrate** (düz `.sql` dosyaları) | |
| `application.yml`, `@Value` | `Config` struct'ı + `os.Getenv` + açık parse/doğrulama | 12-factor |
| Actuator | `/healthz` (canlılık), `/readyz` (DB/Redis hazır mı), `/metrics` (Prometheus) | |
| SLF4J / Logback | `log/slog` (JSON handler) | Yapılandırılmış log, stdlib |
| `@Scheduled` | Worker'da `time.Ticker` + goroutine + `context` ile iptal | |
| `@Async`, thread pool | Goroutine + channel + `errgroup` / worker pool | Eşzamanlılık Go'nun gücü |
| Spring Events | Transactional outbox + worker | Kalıcı ve güvenilir |
| Jackson | `encoding/json` (struct tag'leri) | |
| Lombok | Gerek yok | |
| JUnit + Mockito | `testing` paketi, tablo güdümlü testler, el yazımı fake'ler | Mock framework'süz |
| Testcontainers | **testcontainers-go** (gerçek PostgreSQL ile entegrasyon testi) | |
| Springdoc OpenAPI (koddan spec) | **Spec-first**: `contracts/openapi.yaml` elle yazılır → Swagger UI | Sözleşme önce, kod sonra |

### 2.2 Bağımlılık politikası

**Stdlib önce.** Dış bağımlılık ancak (a) güvenlik açısından kritikse, (b) protokol sürücüsüyse ya da (c) yeniden yazmak öğrenme değeri taşımıyorsa eklenir.

| Bağımlılık | Neden | Alternatif (kendimiz) |
| --- | --- | --- |
| `github.com/jackc/pgx/v5` | PostgreSQL sürücüsü + havuz (`pgxpool`), `COPY`, tipler | — |
| `github.com/redis/go-redis/v9` | Redis sürücüsü | — |
| `golang.org/x/crypto/argon2` | Parola hash (yarı-stdlib) | — |
| `github.com/prometheus/client_golang` | Metrik sunumu | — |
| `github.com/pressly/goose/v3` *veya* `golang-migrate` | Migration çalıştırıcı | Basit bir migrator yazmak da mümkün (öğrenme opsiyonu) |
| `github.com/minio/minio-go/v7` | S3 istemcisi | — |
| `github.com/testcontainers/testcontainers-go` | Entegrasyon testleri | — |
| **Kendimiz yazacaklarımız** | Router üstü küçük yardımcılar, middleware zinciri, **JWT (Ed25519)**, TOTP, girdi doğrulama, config, RFC 9457 hata yanıtları, cursor pagination, rate limiter (Redis + Lua), outbox worker, ETL | Öğrenmenin kalbi |

### 2.3 Önerilen klasör yapısı (tartışmaya açık — kodu sen yazacaksın)

```text
backend/
├── cmd/
│   ├── api/main.go            # HTTP sunucu: kurulum + graceful shutdown
│   ├── worker/main.go         # outbox, e-posta, ETL, zamanlanmış işler
│   └── seed/main.go           # gerçekçi sentetik veri üretici
├── internal/
│   ├── platform/              # modüller arası ortak altyapı
│   │   ├── config/  db/  httpx/  logging/  metrics/  redisx/  clock/  validate/
│   │   └── auth/              # token üretim/doğrulama, principal, middleware
│   ├── iam/                   # her modül: handler.go, service.go, repository.go, model.go, policy.go, *_test.go
│   ├── org/  people/  academic/  curriculum/  offering/
│   ├── enrollment/  grading/  attendance/  lms/
│   ├── communication/  survey/  documents/  finance/
│   └── analytics/             # DW okuma uç noktaları + ETL işleri
├── migrations/                # 0001_init_iam.sql …
└── go.mod
```

Katmanlar: **handler** (HTTP ↔ DTO, doğrulama) → **service** (iş kuralları, transaction, politika) → **repository** (SQL). Domain modelleri framework'ten bağımsız düz struct'lardır.

## 3. API Tasarım Kuralları

| Konu | Karar |
| --- | --- |
| Stil | REST + JSON, kaynak odaklı, sürüm: `/api/v1` |
| Sözleşme | **OpenAPI 3.1, spec-first**: `contracts/openapi.yaml` tek doğruluk kaynağı. Swagger UI ile yayınlanır. TS istemcisi buradan üretilir |
| Hata biçimi | **RFC 9457 `application/problem+json`**: `type, title, status, detail, instance, errors[] (alan bazlı), code (makine okunur: REGISTRATION_ECTS_LIMIT_EXCEEDED)` |
| Sayfalama | Cursor tabanlı (`?limit=50&cursor=…`), uuidv7 sıralaması sayesinde kararlı. Yönetim tablolarında offset de olabilir |
| Filtre/sıralama | `?filter[term]=2026-FALL&sort=-created_at` |
| Eşzamanlılık | `ETag` + `If-Match` → 412 Precondition Failed |
| İdempotentlik | Kritik POST'larda `Idempotency-Key` başlığı |
| Zaman | ISO 8601 UTC (`2026-10-04T10:00:00Z`) |
| Gerçek zamanlı | **SSE** (`/api/v1/notifications/stream`) bildirimler için. Mesajlaşmada gerekirse WebSocket (P1) |
| Bağlam | Öğrenci uç noktaları aktif program bağlamını açıkça alır: `/api/v1/me/programs/{studentProgramId}/registrations/current` |
| Rate limit başlıkları | `RateLimit-Limit`, `RateLimit-Remaining`, `Retry-After` |
| Korelasyon | `X-Request-Id` (yoksa üretilir, log ve yanıtta döner) |

## 4. Web Frontend

| Konu | Seçim |
| --- | --- |
| Araç | **Vite** + **React** + **TypeScript (strict)** |
| UI | **Tailwind CSS v4** + **shadcn/ui** (Radix tabanlı, erişilebilir) + lucide ikonlar |
| Durum | **Redux Toolkit** + **RTK Query** (API önbellek, invalidation). İstemci kodu OpenAPI'den `@rtk-query/codegen-openapi` ile üretilir |
| Yönlendirme | **React Router** (data router), rol bazlı route guard'lar, lazy route'lar |
| Formlar | **react-hook-form** + **zod** (şemalar OpenAPI tipleriyle uyumlu) |
| i18n | **i18next** (TR varsayılan, EN). Eksik anahtar CI'da hata verir |
| Mock | **MSW** (Mock Service Worker). Backend hazır olmadan sözleşmeye göre çalışmak için |
| Tablo / grafik | TanStack Table (shadcn data-table) · Recharts (analitik paneller) |
| Takvim | Haftalık ders programı / takvim bileşeni (ders seçmede çakışma görselleştirme) |
| Test | **Vitest** + Testing Library (bileşen) · **Playwright** (e2e, rol senaryoları) |
| Kalite | ESLint + Prettier + `tsc --noEmit` · erişilebilirlik için axe denetimi |
| Yapı | Özellik bazlı klasörler: `src/features/registration/…`, `src/app/` (store, router), `src/shared/ui` |
| Tema | Açık/koyu/sistem, Ankara Üniversitesi renklerinden esinlenmiş ama özgün kimlik |

## 5. Mobil

| Konu | Seçim |
| --- | --- |
| Çatı | **Expo** (en güncel SDK) + React Native + TypeScript |
| Yönlendirme | **expo-router** (dosya tabanlı) |
| Stil | **NativeWind** (Tailwind sınıfları, web ile ortak tasarım dili) |
| Durum | Redux Toolkit + RTK Query (web ile aynı yaklaşım, ortak üretilmiş API tipleri) |
| Güvenli saklama | `expo-secure-store` (refresh token) |
| Bildirim | `expo-notifications` (Expo push) |
| Kamera | QR yoklama ve QR ile oturum açma (P2) |

> **Ortak kod önerisi:** `packages/api-types` (OpenAPI'den üretilmiş tipler) ve `packages/i18n` (ortak çeviriler). pnpm workspaces ile web ve mobil paylaşır. Bu, "3 alt proje" kararına ek iki küçük paylaşım paketi demek. Onayına bağlı.

## 6. Altyapı ve Geliştirme Ortamı

`docker compose` servisleri: `postgres` (agora + agora_dw), `redis`, `minio`, `mailpit`, `prometheus`, `grafana` (datasource ve paneller kod olarak provision edilir), `loki` (+ `promtail`/`alloy`), `swagger-ui`, opsiyonel `tempo`.

| Konu | Karar |
| --- | --- |
| Görev çalıştırıcı | Kök `Makefile` (ya da `Taskfile.yml`): `make up`, `make migrate`, `make seed`, `make test`, `make loadtest-registration` |
| CI | GitHub Actions: Go (`go vet`, `staticcheck`, `go test -race`, `govulncheck`) · Web (lint, typecheck, test, build) · ileride Playwright e2e ve OWASP ZAP baseline taraması |
| Konteyner | Çok aşamalı Dockerfile, distroless/scratch Go imajı, Trivy imaj taraması |
| Demo dağıtımı | Tek VPS + Docker Compose + Caddy (otomatik HTTPS). Opsiyonel: k3s |
| Gizli bilgiler | `.env` (gitignore'da), `.env.example` repoda. İmza anahtarları dosyadan okunur |

## 7. Gözlemlenebilirlik

| Sinyal | Ne ölçülür |
| --- | --- |
| **RED metrikleri** | `http_requests_total{route,method,status}`, `http_request_duration_seconds` (histogram) |
| **Kaynaklar** | pgxpool (kullanılan/boşta/bekleyen bağlantı, bekleme süresi), Redis gecikmesi, Go runtime (goroutine, GC, heap) |
| **İş metrikleri** | `agora_registration_items_added_total`, `agora_registration_submitted_total`, `agora_section_full_rejections_total`, `agora_grades_published_total`, `agora_login_failures_total`, `agora_refresh_reuse_detected_total`, `agora_outbox_lag_seconds`, `agora_etl_lag_seconds` |
| **Log** | `slog` JSON: `time, level, msg, request_id, user_id (takma), route, status, duration_ms`. PII maskeleme |
| **İz (opsiyonel)** | OpenTelemetry → Tempo. HTTP → servis → SQL span'leri |
| **Uyarılar** | 5xx oranı > %1 (5 dk), p95 > 500 ms, outbox gecikmesi > 60 sn, DB havuzu tükenmesi |
| **SLO panosu** | Uptime, hata bütçesi, p95/p99 |

## 8. Test Stratejisi

| Katman | Araç | Kapsam |
| --- | --- | --- |
| Birim (Go) | `testing`, tablo güdümlü | Kural motoru (AKTS limiti, harf notu, GANO, politika fonksiyonları) — **en yüksek kapsam burada** |
| Entegrasyon (Go) | testcontainers-go + gerçek PostgreSQL | Repository'ler, transaction'lar, **kontenjan yarış testi** (N goroutine aynı anda) |
| Sözleşme | OpenAPI şema doğrulaması | API yanıtları spec ile uyumlu mu |
| Bileşen (web) | Vitest + Testing Library + MSW | |
| E2E | Playwright | Rol senaryoları: öğrenci ders seçer → danışman onaylar → LMS'te ders görünür |
| Yük / stres | **k6** | Aşağıda |
| Güvenlik | govulncheck, gosec, npm audit, OWASP ZAP baseline, Trivy | |

## 9. Yük ve Stres Testi (k6)

**Neden k6 (JMeter yerine)?** Senaryolar JavaScript kodu olarak yazılır, versiyonlanır ve CI'da çalışır. Gerçekçi trafik için `ramping-arrival-rate` gibi yürütücüler (executor) var. **Eşikler (thresholds)** SLO kapısı olarak kullanılabilir. Sonuçlar Prometheus remote write ile Grafana'da canlı izlenir. Düşük kaynakla yüksek yük üretir. Sektörde modern standart haline geldi.

| Senaryo | Tip | Profil | Başarı kriteri |
| --- | --- | --- | --- |
| `smoke` | Duman | 1–5 VU, 1 dk | Hata yok |
| `daily-lms` | Yük | Gün içi gerçekçi karışım (dashboard, ders sayfası, dosya indirme, ödev teslimi), 30 dk | p95 < 300 ms, hata < %0,1 |
| `registration-rush` | **Stres** | Ders seçme açılışı: 0 → 10.000 öğrencinin 5 dakikada giriş yapıp sepete ders eklemesi (`ramping-arrival-rate`), popüler şubelerde yoğun rekabet | **Kontenjan aşımı = 0**, p95 < 800 ms, 5xx < %0,5 |
| `grade-publish-spike` | Ani yük (spike) | Not ilanı anında 20 sn içinde 30.000 not sorgusu | Önbellek etkinliği, p99 < 1 sn |
| `soak` | Dayanıklılık | Orta yük, 4–8 saat | Bellek sızıntısı yok, gecikme kayması yok |
| `breakpoint` | Kırılma noktası | Artan yük, sistem çökene kadar | Kırılma noktası ve darboğaz (DB, CPU, havuz) raporlanır |

Her senaryo sonrası: Grafana ekran görüntüleri, `pg_stat_statements` en yavaş sorgular, iyileştirme → tekrar ölçüm. Bitirme raporunda "önce/sonra" bölümü olarak kullanılır.
