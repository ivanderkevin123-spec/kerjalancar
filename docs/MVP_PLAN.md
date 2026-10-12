# MVP Implementation Plan — milestones, acceptance criteria, tests

Strategy: build-first with controlled scope (D-019). Each milestone produces software that is more usable than the last, has tests the founder can re-run, and ends with a demo on a phone browser. Detailed, step-by-step plans are written one milestone at a time in `docs/superpowers/plans/` (M1: `2026-10-12-m1-bootstrap.md`). Stack per `ARCHITECTURE.md`: Go 1.27, chi, `html/template`, htmx 2.x, Tailwind, PostgreSQL 17, pgx, goose, sqlc where it helps.

Rules that apply to every milestone: server-side validation and authorization; `workspace_id` on every tenant row with composite foreign keys; integer rupiah; no secrets or personal data in the repo; demo data clearly labelled "DEMO"; no feature marked done without a passing test and a manual check on a phone; Indonesian UI copy; status report at the end (files changed, tests run with results, what can be tried, open issues, how to run locally).

## Final decisions this plan relies on

D-001 Go backend · D-002 segment A primary · D-003 price anchors · D-009 no job queue in v1 (PDF clause superseded by D-020) · D-010 money rules · D-011 CRM not accounting · D-012 HTMX frontend · D-013 chi · D-014 single repo/service · D-017 private data out of git · D-019 build-first · D-020 PDF in M5 · D-021 htmx 2.x · D-023 milestone order.

## Business assumptions still open (validation track)

A-02 forgotten leads cost real money · A-03 late balances cost real money · A-04 owners will enter data in a second place · A-05 Rp149k/month is payable · A-06 segment A is right · A-07 simple line items suffice (no full RAB) · A-15 in-app lists are enough without push. Gate G1 (Oct 18) and the pilots test these; a negative result changes scope through `DECISIONS.md`, not silently.

## Milestones

### M1 — Bootstrap (target: 1–2 days once the toolchain is available)

Outcome: a running Go service with configuration, structured logs, graceful shutdown, a Postgres connection, versioned migrations, health and readiness endpoints, a database test harness, and CI.

Scope: `go.mod`, `cmd/server`, `cmd/migrate`, `internal/config`, `internal/web` (router, middleware), `internal/db` (pool, migrations, `testdb`), `Makefile`, `docker-compose.yml` (Postgres 17 + test database), `.env.example`, `.golangci.yml`, `.github/workflows/ci.yml`, `docs/DEVELOPMENT.md`.

Acceptance criteria:

- `make db-up && make migrate && make run` serves `GET /healthz` → 200 `{"status":"ok"}` and `GET /readyz` → 200 when Postgres answers, 503 within 2 s when it does not.
- Missing `DATABASE_URL` or an invalid `PORT` fails startup with a one-line message and exit code 1, no stack trace.
- `SIGTERM` lets an in-flight request finish (bounded by a 10 s shutdown timeout).
- `make test` runs unit tests and Postgres-backed tests in a rollback-per-test transaction; `make lint` passes.
- CI workflow runs vet, lint, race tests against a Postgres service container, and `govulncheck` (verified on first push).

Tests: config table tests; router test for `/healthz`; `/readyz` test with a live pool and with a cancelled/closed pool; migration up/status test against the test database; shutdown test with an in-flight request.

Deferred on purpose: Dockerfile (M7), Tailwind and htmx assets (M2), sqlc (introduced in M2 with the first real queries).

**Status 2026-10-12: M1 complete on branch `m1-bootstrap`.** Verified: healthz/readyz live; readyz 503 in 0.045 s with Postgres stopped and 200 after restart; missing `DATABASE_URL` and `PORT=abc` exit 1 with one-line messages; SIGTERM lets an in-flight request finish (test) and exits 0; `make test-db` green with `-race` (19 tests, rollback-per-test harness); gofmt and govulncheck clean. Partial: `make lint` ran `go vet` only because golangci-lint is not installed locally (CI runs it); CI workflow authored and YAML-validated but not executed until a GitHub repository exists.

### M2 — Authentication, workspace, responsive layout (target: 3–4 days)

Outcome: an owner signs up, creates a workspace, logs in and out, invites one staff member, and sees an empty mobile-first dashboard shell in Indonesian. Nobody can see another workspace.

