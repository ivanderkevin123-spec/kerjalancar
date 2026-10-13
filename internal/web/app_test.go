package web_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kerjalancar/internal/auth"
	"kerjalancar/internal/customer"
	"kerjalancar/internal/db/testdb"
	"kerjalancar/internal/followup"
	"kerjalancar/internal/lead"
	"kerjalancar/internal/quotation"
	"kerjalancar/internal/template"
	"kerjalancar/internal/web"
	"kerjalancar/internal/workspace"
)

var lightArgon = auth.Params{MemoryKiB: 8192, Time: 1, Threads: 1}

type testApp struct {
	h    http.Handler
	pool *pgxpool.Pool
}

// newApp builds the full router against the test database. Each call gets its
// own session manager and rate limiters, so tests do not share counters.
func newApp(t *testing.T, mutate ...func(d *web.Deps)) *testApp {
	t.Helper()
	pool := testdb.Pool(t)
	tmpl, err := web.LoadTemplates()
	require.NoError(t, err)
	leads := lead.NewStore(pool)
	leads.ScheduleHook = followup.ScheduleForLead
	leads.CloseHook = followup.CloseForLead
	d := web.Deps{
		Logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		DB:               pool,
		Sessions:         web.NewSessionManager(pool, false, 0),
		Auth:             auth.NewService(pool, lightArgon),
		Workspaces:       workspace.NewStore(pool),
		Customers:        customer.NewStore(pool),
		Leads:            leads,
		FollowUps:        followup.NewStore(pool),
		MessageTemplates: template.NewStore(pool),
		Quotations:       quotation.NewStore(pool, leads),
		Templates:        tmpl,
		RegistrationOpen: true,
		BaseURL:          "http://example.com",
	}
	for _, m := range mutate {
		m(&d)
	}
	return &testApp{h: web.NewRouter(d), pool: pool}
}

// client keeps cookies between requests like a browser would.
type client struct {
	t       *testing.T
	app     *testApp
	cookies map[string]*http.Cookie
}

func (a *testApp) client(t *testing.T) *client {
	return &client{t: t, app: a, cookies: map[string]*http.Cookie{}}
}

func (c *client) do(method, path string, form url.Values, headers map[string]string) *httptest.ResponseRecorder {
	c.t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req := httptest.NewRequestWithContext(context.Background(), method, "http://example.com"+path, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	for _, ck := range c.cookies {
		req.AddCookie(ck)
	}
	rec := httptest.NewRecorder()
	c.app.h.ServeHTTP(rec, req)
	for _, ck := range rec.Result().Cookies() {
		if ck.MaxAge < 0 {
			delete(c.cookies, ck.Name)
		} else {
			c.cookies[ck.Name] = ck
		}
	}
	return rec
}

func (c *client) get(path string) *httptest.ResponseRecorder {
	return c.do(http.MethodGet, path, nil, nil)
}

func (c *client) post(path string, form url.Values) *httptest.ResponseRecorder {
	return c.do(http.MethodPost, path, form, nil)
}

func (c *client) register(email, name, wsName string) *httptest.ResponseRecorder {
	return c.post("/daftar", url.Values{"name": {name}, "workspace_name": {wsName}, "email": {email}, "password": {"rahasia-123"}})
}

func (c *client) login(email, password string) *httptest.ResponseRecorder {
	return c.post("/masuk", url.Values{"email": {email}, "password": {password}})
}

func (c *client) sessionCookie() string {
	if ck, ok := c.cookies["kl_session"]; ok {
		return ck.Value
	}
	return ""
}

func email(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("web-%s@contoh.id", uuid.NewString()[:8])
}

func TestRegisterLoginLogoutFlow(t *testing.T) {
	c := newApp(t).client(t)
	e := email(t)

	rec := c.register(e, "Budi", "Renovasi Budi")
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/app", rec.Header().Get("Location"))
	require.NotEmpty(t, c.sessionCookie(), "session cookie set after registration")

	rec = c.get("/app")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Halo, Budi")
	assert.Contains(t, rec.Body.String(), "Renovasi Budi")
	assert.Contains(t, rec.Body.String(), "Belum ada data")
	assert.Contains(t, rec.Body.String(), "Akun dan workspace berhasil dibuat.")

	rec = c.post("/keluar", url.Values{})
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/masuk", rec.Header().Get("Location"))

	rec = c.get("/app")
	assert.Equal(t, http.StatusSeeOther, rec.Code, "logged out")

	rec = c.login(e, "rahasia-123")
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/app", rec.Header().Get("Location"))
	assert.Equal(t, http.StatusOK, c.get("/app").Code)
}

func TestLoginFailuresUseOneGenericMessage(t *testing.T) {
	c := newApp(t).client(t)
	e := email(t)
	require.Equal(t, http.StatusSeeOther, c.register(e, "A", "W").Code)
	c.post("/keluar", url.Values{})

	wrong := c.login(e, "salah-sekali")
	assert.Equal(t, http.StatusOK, wrong.Code)
	assert.Contains(t, wrong.Body.String(), "Email atau kata sandi salah.")
	assert.Empty(t, c.sessionCookie())

	unknown := c.login("tidak-ada-"+e, "rahasia-123")
	assert.Equal(t, http.StatusOK, unknown.Code)
	assert.Contains(t, unknown.Body.String(), "Email atau kata sandi salah.")
	assert.NotContains(t, strings.ToLower(unknown.Body.String()), "tidak terdaftar")
}

func TestRegisterValidationMessages(t *testing.T) {
	c := newApp(t).client(t)
	rec := c.post("/daftar", url.Values{"name": {""}, "workspace_name": {" "}, "email": {"bukan-email"}, "password": {"123"}})
	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "Nama wajib diisi.")
	assert.Contains(t, body, "Nama usaha wajib diisi.")
	assert.Contains(t, body, "Email tidak valid.")
	assert.Contains(t, body, "Kata sandi minimal 8 karakter.")
	assert.Contains(t, body, `value="bukan-email"`, "entered values are kept")
	assert.Empty(t, c.sessionCookie())
}

