package web_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimezoneSettingRebucketsFollowUps(t *testing.T) {
	// 16:59 UTC on 19 Oct: 23:59 WIB, 01:59 WIT.
	c := newApp(t, fixedClock(time.Date(2026, 10, 19, 16, 59, 0, 0, time.UTC))).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	page := c.get("/app/pengaturan").Body.String()
	assert.Contains(t, page, `<option value="Asia/Jakarta" selected`)
	assert.Contains(t, page, "WITA (Makassar")
	assert.Contains(t, page, `href="/app/pengaturan/templat"`)

	id := c.createLead(t, quickLead("Pagar", "Pak Agus"))
	require.Equal(t, http.StatusSeeOther, c.post("/app/prospek/"+id+"/follow-up", url.Values{"due_on": {"2026-10-20"}}).Code)
	assert.Contains(t, c.get("/app/follow-up?tab=mendatang").Body.String(), "Pagar")

	hx := c.do(http.MethodPost, "/app/pengaturan/zona-waktu", url.Values{"timezone": {"Asia/Jayapura"}}, map[string]string{"HX-Request": "true"})
	require.Equal(t, http.StatusOK, hx.Code)
	assert.Contains(t, hx.Body.String(), "Zona waktu tersimpan.")
	assert.Contains(t, hx.Body.String(), `<option value="Asia/Jayapura" selected`)
	assert.NotContains(t, hx.Body.String(), "<html")

	assert.Contains(t, c.get("/app/follow-up?tab=hari-ini").Body.String(), "Pagar", "the same row is now today's; nothing was rewritten")
	assert.NotContains(t, c.get("/app/follow-up?tab=mendatang").Body.String(), "Pagar")

	bad := c.post("/app/pengaturan/zona-waktu", url.Values{"timezone": {"Europe/Berlin"}})
	assert.Equal(t, http.StatusOK, bad.Code)
	assert.Contains(t, bad.Body.String(), "Zona waktu tidak dikenal.")
	plain := c.post("/app/pengaturan/zona-waktu", url.Values{"timezone": {"Asia/Makassar"}})
	assert.Equal(t, http.StatusSeeOther, plain.Code)
	assert.Contains(t, c.get("/app/pengaturan").Body.String(), `<option value="Asia/Makassar" selected`)
}

func TestTemplateSettingsEditAndPreview(t *testing.T) {
	app := newApp(t)
	a, b := app.client(t), app.client(t)
	require.Equal(t, http.StatusSeeOther, a.register(email(t), "A", "Studio A").Code)
	require.Equal(t, http.StatusSeeOther, b.register(email(t), "B", "Studio B").Code)

	page := a.get("/app/pengaturan/templat")
	require.Equal(t, http.StatusOK, page.Code)
	body := page.Body.String()
	assert.Contains(t, body, "Follow-up penawaran")
	assert.Contains(t, body, "Pengingat DP")
	assert.Contains(t, body, "Pengingat pelunasan")
	assert.Contains(t, body, "Halo Budi, saya dari Studio A.", "preview renders sample data")
	assert.Contains(t, body, "Rp15.000.000")
	assert.Contains(t, body, "{nama}")

	hx := a.do(http.MethodPost, "/app/pengaturan/templat/followup_penawaran", url.Values{"name": {"Tindak lanjut"}, "body": {"Hai {nama}, ini {usaha}. Penawaran {nilai}?"}}, map[string]string{"HX-Request": "true"})
	require.Equal(t, http.StatusOK, hx.Code)
	assert.Contains(t, hx.Body.String(), "Templat tersimpan.")
	assert.Contains(t, hx.Body.String(), "Hai Budi, ini Studio A. Penawaran Rp15.000.000?")
	assert.NotContains(t, hx.Body.String(), "<html")

	bad := a.do(http.MethodPost, "/app/pengaturan/templat/pengingat_dp", url.Values{"name": {""}, "body": {strings.Repeat("x", 1001)}}, map[string]string{"HX-Request": "true"})
	assert.Equal(t, http.StatusOK, bad.Code)
	assert.Contains(t, bad.Body.String(), "Nama templat wajib diisi.")
	assert.Contains(t, bad.Body.String(), "Isi pesan maksimal 1000 karakter.")
	assert.Contains(t, bad.Body.String(), strings.Repeat("x", 50), "input kept")

	assert.Equal(t, http.StatusNotFound, a.post("/app/pengaturan/templat/custom", url.Values{"name": {"x"}, "body": {"y"}}).Code)

	plain := a.post("/app/pengaturan/templat/pengingat_pelunasan", url.Values{"name": {"Pelunasan"}, "body": {"Halo {nama}, mohon pelunasan {nilai}."}})
	assert.Equal(t, http.StatusSeeOther, plain.Code)
	assert.Equal(t, "/app/pengaturan/templat", plain.Header().Get("Location"))
	assert.Contains(t, a.get("/app/pengaturan/templat").Body.String(), "Templat tersimpan.")

	// B's slots are untouched; A's edit is what the WhatsApp link uses.
	assert.Contains(t, b.get("/app/pengaturan/templat").Body.String(), "Halo Budi, saya dari Studio B.")
	assert.NotContains(t, b.get("/app/pengaturan/templat").Body.String(), "Tindak lanjut")
	id := a.createLead(t, url.Values{"customer_mode": {"baru"}, "customer_name": {"Cici"}, "customer_phone": {"081277770606"}, "title": {"Kanopi"}, "estimated_value": {"2.000.000"}})
	rec := a.post("/app/prospek/"+id+"/wa", url.Values{"template": {"followup_penawaran"}})
	require.Equal(t, http.StatusSeeOther, rec.Code)
	text, _ := url.QueryUnescape(strings.TrimPrefix(rec.Header().Get("Location"), "https://wa.me/6281277770606?text="))
	assert.Equal(t, "Hai Cici, ini Studio A. Penawaran Rp2.000.000?", text)
}

