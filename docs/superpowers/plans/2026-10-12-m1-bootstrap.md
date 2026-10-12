# Milestone 1 — Bootstrap Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A running Go service with validated configuration, JSON logs, graceful shutdown, a PostgreSQL pool, embedded versioned migrations, `/healthz` and `/readyz`, a rollback-per-test database harness, and a CI workflow.

**Architecture:** Single Go binary (`cmd/server`) wiring `internal/config` → `internal/db` → `internal/web` (chi router). Migrations are SQL files embedded in the binary and applied by `cmd/migrate` (never automatically at server start). Tests talk to a real Postgres through `internal/db/testdb`, each test inside a transaction that is rolled back.

**Tech Stack:** Go 1.27.2 · chi v5.3.2 · pgx v5.11.0 (pgxpool + stdlib adapter) · goose v3.29.0 (Provider API, embedded FS) · testify v1.12.1 · PostgreSQL 17 (docker `postgres:17-alpine`) · golangci-lint v2.14.0 · GitHub Actions.

**Spec:** `docs/ARCHITECTURE.md` (§2–§4, §8), `docs/MVP_PLAN.md` (M1 section), decisions D-012, D-013, D-014, D-022 in `docs/DECISIONS.md`.

## Global Constraints

- Go module path is `kerjalancar`; `go.mod` declares `go 1.27`.
- No secrets in the repository. `.env` is git-ignored; `.env.example` holds local-docker credentials only.
- Error messages and logs never contain `DATABASE_URL` or any password. Request logs never contain query strings.
- Every DB call takes a `context.Context` with a deadline from the caller.
- Migrations run only through `cmd/migrate`; the server verifies connectivity but does not migrate.
- Database tests skip when `TEST_DATABASE_URL` is unset and fail when `REQUIRE_DB=1` (CI sets it).
- Commit after each task with the message given; never commit with failing tests.
- Prerequisite: `go version` prints 1.27.x and `docker compose up -d db` works (see `docs/PROJECT_STATUS.md` for the environment options). If either is missing, stop and report; do not improvise an install.

## Review Focus

1. `DATABASE_URL` with a password that cannot be parsed or reached → the startup error must not echo the password. Test in Task 4 (`TestNewPoolErrorDoesNotLeakPassword`).
2. Postgres accepts the TCP connection but never answers `/readyz`'s ping → the endpoint must return 503 within about 2 s, not hang. Test in Task 3 (`TestReadyzTimesOut`).
3. `PORT=abc`, `PORT=70000`, `LOG_LEVEL=verbose`, `APP_ENV=prod` → one combined, readable error listing every problem, exit code 1. Test in Task 2 (`TestLoadReportsAllErrors`).
4. `SIGTERM` while a request is in flight → that request still completes with 200 and the process exits 0 within the 10 s window. Test in Task 5 (`TestServeFinishesInFlightRequestOnCancel`).
5. A request like `/login?token=abc` → the access log line must contain the path but never the query string. Test in Task 3 (`TestRequestLoggerOmitsQueryString`).

---

### Task 1: Repository scaffolding, local Postgres, Makefile

**Files:**
- Create: `go.mod`
- Create: `docker-compose.yml`
- Create: `docker/postgres/init/01-test-database.sql`
- Create: `.env.example`
- Create: `Makefile`
- Create: `.golangci.yml`
- Create: `docs/DEVELOPMENT.md`
- Modify: `.gitignore` (append Go entries)

**Interfaces:**
- Consumes: nothing.
- Produces: `make run`, `make test`, `make test-db`, `make lint`, `make migrate`, `make migrate-status`, `make db-up`, `make db-down`, `make tidy`; environment variable names `APP_ENV`, `PORT`, `BASE_URL`, `LOG_LEVEL`, `DATABASE_URL`, `TEST_DATABASE_URL`, `REQUIRE_DB`.

- [ ] **Step 1: Create `go.mod`**

```
module kerjalancar

go 1.27

require (
	github.com/go-chi/chi/v5 v5.3.2
	github.com/jackc/pgx/v5 v5.11.0
	github.com/pressly/goose/v3 v3.29.0
	github.com/stretchr/testify v1.12.1
)
```

- [ ] **Step 2: Create `docker-compose.yml`**

