# Risk Register

Likelihood and impact: L / M / H. Owner is the founder unless noted. Review at every gate.

## Top five risks

### R-01 Double entry kills adoption (L: H · I: H)

The conversation lives in WhatsApp. Re-typing name, number, and need into another app is friction the free "do nothing" option does not have.

Mitigations (v1): lead capture in one screen with three fields (name, WA number, need) and a default follow-up date; measure time-to-record and weekly writes from pilot day 1; test the behavior before software with the concierge test (`CONCIERGE_TEST.md`). Candidate mitigations, only if evidence demands: Android share-sheet into an installable PWA so a WA contact or message becomes a lead; paste-a-message parser using plain regex for phone numbers (no AI).
Trigger to act: median time-to-record > 20 s or < 50% of pilots write weekly by Week 7.

### R-02 Willingness to pay is below a viable price (L: M · I: H)

Micro businesses may value the tool at Rp0–50,000/month, below what sustains support and hosting.

Mitigations: ask for a commitment with a price and a start date in every interview; run pilots as paid from month one (discounted, not free); ROI framing in onboarding ("satu proyek yang tidak hilang"); test two anchors (Rp149k, Rp249k).
Trigger: < 3 level-3 commitments at G1, or < 2 paid month-2 renewals at G2.

### R-03 Wrong niche or a niche that is really three (L: M · I: H)

Contractors may need RAB depth; wedding vendors need a calendar; a generic tool is nobody's first choice.

Mitigations: parallel A/B interviews with the same scoring sheet; strict MVP boundary with a "not in v1" list; segment-specific features only after G1 names the segment; narrow rather than widen when signals split.
Trigger: top-3 feature requests differ by segment and fall outside MVP.

### R-04 Cross-tenant data leakage or an auth defect (L: M · I: H)

A single bug exposing one business's customers to another ends the product's reputation in a referral-driven market.

Mitigations: workspace_id on every tenant table with composite foreign keys so a child row cannot reference another workspace's parent; workspace resolved from the session, never from the request; repository layer requires workspace_id on every query; generated cross-tenant test matrix (read, update, delete, list) for every resource; CSRF, secure cookies, Argon2id, rate limits on login and share links; threat model before staging; consider Postgres RLS in Week 7.
Trigger: any cross-tenant test failure blocks merge.

### R-05 Scope creep from the segment's "real" workflow (L: H · I: M)

RAB with volumes and unit analysis, progress billing, pekerjaan tambah-kurang, event calendars, inventory, tax invoices.

Mitigations: `DECISIONS.md` entry required for every scope addition; each slice has a written acceptance criterion; features that interviewees rank below 3 wait for G2; keep accounting out by policy (`PRODUCT_BRIEF.md` §5).
Trigger: any week where planned slice is not shippable by Friday.

## Other risks tracked

