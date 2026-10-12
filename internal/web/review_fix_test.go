package web_test

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Static serving must expose only the two assets, never a directory listing
// or the Tailwind source.
func TestStaticHasNoListingAndNoSource(t *testing.T) {
	c := newApp(t).client(t)
	assert.Equal(t, http.StatusNotFound, c.get("/static/").Code, "no directory listing")
	assert.Equal(t, http.StatusNotFound, c.get("/static").Code)
	assert.Equal(t, http.StatusNotFound, c.get("/static/src/app.css").Code, "source is not served")
	assert.Equal(t, http.StatusOK, c.get("/static/app.css").Code)
}

// Correcting validation mistakes must not lock a new user out: registration
// allows ten attempts per e-mail per minute (login keeps five).
func TestRegistrationAllowsTenAttemptsPerEmail(t *testing.T) {
	c := newApp(t).client(t)
	e := email(t)
	for i := 0; i < 10; i++ {
		rec := c.post("/daftar", url.Values{"name": {""}, "workspace_name": {""}, "email": {e}, "password": {"x"}})
		assert.Equal(t, http.StatusOK, rec.Code, fmt.Sprintf("attempt %d", i+1))
	}
	rec := c.post("/daftar", url.Values{"name": {""}, "workspace_name": {""}, "email": {e}, "password": {"x"}})
	assert.Equal(t, http.StatusTooManyRequests, rec.Code, "11th attempt")
}
