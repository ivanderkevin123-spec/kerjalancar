package web_test

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	invoiceLocation  = regexp.MustCompile(`/app/tagihan/([0-9a-f-]{36})$`)
	invoiceShareLink = regexp.MustCompile(`/t/([A-Za-z0-9_-]{22})`)
	paymentKeyField  = regexp.MustCompile(`name="key" value="([A-Za-z0-9_-]{22})"`)
	paymentIDField   = regexp.MustCompile(`/app/tagihan/[0-9a-f-]{36}/pembayaran/([0-9a-f-]{36})/batal`)
)

func invoiceForm(leadID, dueOn string) url.Values {
	return url.Values{
		"lead_id":      {leadID},
		"desc":         {"Kitchen set HPL", "Jasa pasang", ""},
		"qty":          {"2,5", "1", ""},
		"unit":         {"m", "ls", ""},
		"price":        {"1.999", "500.000", ""},
		"discount_pct": {"10"}, "tax_pct": {"11"}, "due_on": {dueOn}, "notes": {"Transfer ke BCA"},
		// tampered fields that must be ignored
		"total": {"1"}, "paid": {"999"}, "balance": {"0"}, "seq": {"99"}, "status": {"issued"}, "share_token": {"xxxxxxxxxxxxxxxxxxxxxx"}, "workspace_id": {"00000000-0000-0000-0000-000000000001"},
	}
}

func (c *client) createInvoice(t *testing.T, form url.Values) string {
	t.Helper()
	rec := c.post("/app/tagihan", form)
	require.Equal(t, http.StatusSeeOther, rec.Code, rec.Body.String())
	m := invoiceLocation.FindStringSubmatch(rec.Header().Get("Location"))
	require.Len(t, m, 2, "redirects to the invoice, got %q", rec.Header().Get("Location"))
	return m[1]
}

// issuedInvoice creates and issues an invoice of Rp504.493 for leadID.
func (c *client) issuedInvoice(t *testing.T, leadID, dueOn string) string {
	t.Helper()
	id := c.createInvoice(t, invoiceForm(leadID, dueOn))
	require.Equal(t, http.StatusSeeOther, c.post("/app/tagihan/"+id+"/terbit", url.Values{}).Code)
	return id
}

// paymentForm reads the fresh idempotency key from the detail page and fills the form.
func (c *client) paymentForm(t *testing.T, id, kind, amount string) url.Values {
	t.Helper()
	body := c.get("/app/tagihan/" + id).Body.String()
	m := paymentKeyField.FindStringSubmatch(body)
	require.Len(t, m, 2, "the detail page carries a payment key")
	return url.Values{"kind": {kind}, "method": {"transfer"}, "amount": {amount}, "received_on": {"2026-10-20"}, "note": {"BCA a.n. Budi"}, "key": {m[1]},
		// tampered
		"invoice_id": {"00000000-0000-0000-0000-000000000001"}, "recorded_by": {"00000000-0000-0000-0000-000000000002"}}
}

