package web_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kerjalancar/internal/db"
	"kerjalancar/internal/db/testdb"
)

// Exercises the real *pgxpool.Pool behind Pinger, not a fake.
func TestReadyzWithRealDatabase(t *testing.T) {
	pool := testdb.Pool(t)

	rec := httptest.NewRecorder()
	newRouter(pool).ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/readyz", nil))
	assert.Equal(t, http.StatusOK, rec.Code)

	// A separate pool that has been closed must make readyz report unavailable;
	// the shared test pool is left untouched for other tests.
	closed, err := db.NewPool(context.Background(), os.Getenv("TEST_DATABASE_URL"))
	require.NoError(t, err)
	closed.Close()

	rec = httptest.NewRecorder()
	newRouter(closed).ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/readyz", nil))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.JSONEq(t, `{"status":"unavailable","check":"database"}`, rec.Body.String())
}
