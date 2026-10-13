package web_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublicQuotationPageNeedsNoSessionAndShowsOnlyTheOffer(t *testing.T) {
	app := newApp(t, fixedClock(noon20))
	c := app.client(t)
	e := email(t)
	require.Equal(t, http.StatusSeeOther, c.register(e, "A", "Studio Uji").Code)
	leadID := c.createLead(t, url.Values{"customer_mode": {"baru"}, "customer_name": {"Budi Santoso"}, "customer_phone": {"081277770101"}, "title": {"Renovasi dapur"}, "notes": {"catatan-internal-rahasia"}})
	id := c.createQuotation(t, quotationForm(leadID))
	anon := app.client(t)
	assert.Equal(t, http.StatusNotFound, anon.get("/p/"+strings.Repeat("a", 22)).Code)

	require.Equal(t, http.StatusSeeOther, c.post("/app/penawaran/"+id+"/kirim", url.Values{}).Code)
	token := shareLink.FindStringSubmatch(c.get("/app/penawaran/" + id).Body.String())[1]

	rec := anon.get("/p/" + token)
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	for _, want := range []string{"Studio Uji", "Penawaran #0001", "Budi Santoso", "Renovasi dapur", "Kitchen set HPL", "Rp4.998", "Rp504.493", "Berlaku sampai", "31 Okt 2026", "Belum termasuk listrik", "/p/" + token + "/pdf"} {
		assert.Contains(t, body, want)
	}
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	assert.Empty(t, rec.Header().Values("Set-Cookie"), "the public page sets no session cookie")
	for _, never := range []string{e, "catatan-internal-rahasia", "/app/", "Keluar", "Pengaturan", "Pak Agus", "081277770101", "0812-7777-0101"} {
		assert.NotContains(t, body, never, "public page leaks %q", never)
	}
	assert.Contains(t, body, "@media print")

	anon.get("/p/" + token)
	owner := c.get("/app/penawaran/" + id).Body.String()
	assert.Contains(t, owner, "Dibuka 2 kali")

	pdf := anon.get("/p/" + token + "/pdf")
	require.Equal(t, http.StatusOK, pdf.Code)
	assert.Equal(t, "application/pdf", pdf.Header().Get("Content-Type"))
	assert.True(t, strings.HasPrefix(pdf.Body.String(), "%PDF-1."))
	assert.Equal(t, "no-store", pdf.Header().Get("Cache-Control"))

	assert.Equal(t, http.StatusNotFound, anon.get("/p/"+token[:21]).Code)
	assert.Equal(t, http.StatusNotFound, anon.get("/p/"+token+"/lain").Code)
}

func TestPublicQuotationPageRefusesExpiredAndRevokedLinks(t *testing.T) {
	app := newApp(t, fixedClock(time.Date(2026, 11, 5, 5, 0, 0, 0, time.UTC))) // after valid_until 2026-10-31
	c := app.client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "Studio Uji").Code)
	leadID := c.createLead(t, quickLead("Pagar", "Pak Agus"))
	id := c.createQuotation(t, quotationForm(leadID))
	require.Equal(t, http.StatusSeeOther, c.post("/app/penawaran/"+id+"/kirim", url.Values{}).Code)
	token := shareLink.FindStringSubmatch(c.get("/app/penawaran/" + id).Body.String())[1]
	anon := app.client(t)
	rec := anon.get("/p/" + token)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "tidak berlaku lagi")
	assert.NotContains(t, rec.Body.String(), "Rp504.493", "an expired offer shows no amounts")
	assert.Equal(t, http.StatusNotFound, anon.get("/p/"+token+"/pdf").Code)
	assert.Contains(t, c.get("/app/penawaran/"+id).Body.String(), "Belum dibuka", "refused views are not counted")
}

func TestPublicQuotationPageIsRateLimited(t *testing.T) {
	app := newApp(t, fixedClock(noon20))
	c := app.client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "Studio Uji").Code)
	leadID := c.createLead(t, quickLead("Pagar", "Pak Agus"))
	id := c.createQuotation(t, quotationForm(leadID))
	require.Equal(t, http.StatusSeeOther, c.post("/app/penawaran/"+id+"/kirim", url.Values{}).Code)
	token := shareLink.FindStringSubmatch(c.get("/app/penawaran/" + id).Body.String())[1]
	anon := app.client(t)
	for i := 0; i < 60; i++ {
		require.Equal(t, http.StatusOK, anon.get("/p/"+token).Code, "request %d", i+1)
	}
	rec := anon.get("/p/" + token)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Contains(t, rec.Body.String(), "Terlalu banyak percobaan")
}

