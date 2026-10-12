package workspace_test

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

// Two owners, two workspaces. The second must never see or change the first.
func setupTwo(t *testing.T) (ctx context.Context, store *workspace.Store, userA, userB uuid.UUID, wsA, wsB uuid.UUID) {
	t.Helper()
	pool := testdb.Pool(t)
	svc := auth.NewService(pool, auth.Params{MemoryKiB: 8192, Time: 1, Threads: 1})
	ctx = context.Background()
	a, wsA, err := svc.Register(ctx, auth.RegisterInput{Email: fmt.Sprintf("a-%s@contoh.id", uuid.NewString()[:8]), Password: "rahasia-123", Name: "A", WorkspaceName: "Studio A"})
	require.NoError(t, err)
	b, wsB, err := svc.Register(ctx, auth.RegisterInput{Email: fmt.Sprintf("b-%s@contoh.id", uuid.NewString()[:8]), Password: "rahasia-123", Name: "B", WorkspaceName: "Studio B"})
	require.NoError(t, err)
	return ctx, workspace.NewStore(pool), a.ID, b.ID, wsA, wsB
}

func TestForUserListsOnlyOwnWorkspaces(t *testing.T) {
	ctx, store, userA, _, wsA, wsB := setupTwo(t)
	list, err := store.ForUser(ctx, userA)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, wsA, list[0].ID)
	assert.NotEqual(t, wsB, list[0].ID)
}

func TestGetRefusesNonMember(t *testing.T) {
	ctx, store, userA, userB, wsA, _ := setupTwo(t)
	ws, err := store.Get(ctx, wsA, userA)
	require.NoError(t, err)
	assert.Equal(t, "Studio A", ws.Name)

	_, err = store.Get(ctx, wsA, userB)
	assert.True(t, errors.Is(err, workspace.ErrNotFound), "non-member must get not-found, got %v", err)
}

func TestRenameRefusesNonMemberAndLeavesNameUnchanged(t *testing.T) {
	ctx, store, userA, userB, wsA, _ := setupTwo(t)

	_, err := store.Rename(ctx, wsA, userB, "Dibajak")
	assert.True(t, errors.Is(err, workspace.ErrNotFound))

	ws, err := store.Get(ctx, wsA, userA)
	require.NoError(t, err)
	assert.Equal(t, "Studio A", ws.Name)

	renamed, err := store.Rename(ctx, wsA, userA, "Studio A Baru")
	require.NoError(t, err)
	assert.Equal(t, "Studio A Baru", renamed.Name)
}

func TestRenameRejectsEmptyName(t *testing.T) {
	ctx, store, userA, _, wsA, _ := setupTwo(t)
	_, err := store.Rename(ctx, wsA, userA, "   ")
	assert.Error(t, err)
}
