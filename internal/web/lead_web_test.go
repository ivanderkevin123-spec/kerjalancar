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
	leadLocationID = regexp.MustCompile(`/app/prospek/([0-9a-f-]{36})$`)
	leadLinks      = regexp.MustCompile(`href="/app/prospek/[0-9a-f-]{36}"`)
)

func (c *client) createLead(t *testing.T, form url.Values) string {
	t.Helper()
	rec := c.post("/app/prospek", form)
	require.Equal(t, http.StatusSeeOther, rec.Code, rec.Body.String())
	m := leadLocationID.FindStringSubmatch(rec.Header().Get("Location"))
	require.Len(t, m, 2, "redirects to the lead detail, got %q", rec.Header().Get("Location"))
	return m[1]
}

func quickLead(title, customerName string) url.Values {
	return url.Values{"customer_mode": {"baru"}, "customer_name": {customerName}, "title": {title}}
}

func TestLeadListEmptyState(t *testing.T) {
	c := newApp(t).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	rec := c.get("/app/prospek")
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "Belum ada prospek")
	assert.Contains(t, body, `href="/app/prospek/baru"`)
	assert.Contains(t, body, "Aktif")
	assert.Contains(t, body, "Semua")
}

func TestLeadCreateWithExistingCustomerShowsDetailAndHistory(t *testing.T) {
	c := newApp(t).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	custID := c.createCustomer(t, url.Values{"name": {"Budi Santoso"}, "phone": {"081277770010"}})

	form := c.get("/app/prospek/baru?pelanggan=" + custID)
	require.Equal(t, http.StatusOK, form.Code)
	assert.Contains(t, form.Body.String(), `value="`+custID+`" selected`)
	assert.Contains(t, form.Body.String(), `value="ada" checked`)

	id := c.createLead(t, url.Values{"customer_mode": {"ada"}, "customer_id": {custID}, "title": {"Renovasi dapur"},
		"project_type": {"renovasi"}, "estimated_value": {"15.000.000"}, "source": {"whatsapp"}, "description": {"Dapur 3x4 meter"}, "next_follow_up_on": {"2026-10-20"}})
	rec := c.get("/app/prospek/" + id)
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "Renovasi dapur")
	assert.Contains(t, body, "Budi Santoso")
	assert.Contains(t, body, "Rp15.000.000")
	assert.Contains(t, body, "Baru")
	assert.Contains(t, body, "WhatsApp")
	assert.Contains(t, body, "Dapur 3x4 meter")
	assert.Contains(t, body, "20 Okt 2026")
	assert.Contains(t, body, "Prospek dibuat")
	assert.Contains(t, body, "Prospek tersimpan.")
	assert.Contains(t, body, `href="/app/pelanggan/`+custID+`"`)

	detail := c.get("/app/pelanggan/" + custID).Body.String()
	assert.Contains(t, detail, "Renovasi dapur", "the customer page lists the lead")
}

func TestLeadCreateWithInlineCustomer(t *testing.T) {
	c := newApp(t).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	id := c.createLead(t, url.Values{"customer_mode": {"baru"}, "customer_name": {"Bu Rina"}, "customer_phone": {"0812 7777 0011"}, "title": {"Kamar mandi"}})
	body := c.get("/app/prospek/" + id).Body.String()
	assert.Contains(t, body, "Bu Rina")
	assert.Contains(t, body, "0812-7777-0011")
	assert.Contains(t, c.get("/app/pelanggan").Body.String(), "Bu Rina")
}

func TestLeadFormValidationKeepsEveryInput(t *testing.T) {
	c := newApp(t).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	rec := c.post("/app/prospek", url.Values{"customer_mode": {"baru"}, "customer_name": {""}, "customer_phone": {"12"}, "title": {""}, "estimated_value": {"15jt"}, "description": {"tetap ada"}, "source": {"tiktok"}})
	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "Nama proyek wajib diisi.")
	assert.Contains(t, body, "Nama pelanggan wajib diisi.")
	assert.Contains(t, body, "Nomor telepon tidak valid")
	assert.Contains(t, body, "Nilai estimasi tidak valid. Contoh: 15.000.000")
	assert.Contains(t, body, "Sumber tidak dikenal.")
	assert.Contains(t, body, `value="15jt"`)
	assert.Contains(t, body, `value="12"`)
	assert.Contains(t, body, ">tetap ada</textarea>")
	assert.Empty(t, leadLinks.FindAllString(c.get("/app/prospek?status=semua").Body.String(), -1), "nothing was created")

	rec = c.post("/app/prospek", url.Values{"customer_mode": {"ada"}, "customer_id": {""}, "title": {"Teras"}})
	assert.Contains(t, rec.Body.String(), "Pilih pelanggan.")
}