func TestRegisterDuplicateEmailIsGeneric(t *testing.T) {
	c := newApp(t).client(t)
	e := email(t)
	require.Equal(t, http.StatusSeeOther, c.register(e, "A", "W").Code)
	c.post("/keluar", url.Values{})

	rec := c.register(strings.ToUpper(e), "B", "W2")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Email tidak dapat digunakan.")
}

func TestRegistrationClosed(t *testing.T) {
	c := newApp(t, func(d *web.Deps) { d.RegistrationOpen = false }).client(t)
	assert.Equal(t, http.StatusNotFound, c.get("/daftar").Code)
	assert.Equal(t, http.StatusNotFound, c.register(email(t), "A", "W").Code)
	assert.NotContains(t, c.get("/masuk").Body.String(), `href="/daftar"`)
}

func TestRegistrationCode(t *testing.T) {
	c := newApp(t, func(d *web.Deps) { d.RegistrationCode = "pilot-2026" }).client(t)
	e := email(t)
	rec := c.post("/daftar", url.Values{"name": {"A"}, "workspace_name": {"W"}, "email": {e}, "password": {"rahasia-123"}, "code": {"salah"}})
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Kode undangan tidak valid.")

	rec = c.post("/daftar", url.Values{"name": {"A"}, "workspace_name": {"W"}, "email": {e}, "password": {"rahasia-123"}, "code": {"pilot-2026"}})
	assert.Equal(t, http.StatusSeeOther, rec.Code)
}

func TestSessionCookieAttributes(t *testing.T) {
	c := newApp(t).client(t)
	rec := c.register(email(t), "A", "W")
	setCookie := strings.Join(rec.Header().Values("Set-Cookie"), "\n")
	assert.Contains(t, setCookie, "kl_session=")
	assert.Contains(t, setCookie, "HttpOnly")
	assert.Contains(t, setCookie, "SameSite=Lax")
	assert.Contains(t, setCookie, "Path=/")
	assert.NotContains(t, setCookie, "Secure", "plain http in tests")

	secure := newApp(t, func(d *web.Deps) { d.Sessions = web.NewSessionManager(d.DB.(*pgxpool.Pool), true, 0) }).client(t)
	rec = secure.register(email(t), "A", "W")
	assert.Contains(t, strings.Join(rec.Header().Values("Set-Cookie"), "\n"), "Secure")
}

func TestSessionRotatesOnLoginAndDiesOnLogout(t *testing.T) {
	app := newApp(t)
	c := app.client(t)
	e := email(t)
	require.Equal(t, http.StatusSeeOther, c.register(e, "A", "W").Code)
	first := c.sessionCookie()
	require.NotEmpty(t, first)

	// Log in again while already holding a session: the token must change.
	require.Equal(t, http.StatusSeeOther, c.login(e, "rahasia-123").Code)
	second := c.sessionCookie()
	require.NotEmpty(t, second)
	assert.NotEqual(t, first, second, "session token rotates on login")

	stale := app.client(t)
	stale.cookies["kl_session"] = &http.Cookie{Name: "kl_session", Value: first} //nolint:gosec // G124: replaying a captured token on purpose
	assert.Equal(t, http.StatusSeeOther, stale.get("/app").Code, "old token is invalid after rotation")
	assert.Equal(t, http.StatusOK, c.get("/app").Code)

	c.post("/keluar", url.Values{})
	dead := app.client(t)
	dead.cookies["kl_session"] = &http.Cookie{Name: "kl_session", Value: second} //nolint:gosec // G124: replaying a captured token on purpose
	assert.Equal(t, http.StatusSeeOther, dead.get("/app").Code, "token is invalid after logout")
}