Scope: tables `users`, `workspaces`, `memberships` (role owner/staff), `sessions` (scs store), `invitations`; Argon2id passwords; server-side sessions with `Secure`/`HttpOnly`/`SameSite=Lax`; CSRF protection (Go 1.25+ `net/http` CrossOriginProtection if confirmed, otherwise double-submit token); login rate limit (per IP and per account); password reset by emailed token is deferred until email sending exists (M7 or pilot feedback), replaced for pilots by founder-assisted reset; app layout (header, bottom nav on mobile, flash messages, form error rendering) with Tailwind standalone CLI and htmx 2.x vendored; `sqlc` introduced; current-workspace middleware resolves membership from the session.

Acceptance criteria:

- Sign-up → workspace → logout → login works on a 360 px wide phone viewport and on desktop.
- Invitation link creates a staff member bound to the right workspace; a used or expired link is rejected.
- Cross-tenant matrix: user in workspace B gets 404 on every workspace-A URL (workspace page, members, invitation).
- 6 failed logins in a minute from one IP or on one account → 429 with an Indonesian message; passwords never logged.
- All forms re-render with field-level Indonesian errors on invalid input; CSRF failure returns 403.

Tests: Argon2id hash/verify and parameter tests; session fixation test (session ID rotates at login); CSRF negative test; rate-limit test; invitation lifecycle; cross-tenant matrix (generated table test); handler tests asserting HTML with goquery; one Playwright smoke (sign-up to dashboard) kept for M7's suite.

### M3 — Customers, leads, pipeline, activity (target: 5 days)

Outcome: the owner records a lead in ≤ 15 s on a phone, moves it through Baru → Dihubungi → Penawaran → Menang/Kalah, and sees the activity history.

Scope: tables `customers` (name, WhatsApp number normalised to E.164, source, notes), `leads` (customer, need, estimated value in rupiah, stage, lost reason, next follow-up date), `lead_activities` (append-only: note, stage change, system events); one-screen lead form that creates the customer inline; pipeline list grouped by stage with a stage-change control (no drag-and-drop); customer detail with timeline; list search; empty states; audit events.

Acceptance criteria:

- New lead from the dashboard in 3 taps and 3 fields; WhatsApp number accepts `08…`, `+62…`, `62…` and stores `+62…`; invalid numbers rejected with an Indonesian message.
- Stage change appends an activity and, for Penawaran, pre-sets a follow-up date (default +3 days, workspace zone).
- Cross-tenant matrix extended to customers, leads, activities (list, read, update, stage change, delete).
- Pipeline list of 200 leads renders under 300 ms server-side on the dev machine (measured, not promised).

Tests: phone normalisation table test; stage transition rules (illegal transitions rejected); activity append-only (no update route exists, DB trigger or lack of UPDATE grant verified by test); cross-tenant matrix; handler tests; a Playwright flow "record lead on mobile viewport".

Deferred: lead sources beyond a free-text/select field, tags, bulk import (pilot feedback decides).

### M4 — Follow-ups, reminder lists, WhatsApp click-to-chat (target: 4 days)

Outcome: the owner opens the app and sees exactly who to contact today and who is overdue, taps once to open WhatsApp with a prepared message, and marks the follow-up done.

Scope: `follow_ups` (lead or invoice reference, due date, note, done_at, done_by); "Hari ini" and "Terlambat" views computed in the workspace time zone; auto-suggested follow-up after stage changes; `message_templates` with placeholders (`{nama}`, `{usaha}`, `{nilai}`, `{tautan}`) rendered server-side and URL-encoded into `https://wa.me/<number>?text=…`; three seed templates in Indonesian per workspace (follow-up penawaran, pengingat DP, pengingat pelunasan); marking done appends an activity.

Acceptance criteria:

- A follow-up due at 2026-10-20 in `Asia/Jayapura` appears in "Hari ini" at 00:00 WIT and not at 00:00 WIB (tested at zone boundaries).
- `wa.me` link opens WhatsApp on Android and desktop with the rendered message; placeholders with missing data render as empty, never as `{nama}`.
- Done/undo is idempotent; counts on the dashboard shell match the lists.

Tests: due-list queries at boundaries; template rendering and URL encoding (including `+`, `&`, newline, emoji); cross-tenant matrix for follow-ups and templates; handler tests.

Deferred: push/email reminders (A-15), recurring follow-ups.

### M5 — Quotations with items, totals, share link, PDF (target: 5–6 days)

Outcome: the owner builds a quotation with line items, sends it as a link or PDF through WhatsApp, and sees when it was opened and whether it was accepted.

Scope: `quotations` (number per workspace, customer, lead, status draft/sent/accepted/rejected/expired, valid_until, notes, totals), `quotation_items` (description, qty in thousandths, unit, unit price in rupiah, line total), optional discount and tax percentage, server-authoritative totals via `internal/money`; editor with htmx row add/remove and a single JS island for instant local totals; public share page by 128-bit token with print CSS and "opened at" tracking; PDF via a pure-Go generator (D-020; library chosen at start of M5) served as a download and attachable in WhatsApp; status changes append activities; accepting a quotation moves the lead to Menang.

