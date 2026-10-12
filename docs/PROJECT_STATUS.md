# Project Status

Updated: 2026-10-13 (Milestone 5 implemented, reviewed and fixed; awaiting the founder's commit approval)

## Current phase

Build-first MVP (D-019). Milestones 1–4 are merged into `main` (M4: `19a07ff`, CI run #3 green). **Milestone 5 (quotations with items, totals, share link, PDF) is implemented and tested on branch `m5-quotations`; the whole change set is uncommitted and awaits the founder's review.** Next: Milestone 6 (invoices, DP/installments, balances, dashboard). Validation track continues in parallel; Gate G1 checkpoint remains Sunday 2026-10-18.

## What works now (try it locally)

- From a lead's page, "Buat penawaran" opens an editor: rows of description, quantity (decimal comma allowed, e.g. `2,5`), unit, unit price; "Tambah baris" via htmx; discount % and tax %; valid-until (pre-filled two weeks ahead); notes. Totals are computed on the server from the saved rows and shown live as a preview while typing.
- Quotation detail (`/app/penawaran/{id}`): items and totals, status badge (Draf, Terkirim, Diterima, Ditolak, Kedaluwarsa), "Kirim" creates the customer link, "Cabut tautan" / "Buat tautan baru", "Tandai diterima" (lead becomes Menang, its follow-up closes) / "Tandai ditolak", "Unduh PDF", WhatsApp button with the template select; "Dibuka N kali, pertama …" once the customer opened the link.
- Customer view (`/p/{token}`): no login, print-friendly page with "Unduh PDF" and "Cetak"; unknown, revoked or expired links show a 404 page (expired ones say the offer is no longer valid).
- List (`/app/penawaran`) with status chips and search by number or customer; lead page lists its quotations; the lead history shows "Penawaran #0001 dibuat/dikirim/diterima/ditolak" with the total.
- WhatsApp templates: `{tautan}` is filled with the link of the lead's most recently sent quotation.

```bash
cp .env.example .env            # once
docker compose up -d db
export PATH=$HOME/sdk/go1.27.2/bin:$PATH
make migrate && make run        # http://localhost:8080 → /app/prospek → a lead → "Buat penawaran"
make test-db                    # full suite against Postgres (two packages at a time)
make lint                       # golangci-lint v2.14.0 (go run on first use)
```

## Change set on `m5-quotations` (uncommitted, from `main` 19a07ff)

| Area | Files |
|---|---|
| Dependency | `github.com/go-pdf/fpdf` v0.9.0 (pure Go, MIT) in `go.mod`/`go.sum`; the v1.4.x tags are retracted upstream (D-055) |
| Schema | `internal/db/migrations/00005_quotations.sql` (`lead_activity_kinds` lookup + FK replacing the CHECK, `quotation_sequences`, `quotations`, `quotation_items`, indexes); `migrate_test.go` (≥ 5); `migrate_m5_test.go` (fresh database staged at v4 with history rows → migrate: kinds seeded, old rows intact, unknown kind refused, numbering unique, qty > 0) |
| Domain | `internal/money` (`LineTotal`, `PercentOf`, `ParseQty`, `FormatQty`); `internal/quotation` (`quotation.go`: statuses, validation, totals; `store.go`: create/update/send/accept/reject/share/public lookup/list; `pdf.go`); `internal/lead` (`ChangeStatusTx`, quotation activity kinds and lines) |
| Web | `quotation_handlers.go`, `public_handlers.go`; routes under `/app/penawaran` and the public `/p/{token}` group (rate limit 60/min per client address, `no-store`); `Deps.Quotations`; lead detail quotation section; `waData` fills `{tautan}`; templates `penawaran_daftar/form/detail.html`, `layouts/share.html`, `share_penawaran.html`; nav entry; rebuilt `static/app.css`; wiring in `cmd/server/main.go` |
| Tests | money tables; `quotation_test.go` (validation, hand-computed totals, effective status), `store_test.go` (transactions, 10 concurrent creators, update/stale/locked, send/token/open tracking/revoke/renew/expiry, accept wins lead + closes follow-up + history, lost lead refuses accept, reject, lists and search, cross-workspace); `pdf_test.go` (text content, size/page-count determinism, sample writer); web: `quotation_web_test.go` (10) and `public_web_test.go` (3) incl. CSRF, foreign ids, anonymous access, tampered totals/status/token ignored, no leakage on the public page, rate limit; lead `Describe` cases |
| Docs | plan `docs/superpowers/plans/2026-10-13-m5-quotations.md`; `DECISIONS` D-050..D-055; `ARCHITECTURE` §3, §4, §6, §8; `RISK_REGISTER` R-24..R-28; `MVP_PLAN` M5 status; `DEVELOPMENT`; this file |

## Tests executed (2026-10-13, Go 1.27.2 darwin/arm64, Postgres 17 in Docker)

| Command | Result |
|---|---|
| `REQUIRE_DB=1 go test -race -count=1 -p 1 ./...` | 12 packages ok; **202 tests passed, 0 failed, 0 skipped** (M1–M4 regression included; rerun after the review fixes) |
| Hand-computed money cases | 2.5 × Rp1.999 = Rp4.998; subtotal Rp504.998, 10 % discount Rp50.500, 11 % tax Rp49.995, total Rp504.493; 15.750.000 + 11 % = 17.482.500; 0.333 × 1.000 = 333; full discount → 0; stored subtotal equals the sum of stored lines (asserted after a round trip) |
| Numbering under concurrency | `TestNumbersAreUniqueUnderConcurrentCreation`: 10 goroutines, 10 distinct numbers |
| Migration from v4 with data, and back | `TestMigration5KeepsHistoryAndAddsQuotations` on a fresh database: up from v4 with history rows, then down to v4 with quotation history present (older lines survive, quotation lines are dropped), then up again; `TestMigrateConcurrentOnFreshDatabase`; the dev and test databases migrated in place (`applied 5`) |
| Tenant isolation | `TestEverySQLOnTenantTablesIsScopedByWorkspace` (now also covers `quotations` and `quotation_items`), store and web cross-workspace tests, public page leakage checks (no e-mail, no lead notes, no app chrome) |
| Lock order | lead → quotation (→ follow-up through the lead hooks); Accept runs `ChangeStatusTx` inside the quotation transaction |
| `gofmt -l cmd internal`, `go vet ./...`, `git diff --check` | clean |
| golangci-lint v2.14.0 (CI version, via `go run`) | 0 issues, rerun after the review fixes (earlier fixes: two `nolint` with reasons for a fixed-host redirect and a temp-dir file, `f.Stat()` instead of `os.Stat(path)`, an ineffectual assignment) |
| govulncheck (x/vuln v1.9.0, database fetched 2026-10-13) | 0 vulnerabilities in called symbols or imported packages, `go-pdf/fpdf` scanned; **GO-2026-5932** (`x/crypto/openpgp`) remains listed at module level, no fixed version, not imported |
| PDF sample | rendered to the scratchpad and viewed: header, recipient, items table with wrapped descriptions, totals block, notes, footer; after the fixes the public PDF of the live quotation was decompressed and checked: no phone digits, date in WIB, effective status |
| Live check (Claude's browser against the dev server on port 8089, since 8080 was held by another local project) | from the lead page: new form with four rows (one added through htmx), decimal quantity `2,5`, 10 % discount, 11 % tax; live preview and saved detail both show Rp504.493; Kirim produced the link; the link and its PDF answered without any cookie with `Cache-Control: no-store`, no app chrome, no e-mail or lead notes in the page; detail then showed "Dibuka 1 kali"; Tandai diterima made the lead Menang, closed its follow-up and wrote the three history lines; list page and status chips rendered; wrong or short tokens gave 404; the app PDF without a session redirected to /masuk; share page and form fit a 375 px viewport without horizontal page scroll |
| Not run | Playwright (M7) |

## Security review (M5)

Independent review by a fresh-context reviewer agent (a separate Claude instance that saw only the repository and the plan; not a human reviewer). It read the whole diff and every new file, built and vetted offline, ran the pure tests and an empirical check of fpdf's byte encoding; it could not run the database suite, lint or govulncheck in its sandbox. Verdict: "ready with fixes"; no Critical finding.

| # | Severity | Finding | Status |
|---|---|---|---|
| 1 | Important | `latin1()` wrote UTF-8 bytes into a single-byte (cp1252) core font, so "Café" printed as "CafÃ©" and widths were wrong | **Fixed**: text goes through fpdf's cp1252 translator, unmapped runes become `?`; `TestRenderPDFEncodesTextForTheCoreFont` pins the bytes (`é` → `e9`, en dash → `96`, emoji → `?`) |
| 2 | Important | The PDF printed the customer's phone and the same bytes are served from the public link; a forwarded link leaked the number | **Fixed**: phone removed from the PDF; `TestRenderPDFOmitsTheCustomerPhone`; the public-page leakage list now includes the phone |
| 3 | Important | The PDF date used the process zone, so a quotation created before 07:00 WIB on a UTC server was dated the previous day | **Fixed**: `PDFOptions.Loc` (workspace zone from the app, WIB from the public link) and `PDFOptions.Today`; `TestRenderPDFUsesTheGivenZoneDayAndKeepsNoteLines` checks WIB and UTC renderings |
| 4 | Minor | R-24 claimed a sentence on the detail page that was not there | **Fixed**: the share box says the link always shows the latest saved version |
| 5 | Minor | Share tokens in the application's request and error logs | **Fixed**: paths are logged as `/p/{token}`; `TestRequestLoggerMasksShareTokens`; R-25 lowered, D-052 updated |
| 7 | Minor | Down migration 00005 failed once quotation history existed | **Fixed**: the Down step deletes `quotation_*` history lines before restoring the CHECK (documented); `MigrateTo` can roll back; the migration test exercises down-then-up |
| 8 | Minor | The browser preview read "2.5" as 25 while the server reads it as 2.5 | **Fixed**: the preview parses quantities like `money.ParseQty`; checked live (1.5 and 2,5 both preview correctly) |
| 11 | Minor | Dead `updated` variable in the edit handler | **Fixed** (no `quotation_updated` history line yet, see open items) |
| 12 | Minor | PDF printed the stored status (an expired offer said "Terkirim") and flattened note line breaks | **Fixed** with #3 |
| 13 | Minor | PDF handler copied the buffer; list pager emitted an empty `q=` | Buffer copy fixed; empty `q=` left as is |
| 6 | Minor | "Dibuka N kali" counts messengers' link-preview fetches | Open, recorded as R-27 |
| 9 | Minor | `{tautan}` can carry an expired or rejected quotation's link | Open, recorded as R-28 |
| 10 | Minor | "Buat tautan baru" on a draft mints a token early (harmless: drafts never resolve publicly) | Open |
| 14 | Minor | A past "berlaku sampai" is accepted, producing a link that answers "tidak berlaku lagi" at once | Open |
| 15 | Minor | Creating a quotation for a closed lead is hidden in the UI but allowed by the store | Open, product decision |
| 16 | Minor | Test gaps: no DB-level "invalid row → nothing written" test | Open; the PDF encoding and phone gaps are closed |

Declined by the reviewer as out of scope: rate-limit keying behind NAT (deployment, D-042), single-instance limiter, lead title on the public page, accepting a never-sent draft, quotations for closed leads, numbers beyond 9999, substring number search.

## Known defects and open items

- Editing a sent quotation changes what the customer's link shows; the detail page now says so, revision history is deferred until pilots ask (R-24).
- Share tokens stay in a reverse proxy's access log and browser history; the application's own logs mask them (R-25).
- PDF text outside cp1252 (emoji, arrows) renders as `?` (R-26); a Unicode font is a follow-up.
- "Dibuka N kali" may count a messenger's link preview (R-27); `{tautan}` may point at an expired or rejected quotation (R-28).
- Editing records no history line (`quotation_updated`), "Buat tautan baru" works on drafts, a past "berlaku sampai" is accepted, and the store allows quotations for closed leads (reviewer minors 10, 11, 14, 15).
- The share page and PDF compute expiry in WIB regardless of the workspace zone (D-052).
- Port 8080 on this laptop was taken by another local project during the live check; the server was run on 8089.

## Decisions awaiting approval

- Commit the M5 change set (proposed message in the session report) and merge `m5-quotations` into `main`.
- D-050..D-055 recorded as routine decisions within the approved scope; say so if any should change.

## Next three actions

1. Founder reviews the M5 report, approves the commit; Claude commits, merges to `main`, watches the CI run.
2. Milestone 6 plan: invoices from quotations (number per workspace, items, due date), append-only payments (DP/cicilan/pelunasan, idempotency key, reversals), balances in SQL, receivables list, dashboard read models, the "Pengingat DP/pelunasan" templates wired to invoices.
3. Founder: Day 1 outreach, Gate G1 (2026-10-18), decide whether the GitHub repository should stay public.
