package web_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kerjalancar/internal/web"
)

// Findings of the independent M2 review, fixed at the start of M3.

// htmx 2 does not swap 4xx/5xx responses by default, so under hx-boost a 429,
// 403, 404 or 500 page would never appear. The layouts must opt in, and
// error pages must retarget the whole body when rendered for an htmx request.
func TestErrorPagesReachHtmxClients(t *testing.T) {
	c := newApp(t).client(t)
	page := c.get("/masuk").Body.String()
	assert.Contains(t, page, `name="htmx-config"`)
	assert.Contains(t, page, `{"code":"[45]..","swap":true,"error":true}`)

	rec := c.do(http.MethodGet, "/tidak-ada", nil, map[string]string{"HX-Request": "true"})
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Equal(t, "body", rec.Header().Get("HX-Retarget"))
	assert.Equal(t, "innerHTML", rec.Header().Get("HX-Reswap"))

	plain := c.get("/tidak-ada")
	assert.Empty(t, plain.Header().Get("HX-Retarget"), "plain requests get no htmx headers")
}

// The limiter must key on the client address the proxy reports, never on a
// header the client can forge, and never on the shared proxy address.
func TestRateLimitKeysOnTheRightClientAddress(t *testing.T) {
	loginFrom := func(c *client, xff string, i int) int {
		return c.do(http.MethodPost, "/masuk", url.Values{"email": {fmt.Sprintf("ip-%d-%s", i, email(t))}, "password": {"salah"}},
			map[string]string{"X-Forwarded-For": xff}).Code
	}

	trusted := newApp(t, func(d *web.Deps) { d.TrustProxy = true }).client(t)
	for i := 0; i < 10; i++ {
		require.Equal(t, http.StatusOK, loginFrom(trusted, "203.0.113.1", i), "attempt %d", i+1)
	}
	assert.Equal(t, http.StatusTooManyRequests, loginFrom(trusted, "203.0.113.1", 10), "11th from the same forwarded address")
	assert.Equal(t, http.StatusOK, loginFrom(trusted, "203.0.113.2", 11), "another forwarded address has its own bucket")
	assert.Equal(t, http.StatusTooManyRequests, loginFrom(trusted, "198.51.100.5, 203.0.113.1", 12),
		"the proxy-appended (rightmost) address counts, not the client-supplied one")

	untrusted := newApp(t).client(t)
	for i := 0; i < 10; i++ {
		require.Equal(t, http.StatusOK, loginFrom(untrusted, fmt.Sprintf("203.0.113.%d", i), i))
	}
	assert.Equal(t, http.StatusTooManyRequests, loginFrom(untrusted, "203.0.113.99", 10), "without a trusted proxy the header is ignored")
}

func TestSecurityHeadersAndNoStoreOnAuthenticatedPages(t *testing.T) {
	c := newApp(t).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	rec := c.get("/app")
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
	assert.Equal(t, "same-origin", rec.Header().Get("Referrer-Policy"))

	login := c.get("/masuk")
	assert.Equal(t, "nosniff", login.Header().Get("X-Content-Type-Options"))

	css := c.get("/static/app.css")
	assert.Contains(t, css.Header().Get("Cache-Control"), "max-age", "static assets stay cacheable")
}

func TestOversizedFormBodyIsRefusedEarly(t *testing.T) {
	c := newApp(t).client(t)
	big := strings.Repeat("a", 70*1024)
	rec := c.post("/masuk", url.Values{"email": {"a@b.id"}, "password": {big}})
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	assert.Contains(t, rec.Body.String(), "terlalu besar")
}

type brokenStore struct{}

func (brokenStore) Delete(string) error                    { return errors.New("store down") }
func (brokenStore) Find(string) ([]byte, bool, error)      { return nil, false, errors.New("store down") }
func (brokenStore) Commit(string, []byte, time.Time) error { return errors.New("store down") }
func (brokenStore) All() (map[string][]byte, error)        { return nil, errors.New("store down") }

func TestSessionStoreFailureRendersTheIndonesianErrorPage(t *testing.T) {
	app := newApp(t, func(d *web.Deps) {
		sm := scs.New()
		sm.Store = brokenStore{}
		sm.Cookie.Name = "kl_session"
		d.Sessions = sm
	})
	c := app.client(t)
	c.cookies["kl_session"] = &http.Cookie{Name: "kl_session", Value: "apa-saja"}
	rec := c.get("/masuk")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "Terjadi kesalahan")
	assert.NotContains(t, rec.Body.String(), "store down")
}

