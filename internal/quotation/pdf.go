package quotation

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"

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

// wib is the fallback zone: the westernmost Indonesian zone, as on the share page.
var wib = time.FixedZone("WIB", 7*3600)

// encode maps text to the core-font code page (cp1252, one byte per
// character). Characters without a glyph become "?" rather than garbage;
// control characters are dropped and newlines kept only where the cell wraps.
func encode(tr func(string) string, s string, multiline bool) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\n' && multiline:
			b.WriteByte('\n')
		case r == '\n' || r == '\t':
			b.WriteByte(' ')
		case r < 0x20 || r == 0x7f:
		case r < 0x80:
			b.WriteRune(r)
		default:
			// fpdf substitutes "." for runes outside the code page; "?" reads as a gap.
			if t := tr(string(r)); len(t) == 1 && t != "." {
				b.WriteString(t)
			} else {
				b.WriteByte('?')
			}
		}
	}
	return b.String()
}

func pdfDate(t *time.Time, loc *time.Location) string {
	if t == nil {
		return "-"
	}
	d := t.In(loc)
	months := [...]string{"", "Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}
	return fmt.Sprintf("%d %s %d", d.Day(), months[d.Month()], d.Year())
}

// RenderPDF writes an A4 document: header with the workspace and number,
// customer name and lead, the items table, totals, notes. The same bytes are
// served to the owner and from the public link, so the customer's phone is
// deliberately left out.
func RenderPDF(w io.Writer, q Quotation, opts PDFOptions) error {
	loc := opts.Loc
	if loc == nil {
		loc = wib
	}
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetCompression(opts.Compress)
	if !opts.CreatedAt.IsZero() {
		pdf.SetCreationDate(opts.CreatedAt)
		pdf.SetModificationDate(opts.CreatedAt)
	}
	// The translator keeps an internal buffer, so one per document, never shared.
	tr := pdf.UnicodeTranslatorFromDescriptor("")
	latin1 := func(s string) string { return encode(tr, s, false) }
	pdf.SetTitle(latin1("Penawaran "+q.Number()+" - "+q.WorkspaceName), false)
	pdf.SetAuthor(latin1(q.WorkspaceName), false)
	pdf.SetProducer("KerjaLancar", false)
	pdf.SetMargins(18, 18, 18)
	// Page breaks are placed by hand (before a row or block that would not
	// fit), so a wrapped description never straddles two pages.
	pdf.SetAutoPageBreak(false, 0)
	pdf.AddPage()
	pageW, pageH := pdf.GetPageSize()
	left, right := 18.0, pageW-18.0
	width := right - left
	bottom := pageH - 20
	ensure := func(h float64) {
		if pdf.GetY()+h > bottom {
			pdf.AddPage()
		}
	}

	// Header
	pdf.SetFont("Helvetica", "B", 18)
	pdf.CellFormat(width/2, 9, "PENAWARAN", "", 0, "L", false, 0, "")
	pdf.SetFont("Helvetica", "B", 12)
	pdf.CellFormat(width/2, 9, latin1(q.WorkspaceName), "", 1, "R", false, 0, "")
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetTextColor(90, 90, 90)
	pdf.CellFormat(width/2, 6, "No. "+q.Number(), "", 0, "L", false, 0, "")
	created := q.CreatedAt
	pdf.CellFormat(width/2, 6, "Tanggal: "+pdfDate(&created, loc), "", 1, "R", false, 0, "")
	status := q.Status
	if !opts.Today.IsZero() {
		status = q.EffectiveStatus(opts.Today)
	}
	pdf.CellFormat(width/2, 6, "Status: "+latin1(status.Label()), "", 0, "L", false, 0, "")
	// ValidUntil is a calendar date (UTC midnight); print it as stored.
	pdf.CellFormat(width/2, 6, "Berlaku sampai: "+pdfDate(q.ValidUntil, time.UTC), "", 1, "R", false, 0, "")
	pdf.SetTextColor(0, 0, 0)
	pdf.Ln(4)
	pdf.SetDrawColor(200, 200, 200)
	pdf.Line(left, pdf.GetY(), right, pdf.GetY())
	pdf.Ln(4)

	// Customer and lead
	pdf.SetFont("Helvetica", "B", 10)
	pdf.CellFormat(width, 6, "Kepada", "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 10)
	pdf.CellFormat(width, 6, latin1(q.CustomerName), "", 1, "L", false, 0, "")
	pdf.CellFormat(width, 6, "Proyek: "+latin1(q.LeadTitle), "", 1, "L", false, 0, "")
	pdf.Ln(4)

	// Items table
	cols := []struct {
		title string
		w     float64
		align string
	}{{"No", 10, "C"}, {"Uraian", 74, "L"}, {"Jumlah", 20, "R"}, {"Satuan", 18, "C"}, {"Harga satuan", 26, "R"}, {"Total", 26, "R"}}
	// Spread rounding into the description column so the table spans the width.
	sum := 0.0
	for _, c := range cols {
		sum += c.w
	}
	cols[1].w += width - sum
	drawHeader := func() {
		pdf.SetFont("Helvetica", "B", 9)
		pdf.SetFillColor(240, 240, 240)
		for _, c := range cols {
			pdf.CellFormat(c.w, 7, c.title, "1", 0, c.align, true, 0, "")
		}
		pdf.Ln(-1)
		pdf.SetFont("Helvetica", "", 9)
	}
	drawHeader()
	lineH := 5.0
	for i, it := range q.Items {
		desc := latin1(it.Description)
		lines := pdf.SplitLines([]byte(desc), cols[1].w-2)
		rowH := lineH * float64(maxInt(len(lines), 1))
		if pdf.GetY()+rowH > bottom {
			pdf.AddPage()
			drawHeader()
		}
		x, y := left, pdf.GetY()
		cells := []string{fmt.Sprint(i + 1), "", money.FormatQty(it.QtyMilli), latin1(it.Unit), money.FormatIDR(it.UnitPrice), money.FormatIDR(it.LineTotal)}
		for ci, c := range cols {
			pdf.SetXY(x, y)
			if ci == 1 {
				pdf.MultiCell(c.w, lineH, desc, "LR", "L", false)
				// MultiCell may be shorter than rowH when other cells drive the height; draw the frame explicitly.
				pdf.SetXY(x, y)
				pdf.CellFormat(c.w, rowH, "", "LRB", 0, "L", false, 0, "")
			} else {
				pdf.CellFormat(c.w, rowH, cells[ci], "1", 0, c.align, false, 0, "")
			}
			x += c.w
		}
		pdf.SetXY(left, y+rowH)
	}
	pdf.Ln(2)

	// Totals (kept together on one page)
	ensure(30)
	label := func(name, value string, bold bool) {
		style := ""
		if bold {
			style = "B"
		}
		pdf.SetFont("Helvetica", style, 10)
		pdf.SetX(left + width - 90)
		pdf.CellFormat(50, 6, name, "", 0, "L", false, 0, "")
		pdf.CellFormat(40, 6, value, "", 1, "R", false, 0, "")
	}
	label("Subtotal", money.FormatIDR(q.Subtotal), false)
	if q.DiscountPct > 0 {
		label(fmt.Sprintf("Diskon %d%%", q.DiscountPct), "-"+money.FormatIDR(q.DiscountAmount), false)
	}
	if q.TaxPct > 0 {
		label(fmt.Sprintf("Pajak %d%%", q.TaxPct), money.FormatIDR(q.TaxAmount), false)
	}
	label("TOTAL", money.FormatIDR(q.Total), true)

	if q.Notes != "" {
		notes := encode(tr, q.Notes, true)
		pdf.Ln(4)
		ensure(6 + 5*float64(len(pdf.SplitLines([]byte(notes), width))))
		pdf.SetFont("Helvetica", "B", 10)
		pdf.CellFormat(width, 6, "Catatan", "", 1, "L", false, 0, "")
		pdf.SetFont("Helvetica", "", 10)
		pdf.MultiCell(width, 5, notes, "", "L", false)
	}
	pdf.Ln(6)
	ensure(5)
	pdf.SetFont("Helvetica", "I", 8)
	pdf.SetTextColor(120, 120, 120)
	pdf.CellFormat(width, 5, "Dibuat dengan KerjaLancar. Harga dalam rupiah.", "", 1, "L", false, 0, "")
	return pdf.Output(w)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