func TestInvoiceFromQuotationCreateIssuePayAndHistory(t *testing.T) {
	c := newApp(t, fixedClock(noon20)).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "Studio Uji").Code)
	leadID := c.createLead(t, url.Values{"customer_mode": {"baru"}, "customer_name": {"Budi Santoso"}, "customer_phone": {"081277770101"}, "title": {"Renovasi dapur"}})
	qid := c.createQuotation(t, quotationForm(leadID))
	require.Equal(t, http.StatusSeeOther, c.post("/app/penawaran/"+qid+"/kirim", url.Values{}).Code)
	require.Equal(t, http.StatusSeeOther, c.post("/app/penawaran/"+qid+"/terima", url.Values{}).Code)

	leadPage := c.get("/app/prospek/" + leadID).Body.String()
	assert.Contains(t, leadPage, `href="/app/tagihan/baru?prospek=`+leadID+`&penawaran=`+qid+`"`, "the lead page offers an invoice from the accepted quotation")
	assert.Contains(t, leadPage, `href="/app/tagihan/baru?prospek=`+leadID+`"`, "and a blank one")

	form := c.get("/app/tagihan/baru?prospek=" + leadID + "&penawaran=" + qid)
	require.Equal(t, http.StatusOK, form.Code)
	body := form.Body.String()
	assert.Contains(t, body, "Dari penawaran #0001")
	assert.Contains(t, body, `value="Kitchen set HPL"`, "rows pre-filled from the quotation")
	assert.Contains(t, body, `value="2026-11-03"`, "due date pre-filled two weeks ahead")
	assert.Equal(t, http.StatusNotFound, c.get("/app/tagihan/baru").Code, "an invoice always starts from a lead")
	assert.Equal(t, http.StatusNotFound, c.get("/app/tagihan/baru?prospek="+leadID+"&penawaran=00000000-0000-0000-0000-000000000009").Code)

	f := invoiceForm(leadID, "2026-10-27")
	f.Set("quotation_id", qid)
	id := c.createInvoice(t, f)
	detail := c.get("/app/tagihan/" + id)
	require.Equal(t, http.StatusOK, detail.Code)
	body = detail.Body.String()
	for _, want := range []string{"Tagihan #0001", ">Draf</span>", "Budi Santoso", "Renovasi dapur", "Kitchen set HPL", "Rp4.998", "Rp504.998", "Diskon 10%", "Pajak 11%", "Rp504.493", "27 Okt 2026", "Transfer ke BCA", "Tagihan tersimpan.", "Terbitkan", "Dari penawaran #0001"} {
		assert.Contains(t, body, want)
	}
	assert.NotContains(t, body, ">Terbit</span>", "tampered status ignored")
	assert.NotContains(t, body, "/t/xxxx", "tampered token ignored")
	assert.NotContains(t, body, `name="key"`, "no payment form on a draft")

	require.Equal(t, http.StatusSeeOther, c.post("/app/tagihan/"+id+"/terbit", url.Values{}).Code)
	body = c.get("/app/tagihan/" + id).Body.String()
	assert.Contains(t, body, ">Terbit</span>")
	assert.Contains(t, body, "Tagihan diterbitkan")
	assert.Regexp(t, invoiceShareLink, body)
	assert.Contains(t, body, "bukan bukti transfer", "D-011 copy next to the payment form")
	assert.NotContains(t, body, `href="/app/tagihan/`+id+`/ubah"`, "issued invoices are not edited")

	rec := c.post("/app/tagihan/"+id+"/bayar", c.paymentForm(t, id, "dp", "200.000"))
	require.Equal(t, http.StatusSeeOther, rec.Code, rec.Body.String())
	body = c.get("/app/tagihan/" + id).Body.String()
	for _, want := range []string{"Pembayaran tercatat.", "Sudah dibayar", "Rp200.000", "Sisa", "Rp304.493", ">DP</span>", "Transfer", "20 Okt 2026", "BCA a.n. Budi"} {
		assert.Contains(t, body, want)
	}
	assert.NotContains(t, body, ">Lunas</span>")

	rec = c.post("/app/tagihan/"+id+"/bayar", c.paymentForm(t, id, "pelunasan", "304.493"))
	require.Equal(t, http.StatusSeeOther, rec.Code)
	body = c.get("/app/tagihan/" + id).Body.String()
	assert.Contains(t, body, ">Lunas</span>")
	assert.NotContains(t, body, `name="key"`, "a settled invoice takes no more payments")

	leadPage = c.get("/app/prospek/" + leadID).Body.String()
	for _, want := range []string{"Tagihan #0001 dibuat (Rp504.493)", "Tagihan #0001 diterbitkan (Rp504.493)", "Pembayaran Rp200.000 dicatat untuk tagihan #0001 (DP)", "Pembayaran Rp304.493 dicatat untuk tagihan #0001 (Pelunasan)", `href="/app/tagihan/` + id + `"`, ">Lunas</span>"} {
		assert.Contains(t, leadPage, want)
	}
}

