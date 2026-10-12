package customer_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kerjalancar/internal/auth"
	"kerjalancar/internal/customer"
	"kerjalancar/internal/db/testdb"
)

var lightArgon = auth.Params{MemoryKiB: 8192, Time: 1, Threads: 1}

// newWorkspace registers a fresh owner and returns their workspace and user ids.
func newWorkspace(t *testing.T) (ws, user uuid.UUID) {
	t.Helper()
	svc := auth.NewService(testdb.Pool(t), lightArgon)
	u, wsID, err := svc.Register(context.Background(), auth.RegisterInput{
		Email: fmt.Sprintf("c-%s@contoh.id", uuid.NewString()[:8]), Password: "rahasia-123", Name: "Pemilik", WorkspaceName: "Studio"})
	require.NoError(t, err)
	return wsID, u.ID
}

func TestValidateNormalisesAndReportsFieldErrors(t *testing.T) {
	in, errs := customer.Validate(customer.Input{Name: "  Budi  ", Phone: "0812-3456-7890", Email: " Budi@Contoh.ID ", Company: " PT Maju ", Notes: " catatan "})
	assert.Empty(t, errs)
	assert.Equal(t, "Budi", in.Name)
	assert.Equal(t, "+6281234567890", in.Phone)
	assert.Equal(t, "budi@contoh.id", in.Email)
	assert.Equal(t, "PT Maju", in.Company)
	assert.Equal(t, "catatan", in.Notes)

	_, errs = customer.Validate(customer.Input{Name: "", Phone: "12", Email: "bukan-email"})
	assert.Equal(t, "Nama wajib diisi.", errs["name"])
	assert.Contains(t, errs["phone"], "Nomor telepon tidak valid")
	assert.Equal(t, "Email tidak valid.", errs["email"])

	long := make([]byte, 151)
	for i := range long {
		long[i] = 'a'
	}
	_, errs = customer.Validate(customer.Input{Name: string(long)})
	assert.Equal(t, "Nama maksimal 150 karakter.", errs["name"])

	_, errs = customer.Validate(customer.Input{Name: "Budi", Phone: "", Email: ""})
	assert.Empty(t, errs, "phone and e-mail are optional")
}

func TestCreateGetUpdateWithinWorkspace(t *testing.T) {
	ctx := context.Background()
	store := customer.NewStore(testdb.Pool(t))
	ws, _ := newWorkspace(t)

	in := customer.Input{Name: "Budi Santoso", Phone: "+6281200000001", Email: "budi@contoh.id", Company: "PT Maju", Notes: "Tetangga Pak Agus"}
	in.Phone = "+62812" + uuid.NewString()[:7]
	in.Phone = numericPhone(t)
	c, err := store.Create(ctx, ws, in)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, c.ID)
	assert.Equal(t, ws, c.WorkspaceID)
	assert.Equal(t, "Budi Santoso", c.Name)
	assert.Equal(t, in.Phone, c.Phone)
	assert.Equal(t, "PT Maju", c.Company)

	got, err := store.Get(ctx, ws, c.ID)
	require.NoError(t, err)
	assert.Equal(t, c.ID, got.ID)
	assert.Equal(t, "budi@contoh.id", got.Email)

	nameOnly, err := store.Create(ctx, ws, customer.Input{Name: "Bu Rina"})
	require.NoError(t, err, "a customer may have only a name")
	assert.Empty(t, nameOnly.Phone)

	upd, err := store.Update(ctx, ws, c.ID, customer.Input{Name: "Budi S.", Phone: "", Email: "", Company: "", Notes: "pindah"})
	require.NoError(t, err)
	assert.Equal(t, "Budi S.", upd.Name)
	assert.Empty(t, upd.Phone, "phone can be cleared")
	assert.Equal(t, "pindah", upd.Notes)
	assert.True(t, upd.UpdatedAt.After(c.UpdatedAt) || upd.UpdatedAt.Equal(c.UpdatedAt))
}

func TestCrossWorkspaceReadAndWriteAreRefused(t *testing.T) {
	ctx := context.Background()
	store := customer.NewStore(testdb.Pool(t))
	wsA, _ := newWorkspace(t)
	wsB, _ := newWorkspace(t)
	c, err := store.Create(ctx, wsA, customer.Input{Name: "Milik A", Phone: numericPhone(t)})
	require.NoError(t, err)

	_, err = store.Get(ctx, wsB, c.ID)
	assert.True(t, errors.Is(err, customer.ErrNotFound), "got %v", err)

	_, err = store.Update(ctx, wsB, c.ID, customer.Input{Name: "Dibajak"})
	assert.True(t, errors.Is(err, customer.ErrNotFound), "got %v", err)

	again, err := store.Get(ctx, wsA, c.ID)
	require.NoError(t, err)
	assert.Equal(t, "Milik A", again.Name, "row unchanged after the foreign update attempt")

	page, err := store.List(ctx, wsB, customer.Query{})
	require.NoError(t, err)
	for _, it := range page.Items {
		assert.NotEqual(t, c.ID, it.ID, "A's customer must not appear in B's list")
	}
}

