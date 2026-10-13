package invoice

import (
	"io"
	"time"

	"kerjalancar/internal/docpdf"
	"kerjalancar/internal/money"
	"kerjalancar/internal/quotation"
)

// PDFOptions mirror quotation.PDFOptions: Loc for the document date (WIB when
// nil) and Today for the effective status.
type PDFOptions struct {
	Compress  bool
	CreatedAt time.Time
	Loc       *time.Location
	Today     time.Time
}

// RenderPDF writes the invoice as an A4 document (layout in docpdf) with the
// amount already paid and the balance. No phone: the same bytes are served
// from the public link.
func RenderPDF(w io.Writer, inv Invoice, opts PDFOptions) error {
	loc := opts.Loc
	if loc == nil {
		loc = docpdf.WIB
	}
	status := inv.Status
	if !opts.Today.IsZero() {
		status = inv.Effective(opts.Today)
	}
	date := inv.Date()
	d := docpdf.Document{
		Kind:   "TAGIHAN",
		Title:  "Tagihan " + inv.Number() + " - " + inv.WorkspaceName,
		Author: inv.WorkspaceName,
		Number: inv.Number(),
		Status: status.Label(),
		Dates: []docpdf.Line{
			{Label: "Tanggal", Value: docpdf.Date(&date, loc)},
			{Label: "Jatuh tempo", Value: docpdf.DateUTC(inv.DueOn)},
		},
		Recipient: inv.CustomerName,
		Project:   inv.LeadTitle,
		Notes:     inv.Notes,
		Footer:    "Dibuat dengan KerjaLancar. Harga dalam rupiah. Catatan pembayaran adalah pencatatan penyedia jasa.",
	}
	for _, it := range inv.Items {
		d.Items = append(d.Items, docpdf.Item{Description: it.Description, Qty: money.FormatQty(it.QtyMilli), Unit: it.Unit, UnitPrice: money.FormatIDR(it.UnitPrice), LineTotal: money.FormatIDR(it.LineTotal)})
	}
	d.Totals = quotation.TotalLines(inv.Subtotal, inv.DiscountPct, inv.DiscountAmount, inv.TaxPct, inv.TaxAmount, inv.Total)
	d.Totals = append(d.Totals,
		docpdf.Line{Label: "Sudah dibayar", Value: money.FormatIDR(inv.Paid)},
		docpdf.Line{Label: "SISA", Value: money.FormatIDR(inv.Balance), Bold: true},
	)
	return docpdf.Render(w, d, docpdf.Options{Compress: opts.Compress, CreatedAt: opts.CreatedAt, Loc: loc})
}