func TestUserWhoLostTheirMembershipIsSentToLogin(t *testing.T) {
	app := newApp(t)
	c := app.client(t)
	e := email(t)
	require.Equal(t, http.StatusSeeOther, c.register(e, "A", "W").Code)
	_, err := app.pool.Exec(context.Background(), `DELETE FROM memberships WHERE user_id = (SELECT id FROM users WHERE email = $1)`, e)
	require.NoError(t, err)
	rec := c.get("/app")
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/masuk", rec.Header().Get("Location"))
}

func TestHeaderWorkspaceNameFollowsAnHtmxRename(t *testing.T) {
	c := newApp(t).client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "Studio Lama").Code)
	frag := c.do(http.MethodPost, "/app/pengaturan", url.Values{"name": {"Studio Baru"}}, map[string]string{"HX-Request": "true"})
	require.Equal(t, http.StatusOK, frag.Code)
	assert.Contains(t, frag.Body.String(), `hx-swap-oob`)
	assert.Contains(t, frag.Body.String(), `id="workspace-name"`)
	assert.Contains(t, frag.Body.String(), "Studio Baru")
}

func TestCSRFBlocksEveryStateChangingRoute(t *testing.T) {
	app := newApp(t)
	owner := app.client(t)
	require.Equal(t, http.StatusSeeOther, owner.register(email(t), "A", "W").Code)
	for _, path := range []string{"/masuk", "/daftar", "/keluar", "/app/pengaturan"} {
		form := url.Values{"email": {"a@b.id"}, "password": {"x"}, "name": {"n"}, "workspace_name": {"w"}}
		cross := owner.do(http.MethodPost, path, form, map[string]string{"Sec-Fetch-Site": "cross-site"})
		assert.Equal(t, http.StatusForbidden, cross.Code, "%s with Sec-Fetch-Site: cross-site", path)
		origin := owner.do(http.MethodPost, path, form, map[string]string{"Origin": "https://jahat.example"})
		assert.Equal(t, http.StatusForbidden, origin.Code, "%s with a foreign Origin", path)
		assert.Equal(t, http.StatusOK, owner.get("/app").Code, "still logged in: the cross-site %s did nothing", path)
		if path == "/keluar" {
			continue // a trusted-origin logout would end the session for the remaining rows
		}
		trusted := owner.do(http.MethodPost, path, form, map[string]string{"Origin": "http://example.com"})
		assert.NotEqual(t, http.StatusForbidden, trusted.Code, "%s from the trusted origin", path)
	}
}

func TestPasswordsNeverReachTheLogs(t *testing.T) {
	var buf bytes.Buffer
	app := newApp(t, func(d *web.Deps) {
		d.Logger = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	})
	c := app.client(t)
	e := email(t)
	const secret = "sandi-unik-7f3a9c"
	c.post("/daftar", url.Values{"name": {"A"}, "workspace_name": {"W"}, "email": {e}, "password": {secret}})
	c.post("/keluar", url.Values{})
	c.login(e, secret+"-salah")
	c.login(e, secret)
	assert.NotContains(t, buf.String(), secret)
	assert.NotContains(t, buf.String(), "kl_session=")
}

func TestExpiredSessionRedirectsToLogin(t *testing.T) {
	app := newApp(t)
	c := app.client(t)
	require.Equal(t, http.StatusSeeOther, c.register(email(t), "A", "W").Code)
	require.Equal(t, http.StatusOK, c.get("/app").Code)
	_, err := app.pool.Exec(context.Background(), `UPDATE sessions SET expiry = now() - interval '1 minute'`)
	require.NoError(t, err)
	rec := c.get("/app")
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/masuk", rec.Header().Get("Location"))
}

// The workspace chosen at login is pinned in the session and re-checked
// against memberships on every request, instead of silently taking the
// first membership (which changes as soon as a user has two).
func TestCurrentWorkspaceIsPinnedInTheSession(t *testing.T) {
	app := newApp(t)
	c := app.client(t)
	e := email(t)
	require.Equal(t, http.StatusSeeOther, c.register(e, "A", "Studio Asli").Code)

	var userID uuid.UUID
	require.NoError(t, app.pool.QueryRow(context.Background(), `SELECT id FROM users WHERE email = $1`, e).Scan(&userID))
	other := uuid.New()
	_, err := app.pool.Exec(context.Background(), `INSERT INTO workspaces (id, name, created_at) VALUES ($1, 'Studio Lain', now() - interval '1 day')`, other)
	require.NoError(t, err)
	_, err = app.pool.Exec(context.Background(), `INSERT INTO memberships (workspace_id, user_id, role) VALUES ($1, $2, 'staff')`, other, userID)
	require.NoError(t, err)

	page := c.get("/app").Body.String()
	assert.Contains(t, page, "Studio Asli")
	assert.NotContains(t, page, "Studio Lain", "an older membership must not hijack the current workspace")
}
