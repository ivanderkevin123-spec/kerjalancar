package auth_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kerjalancar/internal/auth"
	"kerjalancar/internal/db/testdb"
	"kerjalancar/internal/workspace"
)

func uniqueEmail(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("owner-%s@contoh.id", uuid.NewString()[:8])
}

func TestRegisterCreatesUserWorkspaceAndOwnerMembership(t *testing.T) {
	pool := testdb.Pool(t)
	svc := auth.NewService(pool, auth.Params{MemoryKiB: 8192, Time: 1, Threads: 1})
	ctx := context.Background()
	email := uniqueEmail(t)

	user, wsID, err := svc.Register(ctx, auth.RegisterInput{
		Email: "  " + email + "  ", Password: "rahasia-123", Name: "Budi", WorkspaceName: "Renovasi Budi",
	})
	require.NoError(t, err)
	assert.Equal(t, email, user.Email, "e-mail trimmed and lower-cased")
	assert.Equal(t, "Budi", user.Name)
	assert.NotEqual(t, uuid.Nil, user.ID)

	ws, err := workspace.NewStore(pool).Get(ctx, wsID, user.ID)
	require.NoError(t, err)
	assert.Equal(t, "Renovasi Budi", ws.Name)
	assert.Equal(t, "Asia/Jakarta", ws.Timezone)

	var role string
	require.NoError(t, pool.QueryRow(ctx, `SELECT role FROM memberships WHERE workspace_id=$1 AND user_id=$2`, wsID, user.ID).Scan(&role))
	assert.Equal(t, "owner", role)
}

func TestRegisterRejectsDuplicateEmailCaseInsensitively(t *testing.T) {
	pool := testdb.Pool(t)
	svc := auth.NewService(pool, auth.Params{MemoryKiB: 8192, Time: 1, Threads: 1})
	ctx := context.Background()
	email := uniqueEmail(t)

	_, _, err := svc.Register(ctx, auth.RegisterInput{Email: email, Password: "rahasia-123", Name: "A", WorkspaceName: "W"})
	require.NoError(t, err)

	_, _, err = svc.Register(ctx, auth.RegisterInput{Email: "OWNER" + email[5:], Password: "rahasia-123", Name: "B", WorkspaceName: "W2"})
	assert.True(t, errors.Is(err, auth.ErrEmailTaken), "got %v", err)
}

func TestRegisterIsAtomic(t *testing.T) {
	pool := testdb.Pool(t)
	svc := auth.NewService(pool, auth.Params{MemoryKiB: 8192, Time: 1, Threads: 1})
	ctx := context.Background()
	email := uniqueEmail(t)

	// Empty workspace name violates the CHECK constraint after the user row is inserted.
	_, _, err := svc.Register(ctx, auth.RegisterInput{Email: email, Password: "rahasia-123", Name: "A", WorkspaceName: ""})
	require.Error(t, err)

	var n int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE email=$1`, email).Scan(&n))
	assert.Equal(t, 0, n, "user row must be rolled back")
}

func TestAuthenticate(t *testing.T) {
	pool := testdb.Pool(t)
	svc := auth.NewService(pool, auth.Params{MemoryKiB: 8192, Time: 1, Threads: 1})
	ctx := context.Background()
	email := uniqueEmail(t)
	_, _, err := svc.Register(ctx, auth.RegisterInput{Email: email, Password: "rahasia-123", Name: "A", WorkspaceName: "W"})
	require.NoError(t, err)

	user, err := svc.Authenticate(ctx, " "+email+" ", "rahasia-123")
	require.NoError(t, err)
	assert.Equal(t, email, user.Email)

	_, err = svc.Authenticate(ctx, email, "salah")
	assert.True(t, errors.Is(err, auth.ErrInvalidCredentials))

	_, err = svc.Authenticate(ctx, "tidak-ada-"+email, "rahasia-123")
	assert.True(t, errors.Is(err, auth.ErrInvalidCredentials), "unknown e-mail yields the same error")

	got, err := svc.UserByID(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, user.Email, got.Email)
}
