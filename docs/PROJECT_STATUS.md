# Project Status

Updated: 2026-10-13 (Milestone 4 final gate)

## Current phase

Build-first MVP (D-019). Milestones 1–3 are merged into `main` (M3: `d773e1c`; lint fixes `271b52d`; CI green on `297c64a`). **Milestone 4 (follow-ups, "Hari ini" list, WhatsApp click-to-chat) is implemented and tested on branch `m4-followups-whatsapp`; the whole change set is uncommitted and awaits the founder's approval to commit.** Next: Milestone 5 (quotations with items, totals, share link, PDF). Validation track continues in parallel; Gate G1 checkpoint remains Sunday 2026-10-18.

## What works now (try it locally)

- Dashboard (`/app`): Terlambat / Hari ini / Mendatang counts, a "Perlu dihubungi" agenda (overdue first, then today's), then the M3 pipeline summary.
- Follow-up list (`/app/follow-up`): tabs Terlambat, Hari ini, Mendatang (plus "lebih dari seminggu"), Selesai (paginated); each row shows customer, lead, status, due date, note, and offers Selesai / Batalkan selesai (in place via htmx, counts updated), WhatsApp, and the lead page.
- Lead detail: a follow-up panel to schedule or reschedule (date pre-filled today + 3, optional note) and to mark done; a WhatsApp panel with the three templates and a "Buka WhatsApp" button (opens `wa.me` in a new tab with the message filled in; the tap is recorded in the history as "Pesan WhatsApp dibuka"); a hint when a placeholder would be empty; a link to add a number when the customer has none.
- Pipeline rules: entering Penawaran without an open follow-up schedules one for today + 3; closing a lead finishes its follow-up ("Follow-up selesai (prospek ditutup)"); reopening leaves history intact and allows a new follow-up.
- Settings: workspace time zone (WIB / WITA / WIT) which decides what "today" is; three editable WhatsApp templates with a live preview and placeholder help.
- Everything computes "today" in the workspace zone; a follow-up due on 20 Oct is "Hari ini" from 00:00 WIT in Jayapura while Jakarta still shows it as "Mendatang" (tested at the boundary).

```bash
cp .env.example .env            # once
docker compose up -d db
export PATH=$HOME/sdk/go1.27.2/bin:$PATH
make migrate && make run        # http://localhost:8080 → /daftar, /app/prospek/baru, /app/follow-up
make test-db                    # full suite against Postgres (two packages at a time)
make lint                       # golangci-lint v2.14.0 (go run on first use)
```

## Change set on `m4-followups-whatsapp` (uncommitted, from `main` 297c64a)

| Area | Files |
|---|---|
| Schema | `internal/db/migrations/00004_followups_templates.sql` (follow_ups with composite FK and one-open-per-lead partial unique index, backfill from `leads.next_follow_up_on`, message_templates seeded per workspace, time-zone CHECK, widened activity kinds); `internal/db/migrate.go` (`MigrateTo`); `migrate_test.go` (≥ 4); `migrate_backfill_test.go` (fresh database: stage data at version 3, migrate, assert backfill, seeds and CHECK) |
| Domain | `internal/followup` (Today, BucketOf, Validate, DefaultDue; store: Schedule/ScheduleTx/ScheduleForLead, CloseForLead, Done, Undo, Get, OpenForLead, Lists, DonePage; cache of `leads.next_follow_up_on` in the same transaction); `internal/template` (keys, seeds, Render, MissingPlaceholders, WaLink, Validate; store: ForWorkspace, Get, Update, SeedTx); `internal/customer/phone.go` (`WhatsAppDigits`); `internal/workspace/store.go` (Timezones, UpdateTimezone owner-only, seeds templates on creation); `internal/lead` (new activity kinds and lines, exported `AppendActivity`, `ScheduleHook`/`CloseHook`, `RecordWhatsApp`, Update no longer edits the follow-up date, Create schedules through the hook) |
| Web | `followup_handlers.go` (list, schedule, done, undo), `whatsapp_handlers.go`, `settings_handlers.go` (time zone, templates); `Deps.Clock`, `Deps.FollowUps`, `Deps.MessageTemplates`; `location()` cache; `View.Today`; templates `followup_daftar.html`, `templat.html`, follow-up and WhatsApp panels in `prospek_detail.html`, dashboard blocks in `beranda.html`, zone form in `pengaturan.html`, nav entry; `prospek_form.html` shows the first follow-up date only on create; rebuilt `static/app.css`; wiring in `cmd/server/main.go` |
| Tests | `followup_test.go` (zone boundaries, buckets, validation), `followup/store_test.go` (schedule/reschedule, done/undo idempotency, buckets per zone, cross-workspace, lead hooks), `template_test.go` + `template/store_test.go`, workspace time-zone test, `migrate_backfill_test.go`, web: `followup_web_test.go` (7), `whatsapp_web_test.go` (4), `settings_web_test.go` (4: zone re-bucketing, template edit/preview, CSRF for every M4 POST, mass-assignment and anonymous access), lead tests adapted to the hooks |
| Docs | plan `docs/superpowers/plans/2026-10-12-m4-followups-whatsapp.md`; `DECISIONS` D-043..D-049; `ARCHITECTURE` §2, §4, §5.1, §7; `RISK_REGISTER` R-21..R-23; `MVP_PLAN` M4 status; `DEVELOPMENT`; this file |

## Tests executed (2026-10-12, Go 1.27.2 darwin/arm64, Postgres 17 in Docker)

| Command | Result |
|---|---|
| `REQUIRE_DB=1 go test -race -count=1 -p 1 ./...` | 11 packages ok; **165 tests passed, 0 failed, 0 skipped** (M1–M3 regression included; final gate run on the exact tree proposed for the commit) |
| Review-focus scenarios (plan) | 1 number changed after scheduling → `TestWhatsAppButtonRendersTheTemplateWithTheCurrentNumber`; 2 done tapped twice → `TestDoneAndUndoAreIdempotentAndRecorded` (store) and `TestDoneAndUndoFromTheListAreIdempotentOverHtmx` (web); 3 lead reopened after auto-close → `TestLeadStatusChangesDriveFollowUps` and `TestClosingALeadClosesItsFollowUpInTheUI`; 4 empty `{nilai}` → `TestRenderFillsPlaceholdersAndBlanksMissingOnes` and `TestWhatsAppWithoutNumberOrEstimate`; 5 zone changed with follow-ups present → `TestTimezoneSettingRebucketsFollowUps` and `TestListBucketsFollowTheWorkspaceZone` |
| Migration on a new database and an existing one | `TestMigration4BackfillsFollowUpsAndSeedsTemplates` (fresh database staged at version 3, then migrated: open lead backfilled, closed lead not, three templates seeded, foreign zone refused); `TestMigrateConcurrentOnFreshDatabase` (4 migrators); the existing dev and test databases migrated in place (`make migrate`: applied 4) |
| `gofmt -l cmd internal`, `go vet ./...`, `git diff --check` | clean |
| golangci-lint v2.14.0 (CI version, via `go run`) | 0 issues (after two fixes: a whitelisted redirect rewritten as fixed strings, a tagged switch) |
| govulncheck (binary built from x/vuln v1.9.0, database fetched 2026-10-12) | 0 vulnerabilities in called symbols or imported packages; **GO-2026-5932** (`golang.org/x/crypto/openpgp` unmaintained) remains listed at module level: no fixed version exists, the package is not imported, re-checked each milestone |
| Live check (Claude's browser, 375 px viewport, dev server after `make migrate`) | the M3 lead's cached date was backfilled into an open follow-up ("mendatang 15 Okt 2026"); dashboard counts; Selesai from the list via htmx (row swapped in place, counts updated to Selesai 1), Batalkan selesai (counts back, history shows "Follow-up selesai" then "Follow-up dibuka kembali"); lead detail with follow-up and WhatsApp panels; settings with the zone select; template page with previews; no console errors. The `wa.me` redirect was exercised by tests only (a real tap would leave the dev page for WhatsApp) |
| Cache reconciliation (`leads.next_follow_up_on` vs the open `follow_ups` row) | Dev database: 0 mismatches. Test database: 0 mismatches among rows written by the application; the only mismatches were test fixtures that a capacity test had inserted with raw SQL without the cache (test fixed to maintain it); at most one open follow-up per lead everywhere |
| Not run | Playwright (M7); `make test` with the default parallelism was not retried (M3 notes still apply) |

## Security review (M4)

Independent review by a fresh-context reviewer agent (a separate Claude instance that saw only the repository and the plan; not a human reviewer), with the plan's non-negotiables and the five review-focus scenarios as the checklist.

| Severity | Finding | Status |
|---|---|---|
| Critical | none | — |
| Important | Backfill left a stale `leads.next_follow_up_on` on closed leads (M3 allowed dates on closed leads), breaking the cache invariant on day one and suppressing the penawaran auto-schedule after reopen | Fixed: migration 00004 now NULLs the cache on `menang`/`kalah`; asserted in `TestMigration4BackfillsFollowUpsAndSeedsTemplates` |
| Important | `lead.ChangeStatus` still used the wall clock for the penawaran default while the web layer used the injectable clock | Fixed: it takes the workspace's `today`; store tests use fixed dates |
| Important | `Lists` read one capped query and split it in Go, so ≥ 400 overdue rows would hide today's list while the count said otherwise (D-044) | Fixed: one capped query per bucket; `TestListsNeverStarveLaterBuckets` (105 overdue + today + upcoming) |
| Important | Open follow-ups could be scheduled on, or undone for, a closed lead, which then appeared in "Hari ini" wearing a Kalah badge (plan ambiguity) | Fixed per D-045: `ErrLeadClosed` from `Schedule` and `Undo`, panel shows an explanation instead of the form, no "Batalkan selesai" for closed leads; store and web tests |
| Important | Lock-order inversion (lead→follow-up in Schedule/ChangeStatus, follow-up→lead in Done/Undo) could deadlock two devices into a 500 | Fixed: Done/Undo lock the lead row first; unique-violation races map to `ErrAnotherOpen`. Proven by `TestDoneAndUndoWaitForTheLeadLockInsteadOfDeadlocking`: one transaction holds the lead row while Done/Undo run concurrently, waits (via `pg_stat_activity`) until they block, then locks the follow-up row; with the inverted order temporarily reintroduced the test failed with `deadlock detected (SQLSTATE 40P01)`, with the fix it passes (3 of 3 runs) |
| Minor | edit path validated an invisible date field; create path accepted any year for the first follow-up; no-phone POST answered 200 (refresh re-posts); dead `fuRow.Created`; leftover in a test | Fixed (`TestLeadCreateBoundsTheFirstFollowUpDate`; PRG with flash) |
| Minor | plan items not delivered: CI grep for tenant tables; `AppendActivity` without `RETURNING`; tenant matrix file not extended (coverage lives in the foreign-id tests) | CI grep delivered as a Go source scan (`TestEverySQLOnTenantTablesIsScopedByWorkspace`: every SQL literal touching a tenant table must mention `workspace_id`; runs in `make test` and CI); the other two stay open |
| Minor, open | `whatsapp_opened` stores the template label rather than the key (labels are fixed constants, so history is stable); follow-up rows open only the follow-up template; each htmx done/undo recomputes the list data for the chip counts; `internal/template` shadows `html/template` inside package `web`; `dateLabel` duplicates `formatDateID`; "Mendatang" lists beyond-a-week rows without a "showing 100 of N" hint; the Down migration fails once new-kind activity rows exist | Recorded, none affects isolation or integrity |
| Recommendation (M5) | adopt the lead-first lock order as a convention; consider an `activity` package so M5 does not add a third hook; replace the `kind` CHECK with a lookup table or Go whitelist in 00005; share tokens in the path, 128-bit, revocable, rate-limited, `no-store`; PDF generation bounded by a timeout and size cap; `{tautan}` from the share URL | Carried into the M5 plan |

Reviewer's verdict: "Ready to commit: with fixes" — authorization, tenant isolation, CSRF, encoding, logging hygiene and idempotency judged sound and well tested; the five Important items are fixed and tested above.

## Known defects and open items

- Task F's handler code was written together with its tests (the tests passed on their first run); every other task followed red → green.
- Databases that ran the first draft of migration 00004 on this machine (dev and test) did not get the closed-lead cache reset because goose records the version, not the text; no such database exists outside this laptop, and the only closed lead there has no cached date.
- Follow-ups beyond seven days have no tab of their own; they appear under "Mendatang" below a heading. The "Mendatang" count only covers the seven-day window (D-044).
- The WhatsApp tap is recorded when the button is pressed, which is "opened", not "sent"; the history wording says so.
- Templates have three fixed slots; no custom slots or deletion (D-046). Owners-only applies to the time zone; any member can edit templates (one member per workspace today).
- A throwaway test account exists in the local development database only.
- golangci-lint is not installed locally; `make lint` builds it through `go run` (minutes on first use).

## Decisions awaiting approval

- Commit the M4 change set (proposed message in the session report) and merge `m4-followups-whatsapp` into `main`.
- D-043..D-049 recorded as routine decisions within the approved plan; say so if any should change.

## Next three actions

1. Founder reviews the M4 report, approves the commit; Claude commits, merges to `main`, watches the CI run.
2. Milestone 5 plan: quotations (items in thousandths, server-authoritative totals, numbering per workspace, share token page with print CSS, PDF generator choice), `{tautan}` placeholder wired to the share link, `quotation_*` activity kinds.
3. Founder: Day 1 outreach, Gate G1 (2026-10-18), decide whether the GitHub repository should stay public.
