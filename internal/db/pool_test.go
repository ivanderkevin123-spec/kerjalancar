package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kerjalancar/internal/db"
)

func TestNewPoolErrorDoesNotLeakPassword(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	// Port 1 is closed on every developer machine; the connect must fail fast.
	_, err := db.NewPool(ctx, "postgres://app:hunter2secret@127.0.0.1:1/app?sslmode=disable")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "hunter2secret")
}

func TestNewPoolRejectsGarbageURL(t *testing.T) {
	_, err := db.NewPool(context.Background(), "not a url at all with password=hunter2secret")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "hunter2secret")
}
