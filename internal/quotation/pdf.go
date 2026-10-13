package quotation

import (
	"fmt"
	"io"
	"time"

	"kerjalancar/internal/docpdf"
	"kerjalancar/internal/money"
)

// PDFOptions control rendering. Loc is the zone for the document date (WIB
// when nil); Today, when set, decides whether a sent quotation prints as
// expired. Tests disable compression to read the text streams and pin the
// timestamps so two renders have the same size.
type PDFOptions struct {
	Compress  bool
	CreatedAt time.Time
	Loc       *time.Location
	Today     time.Time
}

// RenderPDF writes the quotation as an A4 document (layout in docpdf). The
// same bytes are served to the owner and from the public link, so the
// customer's phone is deliberately left out.
func RenderPDF(w io.Writer, q Quotation, opts PDFOptions) error {
	loc := opts.Loc
	if loc == nil {
		loc = docpdf.WIB
	}
	status := q.Status
	if !opts.Today.IsZero() {
		status = q.EffectiveStatus(opts.Today)
	}
	created := q.CreatedAt
	d := docpdf.Document{
		Kind:   "PENAWARAN",
		Title:  "Penawaran " + q.Number() + " - " + q.WorkspaceName,
		Author: q.WorkspaceName,
		Number: q.Number(),
		Status: status.Label(),
		Dates: []docpdf.Line{
			{Label: "Tanggal", Value: docpdf.Date(&created, loc)},
			// ValidUntil is a calendar date (UTC midnight); print it as stored.
			{Label: "Berlaku sampai", Value: docpdf.DateUTC(q.ValidUntil)},
		},
		Recipient: q.CustomerName,
		Project:   q.LeadTitle,
		Notes:     q.Notes,
		Footer:    "Dibuat dengan KerjaLancar. Harga dalam rupiah.",
	}
	for _, it := range q.Items {
		d.Items = append(d.Items, docpdf.Item{Description: it.Description, Qty: money.FormatQty(it.QtyMilli), Unit: it.Unit, UnitPrice: money.FormatIDR(it.UnitPrice), LineTotal: money.FormatIDR(it.LineTotal)})
	}
	d.Totals = TotalLines(q.Subtotal, q.DiscountPct, q.DiscountAmount, q.TaxPct, q.TaxAmount, q.Total)
	return docpdf.Render(w, d, docpdf.Options{Compress: opts.Compress, CreatedAt: opts.CreatedAt, Loc: loc})
}

// TotalLines formats the totals block shared by quotations and invoices.
func TotalLines(subtotal int64, discountPct int, discountAmount int64, taxPct int, taxAmount, total int64) []docpdf.Line {
	lines := []docpdf.Line{{Label: "Subtotal", Value: money.FormatIDR(subtotal)}}
	if discountPct > 0 {
		lines = append(lines, docpdf.Line{Label: fmt.Sprintf("Diskon %d%%", discountPct), Value: "-" + money.FormatIDR(discountAmount)})
	}
	if taxPct > 0 {
		lines = append(lines, docpdf.Line{Label: fmt.Sprintf("Pajak %d%%", taxPct), Value: money.FormatIDR(taxAmount)})
	}
	return append(lines, docpdf.Line{Label: "TOTAL", Value: money.FormatIDR(total), Bold: true})
}
