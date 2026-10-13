package web_test

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	quotationLocation = regexp.MustCompile(`/app/penawaran/([0-9a-f-]{36})$`)
	shareLink         = regexp.MustCompile(`/p/([A-Za-z0-9_-]{22})`)
	versionField      = regexp.MustCompile(`name="version" value="(\d+)"`)
)

func quotationForm(leadID string) url.Values {
	return url.Values{
		"lead_id":      {leadID},
		"desc":         {"Kitchen set HPL", "Jasa pasang", ""},
		"qty":          {"2,5", "1", ""},
		"unit":         {"m", "ls", ""},
		"price":        {"1.999", "500.000", ""},
		"discount_pct": {"10"}, "tax_pct": {"11"}, "valid_until": {"2026-10-31"}, "notes": {"Belum termasuk listrik"},
		// tampered fields that must be ignored
		"total": {"1"}, "subtotal": {"1"}, "seq": {"99"}, "status": {"accepted"}, "share_token": {"xxxxxxxxxxxxxxxxxxxxxx"}, "workspace_id": {"00000000-0000-0000-0000-000000000001"},
	}
}

func (c *client) createQuotation(t *testing.T, form url.Values) string {
	t.Helper()
	rec := c.post("/app/penawaran", form)
	require.Equal(t, http.StatusSeeOther, rec.Code, rec.Body.String())
	m := quotationLocation.FindStringSubmatch(rec.Header().Get("Location"))
	require.Len(t, m, 2, "redirects to the quotation, got %q", rec.Header().Get("Location"))
	return m[1]
}

func TestQuotationCreateFromLeadShowsDetailAndHistory(t *testing.T) {
	c := newApp(t, fixedClock(noon20)).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "Studio Uji").Code)
	leadID := c.createLead(t, url.Values{"customer_mode": {"baru"}, "customer_name": {"Budi Santoso"}, "customer_phone": {"081277770101"}, "title": {"Renovasi dapur"}})

	form := c.get("/app/penawaran/baru?prospek=" + leadID)
	require.Equal(t, http.StatusOK, form.Code)
	assert.Contains(t, form.Body.String(), "Renovasi dapur")
	assert.Contains(t, form.Body.String(), "Budi Santoso")
	assert.Contains(t, form.Body.String(), `name="desc"`)
	assert.Contains(t, form.Body.String(), `value="2026-11-03"`, "valid-until pre-filled two weeks ahead")
	assert.Equal(t, http.StatusNotFound, c.get("/app/penawaran/baru").Code, "a quotation always starts from a lead")

	id := c.createQuotation(t, quotationForm(leadID))
	detail := c.get("/app/penawaran/" + id)
	require.Equal(t, http.StatusOK, detail.Code)
	body := detail.Body.String()
	for _, want := range []string{"Penawaran #0001", "Draf", "Budi Santoso", "Renovasi dapur", "Kitchen set HPL", "2,5", "Rp1.999", "Rp4.998", "Jasa pasang", "Rp504.998", "Diskon 10%", "Rp50.500", "Pajak 11%", "Rp49.995", "Rp504.493", "31 Okt 2026", "Belum termasuk listrik", "Penawaran tersimpan.", "Kirim", "Unduh PDF"} {
		assert.Contains(t, body, want)
	}
	assert.Contains(t, body, ">Draf</span>")
	assert.NotContains(t, body, ">Diterima</span>", "tampered status ignored")
	assert.NotContains(t, body, "/p/xxxx", "tampered token ignored")

	lead := c.get("/app/prospek/" + leadID).Body.String()
	assert.Contains(t, lead, "Penawaran #0001 dibuat (Rp504.493)")
	assert.Contains(t, lead, `href="/app/penawaran/`+id+`"`)
	assert.Contains(t, lead, `href="/app/penawaran/baru?prospek=`+leadID+`"`)

	second := c.createQuotation(t, quotationForm(leadID))
	assert.Contains(t, c.get("/app/penawaran/"+second).Body.String(), "Penawaran #0002")
}