func TestLeadInlineDuplicatePhoneSelectsTheExistingCustomer(t *testing.T) {
	c := newApp(t).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	custID := c.createCustomer(t, url.Values{"name": {"Budi"}, "phone": {"081277770012"}})
	rec := c.post("/app/prospek", url.Values{"customer_mode": {"baru"}, "customer_name": {"Budi Lagi"}, "customer_phone": {"+62 812-7777-0012"}, "title": {"Teras"}})
	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "sudah terdaftar atas nama Budi")
	assert.Contains(t, body, `value="`+custID+`" selected`)
	assert.Contains(t, body, `value="ada" checked`)
	assert.Contains(t, body, `value="Teras"`, "the lead fields survive")
}

func TestLeadCreateWithAnotherWorkspacesCustomerIsRefused(t *testing.T) {
	app := newApp(t)
	a, b := app.client(t), app.client(t)
	require.Equal(t, http.StatusSeeOther, a.register(email(t), "A", "Studio A").Code)
	require.Equal(t, http.StatusSeeOther, b.register(email(t), "B", "Studio B").Code)
	custA := a.createCustomer(t, url.Values{"name": {"Pelanggan A"}})

	rec := b.post("/app/prospek", url.Values{"customer_mode": {"ada"}, "customer_id": {custA}, "title": {"Curang"}})
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Pelanggan tidak ditemukan.")
	assert.NotContains(t, rec.Body.String(), "Pelanggan A", "B's form never lists A's customers")
	assert.NotContains(t, a.get("/app/prospek?status=semua").Body.String(), "Curang")
	assert.NotContains(t, b.get("/app/prospek?status=semua").Body.String(), "Curang")
}

func TestLeadStatusChangePlainAndHtmx(t *testing.T) {
	c := newApp(t).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	id := c.createLead(t, quickLead("Pagar", "Pak Agus"))
	statusURL := "/app/prospek/" + id + "/status"

	plain := c.post(statusURL, url.Values{"status": {"dihubungi"}})
	assert.Equal(t, http.StatusSeeOther, plain.Code)
	assert.Equal(t, "/app/prospek/"+id, plain.Header().Get("Location"))
	body := c.get("/app/prospek/" + id).Body.String()
	assert.Contains(t, body, "Status diperbarui.")
	assert.Contains(t, body, "Status: Baru → Dihubungi")
	assert.Contains(t, body, `<option value="dihubungi" selected`)

	hx := c.do(http.MethodPost, statusURL, url.Values{"status": {"penawaran"}}, map[string]string{"HX-Request": "true"})
	assert.Equal(t, http.StatusOK, hx.Code)
	frag := hx.Body.String()
	assert.NotContains(t, frag, "<html")
	assert.Contains(t, frag, "Status diperbarui.")
	assert.Contains(t, frag, `id="aktivitas" hx-swap-oob="true"`)
	assert.Contains(t, frag, "Status: Dihubungi → Penawaran")
	assert.Contains(t, frag, `id="lead-badge" hx-swap-oob="true"`)
	assert.Contains(t, frag, "Penawaran")

	same := c.do(http.MethodPost, statusURL, url.Values{"status": {"penawaran"}}, map[string]string{"HX-Request": "true"})
	assert.Equal(t, http.StatusOK, same.Code)
	assert.Contains(t, same.Body.String(), "Status tidak berubah.")

	lost := c.post(statusURL, url.Values{"status": {"kalah"}, "lost_reason": {"harga terlalu tinggi"}})
	assert.Equal(t, http.StatusSeeOther, lost.Code)
	body = c.get("/app/prospek/" + id).Body.String()
	assert.Contains(t, body, "Alasan: harga terlalu tinggi")
	assert.Contains(t, body, "harga terlalu tinggi", "the reason stays visible on the lead")

	flip := c.post(statusURL, url.Values{"status": {"menang"}})
	assert.Equal(t, http.StatusOK, flip.Code)
	assert.Contains(t, flip.Body.String(), "dibuka kembali")

	unknown := c.post(statusURL, url.Values{"status": {"negosiasi"}})
	assert.Equal(t, http.StatusOK, unknown.Code)
	assert.Contains(t, unknown.Body.String(), "Status tidak dikenal.")

	reopen := c.post(statusURL, url.Values{"status": {"survei"}})
	assert.Equal(t, http.StatusSeeOther, reopen.Code)
	closed := c.get("/app/prospek?status=kalah").Body.String()
	assert.NotContains(t, closed, "Pagar")
	assert.Contains(t, c.get("/app/prospek?status=survei").Body.String(), "Pagar")
}