| ID | Risk | L | I | Mitigation |
|---|---|---|---|---|
| R-06 | Solo-founder capacity: interviews, building, and selling compete for the same hours | H | M | Week 0 is validation-only; build weeks have one slice each; cut scope before cutting tests |
| R-07 | Personal-data obligations (UU PDP 27/2022) for customers' names and numbers; cross-border hosting | M | M | Verify obligations before staging; privacy notice and data export/delete in Week 7; Singapore region as default pending verification |
| R-08 | WhatsApp changes click-to-chat behavior or policy | L | M | wa.me is an official, long-standing feature; templates stay copy-paste-able as fallback |
| R-09 | Monetary rounding errors or inconsistent totals | M | M | Integer rupiah; quantity with fixed scale; one rounding function with table-driven tests; stored totals recomputed and asserted |
| R-10 | Time-zone mistakes in due dates and "today" lists | M | M | Workspace time zone; DATE columns for due dates; tests at zone boundaries |
| R-11 | Duplicate or lost payment records | M | M | Idempotency key per payment submission; append-only payments with reversal records, never silent edits |
| R-12 | Go toolchain upgrade (1.17.5 → 1.27.x) disturbs the founder's other Go projects | M | L | Install via Homebrew and manage PATH per project, or use `go.mod` toolchain directive; founder decides (D-004) |
| R-13 | Hosting cost or vendor lock-in before revenue | L | L | No provisioning until approved; 12-factor config; plain Postgres; Dockerfile portable across Fly/Railway/Render |
| R-14 | Share-link token leakage exposes a quotation | M | M | 128-bit random tokens, revocable, optional expiry, no customer PII beyond what the quotation already shows |
| R-15 | Pilots churn silently after Week 7 | M | H | Weekly founder call with each pilot; churn reason mandatory; usage dashboard for the founder |
| R-16 | HTMX hits a complexity ceiling (quotation editor, future interactive screens) | M | M | Quotation editor budgeted as one JS island with server-authoritative totals; revisit triggers in `ARCHITECTURE.md` §1.4; escape hatch is `/api/v1` on the same service, not a frontend rewrite mid-roadmap |
| R-17 | Interview notes, prospect lists, and concierge sheets contain personal data of owners and their customers | H | M | Keep in `docs/interviews/` and `docs/private/` (git-ignored, D-017); initials and last-4 digits only in the repo; founder access to concierge sheets revoked at test end (`CONCIERGE_TEST.md` §6) |
| R-18 | Pipeline search uses `ILIKE '%…%'` on title, customer name, company and phone; with tens of thousands of rows per workspace it stops using indexes | L | L | Lists are paginated (25) and filtered by `workspace_id` and status first; measured 87 ms for 200 leads (M3); add `pg_trgm` indexes when a pilot workspace passes ~5,000 rows |
| R-19 | Duplicate contacts despite the phone rule: two name-only customers for one person, or one person with two numbers | M | L | Inline lead creation refuses a known number and pre-selects that customer (D-034); a merge tool waits for pilot evidence |
| R-20 | Activity history grows without bound on long-lived leads; the detail page shows the newest 100 | L | L | Cap in place; paginate the timeline when a pilot lead exceeds it |
| R-21 | A template rendered with empty placeholders reads oddly ("senilai  sudah") when a lead has no estimate | M | L | The lead page and the template editor list which placeholders will be empty; owners see the text before tapping; estimate is one field away |
| R-22 | `leads.next_follow_up_on` (cache) drifts from `follow_ups` after a future code path forgets the store | L | M | Every write goes through `followup` store functions that update both in one transaction; store tests assert the cache after schedule, done, undo and auto-close; a reconciliation query belongs in the M7 checklist |
| R-23 | WhatsApp opens in a new tab on desktop but replaces the app on some Android browsers; the owner must return manually | M | L | The follow-up stays open until "Selesai" is tapped, so nothing is lost; evaluate an in-app "kembali" hint with pilots |
| R-24 | A quotation edited after it was sent silently changes what the customer's link shows (no revision history until pilots ask) | M | M | The detail page says the link always shows the latest saved version; accepted/rejected quotations are locked; revision history is on the deferred list (M5 scope) |
| R-25 | Share tokens appear in a reverse proxy's access log (path) and in browser history; a leaked log or shared device exposes an offer | L | L | The application masks `/p/{token}` in its own request and error logs (tested); tokens are revocable and expire with the quotation; M7 adds log retention and reconsiders moving the token out of the path (D-052) |
| R-26 | PDF text outside the core font's code page (emoji, arrows, most symbols) renders as `?` | M | L | Accented Latin letters and the en dash render correctly (cp1252 translator, tested); a Unicode font embedded in the binary is a follow-up when a pilot needs it (D-055) |
| R-27 | "Dibuka N kali" counts link-preview fetches by messengers (WhatsApp fetches the URL when the owner pastes it), so an offer can show as opened before the customer tapped it | M | L | Shown as information, not as a trigger for any automation; skip known preview user agents or count from a beacon before pilots rely on it (M5 review #6) |
| R-28 | A WhatsApp template's `{tautan}` is the latest sent quotation's link even after that quotation expired or was rejected, so the message can carry a link that answers "tidak berlaku lagi" | M | L | The editor shows the status next to the link; exclude expired and rejected quotations from the lookup in M6 (M5 review #9) |
| R-29 | An owner records a payment that never arrived (typo, wrong invoice) and the dashboard overstates cash received | M | M | Every payment form and the dashboard say the record is the owner's note, not a bank fact (D-011); reversals with a reason keep the history honest; monthly reconciliation against the bank statement is the owner's job and is said so in the UI |
| R-30 | The idempotency key only covers a repeated submission of the same rendered form; two different renders of the form (two tabs, two phones) typing the same transfer record it twice | M | M | Each record shows date, kind, amount and note on the invoice page so a double entry is visible; a reversal corrects it; a duplicate warning (same amount and date within a minute) is a candidate for M7 |
| R-31 | Rolling back migration 00006 deletes invoices, payments, reversals and their history lines | L | H | Down migrations are a development aid; production rollbacks restore from backup (M7 backup/restore drill); the migration comment and `migrate_m6_test.go` state the loss |
| R-32 | The append-only trigger and the non-cascading `payments → invoices` foreign key mean a workspace, customer or lead with any payment cannot be deleted at the database level; account deletion and data retention (D-017, UU PDP) need an explicit purge path | M | M | Planned for M7 together with workspace export and deletion: a purge procedure that drops the trigger inside one transaction, or soft deletion; until then deletion is a support operation (M6 review #6) |
| R-33 | The mobile bottom navigation now holds seven links in a horizontally scrolling bar without a visual cue, so "Pelanggan" and "Pengaturan" start off-screen on a phone | M | L | The bar scrolls (checked at 375 px); icons or a "Lainnya" entry before pilots (M6 review #5) |
