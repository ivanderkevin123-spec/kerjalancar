package web_test

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kerjalancar/internal/web"
)

// Findings of the independent M3 review.

// A client-supplied X-Real-IP must never override the address the trusted
// proxy appended to X-Forwarded-For; many proxies append XFF but pass
// X-Real-IP through untouched.
func TestRateLimitIgnoresClientSuppliedXRealIP(t *testing.T) {
	c := newApp(t, func(d *web.Deps) { d.TrustProxy = true }).client(t)
	attempt := func(realIP string, i int) int {
		return c.do(http.MethodPost, "/masuk", url.Values{"email": {fmt.Sprintf("xr-%d-%s", i, email(t))}, "password": {"salah"}},
			map[string]string{"X-Real-IP": realIP, "X-Forwarded-For": "198.51.100.5, 203.0.113.1"}).Code
	}
	for i := 0; i < 10; i++ {
		require.Equal(t, http.StatusOK, attempt(fmt.Sprintf("203.0.113.%d", 50+i), i), "attempt %d", i+1)
	}
	assert.Equal(t, http.StatusTooManyRequests, attempt("203.0.113.99", 10), "a fresh X-Real-IP must not open a new bucket")
}

// The out-of-band update of the lead meta block must carry hx-swap-oob on
// the element itself; a wrapper would nest one more div on every swap.
func TestStatusFragmentSwapsMetaWithoutNestingWrappers(t *testing.T) {
	c := newApp(t).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	id := c.createLead(t, quickLead("Pagar", "Pak Agus"))
	frag := c.do(http.MethodPost, "/app/prospek/"+id+"/status", url.Values{"status": {"dihubungi"}}, map[string]string{"HX-Request": "true"}).Body.String()
	assert.Regexp(t, `<dl id="lead-meta" hx-swap-oob="true"`, frag)
	assert.NotContains(t, frag, "lead-meta-wrap")
	assert.Equal(t, 1, len(regexp.MustCompile(`id="lead-meta"`).FindAllString(frag, -1)))

	page := c.get("/app/prospek/" + id).Body.String()
	assert.NotContains(t, page, `hx-swap-oob`, "the full page carries no out-of-band markers")
}

// Every new POST route refuses cross-site and foreign-origin requests.
func TestCSRFBlocksEveryM3Route(t *testing.T) {
	app := newApp(t)
	c := app.client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	custID := c.createCustomer(t, url.Values{"name": {"Budi"}})
	leadID := c.createLead(t, url.Values{"customer_mode": {"ada"}, "customer_id": {custID}, "title": {"Pagar"}})
	routes := []string{
		"/app/pelanggan", "/app/pelanggan/" + custID,
		"/app/prospek", "/app/prospek/" + leadID, "/app/prospek/" + leadID + "/status", "/app/prospek/" + leadID + "/catatan",
	}
	form := url.Values{"name": {"Dibajak"}, "title": {"Dibajak"}, "status": {"kalah"}, "body": {"dibajak"}, "customer_mode": {"baru"}, "customer_name": {"X"}}
	for _, path := range routes {
		assert.Equal(t, http.StatusForbidden, c.do(http.MethodPost, path, form, map[string]string{"Sec-Fetch-Site": "cross-site"}).Code, "%s cross-site", path)
		assert.Equal(t, http.StatusForbidden, c.do(http.MethodPost, path, form, map[string]string{"Origin": "https://jahat.example"}).Code, "%s foreign origin", path)
	}
	lead := c.get("/app/prospek/" + leadID).Body.String()
	assert.Contains(t, lead, "Pagar")
	assert.NotContains(t, lead, "Dibajak")
	assert.NotContains(t, lead, "dibajak")
	assert.Contains(t, c.get("/app/pelanggan/"+custID).Body.String(), "Budi")
}

// Extra form fields that are not part of the form must be ignored, never mapped.
func TestMassAssignmentIsIgnored(t *testing.T) {
	app := newApp(t)
	a, b := app.client(t), app.client(t)
	require.Equal(t, http.StatusSeeOther, a.register(email(t), "A", "Studio A").Code)
	require.Equal(t, http.StatusSeeOther, b.register(email(t), "B", "Studio B").Code)
	custA := a.createCustomer(t, url.Values{"name": {"Budi"}})
	custA2 := a.createCustomer(t, url.Values{"name": {"Cici"}})
	custB := b.createCustomer(t, url.Values{"name": {"Milik B"}})
	leadA := a.createLead(t, url.Values{"customer_mode": {"ada"}, "customer_id": {custA}, "title": {"Pagar"}})

	rec := a.post("/app/prospek/"+leadA, url.Values{"title": {"Pagar depan"}, "status": {"menang"}, "lost_reason": {"x"}, "customer_id": {custA2}, "workspace_id": {"00000000-0000-0000-0000-000000000001"}, "id": {custB}})
	require.Equal(t, http.StatusSeeOther, rec.Code)
	page := a.get("/app/prospek/" + leadA).Body.String()
	assert.Contains(t, page, "Pagar depan")
	assert.Contains(t, page, `<option value="baru" selected`, "status cannot be changed through the edit form")
	assert.Contains(t, page, `href="/app/pelanggan/`+custA+`"`, "the customer cannot be changed through the edit form")
	assert.NotContains(t, page, "Cici")

	rec = a.post("/app/pelanggan/"+custA, url.Values{"name": {"Budi Baru"}, "workspace_id": {"00000000-0000-0000-0000-000000000001"}, "id": {custB}})
	require.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Contains(t, a.get("/app/pelanggan/"+custA).Body.String(), "Budi Baru")
	assert.Contains(t, b.get("/app/pelanggan/"+custB).Body.String(), "Milik B", "B's customer untouched")
	assert.NotContains(t, b.get("/app/pelanggan").Body.String(), "Budi Baru")
}

func TestLeadFormIgnoresForeignPreselectedCustomer(t *testing.T) {
	app := newApp(t)
	a, b := app.client(t), app.client(t)
	require.Equal(t, http.StatusSeeOther, a.register(email(t), "A", "Studio A").Code)
	require.Equal(t, http.StatusSeeOther, b.register(email(t), "B", "Studio B").Code)
	custA := a.createCustomer(t, url.Values{"name": {"Pelanggan A"}})
	page := b.get("/app/prospek/baru?pelanggan=" + custA)
	require.Equal(t, http.StatusOK, page.Code)
	assert.NotContains(t, page.Body.String(), "Pelanggan A")
	assert.NotContains(t, page.Body.String(), custA)
	assert.Contains(t, page.Body.String(), `value="baru" checked`)
}
