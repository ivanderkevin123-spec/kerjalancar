package web_test

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kerjalancar/internal/web"
)

var followUpID = regexp.MustCompile(`/app/follow-up/([0-9a-f-]{36})/selesai`)

// fixedClock pins "now" so date buckets are deterministic.
func fixedClock(t time.Time) func(d *web.Deps) {
	return func(d *web.Deps) { d.Clock = func() time.Time { return t } }
}

var noon20 = time.Date(2026, 10, 20, 5, 0, 0, 0, time.UTC) // 12:00 WIB on 20 Oct 2026

func TestFollowUpListEmptyStates(t *testing.T) {
	c := newApp(t, fixedClock(noon20)).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	for tab, text := range map[string]string{"hari-ini": "Tidak ada follow-up hari ini", "terlambat": "Tidak ada follow-up yang terlambat", "mendatang": "Belum ada follow-up mendatang", "selesai": "Belum ada follow-up yang selesai"} {
		rec := c.get("/app/follow-up?tab=" + tab)
		require.Equal(t, http.StatusOK, rec.Code, tab)
		assert.Contains(t, rec.Body.String(), text, tab)
	}
	assert.Equal(t, http.StatusOK, c.get("/app/follow-up?tab=apa").Code, "unknown tab falls back to today")
	assert.Contains(t, c.get("/app").Body.String(), `href="/app/follow-up"`, "dashboard links to the list")
}

func TestScheduleFromLeadDetailPlainAndHtmx(t *testing.T) {
	c := newApp(t, fixedClock(noon20)).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	id := c.createLead(t, quickLead("Pagar", "Pak Agus"))
	detail := c.get("/app/prospek/" + id).Body.String()
	assert.Contains(t, detail, "Jadwalkan follow-up")
	assert.Contains(t, detail, `value="2026-10-23"`, "date pre-filled with today + 3")

	plain := c.post("/app/prospek/"+id+"/follow-up", url.Values{"due_on": {"2026-10-23"}, "note": {"telepon siang"}})
	require.Equal(t, http.StatusSeeOther, plain.Code)
	detail = c.get("/app/prospek/" + id).Body.String()
	assert.Contains(t, detail, "Follow-up dijadwalkan.")
	assert.Contains(t, detail, "23 Okt 2026")
	assert.Contains(t, detail, "telepon siang")
	assert.Contains(t, detail, "Follow-up dijadwalkan: 23 Okt 2026")
	assert.Contains(t, detail, "Jadwalkan ulang")

	hx := c.do(http.MethodPost, "/app/prospek/"+id+"/follow-up", url.Values{"due_on": {"2026-10-25"}, "note": {"WA dulu"}}, map[string]string{"HX-Request": "true"})
	require.Equal(t, http.StatusOK, hx.Code)
	frag := hx.Body.String()
	assert.NotContains(t, frag, "<html")
	assert.Contains(t, frag, "Follow-up dijadwalkan ulang.")
	assert.Contains(t, frag, "25 Okt 2026")
	assert.Contains(t, frag, `id="aktivitas" hx-swap-oob="true"`)
	assert.Contains(t, frag, "Follow-up dijadwalkan ulang: 23 Okt 2026 → 25 Okt 2026")
	assert.Contains(t, frag, `id="lead-meta" hx-swap-oob="true"`, "the meta block shows the new date too")

	bad := c.do(http.MethodPost, "/app/prospek/"+id+"/follow-up", url.Values{"due_on": {""}, "note": {"tetap"}}, map[string]string{"HX-Request": "true"})
	assert.Equal(t, http.StatusOK, bad.Code)
	assert.Contains(t, bad.Body.String(), "Tanggal wajib diisi.")
	assert.Contains(t, bad.Body.String(), ">tetap</textarea>")
	far := c.post("/app/prospek/"+id+"/follow-up", url.Values{"due_on": {"2028-01-01"}})
	assert.Contains(t, far.Body.String(), "Tanggal terlalu jauh")
}

func TestListBucketsFollowTheWorkspaceZone(t *testing.T) {
	// 16:59 UTC on 19 Oct: 23:59 WIB, 01:59 WIT.
	app := newApp(t, fixedClock(time.Date(2026, 10, 19, 16, 59, 0, 0, time.UTC)))
	c := app.client(t)
	e := email(t)
	require.Equal(t, http.StatusSeeOther, c.register(e, "A", "W").Code)
	id := c.createLead(t, quickLead("Pagar", "Pak Agus"))
	require.Equal(t, http.StatusSeeOther, c.post("/app/prospek/"+id+"/follow-up", url.Values{"due_on": {"2026-10-20"}}).Code)

	assert.Contains(t, c.get("/app/follow-up?tab=mendatang").Body.String(), "Pagar", "WIB: still tomorrow")
	assert.NotContains(t, c.get("/app/follow-up?tab=hari-ini").Body.String(), "Pagar")

	_, err := app.pool.Exec(context.Background(), `UPDATE workspaces SET timezone = 'Asia/Jayapura' WHERE id = (SELECT m.workspace_id FROM memberships m JOIN users u ON u.id = m.user_id WHERE u.email = $1)`, e)
	require.NoError(t, err)
	today := c.get("/app/follow-up?tab=hari-ini").Body.String()
	assert.Contains(t, today, "Pagar", "WIT: already today")
	assert.NotContains(t, c.get("/app/follow-up?tab=mendatang").Body.String(), "Pagar")
	dash := c.get("/app").Body.String()
	assert.Contains(t, dash, "Pagar")
	assert.Regexp(t, `Hari ini[\s\S]{0,200}>1<`, dash)
}