```yaml
services:
  db:
    image: postgres:17-alpine
    environment:
      POSTGRES_USER: kerjalancar
      POSTGRES_PASSWORD: kerjalancar # local development only; never reuse elsewhere
      POSTGRES_DB: kerjalancar
    ports:
      - "127.0.0.1:5432:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data
      - ./docker/postgres/init:/docker-entrypoint-initdb.d:ro
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U kerjalancar -d kerjalancar"]
      interval: 5s
      timeout: 3s
      retries: 10

volumes:
  pgdata:
```

- [ ] **Step 3: Create `docker/postgres/init/01-test-database.sql`**

```sql
-- Runs once when the volume is first created. Creates the test database used by `make test`.
CREATE DATABASE kerjalancar_test;
```

- [ ] **Step 4: Create `.env.example`**

```
# Copy to .env (git-ignored). Values below match docker-compose.yml and are for local development only.
APP_ENV=development
PORT=8080
BASE_URL=http://localhost:8080
LOG_LEVEL=debug
DATABASE_URL=postgres://kerjalancar:kerjalancar@localhost:5432/kerjalancar?sslmode=disable
TEST_DATABASE_URL=postgres://kerjalancar:kerjalancar@localhost:5432/kerjalancar_test?sslmode=disable
```

- [ ] **Step 5: Create `Makefile`**

```make
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
```

- [ ] **Step 6: Create `.golangci.yml`**

```yaml
version: "2"
linters:
  default: standard
  enable:
    - gosec
    - bodyclose
    - noctx
    - misspell
formatters:
  enable:
    - gofmt
    - goimports
```

- [ ] **Step 7: Append to `.gitignore`**

```
# Go
/bin/
*.test
coverage.out
/tmp/

# local toolchains and assets
/.toolchain/
node_modules/
```

- [ ] **Step 8: Create `docs/DEVELOPMENT.md`**

```markdown
# Development setup

Prerequisites: Go 1.27.x on PATH, Docker Desktop running (for Postgres), GNU make.

1. `cp .env.example .env`
2. `make db-up` — starts Postgres 17 on 127.0.0.1:5432 and creates `kerjalancar` and `kerjalancar_test`
3. `make migrate` — applies migrations to `kerjalancar`
4. `make run` — serves http://localhost:8080 ; check `curl -i localhost:8080/healthz` and `/readyz`
5. `make test` — unit tests plus database tests (the test database is migrated automatically by the test harness)
6. `make lint` — `go vet` and, if installed, `golangci-lint`

Environment variables: see `.env.example`. `APP_ENV` is `development`, `test`, or `production`; `BASE_URL` must be https in production.

Troubleshooting:
- `make test` skips database tests: `TEST_DATABASE_URL` is not set; copy `.env.example` to `.env`.
- `connection refused` on 5432: `make db-up`, then `docker compose ps` until the `db` service is healthy.
- Tests that bind local ports fail with "operation not permitted" inside the Claude Code sandbox: run `make test` from your own terminal, or set `sandbox.network.allowLocalBinding: true`.
```

- [ ] **Step 9: Verify the scaffold**

Run: `docker compose config >/dev/null && echo compose-ok`
Expected: `compose-ok`

Run: `make db-up && sleep 8 && docker compose ps`
Expected: service `db` with status `healthy`

Run: `go mod download && go mod verify`
Expected: `all modules verified` (creates `go.sum`)

- [ ] **Step 10: Commit**

```bash
git add go.mod go.sum docker-compose.yml docker/postgres/init/01-test-database.sql .env.example Makefile .golangci.yml .gitignore docs/DEVELOPMENT.md
git commit -m "chore: bootstrap module, local Postgres, Makefile, lint config, dev docs"
```

---

### Task 2: Configuration package

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `type Config struct { Env string; Port int; DatabaseURL string; LogLevel slog.Level; BaseURL string }`, `type Getter func(key string) string`, `func Load(get Getter) (Config, error)`.

- [ ] **Step 1: Write the failing tests**

