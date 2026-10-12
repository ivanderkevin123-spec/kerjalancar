package quotation_test

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kerjalancar/internal/quotation"
)

func sampleQuotation() quotation.Quotation {
	valid := date(2026, 10, 31)
	return quotation.Quotation{
		WorkspaceName: "Studio Uji", CustomerName: "Budi Santoso", CustomerPhone: "+6281277770101", LeadTitle: "Renovasi dapur",
		Seq: 7, Status: quotation.StatusSent, ValidUntil: &valid, Notes: "Harga belum termasuk listrik.\nPembayaran DP 50%.",
		DiscountPct: 10, TaxPct: 11, Subtotal: 504_998, DiscountAmount: 50_500, TaxAmount: 49_995, Total: 504_493,
		CreatedAt: time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC),
		Items: []quotation.Item{
			{Position: 0, Description: "Kitchen set HPL dengan rak gantung dan laci soft-close, finishing doff warna putih tulang, ukuran menyesuaikan lokasi", QtyMilli: 2500, Unit: "m", UnitPrice: 1999, LineTotal: 4998},
			{Position: 1, Description: "Jasa pasang → termasuk transport 😊", QtyMilli: 1000, Unit: "ls", UnitPrice: 500_000, LineTotal: 500_000},
		},
	}
}

func TestRenderPDFContainsTheQuotationText(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, quotation.RenderPDF(&buf, sampleQuotation(), quotation.PDFOptions{Compress: false, CreatedAt: time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)}))
	out := buf.String()
	assert.True(t, strings.HasPrefix(out, "%PDF-1."), "starts like a PDF")
	for _, want := range []string{"PENAWARAN", "Studio Uji", "No. 0007", "Budi Santoso", "Renovasi dapur", "Kitchen set HPL", "Jasa pasang", "Rp1.999", "Rp504.998", "Diskon 10%", "Pajak 11%", "Rp504.493", "31 Okt 2026", "Harga belum termasuk listrik"} {
		assert.Contains(t, out, want)
	}
	assert.NotContains(t, out, "😊", "characters outside the core font become ?")
	assert.Contains(t, out, "Jasa pasang ? termasuk transport ?")
	assert.Greater(t, buf.Len(), 2000)
}

// The core fonts are single-byte (cp1252): text must reach them as one byte
// per character, never as UTF-8, or "Café" prints as "CafÃ©" (review M5 #1).
func TestRenderPDFEncodesTextForTheCoreFont(t *testing.T) {
	q := sampleQuotation()
	q.CustomerName = "Café Rénové"
	q.Items[1].Description = "Jasa pasang – termasuk transport 😊"
	var buf bytes.Buffer
	require.NoError(t, quotation.RenderPDF(&buf, q, quotation.PDFOptions{Compress: false}))
	out := buf.String()
	assert.Contains(t, out, "Caf\xe9 R\xe9nov\xe9", "Latin-1 letters are single cp1252 bytes")
	assert.NotContains(t, out, "Caf\xc3\xa9", "no UTF-8 sequence reaches the font")
	assert.Contains(t, out, "Jasa pasang \x96 termasuk transport ?", "en dash is cp1252 0x96; the emoji becomes ?")
}

// The same document is served from the public link (D-052), so it carries no
// contact data beyond what the share page shows (review M5 #2).
func TestRenderPDFOmitsTheCustomerPhone(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, quotation.RenderPDF(&buf, sampleQuotation(), quotation.PDFOptions{Compress: false}))
	assert.NotContains(t, buf.String(), "7777", "phone digits")
	assert.NotContains(t, buf.String(), "0812")
}

// Dates follow the given zone, never the process zone; the status printed is
// the effective one for the given day; notes keep their line breaks (review M5 #3, #12).
func TestRenderPDFUsesTheGivenZoneDayAndKeepsNoteLines(t *testing.T) {
	q := sampleQuotation()
	q.CreatedAt = time.Date(2026, 10, 12, 22, 0, 0, 0, time.UTC) // 05:00 the next day in WIB
	wib := time.FixedZone("WIB", 7*3600)

	var buf bytes.Buffer
	require.NoError(t, quotation.RenderPDF(&buf, q, quotation.PDFOptions{Loc: wib, Today: date(2026, 11, 1)}))
	out := buf.String()
	assert.Contains(t, out, "Tanggal: 13 Okt 2026")
	assert.Contains(t, out, "Status: Kedaluwarsa", "valid until 31 Okt, printed on 1 Nov")
	assert.NotContains(t, out, "listrik. Pembayaran", "the newline in the notes is not flattened to a space")
	assert.Contains(t, out, "Pembayaran DP 50%")

	buf.Reset()
	require.NoError(t, quotation.RenderPDF(&buf, q, quotation.PDFOptions{Loc: time.UTC, Today: date(2026, 10, 20)}))
	assert.Contains(t, buf.String(), "Tanggal: 12 Okt 2026", "UTC zone honoured")
	assert.Contains(t, buf.String(), "Status: Terkirim")

	buf.Reset()
	require.NoError(t, quotation.RenderPDF(&buf, q, quotation.PDFOptions{}))
	assert.Contains(t, buf.String(), "Tanggal: 13 Okt 2026", "no zone given: WIB")
	assert.Contains(t, buf.String(), "Status: Terkirim", "no day given: stored status")
}

func TestRenderPDFIsDeterministicAndHandlesManyRows(t *testing.T) {
	q := sampleQuotation()
	for i := 0; i < 60; i++ {
		q.Items = append(q.Items, quotation.Item{Position: i + 2, Description: strings.Repeat("Baris panjang ", 8), QtyMilli: 1000, UnitPrice: 1000, LineTotal: 1000})
	}
	opts := quotation.PDFOptions{Compress: true, CreatedAt: time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)}
	var a, b bytes.Buffer
	require.NoError(t, quotation.RenderPDF(&a, q, opts))
	require.NoError(t, quotation.RenderPDF(&b, q, opts))
	// fpdf writes its font objects in map order, so bytes may differ between
	// renders; the size and page count must not.
	assert.Equal(t, a.Len(), b.Len(), "same input, same size")
	count := regexp.MustCompile(`/Count (\d+)`)
	pa, pb := count.FindStringSubmatch(a.String()), count.FindStringSubmatch(b.String())
	require.Len(t, pa, 2)
	require.Len(t, pb, 2)
	assert.Equal(t, pa[1], pb[1], "same page count in both renders")
	n, _ := strconv.Atoi(pa[1])
	assert.GreaterOrEqual(t, n, 3, "62 wrapped rows do not fit on two pages")
	assert.LessOrEqual(t, n, 6)
	assert.Equal(t, 1, strings.Count(a.String(), "/Type /Pages"))
}

// Writes a sample file (into the test's temp dir, or PDF_SAMPLE_DIR when set)
// so the layout can be eyeballed; the test itself only checks the write.
func TestWriteSamplePDF(t *testing.T) {
	dir := os.Getenv("PDF_SAMPLE_DIR")
	if dir == "" {
		dir = t.TempDir()
	}
	path := filepath.Join(dir, "penawaran-sample.pdf")
	f, err := os.Create(path) //nolint:gosec // G304: path is the test temp dir or an operator-chosen directory
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })
	require.NoError(t, quotation.RenderPDF(f, sampleQuotation(), quotation.PDFOptions{Compress: true}))
	info, err := f.Stat()
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(1500))
}