func TestLeadNoteHtmxAndPlain(t *testing.T) {
	c := newApp(t).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	id := c.createLead(t, quickLead("Plafon", "Pak Agus"))
	noteURL := "/app/prospek/" + id + "/catatan"

	hx := c.do(http.MethodPost, noteURL, url.Values{"body": {"Telepon lagi Senin"}}, map[string]string{"HX-Request": "true"})
	assert.Equal(t, http.StatusOK, hx.Code)
	frag := hx.Body.String()
	assert.NotContains(t, frag, "<html")
	assert.Contains(t, frag, "Catatan tersimpan.")
	assert.Contains(t, frag, `id="aktivitas" hx-swap-oob="true"`)
	assert.Contains(t, frag, "Telepon lagi Senin")
	assert.Contains(t, frag, "autofocus", "focus returns to the note box after the swap")
	assert.NotRegexp(t, `(?s)<textarea[^>]*>[^<]*Telepon lagi Senin`, frag, "the box is cleared")

	empty := c.do(http.MethodPost, noteURL, url.Values{"body": {"  "}}, map[string]string{"HX-Request": "true"})
	assert.Equal(t, http.StatusOK, empty.Code)
	assert.Contains(t, empty.Body.String(), "Catatan wajib diisi.")

	plain := c.post(noteURL, url.Values{"body": {"Sudah kirim foto"}})
	assert.Equal(t, http.StatusSeeOther, plain.Code)
	body := c.get("/app/prospek/" + id).Body.String()
	assert.Contains(t, body, "Catatan tersimpan.")
	assert.Contains(t, body, "Sudah kirim foto")
	assert.Contains(t, body, "Telepon lagi Senin")
}

func TestLeadEditRecordsChanges(t *testing.T) {
	c := newApp(t).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	id := c.createLead(t, url.Values{"customer_mode": {"baru"}, "customer_name": {"Pak Agus"}, "title": {"Gudang"}, "estimated_value": {"10.000.000"}, "next_follow_up_on": {"2026-11-01"}})
	created := c.get("/app/prospek/" + id).Body.String()
	assert.Contains(t, created, "1 Nov 2026", "a date on the create form schedules the first follow-up")
	assert.Contains(t, created, "Follow-up dijadwalkan: 1 Nov 2026")
	form := c.get("/app/prospek/" + id + "/ubah")
	require.Equal(t, http.StatusOK, form.Code)
	assert.Contains(t, form.Body.String(), `value="Gudang"`)
	assert.Contains(t, form.Body.String(), `value="10.000.000"`)
	assert.NotContains(t, form.Body.String(), `name="next_follow_up_on"`, "follow-ups are scheduled from the detail page, not the edit form")
	assert.NotContains(t, form.Body.String(), "customer_mode", "the customer is not edited here")

	rec := c.post("/app/prospek/"+id, url.Values{"title": {"Gudang belakang"}, "estimated_value": {"12000000"}, "source": {"instagram"}, "next_follow_up_on": {""}})
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	body := c.get("/app/prospek/" + id).Body.String()
	assert.Contains(t, body, "Gudang belakang")
	assert.Contains(t, body, "Rp12.000.000")
	assert.Contains(t, body, "Perubahan tersimpan.")
	assert.Contains(t, body, "1 Nov 2026", "editing the lead leaves its follow-up alone")
	assert.Contains(t, body, "Diperbarui: Nama proyek (Gudang → Gudang belakang), Nilai estimasi (Rp10.000.000 → Rp12.000.000), Sumber (– → Instagram)")

	bad := c.post("/app/prospek/"+id, url.Values{"title": {""}, "estimated_value": {"abc"}})
	assert.Equal(t, http.StatusOK, bad.Code)
	assert.Contains(t, bad.Body.String(), "Nama proyek wajib diisi.")
	assert.Contains(t, bad.Body.String(), `value="abc"`)
}