```go
package config_test

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kerjalancar/internal/config"
)

func getter(m map[string]string) config.Getter {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := config.Load(getter(map[string]string{
		"DATABASE_URL": "postgres://u:p@localhost:5432/db",
	}))
	require.NoError(t, err)
	assert.Equal(t, "development", cfg.Env)
	assert.Equal(t, 8080, cfg.Port)
	assert.Equal(t, "http://localhost:8080", cfg.BaseURL)
	assert.Equal(t, slog.LevelInfo, cfg.LogLevel)
	assert.Equal(t, "postgres://u:p@localhost:5432/db", cfg.DatabaseURL)
}

func TestLoadParsesValues(t *testing.T) {
	cfg, err := config.Load(getter(map[string]string{
		"APP_ENV":      "production",
		"PORT":         "9090",
		"BASE_URL":     "https://app.kerjalancar.id/",
		"LOG_LEVEL":    "warn",
		"DATABASE_URL": "postgres://u:p@db:5432/app",
	}))
	require.NoError(t, err)
	assert.Equal(t, "production", cfg.Env)
	assert.Equal(t, 9090, cfg.Port)
	assert.Equal(t, "https://app.kerjalancar.id", cfg.BaseURL, "trailing slash removed")
	assert.Equal(t, slog.LevelWarn, cfg.LogLevel)
}

func TestLoadReportsAllErrors(t *testing.T) {
	_, err := config.Load(getter(map[string]string{
		"APP_ENV":   "prod",
		"PORT":      "abc",
		"LOG_LEVEL": "verbose",
	}))
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "APP_ENV")
	assert.Contains(t, msg, "PORT")
	assert.Contains(t, msg, "LOG_LEVEL")
	assert.Contains(t, msg, "DATABASE_URL")
}

func TestLoadRejectsPortOutOfRange(t *testing.T) {
	_, err := config.Load(getter(map[string]string{"PORT": "70000", "DATABASE_URL": "x"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PORT")
}

func TestLoadRequiresHTTPSInProduction(t *testing.T) {
	_, err := config.Load(getter(map[string]string{
		"APP_ENV":      "production",
		"BASE_URL":     "http://example.com",
		"DATABASE_URL": "x",
	}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "https")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -run 'TestLoad' -v`
Expected: FAIL, compile error `undefined: config.Load` (package does not exist yet)

- [ ] **Step 3: Write the implementation**

```go
// Package config loads and validates process configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
)

// Config is the validated process configuration.
type Config struct {
	Env         string // development, test, or production
	Port        int
	DatabaseURL string
	LogLevel    slog.Level
	BaseURL     string // no trailing slash
}

// Getter returns the value of an environment variable, or "" when unset.
// os.Getenv satisfies it; tests pass a map-backed function.
type Getter func(key string) string

// Load reads every variable, validates all of them, and returns every problem
// joined into one error so the operator fixes them in one go.
func Load(get Getter) (Config, error) {
	var errs []error

	cfg := Config{
		Env:     strings.TrimSpace(or(get("APP_ENV"), "development")),
		BaseURL: strings.TrimRight(strings.TrimSpace(or(get("BASE_URL"), "http://localhost:8080")), "/"),
	}

	switch cfg.Env {
	case "development", "test", "production":
	default:
		errs = append(errs, fmt.Errorf("APP_ENV must be development, test, or production (got %q)", cfg.Env))
	}

	rawPort := or(get("PORT"), "8080")
	port, err := strconv.Atoi(strings.TrimSpace(rawPort))
	if err != nil || port < 1 || port > 65535 {
		errs = append(errs, fmt.Errorf("PORT must be a number between 1 and 65535 (got %q)", rawPort))
	}
	cfg.Port = port

	cfg.DatabaseURL = strings.TrimSpace(get("DATABASE_URL"))
	if cfg.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}

	rawLevel := or(get("LOG_LEVEL"), "info")
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(strings.TrimSpace(rawLevel))); err != nil {
		errs = append(errs, fmt.Errorf("LOG_LEVEL must be debug, info, warn, or error (got %q)", rawLevel))
	}
	cfg.LogLevel = lvl

	if cfg.Env == "production" && !strings.HasPrefix(cfg.BaseURL, "https://") {
		errs = append(errs, errors.New("BASE_URL must use https in production"))
	}

	return cfg, errors.Join(errs...)
}

func or(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/ -v`
Expected: PASS for all five tests

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat(config): load and validate environment configuration"
```

---

### Task 3: Router, request logging, health endpoints

**Files:**
- Create: `internal/web/router.go`
- Create: `internal/web/middleware.go`
- Test: `internal/web/router_test.go`
- Test: `internal/web/middleware_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks (uses a `Pinger` interface so it compiles before the DB package exists).
- Produces: `type Pinger interface { Ping(ctx context.Context) error }`, `type Deps struct { Logger *slog.Logger; DB Pinger }`, `func NewRouter(d Deps) http.Handler`. Routes `GET /healthz`, `GET /readyz`.

