# Milestone 4 — Follow-ups, "Hari ini" list, WhatsApp click-to-chat (plan)

> Plan only; implementation starts after the founder reads this. Same working rules as M3: one branch (`m4-followups-whatsapp`), TDD per task, no new dependencies, no commit without approval, independent review before the milestone is declared done.

**Goal:** The owner opens the app and sees exactly who to contact today and who is overdue, taps once to open WhatsApp with a prepared message for that lead, and marks the follow-up done; every step lands in the lead's history.

**Architecture:** Two new packages (`internal/followup`, `internal/template`) own the rules and SQL; migration `00004` adds `follow_ups` and `message_templates`, backfills follow-ups from `leads.next_follow_up_on`, seeds three Indonesian templates per workspace, widens the activity kinds, and constrains `workspaces.timezone`. The web layer adds the follow-up list, scheduling and done/undo endpoints (htmx fragments), template editing in settings, and a redirect endpoint that records the WhatsApp tap before sending the browser to `wa.me`. "Today" is always computed in the workspace zone from an injectable clock so boundary tests are deterministic.

**Tech stack:** unchanged (Go 1.27, chi, html/template, htmx 2.0.11, Tailwind v4, PostgreSQL 17, pgx, goose, testify).

**Spec:** `docs/MVP_PLAN.md` §M4 · `docs/ARCHITECTURE.md` §4, §5, §7, §8 · `docs/PRODUCT_BRIEF.md` §4 items 6–7 · `docs/RISK_REGISTER.md` R-01, R-08, R-10 · M3 reviewer recommendations (PROJECT_STATUS "Security review (M3)").

## Global constraints

- Every follow-up and template query filters on `workspace_id` from the session (`currentWorkspace(ctx)`); ids from URLs are only combined with it; foreign ids answer 404.
- Dates: `due_on` is `DATE`; "today" is computed in the workspace zone (`Asia/Jakarta`, `Asia/Makassar`, `Asia/Jayapura` only, validated in Go and by a CHECK) from a `Clock func() time.Time` injected into stores and handlers (production: `time.Now`).
- Outbound WhatsApp is a link the user taps (`https://wa.me/<digits>?text=…`); the server never sends messages and never calls a WhatsApp API (brief: no WhatsApp Business API, no AI).
- Templates render placeholders `{nama}`, `{usaha}`, `{nilai}`, `{tautan}`; a placeholder with no data renders as empty, never as the literal token; `{tautan}` stays empty until M5 adds share links.
- Marking done and undoing are idempotent and append activities; no silent edits of history (R-11 principle applied to follow-ups).
- Text limits: follow-up note 500, template name 60, template body 1000. Lists paginated or capped (follow-up tabs show at most 100 rows each; "Selesai" tab paginated 25).
- Migration `00004` only; `00001`–`00003` untouched. No deletion of follow-ups or templates in M4 (edit, done, undo only). No demo data.

## Domain decisions to record (D-043..D-049)

| ID | Proposed decision | Why |
|---|---|---|
| D-043 | **`follow_ups` is the source of truth; at most one open follow-up per lead** (partial unique index on `(workspace_id, lead_id) WHERE done_at IS NULL`). `leads.next_follow_up_on` stays as a cache of the open follow-up's `due_on`, written in the same transaction (store invariant, tested), so the pipeline list keeps sorting without a join. M6 adds an `invoice_id` reference with a one-of CHECK; M4 keeps `lead_id NOT NULL`. | One place to mark done and keep history; the cache keeps M3 queries unchanged |
| D-044 | **Three views**: "Terlambat" (`due_on < today`, open), "Hari ini" (`due_on = today`, open), "Mendatang" (next 7 days, open); "Selesai" lists done follow-ups newest first. Counts on the dashboard come from the same query function as the lists (one function, two callers) so they cannot disagree. | MVP acceptance: counts match the lists; zone boundaries tested |
| D-045 | **Scheduling**: from the lead detail ("Jadwalkan follow-up": date + note) and from the follow-up list ("Jadwalkan ulang"). The M3 rule stays: entering `penawaran` with no open follow-up schedules one for today + 3 days; closing a lead (menang/kalah) marks its open follow-up done with reason "lead ditutup" (activity, not deletion). Other status changes only pre-fill the date in the form, never schedule silently. | Keeps the M3 behaviour pilots will already know; no surprise rows |
| D-046 | **Templates**: table `message_templates` with a fixed `key` per workspace (`followup_penawaran`, `pengingat_dp`, `pengingat_pelunasan`), editable name and body, seeded for existing workspaces by the migration and for new ones by `workspace.CreateWithOwner`. No custom templates or deletion in M4 (edit only); creation arrives when a pilot asks. The two payment templates exist now so wording can be refined before M6 wires them to invoices. | Three fixed slots keep the UI to one select; seeds give value on day one |
| D-047 | **wa.me link**: `https://wa.me/` + E.164 digits without `+` (`customer.WhatsAppDigits`) + `?text=` + `url.QueryEscape` of the rendered template (spaces as `%20`, not `+`). The link is reached through `POST /app/prospek/{id}/wa` (template key in the form), which appends a `whatsapp_opened` activity and answers `303` to the `wa.me` URL, so the history shows which message was opened for which lead. A customer without a number gets a message and a link to add one. | Cheap, honest history ("opened", never "sent"); POST keeps CSRF and avoids logging the message in GET query strings |
| D-048 | **Done / undo**: `done_at`, `done_by`; "Selesai" appends `follow_up_done`, "Batalkan selesai" clears `done_at` and appends `follow_up_undone`; both are no-ops with a message when already in that state; undo is refused when another open follow-up exists for the lead (unique index) with a clear message. | Mistakes are recoverable, history stays complete |
| D-049 | **Activity kinds** widened to `follow_up_scheduled`, `follow_up_rescheduled`, `follow_up_done`, `follow_up_undone`, `whatsapp_opened` (data: template key, due date); the CHECK is rewritten in `00004`. `appendActivity` returns the row via `RETURNING` (reviewer recommendation). | Timeline stays the single business history (D-036) |

