package quotation_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kerjalancar/internal/auth"
	"kerjalancar/internal/customer"
	"kerjalancar/internal/db/testdb"
	"kerjalancar/internal/followup"
	"kerjalancar/internal/lead"
	"kerjalancar/internal/quotation"
)

var lightArgon = auth.Params{MemoryKiB: 8192, Time: 1, Threads: 1}

type fixture struct {
	ctx      context.Context
	q        *quotation.Store
	leads    *lead.Store
	fu       *followup.Store
	wsA, wsB uuid.UUID
	userA    uuid.UUID
	userB    uuid.UUID
	leadA    lead.Lead
	leadB    lead.Lead
	today    time.Time
}

func setup(t *testing.T) fixture {
	t.Helper()
	pool := testdb.Pool(t)
	ctx := context.Background()
	svc := auth.NewService(pool, lightArgon)
	customers := customer.NewStore(pool)
	leads := lead.NewStore(pool)
	leads.ScheduleHook = followup.ScheduleForLead
	leads.CloseHook = followup.CloseForLead
	reg := func(label string) (uuid.UUID, uuid.UUID, lead.Lead) {
		u, ws, err := svc.Register(ctx, auth.RegisterInput{Email: fmt.Sprintf("q-%s-%s@contoh.id", label, uuid.NewString()[:8]), Password: "rahasia-123", Name: "Pemilik " + label, WorkspaceName: "Studio " + label})
		require.NoError(t, err)
		c, err := customers.Create(ctx, ws, customer.Input{Name: "Pelanggan " + label, Phone: "+62813" + fmt.Sprintf("%08d", time.Now().UnixNano()%100_000_000)})
		require.NoError(t, err)
		l, err := leads.Create(ctx, ws, u.ID, lead.CreateInput{CustomerID: c.ID, Input: lead.Input{Title: "Proyek " + label}})
		require.NoError(t, err)
		return u.ID, ws, l
	}
	f := fixture{ctx: ctx, q: quotation.NewStore(pool, leads), leads: leads, fu: followup.NewStore(pool), today: date(2026, 10, 20)}
	f.userA, f.wsA, f.leadA = reg("a")
	f.userB, f.wsB, f.leadB = reg("b")
	return f
}

func validForm() quotation.Normalized {
	n, errs := quotation.Validate(quotation.Input{
		Items:       []quotation.ItemInput{{Description: "Kitchen set HPL", Qty: "2,5", Unit: "m", UnitPrice: "1.999"}, {Description: "Jasa pasang", Qty: "1", Unit: "ls", UnitPrice: "500.000"}},
		DiscountPct: "10", TaxPct: "11", ValidUntil: "2026-10-31", Notes: "Belum termasuk listrik",
	})
	if len(errs) > 0 {
		panic(fmt.Sprint(errs))
	}
	return n
}

func (f fixture) kinds(t *testing.T, ws, leadID uuid.UUID) []lead.Kind {
	t.Helper()
	acts, err := f.leads.Activities(f.ctx, ws, leadID, 50)
	require.NoError(t, err)
	out := make([]lead.Kind, 0, len(acts))
	for _, a := range acts {
		out = append(out, a.Kind)
	}
	return out
}

