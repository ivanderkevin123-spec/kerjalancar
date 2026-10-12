package db_test

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kerjalancar/internal/db"
	"kerjalancar/internal/db/testdb"
)

// Migration 00004 must carry existing data forward: an open lead with a planned
// date becomes an open follow-up, and every existing workspace gets the three
// message templates.
func TestMigration4BackfillsFollowUpsAndSeedsTemplates(t *testing.T) {
	admin := testdb.Pool(t)
	ctx := context.Background()
	name := "kl_backfill_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	_, err := admin.Exec(ctx, "CREATE DATABASE "+name)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)") })

	u, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	require.NoError(t, err)
	u.Path = "/" + name
	pool, err := db.NewPool(ctx, u.String())
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	require.NoError(t, db.MigrateTo(ctx, pool, 3))
	ws, user, cust, open, closed := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	_, err = pool.Exec(ctx, `INSERT INTO users (id, email, password_hash, name) VALUES ($1, 'bf@contoh.id', 'x', 'BF')`, user)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO workspaces (id, name) VALUES ($1, 'Studio BF')`, ws)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO memberships (workspace_id, user_id, role) VALUES ($1, $2, 'owner')`, ws, user)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO customers (id, workspace_id, name) VALUES ($1, $2, 'Budi')`, cust, ws)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO leads (id, workspace_id, customer_id, title, status, next_follow_up_on) VALUES ($1, $2, $3, 'Terbuka', 'penawaran', '2026-10-25')`, open, ws, cust)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO leads (id, workspace_id, customer_id, title, status, next_follow_up_on) VALUES ($1, $2, $3, 'Tertutup', 'menang', '2026-10-25')`, closed, ws, cust)
	require.NoError(t, err)

	applied, err := db.Migrate(ctx, pool)
	require.NoError(t, err)
	assert.Contains(t, applied, int64(4))

	var leadID uuid.UUID
	var due time.Time
	var doneAt *time.Time
	require.NoError(t, pool.QueryRow(ctx, `SELECT lead_id, due_on, done_at FROM follow_ups WHERE workspace_id = $1`, ws).Scan(&leadID, &due, &doneAt))
	assert.Equal(t, open, leadID, "only the open lead is backfilled")
	assert.Equal(t, time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC), due)
	assert.Nil(t, doneAt)

	var stale *time.Time
	require.NoError(t, pool.QueryRow(ctx, `SELECT next_follow_up_on FROM leads WHERE id = $1`, closed).Scan(&stale))
	assert.Nil(t, stale, "a closed lead keeps no cached follow-up date (D-043)")
	var kept *time.Time
	require.NoError(t, pool.QueryRow(ctx, `SELECT next_follow_up_on FROM leads WHERE id = $1`, open).Scan(&kept))
	require.NotNil(t, kept)

	var templates int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM message_templates WHERE workspace_id = $1`, ws).Scan(&templates))
	assert.Equal(t, 3, templates)

	_, err = pool.Exec(ctx, `UPDATE workspaces SET timezone = 'Europe/Berlin' WHERE id = $1`, ws)
	assert.Error(t, err, "time zones outside Indonesia are refused by the CHECK")
}
