package lead_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kerjalancar/internal/auth"
	"kerjalancar/internal/customer"
	"kerjalancar/internal/db/testdb"
	"kerjalancar/internal/lead"
)

var (
	lightArgon = auth.Params{MemoryKiB: 8192, Time: 1, Threads: 1}
	jakarta    = time.FixedZone("WIB", 7*3600)
)

type fixture struct {
	ctx       context.Context
	leads     *lead.Store
	customers *customer.Store
	wsA, wsB  uuid.UUID
	userA     uuid.UUID
	userB     uuid.UUID
	custA     customer.Customer
	custB     customer.Customer
}

func setup(t *testing.T) fixture {
	t.Helper()
	pool := testdb.Pool(t)
	svc := auth.NewService(pool, lightArgon)
	ctx := context.Background()
	reg := func(label string) (uuid.UUID, uuid.UUID) {
		u, ws, err := svc.Register(ctx, auth.RegisterInput{Email: fmt.Sprintf("%s-%s@contoh.id", label, uuid.NewString()[:8]), Password: "rahasia-123", Name: "Pemilik " + label, WorkspaceName: "Studio " + label})
		require.NoError(t, err)
		return u.ID, ws
	}
	f := fixture{ctx: ctx, leads: lead.NewStore(pool), customers: customer.NewStore(pool)}
	f.userA, f.wsA = reg("a")
	f.userB, f.wsB = reg("b")
	var err error
	f.custA, err = f.customers.Create(ctx, f.wsA, customer.Input{Name: "Budi (A)"})
	require.NoError(t, err)
	f.custB, err = f.customers.Create(ctx, f.wsB, customer.Input{Name: "Citra (B)"})
	require.NoError(t, err)
	return f
}

func (f fixture) newLead(t *testing.T, title string) lead.Lead {
	t.Helper()
	l, err := f.leads.Create(f.ctx, f.wsA, f.userA, lead.CreateInput{CustomerID: f.custA.ID, Input: lead.Input{Title: title}})
	require.NoError(t, err)
	return l
}

func (f fixture) countRows(t *testing.T, table string, ws uuid.UUID) int {
	t.Helper()
	var n int
	require.NoError(t, testdb.Pool(t).QueryRow(f.ctx, "SELECT count(*) FROM "+table+" WHERE workspace_id = $1", ws).Scan(&n))
	return n
}

func TestCreateWithExistingCustomerWritesLeadAndCreatedActivity(t *testing.T) {
	f := setup(t)
	l, err := f.leads.Create(f.ctx, f.wsA, f.userA, lead.CreateInput{CustomerID: f.custA.ID, Input: lead.Input{
		Title: "Renovasi dapur", ProjectType: "renovasi", EstimatedValue: "15.000.000", Source: "whatsapp", Description: "dapur 3x4"}})
	require.NoError(t, err)
	assert.Equal(t, lead.StatusBaru, l.Status)
	assert.Equal(t, "Budi (A)", l.CustomerName)
	require.NotNil(t, l.EstimatedValue)
	assert.Equal(t, int64(15_000_000), *l.EstimatedValue)
	assert.False(t, l.StatusChangedAt.IsZero())

	acts, err := f.leads.Activities(f.ctx, f.wsA, l.ID, 10)
	require.NoError(t, err)
	require.Len(t, acts, 1)
	assert.Equal(t, lead.KindCreated, acts[0].Kind)
	assert.Equal(t, "Pemilik a", acts[0].ActorName)
}

func TestCreateWithInlineCustomerMakesBothRowsInOneGo(t *testing.T) {
	f := setup(t)
	phone := "+62813" + fmt.Sprintf("%08d", time.Now().UnixNano()%100_000_000)
	l, err := f.leads.Create(f.ctx, f.wsA, f.userA, lead.CreateInput{
		NewCustomer: &customer.Input{Name: "Bu Rina", Phone: phone},
		Input:       lead.Input{Title: "Kamar mandi"}})
	require.NoError(t, err)
	assert.Equal(t, "Bu Rina", l.CustomerName)
	assert.Equal(t, phone, l.CustomerPhone)
	c, err := f.customers.Get(f.ctx, f.wsA, l.CustomerID)
	require.NoError(t, err)
	assert.Equal(t, "Bu Rina", c.Name)

	// An invalid inline customer leaves nothing behind.
	before := f.countRows(t, "leads", f.wsA)
	_, err = f.leads.Create(f.ctx, f.wsA, f.userA, lead.CreateInput{NewCustomer: &customer.Input{Name: "", Phone: "12"}, Input: lead.Input{Title: "Teras"}})
	var verr *customer.ValidationError
	assert.True(t, errors.As(err, &verr), "got %v", err)
	assert.Equal(t, before, f.countRows(t, "leads", f.wsA))

	// A duplicate phone surfaces the existing customer.
	_, err = f.leads.Create(f.ctx, f.wsA, f.userA, lead.CreateInput{NewCustomer: &customer.Input{Name: "Rina lagi", Phone: phone}, Input: lead.Input{Title: "Teras"}})
	var dup *customer.DuplicatePhoneError
	require.True(t, errors.As(err, &dup), "got %v", err)
	assert.Equal(t, "Bu Rina", dup.Name)
}

