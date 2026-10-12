package followup_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kerjalancar/internal/db/testdb"
	"kerjalancar/internal/followup"
)

// Lock-order check (deterministic): device 1 holds the lead row, as
// ChangeStatus and Schedule do, while device 2 taps Selesai or Batalkan.
// With the lead → follow-up order, device 2 waits on the lead lock before
// touching follow_ups, so device 1 can still lock the follow-up row and
// commit, after which device 2 finishes. With the reversed order device 2
// would already hold the follow-up row while waiting for the lead, device 1
// would block on it, and Postgres would abort one of them (40P01), which
// fails this test on one side or the other.
func TestDoneAndUndoWaitForTheLeadLockInsteadOfDeadlocking(t *testing.T) {
	f := setup(t)
	pool := testdb.Pool(t)
	fu, err := f.fu.Schedule(f.ctx, f.wsA, f.userA, f.leadA.ID, followup.Normalized{DueOn: date(2026, 10, 23)})
	require.NoError(t, err)

	run := func(t *testing.T, device2 func() error) {
		t.Helper()
		tx, err := pool.Begin(f.ctx) // device 1
		require.NoError(t, err)
		defer func() { _ = tx.Rollback(f.ctx) }()
		_, err = tx.Exec(f.ctx, `SELECT 1 FROM leads WHERE workspace_id = $1 AND id = $2 FOR UPDATE`, f.wsA, f.leadA.ID)
		require.NoError(t, err)

		result := make(chan error, 1)
		go func() { result <- device2() }()

		// Wait until device 2 is blocked on a lock (its first lock must be the lead row).
		require.Eventually(t, func() bool {
			var n int
			err := pool.QueryRow(f.ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&n)
			return err == nil && n >= 1
		}, 5*time.Second, 25*time.Millisecond, "device 2 never started waiting")

		lockCtx, cancel := context.WithTimeout(f.ctx, 4*time.Second) // longer than Postgres' 1 s deadlock_timeout
		defer cancel()
		_, err = tx.Exec(lockCtx, `SELECT 1 FROM follow_ups WHERE workspace_id = $1 AND id = $2 FOR UPDATE`, f.wsA, fu.ID)
		require.NoError(t, err, "device 1 must still get the follow-up row: device 2 may not hold it while waiting for the lead")
		require.NoError(t, tx.Commit(f.ctx))

		select {
		case err := <-result:
			require.NoError(t, err, "device 2 finishes once the lead lock is released")
		case <-time.After(5 * time.Second):
			t.Fatal("device 2 did not finish after the lead lock was released")
		}
	}

	run(t, func() error { _, err := f.fu.Done(f.ctx, f.wsA, f.userA, fu.ID); return err })
	got, err := f.fu.Get(f.ctx, f.wsA, fu.ID)
	require.NoError(t, err)
	assert.False(t, got.Open())

	run(t, func() error { _, err := f.fu.Undo(f.ctx, f.wsA, f.userA, fu.ID); return err })
	got, err = f.fu.Get(f.ctx, f.wsA, fu.ID)
	require.NoError(t, err)
	assert.True(t, got.Open())
	assert.Equal(t, date(2026, 10, 23), *f.cachedDate(t, f.leadA.ID), "cache consistent after the contended undo")
}

var _ pgx.Tx // keep the pgx import explicit for readers of the lock order