Acceptance criteria:

- Totals match a table of hand-computed cases to the rupiah, including qty 2.5 × Rp1,999 and 10% discount + 11% tax; sum of lines equals the stored total on read.
- Share token is unguessable, revocable, and expires with the quotation; revoked link returns 404; the page shows no data beyond the quotation.
- PDF opens in WhatsApp on Android and prints on A4 with the workspace name and quotation number; golden-file test keeps the layout stable.
- Quotation number is unique per workspace under concurrent creation (DB constraint + retry).

Tests: money rounding table; number-sequence concurrency test; token tests; PDF golden test; cross-tenant matrix for quotations, items, share links; handler tests; Playwright "create and share quotation".

Deferred unless G1/pilots demand: full RAB (volumes, unit price analysis), revision history with diffs, pekerjaan tambah-kurang, e-signature.

### M6 — Invoices, DP/installments, balances, dashboard (target: 5 days)

Outcome: the owner records DP and installments, always knows the outstanding balance per customer and in total, and the dashboard shows pipeline value, quotations by status, receivables, and today's follow-ups.

Scope: `invoices` (from quotation or blank; number per workspace; due date; totals), `invoice_items`, `payments` (append-only; type DP/cicilan/pelunasan; method; received_on; note; idempotency key; recorded_by), `payment_reversals` (reason); balance computed in SQL; receivables list (overdue first); invoice share page and PDF reuse M5; dashboard read models; UI copy near every payment form stating that a recorded payment is the owner's note, not proof of transfer (D-011).

Acceptance criteria:

- Balance = total − payments + reversals, equal in SQL and Go, for every fixture; overpayment rejected with a message.
- Double-submitting the same payment form records exactly one payment (idempotency key); two concurrent payments on one invoice serialize via `FOR UPDATE` and both persist correctly.
- Dashboard numbers reconcile with list totals in a test that builds a fixture workspace.
- Invoice due dates drive the "Terlambat" list from M4.

Tests: balance invariants; idempotency; concurrency; reversal flow; cross-tenant matrix; dashboard reconciliation; handler tests.

Deferred: partial-payment schedules with planned installments (pilot feedback), tax invoices (faktur pajak), bank integration (never in v1).

### M7 — Security review, integration and e2e tests, deployment prep, backup/restore, pilot readiness (target: 5 days)

Outcome: the founder can put 3–5 pilots on a staging deployment with confidence that tenant isolation, sessions, backups, and monitoring work, and that incidents are diagnosable.

Scope: `THREAT_MODEL.md` checklist executed and recorded (auth/session, BOLA, tenant isolation, CSRF/XSS, SQL injection, mass assignment, file generation and access, rate limiting, secrets, sensitive logging, retention/deletion); Playwright suite for the critical flows (sign-up → lead → follow-up → quotation link → payment → dashboard); Dockerfile; `DEPLOYMENT.md` with environment variables, migration procedure, health checks, rollback, smoke tests; backup and restore drill executed against a scratch database and logged; error monitoring wiring (external service, needs approval); workspace export (CSV) and deletion; demo workspace seed clearly labelled DEMO; onboarding checklist for pilots; founder usage view.

Acceptance criteria:

- Every threat-model item has a test or a documented manual check with date and result; no item marked "assumed".
- Restore drill: a backup taken from the test database restores into a fresh database and the app passes smoke tests against it.
- Playwright suite green locally and in CI; `govulncheck` clean.
- Staging deployment only after explicit approval with a cost line (D-005); production smoke tests documented.

Tests: full regression (`make test`, Playwright); smoke script; restore script exercised in CI against a throwaway database.

## Parallel validation checkpoints inside the build

| When | Validation input | Effect on build |
|---|---|---|
| Oct 18 (G1) | Interview and concierge results | NARROW/REVISE may change M3–M5 scope; STOP pauses after the current milestone |
| End of M4 | Show the follow-up loop to 3 interviewees (demo data) | Reaction feeds M5 priorities |
| End of M5 | 3 sanitized real quotations through the PDF | Layout fixes before M6 |
| End of M7 | Pilot onboarding | Pilot metrics (`VALIDATION_PLAN.md` §7) start |

## Environment prerequisites (blocking M1 today)

Go 1.27.2 toolchain (machine has 1.17.5), a PostgreSQL 17 instance (Docker daemon is stopped, no local Postgres), and later Node 22 via nvm for Playwright. Options and the founder's choice: `PROJECT_STATUS.md`.
