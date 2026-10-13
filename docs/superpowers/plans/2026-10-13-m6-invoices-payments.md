# Milestone 6 — Invoices, DP/installments, balances, dashboard: Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The owner issues an invoice for a won project (from an accepted quotation or blank), records DP, instalments and the final payment as append-only notes with reversals, always sees the outstanding balance per invoice, per customer and in total, and the dashboard shows receivables next to the pipeline and today's follow-ups.

**Architecture:** A new `internal/invoice` package mirrors `internal/quotation` (same store shape: per-workspace sequence upsert, lead-first lock order, composite FKs, share token, `version` column). Items of a source quotation are **copied** into `invoice_items`, never referenced. Payments are append-only rows (a trigger blocks `UPDATE`/`DELETE`); corrections are `payment_reversals` rows; the balance is `total − Σ payments + Σ reversed payments`, computed in SQL and asserted in Go. The PDF layout code moves from `internal/quotation/pdf.go` into a small shared `internal/docpdf` package so quotation and invoice PDFs render the same way; `quotation.RenderPDF` keeps its signature. Web handlers, templates and tests follow the M5 files one for one (`/app/tagihan`, public `/t/{token}`).

**Tech Stack:** Go 1.27.2, chi v5, pgx v5, goose (migration `00006`), html/template + htmx 2.0.11, Tailwind v4 (rebuild `static/app.css`), go-pdf/fpdf v0.9.0 (already a dependency). No new dependencies.

**Spec:** `docs/MVP_PLAN.md` §M6 (outcome, scope, acceptance criteria, deferred list); constraints from `docs/ARCHITECTURE.md` §5–§6 (tenant isolation, money, append-only payments, `FOR UPDATE` on payment inserts, optimistic `version`), `docs/DECISIONS.md` D-010, D-011, D-020, D-043, D-050–D-055, `docs/RISK_REGISTER.md` R-11.

## Global Constraints

