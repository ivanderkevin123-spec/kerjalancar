package web_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"kerjalancar/internal/web"
)

func TestRequestLoggerOmitsQueryString(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	h := web.NewRouter(web.Deps{Logger: logger, DB: fakePinger{}})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope?token=supersecret", nil))

	out := buf.String()
	assert.Contains(t, out, `"path":"/nope"`)
	assert.Contains(t, out, `"status":404`)
	assert.NotContains(t, out, "supersecret")
}

func TestRequestLoggerSkipsHealthEndpoints(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	h := web.NewRouter(web.Deps{Logger: logger, DB: fakePinger{}})

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/healthz", nil))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/readyz", nil))

	assert.Empty(t, buf.String(), "health probes must not flood the logs")
}
