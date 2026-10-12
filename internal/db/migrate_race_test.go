package db_test

import (
	"context"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"kerjalancar/internal/db"
	"kerjalancar/internal/db/testdb"
)

// Several test binaries (and, in production, several app instances) may run
// Migrate at the same moment against a database that has never been migrated.
// All of them must succeed.
func TestMigrateConcurrentOnFreshDatabase(t *testing.T) {
	admin := testdb.Pool(t)
	ctx := context.Background()

	name := "kl_race_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	_, err := admin.Exec(ctx, "CREATE DATABASE "+name)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	})

	u, err := url.Parse(os.Getenv("TEST_DATABASE_URL"))
	require.NoError(t, err)
	u.Path = "/" + name
	freshURL := u.String()

	const migrators = 4
	pools := make([]interface{ Close() }, 0, migrators)
	start := make(chan struct{})
	errs := make([]error, migrators)
	var wg sync.WaitGroup
	for i := 0; i < migrators; i++ {
		pool, err := db.NewPool(ctx, freshURL)
		require.NoError(t, err)
		pools = append(pools, pool)
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			mctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			_, errs[i] = db.Migrate(mctx, pool)
		}(i)
	}
	close(start)
	wg.Wait()
	for _, p := range pools {
		p.Close()
	}
	for i, err := range errs {
		require.NoError(t, err, "migrator %d", i)
	}
}