func TestCreateRefusesAnotherWorkspacesCustomerWithoutPartialWrites(t *testing.T) {
	f := setup(t)
	leadsBefore, actsBefore := f.countRows(t, "leads", f.wsA), f.countRows(t, "lead_activities", f.wsA)
	_, err := f.leads.Create(f.ctx, f.wsA, f.userA, lead.CreateInput{CustomerID: f.custB.ID, Input: lead.Input{Title: "Curang"}})
	assert.ErrorIs(t, err, lead.ErrCustomerNotFound)
	assert.Equal(t, leadsBefore, f.countRows(t, "leads", f.wsA))
	assert.Equal(t, actsBefore, f.countRows(t, "lead_activities", f.wsA))

	_, err = f.leads.Create(f.ctx, f.wsA, f.userA, lead.CreateInput{CustomerID: f.custA.ID, Input: lead.Input{Title: ""}})
	var verr *lead.ValidationError
	assert.True(t, errors.As(err, &verr), "got %v", err)
	assert.Equal(t, leadsBefore, f.countRows(t, "leads", f.wsA))
}

func TestChangeStatusRecordsHistoryAndAppliesRules(t *testing.T) {
	f := setup(t)
	l := f.newLead(t, "Pagar")

	moved, err := f.leads.ChangeStatus(f.ctx, f.wsA, f.userA, l.ID, lead.StatusPenawaran, "", jakarta)
	require.NoError(t, err)
	assert.Equal(t, lead.StatusPenawaran, moved.Status)
	assert.True(t, moved.StatusChangedAt.After(l.StatusChangedAt))
	require.NotNil(t, moved.NextFollowUpOn, "penawaran pre-sets a follow-up date")
	now := time.Now().In(jakarta)
	assert.Equal(t, time.Date(now.Year(), now.Month(), now.Day()+3, 0, 0, 0, 0, time.UTC), *moved.NextFollowUpOn)

	_, err = f.leads.ChangeStatus(f.ctx, f.wsA, f.userA, l.ID, lead.StatusPenawaran, "", jakarta)
	assert.ErrorIs(t, err, lead.ErrSameStatus)

	lost, err := f.leads.ChangeStatus(f.ctx, f.wsA, f.userA, l.ID, lead.StatusKalah, "  harga terlalu tinggi ", jakarta)
	require.NoError(t, err)
	assert.Equal(t, "harga terlalu tinggi", lost.LostReason)
	assert.Nil(t, lost.NextFollowUpOn, "closing clears the follow-up date")

	_, err = f.leads.ChangeStatus(f.ctx, f.wsA, f.userA, l.ID, lead.StatusMenang, "", jakarta)
	assert.ErrorIs(t, err, lead.ErrClosedToClosed)

	reopened, err := f.leads.ChangeStatus(f.ctx, f.wsA, f.userA, l.ID, lead.StatusDihubungi, "", jakarta)
	require.NoError(t, err)
	assert.Empty(t, reopened.LostReason, "reason cleared on reopen")

	_, err = f.leads.ChangeStatus(f.ctx, f.wsA, f.userA, l.ID, lead.Status("negosiasi"), "", jakarta)
	assert.ErrorIs(t, err, lead.ErrInvalidStatus)

	acts, err := f.leads.Activities(f.ctx, f.wsA, l.ID, 10)
	require.NoError(t, err)
	require.Len(t, acts, 4, "created + 3 successful changes; refused changes write nothing")
	assert.Equal(t, lead.KindStatusChanged, acts[0].Kind)
	assert.Equal(t, lead.StatusKalah, acts[0].Data.From)
	assert.Equal(t, lead.StatusDihubungi, acts[0].Data.To)
	assert.Equal(t, "harga terlalu tinggi", acts[1].Data.Reason)
	assert.Equal(t, lead.KindCreated, acts[3].Kind)
}