func TestCreateStoresItemsAndTotalsInOneTransaction(t *testing.T) {
	f := setup(t)
	q, err := f.q.Create(f.ctx, f.wsA, f.userA, f.leadA.ID, validForm())
	require.NoError(t, err)
	assert.Equal(t, 1, q.Seq)
	assert.Equal(t, "0001", q.Number())
	assert.Equal(t, quotation.StatusDraft, q.Status)
	assert.Equal(t, f.leadA.CustomerID, q.CustomerID)
	assert.Equal(t, "Pelanggan a", q.CustomerName)
	assert.Equal(t, "Proyek a", q.LeadTitle)
	require.Len(t, q.Items, 2)
	assert.Equal(t, "Kitchen set HPL", q.Items[0].Description)
	assert.Equal(t, int64(4998), q.Items[0].LineTotal)
	assert.Equal(t, int64(504_998), q.Subtotal)
	assert.Equal(t, int64(504_493), q.Total)
	assert.Equal(t, 1, q.Version)
	assert.Empty(t, q.ShareToken, "no link until sent")
	assert.Equal(t, []lead.Kind{lead.KindQuotationCreated, lead.KindCreated}, f.kinds(t, f.wsA, f.leadA.ID))

	again, err := f.q.Get(f.ctx, f.wsA, q.ID)
	require.NoError(t, err)
	assert.Equal(t, q.Total, again.Total)
	assert.Len(t, again.Items, 2)
	sum := int64(0)
	for _, it := range again.Items {
		sum += it.LineTotal
	}
	assert.Equal(t, again.Subtotal, sum, "stored subtotal equals the sum of stored lines")

	second, err := f.q.Create(f.ctx, f.wsA, f.userA, f.leadA.ID, validForm())
	require.NoError(t, err)
	assert.Equal(t, 2, second.Seq)
	_, err = f.q.Create(f.ctx, f.wsA, f.userA, f.leadB.ID, validForm())
	assert.ErrorIs(t, err, quotation.ErrLeadNotFound, "another workspace's lead")
	b, err := f.q.Create(f.ctx, f.wsB, f.userB, f.leadB.ID, validForm())
	require.NoError(t, err)
	assert.Equal(t, 1, b.Seq, "numbers are per workspace")
}

func TestNumbersAreUniqueUnderConcurrentCreation(t *testing.T) {
	f := setup(t)
	const n = 10
	seqs := make(chan int, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			q, err := f.q.Create(f.ctx, f.wsA, f.userA, f.leadA.ID, validForm())
			if assert.NoError(t, err) {
				seqs <- q.Seq
			}
		}()
	}
	wg.Wait()
	close(seqs)
	seen := map[int]bool{}
	for s := range seqs {
		assert.False(t, seen[s], "duplicate number %d", s)
		seen[s] = true
	}
	assert.Len(t, seen, n)
}

func TestUpdateRecomputesAndRefusesLockedOrStale(t *testing.T) {
	f := setup(t)
	q, err := f.q.Create(f.ctx, f.wsA, f.userA, f.leadA.ID, validForm())
	require.NoError(t, err)
	n, errs := quotation.Validate(quotation.Input{Items: []quotation.ItemInput{{Description: "Satu", Qty: "1", UnitPrice: "100"}}})
	require.Empty(t, errs)
	upd, err := f.q.Update(f.ctx, f.wsA, f.userA, q.ID, n, q.Version)
	require.NoError(t, err)
	require.Len(t, upd.Items, 1)
	assert.Equal(t, int64(100), upd.Total)
	assert.Equal(t, 0, upd.DiscountPct)
	assert.Nil(t, upd.ValidUntil)
	assert.Equal(t, 2, upd.Version)

	_, err = f.q.Update(f.ctx, f.wsA, f.userA, q.ID, n, q.Version)
	assert.ErrorIs(t, err, quotation.ErrStale, "editing from a stale page is refused")

	_, err = f.q.Accept(f.ctx, f.wsA, f.userA, q.ID, f.today)
	require.NoError(t, err)
	_, err = f.q.Update(f.ctx, f.wsA, f.userA, q.ID, n, upd.Version)
	assert.ErrorIs(t, err, quotation.ErrLocked)
	locked, err := f.q.Get(f.ctx, f.wsA, q.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(100), locked.Total, "nothing changed")
}