func TestPaymentDoubleSubmitRecordsOnce(t *testing.T) {
	c := newApp(t, fixedClock(noon20)).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	leadID := c.createLead(t, quickLead("Pagar", "Pak Agus"))
	id := c.issuedInvoice(t, leadID, "2026-10-27")
	form := c.paymentForm(t, id, "dp", "200.000")
	require.Equal(t, http.StatusSeeOther, c.post("/app/tagihan/"+id+"/bayar", form).Code)
	first := c.get("/app/tagihan/" + id).Body.String()
	assert.Contains(t, first, "Pembayaran tercatat.")

	rec := c.post("/app/tagihan/"+id+"/bayar", form)
	require.Equal(t, http.StatusSeeOther, rec.Code)
	second := c.get("/app/tagihan/" + id).Body.String()
	assert.Contains(t, second, "Pembayaran ini sudah tercatat.")
	assert.Equal(t, 1, strings.Count(second, "BCA a.n. Budi"), "one payment row")
	assert.Contains(t, second, "Rp304.493")

	form.Set("key", "short")
	rec = c.post("/app/tagihan/"+id+"/bayar", form)
	assert.Equal(t, http.StatusOK, rec.Code, "re-rendered with the error")
	assert.Contains(t, rec.Body.String(), "Formulir kedaluwarsa, muat ulang halaman.")
}

func TestPaymentValidationAndOverpaymentMessages(t *testing.T) {
	c := newApp(t, fixedClock(noon20)).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	leadID := c.createLead(t, quickLead("Pagar", "Pak Agus"))
	id := c.issuedInvoice(t, leadID, "2026-10-27")

	form := c.paymentForm(t, id, "dp", "504.494")
	rec := c.post("/app/tagihan/"+id+"/bayar", form)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Jumlah melebihi sisa tagihan (Rp504.493).")
	assert.Contains(t, rec.Body.String(), `value="504.494"`, "typed amount kept")

	form = c.paymentForm(t, id, "refund", "abc")
	form.Set("received_on", "2026-10-21")
	form.Set("method", "qris")
	form.Set("note", strings.Repeat("a", 301))
	rec = c.post("/app/tagihan/"+id+"/bayar", form)
	assert.Equal(t, http.StatusOK, rec.Code)
	for _, want := range []string{"Jenis pembayaran tidak valid.", "Metode tidak valid.", "Jumlah tidak valid. Contoh: 5.000.000", "Tanggal tidak boleh di masa depan.", "Catatan maksimal 300 karakter."} {
		assert.Contains(t, rec.Body.String(), want)
	}
	assert.Contains(t, c.get("/app/tagihan/"+id).Body.String(), "Rp504.493", "nothing recorded")
	assert.NotContains(t, c.get("/app/tagihan/"+id).Body.String(), "Sudah dibayar")

	// Review M6 #4: a stale form posted after the invoice is settled gets a message, not silence.
	stale := c.paymentForm(t, id, "pelunasan", "504.493")
	require.Equal(t, http.StatusSeeOther, c.post("/app/tagihan/"+id+"/bayar", c.paymentForm(t, id, "pelunasan", "504.493")).Code)
	rec = c.post("/app/tagihan/"+id+"/bayar", stale)
	require.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Contains(t, c.get("/app/tagihan/"+id).Body.String(), "Tagihan ini sudah lunas")

	// Review M6 #3: a key issued for another invoice is a stale form here.
	other := c.issuedInvoice(t, leadID, "2026-10-27")
	foreign := c.paymentForm(t, other, "dp", "1.000")
	third := c.issuedInvoice(t, leadID, "2026-10-27")
	require.Equal(t, http.StatusSeeOther, c.post("/app/tagihan/"+other+"/bayar", foreign).Code)
	rec = c.post("/app/tagihan/"+third+"/bayar", foreign)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Formulir kedaluwarsa, muat ulang halaman.")
	assert.NotContains(t, c.get("/app/tagihan/"+third).Body.String(), "Sudah dibayar")
}

