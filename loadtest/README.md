# Yük testleri (k6)

Sonuçlar ve darboğaz analizi: [docs/mimari/08-yuk-testleri.md](../docs/mimari/08-yuk-testleri.md).

## `registration-rush`: ders seçme açılışı

Ön koşullar:
- Geliştirme veritabanında sentetik veri: `make seed-synthetic`. Bilgisayar Mühendisliği'nin sentetik öğrencilerine ders geçmişi ve demo ders seçme pencereleri seed'den gelir.
- Docker. k6, `grafana/k6:2.3.0` imajıyla çalışır; ayrıca kurulmaz.

1. API'yi test için gevşek hız sınırlarıyla başlatın. Normal sınırlar (IP başına dakikada 20 giriş, kullanıcı başına dakikada 60 ders seçme işlemi) yük testindeki tek IP'yi hemen durdurur:

   ```bash
   AGORA_LOGIN_RATE_LIMIT=100000 AGORA_REGISTRATION_RATE_LIMIT=10000 AGORA_DB_MAX_CONNS=40 make api
   ```

2. Testi çalıştırın. Test bitince kontenjan denetimi (`make loadtest-verify`) kendiliğinden çalışır:

   ```bash
   make loadtest-registration
   ```

   Ayarlar ortam değişkeniyle verilir:

   | Değişken | Varsayılan | Anlamı |
   | --- | --- | --- |
   | `PEAK_RATE` | 25 | Tepe noktada saniyede gelen öğrenci |
   | `RAMP` / `HOLD` | 1m / 1m | Artış ve tepe süresi |
   | `PRELOGIN` | (boş) | `1`: öğrenciler önceden giriş yapmış olur, sadece ders seçme ölçülür |
   | `LOADTEST_BASE_URL` | `http://host.docker.internal:8080` | API adresi (k6 Docker'dan host'a ulaşır) |

   Örnek: `PRELOGIN=1 PEAK_RATE=45 RAMP=15s HOLD=20s make loadtest-registration`

3. Testi tekrarlamadan önce sentetik öğrencilerin ders seçme kayıtlarını silin. Demo öğrencilerine dokunulmaz:

   ```bash
   make loadtest-reset
   ```

k6'nın özet çıktısı `loadtest/results/registration-rush.json` dosyasına yazılır (depoya girmez). İzleme için Grafana'daki "Agora API" panosunun "Ders seçme" satırına bakın.
