// Package docpdf renders KerjaLancar's A4 business documents (quotations,
// invoices) with go-pdf/fpdf: a header with the workspace and number, the
// recipient and project, an items table with manual page breaks, a totals
// block, free-text notes and a footer. Text is mapped to the core font's
// single-byte code page; dates follow the caller's zone (D-055).
package docpdf

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
)

// Options control rendering. Loc is the zone the caller already applied to
// its dates (kept for the metadata); CreatedAt pins the document timestamps.
type Options struct {
	Compress  bool
	CreatedAt time.Time
	Loc       *time.Location
}

// Line is a label/value pair in the header dates or the totals block.
type Line struct {
	Label string
	Value string
	Bold  bool
}

// Item is one formatted table row.
type Item struct {
	Description string
	Qty         string
	Unit        string
	UnitPrice   string
	LineTotal   string
}

// Document is everything the page shows, already formatted.
type Document struct {
	Kind      string // "PENAWARAN" or "TAGIHAN"
	Title     string // PDF metadata title
	Author    string // workspace name, also the header's right column
	Number    string // "0001"
	Status    string
	Dates     []Line // header right column, e.g. Tanggal, Berlaku sampai, Jatuh tempo
	Recipient string
	Project   string
	Items     []Item
	Totals    []Line
	Notes     string
	Footer    string
}

// WIB is the fallback zone: the westernmost Indonesian zone, as on the share pages.
var WIB = time.FixedZone("WIB", 7*3600)

var months = [...]string{"", "Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}

// Date formats a timestamp as "13 Okt 2026" in loc ("-" when nil).
func Date(t *time.Time, loc *time.Location) string {
	if t == nil {
		return "-"
	}
	if loc == nil {
		loc = WIB
	}
	d := t.In(loc)
	return fmt.Sprintf("%d %s %d", d.Day(), months[d.Month()], d.Year())
}

// DateUTC formats a calendar date stored as UTC midnight (date columns).
func DateUTC(t *time.Time) string { return Date(t, time.UTC) }

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

// Render writes the document. The same bytes may be served to the owner and
// from a public link, so callers never put contact data beyond the name in it.
func Render(w io.Writer, d Document, opts Options) error {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetCompression(opts.Compress)
	if !opts.CreatedAt.IsZero() {
		pdf.SetCreationDate(opts.CreatedAt)
		pdf.SetModificationDate(opts.CreatedAt)
	}
	// The translator keeps an internal buffer, so one per document, never shared.
	tr := pdf.UnicodeTranslatorFromDescriptor("")
	latin1 := func(s string) string { return encode(tr, s, false) }
	pdf.SetTitle(latin1(d.Title), false)
	pdf.SetAuthor(latin1(d.Author), false)
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
	pdf.CellFormat(width/2, 9, latin1(d.Kind), "", 0, "L", false, 0, "")
	pdf.SetFont("Helvetica", "B", 12)
	pdf.CellFormat(width/2, 9, latin1(d.Author), "", 1, "R", false, 0, "")
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetTextColor(90, 90, 90)
	leftLines := []string{"No. " + d.Number, "Status: " + d.Status}
	rows := len(leftLines)
	if len(d.Dates) > rows {
		rows = len(d.Dates)
	}
	for i := 0; i < rows; i++ {
		l, r := "", ""
		if i < len(leftLines) {
			l = leftLines[i]
		}
		if i < len(d.Dates) {
			r = d.Dates[i].Label + ": " + d.Dates[i].Value
		}
		pdf.CellFormat(width/2, 6, latin1(l), "", 0, "L", false, 0, "")
		pdf.CellFormat(width/2, 6, latin1(r), "", 1, "R", false, 0, "")
	}
	pdf.SetTextColor(0, 0, 0)
	pdf.Ln(4)
	pdf.SetDrawColor(200, 200, 200)
	pdf.Line(left, pdf.GetY(), right, pdf.GetY())
	pdf.Ln(4)

	// Recipient and project
	pdf.SetFont("Helvetica", "B", 10)
	pdf.CellFormat(width, 6, "Kepada", "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 10)
	pdf.CellFormat(width, 6, latin1(d.Recipient), "", 1, "L", false, 0, "")
	pdf.CellFormat(width, 6, "Proyek: "+latin1(d.Project), "", 1, "L", false, 0, "")
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
	for i, it := range d.Items {
		desc := latin1(it.Description)
		lines := pdf.SplitLines([]byte(desc), cols[1].w-2)
		rowH := lineH * float64(max(len(lines), 1))
		if pdf.GetY()+rowH > bottom {
			pdf.AddPage()
			drawHeader()
		}
		x, y := left, pdf.GetY()
		cells := []string{fmt.Sprint(i + 1), "", latin1(it.Qty), latin1(it.Unit), latin1(it.UnitPrice), latin1(it.LineTotal)}
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
	ensure(6 * float64(len(d.Totals)+1))
	for _, line := range d.Totals {
		style := ""
		if line.Bold {
			style = "B"
		}
		pdf.SetFont("Helvetica", style, 10)
		pdf.SetX(left + width - 90)
		pdf.CellFormat(50, 6, latin1(line.Label), "", 0, "L", false, 0, "")
		pdf.CellFormat(40, 6, latin1(line.Value), "", 1, "R", false, 0, "")
	}

	if d.Notes != "" {
		notes := encode(tr, d.Notes, true)
		pdf.Ln(4)
		ensure(6 + 5*float64(len(pdf.SplitLines([]byte(notes), width))))
		pdf.SetFont("Helvetica", "B", 10)
		pdf.CellFormat(width, 6, "Catatan", "", 1, "L", false, 0, "")
		pdf.SetFont("Helvetica", "", 10)
		pdf.MultiCell(width, 5, notes, "", "L", false)
	}
	if d.Footer != "" {
		pdf.Ln(6)
		ensure(5)
		pdf.SetFont("Helvetica", "I", 8)
		pdf.SetTextColor(120, 120, 120)
		pdf.CellFormat(width, 5, latin1(d.Footer), "", 1, "L", false, 0, "")
	}
	return pdf.Output(w)
}
