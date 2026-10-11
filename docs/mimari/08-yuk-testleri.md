# 08 · Yük Testleri

> **Durum:** Faz 3 · `registration-rush` ilk ölçümleri (2026-10-11). Senaryolar ve hedefler için bkz. [06 · §9](06-sistem-mimarisi-ve-teknoloji.md).

## 1. `registration-rush`: ders seçme açılışı

**Soru:** Ders seçme açıldığında yüzlerce öğrenci aynı anda aynı şubelere saldırırken kontenjan aşılır mı? Ders seçme uçları ne kadar hızlı yanıt verir?

**Senaryo** ([loadtest/k6/registration-rush.js](../../loadtest/k6/registration-rush.js)). Öğrenciler artan bir hızla gelir (`ramping-arrival-rate`). Her öğrenci:

1. Giriş yapar (argon2id).
2. Alınabilir derslerini ister.
3. Önce alttan, sonra dönem derslerini, sonra yarıyılına gelmiş seçmelilerini sepete ekler. Dolu şubede aynı dersin varsa başka şubesini dener.
4. Sepeti danışmana gönderir.

Aynı yarıyıldaki öğrenciler aynı seçmeli şubelerine (kontenjan 40) ve birinci sınıflar kontenjanı 120 olan zorunlu derslere yüklenir: 176 birinci sınıf öğrencisi 120 kişilik COM1015'e girmeye çalışır.

Beklenen 4xx'ler hata sayılmaz:
- 409: kontenjan dolu, pencere kapalı.
- 422: kural ihlali (çakışma, AKTS sınırı, ön koşul).

`PRELOGIN=1` kipinde öğrenciler pencere açılmadan giriş yapmış sayılır: tokenlar kurulumda alınır ve sadece ders seçme yükü ölçülür.

**Kanıt** ([loadtest/sql/verify.sql](../../loadtest/sql/verify.sql)). Test bittikten sonra veritabanında şunlar sayılır:
- Kontenjanını aşan şube.
- Mevcudu sepet satırlarıyla tutmayan şube.
- Program kontenjanı aşımı.
- Aynı dersi iki kez alan sepet.
- AKTS sınırını aşan sepet.

Hepsi 0 olmalıdır.

**Ortam**

| | |
| --- | --- |
| Makine | Geliştirici dizüstü bilgisayarı, 10 çekirdek (Apple Silicon), 12 GB Docker belleği |
| Veritabanı | PostgreSQL 18 (Docker Desktop), aynı makinede |
| API | Tek süreç. DB havuzu 40 bağlantı, 8 parola hash işçisi. Giriş ve ders seçme hız sınırları test için gevşetildi (`AGORA_LOGIN_RATE_LIMIT`, `AGORA_REGISTRATION_RATE_LIMIT`) |
| Parola | argon2id: m = 64 MiB, t = 3, p = 2 |
| Veri | Bilgisayar Mühendisliği'nin 1.121 sentetik öğrencisi (1., 3., 5. ve 7. yarıyıl), 2026 güz dönemi 39 şube. Ders geçmişleri seed'den gelir; kural motoru her eklemede çalışır |
| Araç | k6 2.3.0 (`grafana/k6:2.3.0`), aynı makinede Docker'da |

**Sonuçlar**

| Tur | Kip | Tepe hız (öğrenci/sn) | İstek | 5xx | Ders seçme p95 | p99 | Kontenjan denetimi |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 1 | Giriş dahil, önbelleksiz | 20 | 17.369 | 0 | 1,19 sn ✗ | 1,63 sn | 0 aşım, 0 tutarsızlık |
| 2 | Giriş dahil, kural girdileri önbellekte | 20 | 17.363 | 0 | **501 ms** ✓ | 783 ms | 0 aşım, 0 tutarsızlık |
| 3 | Giriş dahil (doyum) | 45 | 15.208 | 0 | 3,47 sn ✗ | 4,84 sn | 0 aşım, 0 tutarsızlık |
| 4 | Önceden giriş | 45 | 17.366 | 0 | **254 ms** ✓ | 356 ms | 0 aşım, 0 tutarsızlık |