func TestCSRFBlocksEveryM4Route(t *testing.T) {
	c := newApp(t, fixedClock(noon20)).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	id := c.createLead(t, url.Values{"customer_mode": {"baru"}, "customer_name": {"Pak Agus"}, "customer_phone": {"081277770707"}, "title": {"Pagar"}})
	require.Equal(t, http.StatusSeeOther, c.post("/app/prospek/"+id+"/follow-up", url.Values{"due_on": {"2026-10-20"}}).Code)
	fuID := followUpID.FindStringSubmatch(c.get("/app/follow-up").Body.String())[1]
	form := url.Values{"due_on": {"2026-12-01"}, "template": {"followup_penawaran"}, "timezone": {"Asia/Jayapura"}, "name": {"X"}, "body": {"Y"}, "tampilan": {"daftar"}}
	for _, path := range []string{"/app/prospek/" + id + "/follow-up", "/app/follow-up/" + fuID + "/selesai", "/app/follow-up/" + fuID + "/batal", "/app/prospek/" + id + "/wa", "/app/pengaturan/zona-waktu", "/app/pengaturan/templat/followup_penawaran"} {
		assert.Equal(t, http.StatusForbidden, c.do(http.MethodPost, path, form, map[string]string{"Sec-Fetch-Site": "cross-site"}).Code, "%s cross-site", path)
		assert.Equal(t, http.StatusForbidden, c.do(http.MethodPost, path, form, map[string]string{"Origin": "https://jahat.example"}).Code, "%s foreign origin", path)
	}
	detail := c.get("/app/prospek/" + id).Body.String()
	assert.Contains(t, detail, "20 Okt 2026", "follow-up untouched")
	assert.NotContains(t, detail, "Pesan WhatsApp dibuka")
	assert.Contains(t, c.get("/app/pengaturan").Body.String(), `<option value="Asia/Jakarta" selected`)
	assert.NotContains(t, c.get("/app/pengaturan/templat").Body.String(), `value="X"`)
}

func TestFollowUpFormsIgnoreForeignFields(t *testing.T) {
	app := newApp(t, fixedClock(noon20))
	c := app.client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	id := c.createLead(t, quickLead("Pagar", "Pak Agus"))
	other := c.createLead(t, quickLead("Teras", "Bu Rina"))
	rec := c.post("/app/prospek/"+id+"/follow-up", url.Values{"due_on": {"2026-10-20"}, "done_at": {"2020-01-01"}, "lead_id": {other}, "workspace_id": {"00000000-0000-0000-0000-000000000001"}, "id": {"00000000-0000-0000-0000-000000000002"}})
	require.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Contains(t, c.get("/app/prospek/"+id).Body.String(), "20 Okt 2026")
	assert.Contains(t, c.get("/app/prospek/"+other).Body.String(), "Jadwalkan follow-up", "the other lead got nothing")
	fuID := followUpID.FindStringSubmatch(c.get("/app/follow-up").Body.String())[1]
	rec = c.post("/app/follow-up/"+fuID+"/selesai", url.Values{"tampilan": {"daftar"}, "lead_id": {other}, "done_by": {"00000000-0000-0000-0000-000000000003"}})
	require.Equal(t, http.StatusSeeOther, rec.Code)
	var doneBy string
	require.NoError(t, app.pool.QueryRow(context.Background(), `SELECT u.email FROM follow_ups f JOIN users u ON u.id = f.done_by WHERE f.id = $1`, fuID).Scan(&doneBy))
	assert.Contains(t, doneBy, "@contoh.id", "done_by is the session user")

	anon := app.client(t)
	for _, path := range []string{"/app/follow-up", "/app/pengaturan/templat"} {
		assert.Equal(t, http.StatusSeeOther, anon.get(path).Code, path)
	}
	for _, path := range []string{"/app/prospek/" + id + "/follow-up", "/app/follow-up/" + fuID + "/batal", "/app/prospek/" + id + "/wa", "/app/pengaturan/zona-waktu", "/app/pengaturan/templat/pengingat_dp"} {
		rec := anon.post(path, url.Values{"due_on": {"2026-10-20"}})
		assert.Equal(t, http.StatusSeeOther, rec.Code, path)
		assert.Equal(t, "/masuk", rec.Header().Get("Location"))
	}
}