- [ ] **Step 1: Write the failing tests**

`internal/web/router_test.go`:

```go
package web_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"kerjalancar/internal/web"
)

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

// blockingPinger never answers until the context is cancelled.
type blockingPinger struct{}

func (blockingPinger) Ping(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func newRouter(db web.Pinger) http.Handler {
	return web.NewRouter(web.Deps{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		DB:     db,
	})
}

func TestHealthz(t *testing.T) {
	rec := httptest.NewRecorder()
	newRouter(fakePinger{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json; charset=utf-8", rec.Header().Get("Content-Type"))
	assert.JSONEq(t, `{"status":"ok"}`, rec.Body.String())
}

func TestReadyzOK(t *testing.T) {
	rec := httptest.NewRecorder()
	newRouter(fakePinger{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"status":"ok"}`, rec.Body.String())
}

func TestReadyzReportsDatabaseFailure(t *testing.T) {
	rec := httptest.NewRecorder()
	newRouter(fakePinger{err: errors.New("connection refused")}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.JSONEq(t, `{"status":"unavailable","check":"database"}`, rec.Body.String())
}

func TestReadyzTimesOut(t *testing.T) {
	start := time.Now()
	rec := httptest.NewRecorder()
	newRouter(blockingPinger{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Less(t, time.Since(start), 3*time.Second, "readyz must give up after its 2s budget")
}

func TestUnknownRouteIs404(t *testing.T) {
	rec := httptest.NewRecorder()
	newRouter(fakePinger{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
```

`internal/web/middleware_test.go`:

```go
package web_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"kerjalancar/internal/web"
)

func TestRequestLoggerOmitsQueryString(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	h := web.NewRouter(web.Deps{Logger: logger, DB: fakePinger{}})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope?token=supersecret", nil))

	out := buf.String()
	assert.Contains(t, out, `"path":"/nope"`)
	assert.Contains(t, out, `"status":404`)
	assert.NotContains(t, out, "supersecret")
}

func TestRequestLoggerSkipsHealthEndpoints(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	h := web.NewRouter(web.Deps{Logger: logger, DB: fakePinger{}})

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/healthz", nil))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/readyz", nil))

	assert.Empty(t, buf.String(), "health probes must not flood the logs")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/web/ -v`
Expected: FAIL, compile error `undefined: web.NewRouter`

- [ ] **Step 3: Write `internal/web/router.go`**

```go
// Package web wires HTTP transport: router, middleware, handlers, templates.
// Handlers stay thin: parse, authorize, call a service, render.
package web

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Pinger is the only thing the router needs from the database layer today.
// *pgxpool.Pool satisfies it.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Deps are the collaborators the router needs. Keep this struct small.
type Deps struct {
	Logger *slog.Logger
	DB     Pinger
}

const (
	requestTimeout = 30 * time.Second
	readyzTimeout  = 2 * time.Second
)

// NewRouter builds the HTTP handler for the whole application.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(requestLogger(d.Logger))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(requestTimeout))

	r.Get("/healthz", handleHealthz)
	r.Get("/readyz", handleReadyz(d.DB))

	return r
}

// handleHealthz answers "the process is up". It must not touch dependencies.
func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleReadyz answers "the process can serve traffic": the database responds
// within readyzTimeout.
func handleReadyz(db Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), readyzTimeout)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status": "unavailable",
				"check":  "database",
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
```

- [ ] **Step 4: Write `internal/web/middleware.go`**

```go
package web

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// requestLogger writes one structured line per request. It logs the path but
// never the query string, because query strings will carry share tokens later.
func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
				next.ServeHTTP(w, r)
				return
			}
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			log.LogAttrs(r.Context(), slog.LevelInfo, "http request",
				slog.String("request_id", middleware.GetReqID(r.Context())),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", ww.Status()),
				slog.Int("bytes", ww.BytesWritten()),
				slog.Duration("duration", time.Since(start)),
				slog.String("ip", r.RemoteAddr),
			)
		})
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/web/ -race -v`
Expected: PASS for all seven tests; `TestReadyzTimesOut` takes about 2 s

- [ ] **Step 6: Commit**

```bash
git add internal/web/
git commit -m "feat(web): chi router with request logging, healthz and readyz"
```

---

### Task 4: Database pool, embedded migrations, test harness

**Files:**
- Create: `internal/db/pool.go`
- Create: `internal/db/migrate.go`
- Create: `internal/db/migrations/00001_init.sql`
- Create: `internal/db/testdb/testdb.go`
- Test: `internal/db/pool_test.go`
- Test: `internal/db/migrate_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error)`, `func Migrate(ctx context.Context, pool *pgxpool.Pool) ([]int64, error)`, `func MigrationStatus(ctx context.Context, pool *pgxpool.Pool) ([]*goose.MigrationStatus, error)`, `func testdb.Pool(t *testing.T) *pgxpool.Pool`, `func testdb.Tx(t *testing.T) pgx.Tx`.

Before coding, confirm the goose Provider API names against the pinned version:
Run: `go doc github.com/pressly/goose/v3 NewProvider && go doc github.com/pressly/goose/v3 Provider.Up && go doc github.com/pressly/goose/v3 MigrationResult && go doc github.com/jackc/pgx/v5/stdlib OpenDBFromPool`
Expected: `NewProvider(dialect Dialect, db *sql.DB, fsys fs.FS, opts ...ProviderOption)`, `Up(ctx) ([]*MigrationResult, error)`, `MigrationResult.Source.Version int64`, `OpenDBFromPool(pool *pgxpool.Pool, opts ...OptionOpenDB) *sql.DB`. If a name differs, adjust the code below to the documented name; do not change behavior.

- [ ] **Step 1: Write the failing tests**

`internal/db/pool_test.go`:

```go
package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kerjalancar/internal/db"
)

