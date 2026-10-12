package web_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWhatsAppButtonRendersTheTemplateWithTheCurrentNumber(t *testing.T) {
	app, logs := newLoggingApp(t)
	c := app.client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "Studio Uji").Code)
	custID := c.createCustomer(t, url.Values{"name": {"Budi"}, "phone": {"0812 7777 0101"}})
	id := c.createLead(t, url.Values{"customer_mode": {"ada"}, "customer_id": {custID}, "title": {"Dapur"}, "estimated_value": {"15.000.000"}})

	detail := c.get("/app/prospek/" + id).Body.String()
	assert.Contains(t, detail, `action="/app/prospek/`+id+`/wa"`)
	assert.Contains(t, detail, `hx-boost="false"`, "the WhatsApp form must leave the page, not be fetched by htmx")
	assert.Contains(t, detail, `<option value="followup_penawaran"`)
	assert.Contains(t, detail, "Buka WhatsApp")

	rec := c.post("/app/prospek/"+id+"/wa", url.Values{"template": {"followup_penawaran"}})
	require.Equal(t, http.StatusSeeOther, rec.Code, rec.Body.String())
	loc := rec.Header().Get("Location")
	require.True(t, strings.HasPrefix(loc, "https://wa.me/6281277770101?text="), loc)
	text, err := url.QueryUnescape(strings.TrimPrefix(loc, "https://wa.me/6281277770101?text="))
	require.NoError(t, err)
	assert.Equal(t, "Halo Budi, saya dari Studio Uji. Apakah penawaran untuk proyek senilai Rp15.000.000 sudah sempat dilihat? Jika ada yang perlu disesuaikan, kabari saya ya.", text)
	assert.NotContains(t, loc, "+", "spaces are %20")

	after := c.get("/app/prospek/" + id).Body.String()
	assert.Contains(t, after, "Pesan WhatsApp dibuka: Follow-up penawaran")
	out := logs.String()
	assert.Contains(t, out, `"action":"lead.whatsapp"`)
	assert.NotContains(t, out, "Apakah penawaran", "message content never reaches the log")
	assert.NotContains(t, out, "6281277770101")

	// The customer's number changes afterwards: the next link uses the new number.
	require.Equal(t, http.StatusSeeOther, c.post("/app/pelanggan/"+custID, url.Values{"name": {"Budi"}, "phone": {"0813 2222 0202"}}).Code)
	rec = c.post("/app/prospek/"+id+"/wa", url.Values{"template": {"pengingat_dp"}})
	require.Equal(t, http.StatusSeeOther, rec.Code)
	assert.True(t, strings.HasPrefix(rec.Header().Get("Location"), "https://wa.me/6281322220202?text="), rec.Header().Get("Location"))
	assert.Contains(t, c.get("/app/prospek/"+id).Body.String(), "Pesan WhatsApp dibuka: Pengingat DP")
}

func TestWhatsAppWithoutNumberOrEstimate(t *testing.T) {
	c := newApp(t).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "Studio Uji").Code)
	id := c.createLead(t, quickLead("Teras", "Bu Rina"))
	detail := c.get("/app/prospek/" + id).Body.String()
	assert.Contains(t, detail, "belum punya nomor WhatsApp")
	assert.Contains(t, detail, `/ubah"`, "link to add the number")
	assert.NotContains(t, detail, "Buka WhatsApp")

	rec := c.post("/app/prospek/"+id+"/wa", url.Values{"template": {"followup_penawaran"}})
	assert.Equal(t, http.StatusSeeOther, rec.Code, "redirect back so a refresh never re-posts")
	assert.Equal(t, "/app/prospek/"+id, rec.Header().Get("Location"))
	after := c.get("/app/prospek/" + id).Body.String()
	assert.Contains(t, after, "belum punya nomor WhatsApp")
	assert.NotContains(t, after, "Pesan WhatsApp dibuka")

	// With a number but no estimate: the {nilai} placeholder renders empty and the page says so.
	id2 := c.createLead(t, url.Values{"customer_mode": {"baru"}, "customer_name": {"Pak Agus"}, "customer_phone": {"081277770303"}, "title": {"Pagar"}})
	detail2 := c.get("/app/prospek/" + id2).Body.String()
	assert.Contains(t, detail2, "{nilai}")
	assert.Contains(t, detail2, "belum ada estimasi")
	rec = c.post("/app/prospek/"+id2+"/wa", url.Values{"template": {"followup_penawaran"}})
	require.Equal(t, http.StatusSeeOther, rec.Code)
	text, _ := url.QueryUnescape(strings.TrimPrefix(rec.Header().Get("Location"), "https://wa.me/6281277770303?text="))
	assert.NotContains(t, text, "{nilai}")
	assert.Contains(t, text, "proyek senilai  sudah")
}

func TestWhatsAppRejectsBadTemplatesAndForeignLeads(t *testing.T) {
	app := newApp(t)
	a, b := app.client(t), app.client(t)
	require.Equal(t, http.StatusSeeOther, a.register(email(t), "A", "Studio A").Code)
	require.Equal(t, http.StatusSeeOther, b.register(email(t), "B", "Studio B").Code)
	id := a.createLead(t, url.Values{"customer_mode": {"baru"}, "customer_name": {"Pelanggan A"}, "customer_phone": {"081277770404"}, "title": {"Proyek A"}})

	assert.Equal(t, http.StatusNotFound, a.post("/app/prospek/"+id+"/wa", url.Values{"template": {"custom"}}).Code)
	assert.Equal(t, http.StatusNotFound, a.post("/app/prospek/00000000-0000-0000-0000-000000000000/wa", url.Values{"template": {"followup_penawaran"}}).Code)
	rec := b.post("/app/prospek/"+id+"/wa", url.Values{"template": {"followup_penawaran"}})
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.NotContains(t, rec.Header().Get("Location"), "wa.me")
	assert.NotContains(t, a.get("/app/prospek/"+id).Body.String(), "Pesan WhatsApp dibuka")
}

func TestFollowUpRowOffersWhatsApp(t *testing.T) {
	c := newApp(t, fixedClock(noon20)).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	id := c.createLead(t, url.Values{"customer_mode": {"baru"}, "customer_name": {"Pak Agus"}, "customer_phone": {"081277770505"}, "title": {"Pagar"}})
	require.Equal(t, http.StatusSeeOther, c.post("/app/prospek/"+id+"/follow-up", url.Values{"due_on": {"2026-10-20"}}).Code)
	row := c.get("/app/follow-up").Body.String()
	assert.Contains(t, row, `action="/app/prospek/`+id+`/wa"`)
	assert.Contains(t, row, `name="template" value="followup_penawaran"`)
	assert.Contains(t, row, "WhatsApp")
}
