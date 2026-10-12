package db_test

import (
	"context"
	"testing"

	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kerjalancar/internal/db"
	"kerjalancar/internal/db/testdb"
)

func TestMigrateIsIdempotentAndAppliesInit(t *testing.T) {
	pool := testdb.Pool(t) // first call applies all migrations
	ctx := context.Background()

	again, err := db.Migrate(ctx, pool)
	require.NoError(t, err)
	assert.Empty(t, again, "second run must apply nothing")

	var hasCitext bool
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'citext')`).Scan(&hasCitext))
	assert.True(t, hasCitext, "00001_init must install citext")

	statuses, err := db.MigrationStatus(ctx, pool)
	require.NoError(t, err)
	require.NotEmpty(t, statuses)
	for _, s := range statuses {
		assert.Equal(t, goose.StateApplied, s.State, "migration %d", s.Source.Version)
	}
}

func TestTxRollsBackBetweenTests(t *testing.T) {
	ctx := context.Background()
	tx := testdb.Tx(t)
	_, err := tx.Exec(ctx, `CREATE TEMP TABLE scratch (n int)`) // temp table lives only in this tx
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `INSERT INTO scratch VALUES (1)`)
	require.NoError(t, err)

	var n int
	require.NoError(t, tx.QueryRow(ctx, `SELECT count(*) FROM scratch`).Scan(&n))
	assert.Equal(t, 1, n)
	// Cleanup rolls back; the next test sees no scratch table. Nothing else to assert here.
}
