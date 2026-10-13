package docpdf_test

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kerjalancar/internal/docpdf"
)

func sample() docpdf.Document {
	created := time.Date(2026, 10, 12, 22, 0, 0, 0, time.UTC) // 13 Okt 05:00 WIB
	due := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	return docpdf.Document{
		Kind: "TAGIHAN", Title: "Tagihan 0001 - Studio Uji", Author: "Studio Uji", Number: "0001", Status: "Terbit",
		Dates:     []docpdf.Line{{Label: "Tanggal", Value: docpdf.Date(&created, docpdf.WIB)}, {Label: "Jatuh tempo", Value: docpdf.DateUTC(&due)}},
		Recipient: "Café Rénové", Project: "Renovasi dapur",
		Items: []docpdf.Item{
			{Description: "Kitchen set HPL", Qty: "2,5", Unit: "m", UnitPrice: "Rp1.999", LineTotal: "Rp4.998"},
			{Description: "Jasa pasang – termasuk transport 😊", Qty: "1", Unit: "ls", UnitPrice: "Rp500.000", LineTotal: "Rp500.000"},
		},
		Totals: []docpdf.Line{{Label: "Subtotal", Value: "Rp504.998"}, {Label: "TOTAL", Value: "Rp504.493", Bold: true}, {Label: "Sudah dibayar", Value: "Rp200.000"}, {Label: "SISA", Value: "Rp304.493", Bold: true}},
		Notes:  "Transfer ke BCA.\nSertakan nomor tagihan.",
		Footer: "Dibuat dengan KerjaLancar. Harga dalam rupiah.",
	}
}

func TestRenderEncodesAndLaysOutTheDocument(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, docpdf.Render(&buf, sample(), docpdf.Options{Compress: false}))
	out := buf.String()
	assert.True(t, strings.HasPrefix(out, "%PDF-1."))
	for _, want := range []string{"TAGIHAN", "Studio Uji", "No. 0001", "Status: Terbit", "Tanggal: 13 Okt 2026", "Jatuh tempo: 27 Okt 2026", "Proyek: Renovasi dapur", "Kitchen set HPL", "Rp4.998", "Subtotal", "TOTAL", "Sudah dibayar", "SISA", "Rp304.493", "Sertakan nomor tagihan", "Dibuat dengan KerjaLancar"} {
		assert.Contains(t, out, want)
	}
	assert.Contains(t, out, "Caf\xe9 R\xe9nov\xe9", "Latin-1 letters are single cp1252 bytes")
	assert.NotContains(t, out, "Caf\xc3\xa9")
	assert.Contains(t, out, "Jasa pasang \x96 termasuk transport ?", "en dash is cp1252 0x96; the emoji becomes ?")
	assert.NotContains(t, out, "BCA. Sertakan", "newlines in notes are kept")
}

func TestRenderBreaksPagesAndIsSizeStable(t *testing.T) {
	d := sample()
	for i := 0; i < 70; i++ {
		d.Items = append(d.Items, docpdf.Item{Description: strings.Repeat("Baris panjang ", 8), Qty: "1", UnitPrice: "Rp1.000", LineTotal: "Rp1.000"})
	}
	opts := docpdf.Options{Compress: true, CreatedAt: time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)}
	var a, b bytes.Buffer
	require.NoError(t, docpdf.Render(&a, d, opts))
	require.NoError(t, docpdf.Render(&b, d, opts))
	assert.Equal(t, a.Len(), b.Len())
	count := regexp.MustCompile(`/Count (\d+)`)
	m := count.FindStringSubmatch(a.String())
	require.Len(t, m, 2)
	n, _ := strconv.Atoi(m[1])
	assert.GreaterOrEqual(t, n, 3)
	assert.Equal(t, 1, strings.Count(a.String(), "/Type /Pages"))
}

func TestDateHelpers(t *testing.T) {
	assert.Equal(t, "-", docpdf.Date(nil, docpdf.WIB))
	ts := time.Date(2026, 1, 31, 20, 0, 0, 0, time.UTC)
	assert.Equal(t, "1 Feb 2026", docpdf.Date(&ts, docpdf.WIB))
	assert.Equal(t, "31 Jan 2026", docpdf.Date(&ts, time.UTC))
	d := time.Date(2026, 12, 5, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, "5 Des 2026", docpdf.DateUTC(&d))
}