func TestPaymentReversalNeedsAReasonAndRestoresTheBalance(t *testing.T) {
	c := newApp(t, fixedClock(noon20)).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	leadID := c.createLead(t, quickLead("Pagar", "Pak Agus"))
	id := c.issuedInvoice(t, leadID, "2026-10-27")
	require.Equal(t, http.StatusSeeOther, c.post("/app/tagihan/"+id+"/bayar", c.paymentForm(t, id, "dp", "200.000")).Code)
	body := c.get("/app/tagihan/" + id).Body.String()
	m := paymentIDField.FindStringSubmatch(body)
	require.Len(t, m, 2, "the payment row offers a reversal form")
	pid := m[1]

	rec := c.post("/app/tagihan/"+id+"/pembayaran/"+pid+"/batal", url.Values{"reason": {"  "}})
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Alasan wajib diisi")
	assert.Contains(t, c.get("/app/tagihan/"+id).Body.String(), "Rp304.493", "still counted")

	rec = c.post("/app/tagihan/"+id+"/pembayaran/"+pid+"/batal", url.Values{"reason": {"transfer gagal"}})
	require.Equal(t, http.StatusSeeOther, rec.Code)
	body = c.get("/app/tagihan/" + id).Body.String()
	assert.Contains(t, body, "Pembayaran dibatalkan.")
	assert.Contains(t, body, "line-through", "reversed payments stay visible, struck through")
	assert.Contains(t, body, "transfer gagal")
	assert.NotContains(t, body, "/pembayaran/"+pid+"/batal", "no second reversal")
	assert.Contains(t, body, "Sisa")
	assert.Regexp(t, `Sisa</dt><dd[^>]*>Rp504\.493`, body)
	assert.Contains(t, c.get("/app/prospek/"+leadID).Body.String(), "Pembayaran Rp200.000 dibatalkan untuk tagihan #0001")

	assert.Equal(t, http.StatusNotFound, c.post("/app/tagihan/"+id+"/pembayaran/00000000-0000-0000-0000-000000000001/batal", url.Values{"reason": {"x"}}).Code)
}

func TestInvoiceVoidRefusedWithPaymentsThenAllowed(t *testing.T) {
	c := newApp(t, fixedClock(noon20)).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	leadID := c.createLead(t, quickLead("Pagar", "Pak Agus"))
	id := c.issuedInvoice(t, leadID, "2026-10-27")
	require.Equal(t, http.StatusSeeOther, c.post("/app/tagihan/"+id+"/bayar", c.paymentForm(t, id, "dp", "200.000")).Code)
	require.Equal(t, http.StatusSeeOther, c.post("/app/tagihan/"+id+"/batal", url.Values{}).Code)
	body := c.get("/app/tagihan/" + id).Body.String()
	assert.Contains(t, body, "Batalkan pembayarannya dulu")
	assert.Contains(t, body, ">Terbit</span>")

	pid := paymentIDField.FindStringSubmatch(body)[1]
	require.Equal(t, http.StatusSeeOther, c.post("/app/tagihan/"+id+"/pembayaran/"+pid+"/batal", url.Values{"reason": {"salah"}}).Code)
	require.Equal(t, http.StatusSeeOther, c.post("/app/tagihan/"+id+"/batal", url.Values{}).Code)
	body = c.get("/app/tagihan/" + id).Body.String()
	assert.Contains(t, body, ">Dibatalkan</span>")
	assert.Contains(t, body, "Tagihan dibatalkan.")
	assert.NotContains(t, body, `name="key"`)
	assert.Contains(t, c.get("/app/prospek/"+leadID).Body.String(), "Tagihan #0001 dibatalkan")
}