func TestDoneAndUndoFromTheListAreIdempotentOverHtmx(t *testing.T) {
	c := newApp(t, fixedClock(noon20)).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	id := c.createLead(t, quickLead("Pagar", "Pak Agus"))
	require.Equal(t, http.StatusSeeOther, c.post("/app/prospek/"+id+"/follow-up", url.Values{"due_on": {"2026-10-19"}}).Code)

	list := c.get("/app/follow-up?tab=terlambat").Body.String()
	m := followUpID.FindStringSubmatch(list)
	require.Len(t, m, 2, "overdue row offers Selesai")
	fuID := m[1]
	assert.Contains(t, list, "terlambat")

	done := c.do(http.MethodPost, "/app/follow-up/"+fuID+"/selesai", url.Values{"tampilan": {"daftar"}, "tab": {"terlambat"}}, map[string]string{"HX-Request": "true"})
	require.Equal(t, http.StatusOK, done.Code)
	frag := done.Body.String()
	assert.NotContains(t, frag, "<html")
	assert.Contains(t, frag, "Selesai ditandai.")
	assert.Contains(t, frag, "/app/follow-up/"+fuID+"/batal")
	assert.Contains(t, frag, `id="fu-counts" hx-swap-oob="true"`)

	again := c.do(http.MethodPost, "/app/follow-up/"+fuID+"/selesai", url.Values{"tampilan": {"daftar"}}, map[string]string{"HX-Request": "true"})
	assert.Equal(t, http.StatusOK, again.Code)
	assert.Contains(t, again.Body.String(), "Sudah ditandai selesai.")
	timeline := c.get("/app/prospek/" + id).Body.String()
	assert.Equal(t, 1, strings.Count(timeline, "Follow-up selesai"), "the second tap writes nothing")

	assert.Contains(t, c.get("/app/follow-up?tab=selesai").Body.String(), "Pagar")
	assert.NotContains(t, c.get("/app/follow-up?tab=terlambat").Body.String(), "Pagar")

	undo := c.post("/app/follow-up/"+fuID+"/batal", url.Values{"tampilan": {"daftar"}, "tab": {"selesai"}})
	assert.Equal(t, http.StatusSeeOther, undo.Code)
	assert.Equal(t, "/app/follow-up?tab=selesai", undo.Header().Get("Location"))
	assert.Contains(t, c.get("/app/follow-up?tab=terlambat").Body.String(), "Pagar")
	assert.Contains(t, c.get("/app/prospek/"+id).Body.String(), "Follow-up dibuka kembali")

	undoAgain := c.do(http.MethodPost, "/app/follow-up/"+fuID+"/batal", url.Values{"tampilan": {"daftar"}}, map[string]string{"HX-Request": "true"})
	assert.Contains(t, undoAgain.Body.String(), "Follow-up ini masih terbuka.")
}

func TestDoneFromTheLeadDetailPanel(t *testing.T) {
	c := newApp(t, fixedClock(noon20)).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	id := c.createLead(t, quickLead("Pagar", "Pak Agus"))
	require.Equal(t, http.StatusSeeOther, c.post("/app/prospek/"+id+"/follow-up", url.Values{"due_on": {"2026-10-20"}}).Code)
	detail := c.get("/app/prospek/" + id).Body.String()
	m := followUpID.FindStringSubmatch(detail)
	require.Len(t, m, 2, "the panel offers Selesai")
	assert.Contains(t, detail, "hari ini")

	hx := c.do(http.MethodPost, "/app/follow-up/"+m[1]+"/selesai", url.Values{"tampilan": {"prospek"}}, map[string]string{"HX-Request": "true"})
	require.Equal(t, http.StatusOK, hx.Code)
	assert.Contains(t, hx.Body.String(), "Selesai ditandai.")
	assert.Contains(t, hx.Body.String(), "Jadwalkan follow-up", "the panel offers a new schedule")
	assert.Contains(t, hx.Body.String(), `id="aktivitas" hx-swap-oob="true"`)
	assert.Contains(t, hx.Body.String(), "Follow-up selesai")
}

