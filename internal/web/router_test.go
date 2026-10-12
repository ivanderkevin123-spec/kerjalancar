package web_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"kerjalancar/internal/web"
)

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

// blockingPinger never answers until the context is cancelled.
type blockingPinger struct{}

func (blockingPinger) Ping(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func newRouter(db web.Pinger) http.Handler {
	return web.NewRouter(web.Deps{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		DB:     db,
	})
}

func TestHealthz(t *testing.T) {
	rec := httptest.NewRecorder()
	newRouter(fakePinger{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json; charset=utf-8", rec.Header().Get("Content-Type"))
	assert.JSONEq(t, `{"status":"ok"}`, rec.Body.String())
}

func TestReadyzOK(t *testing.T) {
	rec := httptest.NewRecorder()
	newRouter(fakePinger{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"status":"ok"}`, rec.Body.String())
}

func TestReadyzReportsDatabaseFailure(t *testing.T) {
	rec := httptest.NewRecorder()
	newRouter(fakePinger{err: errors.New("connection refused")}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.JSONEq(t, `{"status":"unavailable","check":"database"}`, rec.Body.String())
}

func TestReadyzTimesOut(t *testing.T) {
	start := time.Now()
	rec := httptest.NewRecorder()
	newRouter(blockingPinger{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Less(t, time.Since(start), 3*time.Second, "readyz must give up after its 2s budget")
}

func TestUnknownRouteIs404(t *testing.T) {
	rec := httptest.NewRecorder()
	newRouter(fakePinger{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