Also from the M3 review, folded into this milestone: cache `time.LoadLocation` per zone name; a CI grep that fails when a tenant table is queried without `workspace_id`; `workspaces.timezone` editable in settings (select of the three zones) with the CHECK.

## Schema — `internal/db/migrations/00004_followups_templates.sql`

```sql
-- +goose Up
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
CREATE UNIQUE INDEX follow_ups_one_open_per_lead_idx ON follow_ups (workspace_id, lead_id) WHERE done_at IS NULL;
CREATE INDEX follow_ups_open_due_idx ON follow_ups (workspace_id, due_on) WHERE done_at IS NULL;
CREATE INDEX follow_ups_done_idx ON follow_ups (workspace_id, done_at DESC) WHERE done_at IS NOT NULL;

-- Backfill: every open lead with a planned date gets an open follow-up (ids are v4 here; the app uses v7).
INSERT INTO follow_ups (id, workspace_id, lead_id, due_on, created_at, updated_at)
SELECT gen_random_uuid(), workspace_id, id, next_follow_up_on, now(), now()
FROM leads WHERE next_follow_up_on IS NOT NULL AND status IN ('baru', 'dihubungi', 'survei', 'penawaran');

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
-- Seed the three templates for every existing workspace (new workspaces: workspace.CreateWithOwner).
INSERT INTO message_templates (id, workspace_id, key, name, body, position)
SELECT gen_random_uuid(), w.id, s.key, s.name, s.body, s.position
FROM workspaces w CROSS JOIN (VALUES
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
```

Note: the CHECK constraint name must match what Postgres generated for `00003` (`lead_activities_kind_check`); verify with `\d lead_activities` in task B before relying on it. The migration test asserts 4 migrations, the backfill (a lead with a date → one open follow-up) and the seeds (every workspace has 3 templates).

## Packages and files

```
internal/customer/phone.go                      WhatsAppDigits(e164) string (+ table test incl. foreign codes)
internal/followup/followup.go                   FollowUp, Input, Validate, Bucket (terlambat | hari_ini | mendatang), Today(now, loc)
internal/followup/store.go, store_test.go       Schedule (create or move the open one), Done, Undo, Get, Lists(ws, today) (Overdue, Today, Upcoming), Counts, Done page; cache of leads.next_follow_up_on in the same tx
internal/template/template.go, template_test.go Render(body, Data{Nama, Usaha, Nilai, Tautan}) string; WaLink(e164, text) string; Keys/Labels
internal/template/store.go, store_test.go       ForWorkspace, Get(ws, key), Update(ws, key, name, body); Seed(ctx, tx, ws) used by workspace.CreateWithOwner
internal/lead/activity.go, store.go             new kinds + Describe lines; appendActivity RETURNING; Close* hook: closing marks the open follow-up done
internal/workspace/store.go                     UpdateTimezone (owner), CreateWithOwner seeds templates; timezone validation
internal/web/followup_handlers.go, followup_web_test.go
internal/web/template_handlers.go, template_web_test.go     settings: list and edit the three templates; preview with sample data
internal/web/whatsapp_handlers.go, whatsapp_web_test.go     POST /app/prospek/{id}/wa → activity + 303 wa.me
internal/web/app_handlers.go                    beranda: counts + top 5 of today/overdue; pengaturan: timezone select
internal/web/templates/pages/followup_daftar.html, templat_*.html; prospek_detail.html (follow-up panel, WA button with template select); beranda.html; pengaturan.html
internal/web/tenant_matrix_test.go              extended with follow-up, template and WA routes
internal/web/clock.go                           Deps.Clock func() time.Time (tests inject fixed instants at zone boundaries)
internal/db/migrations/00004_followups_templates.sql; migrate_test.go (≥ 4, backfill, seeds)
.github/workflows/ci.yml                        tenant-table grep step
docs: MVP_PLAN M4 status, DECISIONS D-043..D-049, ARCHITECTURE §4/§7, RISK_REGISTER, PROJECT_STATUS
```

