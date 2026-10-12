package web_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Workspace B tries every customer and lead route with workspace A's ids.
// Every attempt must be a 404 and must leave A's data untouched.
func TestTenantMatrixCustomersAndLeads(t *testing.T) {
	app := newApp(t)
	a, b := app.client(t), app.client(t)
	require.Equal(t, http.StatusSeeOther, a.register(email(t), "A", "Studio A").Code)
	require.Equal(t, http.StatusSeeOther, b.register(email(t), "B", "Studio B").Code)
	custA := a.createCustomer(t, url.Values{"name": {"Pelanggan A"}, "phone": {"081277770020"}})
	leadA := a.createLead(t, url.Values{"customer_mode": {"ada"}, "customer_id": {custA}, "title": {"Proyek A"}})

	routes := []struct {
		method, path string
		form         url.Values
	}{
		{"GET", "/app/pelanggan/" + custA, nil},
		{"GET", "/app/pelanggan/" + custA + "/ubah", nil},
		{"POST", "/app/pelanggan/" + custA, url.Values{"name": {"Dibajak"}}},
		{"GET", "/app/prospek/" + leadA, nil},
		{"GET", "/app/prospek/" + leadA + "/ubah", nil},
		{"POST", "/app/prospek/" + leadA, url.Values{"title": {"Dibajak"}}},
		{"POST", "/app/prospek/" + leadA + "/status", url.Values{"status": {"kalah"}, "lost_reason": {"dibajak"}}},
		{"POST", "/app/prospek/" + leadA + "/catatan", url.Values{"body": {"dibajak"}}},
	}
	for _, rt := range routes {
		rec := b.do(rt.method, rt.path, rt.form, nil)
		assert.Equal(t, http.StatusNotFound, rec.Code, "%s %s", rt.method, rt.path)
		assert.NotContains(t, rec.Body.String(), "Pelanggan A", "%s %s leaks data", rt.method, rt.path)
		assert.NotContains(t, rec.Body.String(), "Proyek A", "%s %s leaks data", rt.method, rt.path)
		hx := b.do(rt.method, rt.path, rt.form, map[string]string{"HX-Request": "true"})
		assert.Equal(t, http.StatusNotFound, hx.Code, "htmx %s %s", rt.method, rt.path)
	}

	// Lists and the lead form never show A's rows to B.
	assert.NotContains(t, b.get("/app/pelanggan").Body.String(), "Pelanggan A")
	assert.NotContains(t, b.get("/app/prospek?status=semua").Body.String(), "Proyek A")
	assert.NotContains(t, b.get("/app/prospek/baru").Body.String(), "Pelanggan A")
	assert.NotContains(t, b.get("/app/pelanggan?q=Pelanggan").Body.String(), "Pelanggan A")
	assert.NotContains(t, b.get("/app/prospek?q=Proyek&status=semua").Body.String(), "Proyek A")

	// A's data is intact: original names, status baru, exactly one activity.
	cust := a.get("/app/pelanggan/" + custA).Body.String()
	assert.Contains(t, cust, "Pelanggan A")
	assert.NotContains(t, cust, "Dibajak")
	lead := a.get("/app/prospek/" + leadA).Body.String()
	assert.Contains(t, lead, "Proyek A")
	assert.NotContains(t, lead, "Dibajak")
	assert.NotContains(t, lead, "dibajak")
	assert.NotContains(t, lead, "Status: Baru → Kalah")
	assert.Contains(t, lead, `<option value="baru" selected`)

	// Unauthenticated requests to the new routes go to the login page.
	anon := app.client(t)
	for _, rt := range routes {
		rec := anon.do(rt.method, rt.path, rt.form, nil)
		assert.Equal(t, http.StatusSeeOther, rec.Code, "anonymous %s %s", rt.method, rt.path)
		assert.Equal(t, "/masuk", rec.Header().Get("Location"))
	}
	for _, path := range []string{"/app/pelanggan", "/app/pelanggan/baru", "/app/prospek", "/app/prospek/baru"} {
		assert.Equal(t, http.StatusSeeOther, anon.get(path).Code, path)
	}
}
