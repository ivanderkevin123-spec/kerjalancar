package web_test

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var locationID = regexp.MustCompile(`/app/pelanggan/([0-9a-f-]{36})$`)

// createCustomer posts the form and returns the new customer's id from the redirect.
func (c *client) createCustomer(t *testing.T, form url.Values) string {
	t.Helper()
	rec := c.post("/app/pelanggan", form)
	require.Equal(t, http.StatusSeeOther, rec.Code, rec.Body.String())
	m := locationID.FindStringSubmatch(rec.Header().Get("Location"))
	require.Len(t, m, 2, "redirects to the detail page, got %q", rec.Header().Get("Location"))
	return m[1]
}

func TestCustomerListEmptyStateAndNav(t *testing.T) {
	c := newApp(t).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	rec := c.get("/app/pelanggan")
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "Belum ada pelanggan")
	assert.Contains(t, body, `href="/app/pelanggan/baru"`)
	assert.Contains(t, body, `href="/app/prospek"`, "nav links to the pipeline")
	assert.NotContains(t, body, "DEMO", "no demo rows")
}

func TestCustomerCreateShowsDetailWithDisplayPhone(t *testing.T) {
	c := newApp(t).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	id := c.createCustomer(t, url.Values{"name": {"Budi Santoso"}, "phone": {"0812-3456-7890"}, "email": {"budi@contoh.id"}, "company": {"PT Maju"}, "notes": {"Tetangga Pak Agus"}})
	rec := c.get("/app/pelanggan/" + id)
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "Budi Santoso")
	assert.Contains(t, body, "0812-3456-7890", "phone shown the Indonesian way")
	assert.Contains(t, body, "PT Maju")
	assert.Contains(t, body, "Tetangga Pak Agus")
	assert.Contains(t, body, "Pelanggan tersimpan.")
	assert.Contains(t, body, "Belum ada prospek untuk pelanggan ini")
	assert.Contains(t, body, `href="/app/prospek/baru?pelanggan=`+id+`"`, "shortcut to record a lead for this customer")

	list := c.get("/app/pelanggan").Body.String()
	assert.Contains(t, list, "Budi Santoso")
	assert.Contains(t, list, `href="/app/pelanggan/`+id+`"`)
}

func TestCustomerFormValidationKeepsInput(t *testing.T) {
	c := newApp(t).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	rec := c.post("/app/pelanggan", url.Values{"name": {""}, "phone": {"12"}, "email": {"bukan-email"}, "company": {"CV Tetap Ada"}})
	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "Nama wajib diisi.")
	assert.Contains(t, body, "Nomor telepon tidak valid")
	assert.Contains(t, body, "Email tidak valid.")
	assert.Contains(t, body, `value="CV Tetap Ada"`)
	assert.Contains(t, body, `value="12"`)
	assert.Contains(t, body, `value="bukan-email"`)

	long := strings.Repeat("n", 151)
	rec = c.post("/app/pelanggan", url.Values{"name": {long}})
	assert.Contains(t, rec.Body.String(), "Nama maksimal 150 karakter.")
}

func TestCustomerDuplicatePhoneNamesTheExistingCustomer(t *testing.T) {
	c := newApp(t).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	first := c.createCustomer(t, url.Values{"name": {"Budi"}, "phone": {"0812 7777 0001"}})
	rec := c.post("/app/pelanggan", url.Values{"name": {"Budi Lagi"}, "phone": {"+62 812 7777 0001"}})
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Nomor ini sudah terdaftar atas nama Budi")
	assert.Contains(t, rec.Body.String(), `href="/app/pelanggan/`+first+`"`)
}