func TestCSRFCrossSiteBlocked(t *testing.T) {
	c := newApp(t).client(t)
	form := url.Values{"email": {"a@b.id"}, "password": {"x"}}
	cross := c.do(http.MethodPost, "/masuk", form, map[string]string{"Sec-Fetch-Site": "cross-site"})
	assert.Equal(t, http.StatusForbidden, cross.Code)
	assert.Contains(t, cross.Body.String(), "Permintaan ditolak")

	same := c.do(http.MethodPost, "/masuk", form, map[string]string{"Sec-Fetch-Site": "same-origin"})
	assert.NotEqual(t, http.StatusForbidden, same.Code)
}

func TestRateLimitPerEmail(t *testing.T) {
	c := newApp(t).client(t)
	e := email(t)
	for i := 0; i < 5; i++ {
		assert.Equal(t, http.StatusOK, c.login(e, "salah").Code, "attempt %d", i+1)
	}
	rec := c.login(e, "salah")
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Contains(t, rec.Body.String(), "Terlalu banyak percobaan")
}

func TestRateLimitPerIP(t *testing.T) {
	c := newApp(t).client(t)
	for i := 0; i < 10; i++ {
		assert.Equal(t, http.StatusOK, c.login(fmt.Sprintf("ip-%d-%s", i, email(t)), "salah").Code, "attempt %d", i+1)
	}
	assert.Equal(t, http.StatusTooManyRequests, c.login("ip-11-"+email(t), "salah").Code)
}

func TestUnauthenticatedRequestsRedirectToLogin(t *testing.T) {
	c := newApp(t).client(t)
	for _, tc := range []struct{ method, path string }{{"GET", "/app"}, {"GET", "/app/pengaturan"}, {"POST", "/app/pengaturan"}} {
		rec := c.do(tc.method, tc.path, url.Values{"name": {"x"}}, nil)
		assert.Equal(t, http.StatusSeeOther, rec.Code, "%s %s", tc.method, tc.path)
		assert.Equal(t, "/masuk", rec.Header().Get("Location"))
	}
	assert.Equal(t, "/masuk", c.get("/").Header().Get("Location"))
}

func TestCrossWorkspaceRenameIsImpossible(t *testing.T) {
	app := newApp(t)
	a, b := app.client(t), app.client(t)
	ea, eb := email(t), email(t)
	require.Equal(t, http.StatusSeeOther, a.register(ea, "A", "Studio A").Code)
	require.Equal(t, http.StatusSeeOther, b.register(eb, "B", "Studio B").Code)

	var wsA uuid.UUID
	require.NoError(t, app.pool.QueryRow(context.Background(),
		`SELECT m.workspace_id FROM memberships m JOIN users u ON u.id = m.user_id WHERE u.email = $1`, ea).Scan(&wsA))

	// B submits A's workspace id in the form; the server must only ever touch B's own workspace.
	rec := b.post("/app/pengaturan", url.Values{"name": {"Dibajak"}, "workspace_id": {wsA.String()}, "id": {wsA.String()}})
	assert.Equal(t, http.StatusSeeOther, rec.Code)

	pageA := a.get("/app").Body.String()
	assert.Contains(t, pageA, "Studio A")
	assert.NotContains(t, pageA, "Dibajak")
	pageB := b.get("/app").Body.String()
	assert.Contains(t, pageB, "Dibajak")
	assert.NotContains(t, pageB, "Studio A")
}

func TestPengaturanHTMXFragmentAndFullPage(t *testing.T) {
	c := newApp(t).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "Studio A").Code)

	frag := c.do(http.MethodPost, "/app/pengaturan", url.Values{"name": {"Studio A Baru"}}, map[string]string{"HX-Request": "true"})
	assert.Equal(t, http.StatusOK, frag.Code)
	assert.Contains(t, frag.Body.String(), "Nama usaha tersimpan.")
	assert.NotContains(t, frag.Body.String(), "<html")

	full := c.post("/app/pengaturan", url.Values{"name": {"Studio A Final"}})
	assert.Equal(t, http.StatusSeeOther, full.Code)
	assert.Equal(t, "/app/pengaturan", full.Header().Get("Location"))
	page := c.get("/app/pengaturan")
	assert.Contains(t, page.Body.String(), "Nama usaha tersimpan.")
	assert.Contains(t, page.Body.String(), `value="Studio A Final"`)

	bad := c.do(http.MethodPost, "/app/pengaturan", url.Values{"name": {"   "}}, map[string]string{"HX-Request": "true"})
	assert.Equal(t, http.StatusOK, bad.Code)
	assert.Contains(t, bad.Body.String(), "Nama usaha wajib diisi.")
}

func TestNotFoundAndStatic(t *testing.T) {
	c := newApp(t).client(t)
	rec := c.get("/tidak-ada")
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "Halaman tidak ditemukan")

	js := c.get("/static/htmx.min.js")
	assert.Equal(t, http.StatusOK, js.Code)
	assert.Contains(t, js.Header().Get("Content-Type"), "javascript")
	assert.NotEmpty(t, js.Header().Get("Cache-Control"))
}
