package web

import (
	"net/http"
	"time"

	"github.com/alexedwards/scs/postgresstore"
	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

const (
	sessionCookieName = "kl_session"
	sessionUserKey    = "user_id"
	sessionFlashKey   = "flash"
)

// NewSessionManager stores sessions server-side in the `sessions` table.
// secure must be true in production (HTTPS). cleanupInterval 0 disables the
// background sweep (tests); the server uses a periodic sweep.
func NewSessionManager(pool *pgxpool.Pool, secure bool, cleanupInterval time.Duration) *scs.SessionManager {
	sm := scs.New()
	sm.Store = postgresstore.NewWithCleanupInterval(stdlib.OpenDBFromPool(pool), cleanupInterval)
	sm.Lifetime = 30 * 24 * time.Hour
	sm.IdleTimeout = 7 * 24 * time.Hour
	sm.Cookie.Name = sessionCookieName
	sm.Cookie.HttpOnly = true
	sm.Cookie.Secure = secure
	sm.Cookie.SameSite = http.SameSiteLaxMode
	sm.Cookie.Path = "/"
	sm.Cookie.Persist = true
	return sm
}