func TestDuplicatePhoneIsRefusedPerWorkspaceOnly(t *testing.T) {
	ctx := context.Background()
	store := customer.NewStore(testdb.Pool(t))
	wsA, _ := newWorkspace(t)
	wsB, _ := newWorkspace(t)
	phone := numericPhone(t)

	first, err := store.Create(ctx, wsA, customer.Input{Name: "Budi", Phone: phone})
	require.NoError(t, err)

	_, err = store.Create(ctx, wsA, customer.Input{Name: "Budi Lagi", Phone: phone})
	var dup *customer.DuplicatePhoneError
	require.True(t, errors.As(err, &dup), "got %v", err)
	assert.Equal(t, "Budi", dup.Name)
	assert.Equal(t, first.ID, dup.ID)

	_, err = store.Create(ctx, wsB, customer.Input{Name: "Budi di B", Phone: phone})
	require.NoError(t, err, "same number in another workspace is fine")

	other, err := store.Create(ctx, wsA, customer.Input{Name: "Cici", Phone: numericPhone(t)})
	require.NoError(t, err)
	_, err = store.Update(ctx, wsA, other.ID, customer.Input{Name: "Cici", Phone: phone})
	require.True(t, errors.As(err, &dup), "update to an existing number is refused too, got %v", err)
}

func TestListSearchesAndPaginates(t *testing.T) {
	ctx := context.Background()
	store := customer.NewStore(testdb.Pool(t))
	ws, _ := newWorkspace(t)
	for i := 0; i < 26; i++ {
		_, err := store.Create(ctx, ws, customer.Input{Name: fmt.Sprintf("Pelanggan %02d", i), Phone: numericPhone(t), Company: "CV Sejahtera"})
		require.NoError(t, err)
	}
	target, err := store.Create(ctx, ws, customer.Input{Name: "Dewi Lestari", Phone: "+6281399990001", Company: "Rumah Dewi"})
	require.NoError(t, err)

	p1, err := store.List(ctx, ws, customer.Query{Page: 1})
	require.NoError(t, err)
	assert.Equal(t, 27, p1.Total)
	assert.Len(t, p1.Items, 25)
	assert.Equal(t, "Dewi Lestari", p1.Items[0].Name, "newest first")
	p2, err := store.List(ctx, ws, customer.Query{Page: 2})
	require.NoError(t, err)
	assert.Len(t, p2.Items, 2)
	assert.Equal(t, 25, p2.PerPage)

	byName, err := store.List(ctx, ws, customer.Query{Q: "dewi"})
	require.NoError(t, err)
	require.Len(t, byName.Items, 1)
	assert.Equal(t, target.ID, byName.Items[0].ID)

	byPhone, err := store.List(ctx, ws, customer.Query{Q: "0813-9999"})
	require.NoError(t, err)
	require.Len(t, byPhone.Items, 1, "phone typed in national format matches the stored E.164 number")

	byCompany, err := store.List(ctx, ws, customer.Query{Q: "rumah"})
	require.NoError(t, err)
	assert.Len(t, byCompany.Items, 1)

	wild, err := store.List(ctx, ws, customer.Query{Q: "%"})
	require.NoError(t, err)
	assert.Equal(t, 0, wild.Total, "a literal % must not match everything")

	none, err := store.List(ctx, ws, customer.Query{Q: "zzz-tidak-ada"})
	require.NoError(t, err)
	assert.Equal(t, 0, none.Total)
}

func TestListClampsPageArguments(t *testing.T) {
	ctx := context.Background()
	store := customer.NewStore(testdb.Pool(t))
	ws, _ := newWorkspace(t)
	_, err := store.Create(ctx, ws, customer.Input{Name: "Satu"})
	require.NoError(t, err)
	for _, page := range []int{-5, 0, 1, 999} {
		p, err := store.List(ctx, ws, customer.Query{Page: page})
		require.NoError(t, err, "page %d", page)
		assert.Equal(t, 1, p.Total)
	}
	last, _ := store.List(ctx, ws, customer.Query{Page: 999})
	assert.Equal(t, 1, last.Page, "pages beyond the end clamp to the last page")
	assert.Len(t, last.Items, 1)
}

// numericPhone returns a unique, valid Indonesian mobile number.
func numericPhone(t *testing.T) string {
	t.Helper()
	var digits []byte
	for _, b := range []byte(uuid.NewString()) {
		if b >= '0' && b <= '9' {
			digits = append(digits, b)
		}
		if len(digits) == 8 {
			break
		}
	}
	return "+62812" + string(digits)
}

func TestAllListsAlphabeticallyWithinTheWorkspace(t *testing.T) {
	ctx := context.Background()
	store := customer.NewStore(testdb.Pool(t))
	wsA, _ := newWorkspace(t)
	wsB, _ := newWorkspace(t)
	for _, n := range []string{"Zainal", "andi", "Budi"} {
		_, err := store.Create(ctx, wsA, customer.Input{Name: n})
		require.NoError(t, err)
	}
	_, err := store.Create(ctx, wsB, customer.Input{Name: "Milik B"})
	require.NoError(t, err)

	all, err := store.All(ctx, wsA)
	require.NoError(t, err)
	names := make([]string, 0, len(all))
	for _, c := range all {
		names = append(names, c.Name)
	}
	assert.Equal(t, []string{"andi", "Budi", "Zainal"}, names, "case-insensitive name order, only this workspace")
}
