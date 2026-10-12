# Milestone 3 — Customers, Leads, Pipeline, Activity (plan)

> Executed natively in one session (founder's brief: continue until the milestone is tested; no commit without approval). Tasks are TDD: failing test → implementation → green → next task. Checkboxes track progress.

**Goal:** The owner records a customer and a project lead from a phone in a few taps, moves the lead through a simple pipeline, writes notes, searches and filters the pipeline, and sees the full history of each lead. Nothing crosses a workspace boundary.

**Architecture:** Two new domain packages (`internal/customer`, `internal/lead`) own their rules and hand-written pgx SQL; `internal/money` holds integer-rupiah parsing and formatting; `internal/web` adds thin handlers, Indonesian templates, htmx fragments, pagination and a technical audit log helper. Migration `00003` adds `customers`, `leads`, `lead_activities` with `workspace_id` on every row, composite foreign keys, and an append-only trigger on activities.

**Tech stack:** Go 1.27 · chi v5 · `html/template` · htmx 2.0.11 · Tailwind v4 (committed `app.css`, rebuilt with `make css-npm` or `make css`) · PostgreSQL 17 · pgx v5 · goose · testify. No new dependencies.

**Spec:** Founder's M3 brief (2026-10-12) · `docs/MVP_PLAN.md` §M3 · `docs/ARCHITECTURE.md` §4–§8 · `docs/PRODUCT_BRIEF.md` §4 · `docs/RISK_REGISTER.md` R-01, R-04, R-09, R-10.

## Global constraints

- Every customer, lead and activity query filters on `workspace_id` taken from the session membership (`currentWorkspace(ctx)`); ids in forms or URLs are only ever combined with that workspace id. Cross-workspace access returns 404, never 403.
- Parameterised SQL only; one transaction per operation that writes more than one row (lead + activity, status + activity, update + activity).
- Money is `BIGINT` rupiah (0 ≤ v ≤ 10^13). No floats anywhere in the money path.
- Server-side validation with Indonesian messages; text limits: name/title 1–150, company 100, e-mail 254, phone 20, notes/description 2000, lost reason 500, search 100.
- `html/template` escaping only; no `template.HTML` of user data. CSRF middleware already wraps every `/app` route; new state changes are `POST` only.
- Lists are paginated (25 per page) and never unbounded; activities on a lead detail are capped at the newest 100.
- Schema changes go in a new migration `00003`; migrations `00001`–`00002` are not touched.
- No demo data; empty states explain what to do next. Indonesian UI copy; URLs `/app/pelanggan` and `/app/prospek`.
- No deletion of customers or leads in M3 (D-037); no quotation/invoice, no WhatsApp API, no AI, no new frontend framework.

## Domain decisions (recorded in `docs/DECISIONS.md` D-033..D-040)

| ID | Decision |
|---|---|
| D-033 | **Customer vs lead.** A customer is a contact (person or business). A lead is one project opportunity. A lead always belongs to exactly one customer; a customer has zero or more leads. "Lead without an identified customer" is modelled as a customer created inline from the lead form with only a name (phone optional). There is no "convert lead to customer" step: the customer exists from the start, and winning is a lead status (`menang`), which M5 ties to an accepted quotation. |
| D-034 | **Phone numbers.** Optional. Normalised to E.164: `08…`, `62…`, `+62…`, and bare `8…` become `+62…`; other `+<cc>` numbers with 8–15 digits are kept. Unique per workspace (partial unique index on `(workspace_id, phone)`); a duplicate is rejected with the existing customer's name so the user picks that customer instead. |
| D-035 | **Pipeline.** Six statuses: `baru`, `dihubungi`, `survei`, `penawaran`, `menang`, `kalah`. `negosiasi` from the brief's candidate list is folded into `penawaran` (M5 quotations carry revisions). Rules: a change to a different status is allowed in either direction (contractors skip and revisit steps); `menang` ↔ `kalah` directly is refused (reopen first); `lost_reason` is optional, allowed only with `kalah`, and cleared on reopen; moving to `penawaran` with no follow-up date sets `next_follow_up_on` to today + 3 days in the workspace zone; moving to `menang`/`kalah` clears it. `status_changed_at` records the last change; every change appends an activity. |
| D-036 | **Activity vs audit.** `lead_activities` is the user-visible business history: `created`, `status_changed`, `note`, `updated` (field labels, old/new for value, status and dates; never free text of description/notes). Append-only, enforced by a `BEFORE UPDATE` trigger. Kinds are extensible for M4 (`follow_up_done`) and M5 (`quotation_sent`, `quotation_accepted`). Technical audit in M3 is a structured `slog` line per mutation (`action`, `user_id`, `workspace_id`, entity id, `request_id`; no content) — a DB-backed audit table waits for M7's threat model. |
| D-037 | **No delete in M3.** Mistakes are fixed by editing; closed leads stay visible; deletion and export arrive with M7's retention work. |
| D-038 | **Money.** `internal/money`: `ParseIDR` accepts `15000000`, `15.000.000`, `Rp 15.000.000`, `Rp15.000.000,00`; rejects negatives, decimals other than `,00`, and values above Rp10 triliun. `FormatIDR(15000000) == "Rp15.000.000"`. Estimated lead value is an estimate, never revenue. |
| D-039 | **Names.** UI says "Pelanggan" (customer) and "Prospek" (lead); code and tables keep `customer`/`lead`. Lead source and project type are small server-side enums with `lainnya` (D-033 of the MVP plan allowed free text; a select is faster on a phone and filterable later). |
| D-040 | **sqlc stays deferred** (needs a tool install; ~25 hand-written queries are manageable). Revisit at M5 when quotations add joins and totals. |

## Schema — `internal/db/migrations/00003_customers_leads_activities.sql`

```sql
-- +goose Up
CREATE TABLE customers (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    name         text NOT NULL CHECK (length(name) BETWEEN 1 AND 150),
    phone        text CHECK (phone ~ '^\+[1-9][0-9]{7,14}$'),
    email        citext CHECK (length(email) <= 254),
    company      text CHECK (length(company) <= 100),
    notes        text CHECK (length(notes) <= 2000),
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, id)
);
CREATE UNIQUE INDEX customers_workspace_phone_idx ON customers (workspace_id, phone) WHERE phone IS NOT NULL;
CREATE INDEX customers_workspace_created_idx ON customers (workspace_id, created_at DESC);
CREATE INDEX customers_workspace_name_idx ON customers (workspace_id, lower(name));

CREATE TABLE leads (
    id                 uuid PRIMARY KEY,
    workspace_id       uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    customer_id        uuid NOT NULL,
    title              text NOT NULL CHECK (length(title) BETWEEN 1 AND 150),
    project_type       text CHECK (project_type IN ('renovasi','interior','bangun_baru','perbaikan','lainnya')),
    description        text CHECK (length(description) <= 2000),
    estimated_value    bigint CHECK (estimated_value BETWEEN 0 AND 10000000000000),
    status             text NOT NULL CHECK (status IN ('baru','dihubungi','survei','penawaran','menang','kalah')),
    source             text CHECK (source IN ('whatsapp','instagram','facebook','referensi','google','marketplace','lainnya')),
    next_follow_up_on  date,
    lost_reason        text CHECK (length(lost_reason) <= 500),
    notes              text CHECK (length(notes) <= 2000),
    status_changed_at  timestamptz NOT NULL DEFAULT now(),
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, id),
    FOREIGN KEY (workspace_id, customer_id) REFERENCES customers (workspace_id, id),
    CHECK (lost_reason IS NULL OR status = 'kalah')
);
CREATE INDEX leads_workspace_status_idx    ON leads (workspace_id, status, created_at DESC);
CREATE INDEX leads_workspace_customer_idx  ON leads (workspace_id, customer_id);
CREATE INDEX leads_workspace_followup_idx  ON leads (workspace_id, next_follow_up_on) WHERE next_follow_up_on IS NOT NULL;

CREATE TABLE lead_activities (
    id            uuid PRIMARY KEY,
    workspace_id  uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    lead_id       uuid NOT NULL,
    kind          text NOT NULL CHECK (kind IN ('created','status_changed','note','updated')),
    actor_user_id uuid REFERENCES users (id) ON DELETE SET NULL,
    body          text CHECK (length(body) <= 2000),
    data          jsonb,
    created_at    timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (workspace_id, lead_id) REFERENCES leads (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX lead_activities_lead_idx ON lead_activities (workspace_id, lead_id, created_at DESC);

-- +goose StatementBegin
CREATE FUNCTION lead_activities_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'lead_activities is append-only';
END $$;
-- +goose StatementEnd
CREATE TRIGGER lead_activities_no_update BEFORE UPDATE ON lead_activities
    FOR EACH ROW EXECUTE FUNCTION lead_activities_immutable();

-- +goose Down
DROP TABLE lead_activities;
DROP FUNCTION lead_activities_immutable();
DROP TABLE leads;
DROP TABLE customers;
```

Why: `UNIQUE (workspace_id, id)` plus the composite foreign key means the database itself refuses a lead that points at another workspace's customer (R-04), independent of application code. `email` is `citext` for case-insensitive search. The `kind` check is widened by a later migration when M4/M5 add kinds.

## Packages and files

```
internal/money/money.go, money_test.go               ParseIDR, FormatIDR (table tests)
internal/customer/phone.go, phone_test.go            NormalizePhone(raw) (string, error); DisplayPhone(e164) string
internal/customer/customer.go                        Customer, Input, Validate(Input) (Normalized, map[string]string)
internal/customer/store.go, store_test.go            Store{Create, Update, Get, List(Query), Search, CountLeads}; ErrNotFound, ErrDuplicatePhone{Name}
internal/lead/lead.go, lead_test.go                  Lead, Status (+Order, Label, Open), Source, ProjectType, Input, Validate, CanTransition(from,to) error
internal/lead/activity.go                            Activity, Kind, Change{Field, From, To}, Describe(Activity) (Indonesian line)
internal/lead/store.go, store_test.go                Store{Create, Update, Get, List(Query), ChangeStatus, AddNote, Activities, ForCustomer, Summary}; ErrNotFound, ErrCustomerNotFound, ErrSameStatus, ErrClosedToClosed
internal/web/listing.go, listing_test.go             parsePage, Pagination{Page, PerPage, Total, Prev, Next}, cleanQuery
internal/web/audit.go                                audit(ctx, logger, action, attrs...) technical audit line (ids only)
internal/web/customer_handlers.go, customer_web_test.go
internal/web/lead_handlers.go, lead_web_test.go
internal/web/tenant_matrix_test.go                   every customer/lead route with workspace A's ids from workspace B → 404 (table-driven)
internal/web/templates.go                            pageLayouts entries, template funcs (rupiah, telepon, tanggal, statusLabel, badgeClass)
internal/web/templates/pages/pelanggan_daftar.html, pelanggan_form.html, pelanggan_detail.html
internal/web/templates/pages/prospek_daftar.html, prospek_form.html, prospek_detail.html
internal/web/templates/partials/pagination.html, status_badge.html, empty.html
internal/web/templates/layouts/app.html              nav: Beranda · Prospek · Pelanggan · Pengaturan
internal/web/templates/pages/beranda.html            counts by status, open pipeline value, two CTAs
internal/web/router.go, app_handlers.go              routes, beranda summary
internal/db/migrations/00003_customers_leads_activities.sql; internal/db/migrate_test.go (≥ 3)
internal/web/static/app.css                          rebuilt (`make css-npm` or `make css`)
docs: MVP_PLAN.md, DECISIONS.md, PROJECT_STATUS.md, ARCHITECTURE.md (activity/audit split), RISK_REGISTER.md (new rows if any)
```

## Routes (all under `/app`, behind `requireLogin`, CSRF-protected)

| Method | Path | Handler | Notes |
|---|---|---|---|
| GET | `/app/pelanggan` | `getPelangganList` | `?q=` search (name, phone, company), `?page=` |
| GET | `/app/pelanggan/baru` | `getPelangganBaru` | form |
| POST | `/app/pelanggan` | `postPelanggan` | create → 303 detail; errors re-render with input kept |
| GET | `/app/pelanggan/{id}` | `getPelanggan` | detail + leads of this customer |
| GET | `/app/pelanggan/{id}/ubah` | `getPelangganUbah` | edit form |
| POST | `/app/pelanggan/{id}` | `postPelangganUbah` | update → 303 detail |
| GET | `/app/prospek` | `getProspekList` | `?status=aktif|semua|baru|…`, `?q=`, `?urut=terbaru|nilai|followup`, `?page=` |
| GET | `/app/prospek/baru` | `getProspekBaru` | form; `?pelanggan=<id>` preselects an existing customer |
| POST | `/app/prospek` | `postProspek` | create (existing customer or inline new customer) in one transaction → 303 detail |
| GET | `/app/prospek/{id}` | `getProspek` | detail, status panel, note form, activity timeline |
| GET | `/app/prospek/{id}/ubah` | `getProspekUbah` | edit form |
| POST | `/app/prospek/{id}` | `postProspekUbah` | update + `updated` activity → 303 detail |
| POST | `/app/prospek/{id}/status` | `postProspekStatus` | status + optional `lost_reason`; htmx: fragment `prospek_status` + OOB `prospek_aktivitas`; else 303 |
| POST | `/app/prospek/{id}/catatan` | `postProspekCatatan` | note; htmx: fragment `prospek_catatan_form` + OOB timeline; else 303 |

Invalid or foreign ids → 404 page. Validation failures → 200 with the form re-rendered (htmx 2 does not swap 4xx by default; same convention as M2).

## Tasks

### A. Money and phone (pure functions)
- [ ] `money_test.go`: table for `ParseIDR` (`"15000000"`, `"15.000.000"`, `"Rp 15.000.000"`, `"Rp15.000.000,00"`, `" 0 "`, `""` → 0,false-ish; `"-1"`, `"1,5"`, `"abc"`, `"10000000000001"` → error) and `FormatIDR` (0 → `Rp0`, 1500 → `Rp1.500`, 15000000 → `Rp15.000.000`).
- [ ] `phone_test.go`: `0812-3456-7890`→`+6281234567890`; `+62 812 3456 7890`→same; `62812…`→same; `812…`→same; `+65 9123 4567`→`+6591234567`; `(021) 555-1234`→`+62215551234`; rejects `12`, `abc`, `+0…`, 16+ digits; `DisplayPhone("+6281234567890") == "0812-3456-7890"`, foreign numbers unchanged.
- [ ] Implement, run `go test ./internal/money ./internal/customer -run 'Phone|IDR'`.

### B. Migration 00003 + customer store
- [ ] Write migration (above). `migrate_test.go`: `len(statuses) >= 3`.
- [ ] `store_test.go` (two workspaces via `auth.Service.Register`, pattern from `internal/workspace/store_test.go`): create with all fields and with name only; update changes fields and `updated_at`; `Get` with the other workspace's id → `ErrNotFound`; `Update` from the other workspace → `ErrNotFound` and row unchanged; duplicate phone in the same workspace → `ErrDuplicatePhone` carrying the existing name; same phone in another workspace succeeds; `List` paginates (26 rows → page 1 has 25, page 2 has 1, `Total` 26) and `q` matches name, phone digits, company (case-insensitive; `%`/`_` escaped).
- [ ] Implement `customer.Store` on `*pgxpool.Pool` with `pgx.Tx`-taking `createTx` so the lead store can create a customer inside its own transaction.

### C. Lead domain and store
- [ ] `lead_test.go` (pure): `CanTransition` table (same → `ErrSameStatus`; `menang`→`kalah` and `kalah`→`menang` → `ErrClosedToClosed`; `baru`→`menang` ok; `kalah`→`baru` ok); `Validate` rejects empty title, 151-char title, unknown status/source/type, negative value, `lost_reason` without `kalah`, follow-up date in the wrong format; `Describe` renders `status_changed` as "Status: Baru → Dihubungi", `updated` with labels.
- [ ] `store_test.go`: create with existing customer writes lead + `created` activity in one transaction; create with the other workspace's customer id → `ErrCustomerNotFound` and no lead row; create with inline new customer makes both rows; `ChangeStatus` writes `status_changed` activity with from/to, sets `status_changed_at`, stores `lost_reason` for `kalah`, clears it on reopen, sets `next_follow_up_on` = today+3 (workspace zone) on `penawaran` when empty, clears it on close; `ChangeStatus` same status → `ErrSameStatus`, `menang`→`kalah` → `ErrClosedToClosed`, nothing written; `AddNote` writes `note`; `Update` records `updated` with the changed field list only; `UPDATE lead_activities` raises (trigger); `List` filters `status=aktif` (open only), by one status, `q` on title and customer name, sorts by `nilai` (value desc, nulls last) and `followup` (date asc, nulls last), paginates; cross-workspace `Get`/`Update`/`ChangeStatus`/`AddNote`/`Activities` → `ErrNotFound` with rows unchanged; `Summary` counts per status and sums open value.

### D. Customer handlers and UI
- [ ] `customer_web_test.go`: list empty state ("Belum ada pelanggan"); create via form → 303 to detail, detail shows name/phone (display format)/company; validation (empty name, bad phone, bad e-mail, 151 chars) → 200 with messages and `value="…"` kept; duplicate phone message names the existing customer; edit → values updated; search `?q=` finds by name and by phone typed as `0812…`; `?page=abc`, `?page=0`, `?page=9999` → 200 (clamped); bad uuid → 404.
- [ ] Templates: list (cards on phone, table on ≥ sm), form (shared create/edit), detail with "Prospek pelanggan ini" and "Catat prospek" link. Nav updated. `listing.go` + tests.

### E. Lead handlers, pipeline and UI
- [x] `lead_web_test.go`: list empty state ("Belum ada prospek"); create with existing customer → 303 detail; create with inline customer; create with foreign customer id → the form again with "Pelanggan tidak ditemukan." (a 404 would discard the typed lead; nothing leaks either way); validation keeps all inputs (title, value as typed, description); status change via plain POST → 303 and detail shows new badge and "Status: Baru → Dihubungi" in timeline; status change via `HX-Request` → 200 fragment without `<html`, contains success message and timeline OOB; same status → message "Status tidak berubah."; `kalah` with reason → reason shown; `menang` after `kalah` → refused message; reopen allowed; note via htmx → fragment with cleared textarea and timeline containing the note; note empty → error; filter chips (`?status=penawaran`) show only that status; `?status=semua` shows closed too; sort params; search by customer name; `?pelanggan=` preselects; estimated value parse error message; dashboard shows counts and CTAs.
- [ ] Templates: list with status chips + counts, row shows title, customer, badge, value, follow-up (overdue mark), age; form; detail with status panel (select + reason textarea + save), note form, timeline; beranda summary.

### F. Tenant matrix, pagination bounds, performance check
- [ ] `tenant_matrix_test.go`: workspace B hits every route in the table above with A's customer id and lead id (GET and POST with valid bodies) → 404 for all; afterwards A's rows are unchanged (status, name) and A's activity count did not grow.
- [ ] 200-lead workspace: list request wall time logged and asserted < 1 s in test (dev-machine number recorded in PROJECT_STATUS).
- [ ] Unauthenticated access to every new route → 303 `/masuk`.

### G. Review, regression, docs
- [ ] `gofmt -l cmd internal`, `go vet ./...`, `go test -race -count=1 ./...` with `REQUIRE_DB=1`, govulncheck (report advisory status honestly), CSS rebuild and commit of `app.css`.
- [ ] Independent review by a fresh reviewer agent (authorization, tenant isolation, SQL, transactions, validation); fix findings; re-run suite.
- [ ] Docs: MVP_PLAN status, DECISIONS D-033..D-040, PROJECT_STATUS (tests actually run, how to try), ARCHITECTURE (activity vs audit), RISK_REGISTER if needed. Report; wait for commit approval.

## Review focus (inputs the spec implies but no acceptance line names)

1. Phone typed with a foreign prefix or a landline (`+65…`, `021…`): must be accepted or rejected with the example message, never silently rewritten to a wrong number → phone table test (A).
2. Estimated value typed as `15jt` or `15.000.000,50`: must be rejected with "Nilai estimasi tidak valid. Contoh: 15.000.000", never truncated → `ParseIDR` table (A) and form test (E).
3. Two tabs: status changed in one, then the other submits the old status again → `ErrSameStatus` message, no duplicate activity (C, E).
4. Search string containing `%`, `_` or very long input: no error, no full-table match, truncated to 100 chars → store search test (B) and `cleanQuery` test (D).
5. Lead created for an existing customer while a second user renames that customer: the lead detail always shows the current customer name (join, not snapshot) → covered by the detail query reading `customers.name` (C/E).

## Verification

```bash
export PATH=$HOME/sdk/go1.27.2/bin:$PATH
make migrate && make run          # http://localhost:8080 → /app/prospek, /app/pelanggan
make test-db                      # full suite against Postgres (sandbox must be off for DB and port tests)
```
