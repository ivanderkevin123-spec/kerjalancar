package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate applies every pending migration and returns the versions it applied
// (empty when the database is already current).
func Migrate(ctx context.Context, pool *pgxpool.Pool) ([]int64, error) {
	provider, closeFn, err := newProvider(pool)
	if err != nil {
		return nil, err
	}
	defer closeFn()

	results, err := provider.Up(ctx)
	if err != nil {
		return nil, fmt.Errorf("migrate up: %w", err)
	}
	versions := make([]int64, 0, len(results))
	for _, r := range results {
		versions = append(versions, r.Source.Version)
	}
	return versions, nil
}

// MigrateTo applies or rolls back migrations up to and including version (tests use it to
// stage data before a later migration's backfill runs).
func MigrateTo(ctx context.Context, pool *pgxpool.Pool, version int64) error {
	provider, closeFn, err := newProvider(pool)
	if err != nil {
		return err
	}
	defer closeFn()
	current, err := provider.GetDBVersion(ctx)
	if err != nil {
		return fmt.Errorf("migrate: current version: %w", err)
	}
	if version < current {
		if _, err := provider.DownTo(ctx, version); err != nil {
			return fmt.Errorf("migrate down to %d: %w", version, err)
		}
		return nil
	}
	if _, err := provider.UpTo(ctx, version); err != nil {
		return fmt.Errorf("migrate up to %d: %w", version, err)
	}
	return nil
}

// MigrationStatus lists every known migration with its applied state.
func MigrationStatus(ctx context.Context, pool *pgxpool.Pool) ([]*goose.MigrationStatus, error) {
	provider, closeFn, err := newProvider(pool)
	if err != nil {
		return nil, err
	}
	defer closeFn()

	statuses, err := provider.Status(ctx)
	if err != nil {
		return nil, fmt.Errorf("migration status: %w", err)
	}
	return statuses, nil
}

// newProvider adapts the pgx pool to database/sql for goose. Closing the
// returned *sql.DB does not close the pool (see pgx stdlib.OpenDBFromPool docs).
func newProvider(pool *pgxpool.Pool) (*goose.Provider, func(), error) {
	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return nil, nil, fmt.Errorf("migrations fs: %w", err)
	}
	// Several app instances or test binaries may migrate at the same moment.
	// A Postgres advisory lock serialises them, so concurrent CREATE TABLE
	// statements for the same migration cannot collide.
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return nil, nil, fmt.Errorf("goose locker: %w", err)
	}
	sqlDB := stdlib.OpenDBFromPool(pool)
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, sub, goose.WithSessionLocker(locker))
	if err != nil {
		_ = sqlDB.Close()
		return nil, nil, fmt.Errorf("goose provider: %w", err)
	}
	return provider, func() { _ = sqlDB.Close() }, nil
}