func TestQuotationValidationKeepsEveryRow(t *testing.T) {
	c := newApp(t, fixedClock(noon20)).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	leadID := c.createLead(t, quickLead("Pagar", "Pak Agus"))
	rec := c.post("/app/penawaran", url.Values{"lead_id": {leadID}, "desc": {"Kitchen set", ""}, "qty": {"abc", "2"}, "unit": {"m", ""}, "price": {"1.999", "100"}, "discount_pct": {"101"}, "tax_pct": {"11"}})
	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "Jumlah tidak valid (contoh: 2,5).")
	assert.Contains(t, body, "Uraian wajib diisi.")
	assert.Contains(t, body, "Diskon harus 0–100 persen.")
	assert.Contains(t, body, `value="abc"`)
	assert.Contains(t, body, `value="Kitchen set"`)
	assert.Contains(t, body, `value="100"`)
	assert.Contains(t, body, `value="101"`)
	assert.NotContains(t, c.get("/app/penawaran?status=semua").Body.String(), "#0001")

	empty := c.post("/app/penawaran", url.Values{"lead_id": {leadID}, "desc": {""}, "qty": {""}, "unit": {""}, "price": {""}})
	assert.Contains(t, empty.Body.String(), "Tambahkan minimal satu baris.")
}

func TestQuotationAddRowFragment(t *testing.T) {
	c := newApp(t).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	rec := c.do(http.MethodGet, "/app/penawaran/baris", nil, map[string]string{"HX-Request": "true"})
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "<tr")
	assert.Contains(t, rec.Body.String(), `name="desc"`)
	assert.NotContains(t, rec.Body.String(), "<html")
}

func TestQuotationEditRecomputesAndDetectsStaleVersions(t *testing.T) {
	c := newApp(t, fixedClock(noon20)).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	leadID := c.createLead(t, quickLead("Pagar", "Pak Agus"))
	id := c.createQuotation(t, quotationForm(leadID))

	edit := c.get("/app/penawaran/" + id + "/ubah")
	require.Equal(t, http.StatusOK, edit.Code)
	assert.Contains(t, edit.Body.String(), `value="2,5"`)
	assert.Contains(t, edit.Body.String(), `value="1.999"`)
	assert.Contains(t, edit.Body.String(), ">Belum termasuk listrik</textarea>")
	v := versionField.FindStringSubmatch(edit.Body.String())
	require.Len(t, v, 2)

	rec := c.post("/app/penawaran/"+id, url.Values{"version": {v[1]}, "desc": {"Satu"}, "qty": {"1"}, "unit": {""}, "price": {"100"}})
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	body := c.get("/app/penawaran/" + id).Body.String()
	assert.Contains(t, body, "Perubahan tersimpan.")
	assert.Contains(t, body, "Rp100")
	assert.NotContains(t, body, "Kitchen set")

	stale := c.post("/app/penawaran/"+id, url.Values{"version": {v[1]}, "desc": {"Dua"}, "qty": {"1"}, "unit": {""}, "price": {"200"}})
	assert.Equal(t, http.StatusOK, stale.Code)
	assert.Contains(t, stale.Body.String(), "diubah di tempat lain")
	assert.NotContains(t, c.get("/app/penawaran/"+id).Body.String(), "Dua")

	require.Equal(t, http.StatusSeeOther, c.post("/app/penawaran/"+id+"/terima", url.Values{}).Code)
	locked := c.get("/app/penawaran/" + id + "/ubah")
	assert.Equal(t, http.StatusSeeOther, locked.Code, "accepted quotations redirect away from the editor")
	assert.Contains(t, c.get("/app/penawaran/"+id).Body.String(), "tidak dapat diubah")
}