func TestNewPoolErrorDoesNotLeakPassword(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	// Port 1 is closed on every developer machine; the connect must fail fast.
	_, err := db.NewPool(ctx, "postgres://app:hunter2secret@127.0.0.1:1/app?sslmode=disable")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "hunter2secret")
}

func TestNewPoolRejectsGarbageURL(t *testing.T) {
	_, err := db.NewPool(context.Background(), "not a url at all with password=hunter2secret")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "hunter2secret")
}
```

`internal/db/migrate_test.go`:

```go
package db_test

import (
	"context"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kerjalancar/internal/db"
	"kerjalancar/internal/db/testdb"
)

func TestMigrateIsIdempotentAndAppliesInit(t *testing.T) {
	pool := testdb.Pool(t) // first call applies all migrations
	ctx := context.Background()

	again, err := db.Migrate(ctx, pool)
	require.NoError(t, err)
	assert.Empty(t, again, "second run must apply nothing")

	var hasCitext bool
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'citext')`).Scan(&hasCitext))
	assert.True(t, hasCitext, "00001_init must install citext")

	statuses, err := db.MigrationStatus(ctx, pool)
	require.NoError(t, err)
	require.NotEmpty(t, statuses)
	for _, s := range statuses {
		assert.Equal(t, goose.StateApplied, s.State, "migration %d", s.Source.Version)
	}
}

func TestTxRollsBackBetweenTests(t *testing.T) {
	ctx := context.Background()
	tx := testdb.Tx(t)
	_, err := tx.Exec(ctx, `CREATE TEMP TABLE scratch (n int)`) // temp table lives only in this tx
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `INSERT INTO scratch VALUES (1)`)
	require.NoError(t, err)

	var n int
	require.NoError(t, tx.QueryRow(ctx, `SELECT count(*) FROM scratch`).Scan(&n))
	assert.Equal(t, 1, n)
	// Cleanup rolls back; the next test sees no scratch table. Nothing else to assert here.
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/db/... -v`
Expected: FAIL, compile errors `undefined: db.NewPool`, `undefined: testdb`

- [ ] **Step 3: Write `internal/db/pool.go`**

```go
// Package db owns the connection pool, migrations, and shared SQL helpers.
package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool opens a pgx pool and verifies connectivity with a bounded ping.
// Errors wrap pgx errors, which redact passwords; the URL is never echoed here.
func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	cfg.MaxConns = 10
	cfg.MinConns = 1
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.HealthCheckPeriod = 30 * time.Second
	cfg.ConnConfig.ConnectTimeout = 5 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}
```

- [ ] **Step 4: Write `internal/db/migrations/00001_init.sql`**

```sql
-- +goose Up
-- citext gives case-insensitive text for e-mail addresses (used from M2).
CREATE EXTENSION IF NOT EXISTS citext;

-- +goose Down
DROP EXTENSION IF EXISTS citext;
```

- [ ] **Step 5: Write `internal/db/migrate.go`**