Her turda yaklaşık 30 şube kontenjanına **tam** olarak doldu: 40/40, 80/80, 120/120. Turlarda:
- yaklaşık 9.200 "kontenjan dolu" yanıtı verildi,
- 380 ekleme kural motoruna takıldı.

Kontenjan aşımı hiçbir turda olmadı. Bunu sağlayan, sepete eklemedeki koşullu atomik güncellemedir:

```sql
UPDATE offering.sections SET enrolled_count = enrolled_count + 1
WHERE id = $1 AND enrolled_count < capacity
```

Satır kilidi eşzamanlı istekleri sıraya koyar; kilidi bekleyen istek koşulu güncel değerle yeniden değerlendirir. Veritabanındaki `CHECK (enrolled_count <= capacity)` kısıtı son savunma hattıdır.

## 2. Darboğaz analizi ve yapılan iyileştirme

**Tur 1 → 2: kural girdilerinin önbelleğe alınması.** Kural motoru her eklemede şunları okuyordu:
- öğrencinin müfredatı,
- bütün eşdeğerlikler ve seçmeli havuzlar,
- ön koşullar,
- yönetmelik parametreleri,
- açık şubelerin bütün ayrıntıları (öğretim elemanları, program).

Bunlar dönem içinde neredeyse hiç değişmez; yürürlükteki müfredat zaten dondurulmuştur. Süreç içi, 5 dakikalık bir önbelleğe alındılar. Ekleme işlemi şube ayrıntısı yerine sadece hangi derslerin açık olduğunu okur. Sonuç: p95 1,19 sn → 501 ms, ortalama 576 → 197 ms.

**Tur 3: doyum.** Saniyede 45 öğrencide yanıt süreleri uzadı. Doyum anında alınan CPU profilinde (`/debug/pprof/profile`) API'nin CPU zamanının **%85'i argon2id parola doğrulamasıydı**: `argon2.processBlockGeneric` ve `blamkaGeneric`. Ders seçme uçlarının kendisi ucuzdu. Etkenler:
- Giriş bilerek pahalıdır: her doğrulama 64 MiB bellek ve birkaç çekirdek-milisaniye ister.
- `golang.org/x/crypto/argon2`'nin arm64 için hızlandırılmış (assembly) uygulaması yoktur. Bu makinede genel Go uygulaması çalıştı; amd64 sunucuda SSE ile daha hızlıdır.
- API, PostgreSQL ve k6 aynı 10 çekirdeği paylaştı. Tepe anında PostgreSQL de %300 CPU kullandı.

**Tur 4: önceden giriş.** Girişler ders seçme açılmadan yapıldığında aynı hızda ders seçme p95'i 254 ms oldu. Gerçek açılışa daha yakın olan da budur: oturumlar boşta 2 saat, en fazla 30 gün sürer ve öğrenciler genellikle açılıştan önce giriş yapıp bekler.

**Öneriler** (Faz 9, sertleştirme):
1. Giriş fırtınası için API yatayda çoğaltılır. Parola hash işçi sayısı (`AGORA_PASSWORD_HASH_WORKERS`) çekirdek sayısının altında tutulur; böylece doğrulama kuyruğu ders seçme isteklerini aç bırakmaz.
2. Sunucu amd64 olur (argon2'nin SSE uygulaması) ya da argon2 parametreleri sunucuda ölçülerek ayarlanır (OWASP alt sınırının altına inmeden).
3. Ders seçme yanıtları kaydın güncel halini yeniden değerlendirir (ikinci bir kural çalıştırması). Yük altında bu, yanıtı işlem sonucundan üretmekle kısaltılabilir.
4. 10.000 öğrencilik hedef senaryo ([06 · §9](06-sistem-mimarisi-ve-teknoloji.md)) birden çok programın ders açmasını ister. Şu an sadece Bilgisayar Mühendisliği'nin gerçek ders planı ve açılan dersleri var; senaryo diğer programlar için sentetik ders açma seed'iyle büyütülecek.

## 3. Nasıl çalıştırılır

[loadtest/README.md](../../loadtest/README.md).
