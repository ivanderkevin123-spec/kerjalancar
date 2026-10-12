# Milestone 5 — Quotations: items, totals, share link, PDF (scope and plan)

> Branch `m5-quotations` from `main` 19a07ff (M4 merged, CI #3 green). Same rules as M3/M4: TDD per task, no new dependency beyond the PDF library D-020 already reserved, no commit without approval, independent review before the milestone is declared done.

## Tujuan dan masalah pengguna

**Goal.** The owner turns a lead into a quotation with line items in a few minutes on a phone, sends it to the customer as a link or PDF through WhatsApp, sees whether it was opened, and records the outcome; accepting a quotation wins the lead.

**Problem solved.** Today (M4) the owner can chase a lead but has nothing to send: quotations live in Excel or a notebook, totals are retyped, the customer gets a photo, and nobody knows whether it was read. M5 gives one source of truth for the offer with server-computed totals in integer rupiah (D-010), a shareable page, a PDF, and the outcome recorded in the lead history.

## Fitur dan kriteria penerimaan

| Feature | Acceptance |
|---|---|
| Create a quotation from a lead (`/app/penawaran/baru?prospek=<id>`) with items: description, quantity (thousandths, typed as `2,5` or `2.5`), unit, unit price (rupiah); optional discount % and tax % (integers 0–100); valid-until date; notes | Totals are computed on the server from the saved items and match a hand-computed table to the rupiah, including qty 2.5 × Rp1.999 and a 10 % discount + 11 % tax case; a form whose totals field is tampered is ignored; validation keeps every row |
| Number per workspace (`0001`, `0002`, …) unique under concurrent creation | 10 concurrent creations in one workspace get 10 distinct numbers (sequence row locked `FOR UPDATE`, `UNIQUE (workspace_id, seq)`) |
| Edit while `draft` or `sent` (items, discount, tax, valid-until, notes); locked once `accepted`/`rejected` | Editing recomputes and re-stores totals; an accepted quotation's edit routes answer the detail page with a message |
| Status: `draft` → `sent` (creates the share link) → `accepted` or `rejected`; `expired` is a computed state (sent and valid-until before today) | Accepting moves the lead to `menang` through the existing status rules (follow-up auto-closed, history line); rejecting records a history line; both are idempotent with a message |
| Share link `/p/{token}` (128-bit random, base64url) with print CSS and "dibuka" tracking (first open, count) | Unknown, revoked or expired token → 404 page; no session needed; `Cache-Control: no-store`; rate-limited per client address; the page shows only the quotation (workspace name, number, items, totals, validity, notes), never other data; revoke makes the old link 404 and "buat tautan baru" issues a new one |
| PDF (`/app/penawaran/{id}/pdf` and `/p/{token}/pdf`) via `github.com/go-pdf/fpdf` v0.9.0 (pure Go, MIT) | A4, workspace name, number, dates, customer, items table, totals, notes; `Content-Disposition: attachment`; a test checks the text content with compression off; generation bounded by the request timeout |
| Lead integration | Lead detail lists its quotations with status; the WhatsApp `{tautan}` placeholder renders the share URL of the latest sent quotation of the lead (empty otherwise, D-047); history lines `quotation_sent`, `quotation_accepted`, `quotation_rejected` with number and total |
| List `/app/penawaran` with status chips, search by number or customer, 25 per page | Same list conventions as prospek |

Out of scope (unchanged): full RAB, revision history with diffs, pekerjaan tambah-kurang, e-signature, invoices and payments (M6), email.

## Perubahan database — migration 00005

- `lead_activity_kinds` lookup table (all kinds so far plus the three quotation kinds); `lead_activities.kind` becomes a foreign key and the CHECK constraint is dropped, so future kinds are rows, not constraint rewrites (D-053).
- `quotation_sequences (workspace_id PK, last_seq)`.
- `quotations`: `id`, `workspace_id`, `lead_id`, `customer_id` (composite FKs), `seq`, `status` CHECK, `valid_until date`, `notes` (≤ 2000), `discount_pct`/`tax_pct` smallint CHECK 0–100, `subtotal`/`discount_amount`/`tax_amount`/`total` bigint CHECK ≥ 0, `share_token` text UNIQUE (nullable), `share_revoked_at`, `first_opened_at`, `open_count`, `sent_at`, `accepted_at`, `rejected_at`, `version` int (optimistic), `created_by`, timestamps; `UNIQUE (workspace_id, id)`, `UNIQUE (workspace_id, seq)`; indexes on `(workspace_id, lead_id)`, `(workspace_id, status, created_at DESC)`, partial on `share_token`.
- `quotation_items`: `id`, `workspace_id`, `quotation_id` (composite FK, cascade), `position`, `description` (1–300), `qty_milli` bigint CHECK 1..1 000 000 000, `unit` (≤ 20), `unit_price` bigint CHECK 0..10^13, `line_total` bigint; `UNIQUE (workspace_id, id)`; index `(workspace_id, quotation_id, position)`.
- Quantities as `bigint` thousandths rather than `NUMERIC(12,3)` (ARCHITECTURE §6 wording): exact, no pgx numeric handling, same scale (D-054).
- Migrations 00001–00004 untouched. Migration test: fresh database staged at version 4 with activity rows → migrate → kinds table populated, FK in place, old rows intact, inserting an unknown kind fails.

## Dampak pada fitur yang ada

- `lead`: new activity kinds and Describe lines; `AppendActivity` unchanged; accept uses `lead.Store.ChangeStatus` (hooks fire as in M4).
- `web`: new routes under `/app/penawaran`, public `/p/*` group outside the session (rate limit, no-store, 404 copy in Indonesian); lead detail gains a "Penawaran" section; `waData` fills `{tautan}`; the `followup_penawaran` seed text is unchanged (the owner may add `{tautan}`); the template editor help already documents `{tautan}`.
- `money`: `LineTotal`, `PercentOf`, `ParseQty`, `FormatQty`.
- No change to customers, follow-ups, templates, sessions, CSRF (the `/app` group already covers the new POSTs; public routes are GET only).

## Tes yang diperlukan

Money tables (line totals with half-up rounding, percentages, qty parsing with comma and dot, bounds); quotation validation (empty items, blank description, qty 0, 101 % discount, bad dates); store: create with items in one transaction (invalid row → nothing written), sequence uniqueness under 10 concurrent creators, update recomputes totals and refuses locked statuses, send/accept/reject transitions and idempotency, accept → lead `menang` + follow-up closed + history, token lifecycle (send issues, revoke hides, regenerate replaces, open tracking), `GetByToken` refuses unknown/revoked/expired, cross-workspace matrix; PDF text test; web: pages render, htmx add-row fragment, validation keeps rows, totals from server not form, share page without session, 404 cases, `no-store`, rate limit, PDF headers, accept/reject flows, lead detail section, `{tautan}` in the WhatsApp link, tenant matrix, CSRF for every new POST, mass-assignment negatives (`seq`, `total`, `share_token`, `status` in forms ignored), unauthenticated access; migration test; regression M1–M4.

## Tasks

A. `money` additions · B. migration 00005 + `quotation` domain (types, validation, totals) · C. `quotation` store (create, update, get, list, send, accept, reject, token lifecycle, open tracking) · D. web: list, form (htmx rows), detail, status actions, lead section · E. public share page + PDF · F. WhatsApp `{tautan}`, tenant/CSRF/mass-assignment tests · G. regression, lint, independent review, docs.

## Decisions to record

D-050 quotation model and status rules · D-051 numbering per workspace · D-052 share links (token, revoke, expiry, open tracking, rate limit) · D-053 activity kinds lookup table · D-054 quantities as bigint thousandths · D-055 PDF library go-pdf/fpdf v0.9.0 (v1.4.x retracted upstream).