func TestSendIssuesALinkAndRecordsHistory(t *testing.T) {
	f := setup(t)
	q, err := f.q.Create(f.ctx, f.wsA, f.userA, f.leadA.ID, validForm())
	require.NoError(t, err)
	sent, err := f.q.Send(f.ctx, f.wsA, f.userA, q.ID)
	require.NoError(t, err)
	assert.Equal(t, quotation.StatusSent, sent.Status)
	assert.Len(t, sent.ShareToken, 22)
	assert.True(t, sent.ShareActive())
	require.NotNil(t, sent.SentAt)
	_, err = f.q.Send(f.ctx, f.wsA, f.userA, q.ID)
	assert.ErrorIs(t, err, quotation.ErrSameStatus)
	kinds := f.kinds(t, f.wsA, f.leadA.ID)
	assert.Equal(t, lead.KindQuotationSent, kinds[0])
	acts, _ := f.leads.Activities(f.ctx, f.wsA, f.leadA.ID, 1)
	assert.Equal(t, "Penawaran #0001 dikirim (Rp504.493)", acts[0].Describe())

	// The public lookup works by token only, with the workspace name for the page.
	pub, err := f.q.GetByToken(f.ctx, sent.ShareToken, f.today)
	require.NoError(t, err)
	assert.Equal(t, q.ID, pub.ID)
	assert.Equal(t, "Studio a", pub.WorkspaceName)
	assert.Len(t, pub.Items, 2)
	require.NoError(t, f.q.RecordOpen(f.ctx, pub.WorkspaceID, pub.ID))
	require.NoError(t, f.q.RecordOpen(f.ctx, pub.WorkspaceID, pub.ID))
	opened, _ := f.q.Get(f.ctx, f.wsA, q.ID)
	assert.Equal(t, 2, opened.OpenCount)
	require.NotNil(t, opened.FirstOpenedAt)

	_, err = f.q.GetByToken(f.ctx, "tidak-ada-token-sama-sekali", f.today)
	assert.ErrorIs(t, err, quotation.ErrNotFound)
	_, err = f.q.GetByToken(f.ctx, sent.ShareToken, date(2026, 11, 1))
	assert.ErrorIs(t, err, quotation.ErrExpired, "past valid_until the link no longer shows the offer")

	revoked, err := f.q.RevokeShare(f.ctx, f.wsA, q.ID)
	require.NoError(t, err)
	assert.False(t, revoked.ShareActive())
	_, err = f.q.GetByToken(f.ctx, sent.ShareToken, f.today)
	assert.ErrorIs(t, err, quotation.ErrNotFound, "a revoked link is indistinguishable from an unknown one")
	renewed, err := f.q.RenewShare(f.ctx, f.wsA, q.ID)
	require.NoError(t, err)
	assert.NotEqual(t, sent.ShareToken, renewed.ShareToken)
	assert.True(t, renewed.ShareActive())
	_, err = f.q.GetByToken(f.ctx, renewed.ShareToken, f.today)
	require.NoError(t, err)
	_, err = f.q.RenewShare(f.ctx, f.wsA, uuid.New())
	assert.ErrorIs(t, err, quotation.ErrNotFound)
}

func TestAcceptWinsTheLeadAndClosesItsFollowUp(t *testing.T) {
	f := setup(t)
	_, err := f.fu.Schedule(f.ctx, f.wsA, f.userA, f.leadA.ID, followup.Normalized{DueOn: date(2026, 10, 25)})
	require.NoError(t, err)
	q, err := f.q.Create(f.ctx, f.wsA, f.userA, f.leadA.ID, validForm())
	require.NoError(t, err)
	_, err = f.q.Send(f.ctx, f.wsA, f.userA, q.ID)
	require.NoError(t, err)

	acc, err := f.q.Accept(f.ctx, f.wsA, f.userA, q.ID, f.today)
	require.NoError(t, err)
	assert.Equal(t, quotation.StatusAccepted, acc.Status)
	require.NotNil(t, acc.AcceptedAt)
	l, err := f.leads.Get(f.ctx, f.wsA, f.leadA.ID)
	require.NoError(t, err)
	assert.Equal(t, lead.StatusMenang, l.Status)
	_, open, err := f.fu.OpenForLead(f.ctx, f.wsA, f.leadA.ID)
	require.NoError(t, err)
	assert.False(t, open, "winning closes the follow-up through the lead rules")
	kinds := f.kinds(t, f.wsA, f.leadA.ID)
	assert.Equal(t, []lead.Kind{lead.KindFollowUpDone, lead.KindStatusChanged, lead.KindQuotationAccepted, lead.KindQuotationSent, lead.KindQuotationCreated, lead.KindFollowUpScheduled, lead.KindCreated}, kinds)

	_, err = f.q.Accept(f.ctx, f.wsA, f.userA, q.ID, f.today)
	assert.ErrorIs(t, err, quotation.ErrSameStatus)
	_, err = f.q.Reject(f.ctx, f.wsA, f.userA, q.ID)
	assert.ErrorIs(t, err, quotation.ErrLocked, "an accepted quotation cannot be rejected")

	// A second quotation on the now-won lead can still be accepted (lead stays menang, no duplicate activity).
	q2, err := f.q.Create(f.ctx, f.wsA, f.userA, f.leadA.ID, validForm())
	require.NoError(t, err)
	_, err = f.q.Accept(f.ctx, f.wsA, f.userA, q2.ID, f.today)
	require.NoError(t, err)
	assert.Equal(t, lead.KindQuotationAccepted, f.kinds(t, f.wsA, f.leadA.ID)[0])
}

