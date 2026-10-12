package template_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kerjalancar/internal/auth"
	"kerjalancar/internal/db/testdb"
	"kerjalancar/internal/template"
)

var lightArgon = auth.Params{MemoryKiB: 8192, Time: 1, Threads: 1}

func newWorkspace(t *testing.T) uuid.UUID {
	t.Helper()
	_, ws, err := auth.NewService(testdb.Pool(t), lightArgon).Register(context.Background(), auth.RegisterInput{
		Email: fmt.Sprintf("tpl-%s@contoh.id", uuid.NewString()[:8]), Password: "rahasia-123", Name: "Pemilik", WorkspaceName: "Studio"})
	require.NoError(t, err)
	return ws
}

func TestRegistrationSeedsTheThreeTemplates(t *testing.T) {
	ctx := context.Background()
	store := template.NewStore(testdb.Pool(t))
	ws := newWorkspace(t)
	list, err := store.ForWorkspace(ctx, ws)
	require.NoError(t, err)
	require.Len(t, list, 3)
	for i, tpl := range list {
		assert.Equal(t, template.Seeds[i].Key, tpl.Key)
		assert.Equal(t, template.Seeds[i].Name, tpl.Name)
		assert.Equal(t, template.Seeds[i].Body, tpl.Body)
		assert.Equal(t, i+1, tpl.Position)
		assert.Equal(t, ws, tpl.WorkspaceID)
	}
	one, err := store.Get(ctx, ws, template.KeyPengingatDP)
	require.NoError(t, err)
	assert.Equal(t, "Pengingat DP", one.Name)
}

func TestUpdateValidatesAndStaysInsideTheWorkspace(t *testing.T) {
	ctx := context.Background()
	store := template.NewStore(testdb.Pool(t))
	wsA, wsB := newWorkspace(t), newWorkspace(t)

	upd, err := store.Update(ctx, wsA, template.KeyFollowUpPenawaran, "  Tindak lanjut ", " Halo {nama}, penawaran {nilai} ya. ")
	require.NoError(t, err)
	assert.Equal(t, "Tindak lanjut", upd.Name)
	assert.Equal(t, "Halo {nama}, penawaran {nilai} ya.", upd.Body)

	_, err = store.Update(ctx, wsA, template.KeyFollowUpPenawaran, "", strings.Repeat("x", 1001))
	var verr *template.ValidationError
	require.True(t, errors.As(err, &verr), "got %v", err)
	assert.Equal(t, "Nama templat wajib diisi.", verr.Fields["name"])
	assert.Equal(t, "Isi pesan maksimal 1000 karakter.", verr.Fields["body"])

	_, err = store.Update(ctx, wsA, template.Key("custom"), "x", "y")
	assert.ErrorIs(t, err, template.ErrNotFound, "unknown slots do not exist")

	_, err = store.Get(ctx, wsB, template.Key("custom"))
	assert.ErrorIs(t, err, template.ErrNotFound)

	b, err := store.Get(ctx, wsB, template.KeyFollowUpPenawaran)
	require.NoError(t, err)
	assert.Equal(t, "Follow-up penawaran", b.Name, "B's slot is untouched by A's edit")
	a, err := store.Get(ctx, wsA, template.KeyFollowUpPenawaran)
	require.NoError(t, err)
	assert.Equal(t, "Tindak lanjut", a.Name)
}
