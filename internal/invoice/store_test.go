package invoice_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kerjalancar/internal/auth"
	"kerjalancar/internal/customer"
	"kerjalancar/internal/db/testdb"
	"kerjalancar/internal/followup"
	"kerjalancar/internal/invoice"
	"kerjalancar/internal/lead"
	"kerjalancar/internal/quotation"
)

var lightArgon = auth.Params{MemoryKiB: 8192, Time: 1, Threads: 1}

type fixture struct {
	ctx   context.Context
	pool  *pgxpool.Pool
	inv   *invoice.Store
	q     *quotation.Store
	leads *lead.Store
	today time.Time
	userA uuid.UUID
	wsA   uuid.UUID
	leadA lead.Lead
	userB uuid.UUID
	wsB   uuid.UUID
	leadB lead.Lead
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
		u, ws, err := svc.Register(ctx, auth.RegisterInput{Email: fmt.Sprintf("i-%s-%s@contoh.id", label, uuid.NewString()[:8]), Password: "rahasia-123", Name: "Pemilik " + label, WorkspaceName: "Studio " + label})
		require.NoError(t, err)
		c, err := customers.Create(ctx, ws, customer.Input{Name: "Pelanggan " + label, Phone: "+62813" + fmt.Sprintf("%08d", time.Now().UnixNano()%100_000_000)})
		require.NoError(t, err)
		l, err := leads.Create(ctx, ws, u.ID, lead.CreateInput{CustomerID: c.ID, Input: lead.Input{Title: "Proyek " + label}})
		require.NoError(t, err)
		return u.ID, ws, l
	}
	f := fixture{ctx: ctx, pool: pool, inv: invoice.NewStore(pool, leads), q: quotation.NewStore(pool, leads), leads: leads, today: date(2026, 10, 20)}
	f.userA, f.wsA, f.leadA = reg("a")
	f.userB, f.wsB, f.leadB = reg("b")
	return f
}

func draft(dueOn string) invoice.Normalized {
	n, errs := invoice.Validate(invoice.Input{Items: rows(), DiscountPct: "10", TaxPct: "11", DueOn: dueOn, Notes: "Transfer ke BCA."})
	if len(errs) > 0 {
		panic(fmt.Sprint(errs))
	}
	return n
}

func key() string {
	k, err := invoice.NewIdempotencyKey()
	if err != nil {
		panic(err)
	}
	return k
}

func payment(kind invoice.PaymentKind, amount int64, k string) invoice.NormalizedPayment {
	return invoice.NormalizedPayment{Kind: kind, Method: invoice.MethodTransfer, Amount: amount, ReceivedOn: date(2026, 10, 20), Note: "BCA", IdempotencyKey: k}
}