func TestQuotationSendAcceptRejectFlows(t *testing.T) {
	c := newApp(t, fixedClock(noon20)).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	leadID := c.createLead(t, quickLead("Pagar", "Pak Agus"))
	require.Equal(t, http.StatusSeeOther, c.post("/app/prospek/"+leadID+"/follow-up", url.Values{"due_on": {"2026-10-25"}}).Code)
	id := c.createQuotation(t, quotationForm(leadID))

	sent := c.post("/app/penawaran/"+id+"/kirim", url.Values{})
	assert.Equal(t, http.StatusSeeOther, sent.Code)
	body := c.get("/app/penawaran/" + id).Body.String()
	assert.Contains(t, body, "Terkirim")
	assert.Contains(t, body, "Penawaran ditandai terkirim")
	m := shareLink.FindStringSubmatch(body)
	require.Len(t, m, 2, "share link shown")
	assert.Contains(t, body, "http://example.com/p/"+m[1], "absolute link to copy")
	assert.Contains(t, body, "Cabut tautan")
	assert.Contains(t, body, "Tandai diterima")
	assert.Contains(t, body, "Belum dibuka")
	again := c.post("/app/penawaran/"+id+"/kirim", url.Values{})
	assert.Equal(t, http.StatusSeeOther, again.Code)
	assert.Contains(t, c.get("/app/penawaran/"+id).Body.String(), "Status tidak berubah.")
	assert.Contains(t, c.get("/app/prospek/"+leadID).Body.String(), "Penawaran #0001 dikirim (Rp504.493)")

	require.Equal(t, http.StatusSeeOther, c.post("/app/penawaran/"+id+"/terima", url.Values{}).Code)
	body = c.get("/app/penawaran/" + id).Body.String()
	assert.Contains(t, body, "Diterima")
	assert.Contains(t, body, "Penawaran ditandai diterima; prospek menjadi Menang.")
	lead := c.get("/app/prospek/" + leadID).Body.String()
	assert.Contains(t, lead, `<option value="menang" selected`)
	assert.Contains(t, lead, "Penawaran #0001 diterima (Rp504.493)")
	assert.Contains(t, lead, "Follow-up selesai (prospek ditutup)")
	rej := c.post("/app/penawaran/"+id+"/tolak", url.Values{})
	assert.Equal(t, http.StatusSeeOther, rej.Code)
	assert.Contains(t, c.get("/app/penawaran/"+id).Body.String(), "tidak dapat diubah")

	second := c.createQuotation(t, quotationForm(leadID))
	require.Equal(t, http.StatusSeeOther, c.post("/app/penawaran/"+second+"/tolak", url.Values{}).Code)
	assert.Contains(t, c.get("/app/penawaran/"+second).Body.String(), "Ditolak")
	assert.Contains(t, c.get("/app/prospek/"+leadID).Body.String(), "Penawaran #0002 ditolak")
}

func TestQuotationShareLinkRevokeAndRenew(t *testing.T) {
	app := newApp(t, fixedClock(noon20))
	c := app.client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "Studio Uji").Code)
	leadID := c.createLead(t, quickLead("Pagar", "Pak Agus"))
	id := c.createQuotation(t, quotationForm(leadID))
	require.Equal(t, http.StatusSeeOther, c.post("/app/penawaran/"+id+"/kirim", url.Values{}).Code)
	token := shareLink.FindStringSubmatch(c.get("/app/penawaran/" + id).Body.String())[1]

	anon := app.client(t)
	assert.Equal(t, http.StatusOK, anon.get("/p/"+token).Code)
	require.Equal(t, http.StatusSeeOther, c.post("/app/penawaran/"+id+"/tautan", url.Values{"aksi": {"cabut"}}).Code)
	body := c.get("/app/penawaran/" + id).Body.String()
	assert.Contains(t, body, "Tautan dicabut")
	assert.Contains(t, body, "Buat tautan baru")
	assert.NotContains(t, body, "/p/"+token)
	assert.Equal(t, http.StatusNotFound, anon.get("/p/"+token).Code)

	require.Equal(t, http.StatusSeeOther, c.post("/app/penawaran/"+id+"/tautan", url.Values{"aksi": {"baru"}}).Code)
	fresh := shareLink.FindStringSubmatch(c.get("/app/penawaran/" + id).Body.String())[1]
	assert.NotEqual(t, token, fresh)
	assert.Equal(t, http.StatusOK, anon.get("/p/"+fresh).Code)
	assert.Equal(t, http.StatusNotFound, anon.get("/p/"+token).Code, "the old link stays dead")
	bad := c.post("/app/penawaran/"+id+"/tautan", url.Values{"aksi": {"apa"}})
	assert.Equal(t, http.StatusBadRequest, bad.Code)
}

