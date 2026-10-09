# Agora geliştirme komutları. Liste için: make help
#
# Kökte bir .env dosyası varsa (cp .env.example .env) içindeki değişkenler
# tüm komutlara ortam değişkeni olarak aktarılır.

BACKEND := backend
WEB     := web
# pnpm, corepack ile web/package.json'daki "packageManager" sürümüyle çalışır: ayrıca kurulmaz.
# corepack sürümü çalışma dizinindeki package.json'dan okur, bu yüzden önce web/'e geçilir.
PNPM    := cd $(WEB) && COREPACK_ENABLE_DOWNLOAD_PROMPT=0 corepack pnpm
GOOSE       := github.com/pressly/goose/v3/cmd/goose@v3.28.0
# staticcheck ve govulncheck backend/tools modülünde sabitlenmiştir (go.sum ile doğrulanır).
GOTOOL      := go -C $(BACKEND) tool -modfile=tools/go.mod
REDOCLY     := @redocly/cli@2.53.3

-include .env
export

.DEFAULT_GOAL := help

.PHONY: help up down ps logs psql redis api worker migrate-up migrate-down migrate-status migration seed seed-synthetic \
        test test-unit cover vet lint vuln fmt contract-lint check \
        web-install web-dev web-codegen web-check web-e2e

help: ## Komutları listeler
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

# --- Altyapı -----------------------------------------------------------------

up: ## Docker servislerini başlatır (PostgreSQL, Redis, Mailpit, Prometheus, Grafana)
	docker compose up -d

down: ## Docker servislerini durdurur (veriler volume'larda korunur)
	docker compose down

ps: ## Docker servislerinin durumunu gösterir
	docker compose ps

logs: ## Docker servislerinin log'larını izler
	docker compose logs -f

psql: ## agora veritabanına psql ile bağlanır
	docker compose exec postgres psql -U agora -d agora

# --- Uygulama ----------------------------------------------------------------

redis: ## Redis'e redis-cli ile bağlanır
	docker compose exec redis redis-cli

api: ## API'yi çalıştırır
	go -C $(BACKEND) run ./cmd/api

worker: ## Worker'ı çalıştırır (e-posta outbox'ı, bakım işleri). E-postalar: http://localhost:8025
	go -C $(BACKEND) run ./cmd/worker

# --- Veritabanı migration'ları -----------------------------------------------

migrate-up: ## Bekleyen migration'ları uygular
	go -C $(BACKEND) run ./cmd/migrate up

migrate-down: ## Son uygulanan migration'ı geri alır
	go -C $(BACKEND) run ./cmd/migrate down

migrate-status: ## Migration'ların durumunu listeler
	go -C $(BACKEND) run ./cmd/migrate status

migration: ## Yeni migration dosyası oluşturur: make migration name=create_iam_users
	@test -n "$(name)" || (echo "kullanım: make migration name=<ad>"; exit 1)
	go -C $(BACKEND) run $(GOOSE) -dir migrations -s create $(name) sql

seed: ## Geliştirme seed verisini yükler: organizasyon (SQL) + kullanıcılar (Go). Tekrar çalıştırılabilir
	@for f in infra/seed/dev/*.sql; do \
		echo "seed: $$f"; \
		docker compose exec -T postgres psql -q -v ON_ERROR_STOP=1 -U agora -d agora < $$f || exit 1; \
	done
	go -C $(BACKEND) run ./cmd/seed

seed-synthetic: seed ## Ek olarak ~92 bin öğrenci ve ~10 bin akademisyen üretir (yük testi ve demo için, ~10 sn)
	go -C $(BACKEND) run ./cmd/seed -synthetic

# --- Kalite ------------------------------------------------------------------

test: ## Tüm testleri (Docker gerektiren entegrasyon testleri dahil) race dedektörüyle çalıştırır
	go -C $(BACKEND) test -race ./...

test-unit: ## Sadece birim testlerini çalıştırır (Docker gerekmez)
	go -C $(BACKEND) test -race -short ./...

cover: ## Test kapsama raporunu tarayıcıda açar
	go -C $(BACKEND) test -coverprofile=coverage.out ./...
	go -C $(BACKEND) tool cover -html=coverage.out

vet: ## go vet çalıştırır
	go -C $(BACKEND) vet ./...

lint: ## staticcheck ile statik analiz yapar
	$(GOTOOL) staticcheck ./...

vuln: ## Bağımlılıklardaki bilinen güvenlik açıklarını tarar (govulncheck)
	$(GOTOOL) govulncheck ./...

fmt: ## Tüm Go dosyalarını biçimlendirir
	gofmt -w $(BACKEND)

contract-lint: ## OpenAPI sözleşmesini doğrular (Node.js gerektirir)
	npx --yes $(REDOCLY) lint contracts/openapi.yaml --config contracts/redocly.yaml

check: ## CI ile aynı kontroller: biçim, go.mod, vet, staticcheck, güvenlik, test
	@test -z "$$(gofmt -l $(BACKEND))" || (echo "biçimlendirilmesi gereken dosyalar:"; gofmt -l $(BACKEND); exit 1)
	go -C $(BACKEND) mod tidy -diff
	go -C $(BACKEND) vet ./...
	go -C $(BACKEND)/tools mod tidy -diff
	$(GOTOOL) staticcheck ./...
	$(GOTOOL) govulncheck ./...
	go -C $(BACKEND) test -race ./...

# --- Web ---------------------------------------------------------------------

web-install: ## Web bağımlılıklarını kurar (lockfile'a birebir uyar)
	$(PNPM) install --frozen-lockfile

web-dev: ## Web geliştirme sunucusu: http://localhost:5173 (API isteklerini :8080'e aktarır)
	$(PNPM) dev

web-codegen: ## OpenAPI sözleşmesinden RTK Query istemcisini üretir
	$(PNPM) codegen

web-check: ## Web: biçim, lint, tip kontrolü, test ve build (CI ile aynı)
	$(PNPM) check

web-e2e: ## Uçtan uca testler (Playwright): ayrı bir veritabanı kurar, API, worker ve web'i kendisi başlatır
	$(PNPM) e2e