// issued creates and issues an invoice of Rp504.493 due on dueOn for lead A.
func (f fixture) issued(t *testing.T, dueOn string) invoice.Invoice {
	t.Helper()
	inv, err := f.inv.Create(f.ctx, f.wsA, f.userA, f.leadA.ID, nil, draft(dueOn))
	require.NoError(t, err)
	inv, err = f.inv.Issue(f.ctx, f.wsA, f.userA, inv.ID)
	require.NoError(t, err)
	return inv
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

func TestCreateFromQuotationRecordsProvenanceAndSavesWhatWasTyped(t *testing.T) {
	f := setup(t)
	qn, errs := quotation.Validate(quotation.Input{Items: rows(), DiscountPct: "10", TaxPct: "11", ValidUntil: "2026-10-31"})
	require.Empty(t, errs)
	q, err := f.q.Create(f.ctx, f.wsA, f.userA, f.leadA.ID, qn)
	require.NoError(t, err)
	q, err = f.q.Send(f.ctx, f.wsA, f.userA, q.ID)
	require.NoError(t, err)
	q, err = f.q.Accept(f.ctx, f.wsA, f.userA, q.ID, f.today)
	require.NoError(t, err)

	// The form arrives pre-filled from the quotation but the owner may have
	// edited it (a first-phase bill, say); what was typed is what is saved.
	typed := draft("2026-10-27")
	typed.Notes = "DP 50%"
	inv, err := f.inv.Create(f.ctx, f.wsA, f.userA, f.leadA.ID, &q.ID, typed)
	require.NoError(t, err)
	assert.Equal(t, 1, inv.Seq)
	assert.Equal(t, invoice.StatusDraft, inv.Status)
	require.NotNil(t, inv.QuotationID)
	assert.Equal(t, q.ID, *inv.QuotationID, "provenance recorded")
	assert.Equal(t, q.Total, inv.Total, "same rows typed → same total")
	assert.Equal(t, inv.Total, inv.Balance, "nothing paid yet")
	assert.Equal(t, "DP 50%", inv.Notes)
	require.Len(t, inv.Items, 2)
	assert.Equal(t, "Kitchen set HPL", inv.Items[0].Description)
	assert.Equal(t, "Pelanggan a", inv.CustomerName)
	assert.Equal(t, "Proyek a", inv.LeadTitle)
	assert.Equal(t, "Studio a", inv.WorkspaceName)

	edited := draft("2026-10-27")
	edited.Items = edited.Items[:1] // the owner removed the second row
	edited.DiscountPct, edited.TaxPct = 0, 0
	totals, err := quotation.ComputeTotals(edited.Items, 0, 0)
	require.NoError(t, err)
	edited.Totals = totals
	phase, err := f.inv.Create(f.ctx, f.wsA, f.userA, f.leadA.ID, &q.ID, edited)
	require.NoError(t, err)
	assert.Equal(t, int64(4998), phase.Total, "edits are honoured, not replaced by the quotation copy")
	require.Len(t, phase.Items, 1)
	assert.Equal(t, q.ID, *phase.QuotationID)

	// Editing the invoice never touches the quotation (rows are copied, not referenced).
	upd, err := f.inv.Update(f.ctx, f.wsA, f.userA, inv.ID, edited, inv.Version)
	require.NoError(t, err)
	assert.Len(t, upd.Items, 1)
	assert.Equal(t, int64(4998), upd.Total)
	again, err := f.q.Get(f.ctx, f.wsA, q.ID)
	require.NoError(t, err)
	assert.Len(t, again.Items, 2)
	assert.Equal(t, int64(504_493), again.Total)

	assert.Equal(t, []lead.Kind{lead.KindInvoiceCreated, lead.KindInvoiceCreated, lead.KindStatusChanged, lead.KindQuotationAccepted, lead.KindQuotationSent, lead.KindQuotationCreated, lead.KindCreated}, f.kinds(t, f.wsA, f.leadA.ID), "newest first; the lead went Baru → Menang when the quotation was accepted")

	// A quotation of another lead or workspace is not a valid source.
	_, err = f.inv.Create(f.ctx, f.wsA, f.userA, f.leadA.ID, ptr(uuid.New()), draft(""))
	assert.ErrorIs(t, err, invoice.ErrQuotationNotFound)
	_, err = f.inv.Create(f.ctx, f.wsB, f.userB, f.leadB.ID, &q.ID, draft(""))
	assert.ErrorIs(t, err, invoice.ErrQuotationNotFound)
}

// Review M6 #1: voiding while a payment is being recorded must not deadlock.
// The payment path holds the invoice row and then writes a history line (an
// FK insert that takes KEY SHARE on the lead); the void path holds the lead
// and waits for the invoice. The lead lock therefore must not block KEY SHARE.
func TestVoidWhilePayingDoesNotDeadlock(t *testing.T) {
	f := setup(t)
	inv := f.issued(t, "2026-10-27")

	// A payment transaction half-way: invoice row locked, history not yet written.
	tx1, err := f.pool.Begin(f.ctx)
	require.NoError(t, err)
	defer func() { _ = tx1.Rollback(f.ctx) }()
	var leadID uuid.UUID
	require.NoError(t, tx1.QueryRow(f.ctx, `SELECT lead_id FROM invoices WHERE workspace_id = $1 AND id = $2 FOR UPDATE`, f.wsA, inv.ID).Scan(&leadID))

	done := make(chan error, 1)
	go func() {
		_, err := f.inv.Void(f.ctx, f.wsA, f.userA, inv.ID)
		done <- err
	}()
	// Wait until the void transaction holds the lead and is queued on the invoice row.
	require.Eventually(t, func() bool {
		var n int
		_ = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM pg_stat_activity WHERE wait_event_type = 'Lock' AND query ILIKE '%FROM invoices WHERE%FOR UPDATE%'`).Scan(&n)
		return n > 0
	}, 5*time.Second, 20*time.Millisecond, "void should be waiting on the invoice row")

	// The payment path now appends its history line.
	ctx, cancel := context.WithTimeout(f.ctx, 5*time.Second)
	defer cancel()
	err = lead.AppendActivity(ctx, tx1, f.wsA, leadID, f.userA, lead.KindPaymentRecorded, "", lead.ActivityData{Number: inv.Number(), Amount: "Rp1", Label: "DP"})
	require.NoError(t, err, "the void transaction's lead lock must not block the payment's history insert (40P01 before the fix)")
	require.NoError(t, tx1.Commit(f.ctx))

	select {
	case err := <-done:
		require.NoError(t, err, "void proceeds once the payment transaction commits")
	case <-time.After(5 * time.Second):
		t.Fatal("void did not finish")
	}
	got, err := f.inv.Get(f.ctx, f.wsA, inv.ID)
	require.NoError(t, err)
	assert.Equal(t, invoice.StatusVoid, got.Status)
}

// Review M6 #3: a key issued for one invoice must not be honoured on another.
func TestRecordPaymentRefusesAKeyFromAnotherInvoice(t *testing.T) {
	f := setup(t)
	a := f.issued(t, "2026-10-27")
	b := f.issued(t, "2026-10-27")
	k := key()
	_, created, err := f.inv.RecordPayment(f.ctx, f.wsA, f.userA, a.ID, payment(invoice.KindDP, 200_000, k))
	require.NoError(t, err)
	assert.True(t, created)
	_, _, err = f.inv.RecordPayment(f.ctx, f.wsA, f.userA, b.ID, payment(invoice.KindDP, 200_000, k))
	assert.ErrorIs(t, err, invoice.ErrKeyMismatch)
	got, err := f.inv.Get(f.ctx, f.wsA, b.ID)
	require.NoError(t, err)
	assert.Empty(t, got.Payments, "nothing recorded on the other invoice")
	assert.Equal(t, int64(504_493), got.Balance)
}

func TestCreateBlankUpdateDraftAndLocks(t *testing.T) {
	f := setup(t)
	inv, err := f.inv.Create(f.ctx, f.wsA, f.userA, f.leadA.ID, nil, draft(""))
	require.NoError(t, err)
	assert.Nil(t, inv.QuotationID)
	assert.Nil(t, inv.DueOn)
	assert.Equal(t, int64(504_493), inv.Total)
	assert.Equal(t, 1, inv.Version)

	upd, err := f.inv.Update(f.ctx, f.wsA, f.userA, inv.ID, draft("2026-10-27"), 1)
	require.NoError(t, err)
	assert.Equal(t, 2, upd.Version)
	require.NotNil(t, upd.DueOn)
	_, err = f.inv.Update(f.ctx, f.wsA, f.userA, inv.ID, draft("2026-10-28"), 1)
	assert.ErrorIs(t, err, invoice.ErrStale, "stale version")

	issued, err := f.inv.Issue(f.ctx, f.wsA, f.userA, inv.ID)
	require.NoError(t, err)
	assert.Equal(t, invoice.StatusIssued, issued.Status)
	assert.Len(t, issued.ShareToken, 22)
	assert.NotNil(t, issued.IssuedAt)
	_, err = f.inv.Update(f.ctx, f.wsA, f.userA, inv.ID, draft("2026-10-29"), issued.Version)
	assert.ErrorIs(t, err, invoice.ErrLocked, "issued invoices do not change")
	_, err = f.inv.Issue(f.ctx, f.wsA, f.userA, inv.ID)
	assert.ErrorIs(t, err, invoice.ErrLocked, "issuing twice")

	second, err := f.inv.Create(f.ctx, f.wsA, f.userA, f.leadA.ID, nil, draft(""))
	require.NoError(t, err)
	assert.Equal(t, 2, second.Seq, "numbers run per workspace")
	other, err := f.inv.Create(f.ctx, f.wsB, f.userB, f.leadB.ID, nil, draft(""))
	require.NoError(t, err)
	assert.Equal(t, 1, other.Seq)
}

func TestIssueNeedsADueDateAndATotal(t *testing.T) {
	f := setup(t)
	inv, err := f.inv.Create(f.ctx, f.wsA, f.userA, f.leadA.ID, nil, draft(""))
	require.NoError(t, err)
	_, err = f.inv.Issue(f.ctx, f.wsA, f.userA, inv.ID)
	assert.ErrorIs(t, err, invoice.ErrNoDueDate)

	zero := draft("2026-10-27")
	zero.Items = []quotation.NormalizedItem{{Description: "Gratis", QtyMilli: 1000, UnitPrice: 0, LineTotal: 0}}
	zero.Totals = quotation.Totals{}
	empty, err := f.inv.Create(f.ctx, f.wsA, f.userA, f.leadA.ID, nil, zero)
	require.NoError(t, err)
	_, err = f.inv.Issue(f.ctx, f.wsA, f.userA, empty.ID)
	assert.ErrorIs(t, err, invoice.ErrEmpty)

	lost, err := f.leads.Create(f.ctx, f.wsA, f.userA, lead.CreateInput{CustomerID: f.leadA.CustomerID, Input: lead.Input{Title: "Kalah"}})
	require.NoError(t, err)
	_, err = f.leads.ChangeStatus(f.ctx, f.wsA, f.userA, lost.ID, lead.StatusKalah, "harga", f.today)
	require.NoError(t, err)
	_, err = f.inv.Create(f.ctx, f.wsA, f.userA, lost.ID, nil, draft(""))
	assert.ErrorIs(t, err, invoice.ErrLeadClosed, "no invoices for lost leads")
}

func TestBalanceMatchesSQLForEveryFixture(t *testing.T) {
	f := setup(t)
	paid := f.issued(t, "2026-10-27")
	partial := f.issued(t, "2026-10-27")
	untouched := f.issued(t, "2026-10-27")

	_, created, err := f.inv.RecordPayment(f.ctx, f.wsA, f.userA, paid.ID, payment(invoice.KindDP, 200_000, key()))
	require.NoError(t, err)
	assert.True(t, created)
	_, _, err = f.inv.RecordPayment(f.ctx, f.wsA, f.userA, paid.ID, payment(invoice.KindPelunasan, 304_493, key()))
	require.NoError(t, err)
	p1, _, err := f.inv.RecordPayment(f.ctx, f.wsA, f.userA, partial.ID, payment(invoice.KindDP, 200_000, key()))
	require.NoError(t, err)
	p2, _, err := f.inv.RecordPayment(f.ctx, f.wsA, f.userA, partial.ID, payment(invoice.KindCicilan, 100_000, key()))
	require.NoError(t, err)
	_, err = f.inv.ReversePayment(f.ctx, f.wsA, f.userA, p2.ID, "salah input")
	require.NoError(t, err)

	for _, id := range []uuid.UUID{paid.ID, partial.ID, untouched.ID} {
		inv, err := f.inv.Get(f.ctx, f.wsA, id)
		require.NoError(t, err)
		var sum, reversed int64
		for _, p := range inv.Payments {
			sum += p.Amount
			if p.Reversal != nil {
				reversed += p.Amount
			}
		}
		assert.Equal(t, inv.Total-sum+reversed, inv.Balance, "balance in SQL equals total − payments + reversals in Go")
		assert.Equal(t, sum-reversed, inv.Paid)
	}
	got, err := f.inv.Get(f.ctx, f.wsA, paid.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), got.Balance)
	assert.Equal(t, invoice.StatusPaid, got.Effective(f.today))
	got, err = f.inv.Get(f.ctx, f.wsA, partial.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(304_493), got.Balance)
	assert.Equal(t, p1.ID, got.Payments[0].ID)
	require.NotNil(t, got.Payments[1].Reversal)
	assert.Equal(t, "salah input", got.Payments[1].Reversal.Reason)
	assert.Equal(t, "Pemilik a", got.Payments[1].Reversal.ReversedByName)
}

func TestRecordPaymentRefusesOverpaymentAndDrafts(t *testing.T) {
	f := setup(t)
	inv := f.issued(t, "2026-10-27")
	_, _, err := f.inv.RecordPayment(f.ctx, f.wsA, f.userA, inv.ID, payment(invoice.KindDP, 504_494, key()))
	var over *invoice.OverpaymentError
	require.ErrorAs(t, err, &over)
	assert.Equal(t, int64(504_493), over.Balance)
	assert.ErrorIs(t, err, invoice.ErrOverpayment)

	_, created, err := f.inv.RecordPayment(f.ctx, f.wsA, f.userA, inv.ID, payment(invoice.KindPelunasan, 504_493, key()))
	require.NoError(t, err)
	assert.True(t, created)
	_, _, err = f.inv.RecordPayment(f.ctx, f.wsA, f.userA, inv.ID, payment(invoice.KindCicilan, 1, key()))
	require.ErrorAs(t, err, &over, "a paid invoice takes nothing more")
	assert.Equal(t, int64(0), over.Balance)

	d, err := f.inv.Create(f.ctx, f.wsA, f.userA, f.leadA.ID, nil, draft("2026-10-27"))
	require.NoError(t, err)
	_, _, err = f.inv.RecordPayment(f.ctx, f.wsA, f.userA, d.ID, payment(invoice.KindDP, 1, key()))
	assert.ErrorIs(t, err, invoice.ErrNotIssued)
}

func TestRecordPaymentIsIdempotentOnTheKey(t *testing.T) {
	f := setup(t)
	inv := f.issued(t, "2026-10-27")
	k := key()
	first, created, err := f.inv.RecordPayment(f.ctx, f.wsA, f.userA, inv.ID, payment(invoice.KindDP, 200_000, k))
	require.NoError(t, err)
	assert.True(t, created)
	second, created, err := f.inv.RecordPayment(f.ctx, f.wsA, f.userA, inv.ID, payment(invoice.KindDP, 200_000, k))
	require.NoError(t, err)
	assert.False(t, created, "the same submission again is not a second payment")
	assert.Equal(t, first.ID, second.ID)
	got, err := f.inv.Get(f.ctx, f.wsA, inv.ID)
	require.NoError(t, err)
	assert.Len(t, got.Payments, 1)
	assert.Equal(t, int64(304_493), got.Balance)
	assert.Equal(t, []lead.Kind{lead.KindPaymentRecorded, lead.KindInvoiceIssued, lead.KindInvoiceCreated, lead.KindCreated}, f.kinds(t, f.wsA, f.leadA.ID), "one history line")
}

func TestConcurrentPaymentsSerializeOnTheInvoice(t *testing.T) {
	f := setup(t)
	run := func(amount int64) (okCount, overCount int) {
		inv := f.issued(t, "2026-10-27")
		var wg sync.WaitGroup
		start := make(chan struct{})
		results := make(chan error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, _, err := f.inv.RecordPayment(f.ctx, f.wsA, f.userA, inv.ID, payment(invoice.KindCicilan, amount, key()))
				results <- err
			}()
		}
		close(start)
		wg.Wait()
		close(results)
		for err := range results {
			switch {
			case err == nil:
				okCount++
			case errors.Is(err, invoice.ErrOverpayment):
				overCount++
			default:
				t.Fatalf("unexpected error: %v", err)
			}
		}
		got, err := f.inv.Get(f.ctx, f.wsA, inv.ID)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, got.Balance, int64(0), "never below zero")
		assert.Equal(t, got.Total-int64(okCount)*amount, got.Balance)
		return
	}
	okN, overN := run(302_696) // 60 %: only one fits
	assert.Equal(t, 1, okN)
	assert.Equal(t, 1, overN)
	okN, overN = run(201_797) // 40 %: both fit
	assert.Equal(t, 2, okN)
	assert.Equal(t, 0, overN)
}

func TestReversePaymentRestoresTheBalance(t *testing.T) {
	f := setup(t)
	inv := f.issued(t, "2026-10-27")
	p, _, err := f.inv.RecordPayment(f.ctx, f.wsA, f.userA, inv.ID, payment(invoice.KindDP, 200_000, key()))
	require.NoError(t, err)
	rev, err := f.inv.ReversePayment(f.ctx, f.wsA, f.userA, p.ID, "transfer ternyata gagal")
	require.NoError(t, err)
	require.NotNil(t, rev.Reversal)
	assert.Equal(t, "transfer ternyata gagal", rev.Reversal.Reason)
	_, err = f.inv.ReversePayment(f.ctx, f.wsA, f.userA, p.ID, "lagi")
	assert.ErrorIs(t, err, invoice.ErrAlreadyReversed)
	_, err = f.inv.ReversePayment(f.ctx, f.wsA, f.userA, p.ID, "   ")
	assert.ErrorIs(t, err, invoice.ErrInvalidReason)

	got, err := f.inv.Get(f.ctx, f.wsA, inv.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(504_493), got.Balance)
	// The same amount can be recorded again with a fresh key.
	_, created, err := f.inv.RecordPayment(f.ctx, f.wsA, f.userA, inv.ID, payment(invoice.KindDP, 200_000, key()))
	require.NoError(t, err)
	assert.True(t, created)
	got, err = f.inv.Get(f.ctx, f.wsA, inv.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(304_493), got.Balance)
	assert.Equal(t, []lead.Kind{lead.KindPaymentRecorded, lead.KindPaymentReversed, lead.KindPaymentRecorded, lead.KindInvoiceIssued, lead.KindInvoiceCreated, lead.KindCreated}, f.kinds(t, f.wsA, f.leadA.ID))
}

func TestVoidRefusesInvoicesWithPayments(t *testing.T) {
	f := setup(t)
	inv := f.issued(t, "2026-10-27")
	p, _, err := f.inv.RecordPayment(f.ctx, f.wsA, f.userA, inv.ID, payment(invoice.KindDP, 200_000, key()))
	require.NoError(t, err)
	_, err = f.inv.Void(f.ctx, f.wsA, f.userA, inv.ID)
	assert.ErrorIs(t, err, invoice.ErrHasPayments)
	_, err = f.inv.ReversePayment(f.ctx, f.wsA, f.userA, p.ID, "batal")
	require.NoError(t, err)
	voided, err := f.inv.Void(f.ctx, f.wsA, f.userA, inv.ID)
	require.NoError(t, err)
	assert.Equal(t, invoice.StatusVoid, voided.Status)
	assert.NotNil(t, voided.VoidedAt)
	assert.False(t, voided.ShareActive())
	_, err = f.inv.GetByToken(f.ctx, inv.ShareToken)
	assert.ErrorIs(t, err, invoice.ErrNotFound, "the public link stops")
	_, _, err = f.inv.RecordPayment(f.ctx, f.wsA, f.userA, inv.ID, payment(invoice.KindDP, 1, key()))
	assert.ErrorIs(t, err, invoice.ErrNotIssued)
	_, err = f.inv.Void(f.ctx, f.wsA, f.userA, inv.ID)
	assert.ErrorIs(t, err, invoice.ErrLocked, "voiding twice")

	d, err := f.inv.Create(f.ctx, f.wsA, f.userA, f.leadA.ID, nil, draft(""))
	require.NoError(t, err)
	dv, err := f.inv.Void(f.ctx, f.wsA, f.userA, d.ID)
	require.NoError(t, err, "drafts can be voided")
	assert.Equal(t, invoice.StatusVoid, dv.Status)
}

func TestShareLinkLifecycle(t *testing.T) {
	f := setup(t)
	inv := f.issued(t, "2026-10-27")
	_, _, err := f.inv.RecordPayment(f.ctx, f.wsA, f.userA, inv.ID, payment(invoice.KindDP, 200_000, key()))
	require.NoError(t, err)
	pub, err := f.inv.GetByToken(f.ctx, inv.ShareToken)
	require.NoError(t, err)
	assert.Equal(t, inv.ID, pub.ID)
	assert.Equal(t, int64(304_493), pub.Balance)
	assert.Len(t, pub.Items, 2)
	assert.Len(t, pub.Payments, 1)
	require.NoError(t, f.inv.RecordOpen(f.ctx, f.wsA, inv.ID))
	require.NoError(t, f.inv.RecordOpen(f.ctx, f.wsA, inv.ID))
	got, err := f.inv.Get(f.ctx, f.wsA, inv.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, got.OpenCount)
	assert.NotNil(t, got.FirstOpenedAt)

	revoked, err := f.inv.RevokeShare(f.ctx, f.wsA, inv.ID)
	require.NoError(t, err)
	assert.False(t, revoked.ShareActive())
	_, err = f.inv.GetByToken(f.ctx, inv.ShareToken)
	assert.ErrorIs(t, err, invoice.ErrNotFound)
	renewed, err := f.inv.RenewShare(f.ctx, f.wsA, inv.ID)
	require.NoError(t, err)
	assert.True(t, renewed.ShareActive())
	assert.NotEqual(t, inv.ShareToken, renewed.ShareToken)
	_, err = f.inv.GetByToken(f.ctx, "short")
	assert.ErrorIs(t, err, invoice.ErrNotFound)

	d, err := f.inv.Create(f.ctx, f.wsA, f.userA, f.leadA.ID, nil, draft(""))
	require.NoError(t, err)
	_, err = f.inv.RenewShare(f.ctx, f.wsA, d.ID)
	assert.ErrorIs(t, err, invoice.ErrLocked, "drafts have no link")

	latest, ok, err := f.inv.LatestShared(f.ctx, f.wsA, f.leadA.ID)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, renewed.ShareToken, latest.ShareToken)
	_, ok, err = f.inv.LatestShared(f.ctx, f.wsB, f.leadB.ID)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestListsOverdueAndSummaryReconcile(t *testing.T) {
	f := setup(t)
	paid := f.issued(t, "2026-10-15")
	_, _, err := f.inv.RecordPayment(f.ctx, f.wsA, f.userA, paid.ID, payment(invoice.KindPelunasan, 504_493, key()))
	require.NoError(t, err)
	overdue := f.issued(t, "2026-10-19")
	_, _, err = f.inv.RecordPayment(f.ctx, f.wsA, f.userA, overdue.ID, payment(invoice.KindDP, 200_000, key()))
	require.NoError(t, err)
	dueToday := f.issued(t, "2026-10-20")
	future := f.issued(t, "2026-11-01")
	d, err := f.inv.Create(f.ctx, f.wsA, f.userA, f.leadA.ID, nil, draft(""))
	require.NoError(t, err)
	v, err := f.inv.Create(f.ctx, f.wsA, f.userA, f.leadA.ID, nil, draft(""))
	require.NoError(t, err)
	_, err = f.inv.Void(f.ctx, f.wsA, f.userA, v.ID)
	require.NoError(t, err)
	// Noise in another workspace must not show anywhere.
	other, err := f.inv.Create(f.ctx, f.wsB, f.userB, f.leadB.ID, nil, draft("2026-10-01"))
	require.NoError(t, err)
	_, err = f.inv.Issue(f.ctx, f.wsB, f.userB, other.ID)
	require.NoError(t, err)

	ids := func(p invoice.Page) []uuid.UUID {
		out := []uuid.UUID{}
		for _, i := range p.Items {
			out = append(out, i.ID)
		}
		return out
	}
	open, err := f.inv.List(f.ctx, f.wsA, invoice.ListQuery{}, f.today)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{overdue.ID, dueToday.ID, future.ID}, ids(open), "unpaid, overdue first then by due date")
	late, err := f.inv.List(f.ctx, f.wsA, invoice.ListQuery{Status: "terlambat"}, f.today)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{overdue.ID}, ids(late))
	lunas, err := f.inv.List(f.ctx, f.wsA, invoice.ListQuery{Status: "lunas"}, f.today)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{paid.ID}, ids(lunas))
	drafts, err := f.inv.List(f.ctx, f.wsA, invoice.ListQuery{Status: "draf"}, f.today)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{d.ID}, ids(drafts))
	voids, err := f.inv.List(f.ctx, f.wsA, invoice.ListQuery{Status: "dibatalkan"}, f.today)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{v.ID}, ids(voids))
	all, err := f.inv.List(f.ctx, f.wsA, invoice.ListQuery{Status: "semua"}, f.today)
	require.NoError(t, err)
	assert.Equal(t, 6, all.Total)
	byNumber, err := f.inv.List(f.ctx, f.wsA, invoice.ListQuery{Status: "semua", Q: "0002"}, f.today)
	require.NoError(t, err)
	assert.Equal(t, []uuid.UUID{overdue.ID}, ids(byNumber))
	byName, err := f.inv.List(f.ctx, f.wsA, invoice.ListQuery{Status: "semua", Q: "pelanggan a"}, f.today)
	require.NoError(t, err)
	assert.Equal(t, 6, byName.Total)

	due, err := f.inv.Overdue(f.ctx, f.wsA, f.today, 10)
	require.NoError(t, err)
	require.Len(t, due, 1)
	assert.Equal(t, overdue.ID, due[0].ID)
	assert.Equal(t, int64(304_493), due[0].Balance)

	sum, err := f.inv.Summary(f.ctx, f.wsA, f.today)
	require.NoError(t, err)
	var outstanding, overdueAmt int64
	var outstandingN, overdueN int
	for _, i := range open.Items {
		outstanding += i.Balance
		outstandingN++
		if i.Effective(f.today) == invoice.StatusOverdue {
			overdueAmt += i.Balance
			overdueN++
		}
	}
	assert.Equal(t, outstanding, sum.Outstanding, "dashboard reconciles with the list")
	assert.Equal(t, outstandingN, sum.OutstandingCount)
	assert.Equal(t, overdueAmt, sum.OverdueAmount)
	assert.Equal(t, overdueN, sum.OverdueCount)
	assert.Equal(t, int64(704_493), sum.PaidThisMonth, "both payments were received in October")

	forLead, err := f.inv.ForLead(f.ctx, f.wsA, f.leadA.ID)
	require.NoError(t, err)
	assert.Len(t, forLead, 6)
	assert.Equal(t, v.ID, forLead[0].ID, "newest first")
}

func TestCrossWorkspaceIsInvisible(t *testing.T) {
	f := setup(t)
	inv := f.issued(t, "2026-10-27")
	p, _, err := f.inv.RecordPayment(f.ctx, f.wsA, f.userA, inv.ID, payment(invoice.KindDP, 200_000, key()))
	require.NoError(t, err)

	_, err = f.inv.Get(f.ctx, f.wsB, inv.ID)
	assert.ErrorIs(t, err, invoice.ErrNotFound)
	_, err = f.inv.Update(f.ctx, f.wsB, f.userB, inv.ID, draft(""), 1)
	assert.ErrorIs(t, err, invoice.ErrNotFound)
	_, err = f.inv.Issue(f.ctx, f.wsB, f.userB, inv.ID)
	assert.ErrorIs(t, err, invoice.ErrNotFound)
	_, err = f.inv.Void(f.ctx, f.wsB, f.userB, inv.ID)
	assert.ErrorIs(t, err, invoice.ErrNotFound)
	_, _, err = f.inv.RecordPayment(f.ctx, f.wsB, f.userB, inv.ID, payment(invoice.KindDP, 1, key()))
	assert.ErrorIs(t, err, invoice.ErrNotFound)
	_, err = f.inv.ReversePayment(f.ctx, f.wsB, f.userB, p.ID, "bukan punya saya")
	assert.ErrorIs(t, err, invoice.ErrNotFound)
	_, err = f.inv.RevokeShare(f.ctx, f.wsB, inv.ID)
	assert.ErrorIs(t, err, invoice.ErrNotFound)
	assert.NoError(t, errors.Join(nil))
	_, err = f.inv.Create(f.ctx, f.wsB, f.userB, f.leadA.ID, nil, draft(""))
	assert.ErrorIs(t, err, invoice.ErrLeadNotFound, "a lead of another workspace")

	got, err := f.inv.Get(f.ctx, f.wsA, inv.ID)
	require.NoError(t, err)
	assert.Equal(t, invoice.StatusIssued, got.Status)
	assert.Equal(t, int64(304_493), got.Balance)
	require.Len(t, got.Payments, 1)
	assert.Nil(t, got.Payments[0].Reversal, "nothing written from the other workspace")
	assert.Equal(t, []lead.Kind{lead.KindCreated}, f.kinds(t, f.wsB, f.leadB.ID))
}

func ptr[T any](v T) *T { return &v }