```go
package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate applies every pending migration and returns the versions it applied
// (empty when the database is already current).
func Migrate(ctx context.Context, pool *pgxpool.Pool) ([]int64, error) {
	provider, closeFn, err := newProvider(pool)
	if err != nil {
		return nil, err
	}
	defer closeFn()

	results, err := provider.Up(ctx)
	if err != nil {
		return nil, fmt.Errorf("migrate up: %w", err)
	}
	versions := make([]int64, 0, len(results))
	for _, r := range results {
		versions = append(versions, r.Source.Version)
	}
	return versions, nil
}

// MigrationStatus lists every known migration with its applied state.
func MigrationStatus(ctx context.Context, pool *pgxpool.Pool) ([]*goose.MigrationStatus, error) {
	provider, closeFn, err := newProvider(pool)
	if err != nil {
		return nil, err
	}
	defer closeFn()

	statuses, err := provider.Status(ctx)
	if err != nil {
		return nil, fmt.Errorf("migration status: %w", err)
	}
	return statuses, nil
}

// newProvider adapts the pgx pool to database/sql for goose. Closing the
// returned *sql.DB does not close the pool (see pgx stdlib.OpenDBFromPool docs).
func newProvider(pool *pgxpool.Pool) (*goose.Provider, func(), error) {
	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return nil, nil, fmt.Errorf("migrations fs: %w", err)
	}
	sqlDB := stdlib.OpenDBFromPool(pool)
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, sub)
	if err != nil {
		_ = sqlDB.Close()
		return nil, nil, fmt.Errorf("goose provider: %w", err)
	}
	return provider, func() { _ = sqlDB.Close() }, nil
}
```

- [ ] **Step 6: Write `internal/db/testdb/testdb.go`**

```go
// Package testdb gives tests a migrated PostgreSQL pool and per-test transactions.
package testdb

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"kerjalancar/internal/db"
)

var (
	once    sync.Once
	pool    *pgxpool.Pool
	poolErr error
)

// Pool returns a process-wide pool connected to TEST_DATABASE_URL with all
// migrations applied. Tests skip when the variable is unset; with REQUIRE_DB=1
// (CI) they fail instead, so a misconfigured CI cannot pass by skipping.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		if os.Getenv("REQUIRE_DB") == "1" {
			t.Fatal("TEST_DATABASE_URL is required because REQUIRE_DB=1")
		}
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		pool, poolErr = db.NewPool(ctx, url)
		if poolErr != nil {
			return
		}
		_, poolErr = db.Migrate(ctx, pool)
	})
	if poolErr != nil {
		t.Fatalf("test database: %v", poolErr)
	}
	return pool
}

// Tx begins a transaction on the test pool and rolls it back when the test
// ends, so tests never see each other's rows.
func Tx(t *testing.T) pgx.Tx {
	t.Helper()
	tx, err := Pool(t).Begin(context.Background())
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	return tx
}
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go mod tidy && set -a && source .env && set +a && go test ./internal/db/... -race -v`
Expected: PASS for all four tests. `TestNewPoolErrorDoesNotLeakPassword` finishes in well under 8 s.

Run: `go test ./internal/db/... -run TestMigrate -v` with `TEST_DATABASE_URL` unset:
`env -u TEST_DATABASE_URL go test ./internal/db/ -run TestMigrate -v`
Expected: `--- SKIP` with the message "TEST_DATABASE_URL not set"

- [ ] **Step 8: Commit**

```bash
git add internal/db/ go.mod go.sum
git commit -m "feat(db): pgx pool, embedded goose migrations, test harness with rollback per test"
```

---

### Task 5: Graceful server, `cmd/server`, `cmd/migrate`

**Files:**
- Create: `internal/web/server.go`
- Create: `cmd/server/main.go`
- Create: `cmd/migrate/main.go`
- Test: `internal/web/server_test.go`

**Interfaces:**
- Consumes: `config.Load`, `db.NewPool`, `db.Migrate`, `db.MigrationStatus`, `web.NewRouter`, `web.Deps`.
- Produces: `func Serve(ctx context.Context, srv *http.Server, ln net.Listener, shutdownTimeout time.Duration) error`; binaries `cmd/server` and `cmd/migrate` (`up`, `status`).

- [ ] **Step 1: Write the failing test**

