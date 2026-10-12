package web_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kerjalancar/internal/customer"
	"kerjalancar/internal/lead"
)

// MVP_PLAN M3: a pipeline of 200 leads must render well under 300 ms on the
// dev machine. The assertion is loose (1 s) so CI noise cannot fail it; the
// measured value is logged for PROJECT_STATUS.
func TestPipelineListWith200LeadsRendersQuickly(t *testing.T) {
	app := newApp(t)
	c := app.client(t)
	e := email(t)
	require.Equal(t, http.StatusSeeOther, c.register(e, "A", "W").Code)
	ctx := context.Background()
	var wsID, userID uuid.UUID
	require.NoError(t, app.pool.QueryRow(ctx, `SELECT m.workspace_id, m.user_id FROM memberships m JOIN users u ON u.id = m.user_id WHERE u.email = $1`, e).Scan(&wsID, &userID))
	customers := customer.NewStore(app.pool)
	leads := lead.NewStore(app.pool)
	for i := 0; i < 200; i++ {
		cu, err := customers.Create(ctx, wsID, customer.Input{Name: fmt.Sprintf("Pelanggan %03d", i)})
		require.NoError(t, err)
		_, err = leads.Create(ctx, wsID, userID, lead.CreateInput{CustomerID: cu.ID, Input: lead.Input{Title: fmt.Sprintf("Proyek %03d", i), EstimatedValue: fmt.Sprintf("%d000000", i+1)}})
		require.NoError(t, err)
	}

	c.get("/app/prospek?status=semua") // warm up
	var worst time.Duration
	for i := 0; i < 5; i++ {
		start := time.Now()
		rec := c.get("/app/prospek?status=semua&urut=nilai")
		d := time.Since(start)
		require.Equal(t, http.StatusOK, rec.Code)
		if d > worst {
			worst = d
		}
	}
	t.Logf("pipeline list of 200 leads: worst of 5 requests = %s", worst)
	assert.Less(t, worst, time.Second)
}