func TestClosingALeadClosesItsFollowUpInTheUI(t *testing.T) {
	c := newApp(t, fixedClock(noon20)).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	id := c.createLead(t, quickLead("Pagar", "Pak Agus"))
	require.Equal(t, http.StatusSeeOther, c.post("/app/prospek/"+id+"/follow-up", url.Values{"due_on": {"2026-10-20"}}).Code)
	require.Equal(t, http.StatusSeeOther, c.post("/app/prospek/"+id+"/status", url.Values{"status": {"kalah"}, "lost_reason": {"harga"}}).Code)
	detail := c.get("/app/prospek/" + id).Body.String()
	assert.Contains(t, detail, "Follow-up selesai (prospek ditutup)")
	assert.NotContains(t, c.get("/app/follow-up?tab=hari-ini").Body.String(), "Pagar")

	closed := c.get("/app/prospek/" + id).Body.String()
	assert.NotContains(t, closed, "Jadwalkan follow-up", "a closed lead offers no schedule form")
	assert.Contains(t, closed, "Prospek sudah ditutup")
	refused := c.post("/app/prospek/"+id+"/follow-up", url.Values{"due_on": {"2026-10-25"}})
	assert.Equal(t, http.StatusOK, refused.Code)
	assert.Contains(t, refused.Body.String(), "Prospek sudah ditutup")
	doneTab := c.get("/app/follow-up?tab=selesai").Body.String()
	assert.Contains(t, doneTab, "Pagar")
	assert.NotContains(t, doneTab, "Batalkan selesai", "an auto-closed follow-up of a closed lead cannot be reopened")

	require.Equal(t, http.StatusSeeOther, c.post("/app/prospek/"+id+"/status", url.Values{"status": {"survei"}}).Code)
	reopened := c.get("/app/prospek/" + id).Body.String()
	assert.Contains(t, reopened, "Jadwalkan follow-up", "a reopened lead can get a new follow-up")
	assert.NotContains(t, c.get("/app/follow-up?tab=hari-ini").Body.String(), "Pagar")
}

func TestLeadCreateBoundsTheFirstFollowUpDate(t *testing.T) {
	c := newApp(t, fixedClock(noon20)).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	rec := c.post("/app/prospek", url.Values{"customer_mode": {"baru"}, "customer_name": {"Pak Agus"}, "title": {"Pagar"}, "next_follow_up_on": {"2099-01-01"}})
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Tanggal terlalu jauh")
	assert.Contains(t, rec.Body.String(), `value="2099-01-01"`)

	id := c.createLead(t, url.Values{"customer_mode": {"baru"}, "customer_name": {"Pak Agus"}, "title": {"Pagar"}, "next_follow_up_on": {"2026-10-22"}})
	rec = c.post("/app/prospek/"+id, url.Values{"title": {"Pagar depan"}, "next_follow_up_on": {"bukan-tanggal"}})
	assert.Equal(t, http.StatusSeeOther, rec.Code, "the edit form has no date field; a stray value is ignored, not validated")
	assert.Contains(t, c.get("/app/prospek/"+id).Body.String(), "22 Okt 2026")
}

func TestFollowUpRoutesRejectBadAndForeignIds(t *testing.T) {
	app := newApp(t, fixedClock(noon20))
	a, b := app.client(t), app.client(t)
	require.Equal(t, http.StatusSeeOther, a.register(email(t), "A", "Studio A").Code)
	require.Equal(t, http.StatusSeeOther, b.register(email(t), "B", "Studio B").Code)
	id := a.createLead(t, quickLead("Proyek A", "Pelanggan A"))
	require.Equal(t, http.StatusSeeOther, a.post("/app/prospek/"+id+"/follow-up", url.Values{"due_on": {"2026-10-20"}}).Code)
	fuID := followUpID.FindStringSubmatch(a.get("/app/follow-up").Body.String())[1]

	zero := "00000000-0000-0000-0000-000000000000"
	assert.Equal(t, http.StatusNotFound, a.post("/app/follow-up/"+zero+"/selesai", url.Values{}).Code)
	assert.Equal(t, http.StatusNotFound, a.post("/app/follow-up/bukan/batal", url.Values{}).Code)
	assert.Equal(t, http.StatusNotFound, a.post("/app/prospek/"+zero+"/follow-up", url.Values{"due_on": {"2026-10-20"}}).Code)

	for _, path := range []string{"/app/follow-up/" + fuID + "/selesai", "/app/follow-up/" + fuID + "/batal", "/app/prospek/" + id + "/follow-up"} {
		rec := b.do(http.MethodPost, path, url.Values{"due_on": {"2026-12-01"}, "tampilan": {"daftar"}}, nil)
		assert.Equal(t, http.StatusNotFound, rec.Code, path)
		assert.NotContains(t, rec.Body.String(), "Proyek A")
	}
	assert.NotContains(t, b.get("/app/follow-up").Body.String(), "Proyek A")
	assert.Contains(t, a.get("/app/follow-up").Body.String(), "Proyek A", "A's follow-up is still open")
	assert.Equal(t, 1, strings.Count(a.get("/app/prospek/"+id).Body.String(), "Follow-up dijadwalkan"))
}
