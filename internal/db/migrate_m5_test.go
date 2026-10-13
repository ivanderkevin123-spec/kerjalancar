package db_test

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kerjalancar/internal/db"
	"kerjalancar/internal/db/testdb"
)

// Migration 00005 swaps the activity-kind CHECK for a lookup table without
// touching existing history, and adds the quotation tables.
func TestMigration5KeepsHistoryAndAddsQuotations(t *testing.T) {
	admin := testdb.Pool(t)
	ctx := context.Background()
	name := "kl_m5_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	_, err := admin.Exec(ctx, "CREATE DATABASE "+name)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)") })
	u, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	require.NoError(t, err)
	u.Path = "/" + name
	pool, err := db.NewPool(ctx, u.String())
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	require.NoError(t, db.MigrateTo(ctx, pool, 4))
	ws, user, cust, lead, act := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, q := range []string{
		`INSERT INTO users (id, email, password_hash, name) VALUES ('` + user.String() + `', 'm5@contoh.id', 'x', 'M5')`,
		`INSERT INTO workspaces (id, name) VALUES ('` + ws.String() + `', 'Studio M5')`,
		`INSERT INTO memberships (workspace_id, user_id, role) VALUES ('` + ws.String() + `', '` + user.String() + `', 'owner')`,
		`INSERT INTO customers (id, workspace_id, name) VALUES ('` + cust.String() + `', '` + ws.String() + `', 'Budi')`,
		`INSERT INTO leads (id, workspace_id, customer_id, title, status) VALUES ('` + lead.String() + `', '` + ws.String() + `', '` + cust.String() + `', 'Dapur', 'penawaran')`,
		`INSERT INTO lead_activities (id, workspace_id, lead_id, kind, body, data) VALUES ('` + act.String() + `', '` + ws.String() + `', '` + lead.String() + `', 'note', 'lama', '{}')`,
	} {
		_, err := pool.Exec(ctx, q)
		require.NoError(t, err, q)
	}

	require.NoError(t, db.MigrateTo(ctx, pool, 5), "up to 5 only; 00006 adds more kinds")

	var kinds int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM lead_activity_kinds`).Scan(&kinds))
	assert.Equal(t, 13, kinds)
	var body string
	require.NoError(t, pool.QueryRow(ctx, `SELECT body FROM lead_activities WHERE id = $1`, act).Scan(&body))
	assert.Equal(t, "lama", body, "existing history survives the constraint swap")
	_, err = pool.Exec(ctx, `INSERT INTO lead_activities (id, workspace_id, lead_id, kind, data) VALUES ($1, $2, $3, 'bogus', '{}')`, uuid.New(), ws, lead)
	assert.Error(t, err, "unknown kinds are still refused (foreign key)")
	_, err = pool.Exec(ctx, `INSERT INTO lead_activities (id, workspace_id, lead_id, kind, data) VALUES ($1, $2, $3, 'quotation_sent', '{}')`, uuid.New(), ws, lead)
	assert.NoError(t, err, "new kinds are rows, not a rewritten CHECK")

	q1, q2 := uuid.New(), uuid.New()
	_, err = pool.Exec(ctx, `INSERT INTO quotations (id, workspace_id, lead_id, customer_id, seq, status) VALUES ($1, $2, $3, $4, 1, 'draft')`, q1, ws, lead, cust)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO quotations (id, workspace_id, lead_id, customer_id, seq, status) VALUES ($1, $2, $3, $4, 1, 'draft')`, q2, ws, lead, cust)
	assert.Error(t, err, "numbers are unique per workspace")
	_, err = pool.Exec(ctx, `INSERT INTO quotation_items (id, workspace_id, quotation_id, position, description, qty_milli, unit_price, line_total) VALUES ($1, $2, $3, 0, 'Kitchen set', 2500, 1999, 4998)`, uuid.New(), ws, q1)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO quotation_items (id, workspace_id, quotation_id, position, description, qty_milli, unit_price, line_total) VALUES ($1, $2, $3, 1, 'Nol', 0, 1999, 0)`, uuid.New(), ws, q1)
	assert.Error(t, err, "quantity must be positive")

	// Rolling back drops the quotation tables and their history lines (the
	// restored CHECK cannot hold them) but keeps every older activity.
	require.NoError(t, db.MigrateTo(ctx, pool, 4), "down to 4 with quotation activity present")
	var tables int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name IN ('quotations', 'quotation_items', 'quotation_sequences', 'lead_activity_kinds')`).Scan(&tables))
	assert.Equal(t, 0, tables, "quotation tables are gone after the rollback")
	require.NoError(t, pool.QueryRow(ctx, `SELECT body FROM lead_activities WHERE id = $1`, act).Scan(&body))
	assert.Equal(t, "lama", body, "older history survives the rollback")
	var quotationLines int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM lead_activities WHERE kind LIKE 'quotation%'`).Scan(&quotationLines))
	assert.Equal(t, 0, quotationLines)
	applied, err := db.Migrate(ctx, pool)
	require.NoError(t, err, "and up again")
	assert.Contains(t, applied, int64(5))
}
