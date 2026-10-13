-- +goose Up
-- Milestone 5: quotations with items and server-computed totals, per-workspace numbering,
-- share links, and activity kinds as a lookup table instead of a rewritten CHECK (D-050..D-054).

CREATE TABLE lead_activity_kinds (
    kind text PRIMARY KEY
);
INSERT INTO lead_activity_kinds (kind) VALUES
    ('created'), ('status_changed'), ('note'), ('updated'),
    ('follow_up_scheduled'), ('follow_up_rescheduled'), ('follow_up_done'), ('follow_up_undone'), ('whatsapp_opened'),
    ('quotation_created'), ('quotation_sent'), ('quotation_accepted'), ('quotation_rejected');
ALTER TABLE lead_activities DROP CONSTRAINT lead_activities_kind_check;
ALTER TABLE lead_activities ADD CONSTRAINT lead_activities_kind_fkey FOREIGN KEY (kind) REFERENCES lead_activity_kinds (kind);

CREATE TABLE quotation_sequences (
    workspace_id uuid PRIMARY KEY REFERENCES workspaces (id) ON DELETE CASCADE,
    last_seq     integer NOT NULL DEFAULT 0
);

CREATE TABLE quotations (
    id               uuid PRIMARY KEY,
    workspace_id     uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    lead_id          uuid NOT NULL,
    customer_id      uuid NOT NULL,
    seq              integer NOT NULL CHECK (seq > 0),
    status           text NOT NULL CHECK (status IN ('draft', 'sent', 'accepted', 'rejected')),
    valid_until      date,
    notes            text CHECK (length(notes) <= 2000),
    discount_pct     smallint NOT NULL DEFAULT 0 CHECK (discount_pct BETWEEN 0 AND 100),
    tax_pct          smallint NOT NULL DEFAULT 0 CHECK (tax_pct BETWEEN 0 AND 100),
    subtotal         bigint NOT NULL DEFAULT 0 CHECK (subtotal >= 0),
    discount_amount  bigint NOT NULL DEFAULT 0 CHECK (discount_amount >= 0),
    tax_amount       bigint NOT NULL DEFAULT 0 CHECK (tax_amount >= 0),
    total            bigint NOT NULL DEFAULT 0 CHECK (total >= 0),
    share_token      text UNIQUE CHECK (length(share_token) = 22),
    share_revoked_at timestamptz,
    first_opened_at  timestamptz,
    open_count       integer NOT NULL DEFAULT 0,
    sent_at          timestamptz,
    accepted_at      timestamptz,
    rejected_at      timestamptz,
    version          integer NOT NULL DEFAULT 1,
    created_by       uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, id),
    UNIQUE (workspace_id, seq),
    FOREIGN KEY (workspace_id, lead_id) REFERENCES leads (workspace_id, id),
    FOREIGN KEY (workspace_id, customer_id) REFERENCES customers (workspace_id, id)
);
CREATE INDEX quotations_workspace_lead_idx   ON quotations (workspace_id, lead_id);
CREATE INDEX quotations_workspace_status_idx ON quotations (workspace_id, status, created_at DESC);

CREATE TABLE quotation_items (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    quotation_id uuid NOT NULL,
    position     integer NOT NULL CHECK (position >= 0),
    description  text NOT NULL CHECK (length(description) BETWEEN 1 AND 300),
    qty_milli    bigint NOT NULL CHECK (qty_milli BETWEEN 1 AND 1000000000),
    unit         text CHECK (length(unit) <= 20),
    unit_price   bigint NOT NULL CHECK (unit_price BETWEEN 0 AND 10000000000000),
    line_total   bigint NOT NULL CHECK (line_total >= 0),
    UNIQUE (workspace_id, id),
    FOREIGN KEY (workspace_id, quotation_id) REFERENCES quotations (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX quotation_items_quotation_idx ON quotation_items (workspace_id, quotation_id, position);

-- +goose Down
DROP TABLE quotation_items;
DROP TABLE quotations;
DROP TABLE quotation_sequences;
ALTER TABLE lead_activities DROP CONSTRAINT lead_activities_kind_fkey;
-- Rolling back loses the quotation history lines: the restored CHECK cannot
-- hold them (the append-only trigger guards updates, not deletes).
DELETE FROM lead_activities WHERE kind LIKE 'quotation\_%';
ALTER TABLE lead_activities ADD CONSTRAINT lead_activities_kind_check CHECK (kind IN (
    'created', 'status_changed', 'note', 'updated',
    'follow_up_scheduled', 'follow_up_rescheduled', 'follow_up_done', 'follow_up_undone', 'whatsapp_opened'));
DROP TABLE lead_activity_kinds;
