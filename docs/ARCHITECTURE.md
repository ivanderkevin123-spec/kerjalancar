# Architecture

Status: frontend decision D-012 PROPOSED (2026-10-12). Backend Go + PostgreSQL approved (D-001). Nothing is installed or built until Gate G1 passes and D-012 is approved.

## 1. The decision: Go server-rendered HTML + HTMX (Option A), not Next.js + Go API (Option B)

Both options share Go, `chi`, PostgreSQL, `pgx`/`sqlc`, versioned migrations, structured logging, graceful shutdown, context-aware DB calls, and explicit timeouts. The question is only where the UI is rendered and whether a second service and an API contract exist from day one.

**Recommendation: Option A.** The MVP is forms, lists, and a handful of computed views on a phone. Nothing in it needs client-side state beyond a form. Option B's benefits (component ecosystem, rich client interactions, an API for future native apps) address needs that are explicitly out of v1 scope, while its costs (second service, API contract, duplicated validation for UX, two test stacks, auth across origins, Node runtime, framework churn) are paid every week by a solo founder.

### 1.1 Workflow inventory against both options

| Screen | Interaction needed | Option A (HTMX) | Option B (Next.js) |
|---|---|---|---|
| Lead capture (3 fields + default follow-up date) | Form, inline validation | Native | Native, plus schema duplication for UX |
| Pipeline by stage, move stage | Button or select → row re-render | `hx-post` + partial swap | Client state + API call |
| Customer detail with activity timeline | List + add-note form | Native | Native |
| Quotation editor | Add/remove rows, live totals, Rp formatting | `hx-post` to recompute totals server-side (authoritative) plus a ~50-line JS island for instant local totals | Natural in React, but totals still must be recomputed in Go |
| Public share page (quotation, invoice) | Static, print CSS | Native | Needs SSR or a separate static route |
| Invoice, DP/installment forms | Forms | Native | Native |
| "Hari ini" follow-up list, mark done | Button → swap | Native | Client state + API |
| Dashboard | Numbers + lists | Native | Native |
| WA template → `wa.me` link | Server renders encoded URL | Native | Native |
| Search/filter lists | Debounced `hx-get` | Native | Client state |
| Drag-and-drop kanban, offline mode, real-time updates | Not in v1 | Would need JS islands or a later API | Natural |

Verdict: ten of eleven screens are native to Option A; the quotation editor is the only one needing a small JS island. Option B would only pull ahead for features excluded from v1.

### 1.2 Comparison

| Criterion | Option A | Option B | Notes |
|---|---|---|---|
| Implementation speed, solo engineer with Claude Code | One codebase, one language for all rules, no contract to keep in sync | Two apps, OpenAPI, generated client, zod schemas mirroring Go validation, two CI pipelines | Our estimate, not a measurement: B adds 30–50% effort over 8 weeks |
| UX quality on mid-range Android | Server HTML is fast on weak connections; HTMX partials keep pages light; design quality depends on Tailwind discipline and a partials library | Richer component kits (shadcn/ui); heavier bundles; hydration cost on cheap phones | Both can be excellent; the determinant is design discipline, not framework |
| Frontend complexity ceiling | Low ceiling: beyond 3–4 JS islands, HTMX becomes awkward | High ceiling | Escape hatch in §1.4 |
| Testing | Go unit + `httptest` handler tests asserting HTML + Postgres integration + Playwright for 4–5 critical flows | All of that plus Vitest/RTL component tests and contract tests | Fewer layers to keep green in A |
| Security surface | One origin, session cookie, CSRF, auto-escaped templates, no tokens in browser | Two origins or a proxy, cookie-vs-token decisions, CORS, secrets in two runtimes; React reduces XSS risk but adds API auth complexity | A has fewer places to get it wrong |
| Deployment and cost | One container + managed Postgres | Two services + managed Postgres; Node runtime | Roughly double the service bill at pilot scale |
| Maintainability and churn | Go and HTMX change slowly | Next.js major-version churn (routing, caching, server components) is a known maintenance tax | Matters for a one-person team |
| Future native app or integrations | Add `/api/v1` JSON handlers over the same service packages | Already has an API | Additive in A because handlers are thin and domain logic lives in packages |
| Founder fit | Go and Ruby background; server-rendered apps are familiar territory | TypeScript/React exposure exists but is not the founder's core skill | — |