func TestLeadListFiltersSortAndSearch(t *testing.T) {
	c := newApp(t).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	cheap := c.createLead(t, url.Values{"customer_mode": {"baru"}, "customer_name": {"Dewi"}, "title": {"Cat ulang"}, "estimated_value": {"2.000.000"}})
	expensive := c.createLead(t, url.Values{"customer_mode": {"baru"}, "customer_name": {"Hendra"}, "title": {"Rumah dua lantai"}, "estimated_value": {"900.000.000"}})
	won := c.createLead(t, url.Values{"customer_mode": {"baru"}, "customer_name": {"Lina"}, "title": {"Kitchen set"}, "estimated_value": {"30.000.000"}})
	require.Equal(t, http.StatusSeeOther, c.post("/app/prospek/"+won+"/status", url.Values{"status": {"menang"}}).Code)

	def := c.get("/app/prospek").Body.String()
	assert.Contains(t, def, "Cat ulang")
	assert.NotContains(t, def, "Kitchen set", "won leads leave the active view")
	assert.Contains(t, def, "2 prospek")

	all := c.get("/app/prospek?status=semua").Body.String()
	assert.Contains(t, all, "Kitchen set")
	assert.Contains(t, all, "Menang")

	onlyWon := c.get("/app/prospek?status=menang").Body.String()
	assert.Contains(t, onlyWon, "Kitchen set")
	assert.NotContains(t, onlyWon, "Cat ulang")

	byValue := c.get("/app/prospek?status=semua&urut=nilai").Body.String()
	assert.Less(t, strings.Index(byValue, expensive), strings.Index(byValue, cheap), "highest estimate first")

	byTitle := c.get("/app/prospek?q=lantai&status=semua").Body.String()
	assert.Contains(t, byTitle, "Rumah dua lantai")
	assert.NotContains(t, byTitle, "Cat ulang")
	assert.Contains(t, byTitle, `value="lantai"`)

	byCustomer := c.get("/app/prospek?q=dewi&status=semua").Body.String()
	assert.Contains(t, byCustomer, "Cat ulang")
	assert.NotContains(t, byCustomer, "Kitchen set")

	none := c.get("/app/prospek?q=zzz").Body.String()
	assert.Contains(t, none, "Tidak ada prospek yang cocok")

	for _, qs := range []string{"status=apa", "urut=apa", "page=abc", "page=99", "q=" + url.QueryEscape(strings.Repeat("x", 400))} {
		assert.Equal(t, http.StatusOK, c.get("/app/prospek?"+qs).Code, qs)
	}
}

func TestLeadRoutesRejectBadIds(t *testing.T) {
	c := newApp(t).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	zero := "00000000-0000-0000-0000-000000000000"
	for _, path := range []string{"/app/prospek/bukan-uuid", "/app/prospek/" + zero, "/app/prospek/" + zero + "/ubah"} {
		assert.Equal(t, http.StatusNotFound, c.get(path).Code, path)
	}
	assert.Equal(t, http.StatusNotFound, c.post("/app/prospek/"+zero+"/status", url.Values{"status": {"kalah"}}).Code)
	assert.Equal(t, http.StatusNotFound, c.post("/app/prospek/"+zero+"/catatan", url.Values{"body": {"x"}}).Code)
	assert.Equal(t, http.StatusNotFound, c.post("/app/prospek/"+zero, url.Values{"title": {"x"}}).Code)
}

func TestDashboardShowsPipelineSummary(t *testing.T) {
	c := newApp(t).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	empty := c.get("/app").Body.String()
	assert.Contains(t, empty, "Belum ada data")
	assert.Contains(t, empty, `href="/app/prospek/baru"`)

	c.createLead(t, url.Values{"customer_mode": {"baru"}, "customer_name": {"Dewi"}, "title": {"Cat ulang"}, "estimated_value": {"2.000.000"}})
	c.createLead(t, url.Values{"customer_mode": {"baru"}, "customer_name": {"Hendra"}, "title": {"Teras"}, "estimated_value": {"13.000.000"}})
	body := c.get("/app").Body.String()
	assert.NotContains(t, body, "Belum ada data")
	assert.Contains(t, body, "Prospek aktif")
	assert.Contains(t, body, "Rp15.000.000")
	assert.Contains(t, body, "Catat prospek")
}