func TestInvoiceDraftEditIssueRulesAndStale(t *testing.T) {
	c := newApp(t, fixedClock(noon20)).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	leadID := c.createLead(t, quickLead("Pagar", "Pak Agus"))
	id := c.createInvoice(t, invoiceForm(leadID, ""))

	rec := c.post("/app/tagihan/"+id+"/terbit", url.Values{})
	require.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Contains(t, c.get("/app/tagihan/"+id).Body.String(), "Isi tanggal jatuh tempo", "cannot issue without a due date")

	edit := c.get("/app/tagihan/" + id + "/ubah")
	require.Equal(t, http.StatusOK, edit.Code)
	version := versionField.FindStringSubmatch(edit.Body.String())[1]
	f := invoiceForm(leadID, "2026-10-27")
	f.Set("version", version)
	f["desc"] = []string{"Hanya satu baris"}
	f["qty"] = []string{"1"}
	f["unit"] = []string{"ls"}
	f["price"] = []string{"1.000.000"}
	f.Set("discount_pct", "0")
	f.Set("tax_pct", "0")
	require.Equal(t, http.StatusSeeOther, c.post("/app/tagihan/"+id, f).Code)
	body := c.get("/app/tagihan/" + id).Body.String()
	assert.Contains(t, body, "Hanya satu baris")
	assert.Contains(t, body, "Rp1.000.000")
	assert.NotContains(t, body, "Kitchen set HPL")

	rec = c.post("/app/tagihan/"+id, f)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "diubah di tempat lain", "stale version")

	rec = c.post("/app/tagihan", url.Values{"lead_id": {leadID}, "desc": {"x", ""}, "qty": {"abc", "2"}, "price": {"1", "1"}, "due_on": {"2026-13-40"}, "tax_pct": {"101"}})
	assert.Equal(t, http.StatusOK, rec.Code)
	for _, want := range []string{"Jumlah tidak valid (contoh: 2,5).", "Uraian wajib diisi.", "Tanggal tidak valid.", "Pajak harus 0–100 persen."} {
		assert.Contains(t, rec.Body.String(), want)
	}

	require.Equal(t, http.StatusSeeOther, c.post("/app/tagihan/"+id+"/terbit", url.Values{}).Code)
	assert.Equal(t, http.StatusSeeOther, c.get("/app/tagihan/"+id+"/ubah").Code, "edit of an issued invoice redirects")
	assert.Contains(t, c.get("/app/tagihan/"+id).Body.String(), "Tagihan yang sudah terbit tidak dapat diubah")
}

