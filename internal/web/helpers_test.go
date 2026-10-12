package web_test

import (
	"bytes"
	"log/slog"
	"testing"

	"kerjalancar/internal/web"
)

// newLoggingApp captures the server's JSON log for assertions.
func newLoggingApp(t *testing.T) (*testApp, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	app := newApp(t, func(d *web.Deps) {
		d.Logger = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	})
	return app, &buf
}
