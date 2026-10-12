# Project Status

Updated: 2026-10-12 (evening, after Milestone 3)

## Current phase

Build-first MVP (D-019). Milestone 2 is merged into `main` (`c9fba63`). **Milestone 3 (customers, leads, pipeline, activity history) is implemented and tested on branch `m3-customers-leads`; the whole change set is uncommitted and awaits the founder's approval to commit.** Next: Milestone 4 (follow-ups, "Hari ini" list, WhatsApp click-to-chat). Validation track continues in parallel; Gate G1 checkpoint remains Sunday 2026-10-18.

## What works now (try it locally)

- Dashboard (`/app`): active lead count, estimated pipeline value (labelled as an estimate), counts per status, two shortcuts; an honest empty state before the first lead.
- Record a lead (`/app/prospek/baru`) in one screen: new customer (name, optional WhatsApp number) or an existing one, project title, type, source, estimated value in rupiah, follow-up date, description. A known number pre-selects the existing customer instead of duplicating it.
- Lead detail: status select with the pipeline rules (D-035), optional reason when lost, notes, and the full history (created, status changes with reasons, notes, which fields were edited). Status and note forms answer in place through htmx; the badge, the follow-up date and the timeline update without a reload and focus returns to the note box.
- Pipeline list (`/app/prospek`): status chips with counts (active by default; won and lost stay visible under their chip or "Semua"), search by project or customer, sort by newest, highest estimate or nearest follow-up, 25 per page.
- Customers (`/app/pelanggan`): list with search by name, number (any format) or company, add, edit, detail with that customer's leads and a "Catat prospek" shortcut.
- Phone numbers are stored as E.164 and shown as `0812-3456-7890`; money is integer rupiah; dates and times render in the workspace zone.
- Security: every query bound to the session's workspace (pinned at login, re-verified per request); foreign ids answer 404; composite foreign keys and CHECK constraints in the schema; append-only activities (trigger); CSRF on every POST; 64 KiB body limit; security headers; `no-store` on pages; rate limits keyed on the real client address; technical audit log lines with ids only.

```bash
cp .env.example .env            # once
docker compose up -d db
export PATH=$HOME/sdk/go1.27.2/bin:$PATH
make migrate && make run        # http://localhost:8080 → /daftar, then /app/prospek/baru
make test-db                    # full suite against Postgres (two packages at a time)
```

## Change set on `m3-customers-leads` (uncommitted, from `main` c9fba63)

| Area | Files |
|---|---|
| Schema | `internal/db/migrations/00003_customers_leads_activities.sql` (customers, leads, lead_activities, indexes, append-only trigger); `migrate_test.go` expects 3 migrations |
| Domain | `internal/money` (ParseIDR/FormatIDR), `internal/customer` (phone normalisation, validation, store), `internal/lead` (statuses, transitions, validation, activities, store with transactions) |
| Web | `customer_handlers.go`, `lead_handlers.go`, `listing.go` (pagination), `audit.go`; templates `pelanggan_*`, `prospek_*`, `beranda`, partials `pagination`, `status_badge`, `input`; nav in `layouts/app.html`; `templates.go` (page set, display funcs); routes in `router.go`; wiring in `cmd/server/main.go` (+ embedded tzdata); rebuilt `static/app.css` |
| M2 review fixes (D-042) | `middleware.go` (client IP, headers, no-store, body limit), `router.go` (LimitBy, scs ErrorFunc), `pages.go` (HX-Retarget, 413, fragment 500), `authz.go` + `session.go` + `auth_handlers.go` (pinned workspace, membership loss), layouts (htmx-config meta), `pengaturan.html` (header name OOB) |
| Infrastructure | `internal/db/migrate.go` (goose Postgres session lock, D-041), `testdb.go` (connection retry), `Makefile` (`-p 2`), `go mod tidy` |
| Tests | `money_test`, `phone_test`, customer `store_test`, lead `lead_test` + `store_test`, web `hardening_test` (11), `customer_web_test` (8), `lead_web_test` (11), `tenant_matrix_test`, `perf_test`, `helpers_test` |
| Docs | plan `docs/superpowers/plans/2026-10-12-m3-customers-leads.md`; `DECISIONS` D-033..D-042; `ARCHITECTURE` §3, §4, §5, §5.1, §8; `RISK_REGISTER` R-18..R-20; `DEVELOPMENT`; `MVP_PLAN` M3 status; this file |

