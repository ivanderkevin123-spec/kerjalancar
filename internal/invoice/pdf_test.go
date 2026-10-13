package invoice_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kerjalancar/internal/invoice"
	"kerjalancar/internal/quotation"
)

func TestRenderPDFShowsBalanceAndNoPhone(t *testing.T) {
	due := date(2026, 10, 27)
	issued := time.Date(2026, 10, 12, 22, 0, 0, 0, time.UTC) // 13 Okt 05:00 WIB
	inv := invoice.Invoice{
		WorkspaceName: "Studio Uji", CustomerName: "Budi Santoso", CustomerPhone: "+6281277770101", LeadTitle: "Renovasi dapur",
		Seq: 3, Status: invoice.StatusIssued, DueOn: &due, IssuedAt: &issued, CreatedAt: issued, Notes: "Transfer ke BCA 123.\nSertakan nomor tagihan.",
		DiscountPct: 10, TaxPct: 11, Subtotal: 504_998, DiscountAmount: 50_500, TaxAmount: 49_995, Total: 504_493, Paid: 200_000, Balance: 304_493,
		Items: []quotation.Item{{Description: "Kitchen set HPL", QtyMilli: 2500, Unit: "m", UnitPrice: 1999, LineTotal: 4998}, {Description: "Jasa pasang", QtyMilli: 1000, Unit: "ls", UnitPrice: 500_000, LineTotal: 500_000}},
	}
	var buf bytes.Buffer
	require.NoError(t, invoice.RenderPDF(&buf, inv, invoice.PDFOptions{Compress: false, Today: date(2026, 11, 1)}))
	out := buf.String()
	for _, want := range []string{"TAGIHAN", "No. 0003", "Status: Terlambat", "Tanggal: 13 Okt 2026", "Jatuh tempo: 27 Okt 2026", "Budi Santoso", "Renovasi dapur", "Kitchen set HPL", "Rp4.998", "Subtotal", "Diskon 10%", "Pajak 11%", "TOTAL", "Rp504.493", "Sudah dibayar", "Rp200.000", "SISA", "Rp304.493", "Sertakan nomor tagihan"} {
		assert.Contains(t, out, want)
	}
	assert.NotContains(t, out, "7777", "the customer's phone never reaches the PDF")
	assert.NotContains(t, out, "BCA 123. Sertakan", "newline kept")

	buf.Reset()
	inv.Balance, inv.Paid = 0, 504_493
	require.NoError(t, invoice.RenderPDF(&buf, inv, invoice.PDFOptions{Compress: false, Today: date(2026, 11, 1)}))
	assert.Contains(t, buf.String(), "Status: Lunas")
}