func TestInvoiceListChipsSearchAndShareActions(t *testing.T) {
	c := newApp(t, fixedClock(noon20)).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	leadID := c.createLead(t, url.Values{"customer_mode": {"baru"}, "customer_name": {"Budi Santoso"}, "customer_phone": {"081277770101"}, "title": {"Renovasi dapur"}})
	overdue := c.issuedInvoice(t, leadID, "2026-10-19")
	future := c.issuedInvoice(t, leadID, "2026-11-01")
	draft := c.createInvoice(t, invoiceForm(leadID, ""))
	paid := c.issuedInvoice(t, leadID, "2026-10-25")
	require.Equal(t, http.StatusSeeOther, c.post("/app/tagihan/"+paid+"/bayar", c.paymentForm(t, paid, "pelunasan", "504.493")).Code)

	list := c.get("/app/tagihan").Body.String()
	assert.Contains(t, list, "/app/tagihan/"+overdue)
	assert.Contains(t, list, "/app/tagihan/"+future)
	assert.NotContains(t, list, "/app/tagihan/"+draft, "the default view is what is owed")
	assert.NotContains(t, list, "/app/tagihan/"+paid)
	assert.Less(t, strings.Index(list, "/app/tagihan/"+overdue), strings.Index(list, "/app/tagihan/"+future), "overdue first")
	assert.Contains(t, list, ">Terlambat</span>")
	assert.Contains(t, list, "2 tagihan")
	for _, chipText := range []string{"Belum lunas", "Terlambat", "Lunas", "Draf", "Dibatalkan", "Semua"} {
		assert.Contains(t, list, chipText)
	}
	assert.Contains(t, c.get("/app/tagihan?status=lunas").Body.String(), "/app/tagihan/"+paid)
	assert.Contains(t, c.get("/app/tagihan?status=draf").Body.String(), "/app/tagihan/"+draft)
	late := c.get("/app/tagihan?status=terlambat").Body.String()
	assert.Contains(t, late, "/app/tagihan/"+overdue)
	assert.NotContains(t, late, "/app/tagihan/"+future)
	byNumber := c.get("/app/tagihan?status=semua&q=0002").Body.String()
	assert.Contains(t, byNumber, "/app/tagihan/"+future)
	assert.NotContains(t, byNumber, "/app/tagihan/"+overdue)
	assert.Contains(t, c.get("/app/tagihan?status=semua&q=zzz").Body.String(), "Tidak ada tagihan yang cocok")

	body := c.get("/app/tagihan/" + future).Body.String()
	token := invoiceShareLink.FindStringSubmatch(body)[1]
	require.Equal(t, http.StatusSeeOther, c.post("/app/tagihan/"+future+"/tautan", url.Values{"aksi": {"cabut"}}).Code)
	body = c.get("/app/tagihan/" + future).Body.String()
	assert.Contains(t, body, "Tautan dicabut")
	assert.NotContains(t, body, token)
	require.Equal(t, http.StatusSeeOther, c.post("/app/tagihan/"+future+"/tautan", url.Values{"aksi": {"baru"}}).Code)
	body = c.get("/app/tagihan/" + future).Body.String()
	assert.Contains(t, body, "Tautan baru dibuat")
	assert.NotEqual(t, token, invoiceShareLink.FindStringSubmatch(body)[1])
	assert.Equal(t, http.StatusBadRequest, c.post("/app/tagihan/"+future+"/tautan", url.Values{"aksi": {"hapus"}}).Code)
	assert.Equal(t, http.StatusSeeOther, c.post("/app/tagihan/"+draft+"/tautan", url.Values{"aksi": {"baru"}}).Code)
	assert.Contains(t, c.get("/app/tagihan/"+draft).Body.String(), "belum terbit", "drafts have no link")

	pdf := c.get("/app/tagihan/" + overdue + "/pdf")
	require.Equal(t, http.StatusOK, pdf.Code)
	assert.Equal(t, "application/pdf", pdf.Header().Get("Content-Type"))
	assert.Equal(t, `attachment; filename="tagihan-0001.pdf"`, pdf.Header().Get("Content-Disposition"))
	assert.True(t, strings.HasPrefix(pdf.Body.String(), "%PDF-1."))
}

func TestInvoiceCSRFForeignIdsAndMassAssignment(t *testing.T) {
	app := newApp(t, fixedClock(noon20))
	a := app.client(t)
	require.Equal(t, http.StatusSeeOther, a.register(email(t), "A", "Studio A").Code)
	leadID := a.createLead(t, quickLead("Pagar", "Pak Agus"))
	id := a.issuedInvoice(t, leadID, "2026-10-27")
	form := a.paymentForm(t, id, "dp", "200.000")

	// Cross-origin POSTs are refused before anything is written.
	for _, path := range []string{"/app/tagihan", "/app/tagihan/" + id, "/app/tagihan/" + id + "/terbit", "/app/tagihan/" + id + "/batal", "/app/tagihan/" + id + "/bayar", "/app/tagihan/" + id + "/tautan", "/app/tagihan/" + id + "/wa"} {
		rec := a.do(http.MethodPost, path, form, map[string]string{"Origin": "https://evil.example", "Sec-Fetch-Site": "cross-site"})
		assert.Equal(t, http.StatusForbidden, rec.Code, path)
	}
	assert.NotContains(t, a.get("/app/tagihan/"+id).Body.String(), "Sudah dibayar")

	// Another workspace sees nothing and writes nothing.
	b := app.client(t)
	require.Equal(t, http.StatusSeeOther, b.register(email(t), "B", "Studio B").Code)
	assert.Equal(t, http.StatusNotFound, b.get("/app/tagihan/"+id).Code)
	assert.Equal(t, http.StatusNotFound, b.get("/app/tagihan/"+id+"/ubah").Code)
	assert.Equal(t, http.StatusNotFound, b.get("/app/tagihan/"+id+"/pdf").Code)
	assert.Equal(t, http.StatusNotFound, b.get("/app/tagihan/baru?prospek="+leadID).Code)
	assert.Equal(t, http.StatusNotFound, b.post("/app/tagihan", invoiceForm(leadID, "2026-10-27")).Code)
	for _, path := range []string{"/app/tagihan/" + id + "/terbit", "/app/tagihan/" + id + "/batal", "/app/tagihan/" + id + "/bayar", "/app/tagihan/" + id + "/tautan", "/app/tagihan/" + id + "/wa"} {
		assert.Equal(t, http.StatusNotFound, b.post(path, form).Code, path)
	}
	bLead := b.createLead(t, quickLead("Kanopi", "Bu Sri"))
	f := invoiceForm(bLead, "2026-10-27")
	f.Set("quotation_id", "00000000-0000-0000-0000-000000000009")
	assert.Equal(t, http.StatusNotFound, b.post("/app/tagihan", f).Code, "a foreign quotation id is not a source")
	assert.NotContains(t, b.get("/app/tagihan?status=semua").Body.String(), "/app/tagihan/"+id)
	own := a.get("/app/tagihan/" + id).Body.String()
	assert.NotContains(t, own, "Sudah dibayar", "nothing written by B")
	assert.Contains(t, own, ">Terbit</span>")

	// Anonymous requests go to the login page.
	anon := app.client(t)
	rec := anon.get("/app/tagihan/" + id)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/masuk", rec.Header().Get("Location"))
	assert.Equal(t, http.StatusSeeOther, anon.post("/app/tagihan/"+id+"/bayar", form).Code)
}

