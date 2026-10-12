-- +goose Up
-- Milestone 3: customers (contacts), leads (project opportunities), lead activities (append-only history).
-- Every row carries workspace_id; UNIQUE (workspace_id, id) lets child tables use composite foreign keys,
-- so the database itself refuses a lead that points at another workspace's customer (R-04).

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
    id                uuid PRIMARY KEY,
    workspace_id      uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    customer_id       uuid NOT NULL,
    title             text NOT NULL CHECK (length(title) BETWEEN 1 AND 150),
    project_type      text CHECK (project_type IN ('renovasi', 'interior', 'bangun_baru', 'perbaikan', 'lainnya')),
    description       text CHECK (length(description) <= 2000),
    estimated_value   bigint CHECK (estimated_value BETWEEN 0 AND 10000000000000),
    status            text NOT NULL CHECK (status IN ('baru', 'dihubungi', 'survei', 'penawaran', 'menang', 'kalah')),
    source            text CHECK (source IN ('whatsapp', 'instagram', 'facebook', 'referensi', 'google', 'marketplace', 'lainnya')),
    next_follow_up_on date,
    lost_reason       text CHECK (length(lost_reason) <= 500),
    notes             text CHECK (length(notes) <= 2000),
    status_changed_at timestamptz NOT NULL DEFAULT now(),
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, id),
    FOREIGN KEY (workspace_id, customer_id) REFERENCES customers (workspace_id, id),
    CHECK (lost_reason IS NULL OR status = 'kalah')
);
CREATE INDEX leads_workspace_status_idx   ON leads (workspace_id, status, created_at DESC);
CREATE INDEX leads_workspace_customer_idx ON leads (workspace_id, customer_id);
CREATE INDEX leads_workspace_followup_idx ON leads (workspace_id, next_follow_up_on) WHERE next_follow_up_on IS NOT NULL;

CREATE TABLE lead_activities (
    id            uuid PRIMARY KEY,
    workspace_id  uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    lead_id       uuid NOT NULL,
    kind          text NOT NULL CHECK (kind IN ('created', 'status_changed', 'note', 'updated')),
    actor_user_id uuid REFERENCES users (id) ON DELETE SET NULL,
    body          text CHECK (length(body) <= 2000),
    data          jsonb,
    created_at    timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (workspace_id, lead_id) REFERENCES leads (workspace_id, id) ON DELETE CASCADE
);
CREATE INDEX lead_activities_lead_idx ON lead_activities (workspace_id, lead_id, created_at DESC);

-- Activities are history: they may be added, never changed.
-- +goose StatementBegin
CREATE FUNCTION lead_activities_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'lead_activities is append-only';
END
$$;
-- +goose StatementEnd
CREATE TRIGGER lead_activities_no_update BEFORE UPDATE ON lead_activities
    FOR EACH ROW EXECUTE FUNCTION lead_activities_immutable();

-- +goose Down
DROP TABLE lead_activities;
DROP FUNCTION lead_activities_immutable();
DROP TABLE leads;
DROP TABLE customers;
