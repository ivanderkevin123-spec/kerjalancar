// Package testdb gives tests a migrated PostgreSQL pool and per-test transactions.
package testdb

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"kerjalancar/internal/db"
)

var (
	once    sync.Once
	pool    *pgxpool.Pool
	poolErr error
)

// Pool returns a process-wide pool connected to TEST_DATABASE_URL with all
// migrations applied. Tests skip when the variable is unset; with REQUIRE_DB=1
// (CI) they fail instead, so a misconfigured CI cannot pass by skipping.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		if os.Getenv("REQUIRE_DB") == "1" {
			t.Fatal("TEST_DATABASE_URL is required because REQUIRE_DB=1")
		}
		t.Skip("TEST_DATABASE_URL not set; skipping database test")
	}
	once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		// Several test binaries start at once and compete with the compiler for
		// CPU; a single 5 s connect attempt can time out on a loaded machine or
		// CI runner, so keep trying within the overall budget.
		for {
			pool, poolErr = db.NewPool(ctx, url)
			if poolErr == nil || ctx.Err() != nil {
				break
			}
			time.Sleep(2 * time.Second)
		}
		if poolErr != nil {
			return
		}
		_, poolErr = db.Migrate(ctx, pool)
	})
	if poolErr != nil {
		t.Fatalf("test database: %v", poolErr)
	}
	return pool
}

// Tx begins a transaction on the test pool and rolls it back when the test
// ends, so tests never see each other's rows.
func Tx(t *testing.T) pgx.Tx {
	t.Helper()
	tx, err := Pool(t).Begin(context.Background())
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	return tx
}