func TestQuotationListFiltersAndSearch(t *testing.T) {
	c := newApp(t, fixedClock(noon20)).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	leadID := c.createLead(t, quickLead("Pagar", "Pak Agus"))
	assert.Contains(t, c.get("/app/penawaran").Body.String(), "Belum ada penawaran")
	draft := c.createQuotation(t, quotationForm(leadID))
	sent := c.createQuotation(t, quotationForm(leadID))
	require.Equal(t, http.StatusSeeOther, c.post("/app/penawaran/"+sent+"/kirim", url.Values{}).Code)

	all := c.get("/app/penawaran").Body.String()
	assert.Contains(t, all, "#0001")
	assert.Contains(t, all, "#0002")
	assert.Contains(t, all, "Pak Agus")
	assert.Contains(t, all, "Rp504.493")
	onlyDraft := c.get("/app/penawaran?status=draft").Body.String()
	assert.Contains(t, onlyDraft, `href="/app/penawaran/`+draft+`"`)
	assert.NotContains(t, onlyDraft, `href="/app/penawaran/`+sent+`"`)
	byNumber := c.get("/app/penawaran?q=0002").Body.String()
	assert.Contains(t, byNumber, `href="/app/penawaran/`+sent+`"`)
	assert.NotContains(t, byNumber, `href="/app/penawaran/`+draft+`"`)
	assert.Contains(t, c.get("/app/penawaran?q=zzz").Body.String(), "Tidak ada penawaran yang cocok")
	for _, qs := range []string{"status=apa", "page=abc", "page=99"} {
		assert.Equal(t, http.StatusOK, c.get("/app/penawaran?"+qs).Code, qs)
	}

	// After the validity date the sent one is listed as expired.
	later := newApp(t, fixedClock(time.Date(2026, 11, 5, 5, 0, 0, 0, time.UTC)))
	c2 := later.client(t)
	require.Equal(t, http.StatusSeeOther, c2.register(email(t), "B", "W2").Code)
	lead2 := c2.createLead(t, quickLead("Teras", "Bu Rina"))
	q2 := c2.createQuotation(t, quotationForm(lead2))
	require.Equal(t, http.StatusSeeOther, c2.post("/app/penawaran/"+q2+"/kirim", url.Values{}).Code)
	assert.Contains(t, c2.get("/app/penawaran?status=expired").Body.String(), `href="/app/penawaran/`+q2+`"`)
	assert.Contains(t, c2.get("/app/penawaran/"+q2).Body.String(), "Kedaluwarsa")
}

func TestQuotationPDFDownload(t *testing.T) {
	c := newApp(t).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "Studio Uji").Code)
	leadID := c.createLead(t, quickLead("Pagar", "Pak Agus"))
	id := c.createQuotation(t, quotationForm(leadID))
	rec := c.get("/app/penawaran/" + id + "/pdf")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/pdf", rec.Header().Get("Content-Type"))
	assert.Equal(t, `attachment; filename="penawaran-0001.pdf"`, rec.Header().Get("Content-Disposition"))
	assert.True(t, strings.HasPrefix(rec.Body.String(), "%PDF-1."))
	assert.Greater(t, rec.Body.Len(), 1500)
}