func TestFollowUpPageAndDashboardShowReceivablesThatReconcile(t *testing.T) {
	c := newApp(t, fixedClock(noon20)).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	leadID := c.createLead(t, url.Values{"customer_mode": {"baru"}, "customer_name": {"Budi Santoso"}, "customer_phone": {"081277770101"}, "title": {"Renovasi dapur"}})
	paid := c.issuedInvoice(t, leadID, "2026-10-15")
	require.Equal(t, http.StatusSeeOther, c.post("/app/tagihan/"+paid+"/bayar", c.paymentForm(t, paid, "pelunasan", "504.493")).Code)
	overdue := c.issuedInvoice(t, leadID, "2026-10-19")
	require.Equal(t, http.StatusSeeOther, c.post("/app/tagihan/"+overdue+"/bayar", c.paymentForm(t, overdue, "dp", "200.000")).Code)
	future := c.issuedInvoice(t, leadID, "2026-11-01")
	_ = future
	c.createInvoice(t, invoiceForm(leadID, ""))

	home := c.get("/app").Body.String()
	// Outstanding: 304.493 (overdue) + 504.493 (future) = 808.986; overdue 1 of 2; received this month 704.493.
	for _, want := range []string{"Piutang", "Rp808.986", "2 tagihan belum lunas", "1 terlambat", "Rp304.493", "Diterima bulan ini", "Rp704.493", "Penawaran per status"} {
		assert.Contains(t, home, want)
	}
	// The receivables list agrees with the card.
	list := c.get("/app/tagihan").Body.String()
	assert.Contains(t, list, "2 tagihan")
	assert.Contains(t, list, "Total sisa: Rp808.986")

	late := c.get("/app/follow-up?tab=terlambat").Body.String()
	assert.Contains(t, late, "Tagihan jatuh tempo")
	assert.Contains(t, late, "/app/tagihan/"+overdue)
	assert.Contains(t, late, "Rp304.493")
	assert.Contains(t, late, "19 Okt 2026")
	assert.NotContains(t, late, "/app/tagihan/"+paid)
	assert.NotContains(t, late, "/app/tagihan/"+future)
	today := c.get("/app/follow-up").Body.String()
	assert.NotContains(t, today, "Tagihan jatuh tempo", "only the Terlambat tab lists overdue invoices")
}

