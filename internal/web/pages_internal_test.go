package web

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Review M6 recommendation: a deadlock or serialization failure that slips
// through is a "coba lagi", not a 500.
func TestServerErrorMapsTransactionConflictsToRetry(t *testing.T) {
	tmpl, err := LoadTemplates()
	require.NoError(t, err)
	s := &server{Deps: Deps{Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Templates: tmpl}}
	for _, code := range []string{"40P01", "40001"} {
		rec := httptest.NewRecorder()
		r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/app/tagihan/x/bayar", nil)
		s.serverError(rec, r, fmt.Errorf("invoice: insert payment: %w", &pgconn.PgError{Code: code, Message: "deadlock detected"}))
		assert.Equal(t, http.StatusConflict, rec.Code, code)
		assert.Contains(t, rec.Body.String(), "coba lagi")
	}
	rec := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/app", nil)
	s.serverError(rec, r, errors.New("boom"))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
