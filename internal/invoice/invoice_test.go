package invoice_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kerjalancar/internal/invoice"
	"kerjalancar/internal/quotation"
)

func date(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func rows() []quotation.ItemInput {
	return []quotation.ItemInput{{Description: "Kitchen set HPL", Qty: "2,5", Unit: "m", UnitPrice: "1.999"}, {Description: "Jasa pasang", Qty: "1", Unit: "ls", UnitPrice: "500.000"}}
}

func TestValidateComputesTotalsAndParsesTheDueDate(t *testing.T) {
	n, errs := invoice.Validate(invoice.Input{Items: rows(), DiscountPct: "10", TaxPct: "11", DueOn: "2026-10-27", Notes: " Transfer ke BCA. "})
	require.Empty(t, errs)
	assert.Equal(t, int64(504_998), n.Subtotal)
	assert.Equal(t, int64(50_500), n.DiscountAmount)
	assert.Equal(t, int64(49_995), n.TaxAmount)
	assert.Equal(t, int64(504_493), n.Total)
	require.NotNil(t, n.DueOn)
	assert.Equal(t, date(2026, 10, 27), *n.DueOn)
	assert.Equal(t, "Transfer ke BCA.", n.Notes)

	n, errs = invoice.Validate(invoice.Input{Items: rows()})
	require.Empty(t, errs, "a draft may be saved without a due date")
	assert.Nil(t, n.DueOn)

	_, errs = invoice.Validate(invoice.Input{Items: rows(), DueOn: "2026-13-40"})
	assert.Equal(t, "Tanggal tidak valid.", errs["due_on"])

	_, errs = invoice.Validate(invoice.Input{Items: []quotation.ItemInput{{Description: "x", Qty: "abc", UnitPrice: "1"}}, DiscountPct: "101"})
	assert.Equal(t, "Jumlah tidak valid (contoh: 2,5).", errs["items.0.qty"], "item errors come from the quotation validator")
	assert.Equal(t, "Diskon harus 0–100 persen.", errs["discount_pct"])
}

func TestValidatePayment(t *testing.T) {
	today := date(2026, 10, 20)
	key := "AAAAAAAAAAAAAAAAAAAAAA"
	ok := invoice.PaymentInput{Kind: "dp", Method: "transfer", Amount: "5.000.000", ReceivedOn: "2026-10-20", Note: " BCA a.n. Budi ", IdempotencyKey: key}
	p, errs := invoice.ValidatePayment(ok, today)
	require.Empty(t, errs)
	assert.Equal(t, invoice.KindDP, p.Kind)
	assert.Equal(t, invoice.MethodTransfer, p.Method)
	assert.Equal(t, int64(5_000_000), p.Amount)
	assert.Equal(t, today, p.ReceivedOn)
	assert.Equal(t, "BCA a.n. Budi", p.Note)
	assert.Equal(t, key, p.IdempotencyKey)

	cases := []struct {
		name  string
		mut   func(*invoice.PaymentInput)
		field string
		msg   string
	}{
		{"zero amount", func(i *invoice.PaymentInput) { i.Amount = "0" }, "amount", "Jumlah tidak valid. Contoh: 5.000.000"},
		{"text amount", func(i *invoice.PaymentInput) { i.Amount = "abc" }, "amount", "Jumlah tidak valid. Contoh: 5.000.000"},
		{"empty amount", func(i *invoice.PaymentInput) { i.Amount = "" }, "amount", "Jumlah tidak valid. Contoh: 5.000.000"},
		{"kind", func(i *invoice.PaymentInput) { i.Kind = "refund" }, "kind", "Jenis pembayaran tidak valid."},
		{"method", func(i *invoice.PaymentInput) { i.Method = "qris" }, "method", "Metode tidak valid."},
		{"future date", func(i *invoice.PaymentInput) { i.ReceivedOn = "2026-10-21" }, "received_on", "Tanggal tidak boleh di masa depan."},
		{"bad date", func(i *invoice.PaymentInput) { i.ReceivedOn = "kemarin" }, "received_on", "Tanggal tidak valid."},
		{"empty date", func(i *invoice.PaymentInput) { i.ReceivedOn = "" }, "received_on", "Tanggal tidak valid."},
		{"long note", func(i *invoice.PaymentInput) { i.Note = strings.Repeat("a", 301) }, "note", "Catatan maksimal 300 karakter."},
		{"short key", func(i *invoice.PaymentInput) { i.IdempotencyKey = "short" }, "form", "Formulir kedaluwarsa, muat ulang halaman."},
		{"key with slash", func(i *invoice.PaymentInput) { i.IdempotencyKey = "AAAAAAAAAAAAAAAAAAAAA/" }, "form", "Formulir kedaluwarsa, muat ulang halaman."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := ok
			tc.mut(&in)
			_, errs := invoice.ValidatePayment(in, today)
			assert.Equal(t, tc.msg, errs[tc.field], "%v", errs)
		})
	}
	k1, err := invoice.NewIdempotencyKey()
	require.NoError(t, err)
	k2, err := invoice.NewIdempotencyKey()
	require.NoError(t, err)
	assert.Len(t, k1, 22)
	assert.NotEqual(t, k1, k2)
}

func TestEffectiveStatus(t *testing.T) {
	today := date(2026, 10, 20)
	yesterday, tomorrow := date(2026, 10, 19), date(2026, 10, 21)
	cases := []struct {
		name string
		inv  invoice.Invoice
		want invoice.Status
	}{
		{"draft", invoice.Invoice{Status: invoice.StatusDraft, Total: 100, Balance: 100}, invoice.StatusDraft},
		{"void", invoice.Invoice{Status: invoice.StatusVoid, Total: 100, Balance: 100, DueOn: &yesterday}, invoice.StatusVoid},
		{"issued, paid", invoice.Invoice{Status: invoice.StatusIssued, Total: 100, Balance: 0, DueOn: &yesterday}, invoice.StatusPaid},
		{"issued, overdue", invoice.Invoice{Status: invoice.StatusIssued, Total: 100, Balance: 40, DueOn: &yesterday}, invoice.StatusOverdue},
		{"issued, due today", invoice.Invoice{Status: invoice.StatusIssued, Total: 100, Balance: 40, DueOn: &today}, invoice.StatusIssued},
		{"issued, due tomorrow", invoice.Invoice{Status: invoice.StatusIssued, Total: 100, Balance: 40, DueOn: &tomorrow}, invoice.StatusIssued},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { assert.Equal(t, tc.want, tc.inv.Effective(today)) })
	}
	assert.Equal(t, "Lunas", invoice.StatusPaid.Label())
	assert.Equal(t, "Terlambat", invoice.StatusOverdue.Label())
	assert.Equal(t, "Terbit", invoice.StatusIssued.Label())
	assert.Equal(t, "Dibatalkan", invoice.StatusVoid.Label())
	assert.Equal(t, "Draf", invoice.StatusDraft.Label())
	assert.Equal(t, "0007", invoice.Invoice{Seq: 7}.Number())
	assert.Equal(t, "DP", invoice.KindDP.Label())
	assert.Equal(t, "Pelunasan", invoice.KindPelunasan.Label())
	assert.Equal(t, "Tunai", invoice.MethodTunai.Label())
}
