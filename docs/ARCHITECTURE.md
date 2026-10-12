# Architecture (DRAFT, pending decision D-001)

This is the proposed architecture for Phase B review. It will be rewritten into the final document once the stack decision is approved and Gate G1 passes.

## 1. Stack decision

The brief lists Golang alongside Hotwire, Active Job, and RSpec/Minitest. The last three are Ruby on Rails components, so the brief mixes two stacks. Options:

| Option | Delivery speed | Reliability / maintainability | Fit with founder | Notes |
|---|---|---|---|---|
| **1. Go + server-rendered HTML + Turbo/Stimulus + Tailwind + PostgreSQL (recommended)** | Good. CRUD is more verbose than Rails, but no learning curve for the founder | Single static binary, small dependency tree, strong typing, easy to test | Founder is a Go engineer; Go toolchains already in use | Hotwire (Turbo + Stimulus) is plain JavaScript and works with any backend that returns HTML |
| 2. Ruby on Rails 8 + Hotwire + Solid Queue + Minitest | Fastest for forms/CRUD/PDF if you know Rails | Mature, batteries included | Ruby not installed; founder would learn Rails while validating a market | Matches the brief's vocabulary literally |
| 3. Go + htmx instead of Turbo/Stimulus | Same as 1 | Same as 1 | Slightly simpler mental model | Equivalent; Turbo Drive gives app-like navigation with zero custom JS, so we keep Hotwire as asked |

**Recommendation: Option 1.** Rationale: the founder's expertise is the scarcest resource; a modular Go monolith is boring and reliable; everything in the MVP is forms, lists, and a few computed views. Rails' advantages (generators, Active Job, PDF gems) are not decisive for this scope.

Proposed components (versions to be confirmed in Phase B against the hosting platform):

- Go 1.27.x (Homebrew reports 1.27.1 stable on 2026-10-12; the machine currently has 1.17.5, see D-004)
- `net/http` with Go 1.22+ pattern routing; `html/template`; `log/slog` JSON logs
- Turbo and Stimulus vendored as pinned static files (no Node build step); Tailwind CSS standalone CLI binary
- PostgreSQL 16 or 17; `pgx/v5`; `sqlc` for typed queries; `goose` for migrations
- Sessions server-side in Postgres (`alexedwards/scs`), passwords with Argon2id (`golang.org/x/crypto/argon2`), CSRF via Go 1.25+ `net/http` CrossOriginProtection (verify) with a token fallback for older browsers if needed
- Rate limiting in-process (single instance for pilots); structured errors; Sentry Go SDK or GlitchTip for error monitoring (free tier; wiring needs approval as an external service)
- Background jobs: **none in v1**. Reminder lists are queries ("due today", "overdue"). If daily digests are validated, add River (Postgres-backed queue), not Redis
- PDF: **none in v1**. Quotation and invoice have a public share page with print CSS; the browser's "Save as PDF" covers the need. Pure-Go PDF generation (e.g., maroto) only if pilots ask
- Tests: standard `testing` + `testify`; DB tests against a real Postgres (local or Docker) with per-test transactions; `-race`; `golangci-lint`; `govulncheck`
- CI: GitHub Actions with a Postgres service container
- Hosting (decision D-005, no spend yet): Fly.io (sin), Railway (Singapore), or Render (Singapore) with managed Postgres; Docker image; daily automated backups plus a tested restore script

## 2. Modular monolith layout

```
cmd/server/            main: config, DB pool, router, graceful shutdown
internal/
  auth/                users, sessions, password hashing, login rate limit
  workspace/           workspaces, memberships, roles (owner, staff), settings (time zone, currency copy)
  customer/            customers and contact notes
  lead/                leads, stages, activities
  quotation/           quotations, items, totals, share tokens, status
  invoice/             invoices, items, payments (DP / cicilan / pelunasan), balances
  followup/            tasks, due lists, completion
  template/            WhatsApp message templates, placeholder rendering, wa.me links
  dashboard/           read models for pipeline, receivables, today's list
  audit/               append-only activity events
  money/               integer rupiah, quantity scale, rounding, formatting "Rp1.250.000"
  db/                  sqlc output, migrations, test helpers
  web/                 HTTP handlers, middleware, templates, static assets, Indonesian copy
```

Each package exposes a small service API and owns its SQL. Packages do not import each other's internals; cross-module reads go through explicit functions (e.g., `invoice` asks `customer` for a name, not the table). Handlers stay thin: parse, authorize, call service, render.

## 3. Tenancy and authorization

- Every tenant table has `workspace_id NOT NULL` and `UNIQUE (workspace_id, id)`.
- Child tables reference parents with composite foreign keys `(workspace_id, parent_id) → parent(workspace_id, id)`. The database then refuses a lead that points at another workspace's customer, regardless of application bugs.
- The current workspace comes from the session and membership, never from a form field or URL parameter beyond a slug that is re-checked against membership.
- Every repository function takes `workspaceID` as its first argument; `sqlc` queries all filter on it. A lint rule (grep in CI) fails the build if a tenant table is queried without `workspace_id`.
- Roles: `owner` (billing, members, delete) and `staff` (operate). Object-level checks are "row belongs to workspace", enforced by the query itself.
- Cross-tenant test matrix: for every resource, workspace B attempts list, read, update, delete, and share-link access on workspace A's rows; all must 404.
- Postgres Row-Level Security is a Week 7 hardening candidate, not a v1 requirement.

## 4. Money and quantities

- Amounts are `BIGINT` rupiah. No floats anywhere in money paths.
- Quantity is `NUMERIC(12,3)` in the database and an integer in thousandths in Go.
- `line_total = round_half_up(qty_milli × unit_price / 1000)`; discounts are absolute rupiah or a percentage rounded per line; optional tax is a percentage on the subtotal rounded to the rupiah. One `money` package, table-driven tests, totals stored and re-verified on read.
- Payments are append-only. A mistake is corrected by a reversal record with a reason, which keeps the audit trail honest. Each payment submission carries an idempotency key so a double tap cannot record twice.
- Outstanding balance = invoice total − sum(confirmed payments) − sum(reversals), computed in SQL and asserted in Go.

## 5. Time

- Timestamps are `timestamptz` (UTC). Due dates are `DATE` in the workspace time zone (default `Asia/Jakarta`; `Asia/Makassar`, `Asia/Jayapura` selectable).
- "Hari ini" and "terlambat" are computed with the workspace zone, tested at midnight boundaries.

## 6. Security baseline (expanded in `THREAT_MODEL.md` in Phase B)

HTTPS only; `Secure`, `HttpOnly`, `SameSite=Lax` cookies; Argon2id; login and share-link rate limits; CSRF protection; strict `html/template` escaping; no mass assignment (explicit struct mapping per form); input validation at the handler boundary with Indonesian error messages; share tokens 128-bit random, revocable; secrets only from environment; no secrets in git; `govulncheck` in CI; audit events for create/update/delete of customers, leads, quotations, invoices, payments, members.

## 7. Data retention and backup (outline)

Daily managed-Postgres snapshots with 7-day retention for pilots; a restore drill documented and executed once before pilots start; workspace export (CSV/JSON) and deletion on request within 30 days; soft delete for operational records with a purge job later.

## 8. Development setup (prerequisites only; not executed yet)

Go 1.27.x, PostgreSQL 16+ (Homebrew or Docker; Docker daemon is currently stopped on this machine), Tailwind standalone CLI, `sqlc`, `goose`, `golangci-lint`, `air` for reload. GitHub CLI is not installed; CI setup will need it or the web UI.