```go
package web_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kerjalancar/internal/web"
)

// Binds a local port. Inside the Claude Code sandbox this needs
// sandbox.network.allowLocalBinding: true; otherwise run from a normal terminal.
func TestServeFinishesInFlightRequestOnCancel(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	started := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/slow", func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "done")
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: time.Second}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	var serveErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		serveErr = web.Serve(ctx, srv, ln, 5*time.Second)
	}()

	var (
		resp    *http.Response
		respErr error
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		resp, respErr = http.Get("http://" + ln.Addr().String() + "/slow") //nolint:noctx // test client
	}()

	<-started
	cancel() // simulate SIGTERM while /slow is in flight
	wg.Wait()

	require.NoError(t, serveErr)
	require.NoError(t, respErr)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "done", string(body))
}

func TestServeReturnsListenerErrors(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	_ = ln.Close() // a closed listener makes Serve fail immediately

	err = web.Serve(context.Background(), &http.Server{Handler: http.NotFoundHandler(), ReadHeaderTimeout: time.Second}, ln, time.Second)
	require.Error(t, err)
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/web/ -run TestServe -v`
Expected: FAIL, compile error `undefined: web.Serve`

- [ ] **Step 3: Write `internal/web/server.go`**

```go
package web

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

// Serve runs srv on ln until ctx is cancelled, then shuts down gracefully:
// in-flight requests may finish within shutdownTimeout, new connections are refused.
func Serve(ctx context.Context, srv *http.Server, ln net.Listener, shutdownTimeout time.Duration) error {
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	<-errCh // Serve returns http.ErrServerClosed once Shutdown completes
	return nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/web/ -run TestServe -race -v`
Expected: PASS for both tests

- [ ] **Step 5: Write `cmd/server/main.go`**

```go
// Command server runs the KerjaLancar web application.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"kerjalancar/internal/config"
	"kerjalancar/internal/db"
	"kerjalancar/internal/web"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "kerjalancar:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer pool.Close()

	addr := net.JoinHostPort("", strconv.Itoa(cfg.Port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}

	srv := &http.Server{
		Handler:           web.NewRouter(web.Deps{Logger: logger, DB: pool}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	logger.Info("server listening", slog.String("addr", ln.Addr().String()), slog.String("env", cfg.Env))
	err = web.Serve(ctx, srv, ln, 10*time.Second)
	logger.Info("server stopped")
	return err
}
```

- [ ] **Step 6: Write `cmd/migrate/main.go`**

```go
// Command migrate applies or reports database migrations. Usage: migrate up|status
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"kerjalancar/internal/config"
	"kerjalancar/internal/db"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 1 || (args[0] != "up" && args[0] != "status") {
		return fmt.Errorf("usage: migrate up|status")
	}
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer pool.Close()

	switch args[0] {
	case "up":
		versions, err := db.Migrate(ctx, pool)
		if err != nil {
			return err
		}
		if len(versions) == 0 {
			fmt.Println("database is up to date")
			return nil
		}
		for _, v := range versions {
			fmt.Printf("applied %d\n", v)
		}
	case "status":
		statuses, err := db.MigrationStatus(ctx, pool)
		if err != nil {
			return err
		}
		for _, s := range statuses {
			applied := "pending"
			if !s.AppliedAt.IsZero() {
				applied = s.AppliedAt.UTC().Format(time.RFC3339)
			}
			fmt.Printf("%-8d %-10s %s\n", s.Source.Version, s.State, applied)
		}
	}
	return nil
}
```

- [ ] **Step 7: Verify the binaries end to end**

Run: `go build ./... && go vet ./...`
Expected: no output (success)

Run: `make migrate`
Expected: `applied 1` on first run; `database is up to date` on a second run

Run: `make migrate-status`
Expected: one line for version 1 with state `applied`

Run (two terminals, or background the first): `make run` then
`curl -s -i localhost:8080/healthz | head -1 && curl -s localhost:8080/readyz`
Expected: `HTTP/1.1 200 OK` and `{"status":"ok"}`