func TestInvoiceWhatsAppFillsBalanceAndLink(t *testing.T) {
	app := newApp(t, fixedClock(noon20))
	c := app.client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "Studio Uji").Code)
	leadID := c.createLead(t, url.Values{"customer_mode": {"baru"}, "customer_name": {"Budi"}, "customer_phone": {"081277770101"}, "title": {"Dapur"}, "estimated_value": {"15.000.000"}})
	id := c.issuedInvoice(t, leadID, "2026-10-27")
	require.Equal(t, http.StatusSeeOther, c.post("/app/tagihan/"+id+"/bayar", c.paymentForm(t, id, "dp", "200.000")).Code)
	token := invoiceShareLink.FindStringSubmatch(c.get("/app/tagihan/" + id).Body.String())[1]

	rec := c.post("/app/tagihan/"+id+"/wa", url.Values{"template": {"pengingat_pelunasan"}})
	require.Equal(t, http.StatusSeeOther, rec.Code)
	loc := rec.Header().Get("Location")
	assert.True(t, strings.HasPrefix(loc, "https://wa.me/6281277770101?text="), loc)
	text, _ := url.QueryUnescape(strings.TrimPrefix(loc, "https://wa.me/6281277770101?text="))
	assert.Contains(t, text, "Rp304.493", "{nilai} is the remaining balance, not the lead estimate")
	assert.NotContains(t, text, "Rp15.000.000")
	assert.Contains(t, c.get("/app/prospek/"+leadID).Body.String(), "Pesan WhatsApp dibuka: Pengingat pelunasan")

	// A template that mentions the link gets the invoice link.
	require.Equal(t, http.StatusSeeOther, c.post("/app/pengaturan/templat/pengingat_dp", url.Values{"name": {"Pengingat DP"}, "body": {"Halo {nama}, sisa {nilai}. Tagihan: {tautan}"}}).Code)
	rec = c.post("/app/tagihan/"+id+"/wa", url.Values{"template": {"pengingat_dp"}})
	require.Equal(t, http.StatusSeeOther, rec.Code)
	text, _ = url.QueryUnescape(strings.TrimPrefix(rec.Header().Get("Location"), "https://wa.me/6281277770101?text="))
	assert.Contains(t, text, "/t/"+token)
	assert.Equal(t, http.StatusNotFound, c.post("/app/tagihan/"+id+"/wa", url.Values{"template": {"nope"}}).Code)
}

// Review M6 #2: the pre-filled form from a quotation may be edited; what the
// owner typed is what is saved (a first-phase bill, a removed row).
func TestInvoiceFromQuotationHonoursEdits(t *testing.T) {
	c := newApp(t, fixedClock(noon20)).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	leadID := c.createLead(t, quickLead("Pagar", "Pak Agus"))
	qid := c.createQuotation(t, quotationForm(leadID))
	require.Equal(t, http.StatusSeeOther, c.post("/app/penawaran/"+qid+"/kirim", url.Values{}).Code)
	require.Equal(t, http.StatusSeeOther, c.post("/app/penawaran/"+qid+"/terima", url.Values{}).Code)

	f := invoiceForm(leadID, "2026-10-27")
	f.Set("quotation_id", qid)
	f["desc"] = []string{"Termin 1: kitchen set (50%)"}
	f["qty"] = []string{"1"}
	f["unit"] = []string{"ls"}
	f["price"] = []string{"250.000"}
	f.Set("discount_pct", "0")
	f.Set("tax_pct", "0")
	id := c.createInvoice(t, f)
	body := c.get("/app/tagihan/" + id).Body.String()
	assert.Contains(t, body, "Termin 1: kitchen set (50%)")
	assert.Contains(t, body, "Rp250.000")
	assert.NotContains(t, body, "Kitchen set HPL", "the quotation rows were not forced back in")
	assert.NotContains(t, body, "Rp504.493")
	assert.Contains(t, body, "Dari penawaran #0001", "provenance kept")

	// Row errors on the edited form are reported, not ignored.
	f["qty"] = []string{"abc"}
	rec := c.post("/app/tagihan", f)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Jumlah tidak valid (contoh: 2,5).")
}
