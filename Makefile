SHELL := /bin/bash
GO ?= go
LOAD_ENV = set -a; [ -f .env ] && source .env; set +a;

.PHONY: help run test test-db lint vet migrate migrate-status db-up db-down db-logs tidy

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
