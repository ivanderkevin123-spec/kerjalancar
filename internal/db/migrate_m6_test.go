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

// Migration 00006 adds invoices, append-only payments and reversals on top of
// a v5 database that already holds history and a quotation, and rolls back
// with that data present.
func TestMigration6AddsInvoicesAndAppendOnlyPayments(t *testing.T) {
	admin := testdb.Pool(t)
	ctx := context.Background()
	name := "kl_m6_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	_, err := admin.Exec(ctx, "CREATE DATABASE "+name)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)") })
	u, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	require.NoError(t, err)
	u.Path = "/" + name
	pool, err := db.NewPool(ctx, u.String())
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	require.NoError(t, db.MigrateTo(ctx, pool, 5))
	ws, user, cust, lead, act, q1 := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	for _, stmt := range []string{
		`INSERT INTO users (id, email, password_hash, name) VALUES ('` + user.String() + `', 'm6@contoh.id', 'x', 'M6')`,
		`INSERT INTO workspaces (id, name) VALUES ('` + ws.String() + `', 'Studio M6')`,
		`INSERT INTO memberships (workspace_id, user_id, role) VALUES ('` + ws.String() + `', '` + user.String() + `', 'owner')`,
		`INSERT INTO customers (id, workspace_id, name) VALUES ('` + cust.String() + `', '` + ws.String() + `', 'Budi')`,
		`INSERT INTO leads (id, workspace_id, customer_id, title, status) VALUES ('` + lead.String() + `', '` + ws.String() + `', '` + cust.String() + `', 'Dapur', 'menang')`,
		`INSERT INTO lead_activities (id, workspace_id, lead_id, kind, body, data) VALUES ('` + act.String() + `', '` + ws.String() + `', '` + lead.String() + `', 'note', 'lama', '{}')`,
		`INSERT INTO quotations (id, workspace_id, lead_id, customer_id, seq, status, total) VALUES ('` + q1.String() + `', '` + ws.String() + `', '` + lead.String() + `', '` + cust.String() + `', 1, 'accepted', 4998)`,
		`INSERT INTO quotation_items (id, workspace_id, quotation_id, position, description, qty_milli, unit_price, line_total) VALUES ('` + uuid.NewString() + `', '` + ws.String() + `', '` + q1.String() + `', 0, 'Kitchen set', 2500, 1999, 4998)`,
	} {
		_, err := pool.Exec(ctx, stmt)
		require.NoError(t, err, stmt)
	}

	applied, err := db.Migrate(ctx, pool)
	require.NoError(t, err)
	assert.Contains(t, applied, int64(6))

	var kinds int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM lead_activity_kinds`).Scan(&kinds))
	assert.Equal(t, 18, kinds, "13 kinds from 00005 plus five invoice and payment kinds")

	inv, other := uuid.New(), uuid.New()
	_, err = pool.Exec(ctx, `INSERT INTO invoices (id, workspace_id, lead_id, customer_id, quotation_id, seq, status, total) VALUES ($1, $2, $3, $4, $5, 1, 'draft', 4998)`, inv, ws, lead, cust, q1)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO invoices (id, workspace_id, lead_id, customer_id, seq, status) VALUES ($1, $2, $3, $4, 1, 'draft')`, other, ws, lead, cust)
	assert.Error(t, err, "numbers are unique per workspace")
	_, err = pool.Exec(ctx, `INSERT INTO invoices (id, workspace_id, lead_id, customer_id, seq, status) VALUES ($1, $2, $3, $4, 2, 'paid')`, other, ws, lead, cust)
	assert.Error(t, err, "only draft, issued and void are stored statuses")
	_, err = pool.Exec(ctx, `INSERT INTO invoice_items (id, workspace_id, invoice_id, position, description, qty_milli, unit_price, line_total) VALUES ($1, $2, $3, 0, 'Nol', 0, 1999, 0)`, uuid.New(), ws, inv)
	assert.Error(t, err, "quantity must be positive")
	_, err = pool.Exec(ctx, `INSERT INTO invoice_items (id, workspace_id, invoice_id, position, description, qty_milli, unit_price, line_total) VALUES ($1, $2, $3, 0, 'Kitchen set', 2500, 1999, 4998)`, uuid.New(), ws, inv)
	require.NoError(t, err)

	pay := uuid.New()
	_, err = pool.Exec(ctx, `INSERT INTO payments (id, workspace_id, invoice_id, kind, method, amount, received_on, idempotency_key, recorded_by) VALUES ($1, $2, $3, 'dp', 'transfer', 2000, '2026-10-13', 'kunci-pertama-0001', $4)`, pay, ws, inv, user)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO payments (id, workspace_id, invoice_id, kind, method, amount, received_on, idempotency_key) VALUES ($1, $2, $3, 'dp', 'transfer', 1, '2026-10-13', 'kunci-pertama-0001')`, uuid.New(), ws, inv)
	assert.Error(t, err, "the idempotency key is unique per workspace")
	_, err = pool.Exec(ctx, `INSERT INTO payments (id, workspace_id, invoice_id, kind, method, amount, received_on, idempotency_key) VALUES ($1, $2, $3, 'dp', 'transfer', 0, '2026-10-13', 'kunci-nol-00000001')`, uuid.New(), ws, inv)
	assert.Error(t, err, "amount must be positive")
	_, err = pool.Exec(ctx, `INSERT INTO payments (id, workspace_id, invoice_id, kind, method, amount, received_on, idempotency_key) VALUES ($1, $2, $3, 'refund', 'transfer', 5, '2026-10-13', 'kunci-jenis-0000001')`, uuid.New(), ws, inv)
	assert.Error(t, err, "unknown kinds are refused")
	_, err = pool.Exec(ctx, `UPDATE payments SET amount = 1 WHERE id = $1`, pay)
	assert.Error(t, err, "payments are append-only (update)")
	_, err = pool.Exec(ctx, `DELETE FROM payments WHERE id = $1`, pay)
	assert.Error(t, err, "payments are append-only (delete)")
	var amount int64
	require.NoError(t, pool.QueryRow(ctx, `SELECT amount FROM payments WHERE id = $1`, pay).Scan(&amount))
	assert.Equal(t, int64(2000), amount)

	_, err = pool.Exec(ctx, `INSERT INTO payment_reversals (id, workspace_id, payment_id, reason, reversed_by) VALUES ($1, $2, $3, 'salah input', $4)`, uuid.New(), ws, pay, user)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO payment_reversals (id, workspace_id, payment_id, reason) VALUES ($1, $2, $3, 'lagi')`, uuid.New(), ws, pay)
	assert.Error(t, err, "one reversal per payment")
	_, err = pool.Exec(ctx, `INSERT INTO lead_activities (id, workspace_id, lead_id, kind, data) VALUES ($1, $2, $3, 'payment_recorded', '{}')`, uuid.New(), ws, lead)
	require.NoError(t, err, "new kinds are rows in the lookup table")

	// Rollback with data present: invoice tables and their history lines go,
	// older history and the quotation stay.
	require.NoError(t, db.MigrateTo(ctx, pool, 5), "down to 5 with payments and reversals present")
	var tables int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name IN ('invoices', 'invoice_items', 'invoice_sequences', 'payments', 'payment_reversals')`).Scan(&tables))
	assert.Equal(t, 0, tables)
	var body string
	require.NoError(t, pool.QueryRow(ctx, `SELECT body FROM lead_activities WHERE id = $1`, act).Scan(&body))
	assert.Equal(t, "lama", body)
	var quotations int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM quotations WHERE id = $1`, q1).Scan(&quotations))
	assert.Equal(t, 1, quotations, "the quotation survives the rollback")
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM lead_activity_kinds`).Scan(&kinds))
	assert.Equal(t, 13, kinds)
	applied, err = db.Migrate(ctx, pool)
	require.NoError(t, err, "and up again")
	assert.Contains(t, applied, int64(6))
}
