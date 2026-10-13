-- +goose Up
-- Milestone 6: invoices with copied items and per-workspace numbering, append-only
-- payments with reversals, share links as in 00005 (D-056..D-060).

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
    FOREIGN KEY (workspace_id, quotation_id) REFERENCES quotations (workspace_id, id)
);
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

-- Payments are the owner's notes of money received (D-011), never edited:
-- a mistake is undone by a reversal row with a reason (D-010, R-11).
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
-- Rolling back loses invoices, payments and their history lines; older history
-- and quotations stay. Dropping the tables does not fire the row trigger.
DROP TRIGGER payments_append_only ON payments;
DROP FUNCTION payments_append_only();
DROP TABLE payment_reversals;
DROP TABLE payments;
DROP TABLE invoice_items;
DROP TABLE invoices;
DROP TABLE invoice_sequences;
DELETE FROM lead_activities WHERE kind IN ('invoice_created', 'invoice_issued', 'invoice_voided', 'payment_recorded', 'payment_reversed');
DELETE FROM lead_activity_kinds WHERE kind IN ('invoice_created', 'invoice_issued', 'invoice_voided', 'payment_recorded', 'payment_reversed');
