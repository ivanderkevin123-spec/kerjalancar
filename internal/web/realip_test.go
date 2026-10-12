package web_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"kerjalancar/internal/web"
)

func loggedIP(t *testing.T, trustProxy bool) string {
	t.Helper()
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	h := web.NewRouter(web.Deps{Logger: logger, DB: fakePinger{}, TrustProxy: trustProxy})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/nope", nil)
	req.RemoteAddr = "10.0.0.7:4321"
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	h.ServeHTTP(httptest.NewRecorder(), req)
	return buf.String()
}

func TestForwardedForIsIgnoredWithoutTrustedProxy(t *testing.T) {
	out := loggedIP(t, false)
	assert.Contains(t, out, `"ip":"10.0.0.7"`)
	assert.NotContains(t, out, "203.0.113.9")
}

func TestForwardedForIsHonoredBehindTrustedProxy(t *testing.T) {
	out := loggedIP(t, true)
	assert.Contains(t, out, `"ip":"203.0.113.9"`)
}