func TestAddNoteAndUpdateAppendActivities(t *testing.T) {
	f := setup(t)
	l := f.newLead(t, "Plafon")

	note, err := f.leads.AddNote(f.ctx, f.wsA, f.userA, l.ID, "  Telepon lagi Senin ")
	require.NoError(t, err)
	assert.Equal(t, "Telepon lagi Senin", note.Body)
	_, err = f.leads.AddNote(f.ctx, f.wsA, f.userA, l.ID, "   ")
	var verr *lead.ValidationError
	assert.True(t, errors.As(err, &verr), "empty note refused, got %v", err)

	upd, err := f.leads.Update(f.ctx, f.wsA, f.userA, l.ID, lead.Input{Title: "Plafon", EstimatedValue: "12.000.000", Source: "instagram", Description: "gypsum"})
	require.NoError(t, err)
	assert.Equal(t, lead.Source("instagram"), upd.Source)

	acts, err := f.leads.Activities(f.ctx, f.wsA, l.ID, 10)
	require.NoError(t, err)
	require.Len(t, acts, 3)
	assert.Equal(t, lead.KindUpdated, acts[0].Kind)
	fields := map[string]lead.Change{}
	for _, c := range acts[0].Data.Changes {
		fields[c.Field] = c
	}
	assert.Equal(t, "Rp12.000.000", fields["estimated_value"].To)
	assert.Equal(t, "Instagram", fields["source"].To)
	_, hasDesc := fields["description"]
	assert.True(t, hasDesc)
	assert.Empty(t, fields["description"].To, "long text never lands in the history")
	_, hasTitle := fields["title"]
	assert.False(t, hasTitle, "unchanged fields are not listed")

	same, err := f.leads.Update(f.ctx, f.wsA, f.userA, l.ID, lead.Input{Title: "Plafon", EstimatedValue: "12.000.000", Source: "instagram", Description: "gypsum"})
	require.NoError(t, err)
	assert.Equal(t, upd.ID, same.ID)
	acts, _ = f.leads.Activities(f.ctx, f.wsA, l.ID, 10)
	assert.Len(t, acts, 3, "a no-op update writes no activity")
}

func TestActivitiesAreAppendOnlyInTheDatabase(t *testing.T) {
	f := setup(t)
	l := f.newLead(t, "Gudang")
	acts, err := f.leads.Activities(f.ctx, f.wsA, l.ID, 1)
	require.NoError(t, err)
	_, err = testdb.Pool(t).Exec(f.ctx, `UPDATE lead_activities SET body = 'diubah' WHERE id = $1`, acts[0].ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "append-only")
}

func TestCrossWorkspaceAccessIsNotFoundAndWritesNothing(t *testing.T) {
	f := setup(t)
	l := f.newLead(t, "Milik A")

	_, err := f.leads.Get(f.ctx, f.wsB, l.ID)
	assert.ErrorIs(t, err, lead.ErrNotFound)
	_, err = f.leads.Update(f.ctx, f.wsB, f.userB, l.ID, lead.Input{Title: "Dibajak"})
	assert.ErrorIs(t, err, lead.ErrNotFound)
	_, err = f.leads.ChangeStatus(f.ctx, f.wsB, f.userB, l.ID, lead.StatusKalah, "", jakarta)
	assert.ErrorIs(t, err, lead.ErrNotFound)
	_, err = f.leads.AddNote(f.ctx, f.wsB, f.userB, l.ID, "hai")
	assert.ErrorIs(t, err, lead.ErrNotFound)
	_, err = f.leads.Activities(f.ctx, f.wsB, l.ID, 10)
	assert.ErrorIs(t, err, lead.ErrNotFound)

	again, err := f.leads.Get(f.ctx, f.wsA, l.ID)
	require.NoError(t, err)
	assert.Equal(t, "Milik A", again.Title)
	assert.Equal(t, lead.StatusBaru, again.Status)
	acts, _ := f.leads.Activities(f.ctx, f.wsA, l.ID, 10)
	assert.Len(t, acts, 1)

	page, err := f.leads.List(f.ctx, f.wsB, lead.ListQuery{Status: "semua"})
	require.NoError(t, err)
	for _, it := range page.Items {
		assert.NotEqual(t, l.ID, it.ID)
	}
	forCust, err := f.leads.ForCustomer(f.ctx, f.wsB, f.custA.ID)
	require.NoError(t, err)
	assert.Empty(t, forCust)
}