## Routes (under `/app`, `requireLogin`, CSRF)

| Method | Path | Purpose |
|---|---|---|
| GET | `/app/follow-up` | tabs `?tab=hari-ini` (default) `terlambat` `mendatang` `selesai`; rows: customer, lead, due date, note, actions |
| POST | `/app/prospek/{id}/follow-up` | schedule or move the open follow-up (`due_on`, `note`); htmx fragment `prospek_followup` + OOB timeline |
| POST | `/app/follow-up/{id}/selesai` | mark done (idempotent); htmx: row fragment + OOB counts |
| POST | `/app/follow-up/{id}/batal` | undo done (idempotent, refused if another open follow-up exists) |
| POST | `/app/prospek/{id}/wa` | `template` key; records `whatsapp_opened`; `303` to `https://wa.me/…`; no phone → form error |
| GET | `/app/pengaturan/templat` | the three templates with a rendered preview (sample data) |
| POST | `/app/pengaturan/templat/{key}` | edit name and body; htmx fragment with preview |
| POST | `/app/pengaturan` | existing rename gains a `timezone` select (owner only) |

## Tasks (TDD, one commit each)

A. **Pure rules**: `followup.Today`/`Bucket` at zone boundaries (23:59 WIB vs 00:00 WIT on the same instant), `Validate` (date required, not more than 1 year ahead, note ≤ 500); `template.Render` (all placeholders, missing data → empty, no literal `{nama}` left), `WaLink` encoding table (`+`, `&`, newline, emoji, `#`), `customer.WhatsAppDigits`.
B. **Migration 00004 + followup store**: schedule creates one open row and caches the date on the lead; schedule again moves it (`follow_up_rescheduled`); done sets `done_at`/`done_by`, clears the lead cache, appends activity; done twice → no-op; undo restores; lists per bucket with an injected clock in three zones; counts equal list lengths; cross-workspace Get/Done/Undo/Schedule → ErrNotFound and nothing written; backfill and seed assertions; closing a lead marks its follow-up done.
C. **Template store + workspace timezone**: seed on registration; update validates lengths and refuses unknown keys; `UpdateTimezone` owner-only and whitelisted; cross-workspace refused.
D. **Follow-up UI**: list with tabs and empty states ("Tidak ada follow-up hari ini. Bagus!"), done/undo via htmx with counts OOB, schedule panel on the lead detail, dashboard counts and top-5 lists; boundary test through the handler with a fixed clock (`Asia/Jayapura` follow-up appears at 00:00 WIT, not at 00:00 WIB).
E. **WhatsApp**: button on lead detail and follow-up rows with a template select; redirect test asserts `Location` starts with `https://wa.me/62812…?text=` and decodes to the rendered message; activity line "Pesan WhatsApp dibuka: Follow-up penawaran"; no-phone path; cross-workspace 404.
F. **Settings**: timezone select, template editing with live preview (htmx), validation keeps input; matrix and CSRF tests extended to every new POST; mass-assignment negatives (`done_at`, `workspace_id`, `lead_id` in forms ignored).
G. **Regression, review, docs**: gofmt, vet, `make test-db` (`-p 2`), govulncheck (report GO-2026-5932 status), CSS rebuild, independent reviewer agent, fixes, docs, report; wait for commit approval.

## Review focus (inputs the spec implies but no acceptance line names)

1. A customer's number changed after a follow-up was scheduled: the WA link must use the current number (read at click time, never stored on the follow-up).
2. Two devices: follow-up marked done on one, "Selesai" tapped again on the other → "Sudah selesai" message, one activity, no second `done_at`.
3. A lead reopened from `kalah` whose follow-up was auto-closed: the detail offers "Jadwalkan follow-up" again and the history shows both the auto-close and the new schedule.
4. Template body with `{nilai}` for a lead without an estimate: renders "" and the preview says which placeholders will be empty, so the owner edits before sending.
5. Workspace zone changed in settings while follow-ups exist: lists re-bucket on the next request; nothing is rewritten in the database (dates stay dates).

## Out of scope (stays deferred)

Push, e-mail or scheduled reminders (A-15); recurring follow-ups; custom template creation or deletion; follow-ups on invoices (M6); WhatsApp Business API; any AI.

## Estimate

Four working days as in `MVP_PLAN.md` (A–B day 1, C–D day 2, E–F day 3, G day 4), assuming the Postgres container and the sequential test runner behave as in M3.