func TestAcceptOnALostLeadAndRejectFlow(t *testing.T) {
	f := setup(t)
	q, err := f.q.Create(f.ctx, f.wsA, f.userA, f.leadA.ID, validForm())
	require.NoError(t, err)
	_, err = f.leads.ChangeStatus(f.ctx, f.wsA, f.userA, f.leadA.ID, lead.StatusKalah, "", f.today)
	require.NoError(t, err)
	_, err = f.q.Accept(f.ctx, f.wsA, f.userA, q.ID, f.today)
	assert.ErrorIs(t, err, quotation.ErrLeadClosed, "reopen the lead first")
	still, _ := f.q.Get(f.ctx, f.wsA, q.ID)
	assert.Equal(t, quotation.StatusDraft, still.Status)

	rej, err := f.q.Reject(f.ctx, f.wsA, f.userA, q.ID)
	require.NoError(t, err)
	assert.Equal(t, quotation.StatusRejected, rej.Status)
	require.NotNil(t, rej.RejectedAt)
	_, err = f.q.Reject(f.ctx, f.wsA, f.userA, q.ID)
	assert.ErrorIs(t, err, quotation.ErrSameStatus)
	assert.Equal(t, lead.KindQuotationRejected, f.kinds(t, f.wsA, f.leadA.ID)[0])
	_, err = f.q.Send(f.ctx, f.wsA, f.userA, q.ID)
	assert.ErrorIs(t, err, quotation.ErrLocked)
}