func TestListFiltersSearchesSortsAndPaginates(t *testing.T) {
	f := setup(t)
	mk := func(title, value string) lead.Lead {
		l, err := f.leads.Create(f.ctx, f.wsA, f.userA, lead.CreateInput{CustomerID: f.custA.ID, Input: lead.Input{Title: title, EstimatedValue: value}})
		require.NoError(t, err)
		return l
	}
	cheap := mk("Cat ulang", "2.000.000")
	expensive := mk("Rumah dua lantai", "900.000.000")
	none := mk("Konsultasi", "")
	won := mk("Kitchen set", "30.000.000")
	_, err := f.leads.ChangeStatus(f.ctx, f.wsA, f.userA, won.ID, lead.StatusMenang, "", jakarta)
	require.NoError(t, err)
	_, err = f.leads.ChangeStatus(f.ctx, f.wsA, f.userA, none.ID, lead.StatusPenawaran, "", jakarta)
	require.NoError(t, err)

	active, err := f.leads.List(f.ctx, f.wsA, lead.ListQuery{})
	require.NoError(t, err)
	assert.Equal(t, 3, active.Total, "default shows open leads only")
	for _, it := range active.Items {
		assert.NotEqual(t, won.ID, it.ID)
	}

	all, err := f.leads.List(f.ctx, f.wsA, lead.ListQuery{Status: "semua"})
	require.NoError(t, err)
	assert.Equal(t, 4, all.Total)

	onlyWon, err := f.leads.List(f.ctx, f.wsA, lead.ListQuery{Status: "menang"})
	require.NoError(t, err)
	require.Len(t, onlyWon.Items, 1)
	assert.Equal(t, won.ID, onlyWon.Items[0].ID)

	byValue, err := f.leads.List(f.ctx, f.wsA, lead.ListQuery{Status: "semua", Sort: "nilai"})
	require.NoError(t, err)
	require.Len(t, byValue.Items, 4)
	assert.Equal(t, expensive.ID, byValue.Items[0].ID)
	assert.Equal(t, none.ID, byValue.Items[3].ID, "leads without an estimate sort last")

	byFollowUp, err := f.leads.List(f.ctx, f.wsA, lead.ListQuery{Status: "semua", Sort: "followup"})
	require.NoError(t, err)
	assert.Equal(t, none.ID, byFollowUp.Items[0].ID, "the only lead with a follow-up date comes first")

	byTitle, err := f.leads.List(f.ctx, f.wsA, lead.ListQuery{Status: "semua", Q: "lantai"})
	require.NoError(t, err)
	require.Len(t, byTitle.Items, 1)
	assert.Equal(t, expensive.ID, byTitle.Items[0].ID)

	byCustomer, err := f.leads.List(f.ctx, f.wsA, lead.ListQuery{Status: "semua", Q: "budi"})
	require.NoError(t, err)
	assert.Equal(t, 4, byCustomer.Total, "search also matches the customer name")

	wild, err := f.leads.List(f.ctx, f.wsA, lead.ListQuery{Status: "semua", Q: "%"})
	require.NoError(t, err)
	assert.Equal(t, 0, wild.Total)

	for i := 0; i < 25; i++ {
		mk(fmt.Sprintf("Proyek %02d", i), "")
	}
	p1, err := f.leads.List(f.ctx, f.wsA, lead.ListQuery{Status: "semua", Page: 1})
	require.NoError(t, err)
	assert.Equal(t, 29, p1.Total)
	assert.Len(t, p1.Items, 25)
	assert.Equal(t, 2, p1.Pages)
	p9, err := f.leads.List(f.ctx, f.wsA, lead.ListQuery{Status: "semua", Page: 9})
	require.NoError(t, err)
	assert.Equal(t, 2, p9.Page, "beyond the end clamps to the last page")
	assert.Len(t, p9.Items, 4)
	_ = cheap

	forCust, err := f.leads.ForCustomer(f.ctx, f.wsA, f.custA.ID)
	require.NoError(t, err)
	assert.Len(t, forCust, 29)

	sum, err := f.leads.Summary(f.ctx, f.wsA)
	require.NoError(t, err)
	assert.Equal(t, 28, sum.OpenCount)
	assert.Equal(t, 1, sum.ByStatus[lead.StatusMenang])
	assert.Equal(t, int64(902_000_000), sum.OpenValue, "won leads are not pipeline value")
}
