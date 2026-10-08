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