### 1.3 What we give up with Option A, and why that is acceptable

- No shadcn/ui. We build a small set of Tailwind partials (button, input, select, card, list row, empty state, badge, money) and reuse them. Fewer, consistent components are also easier to keep accessible.
- Rich client interactions need JS islands. The only v1 case is the quotation editor. If pilots demand drag-and-drop or offline, that is new evidence and goes through `DECISIONS.md`.
- No generated TypeScript client. There is no second consumer to generate for.

### 1.4 Revisit triggers

Re-open D-012 if any of these happen: the Week 3 quotation editor misses its acceptance criteria because of HTMX limits; pilots require offline capture; a native app is validated; more than four JS islands exist. The first step in each case is to add a JSON API on the same Go service, not to split services.

### 1.5 Repository layout

Single repository, single deployable service. A monorepo toolchain (Turborepo, workspaces) solves a problem we do not have; two repos would split one product's history for no benefit. Playwright tests live in `e2e/` and use Node only as a test runner (nvm already has Node 22).

## 2. System diagram

```
   Owner's phone or laptop (browser; installable as PWA later)
        │  HTTPS · session cookie · HTML pages + HTMX partial requests
        ▼
┌──────────────────────────────────────────────────────────────────┐
│  kerjalancar — one Go binary, one container                      │
│                                                                  │
│  internal/web      chi router · middleware (request id, slog,    │
│                    session, CSRF, rate limit, timeouts)          │
│                    handlers (parse → authorize → call → render)  │
│                    html/template pages + HTMX partials · static  │
│                    (HTMX, optional Stimulus, Tailwind CSS)       │
│  ────────────── thin transport; no business rules ───────────── │
│  internal/{auth, workspace, customer, lead, quotation, invoice,  │
│            followup, template, dashboard, audit}                 │
│            each owns its rules, SQL (sqlc), and tests            │
│  internal/money  integer rupiah, quantity scale, rounding        │
│  internal/db     pgx pool · goose migrations · test helpers      │
└───────────────────────────────┬──────────────────────────────────┘
                                │ pgx (context, timeouts, transactions)
                                ▼
              Managed PostgreSQL · workspace_id on every tenant row
              composite foreign keys · daily backups · tested restore

   Outbound (v1): wa.me click-to-chat links only (no WhatsApp API)
   Later, with approval: error monitoring, transactional email, /api/v1
```

## 3. Components (versions confirmed in Phase B against the hosting platform)