func TestListFiltersAndForLead(t *testing.T) {
	f := setup(t)
	mk := func() quotation.Quotation {
		q, err := f.q.Create(f.ctx, f.wsA, f.userA, f.leadA.ID, validForm())
		require.NoError(t, err)
		return q
	}
	draft := mk()
	sent := mk()
	_, err := f.q.Send(f.ctx, f.wsA, f.userA, sent.ID)
	require.NoError(t, err)
	expired := mk()
	_, err = f.q.Send(f.ctx, f.wsA, f.userA, expired.ID)
	require.NoError(t, err)
	acc := mk()
	_, err = f.q.Accept(f.ctx, f.wsA, f.userA, acc.ID, f.today)
	require.NoError(t, err)

	all, err := f.q.List(f.ctx, f.wsA, quotation.ListQuery{Status: "semua"}, f.today)
	require.NoError(t, err)
	assert.Equal(t, 4, all.Total)
	assert.Equal(t, acc.ID, all.Items[0].ID, "newest first")

	onlyDraft, err := f.q.List(f.ctx, f.wsA, quotation.ListQuery{Status: "draft"}, f.today)
	require.NoError(t, err)
	require.Len(t, onlyDraft.Items, 1)
	assert.Equal(t, draft.ID, onlyDraft.Items[0].ID)

	// valid_until 2026-10-31: with "today" past it, sent quotations are expired.
	later := date(2026, 11, 2)
	exp, err := f.q.List(f.ctx, f.wsA, quotation.ListQuery{Status: "expired"}, later)
	require.NoError(t, err)
	assert.Len(t, exp.Items, 2)
	for _, it := range exp.Items {
		assert.Equal(t, quotation.StatusExpired, it.EffectiveStatus(later))
	}
	sentNow, err := f.q.List(f.ctx, f.wsA, quotation.ListQuery{Status: "sent"}, later)
	require.NoError(t, err)
	assert.Empty(t, sentNow.Items, "expired ones leave the sent filter")

	byNumber, err := f.q.List(f.ctx, f.wsA, quotation.ListQuery{Status: "semua", Q: "0003"}, f.today)
	require.NoError(t, err)
	require.Len(t, byNumber.Items, 1)
	assert.Equal(t, expired.ID, byNumber.Items[0].ID)
	byCustomer, err := f.q.List(f.ctx, f.wsA, quotation.ListQuery{Status: "semua", Q: "pelanggan a"}, f.today)
	require.NoError(t, err)
	assert.Equal(t, 4, byCustomer.Total)

	forLead, err := f.q.ForLead(f.ctx, f.wsA, f.leadA.ID)
	require.NoError(t, err)
	assert.Len(t, forLead, 4)
	assert.Empty(t, forLead[0].Items, "lists carry no items")

	latest, ok, err := f.q.LatestShared(f.ctx, f.wsA, f.leadA.ID)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, expired.ID, latest.ID, "the most recently sent quotation with an active link")

	none, err := f.q.List(f.ctx, f.wsB, quotation.ListQuery{Status: "semua"}, f.today)
	require.NoError(t, err)
	assert.Equal(t, 0, none.Total)
}

func TestCrossWorkspaceAccessIsRefusedAndWritesNothing(t *testing.T) {
	f := setup(t)
	q, err := f.q.Create(f.ctx, f.wsA, f.userA, f.leadA.ID, validForm())
	require.NoError(t, err)
	n, _ := quotation.Validate(quotation.Input{Items: []quotation.ItemInput{{Description: "X", Qty: "1", UnitPrice: "1"}}})

	_, err = f.q.Get(f.ctx, f.wsB, q.ID)
	assert.ErrorIs(t, err, quotation.ErrNotFound)
	_, err = f.q.Update(f.ctx, f.wsB, f.userB, q.ID, n, q.Version)
	assert.ErrorIs(t, err, quotation.ErrNotFound)
	_, err = f.q.Send(f.ctx, f.wsB, f.userB, q.ID)
	assert.ErrorIs(t, err, quotation.ErrNotFound)
	_, err = f.q.Accept(f.ctx, f.wsB, f.userB, q.ID, f.today)
	assert.ErrorIs(t, err, quotation.ErrNotFound)
	_, err = f.q.Reject(f.ctx, f.wsB, f.userB, q.ID)
	assert.ErrorIs(t, err, quotation.ErrNotFound)
	_, err = f.q.RevokeShare(f.ctx, f.wsB, q.ID)
	assert.ErrorIs(t, err, quotation.ErrNotFound)
	_, err = f.q.RenewShare(f.ctx, f.wsB, q.ID)
	assert.ErrorIs(t, err, quotation.ErrNotFound)
	assert.Error(t, f.q.RecordOpen(f.ctx, f.wsB, q.ID))
	forLead, err := f.q.ForLead(f.ctx, f.wsB, f.leadA.ID)
	require.NoError(t, err)
	assert.Empty(t, forLead)

	again, err := f.q.Get(f.ctx, f.wsA, q.ID)
	require.NoError(t, err)
	assert.Equal(t, quotation.StatusDraft, again.Status)
	assert.Equal(t, 1, again.Version)
	assert.Len(t, again.Items, 2)
	assert.Equal(t, 0, again.OpenCount)
	assert.Len(t, f.kinds(t, f.wsA, f.leadA.ID), 2)
}

var _ = errors.Is
