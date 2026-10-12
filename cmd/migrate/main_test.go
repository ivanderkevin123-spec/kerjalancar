package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// A migration job needs DATABASE_URL and nothing else; unrelated or even
// invalid web settings must not stop it.
func TestRunNeedsOnlyDatabaseURL(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("BASE_URL", "http://not-https.example")
	t.Setenv("PORT", "abc")
	t.Setenv("DATABASE_URL", "postgres://app:pw@127.0.0.1:1/app?sslmode=disable") // closed port

	err := run([]string{"status"})
	require.Error(t, err)
	msg := err.Error()
	require.True(t, strings.Contains(msg, "database"), "expected a database connection error, got: %s", msg)
	require.NotContains(t, msg, "PORT")
	require.NotContains(t, msg, "BASE_URL")
}

func TestRunRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	err := run([]string{"up"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "DATABASE_URL")
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	err := run([]string{"down"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "usage")
}