- Money is integer rupiah (`int64`, `money.MaxIDR`), quantities are thousandths (`qty_milli`), half-up rounding via `money.LineTotal` / `money.PercentOf` (D-010, D-054).
- KerjaLancar is not accounting software: no ledger, no tax computation beyond a percentage line, no bank data; every payment form says the record is the owner's note, not proof of transfer (D-011).
- Every SQL statement on a tenant table carries `workspace_id`; tenant tables get `UNIQUE (workspace_id, id)` and composite foreign keys (ARCHITECTURE §5); `TestEverySQLOnTenantTablesIsScopedByWorkspace` must list the new tables **and** the M5 tables it currently misses (`quotations`, `quotation_items`).
- Lock order: lead → quotation → invoice → (payments are inserted under the invoice lock); never lock in another order.
- Payments: append-only, idempotency key per form render, overpayment refused with a message, two concurrent payments serialize on `SELECT … FOR UPDATE` of the invoice row (ARCHITECTURE §6, R-11).
- Dates follow the workspace zone in the app (`s.today(ws)`) and WIB on public pages (D-052); `due_on` and `received_on` are `date` columns.
- Indonesian UI, mobile-first (no horizontal page scroll at 375 px), CSRF via the existing `http.CrossOriginProtection`, `no-store` on HTML, public routes rate-limited `60/min` per client address and logged as `/t/{token}`.
- Migration `00006` only; never edit `00001`–`00005`. Down path must work with data present (M5 review #7 convention).
- No new Go dependencies; no production configuration or deployment changes.

## Review Focus

1. A payment posted twice in one second (double tap on a phone) must record one payment and show "Pembayaran ini sudah tercatat." — pinned in Task 3 (`TestRecordPaymentIsIdempotentOnTheKey`) and Task 6 (`TestPaymentDoubleSubmitRecordsOnce`).
2. Two owners recording payments on the same invoice at the same time must never push the balance below zero — Task 3 (`TestConcurrentPaymentsSerializeOnTheInvoice`).
3. Reversing a payment and then recording the same amount again must work (the idempotency key is per submission, not per amount) — Task 3 (`TestReversalFrees TheBalance` in `TestReversePaymentRestoresTheBalance`).
4. An invoice issued with a due date in the past must appear in "Terlambat" immediately and still accept payments — Task 3 (`TestOverdueListsIssuedUnpaidInvoicesPastDue`) and Task 6 (`TestFollowUpPageListsOverdueInvoices`).
5. Voiding an invoice that already has a payment must be refused, and a voided invoice must drop out of receivables and answer 404 on its public link — Task 3 (`TestVoidRefusesInvoicesWithPayments`) and Task 6 (`TestPublicInvoiceLinkStopsAfterVoid`).

---

## File structure

| File | Responsibility |
|---|---|
| `internal/db/migrations/00006_invoices_payments.sql` | tables `invoice_sequences`, `invoices`, `invoice_items`, `payments`, `payment_reversals`; append-only trigger on `payments`; new `lead_activity_kinds`; indexes; Down with data present |
| `internal/db/migrate_m6_test.go` | fresh DB at v5 with data → v6 → v5 → v6 |
| `internal/db/tenant_scope_test.go` | regex extended with `quotations|quotation_items|invoices|invoice_items|payments|payment_reversals` |
| `internal/docpdf/docpdf.go` (+ `_test.go`) | generic A4 document renderer (header, parties, items table, totals, extra lines, notes, footer), cp1252 encoding, zone-aware dates; moved out of `internal/quotation/pdf.go` |
| `internal/quotation/pdf.go` | thin wrapper building a `docpdf.Document`; tests unchanged |
| `internal/invoice/invoice.go` | `Status`, `Invoice`, `Payment`, `Reversal`, `Input`/`Validate` (items via `quotation.Validate`, due date required to issue), `PaymentInput`/`ValidatePayment`, effective status, labels |
| `internal/invoice/store.go` | `NewStore(pool, leads, quotations)`; `Create`, `Get`, `Update`, `Issue`, `Void`, `RevokeShare`, `RenewShare`, `GetByToken`, `RecordOpen`, `RecordPayment`, `ReversePayment`, `List`, `ForLead`, `Overdue`, `Summary`, `LatestShared`, `BalanceSQL` check helper |
| `internal/invoice/pdf.go` | `RenderPDF(w, inv, docpdf.Options)` via `docpdf` |
| `internal/invoice/{invoice,store,pdf}_test.go` | validation tables, balance invariants (SQL = Go), idempotency, concurrency, reversal, overpayment, void rules, lists, overdue, summary reconciliation, cross-tenant, PDF text |
| `internal/lead/activity.go` (+ test) | kinds `invoice_created`, `invoice_issued`, `invoice_voided`, `payment_recorded`, `payment_reversed`; `ActivityData.Amount`, `Describe` lines |
| `internal/web/invoice_handlers.go` | `/app/tagihan…` handlers, `postTagihanWa`, `writeInvoicePDF` |
| `internal/web/public_handlers.go` | `/t/{token}` and `/t/{token}/pdf` |
| `internal/web/middleware.go` | `logPath` masks `/t/` too |
| `internal/web/router.go`, `templates.go`, `cmd/server/main.go`, `app_test.go` | wiring, `Deps.Invoices`, page registry, funcs `iBadgeClass`, `payKindLabel` |
| `internal/web/lead_handlers.go`, `followup_handlers.go`, `app_handlers.go` | lead page "Tagihan" section; Terlambat tab lists overdue invoices; dashboard receivables + quotation counts |
| templates `tagihan_daftar.html`, `tagihan_form.html`, `tagihan_detail.html`, `share_tagihan.html`; edits to `prospek_detail.html`, `followup_daftar.html`, `beranda.html`, `layouts/app.html` | UI |
| `internal/web/{invoice_web_test,public_web_test}.go` | handler tests incl. CSRF, foreign ids, anonymous, mass assignment, double submit, overpayment message, reversal, public leakage, rate limit, dashboard reconciliation |
| docs | `DECISIONS` D-056–D-060, `RISK_REGISTER`, `ARCHITECTURE` §6/§8, `MVP_PLAN` M6 status, `PROJECT_STATUS`, `DEVELOPMENT` |

## Domain rules (the spec made concrete)

- An invoice belongs to one lead and that lead's customer, optionally records `quotation_id` (source; items copied at creation). Allowed on any lead that is not `kalah`.
- Stored statuses: `draft` (editable: items, discount, tax, due date, notes), `issued` (locked; share link; payments allowed), `void` (no payments existed; link revoked). Effective status for display: `draft` → Draf, `void` → Dibatalkan, `issued` + balance 0 → Lunas, `issued` + balance > 0 + `due_on < today` → Terlambat, otherwise Terbit.
- Number: `#0001` per workspace from `invoice_sequences` (separate from quotations).
- Issue requires `due_on` (any date; a past date is allowed and shows Terlambat at once) and `total > 0`.
- Payment: `kind ∈ {dp, cicilan, pelunasan}` (labels DP, Cicilan, Pelunasan), `method ∈ {transfer, tunai, lainnya}`, `1 ≤ amount ≤ balance`, `received_on ≤ today` (workspace day), note ≤ 300, `idempotency_key` 22 base64url chars generated when the form is rendered; a second insert with the same key returns the first payment (`created=false`).
- Reversal: one per payment (`UNIQUE (workspace_id, payment_id)`), reason 1–300, done under the invoice `FOR UPDATE`; after it the payment is shown struck through with the reason; the amount returns to the balance.
- Void: only `draft` or `issued` with no active (non-reversed) payment; revokes the share token.
- Balance (SQL): `total − COALESCE((SELECT sum(p.amount) FROM payments p WHERE p.workspace_id = i.workspace_id AND p.invoice_id = i.id AND NOT EXISTS (SELECT 1 FROM payment_reversals r WHERE r.workspace_id = p.workspace_id AND r.payment_id = p.id)), 0)`.
- Dashboard: `invoice.Summary{Outstanding, OutstandingCount, OverdueAmount, OverdueCount, PaidThisMonth}` and `quotation.CountByStatus(today)`; both reconcile with their lists in a test.
- Follow-up page, tab Terlambat: a "Tagihan jatuh tempo" section with the overdue invoices (customer, number, balance, due date, link); dashboard "Terlambat" count of follow-ups stays as is; a separate "Piutang" card shows outstanding and overdue counts.
- WhatsApp from the invoice page: `{nilai}` = remaining balance, `{tautan}` = the invoice's public link; records `whatsapp_opened` on the lead with the template label. The lead page's WhatsApp data is unchanged.
- Public page `/t/{token}`: workspace name, number, dates, items, totals, "Sudah dibayar", "Sisa", list of active payments (date, kind, amount), due date; no phone, no notes of payments; 404 for draft/void/revoked/unknown.

---

### Task 1: Migration 00006 and its test

**Files:**
- Create: `internal/db/migrations/00006_invoices_payments.sql`
- Create: `internal/db/migrate_m6_test.go`
- Modify: `internal/db/migrate_test.go` (expects ≥ 6), `internal/db/tenant_scope_test.go:21`

- [x] **Step 1: Write the failing migration test** (`internal/db/migrate_m6_test.go`), modelled on `migrate_m5_test.go`: create a fresh database, `db.MigrateTo(ctx, pool, 5)`, insert user/workspace/membership/customer/lead/one `note` activity/one quotation with one item, then `db.Migrate` and assert: 18 kinds in `lead_activity_kinds`; insert `invoices` row (seq 1, status `draft`) ok; duplicate seq refused; `invoice_items` with `qty_milli 0` refused; `payments` with `amount 0` refused; `UPDATE payments SET amount = …` refused (`append-only`); `DELETE FROM payments` refused; `payment_reversals` second row for the same payment refused; `MigrateTo(ctx, pool, 5)` succeeds with a `payment_recorded` activity present and drops the tables; `Migrate` again applies 6.
- [x] **Step 2: Run** `REQUIRE_DB=1 go test -run TestMigration6 ./internal/db` → FAIL (migration missing: tables do not exist).
- [x] **Step 3: Write the migration**

```sql
-- +goose Up
INSERT INTO lead_activity_kinds (kind) VALUES
    ('invoice_created'), ('invoice_issued'), ('invoice_voided'), ('payment_recorded'), ('payment_reversed');

CREATE TABLE invoice_sequences (
    workspace_id uuid PRIMARY KEY REFERENCES workspaces (id) ON DELETE CASCADE,
    last_seq     integer NOT NULL DEFAULT 0
);

CREATE TABLE invoices (
    id               uuid PRIMARY KEY,
    workspace_id     uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    lead_id          uuid NOT NULL,
    customer_id      uuid NOT NULL,
    quotation_id     uuid,
    seq              integer NOT NULL CHECK (seq > 0),
    status           text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'issued', 'void')),
    due_on           date,
    notes            text CHECK (notes IS NULL OR length(notes) <= 2000),
    discount_pct     integer NOT NULL DEFAULT 0 CHECK (discount_pct BETWEEN 0 AND 100),
    tax_pct          integer NOT NULL DEFAULT 0 CHECK (tax_pct BETWEEN 0 AND 100),
    subtotal         bigint NOT NULL DEFAULT 0 CHECK (subtotal >= 0),
    discount_amount  bigint NOT NULL DEFAULT 0 CHECK (discount_amount >= 0),
    tax_amount       bigint NOT NULL DEFAULT 0 CHECK (tax_amount >= 0),
    total            bigint NOT NULL DEFAULT 0 CHECK (total >= 0),
    share_token      text UNIQUE CHECK (share_token IS NULL OR length(share_token) = 22),
    share_revoked_at timestamptz,
    first_opened_at  timestamptz,
    open_count       integer NOT NULL DEFAULT 0,
    issued_at        timestamptz,
    voided_at        timestamptz,
    version          integer NOT NULL DEFAULT 1,
    created_by       uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, id),
    UNIQUE (workspace_id, seq),
    FOREIGN KEY (workspace_id, lead_id) REFERENCES leads (workspace_id, id),
    FOREIGN KEY (workspace_id, customer_id) REFERENCES customers (workspace_id, id),
    FOREIGN KEY (workspace_id, quotation_id) REFERENCES quotations (workspace_id, id) ON DELETE SET NULL (workspace_id... )
);
```
  (The last line is written out in the real file as `FOREIGN KEY (workspace_id, quotation_id) REFERENCES quotations (workspace_id, id)` with `quotation_id` nullable and no cascade.)

```sql
CREATE INDEX invoices_workspace_status_due_idx ON invoices (workspace_id, status, due_on);
CREATE INDEX invoices_workspace_lead_idx ON invoices (workspace_id, lead_id);
CREATE INDEX invoices_workspace_customer_idx ON invoices (workspace_id, customer_id);

CREATE TABLE invoice_items (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL,
    invoice_id   uuid NOT NULL,
    position     integer NOT NULL CHECK (position >= 0),
    description  text NOT NULL CHECK (length(description) BETWEEN 1 AND 300),
    qty_milli    bigint NOT NULL CHECK (qty_milli > 0 AND qty_milli <= 1000000000),
    unit         text CHECK (unit IS NULL OR length(unit) <= 20),
    unit_price   bigint NOT NULL CHECK (unit_price >= 0 AND unit_price <= 10000000000000),
    line_total   bigint NOT NULL CHECK (line_total >= 0),
    UNIQUE (workspace_id, id),
    FOREIGN KEY (workspace_id, invoice_id) REFERENCES invoices (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX invoice_items_invoice_idx ON invoice_items (workspace_id, invoice_id, position);

CREATE TABLE payments (
    id              uuid PRIMARY KEY,
    workspace_id    uuid NOT NULL,
    invoice_id      uuid NOT NULL,
    kind            text NOT NULL CHECK (kind IN ('dp', 'cicilan', 'pelunasan')),
    method          text NOT NULL CHECK (method IN ('transfer', 'tunai', 'lainnya')),
    amount          bigint NOT NULL CHECK (amount > 0 AND amount <= 10000000000000),
    received_on     date NOT NULL,
    note            text CHECK (note IS NULL OR length(note) <= 300),
    idempotency_key text NOT NULL CHECK (length(idempotency_key) BETWEEN 8 AND 64),
    recorded_by     uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, id),
    UNIQUE (workspace_id, idempotency_key),
    FOREIGN KEY (workspace_id, invoice_id) REFERENCES invoices (workspace_id, id)
);
CREATE INDEX payments_invoice_idx ON payments (workspace_id, invoice_id, received_on);

CREATE TABLE payment_reversals (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL,
    payment_id   uuid NOT NULL,
    reason       text NOT NULL CHECK (length(reason) BETWEEN 1 AND 300),
    reversed_by  uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, id),
    UNIQUE (workspace_id, payment_id),
    FOREIGN KEY (workspace_id, payment_id) REFERENCES payments (workspace_id, id)
);

-- +goose StatementBegin
CREATE FUNCTION payments_append_only() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'payments are append-only; record a reversal instead' USING ERRCODE = 'restrict_violation';
END $$;
-- +goose StatementEnd
CREATE TRIGGER payments_append_only BEFORE UPDATE OR DELETE ON payments FOR EACH ROW EXECUTE FUNCTION payments_append_only();

-- +goose Down
DROP TRIGGER payments_append_only ON payments;
DROP FUNCTION payments_append_only();
DROP TABLE payment_reversals;
DROP TABLE payments;
DROP TABLE invoice_items;
DROP TABLE invoices;
DROP TABLE invoice_sequences;
DELETE FROM lead_activities WHERE kind IN ('invoice_created', 'invoice_issued', 'invoice_voided', 'payment_recorded', 'payment_reversed');
DELETE FROM lead_activity_kinds WHERE kind IN ('invoice_created', 'invoice_issued', 'invoice_voided', 'payment_recorded', 'payment_reversed');
```
  Note on the Down: `DROP TABLE payments` runs with the trigger still defined if the order is wrong; drop the trigger first (as above). Dropping a table does not fire row triggers, so the order above is safe either way; keep it explicit.

- [x] **Step 4: Run** the migration test → PASS; `migrate_test.go` expectation bumped to ≥ 6; `tenant_scope_test.go` regex becomes `(customers|leads|lead_activities|follow_ups|message_templates|quotations|quotation_items|invoices|invoice_items|payments|payment_reversals)`; run `go test ./internal/db` → PASS (if a quotation statement fails the scan, scope it; do not loosen the test).
- [x] **Step 5: Apply to the dev database** (`make migrate`) — status shows 6 applied.

### Task 2: `internal/docpdf` and the quotation wrapper

**Files:**
- Create: `internal/docpdf/docpdf.go`, `internal/docpdf/docpdf_test.go`
- Modify: `internal/quotation/pdf.go` (becomes a wrapper), keep `internal/quotation/pdf_test.go` green unchanged

**Interfaces (Produces):**

```go
package docpdf

type Options struct { Compress bool; CreatedAt time.Time; Loc *time.Location }
type Line struct { Label, Value string; Bold bool }
type Item struct { Description, Qty, Unit, UnitPrice, LineTotal string }
type Document struct {
    Kind        string // "PENAWARAN" | "TAGIHAN"
    Title       string // metadata title
    Author      string // workspace name
    Number      string // "0001"
    Status      string
    Dates       []Line // right column of the header, e.g. {"Tanggal", "13 Okt 2026"}, {"Jatuh tempo", "27 Okt 2026"}
    Recipient   string // customer name
    Project     string // lead title
    Items       []Item
    Totals      []Line // Subtotal, Diskon, Pajak, TOTAL(bold), Sudah dibayar, SISA(bold)
    Notes       string
    Footer      string
}
func Render(w io.Writer, d Document, opts Options) error
func Date(t *time.Time, loc *time.Location) string // "13 Okt 2026" or "-"
func DateUTC(t *time.Time) string                   // for date columns
```

- [x] **Step 1: Write the failing test** `docpdf_test.go`: render a Document with `Recipient: "Café Rénové"`, an item description with an en dash and an emoji, three totals lines and notes with a newline, `Compress: false`; assert the bytes contain `Caf\xe9`, `\x96`, `?`, `TAGIHAN`, `No. 0001`, both totals labels, that the newline is not flattened, and that 70 long rows produce ≥ 3 pages with one `/Type /Pages`.
- [x] **Step 2: Run** `go test ./internal/docpdf` → FAIL (package missing).
- [x] **Step 3: Move** the body of `quotation.RenderPDF` (encode, dates, header, parties, table with manual page breaks, totals, notes, footer) into `docpdf.Render`, parameterised by `Document`. Rewrite `quotation.RenderPDF` as:

```go
func RenderPDF(w io.Writer, q Quotation, opts PDFOptions) error {
    loc := opts.Loc; if loc == nil { loc = docpdf.WIB }
    status := q.Status; if !opts.Today.IsZero() { status = q.EffectiveStatus(opts.Today) }
    d := docpdf.Document{Kind: "PENAWARAN", Title: "Penawaran " + q.Number() + " - " + q.WorkspaceName, Author: q.WorkspaceName, Number: q.Number(), Status: status.Label(),
        Dates: []docpdf.Line{{Label: "Tanggal", Value: docpdf.Date(&q.CreatedAt, loc)}, {Label: "Berlaku sampai", Value: docpdf.DateUTC(q.ValidUntil)}},
        Recipient: q.CustomerName, Project: q.LeadTitle, Notes: q.Notes, Footer: "Dibuat dengan KerjaLancar. Harga dalam rupiah."}
    for _, it := range q.Items { d.Items = append(d.Items, docpdf.Item{Description: it.Description, Qty: money.FormatQty(it.QtyMilli), Unit: it.Unit, UnitPrice: money.FormatIDR(it.UnitPrice), LineTotal: money.FormatIDR(it.LineTotal)}) }
    d.Totals = append(d.Totals, docpdf.Line{Label: "Subtotal", Value: money.FormatIDR(q.Subtotal)})
    if q.DiscountPct > 0 { d.Totals = append(d.Totals, docpdf.Line{Label: fmt.Sprintf("Diskon %d%%", q.DiscountPct), Value: "-" + money.FormatIDR(q.DiscountAmount)}) }
    if q.TaxPct > 0 { d.Totals = append(d.Totals, docpdf.Line{Label: fmt.Sprintf("Pajak %d%%", q.TaxPct), Value: money.FormatIDR(q.TaxAmount)}) }
    d.Totals = append(d.Totals, docpdf.Line{Label: "TOTAL", Value: money.FormatIDR(q.Total), Bold: true})
    return docpdf.Render(w, d, docpdf.Options{Compress: opts.Compress, CreatedAt: opts.CreatedAt, Loc: loc})
}
```
- [x] **Step 4: Run** `go test ./internal/docpdf ./internal/quotation` → PASS (all five existing PDF tests unchanged).

### Task 3: `internal/invoice` — model, store, PDF

**Files:**
- Create: `internal/invoice/invoice.go`, `store.go`, `pdf.go`, `invoice_test.go`, `store_test.go`, `pdf_test.go`
- Modify: `internal/lead/activity.go` (+ `lead_test.go`): kinds and `Describe` lines

**Interfaces (Produces):**

```go
package invoice

type Status string // "draft" | "issued" | "void"; Effective adds StatusPaid "lunas", StatusOverdue "terlambat"
func (s Status) Label() string // Draf, Terbit, Lunas, Terlambat, Dibatalkan
type Invoice struct {
    ID, WorkspaceID, LeadID, CustomerID uuid.UUID; QuotationID *uuid.UUID
    LeadTitle, CustomerName, CustomerPhone, WorkspaceName string
    Seq int; Status Status; DueOn *time.Time; Notes string
    DiscountPct, TaxPct int; Subtotal, DiscountAmount, TaxAmount, Total, Paid, Balance int64
    ShareToken string; ShareRevokedAt, FirstOpenedAt *time.Time; OpenCount int
    IssuedAt, VoidedAt *time.Time; Version int; CreatedAt, UpdatedAt time.Time
    Items []quotation.Item; Payments []Payment
}
func (i Invoice) Number() string            // "0001"
func (i Invoice) Effective(today time.Time) Status
func (i Invoice) Editable() bool            // draft
func (i Invoice) ShareActive() bool
type Payment struct { ID, InvoiceID uuid.UUID; Kind PaymentKind; Method Method; Amount int64; ReceivedOn time.Time; Note string; RecordedBy *uuid.UUID; RecordedByName string; CreatedAt time.Time; Reversal *Reversal }
type Reversal struct { ID uuid.UUID; Reason string; ReversedBy *uuid.UUID; ReversedByName string; CreatedAt time.Time }
type PaymentKind string // dp, cicilan, pelunasan (Label: DP, Cicilan, Pelunasan)
type Method string      // transfer, tunai, lainnya (Label: Transfer, Tunai, Lainnya)

type Input struct { Items []quotation.ItemInput; DiscountPct, TaxPct, DueOn, Notes string }
type Normalized struct { Items []quotation.NormalizedItem; DiscountPct, TaxPct int; DueOn *time.Time; Notes string; quotation.Totals }
func Validate(in Input) (Normalized, map[string]string) // items via quotation.Validate; "due_on" error "Tanggal tidak valid."

type PaymentInput struct { Kind, Method, Amount, ReceivedOn, Note, IdempotencyKey string }
type NormalizedPayment struct { Kind PaymentKind; Method Method; Amount int64; ReceivedOn time.Time; Note, IdempotencyKey string }
func ValidatePayment(in PaymentInput, today time.Time) (NormalizedPayment, map[string]string)
// errors: "kind" Jenis pembayaran tidak valid. | "method" Metode tidak valid. | "amount" Jumlah tidak valid. Contoh: 5.000.000 | "received_on" Tanggal tidak valid. / Tanggal tidak boleh di masa depan. | "note" Catatan maksimal 300 karakter. | "form" Formulir kedaluwarsa, muat ulang halaman. (bad key)
func NewIdempotencyKey() (string, error) // 22 base64url chars

var ErrNotFound, ErrLeadNotFound, ErrLeadClosed, ErrLocked, ErrStale, ErrNotIssued, ErrNoDueDate, ErrEmpty, ErrOverpayment, ErrHasPayments, ErrAlreadyReversed, ErrQuotationNotFound error
type OverpaymentError struct { Balance int64 } // Is(ErrOverpayment); message built in the handler: "Jumlah melebihi sisa tagihan (Rp…)."

func NewStore(pool *pgxpool.Pool, leads *lead.Store) *Store
func (s *Store) Create(ctx, ws, actor, leadID uuid.UUID, fromQuotation *uuid.UUID, n Normalized) (Invoice, error) // fromQuotation: items/discount/tax copied from the quotation (must belong to ws+lead), n.Items ignored
func (s *Store) Get(ctx, ws, id) (Invoice, error)          // with Items and Payments (reversals joined)
func (s *Store) Update(ctx, ws, actor, id, n Normalized, version int) (Invoice, error) // draft only
func (s *Store) Issue(ctx, ws, actor, id) (Invoice, error) // draft → issued; needs DueOn and Total > 0; mints token
func (s *Store) Void(ctx, ws, actor, id) (Invoice, error)  // draft|issued without active payments → void; revokes token
func (s *Store) RevokeShare / RenewShare(ctx, ws, id) (Invoice, error) // issued only
func (s *Store) GetByToken(ctx, token string) (Invoice, error) // issued + not revoked
func (s *Store) RecordOpen(ctx, ws, id) error
func (s *Store) RecordPayment(ctx, ws, actor, id uuid.UUID, p NormalizedPayment) (Payment, bool, error) // bool=created; ErrNotIssued, *OverpaymentError
func (s *Store) ReversePayment(ctx, ws, actor, paymentID uuid.UUID, reason string) (Payment, error)
type ListQuery struct { Status string; Q string; Page int } // Status: "" (belum lunas), terlambat, lunas, draf, dibatalkan, semua
func (s *Store) List(ctx, ws, lq ListQuery, today) (Page, error)
func (s *Store) ForLead(ctx, ws, leadID) ([]Invoice, error)
func (s *Store) Overdue(ctx, ws, today, limit int) ([]Invoice, error)
type Summary struct { Outstanding int64; OutstandingCount int; OverdueAmount int64; OverdueCount int; PaidThisMonth int64 }
func (s *Store) Summary(ctx, ws, today) (Summary, error)
func (s *Store) LatestShared(ctx, ws, leadID) (Invoice, bool, error)
func RenderPDF(w io.Writer, inv Invoice, opts docpdf.Options, today time.Time) error
```

In `internal/lead/activity.go`: `KindInvoiceCreated/Issued/Voided`, `KindPaymentRecorded`, `KindPaymentReversed`; `ActivityData.Amount string` (formatted); lines: `Tagihan #0001 dibuat (Rp504.493)`, `Tagihan #0001 diterbitkan (Rp504.493)`, `Tagihan #0001 dibatalkan`, `Pembayaran Rp200.000 dicatat untuk tagihan #0001 (DP)`, `Pembayaran Rp200.000 dibatalkan untuk tagihan #0001`.

- [x] **Step 1: Failing unit tests** (`invoice_test.go`): `TestValidateRequiresADueDateOnlyWhenGiven` (blank due date allowed in draft; `2026-13-40` → "Tanggal tidak valid."), `TestValidatePayment` table (amount `5.000.000` ok; `0` invalid; `abc` invalid; kind `refund` invalid; method `qris` invalid; received_on tomorrow → "Tanggal tidak boleh di masa depan."; note 301 chars; key `short` → "form"), `TestEffectiveStatus` (issued+balance 0 → Lunas; issued+due yesterday → Terlambat; issued+due today → Terbit; void → Dibatalkan).
- [x] **Step 2: Failing store tests** (`store_test.go`, DB), fixtures via `auth.Service.Register` + `lead.Store.Create` as in `quotation/store_test.go`:
  - `TestCreateCopiesTheAcceptedQuotation` (items, discount, tax, totals equal; `QuotationID` set; editing the invoice does not change the quotation; `invoice_created` activity).
  - `TestCreateBlankAndUpdateDraft` (+ `ErrStale`, `ErrLocked` after issue).
  - `TestIssueNeedsDueDateAndTotal`, `TestIssueMintsAShareToken`.
  - `TestBalanceMatchesSQLForEveryFixture`: payments 200.000 + 100.000, reversal of 100.000 → Go `Total−Σ+Σrev` equals `Balance` scanned from SQL for each of three invoices (paid, partial, untouched).
  - `TestRecordPaymentRefusesOverpayment` (balance 504.493; 504.494 → `*OverpaymentError{Balance: 504_493}`; 504.493 ok → Lunas).
  - `TestRecordPaymentIsIdempotentOnTheKey` (same key twice → same ID, `created=false`, one row).
  - `TestConcurrentPaymentsSerializeOnTheInvoice`: barrier-started goroutines; (a) two × 60 % → exactly one `OverpaymentError`; (b) two × 40 % → both persist, balance 20 %.
  - `TestReversePaymentRestoresTheBalance` (+ second reversal → `ErrAlreadyReversed`; paying the same amount again with a new key works).
  - `TestVoidRefusesInvoicesWithPayments` (active payment → `ErrHasPayments`; after reversing it → void ok; token revoked; `GetByToken` → `ErrNotFound`).
  - `TestRecordPaymentNeedsIssued` (draft → `ErrNotIssued`).
  - `TestListsAndOverdue` (default list = issued with balance > 0, overdue first; `terlambat`; `lunas`; `draf`; search by number and customer; `Overdue(today)` lists only due < today).
  - `TestSummaryReconcilesWithLists`.
  - `TestCrossWorkspaceIsInvisible` (Get/Update/Issue/RecordPayment/ReversePayment/Void with another workspace → `ErrNotFound`, nothing written).
  - `TestPaymentsAreAppendOnlyAtTheDatabase` (raw `UPDATE payments` → error).
- [x] **Step 3: Run** → FAIL (package missing).
- [x] **Step 4: Implement** `invoice.go`, `store.go` (SQL mirrors `quotation/store.go`; `columns` adds the balance subquery as `paid` and `balance`; `RecordPayment`:

```go
tx := begin; defer rollback
inv, err := get(ctx, tx, ws, id, true /* FOR UPDATE OF i */)   // invoice lock; lead not needed
if inv.Status != StatusIssued { return ErrNotIssued }
if p.Amount > inv.Balance { return &OverpaymentError{Balance: inv.Balance} }
// idempotency: look up first inside the lock
if existing found by (ws, key) { commit; return existing, false, nil }
INSERT payments …; AppendActivity(KindPaymentRecorded, data{Number, Amount, Kind label}); commit; return payment, true, nil
```
  `ReversePayment` locks the invoice row through the payment's `invoice_id`, inserts the reversal (unique per payment → `ErrAlreadyReversed` on 23505), appends `payment_reversed`. `Create` with `fromQuotation`: `lockLead` → read the quotation (`ws`, `lead_id` must match; `ErrQuotationNotFound`) → copy items. `Issue`: `lockLead` then invoice `FOR UPDATE`; refuse without `due_on` (`ErrNoDueDate`) or `total = 0` (`ErrEmpty`); token via `crypto/rand`. `Void`: lock; refuse when `EXISTS active payment` (`ErrHasPayments`).
- [x] **Step 5: Run** `REQUIRE_DB=1 go test -race -count=1 ./internal/invoice ./internal/lead` → PASS.
- [x] **Step 6: PDF** (`pdf.go` + `pdf_test.go`): `RenderPDF` builds a `docpdf.Document{Kind: "TAGIHAN", Dates: Tanggal (issued_at or created_at in loc), Jatuh tempo; Totals: …, TOTAL, Sudah dibayar, SISA}`; test asserts `TAGIHAN`, `Jatuh tempo`, `Sudah dibayar`, `SISA`, no phone digits.

### Task 4: Web — handlers, routes, templates, wiring

**Files:**
- Create: `internal/web/invoice_handlers.go`, templates `tagihan_daftar.html`, `tagihan_form.html`, `tagihan_detail.html`, `share_tagihan.html`
- Modify: `router.go`, `templates.go`, `public_handlers.go`, `middleware.go` (`logPath` for `/t/`), `lead_handlers.go` + `prospek_detail.html` (Tagihan section with "Buat tagihan dari penawaran #000N" for each accepted quotation and "Buat tagihan kosong"), `followup_handlers.go` + `followup_daftar.html` (Terlambat tab: "Tagihan jatuh tempo"), `app_handlers.go` + `beranda.html` (Piutang card: outstanding, overdue count; "Penawaran per status"), `layouts/app.html` (nav "Tagihan"), `cmd/server/main.go`, `app_test.go` (`Deps.Invoices`), `static/app.css` (rebuild)

**Routes:**

```
GET  /app/tagihan                 list (chips: Belum lunas | Terlambat | Lunas | Draf | Dibatalkan | Semua; search q)
GET  /app/tagihan/baris           add-row fragment (reuse penawaran_baris_baru)
GET  /app/tagihan/baru?prospek=&penawaran=   form (prefilled from the quotation when given; due date default +14 days)
POST /app/tagihan                 create → 303 detail
GET  /app/tagihan/{id}            detail: items/totals, status, share box, payments list, payment form (hidden idempotency key), actions
GET  /app/tagihan/{id}/ubah       edit draft
POST /app/tagihan/{id}            update draft (version)
POST /app/tagihan/{id}/terbit     issue
POST /app/tagihan/{id}/batal      void
POST /app/tagihan/{id}/tautan     aksi=cabut|baru
POST /app/tagihan/{id}/bayar      record payment (flash "Pembayaran tercatat." / "Pembayaran ini sudah tercatat." / errors re-render detail with the form errors)
POST /app/tagihan/{id}/pembayaran/{pid}/batal   reversal (reason)
POST /app/tagihan/{id}/wa         WhatsApp with {nilai}=balance, {tautan}=link
GET  /app/tagihan/{id}/pdf
GET  /t/{token}, GET /t/{token}/pdf   public (same group as /p)
```

Copy near the payment form (D-011): "Catatan pembayaran ini adalah pencatatan Anda sendiri, bukan bukti transfer. Cocokkan dengan mutasi rekening Anda."

- [x] **Step 1: Failing web tests** (`invoice_web_test.go`): `TestInvoiceFromQuotationCreateIssuePayAndHistory` (create from accepted quotation → detail shows `Tagihan #0001`, items, `Rp504.493`; issue → `Terbit`, link `/t/…`; pay DP 200.000 → `Sudah dibayar Rp200.000`, `Sisa Rp304.493`, lead history line; pay 304.493 → `Lunas`), `TestPaymentDoubleSubmitRecordsOnce` (post the same form twice → one row, flash "sudah tercatat"), `TestPaymentOverpaymentShowsMessage` (`Jumlah melebihi sisa tagihan (Rp304.493).`), `TestPaymentReversalNeedsReason` (empty reason → error; with reason → struck through, balance back), `TestVoidRefusedWithPaymentsThenAllowed`, `TestInvoiceListChipsAndSearch`, `TestInvoiceCSRFAndForeignIds` (POSTs without CSRF/with another workspace's id → 403/404; nothing written), `TestInvoiceMassAssignmentIgnored` (`total`, `status`, `balance`, `paid`, `share_token` in forms ignored), `TestFollowUpPageListsOverdueInvoices`, `TestDashboardShowsReceivablesThatReconcile` (fixture: paid, partial, overdue; card numbers equal the list totals), `TestInvoiceWhatsAppFillsBalanceAndLink`, `TestInvoicePDFDownload`.
  `public_web_test.go`: `TestPublicInvoicePageAndPDF` (no cookie, `no-store`, items, `Sisa`, no phone, no payment notes, no `/app/`; open count), `TestPublicInvoiceLinkStopsAfterVoid` and after revoke (404), rate limit 61st → 429.
- [x] **Step 2: Run** → FAIL (routes 404).
- [x] **Step 3: Implement** handlers and templates following `quotation_handlers.go` / `penawaran_*.html` one for one; `rowsFromForm`/`rowsToInput` reused; `GET /app/tagihan/{id}` renders the payment form with `IdempotencyKey` from `invoice.NewIdempotencyKey()`; `POST …/bayar` validates with `s.today(ws)`, maps `*OverpaymentError` to the `amount` error with `money.FormatIDR(Balance)`, `created=false` → flash `msgPaymentDuplicate`.
- [x] **Step 4: Rebuild CSS** (`.toolchain/tw/node_modules/.bin/tailwindcss -i internal/web/static/src/app.css -o internal/web/static/app.css --minify` with Node 22 on PATH).
- [x] **Step 5: Run** `REQUIRE_DB=1 go test -race -count=1 -p 1 ./internal/web` → PASS; then the whole suite.

### Task 5: Browser check, docs, review

- [x] Live check on the dev server (port 8089 if 8080 is busy): lead → Buat tagihan dari penawaran → Terbitkan → public `/t/` link in curl (no cookie) → PDF → Catat pembayaran DP → double submit → reversal → Lunas; dashboard cards; follow-up Terlambat section with a past due date; 375 px viewport without horizontal scroll.
- [x] Docs: `DECISIONS` D-056..D-060, `RISK_REGISTER` (R-29 payments are owner notes, R-30 idempotency key lifetime, R-31 invoice edited after issue impossible but quotation drift), `ARCHITECTURE` §6/§8 (docpdf, invoice package, `/t/`), `MVP_PLAN` M6 status, `PROJECT_STATUS` (results actually run), `DEVELOPMENT` (how to try).
- [x] Independent review (fresh-context reviewer agent) → fix Critical/Important with tests → rerun suite, lint, vet, `git diff --check`, govulncheck. (Two Important and two Minor findings fixed with regression tests; see PROJECT_STATUS.)
- [x] Final: `git diff main...HEAD --stat`; no secrets/local artefacts; working tree clean; **no merge, no push of M6** (the change set is left uncommitted for the founder's review, per the brief).

## Self-review notes

- Spec coverage: invoices from quotation or blank ✓ (Task 3 Create), number per workspace ✓, due date ✓, totals ✓, `invoice_items` ✓, payments with type/method/received_on/note/idempotency key/recorded_by ✓, `payment_reversals` with reason ✓, balance in SQL ✓ (+ Go assertion), receivables list overdue first ✓ (List default order), share page and PDF reuse M5 ✓ (`/t/`, docpdf), dashboard read models ✓ (Summary, CountByStatus), D-011 copy ✓, acceptance criteria 1–4 each have a named test. Deferred per spec: instalment schedules, faktur pajak, bank integration.
- D-043's "M6 adds an invoice reference" to follow-ups is **not** implemented: the Terlambat tab lists overdue invoices from `invoices` directly; recorded in D-059.
- Type consistency: `quotation.ItemInput`, `quotation.NormalizedItem`, `quotation.Totals`, `quotation.Item` are reused by `invoice`; `docpdf.Options` replaces `quotation.PDFOptions` internals but the public `PDFOptions` stays.
