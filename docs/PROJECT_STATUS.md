# Project Status

Updated: 2026-10-13 (Milestone 5 merged and on GitHub with CI green; Milestone 6 implemented, tested and reviewed on its feature branch, awaiting the founder's review)

## Current phase

Build-first MVP (D-019). Milestones 1–5 are merged into `main` (M5 merge `e2f0e06`, pushed, CI run #4 green). **Milestone 6 (invoices, DP/instalments, balances, dashboard) is implemented and tested on branch `m6-invoices-payments`; the change set is uncommitted, not merged and not pushed, pending the founder's review.** Next: Milestone 7 (security review, e2e tests, deployment prep, backup/restore, pilot readiness). Validation track continues in parallel; Gate G1 checkpoint remains Sunday 2026-10-18.

## What works now (try it locally)

- From a won lead's page: "Buat tagihan dari penawaran #000N" (items, discount and tax copied from the accepted quotation; editable while a draft) or "Buat tagihan kosong". Due date pre-filled two weeks ahead; "Terbitkan" locks the invoice, mints the customer link and opens the payment form.
- Invoice detail (`/app/tagihan/{id}`): total, remaining balance, due date, open tracking; share link (cabut / buat baru); "Unduh PDF"; WhatsApp with `{nilai}` = balance and `{tautan}` = invoice link; payments table with "Batalkan" (reason required) and a "Catat pembayaran" form (DP / Cicilan / Pelunasan; transfer / tunai / lainnya; date; note) that says the record is the owner's note, not proof of transfer; "Batalkan tagihan".
- Receivables list (`/app/tagihan`): unpaid invoices overdue first with "Total sisa", chips Belum lunas / Terlambat / Lunas / Draf / Dibatalkan / Semua, search by number or customer.
- Customer view (`/t/{token}`): items, totals, "Sudah dibayar", "Sisa", counting payments (date, kind, amount), print CSS and PDF; 404 for drafts, void, revoked and unknown links.
- Dashboard: "Piutang" card (outstanding, count, overdue, received this month) and "Penawaran per status"; follow-up page's Terlambat tab lists "Tagihan jatuh tempo".
- Lead history shows "Tagihan #0001 dibuat / diterbitkan / dibatalkan" and "Pembayaran Rp… dicatat / dibatalkan".

```bash
cp .env.example .env            # once
docker compose up -d db
export PATH=$HOME/sdk/go1.27.2/bin:$PATH
make migrate && make run        # http://localhost:8080 → /app/prospek → a won lead → "Buat tagihan dari penawaran"
make test-db                    # full suite against Postgres (two packages at a time)
make lint                       # golangci-lint v2.14.0 (go run on first use)
```

## Change set on `m6-invoices-payments` (uncommitted, from `main` e2f0e06)

| Area | Files |
|---|---|
| Schema | `internal/db/migrations/00006_invoices_payments.sql` (`invoice_sequences`, `invoices`, `invoice_items`, append-only `payments` with trigger, `payment_reversals`, five activity kinds, indexes; Down with data present); `migrate_m6_test.go` (fresh database at v5 with history and a quotation → v6: constraints, trigger, unique key, one reversal per payment → v5 → v6); `migrate_test.go` (≥ 6); `migrate_m5_test.go` pinned to version 5; `tenant_scope_test.go` now scans quotations, quotation_items, invoices, invoice_items, payments, payment_reversals (the M5 status wrongly claimed the quotation tables were covered; they are now) |
| Domain | `internal/invoice` (`invoice.go`: statuses, validation, idempotency keys; `store.go`: create from quotation or blank, update, issue, void, share, public lookup, payments, reversals, list, overdue, summary; `pdf.go`); `internal/docpdf` (A4 layout extracted from `internal/quotation/pdf.go`); `internal/quotation` (`RenderPDF` wrapper + `TotalLines`, `CountByStatus`, `RevokeShare`/`RenewShare` as single scoped statements); `internal/lead` (five kinds, `ActivityData.Amount/Label`, history lines) |
| Web | `invoice_handlers.go`; `public_handlers.go` (`/t/{token}`, `/t/{token}/pdf`); `middleware.go` (logs `/t/{token}`); `router.go` (`Deps.Invoices`, routes); `templates.go` (pages, `iBadgeClass`, `tanggalTgl`, `rupiahPlain`, `qStatuses`); `app_handlers.go` + `beranda.html` (Piutang, Penawaran per status); `lead_handlers.go` + `prospek_detail.html` (Tagihan section); `followup_handlers.go` + `followup_daftar.html` (Tagihan jatuh tempo); `layouts/app.html` (nav "Tagihan"; the mobile bottom nav scrolls instead of clipping its seven links); templates `tagihan_daftar/form/detail.html`, `share_tagihan.html`; `partials/item_editor.html` (row, add-row fragment and preview script shared with the quotation form); `static/app.css` rebuilt; wiring in `cmd/server/main.go` and `app_test.go` |
| Tests | `internal/invoice/{invoice,store,pdf}_test.go` (validation tables; copy from quotation; draft/issue/void rules; balance SQL = Go; overpayment; idempotency; two concurrent payments; reversal; share lifecycle; lists, overdue and summary reconciliation; cross-workspace; PDF); `internal/docpdf/docpdf_test.go`; `internal/web/invoice_web_test.go` (10: create from quotation → issue → pay → Lunas with history; double submit; validation and overpayment messages; reversal; void rules; draft edit/stale/issue rules; list chips, search, share actions, PDF; CSRF, foreign ids, anonymous, mass assignment; follow-up page and dashboard reconciliation; WhatsApp balance and link); `public_web_test.go` (+3: public page and PDF without a session and without leakage; link stops after revoke and void; rate limit); `middleware_test.go` (`/t/` masking); quotation `CountByStatus` test; lead `Describe` cases |
| Docs | plan `docs/superpowers/plans/2026-10-13-m6-invoices-payments.md`; `DECISIONS` D-056..D-060; `RISK_REGISTER` R-29..R-31; `ARCHITECTURE` §3, §6, §8; `MVP_PLAN` M6 status; `DEVELOPMENT`; this file |

## Tests executed (2026-10-13, Go 1.27.2 darwin/arm64, Postgres 17 in Docker)

| Command | Result |
|---|---|
| `REQUIRE_DB=1 go test -race -count=1 -p 1 ./...` on the M6 branch, after the review fixes | 14 packages ok; **257 tests passed, 0 failed, 0 skipped** (M1–M5 regression included; 253 before the four regression tests from the review) |
| Same command on `main` after the M5 merge (before the push) | 12 packages ok; 202 passed, 0 failed, 0 skipped |
| GitHub Actions on `main` `e2f0e06` | run #4 success (vet, golangci-lint, tests with Postgres service, govulncheck) |
| Hand-computed money cases (M6) | invoice of Rp504.493: DP Rp200.000 → balance Rp304.493; Rp504.494 refused with "Jumlah melebihi sisa tagihan (Rp504.493)."; pelunasan Rp304.493 → Lunas; reversal of Rp100.000 restores the balance; `Total − Σ payments + Σ reversed` from Go equals the SQL balance for paid, partial and untouched fixtures; summary outstanding Rp808.986 = list total, overdue 1 of 2, received this month Rp704.493 |
| Concurrency | `TestConcurrentPaymentsSerializeOnTheInvoice`: two × 60 % → one payment and one overpayment refusal; two × 40 % → both persist; balance never below zero |
| Idempotency | `TestRecordPaymentIsIdempotentOnTheKey` (store) and `TestPaymentDoubleSubmitRecordsOnce` (web): one row, one history line, "Pembayaran ini sudah tercatat." |
| Migration | `TestMigration6AddsInvoicesAndAppendOnlyPayments` (fresh database, v5 with data → v6 → v5 → v6; trigger refuses UPDATE/DELETE on payments); `TestMigrateConcurrentOnFreshDatabase`; dev database migrated in place (`applied 6`) |
| Tenant isolation | `TestEverySQLOnTenantTablesIsScopedByWorkspace` over all M5 and M6 tables; `TestCrossWorkspaceIsInvisible` (store); `TestInvoiceCSRFForeignIdsAndMassAssignment` (web); public page leakage list (e-mail, lead notes, payment notes, phone, app chrome) |
| `gofmt -l cmd internal`, `go vet ./...`, `git diff --check` | clean |
| golangci-lint v2.14.0 (CI version, via `go run`) | 0 issues |
| govulncheck (x/vuln v1.9.0) after the review fixes | 0 vulnerabilities in called symbols or imported packages; **GO-2026-5932** (`x/crypto/openpgp`) remains listed at module level, no fixed version, not imported |
| Live check (Claude's browser against the dev server on port 8089; 8080 held by another local project) | lead page → "Buat tagihan dari penawaran #0001" (rows, 10 %, 11 %, notes pre-filled, preview Rp504.493) → due date set to 12 Oct → Simpan → Terbitkan: status Terlambat, link `/t/…`; DP Rp200.000 recorded → Sisa Rp304.493, row with note and recorder; the same form re-posted with the same key via `fetch` → still one payment; public page and PDF fetched with curl without a cookie: `no-store`, Sudah dibayar / Sisa, no e-mail, phone, payment note or app chrome; wrong token 404; dashboard Piutang Rp304.493, 1 terlambat, Diterima bulan ini Rp200.000, Penawaran per status; follow-up Terlambat tab shows "Tagihan jatuh tempo"; 375 px viewport: detail, share page, dashboard and list without horizontal page scroll; mobile bottom nav scrolls and reaches all seven links |
| Not run | Playwright (M7) |

## Security review (M6)

Independent review by a fresh-context reviewer agent (a separate Claude instance that saw only the repository and the plan; not a human reviewer). It read the whole diff and every new file, built and vetted, ran the database suites for invoice, db, quotation, lead and web with `-race`, and reproduced one finding with a raw-SQL probe against the test database. Verdict: "ready with fixes"; no Critical finding.

| # | Severity | Finding | Status |
|---|---|---|---|
| 1 | Important | Void-while-paying could deadlock: the payment path held the invoice row and then inserted a history line (KEY SHARE on the lead through the foreign key) while Void held the lead `FOR UPDATE` and waited for the invoice; reproduced as SQLSTATE 40P01 | **Fixed**: the invoice store's lead lock is `FOR NO KEY UPDATE` (compatible with KEY SHARE, still serializes lead-first writers); `TestVoidWhilePayingDoesNotDeadlock` drives the exact interleaving (invoice locked by a half-way payment, void queued, history insert) and fails with 40P01 under the old lock mode (checked by mutation) |
| 2 | Important | "Buat tagihan dari penawaran" promised editable rows but the store replaced the typed form with a copy of the quotation, so an edited first-phase bill was saved with the full figures | **Fixed**: the store saves the typed form and only verifies that the quotation belongs to the lead (provenance); `TestCreateFromQuotationRecordsProvenanceAndSavesWhatWasTyped` and `TestInvoiceFromQuotationHonoursEdits`; D-056 reworded |
| 3 | Minor | An idempotency key from one invoice posted to another returned the first invoice's payment as "sudah tercatat" | **Fixed**: `ErrKeyMismatch` → "Formulir kedaluwarsa, muat ulang halaman."; `TestRecordPaymentRefusesAKeyFromAnotherInvoice` and a web case |
| 4 | Minor | A stale payment form posted after the invoice settled produced no message (the form had disappeared) | **Fixed**: flash "Tagihan ini sudah lunas; tidak ada sisa untuk dibayar."; web case |
| 5 | Minor | Seven links in the mobile bottom bar scroll without a visual cue | Open, recorded as R-33 (the bar scrolls; checked at 375 px) |
| 6 | Minor | The append-only trigger plus non-cascading FKs block deleting a workspace, customer or lead with payments | Open for M7 (deletion and retention), recorded as R-32 |
| 7 | Minor | Test gaps for #1–#3 | Closed by the tests above |
| — | Recommendation | Map deadlock / serialization failures to a "coba lagi" page | **Done**: `serverError` answers 409 "Perubahan bertabrakan … coba lagi" for SQLSTATE 40P01 and 40001 and logs a warning; `TestServerErrorMapsTransactionConflictsToRetry` |

Declined by the reviewer as out of scope: instalment schedules, faktur pajak, bank integration (deferred by the spec); no `follow_ups.invoice_id` (D-059); any quotation of the lead accepted as a source via URL (the lead page offers accepted ones); `/app/penawaran/baris` reused for both editors; R-27, R-28, R-30 as documented; drafts' edits not written to history.

## Known defects and open items

- Payments are the owner's notes (R-29); a duplicate entry from two different form renders is visible but not blocked (R-30).
- Deleting a workspace, customer or lead that has payments is blocked at the database until M7 adds a purge path (R-32); the mobile bottom bar scrolls without a visual cue (R-33).
- Rolling back 00006 deletes invoice and payment data (R-31); production rollbacks are backup restores (M7).
- "Dibuka N kali" on invoices has the same link-preview caveat as quotations (R-27); `{tautan}` on the lead page still points at the latest sent quotation (R-28).
- The share page and PDF compute dates in WIB regardless of the workspace zone (D-052, D-058).
- Port 8080 on this laptop was taken by another local project during the live checks; the server was run on 8089.

## Decisions awaiting approval

- Commit the M6 change set and merge `m6-invoices-payments` into `main` (proposed commit message in the session report).
- D-056..D-060 recorded as routine decisions within the approved scope; say so if any should change.

## Next three actions

1. Founder reviews the M6 report, approves the commit; Claude commits, merges to `main`, pushes, watches the CI run.
2. Milestone 7 plan: `THREAT_MODEL.md` checklist, Playwright suite for the critical flow (sign-up → lead → follow-up → quotation link → invoice → payment → dashboard), Dockerfile and `DEPLOYMENT.md`, backup/restore drill, error monitoring (needs approval), workspace export and deletion, demo seed, pilot onboarding checklist.
3. Founder: Day 1 outreach, Gate G1 (2026-10-18), decide whether the GitHub repository should stay public.
