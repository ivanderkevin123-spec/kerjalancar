-- +goose Up
-- Milestone 4: follow-ups (source of truth for "next follow-up"), WhatsApp message templates,
-- validated workspace time zones, new activity kinds. leads.next_follow_up_on becomes a cache
-- of the open follow-up's due date, written in the same transaction by the follow-up store (D-043).

ALTER TABLE workspaces ADD CONSTRAINT workspaces_timezone_check
    CHECK (timezone IN ('Asia/Jakarta', 'Asia/Makassar', 'Asia/Jayapura'));

CREATE TABLE follow_ups (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    lead_id      uuid NOT NULL,
    due_on       date NOT NULL,
    note         text CHECK (length(note) <= 500),
    done_at      timestamptz,
    done_by      uuid REFERENCES users (id) ON DELETE SET NULL,
    created_by   uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, id),
    FOREIGN KEY (workspace_id, lead_id) REFERENCES leads (workspace_id, id) ON DELETE CASCADE
);
-- At most one open follow-up per lead (D-043).
CREATE UNIQUE INDEX follow_ups_one_open_per_lead_idx ON follow_ups (workspace_id, lead_id) WHERE done_at IS NULL;
CREATE INDEX follow_ups_open_due_idx ON follow_ups (workspace_id, due_on) WHERE done_at IS NULL;
CREATE INDEX follow_ups_done_idx ON follow_ups (workspace_id, done_at DESC) WHERE done_at IS NOT NULL;

-- Backfill: every open lead with a planned date gets an open follow-up; closed leads
-- drop their cached date so the cache invariant (D-043) holds from the first day.
INSERT INTO follow_ups (id, workspace_id, lead_id, due_on, created_at, updated_at)
SELECT gen_random_uuid(), workspace_id, id, next_follow_up_on, now(), now()
FROM leads
WHERE next_follow_up_on IS NOT NULL AND status IN ('baru', 'dihubungi', 'survei', 'penawaran');
UPDATE leads SET next_follow_up_on = NULL
WHERE next_follow_up_on IS NOT NULL AND status IN ('menang', 'kalah');

CREATE TABLE message_templates (
    id           uuid PRIMARY KEY,
    workspace_id uuid NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
    key          text NOT NULL CHECK (key IN ('followup_penawaran', 'pengingat_dp', 'pengingat_pelunasan')),
    name         text NOT NULL CHECK (length(name) BETWEEN 1 AND 60),
    body         text NOT NULL CHECK (length(body) BETWEEN 1 AND 1000),
    position     smallint NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, id),
    UNIQUE (workspace_id, key)
);
-- Seed the three slots for every existing workspace (new workspaces: template.Seeds via workspace.CreateWithOwner).
-- Keep this text in sync with internal/template/template.go.
INSERT INTO message_templates (id, workspace_id, key, name, body, position)
SELECT gen_random_uuid(), w.id, s.key, s.name, s.body, s.position
FROM workspaces w
CROSS JOIN (VALUES
    ('followup_penawaran', 'Follow-up penawaran', 'Halo {nama}, saya dari {usaha}. Apakah penawaran untuk proyek senilai {nilai} sudah sempat dilihat? Jika ada yang perlu disesuaikan, kabari saya ya.', 1),
    ('pengingat_dp', 'Pengingat DP', 'Halo {nama}, terima kasih sudah mempercayakan proyek ini ke {usaha}. Sebagai pengingat, DP sebesar {nilai} bisa ditransfer agar jadwal pengerjaan dapat kami kunci.', 2),
    ('pengingat_pelunasan', 'Pengingat pelunasan', 'Halo {nama}, pekerjaan dari {usaha} sudah selesai. Mohon pelunasan sebesar {nilai}. Terima kasih atas kepercayaannya.', 3)
) AS s (key, name, body, position);

ALTER TABLE lead_activities DROP CONSTRAINT lead_activities_kind_check;
ALTER TABLE lead_activities ADD CONSTRAINT lead_activities_kind_check CHECK (kind IN (
    'created', 'status_changed', 'note', 'updated',
    'follow_up_scheduled', 'follow_up_rescheduled', 'follow_up_done', 'follow_up_undone', 'whatsapp_opened'));

-- +goose Down
ALTER TABLE lead_activities DROP CONSTRAINT lead_activities_kind_check;
ALTER TABLE lead_activities ADD CONSTRAINT lead_activities_kind_check CHECK (kind IN ('created', 'status_changed', 'note', 'updated'));
DROP TABLE message_templates;
DROP TABLE follow_ups;
ALTER TABLE workspaces DROP CONSTRAINT workspaces_timezone_check;
