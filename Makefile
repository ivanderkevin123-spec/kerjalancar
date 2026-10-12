SHELL := /bin/bash
GO ?= go
TAILWIND ?= .toolchain/bin/tailwindcss
TAILWIND_VERSION ?= v4.3.3
TAILWIND_PLATFORM ?= macos-arm64
LOAD_ENV = set -a; [ -f .env ] && source .env; set +a;

.PHONY: help run build test test-db lint vet migrate migrate-status db-up db-down db-logs tidy css css-tool css-watch

help: ## Show targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-16s %s\n", $$1, $$2}'

run: ## Run the server (loads .env if present)
	@$(LOAD_ENV) $(GO) run ./cmd/server

test: ## Unit and database tests (database tests skip when TEST_DATABASE_URL is unset)
	@$(LOAD_ENV) $(GO) test -race -count=1 ./...

test-db: ## Same as test but fails if the database is not reachable (used in CI)
	@$(LOAD_ENV) REQUIRE_DB=1 $(GO) test -race -count=1 ./...

vet: ## go vet
	$(GO) vet ./...

lint: vet ## golangci-lint if installed, otherwise vet only
	@if command -v golangci-lint >/dev/null 2>&1; then golangci-lint run ./...; else echo "golangci-lint not installed; ran go vet only"; fi

migrate: ## Apply pending migrations to DATABASE_URL
	@$(LOAD_ENV) $(GO) run ./cmd/migrate up

migrate-status: ## Show migration status for DATABASE_URL
	@$(LOAD_ENV) $(GO) run ./cmd/migrate status

db-up: ## Start local Postgres (docker compose)
	docker compose up -d db

db-down: ## Stop local Postgres (data volume is kept)
	docker compose down

db-logs: ## Tail Postgres logs
	docker compose logs -f db

tidy: ## go mod tidy
	$(GO) mod tidy

build: ## Build binaries into bin/
	$(GO) build -o bin/server ./cmd/server && $(GO) build -o bin/migrate ./cmd/migrate

css-tool: ## Download the Tailwind v4 standalone CLI into .toolchain/bin (set TAILWIND_PLATFORM=linux-x64 on Linux)
	mkdir -p .toolchain/bin && curl -fL --http1.1 -o $(TAILWIND) https://github.com/tailwindlabs/tailwindcss/releases/download/$(TAILWIND_VERSION)/tailwindcss-$(TAILWIND_PLATFORM) && chmod +x $(TAILWIND)

css: ## Build internal/web/static/app.css from static/src/app.css (committed; rerun after template changes)
	$(TAILWIND) -i internal/web/static/src/app.css -o internal/web/static/app.css --minify

css-watch: ## Rebuild CSS on template changes
	$(TAILWIND) -i internal/web/static/src/app.css -o internal/web/static/app.css --watch
