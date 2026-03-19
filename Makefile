MIGRATIONS_PATH=db/migrations

ifneq (,$(wildcard deploy/.env))
  include deploy/.env
  export
endif

ifneq (,$(wildcard .env))
  include .env
  export
endif

DB_URL=postgresql://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@localhost:$(POSTGRES_PORT)/$(POSTGRES_DB)?sslmode=disable
DB_URL_Docker=postgresql://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@postgres:5432/$(POSTGRES_DB)?sslmode=disable

docker_run:
	docker run --rm \
		--name odoodevtools-server \
		--network odoodevtools_backend \
		-p 8080:8080 \
		-e DB_SOURCE=$(DB_URL_Docker) \
		-e GIN_MODE=release \
		odoodevtools:latest

## ── Docker Compose ───────────────────────────────────────────────

COMPOSE_FILE := deploy/docker-compose.yml
COMPOSE := docker compose --env-file .env -f $(COMPOSE_FILE)

.PHONY: up down restart ps logs logs-f db-shell redis-shell dev-up dev-down

up: ## Start all containers (detached)
	$(COMPOSE) up -d

dev-up: ## Start dev stack with odoo-dev profile
	$(COMPOSE) --profile odoo-dev up -d

dev-down: ## Stop dev stack (odoo-dev profile)
	$(COMPOSE) --profile odoo-dev down

down: ## Stop and remove all containers
	$(COMPOSE) down

down-v: ## Stop containers and remove volumes (fresh start)
	$(COMPOSE) down -v

restart: down up ## Restart all containers

ps: ## Show running containers
	$(COMPOSE) ps

logs: ## Show container logs (last 100 lines)
	$(COMPOSE) logs --tail=100

logs-f: ## Follow container logs live
	$(COMPOSE) logs -f

db-shell: ## Open psql shell in postgres container
	$(COMPOSE) exec postgres psql -U $(POSTGRES_USER) -d $(POSTGRES_DB)

redis-shell: ## Open redis-cli in redis container
	$(COMPOSE) exec redis redis-cli -a $(REDIS_PASSWORD)

## ── Dev Seed ─────────────────────────────────────────────────────

.PHONY: seed-dev-agent

seed-dev-agent: ## Seed a local-dev environment + API key, then patch .env.agent (idempotent)
	@psql "$(DB_URL)" -f scripts/seed_dev_agent.sql -q
	@env_uuid=$$(psql "$(DB_URL)" -tAq -c "SELECT id FROM environments WHERE agent_id='local-dev-agent' LIMIT 1"); \
	if [ -n "$$env_uuid" ] && [ -f .env.agent ]; then \
		sed -i.bak "s|^AGENT_ID=.*|AGENT_ID=$$env_uuid|" .env.agent && rm -f .env.agent.bak; \
		echo "  → AGENT_ID=$$env_uuid written to .env.agent"; \
	fi

## ── Migrations ───────────────────────────────────────────────────

new_migration:
	migrate create -ext sql -dir $(MIGRATIONS_PATH) -seq $(name)

migrate_up:
	migrate -path $(MIGRATIONS_PATH) -database "$(DB_URL)" -verbose up $(steps)

migrate_down:
	migrate -path $(MIGRATIONS_PATH) -database "$(DB_URL)" -verbose down $(steps)

migrate_goto:
	migrate -path $(MIGRATIONS_PATH) -database "$(DB_URL)" -verbose goto $(version)

migrate_force:
	migrate -path $(MIGRATIONS_PATH) -database "$(DB_URL)" -verbose force $(version)

sqlc:
	sqlc generate
generate:
	go generate

swagger:
	swag init --parseDependency -g cmd/server/main.go



GOLANGCI_LINT_VERSION := v2.11.1

## ── Install / Setup ──────────────────────────────────────────────

.PHONY: tools
tools: ## Install golangci-lint (same version as CI)
	@if ! command -v golangci-lint >/dev/null 2>&1 || \
		[ "$$(golangci-lint version --short 2>/dev/null)" != "$(GOLANGCI_LINT_VERSION)" ]; then \
		echo "Installing golangci-lint $(GOLANGCI_LINT_VERSION)..."; \
		go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION); \
	else \
		echo "golangci-lint $(GOLANGCI_LINT_VERSION) already installed"; \
	fi

.PHONY: hooks
hooks: ## Install git pre-push hook
	@cp scripts/pre-push .git/hooks/pre-push
	@chmod +x .git/hooks/pre-push
	@echo "✅ pre-push hook installed"

## ── Lint (mirrors CI exactly) ────────────────────────────────────

.PHONY: lint
lint: ## Run golangci-lint + go vet (same as CI)
	golangci-lint run --timeout 5m
	go vet ./...
	@echo ""
	@echo "✅ Lint passed — safe to push"

.PHONY: lint-fix
lint-fix: ## Run golangci-lint with auto-fix where possible
	golangci-lint run --timeout 5m --fix
	go vet ./...

.PHONY: lint-new
lint-new: ## Only lint uncommitted changes (fast iteration)
	golangci-lint run --timeout 5m --new-from-rev=HEAD~1

## ── Test ─────────────────────────────────────────────────────────

.PHONY: test
test: ## Run unit tests (short mode, no external deps)
	go test -v -race -short ./...

.PHONY: test-integration
test-integration: ## Run all tests including integration
	go test -v -race -coverprofile=coverage.out ./...

## ── Dev (live reload) ────────────────────────────────────────────

.PHONY: dev dev-server dev-agent
dev: migrate_up seed-dev-agent ## Run migrations + seed, then start server + agent with live reload
	@trap 'kill 0' SIGINT SIGTERM; \
	air -c .air.toml & \
	air -c .air.agent.toml & \
	wait

dev-server: ## Run server only with live reload
	air -c .air.toml

dev-agent: ## Run agent only with live reload
	air -c .air.agent.toml

dev-worker: ## Run worker only with live reload
	air -c .air.worker.toml
## ── Build ────────────────────────────────────────────────────────

.PHONY: build build-agent
build: ## Build server binary
	go build -o bin/server ./cmd/server

build-agent: ## Build agent binary
	go build -o bin/agent ./cmd/agent

## ── Format ───────────────────────────────────────────────────────

.PHONY: fmt
fmt: ## Format code
	gofmt -w .
	goimports -w .

.PHONY: tidy
tidy: ## Tidy go.mod
	go mod tidy

## ── Pre-push check (everything CI checks) ───────────────────────

.PHONY: check
check: tidy fmt lint test build ## Full pre-push check (mirrors CI)
	@echo ""
	@echo "════════════════════════════════════════"
	@echo "  ✅ All checks passed — safe to push"
	@echo "════════════════════════════════════════"

## ── Help ─────────────────────────────────────────────────────────

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help

.PHONY: sqlc docker_run new_migration migrate_up migrate_down migrate_goto migrate_force swagger generate lint-new lint-fix check hooks tools up down down-v restart ps logs logs-f db-shell redis-shell dev-up dev-down dev dev-server dev-agent build-agent