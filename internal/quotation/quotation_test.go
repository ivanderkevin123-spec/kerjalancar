package quotation_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kerjalancar/internal/quotation"
)

func date(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func TestValidateComputesTotalsFromTheItems(t *testing.T) {
	n, errs := quotation.Validate(quotation.Input{
		Items: []quotation.ItemInput{
			{Description: " Kitchen set HPL ", Qty: "2,5", Unit: "m", UnitPrice: "1.999"},
			{Description: "", Qty: "", Unit: "", UnitPrice: ""}, // blank row is skipped
			{Description: "Jasa pasang", Qty: "1", Unit: "ls", UnitPrice: "Rp 500.000"},
		},
		DiscountPct: "10", TaxPct: "11", ValidUntil: "2026-10-31", Notes: " Harga belum termasuk listrik ",
	})
	require.Empty(t, errs)
	require.Len(t, n.Items, 2)
	assert.Equal(t, "Kitchen set HPL", n.Items[0].Description)
	assert.Equal(t, int64(2500), n.Items[0].QtyMilli)
	assert.Equal(t, int64(4998), n.Items[0].LineTotal, "2.5 × 1.999 = 4.997,5 → 4.998")
	assert.Equal(t, int64(500_000), n.Items[1].LineTotal)
	assert.Equal(t, int64(504_998), n.Subtotal)
	assert.Equal(t, int64(50_500), n.DiscountAmount, "10 % of 504.998 = 50.499,8 → 50.500")
	assert.Equal(t, int64(49_995), n.TaxAmount, "11 % of 454.498 = 49.994,78 → 49.995")
	assert.Equal(t, int64(504_493), n.Total)
	assert.Equal(t, 10, n.DiscountPct)
	assert.Equal(t, 11, n.TaxPct)
	require.NotNil(t, n.ValidUntil)
	assert.Equal(t, date(2026, 10, 31), *n.ValidUntil)
	assert.Equal(t, "Harga belum termasuk listrik", n.Notes)
}

func TestValidateHandComputedTable(t *testing.T) {
	cases := []struct {
		name     string
		items    []quotation.ItemInput
		disc, tx string
		want     [4]int64 // subtotal, discount, tax, total
	}{
		{"single whole item, no discount or tax", []quotation.ItemInput{{Description: "A", Qty: "1", UnitPrice: "15.000.000"}}, "0", "0", [4]int64{15_000_000, 0, 0, 15_000_000}},
		{"two items with PPN 11 %", []quotation.ItemInput{{Description: "A", Qty: "1", UnitPrice: "15.000.000"}, {Description: "B", Qty: "3", UnitPrice: "250.000"}}, "", "11", [4]int64{15_750_000, 0, 1_732_500, 17_482_500}},
		{"fractional qty and rounding", []quotation.ItemInput{{Description: "A", Qty: "0,333", UnitPrice: "1.000"}}, "", "", [4]int64{333, 0, 0, 333}},
		{"full discount", []quotation.ItemInput{{Description: "A", Qty: "2", UnitPrice: "100"}}, "100", "11", [4]int64{200, 200, 0, 0}},
	}
	for _, tc := range cases {
		n, errs := quotation.Validate(quotation.Input{Items: tc.items, DiscountPct: tc.disc, TaxPct: tc.tx})
		require.Empty(t, errs, tc.name)
		assert.Equal(t, tc.want, [4]int64{n.Subtotal, n.DiscountAmount, n.TaxAmount, n.Total}, tc.name)
	}
}

func TestValidateReportsEveryBadField(t *testing.T) {
	_, errs := quotation.Validate(quotation.Input{Items: []quotation.ItemInput{{}}})
	assert.Equal(t, "Tambahkan minimal satu baris.", errs["items"])

	_, errs = quotation.Validate(quotation.Input{
		Items:       []quotation.ItemInput{{Description: "", Qty: "0", Unit: strings.Repeat("u", 21), UnitPrice: "abc"}, {Description: strings.Repeat("d", 301), Qty: "1,2345", UnitPrice: "-1"}},
		DiscountPct: "101", TaxPct: "-1", ValidUntil: "31/10/2026", Notes: strings.Repeat("n", 2001),
	})
	assert.Equal(t, "Uraian wajib diisi.", errs["items.0.description"])
	assert.Equal(t, "Jumlah tidak valid (contoh: 2,5).", errs["items.0.qty"])
	assert.Equal(t, "Satuan maksimal 20 karakter.", errs["items.0.unit"])
	assert.Equal(t, "Harga satuan tidak valid. Contoh: 150.000", errs["items.0.unit_price"])
	assert.Equal(t, "Uraian maksimal 300 karakter.", errs["items.1.description"])
	assert.Equal(t, "Jumlah tidak valid (contoh: 2,5).", errs["items.1.qty"])
	assert.Equal(t, "Harga satuan tidak valid. Contoh: 150.000", errs["items.1.unit_price"])
	assert.Equal(t, "Diskon harus 0–100 persen.", errs["discount_pct"])
	assert.Equal(t, "Pajak harus 0–100 persen.", errs["tax_pct"])
	assert.Equal(t, "Tanggal tidak valid.", errs["valid_until"])
	assert.Equal(t, "Catatan maksimal 2000 karakter.", errs["notes"])

	_, errs = quotation.Validate(quotation.Input{Items: []quotation.ItemInput{{Description: "Besar", Qty: "1000000", UnitPrice: "10.000.000.000.000"}}})
	assert.Equal(t, "Jumlah baris melebihi batas.", errs["items.0.unit_price"], "a line beyond MaxIDR is reported on its row")
	_, errs = quotation.Validate(quotation.Input{Items: []quotation.ItemInput{{Description: "A", Qty: "1", UnitPrice: "10.000.000.000.000"}, {Description: "B", Qty: "1", UnitPrice: "1"}}})
	assert.Equal(t, "Total melebihi batas.", errs["items"], "lines that are each fine may still overflow together")
}

func TestStatusesAndEffectiveStatus(t *testing.T) {
	assert.Equal(t, "Draf", quotation.StatusDraft.Label())
	assert.Equal(t, "Kedaluwarsa", quotation.StatusExpired.Label())
	today := date(2026, 10, 20)
	due := date(2026, 10, 19)
	q := quotation.Quotation{Status: quotation.StatusSent, ValidUntil: &due}
	assert.Equal(t, quotation.StatusExpired, q.EffectiveStatus(today))
	q.ValidUntil = &today
	assert.Equal(t, quotation.StatusSent, q.EffectiveStatus(today), "valid through the day itself")
	q.Status = quotation.StatusAccepted
	q.ValidUntil = &due
	assert.Equal(t, quotation.StatusAccepted, q.EffectiveStatus(today), "only sent quotations expire")
	assert.True(t, quotation.Quotation{Status: quotation.StatusDraft}.Editable())
	assert.True(t, quotation.Quotation{Status: quotation.StatusSent}.Editable())
	assert.False(t, quotation.Quotation{Status: quotation.StatusAccepted}.Editable())
	assert.Equal(t, "0007", quotation.FormatNumber(7))
	assert.Equal(t, "10234", quotation.FormatNumber(10234))
}
