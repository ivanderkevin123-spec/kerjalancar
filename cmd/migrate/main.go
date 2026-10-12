// Command migrate applies or reports database migrations. Usage: migrate up|status
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"kerjalancar/internal/db"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 1 || (args[0] != "up" && args[0] != "status") {
		return fmt.Errorf("usage: migrate up|status")
	}
	// Only DATABASE_URL matters here; web settings must never block a migration job.
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	pool, err := db.NewPool(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer pool.Close()

	switch args[0] {
	case "up":
		versions, err := db.Migrate(ctx, pool)
		if err != nil {
			return err
		}
		if len(versions) == 0 {
			fmt.Println("database is up to date")
			return nil
		}
		for _, v := range versions {
			fmt.Printf("applied %d\n", v)
		}
	case "status":
		statuses, err := db.MigrationStatus(ctx, pool)
		if err != nil {
			return err
		}
		for _, s := range statuses {
			applied := "pending"
			if !s.AppliedAt.IsZero() {
				applied = s.AppliedAt.UTC().Format(time.RFC3339)
			}
			fmt.Printf("%-8d %-10s %s\n", s.Source.Version, s.State, applied)
		}
	}
	return nil
}