func TestPublicInvoicePageAndPDFShowOnlyTheBill(t *testing.T) {
	app := newApp(t, fixedClock(noon20))
	c := app.client(t)
	e := email(t)
	require.Equal(t, http.StatusSeeOther, c.register(e, "A", "Studio Uji").Code)
	leadID := c.createLead(t, url.Values{"customer_mode": {"baru"}, "customer_name": {"Budi Santoso"}, "customer_phone": {"081277770101"}, "title": {"Renovasi dapur"}, "notes": {"catatan-internal-rahasia"}})
	draft := c.createInvoice(t, invoiceForm(leadID, "2026-10-27"))
	anon := app.client(t)
	assert.Equal(t, http.StatusNotFound, anon.get("/t/"+strings.Repeat("a", 22)).Code)
	assert.Equal(t, http.StatusNotFound, anon.get("/t/short").Code)

	require.Equal(t, http.StatusSeeOther, c.post("/app/tagihan/"+draft+"/terbit", url.Values{}).Code)
	id := draft
	require.Equal(t, http.StatusSeeOther, c.post("/app/tagihan/"+id+"/bayar", c.paymentForm(t, id, "dp", "200.000")).Code)
	token := invoiceShareLink.FindStringSubmatch(c.get("/app/tagihan/" + id).Body.String())[1]

	rec := anon.get("/t/" + token)
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	for _, want := range []string{"Studio Uji", "Tagihan #0001", "Budi Santoso", "Renovasi dapur", "Kitchen set HPL", "Rp4.998", "Rp504.493", "Jatuh tempo", "27 Okt 2026", "Sudah dibayar", "Rp200.000", "Sisa", "Rp304.493", "DP", "20 Okt 2026", "Transfer ke BCA", "/t/" + token + "/pdf"} {
		assert.Contains(t, body, want)
	}
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	assert.Empty(t, rec.Header().Values("Set-Cookie"), "the public page sets no session cookie")
	for _, never := range []string{e, "catatan-internal-rahasia", "BCA a.n. Budi", "/app/", "Keluar", "Pengaturan", "081277770101", "0812-7777-0101"} {
		assert.NotContains(t, body, never, "public page leaks %q", never)
	}
	assert.Contains(t, body, "@media print")

	anon.get("/t/" + token)
	owner := c.get("/app/tagihan/" + id).Body.String()
	assert.Contains(t, owner, "Dibuka 2 kali")

	pdf := anon.get("/t/" + token + "/pdf")
	require.Equal(t, http.StatusOK, pdf.Code)
	assert.Equal(t, "application/pdf", pdf.Header().Get("Content-Type"))
	assert.Equal(t, "no-store", pdf.Header().Get("Cache-Control"))
	assert.True(t, strings.HasPrefix(pdf.Body.String(), "%PDF-1."))
	assert.Equal(t, http.StatusNotFound, anon.get("/t/"+token+"/pdf/x").Code)
}

func TestPublicInvoiceLinkStopsAfterRevokeAndVoid(t *testing.T) {
	app := newApp(t, fixedClock(noon20))
	c := app.client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "Studio Uji").Code)
	leadID := c.createLead(t, quickLead("Pagar", "Pak Agus"))
	anon := app.client(t)

	revoked := c.issuedInvoice(t, leadID, "2026-10-27")
	token := invoiceShareLink.FindStringSubmatch(c.get("/app/tagihan/" + revoked).Body.String())[1]
	require.Equal(t, http.StatusOK, anon.get("/t/"+token).Code)
	require.Equal(t, http.StatusSeeOther, c.post("/app/tagihan/"+revoked+"/tautan", url.Values{"aksi": {"cabut"}}).Code)
	assert.Equal(t, http.StatusNotFound, anon.get("/t/"+token).Code)
	assert.Equal(t, http.StatusNotFound, anon.get("/t/"+token+"/pdf").Code)

	voided := c.issuedInvoice(t, leadID, "2026-10-27")
	token = invoiceShareLink.FindStringSubmatch(c.get("/app/tagihan/" + voided).Body.String())[1]
	require.Equal(t, http.StatusOK, anon.get("/t/"+token).Code)
	require.Equal(t, http.StatusSeeOther, c.post("/app/tagihan/"+voided+"/batal", url.Values{}).Code)
	assert.Equal(t, http.StatusNotFound, anon.get("/t/"+token).Code)
}

func TestPublicInvoicePageIsRateLimited(t *testing.T) {
	app := newApp(t, fixedClock(noon20))
	c := app.client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "Studio Uji").Code)
	leadID := c.createLead(t, quickLead("Pagar", "Pak Agus"))
	id := c.issuedInvoice(t, leadID, "2026-10-27")
	token := invoiceShareLink.FindStringSubmatch(c.get("/app/tagihan/" + id).Body.String())[1]
	anon := app.client(t)
	for i := 0; i < 60; i++ {
		require.Equal(t, http.StatusOK, anon.get("/t/"+token).Code, "request %d", i+1)
	}
	rec := anon.get("/t/" + token)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
}