func TestQuotationRoutesRejectForeignAndBadIdsAndCSRF(t *testing.T) {
	app := newApp(t, fixedClock(noon20))
	a, b := app.client(t), app.client(t)
	require.Equal(t, http.StatusSeeOther, a.register(email(t), "A", "Studio A").Code)
	require.Equal(t, http.StatusSeeOther, b.register(email(t), "B", "Studio B").Code)
	leadA := a.createLead(t, quickLead("Proyek A", "Pelanggan A"))
	qA := a.createQuotation(t, quotationForm(leadA))
	zero := "00000000-0000-0000-0000-000000000000"

	assert.Equal(t, http.StatusNotFound, b.get("/app/penawaran/baru?prospek="+leadA).Code)
	assert.Equal(t, http.StatusNotFound, b.post("/app/penawaran", quotationForm(leadA)).Code, "another workspace's lead")
	for _, path := range []string{"/app/penawaran/" + qA, "/app/penawaran/" + qA + "/ubah", "/app/penawaran/" + qA + "/pdf", "/app/penawaran/" + zero, "/app/penawaran/bukan"} {
		rec := b.get(path)
		assert.Equal(t, http.StatusNotFound, rec.Code, path)
		assert.NotContains(t, rec.Body.String(), "Proyek A")
	}
	for _, path := range []string{"/app/penawaran/" + qA, "/app/penawaran/" + qA + "/kirim", "/app/penawaran/" + qA + "/terima", "/app/penawaran/" + qA + "/tolak", "/app/penawaran/" + qA + "/tautan"} {
		assert.Equal(t, http.StatusNotFound, b.post(path, url.Values{"version": {"1"}, "desc": {"x"}, "qty": {"1"}, "price": {"1"}, "aksi": {"baru"}}).Code, path)
	}
	assert.Contains(t, a.get("/app/penawaran/"+qA).Body.String(), "Draf", "untouched")
	assert.NotContains(t, b.get("/app/penawaran").Body.String(), "Proyek A")

	form := url.Values{"lead_id": {leadA}, "desc": {"x"}, "qty": {"1"}, "price": {"1"}, "version": {"1"}, "aksi": {"baru"}}
	for _, path := range []string{"/app/penawaran", "/app/penawaran/" + qA, "/app/penawaran/" + qA + "/kirim", "/app/penawaran/" + qA + "/terima", "/app/penawaran/" + qA + "/tolak", "/app/penawaran/" + qA + "/tautan"} {
		assert.Equal(t, http.StatusForbidden, a.do(http.MethodPost, path, form, map[string]string{"Sec-Fetch-Site": "cross-site"}).Code, "%s cross-site", path)
	}
	anon := app.client(t)
	for _, path := range []string{"/app/penawaran", "/app/penawaran/" + qA, "/app/penawaran/" + qA + "/pdf", "/app/penawaran/baru?prospek=" + leadA} {
		rec := anon.get(path)
		assert.Equal(t, http.StatusSeeOther, rec.Code, path)
		assert.Equal(t, "/masuk", rec.Header().Get("Location"))
	}
	assert.Equal(t, http.StatusSeeOther, anon.post("/app/penawaran/"+qA+"/kirim", url.Values{}).Code)
}

func TestWhatsAppTautanUsesTheLatestSharedQuotation(t *testing.T) {
	c := newApp(t, fixedClock(noon20)).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "Studio Uji").Code)
	require.Equal(t, http.StatusSeeOther, c.post("/app/pengaturan/templat/followup_penawaran", url.Values{"name": {"Follow-up"}, "body": {"Halo {nama}, penawaran {nilai}: {tautan}"}}).Code)
	leadID := c.createLead(t, url.Values{"customer_mode": {"baru"}, "customer_name": {"Budi"}, "customer_phone": {"081277770101"}, "title": {"Dapur"}, "estimated_value": {"15.000.000"}})

	rec := c.post("/app/prospek/"+leadID+"/wa", url.Values{"template": {"followup_penawaran"}})
	require.Equal(t, http.StatusSeeOther, rec.Code)
	text, _ := url.QueryUnescape(strings.TrimPrefix(rec.Header().Get("Location"), "https://wa.me/6281277770101?text="))
	assert.Equal(t, "Halo Budi, penawaran Rp15.000.000: ", text, "no link before a quotation is sent")

	id := c.createQuotation(t, quotationForm(leadID))
	require.Equal(t, http.StatusSeeOther, c.post("/app/penawaran/"+id+"/kirim", url.Values{}).Code)
	token := shareLink.FindStringSubmatch(c.get("/app/penawaran/" + id).Body.String())[1]
	rec = c.post("/app/prospek/"+leadID+"/wa", url.Values{"template": {"followup_penawaran"}})
	require.Equal(t, http.StatusSeeOther, rec.Code)
	text, _ = url.QueryUnescape(strings.TrimPrefix(rec.Header().Get("Location"), "https://wa.me/6281277770101?text="))
	assert.Equal(t, "Halo Budi, penawaran Rp15.000.000: http://example.com/p/"+token, text)
	assert.Contains(t, c.get("/app/penawaran/"+id).Body.String(), `action="/app/prospek/`+leadID+`/wa"`, "the quotation page offers the WhatsApp button too")
}
