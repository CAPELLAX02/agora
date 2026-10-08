# Agora geliştirme komutları. Liste için: make help
#
# Kökte bir .env dosyası varsa (cp .env.example .env) içindeki değişkenler
# tüm komutlara ortam değişkeni olarak aktarılır.

BACKEND := backend
GOOSE   := github.com/pressly/goose/v3/cmd/goose@v3.28.0

-include .env
export

.DEFAULT_GOAL := help

.PHONY: help up down ps logs psql api migrate-up migrate-down migrate-status migration seed \
        test test-unit cover vet fmt check

help: ## Komutları listeler
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

# --- Altyapı -----------------------------------------------------------------

up: ## Docker servislerini başlatır (PostgreSQL, Redis)
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

api: ## API'yi çalıştırır
	go -C $(BACKEND) run ./cmd/api

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

seed: ## Geliştirme seed verisini yükler (tekrar çalıştırılabilir)
	@for f in infra/seed/dev/*.sql; do \
		echo "seed: $$f"; \
		docker compose exec -T postgres psql -q -v ON_ERROR_STOP=1 -U agora -d agora < $$f || exit 1; \
	done

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

fmt: ## Tüm Go dosyalarını biçimlendirir
	gofmt -w $(BACKEND)

check: ## CI ile aynı kontroller: biçim, vet, test
	@test -z "$$(gofmt -l $(BACKEND))" || (echo "biçimlendirilmesi gereken dosyalar:"; gofmt -l $(BACKEND); exit 1)
	go -C $(BACKEND) vet ./...
	go -C $(BACKEND) test -race ./...