- Go 1.27.2 (latest stable on go.dev, 2026-10-12; machine has 1.17.5; install method is the founder's choice, D-022)
- `chi` v5 router on `net/http`; `html/template`; `log/slog` JSON logs; graceful shutdown; per-request timeouts
- htmx 2.0.11 vendored as a pinned static file (D-021: not 4.0.0); Stimulus only if JS islands exceed two or three controllers, otherwise small vanilla modules; Tailwind CSS v4 standalone CLI (`tailwindcss-macos-arm64` / `tailwindcss-linux-x64`, no Node in the build)
- PostgreSQL 17 (`postgres:17-alpine` locally); `pgx/v5` v5.11.0; `sqlc` v1.31.1 typed queries from M2; `goose` v3.29.0 migrations (Provider API, embedded SQL); `chi` v5.3.2; `testify` v1.12.1
- Sessions server-side in Postgres (`alexedwards/scs` v2.9.0 + postgresstore); Argon2id (`golang.org/x/crypto/argon2`); CSRF via `net/http.NewCrossOriginProtection` (confirmed: added in Go 1.25) with a double-submit token fallback for browsers without Fetch metadata
- In-process rate limiting (single instance during pilots); error monitoring via Sentry Go SDK or GlitchTip [external service, needs approval]
- Background jobs: none in v1 (reminders are queries). If daily digests are validated, add River (Postgres-backed), not Redis
- PDF generation runs synchronously in the request (bounded by the 30 s timeout); no worker
- PDF: required in v1 for quotations and invoices (D-020, built in M5) with a pure-Go generator chosen at M5 start; share pages with print CSS remain
- Tests: `testing` + `testify`; handler tests with `httptest` and HTML assertions; Postgres integration tests with per-test transactions; `-race`; `golangci-lint`; `govulncheck`; Playwright for critical flows (sign-up → first lead → quotation link → payment → today list)
- CI: GitHub Actions with a Postgres service container
- Hosting (D-005): Fly.io (sin), Railway (Singapore), or Render (Singapore) with managed Postgres; Docker image; daily backups and a tested restore script

## 4. Modular monolith layout

```
cmd/server/            main: config, DB pool, router, graceful shutdown
internal/
  auth/                users, sessions, password hashing, login rate limit
  workspace/           workspaces, memberships, roles (owner, staff), settings (time zone)
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
  web/                 chi router, middleware, handlers, templates, partials, static, Indonesian copy
e2e/                   Playwright specs for critical flows
```

Each package exposes a small service API and owns its SQL. Packages do not import each other's internals; cross-module reads go through explicit functions. Handlers stay thin: parse, authorize, call service, render.

## 5. Tenancy and authorization

- Every tenant table has `workspace_id NOT NULL` and `UNIQUE (workspace_id, id)`.
- Child tables reference parents with composite foreign keys `(workspace_id, parent_id) → parent(workspace_id, id)`, so the database refuses a lead that points at another workspace's customer regardless of application bugs.
- The current workspace comes from the session and membership, never from a form field or URL parameter beyond a slug that is re-checked against membership.
- Every repository function takes `workspaceID` first; all `sqlc` queries filter on it. A CI grep fails the build if a tenant table is queried without `workspace_id`.
- Roles: `owner` (billing, members, delete) and `staff` (operate). Object-level checks are "row belongs to workspace", enforced by the query itself.
- Cross-tenant test matrix: for every resource, workspace B attempts list, read, update, delete, and share-link access on workspace A's rows; all must return 404.
- Postgres Row-Level Security is a Week 7 hardening candidate, not a v1 requirement.

## 6. Money and quantities

- Amounts are `BIGINT` rupiah. No floats in money paths.
- Quantity is `NUMERIC(12,3)` in the database and an integer in thousandths in Go.
- `line_total = round_half_up(qty_milli × unit_price / 1000)`; discounts are absolute rupiah or a percentage rounded per line; optional tax is a percentage on the subtotal rounded to the rupiah. One `money` package, table-driven tests, totals stored and re-verified on read.
- Payments are append-only; corrections are reversal records with a reason. Each payment submission carries an idempotency key.
- Outstanding balance = invoice total − sum(confirmed payments) + sum(reversals), computed in SQL and asserted in Go.
- Concurrent updates: optimistic version column on quotations and invoices; payment inserts in a transaction that re-reads the invoice with `FOR UPDATE`.

## 7. Time

- Timestamps are `timestamptz` (UTC). Due dates are `DATE` in the workspace time zone (default `Asia/Jakarta`; `Asia/Makassar`, `Asia/Jayapura` selectable).
- "Hari ini" and "terlambat" are computed in the workspace zone and tested at midnight boundaries.

## 8. Security baseline (expanded in `THREAT_MODEL.md` in Phase B)

HTTPS only; `Secure`, `HttpOnly`, `SameSite=Lax` cookies; Argon2id; login and share-link rate limits; CSRF protection; strict `html/template` escaping; no mass assignment (explicit struct mapping per form); server-side validation with Indonesian error messages; share tokens 128-bit random, revocable; secrets only from environment; no secrets in git; `govulncheck` in CI; audit events for create/update/delete of customers, leads, quotations, invoices, payments, members; no personal data in logs.

## 9. Data retention and backup (outline)

Daily managed-Postgres snapshots with 7-day retention for pilots; a restore drill executed and documented before pilots start; workspace export (CSV) and deletion on request within 30 days; soft delete for operational records with a purge job later.

## 10. Development prerequisites (not executed; installs need approval per D-004)

Go 1.27.x, PostgreSQL 16+ (Homebrew or Docker; the Docker daemon is currently stopped), Tailwind standalone CLI, `sqlc`, `goose`, `golangci-lint`, `air` for reload, Node 22 via nvm for Playwright only. GitHub CLI is absent; CI can be configured through the GitHub web UI.