## Tests executed (2026-10-12, Go 1.27.2 darwin/arm64, Postgres 17 in Docker)

| Command | Result |
|---|---|
| `REQUIRE_DB=1 go test -race -count=1 -p 1 ./...` | 9 packages ok; **125 tests passed, 0 failed, 0 skipped** (M1 + M2 regression included) in about 40 s; re-run after the review fixes |
| Same with the default `-p 8` | Twice failed with `ping database: context deadline exceeded` while the laptop ran an iOS simulator, Chrome, the dev server and a reviewer agent (load average 40; the Postgres container was starved). Every package passes on its own. `make test` now uses `-p 2` and the test harness retries the first connection for up to 60 s |
| M3 coverage | rupiah parsing/formatting table; phone normalisation table; customer create/get/update/search/pagination/clamping, duplicate phone per workspace, cross-workspace read and write refused, alphabetical `All`; lead transition rules, validation messages, activity descriptions; lead create with existing or inline customer (one transaction, nothing left behind on failure), foreign customer refused, status rules incl. follow-up default and reason handling, notes, update diff, append-only trigger, cross-workspace matrix at store level, list filters/search/sort/pagination, summary; web: every page renders, empty and error states, validation keeps input, duplicate phone pre-selects the customer, htmx fragments vs full pages for status and note, OOB swaps, dashboard summary, bad ids → 404, audit lines without content; tenant matrix over every customer/lead route (GET and POST, plain and htmx) plus unauthenticated → 303; 200-lead pipeline list timing |
| M2 review fixes coverage | error pages reach htmx clients; limiter keys on forwarded address only behind a trusted proxy; security headers and `no-store`; 413 on oversized body; session store failure → Indonesian 500 page; membership removed → logout; header name follows an htmx rename; CSRF over every state-changing route (cross-site, foreign Origin, trusted origin); passwords never in logs; expired session → login; pinned workspace |
| Pipeline list of 200 leads | 16 ms worst of 5 requests under `-race` (sequential run); 87 ms without `-race` while the machine was loaded (MVP target < 300 ms) |
| `gofmt -l cmd internal`, `go vet ./...`, `git diff --check` | clean |
| govulncheck (v1.9.0, database fetched 2026-10-12) | 0 vulnerabilities in called symbols or imported packages; **GO-2026-5932** (`golang.org/x/crypto/openpgp` unmaintained) remains listed at module level: no fixed version exists, the package is not imported anywhere, status re-checked each milestone |
| `make lint` | `go vet` only (golangci-lint not installed locally; CI runs v2.14.0). The deprecated-API usages the M2 review flagged (`middleware.RealIP`, `httprate.WithKeyByIP`, `httprate.Limit`) are gone |
| Live check (Claude's browser, 375 px viewport, dev server) | register → dashboard empty state → record a lead with an inline customer → detail shows value, type, source, description, history → status to Penawaran via htmx (badge, follow-up 15 Okt 2026 and timeline updated in place, single POST 200) → note via htmx (saved, box cleared, focus back in the box, timeline updated) → pipeline list with chips and counts → customers list → dashboard summary → unknown id renders the 404 page. A throwaway account exists in the local development database only |

## Security review (M3)

Independent review by a fresh-context reviewer agent (a separate Claude instance that saw only the repository, not this session; not a human reviewer) over the whole uncommitted change set, with the brief's authorization, tenant-isolation, SQL, transaction and validation requirements as the checklist.

| Severity | Finding | Status |
|---|---|---|
| Critical | none | — |
| Important | `X-Real-IP` was honoured ahead of the rightmost `X-Forwarded-For` behind a trusted proxy; proxies that pass the header through would let a client pick its own rate-limit bucket | Fixed: `X-Real-IP` ignored; test `TestRateLimitIgnoresClientSuppliedXRealIP`; D-042 reworded |
| Important | The out-of-band update of the lead meta block used a wrapper `div`, nesting one more `div` into the DOM on every status change | Fixed: `hx-swap-oob` on the `dl` itself; test `TestStatusFragmentSwapsMetaWithoutNestingWrappers`; checked live |
| Important | Negative-test gaps: CSRF for the seven new POST routes, mass-assignment attempts on the edit forms, foreign `?pelanggan=` preselect | Fixed: `TestCSRFBlocksEveryM3Route`, `TestMassAssignmentIsIgnored`, `TestLeadFormIgnoresForeignPreselectedCustomer` (all passed on first run; the behaviour was already correct, now proven) |
| Minor | fragment render failure answered a bare text 500 instead of the error page | Fixed (goes through `serverError`, retargeted for htmx) |
| Minor | `AddNote` indexed the re-read slice without a guard; `DisplayPhone` could panic on a malformed short `+62` value | Fixed, guard tests added |
| Minor | A unique-violation race produced a duplicate message with an empty name and a nil preselect | Fixed: generic message, no preselect when the owner is unknown |
| Minor | comment on membership fallback, `sprintf` alias, unused constant, test import-keepers, plan text for the foreign-customer case, D-036 wording | Fixed |
| Minor, open | `lost_reason` typed together with a non-`kalah` status is dropped silently (label already says it applies to Kalah only); the edit form accepts a follow-up date on a closed lead; chunked bodies over 64 KiB answer 400 rather than 413; customer select on the lead form caps at 500; `Customers.All`/`ForCustomer` limits are invisible to the user | Recorded; none affects isolation or integrity (the schema CHECK still enforces `lost_reason` only with `kalah`) |
| Recommendation | `customer.WhatsAppDigits` for `wa.me` in M4; cache `time.LoadLocation`; `RETURNING` in `appendActivity`; CI grep for tenant tables queried without `workspace_id`; widen `lead_activities.kind` in migration 00004 | Carried into the M4 plan |

Reviewer's verdict: "Ready to commit: with fixes" — no Critical findings; tenant isolation, SQL safety, transactions and validation judged sound and tested at store and handler level. The three Important items are fixed and tested above.

## Known defects and open items

- The fragment-render-failure path (now a 500 instead of an empty 200) and scs `ErrorFunc` wiring have tests only for the latter; the former is a three-line defensive change that a template corruption would trigger.
- Deferred from the M2 review as known gaps: no cap on concurrent Argon2 verifications (small pilot instances should run with ≥ 512 MB); registration reveals whether an e-mail is taken (mitigated by closed registration and the 10/min per-IP limit).
- Lead search is `ILIKE` without trigram indexes (R-18); the customer select on the lead form lists at most 500 customers by name.
- CSS build needs Node 22 on PATH (`make css-npm`); the standalone Tailwind download still stalls on this machine (D-031).
- golangci-lint not installed locally; GitHub remote still missing, CI unexecuted.

## Decisions awaiting approval

- Commit the M3 change set (proposed message in the session report) and merge `m3-customers-leads` into `main` after review.
- D-033..D-042 recorded as routine decisions within the brief; say so if any should change.

## Next three actions

1. Founder reviews the M3 report, approves the commit; Claude commits, merges to `main`, re-runs the suite there.
2. Milestone 4 plan: follow-ups table or promotion of `next_follow_up_on`, "Hari ini"/"Terlambat" lists in the workspace zone, WhatsApp templates and `wa.me` links, activity kinds for follow-ups.
3. Founder: Day 1 outreach, GitHub repository, golangci-lint decision.