func TestCustomerEditUpdatesAndRecordsNothingElse(t *testing.T) {
	c := newApp(t).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	id := c.createCustomer(t, url.Values{"name": {"Cici"}, "phone": {"081277770002"}})
	form := c.get("/app/pelanggan/" + id + "/ubah")
	require.Equal(t, http.StatusOK, form.Code)
	assert.Contains(t, form.Body.String(), `value="Cici"`)
	assert.Contains(t, form.Body.String(), `value="0812-7777-0002"`, "edit form shows the display format")

	rec := c.post("/app/pelanggan/"+id, url.Values{"name": {"Cici Paramida"}, "phone": {""}, "company": {"Rumah Cici"}})
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/app/pelanggan/"+id, rec.Header().Get("Location"))
	detail := c.get("/app/pelanggan/" + id).Body.String()
	assert.Contains(t, detail, "Cici Paramida")
	assert.Contains(t, detail, "Rumah Cici")
	assert.Contains(t, detail, "Perubahan tersimpan.")
	assert.NotContains(t, detail, "0812-7777-0002", "phone was cleared")
}

func TestCustomerSearchAndPagination(t *testing.T) {
	app := newApp(t)
	c := app.client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	for i := 0; i < 26; i++ {
		c.createCustomer(t, url.Values{"name": {fmt.Sprintf("Pelanggan %02d", i)}})
	}
	c.createCustomer(t, url.Values{"name": {"Dewi Lestari"}, "phone": {"0813 9999 0001"}})

	p1 := c.get("/app/pelanggan").Body.String()
	assert.Contains(t, p1, "Halaman 1 dari 2")
	assert.Contains(t, p1, `href="/app/pelanggan?page=2"`)
	p2 := c.get("/app/pelanggan?page=2").Body.String()
	assert.Contains(t, p2, "Halaman 2 dari 2")
	assert.Contains(t, p2, "Pelanggan 00")

	for _, q := range []string{"dewi", "0813-9999", "+62813"} {
		res := c.get("/app/pelanggan?q=" + url.QueryEscape(q)).Body.String()
		assert.Contains(t, res, "Dewi Lestari", "q=%s", q)
		assert.NotContains(t, res, "Pelanggan 25", "q=%s", q)
	}
	none := c.get("/app/pelanggan?q=zzz").Body.String()
	assert.Contains(t, none, "Tidak ada pelanggan yang cocok")
	assert.Contains(t, none, `value="zzz"`, "search box keeps the query")

	for _, p := range []string{"abc", "0", "-1", "9999"} {
		assert.Equal(t, http.StatusOK, c.get("/app/pelanggan?page="+p).Code, "page=%s", p)
	}
	assert.Equal(t, http.StatusOK, c.get("/app/pelanggan?q="+url.QueryEscape(strings.Repeat("x", 500))).Code)
}

func TestCustomerRoutesRejectBadIds(t *testing.T) {
	c := newApp(t).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	for _, path := range []string{"/app/pelanggan/bukan-uuid", "/app/pelanggan/00000000-0000-0000-0000-000000000000", "/app/pelanggan/bukan-uuid/ubah"} {
		rec := c.get(path)
		assert.Equal(t, http.StatusNotFound, rec.Code, path)
		assert.Contains(t, rec.Body.String(), "Halaman tidak ditemukan")
	}
	assert.Equal(t, http.StatusNotFound, c.post("/app/pelanggan/00000000-0000-0000-0000-000000000000", url.Values{"name": {"x"}}).Code)
}

func TestCustomerMutationsAreAuditLogged(t *testing.T) {
	app, logs := newLoggingApp(t)
	c := app.client(t)
	e := email(t)
	require.Equal(t, http.StatusSeeOther, c.register(e, "A", "W").Code)
	id := c.createCustomer(t, url.Values{"name": {"Rahasia Nama"}, "phone": {"081277770003"}, "notes": {"catatan-rahasia"}})
	out := logs.String()
	assert.Contains(t, out, `"action":"customer.create"`)
	assert.Contains(t, out, `"customer_id":"`+id+`"`)
	assert.Contains(t, out, `"workspace_id"`)
	assert.NotContains(t, out, "Rahasia Nama", "content never reaches the technical audit log")
	assert.NotContains(t, out, "catatan-rahasia")
	assert.NotContains(t, out, "081277770003")
}