Run: `make db-down && curl -s -o /dev/null -w '%{http_code} %{time_total}s\n' localhost:8080/readyz; make db-up`
Expected: `503` and a time under 3 s (the pool's reconnect attempt is bounded by the 2 s readyz budget)

Run: `DATABASE_URL= go run ./cmd/server; echo "exit=$?"`
Expected: `kerjalancar: DATABASE_URL is required` and `exit=1`

Run: `PORT=abc go run ./cmd/server; echo "exit=$?"` (with `.env` sourced)
Expected: one line mentioning `PORT must be a number between 1 and 65535 (got "abc")` and `exit=1`

Run: start `make run`, then in another terminal `kill -TERM $(pgrep -f 'cmd/server')`
Expected: log lines `shutting down`/`server stopped` and a clean exit (`echo $?` → 0 in the first terminal)

- [ ] **Step 8: Commit**

```bash
git add internal/web/server.go internal/web/server_test.go cmd/
git commit -m "feat: server with graceful shutdown and migrate command"
```

---

### Task 6: Continuous integration workflow

**Files:**
- Create: `.github/workflows/ci.yml`

**Interfaces:**
- Consumes: `make`-free commands so CI does not depend on the Makefile; environment variables `TEST_DATABASE_URL`, `REQUIRE_DB`.
- Produces: a workflow named `ci` on push to `main` and on pull requests.

- [ ] **Step 1: Create `.github/workflows/ci.yml`**

```yaml
name: ci

on:
  push:
    branches: [main]
  pull_request:

permissions:
  contents: read

jobs:
  test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:17-alpine
        env:
          POSTGRES_USER: kerjalancar
          POSTGRES_PASSWORD: kerjalancar
          POSTGRES_DB: kerjalancar_test
        ports:
          - 5432:5432
        options: >-
          --health-cmd "pg_isready -U kerjalancar -d kerjalancar_test"
          --health-interval 5s
          --health-timeout 3s
          --health-retries 10
    env:
      TEST_DATABASE_URL: postgres://kerjalancar:kerjalancar@localhost:5432/kerjalancar_test?sslmode=disable
      REQUIRE_DB: "1"
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
          cache: true
      - run: go mod download
      - run: go vet ./...
      - uses: golangci/golangci-lint-action@v8
        with:
          version: v2.14.0
      - run: go test -race -count=1 ./...
      - run: go run golang.org/x/vuln/cmd/govulncheck@latest ./...
```

- [ ] **Step 2: Validate the YAML locally**

Run: `python3 -c 'import yaml,sys; yaml.safe_load(open(".github/workflows/ci.yml")); print("yaml-ok")'`
Expected: `yaml-ok` (if PyYAML is missing, `ruby -ryaml -e 'YAML.load_file(".github/workflows/ci.yml"); puts "yaml-ok"'`)

The workflow itself can only be exercised after the founder creates the GitHub repository and pushes (external action, founder's call). Record "CI: authored, not yet run" in `PROJECT_STATUS.md` until the first green run; bump the action versions if GitHub warns they are deprecated.

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: vet, lint, race tests against Postgres, govulncheck"
```

---

### Task 7: Milestone close-out

**Files:**
- Modify: `docs/PROJECT_STATUS.md`
- Modify: `docs/MVP_PLAN.md` (tick M1 acceptance criteria that passed; note anything that did not)

- [ ] **Step 1: Run the full suite one last time and capture the output**

Run: `make lint && make test-db 2>&1 | tail -20`
Expected: lint clean; `ok  kerjalancar/internal/config`, `ok  kerjalancar/internal/db`, `ok  kerjalancar/internal/web` with no SKIP lines

- [ ] **Step 2: Update `docs/PROJECT_STATUS.md`**

Fill "Tests executed" with the exact commands and the pasted summary lines; list M1 acceptance criteria with pass/fail; set "Next three actions" to M2's first tasks.

- [ ] **Step 3: Commit**

```bash
git add docs/PROJECT_STATUS.md docs/MVP_PLAN.md
git commit -m "docs: close Milestone 1 with test results"
```

---

## Self-review notes

- Spec coverage: config ✔ (Task 2), structured logging ✔ (Task 3), graceful shutdown and timeouts ✔ (Task 5), context-aware DB calls ✔ (Tasks 3–5), migrations ✔ (Task 4), health and readiness ✔ (Task 3), test harness ✔ (Task 4), CI ✔ (Task 6), dev docs ✔ (Task 1). Dockerfile intentionally deferred to M7 per `MVP_PLAN.md`.
- Type consistency: `web.Pinger`, `web.Deps`, `web.NewRouter`, `web.Serve`, `db.NewPool`, `db.Migrate`, `db.MigrationStatus`, `testdb.Pool`, `testdb.Tx`, `config.Load` are used with the same signatures in every task.
- Review Focus: each of the five items has a named test in the owning task.
- Known verification point: goose Provider names are checked with `go doc` at the start of Task 4 against the pinned v3.29.0.
