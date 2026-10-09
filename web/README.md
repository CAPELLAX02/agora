# Agora · Web

Vite, React, TypeScript (strict), Tailwind CSS v4, shadcn/ui yaklaşımıyla yazılmış
bileşenler ve Redux Toolkit (RTK Query) ile geliştirilen web arayüzü.

## Çalıştırma

```bash
make up && make migrate-up && make seed   # PostgreSQL, Redis, şema ve demo veriler
make api                                   # API: http://localhost:8080
make web-install                           # ilk seferde
make web-dev                               # Web: http://localhost:5173
```

pnpm ayrıca kurulmaz: Node.js ile gelen corepack, `package.json`'daki `packageManager`
sürümünü kullanır.

Geliştirme sunucusu `/api` isteklerini API'ye aktarır (`AGORA_API_URL`, varsayılan
`http://localhost:8080`). Tarayıcı için web ve API aynı origin'dedir, production'daki gibi.

## Komutlar

| Komut              | Açıklama                                                  |
| ------------------ | --------------------------------------------------------- |
| `make web-check`   | Biçim, lint, tip kontrolü, test ve build (CI ile aynı)    |
| `make web-codegen` | `contracts/openapi.yaml`'dan RTK Query istemcisini üretir |
| `pnpm test:watch`  | Testleri izleme modunda çalıştırır (`web/` içinde)        |

## Mimari

| Konu          | Yaklaşım                                                                                                                                                                                               |
| ------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Klasörler     | `src/app` (store, router, tema), `src/features/<özellik>` (sayfalar ve özelliğe ait mantık), `src/shared/ui` (tasarım sistemi), `src/shared/api` (API istemcisi)                                       |
| API istemcisi | `contracts/openapi.yaml`'dan üretilir (`src/shared/api/generated.ts`, elle düzenlenmez). CI, üretilmiş dosyanın sözleşmeyle uyumlu olduğunu denetler                                                   |
| Oturum        | Access token sadece Redux'ta (bellekte) tutulur, refresh token HttpOnly çerezdedir. Sayfa açılınca oturum çerezle sessizce geri yüklenir. 401 alan istekler tek bir yenilemeyi paylaşır ve tekrarlanır |
| Yetki         | Menü ve düğmeler `GET /me/permissions`'a göre gösterilir. Asıl kontrol sunucudadır. MFA gerektiren yetkiler MFA'sız oturumda listede yoktur, kullanıcıya MFA kurması önerilir                          |
| Hatalar       | Sunucunun `code` alanı kullanıcının dilinde bir mesaja çevrilir (`useErrorMessage`)                                                                                                                    |
| Çeviri        | `src/i18n/locales/{tr,en}.json`. Anahtarlar tip olarak denetlenir, iki dilin aynı anahtarlara sahip olduğu test edilir                                                                                 |
| Tema          | Renkler `src/index.css`'teki belirteçlerdir (açık/koyu). Bileşenler renkleri doğrudan kullanmaz                                                                                                        |
| Testler       | Vitest + Testing Library. API, MSW ile sözleşmedeki biçimde taklit edilir. Bütün uygulama bellek içi router'la çizilip kullanıcı gibi gezilir                                                          |
