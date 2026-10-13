// Package web wires HTTP transport: router, middleware, handlers, templates.
// Handlers stay thin: parse, authorize, call a service, render.
package web

import (
	"context"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"

	"kerjalancar/internal/auth"
	"kerjalancar/internal/customer"
	"kerjalancar/internal/followup"
	"kerjalancar/internal/lead"
	"kerjalancar/internal/quotation"
	"kerjalancar/internal/template"
	"kerjalancar/internal/workspace"
)

// Pinger is the only thing the router needs from the database layer today.
// *pgxpool.Pool satisfies it.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Deps are the collaborators the router needs. With Sessions nil the router
// serves only health endpoints and static files (used by infrastructure tests).
type Deps struct {
	Logger *slog.Logger
	DB     Pinger
	// TrustProxy: take the client address from the rightmost X-Forwarded-For
	// entry. Set it only when exactly one trusted proxy sits in front of the
	// app and appends that header; X-Real-IP is never honoured.
	TrustProxy bool

	Sessions         *scs.SessionManager
	Auth             *auth.Service
	Workspaces       *workspace.Store
	Customers        *customer.Store
	Leads            *lead.Store
	FollowUps        *followup.Store
	MessageTemplates *template.Store
	Quotations       *quotation.Store
	Templates        *Templates
	// Clock supplies "now"; nil means time.Now. Tests pin it to check date buckets.
	Clock            func() time.Time
	RegistrationOpen bool
	RegistrationCode string
	BaseURL          string // trusted origin for CSRF checks behind a proxy
}

// server groups the dependencies for handler methods.
type server struct {
	Deps
}

const (
	publicLimitPerIP      = 60 // share-page requests per minute per client IP
	loginLimitPerIP       = 10 // requests per minute per client IP, per endpoint
	loginLimitPerEmail    = 5  // login attempts per minute per e-mail
	registerLimitPerEmail = 10 // registration attempts per e-mail; typo corrections must not lock people out
)

const (
	requestTimeout = 30 * time.Second
	readyzTimeout  = 2 * time.Second
)

// NewRouter builds the HTTP handler for the whole application.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(clientIPMiddleware(d.TrustProxy))
	r.Use(requestLogger(d.Logger))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(requestTimeout))
	r.Use(securityHeaders)

	r.Get("/healthz", handleHealthz)
	r.Get("/readyz", handleReadyz(d.DB))

	static, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic("web: static fs: " + err.Error())
	}
	r.Handle("/static/*", cacheFor(time.Hour, noListing(http.StripPrefix("/static/", http.FileServerFS(static)))))

	if d.Sessions == nil {
		return r
	}
	s := &server{Deps: d}

	csrf := http.NewCrossOriginProtection()
	if d.BaseURL != "" {
		if err := csrf.AddTrustedOrigin(d.BaseURL); err != nil {
			panic("web: BASE_URL is not a valid origin: " + err.Error())
		}
	}
	csrf.SetDenyHandler(http.HandlerFunc(s.forbidden))
	// A session store failure must render our error page, not scs's plain text.
	d.Sessions.ErrorFunc = func(w http.ResponseWriter, r *http.Request, err error) { s.serverError(w, r, err) }

	r.NotFound(s.notFound)
	r.MethodNotAllowed(s.methodNotAllowed)

	// Public share pages: outside the session group, GET only, rate-limited, never cached.
	r.Group(func(r chi.Router) {
		r.Use(noStore)
		r.Use(httprate.LimitBy(publicLimitPerIP, time.Minute, func(r *http.Request) (string, error) {
			return "public:" + ClientIP(r), nil
		}, httprate.WithLimitHandler(s.tooManyRequests)))
		r.Get("/p/{token}", s.getPublicQuotation)
		r.Get("/p/{token}/pdf", s.getPublicQuotationPDF)
	})

	r.Group(func(r chi.Router) {
		r.Use(noStore)
		r.Use(s.limitBody)
		r.Use(func(next http.Handler) http.Handler { return csrf.Handler(next) })
		r.Use(d.Sessions.LoadAndSave)

		r.Get("/", s.root)
		r.Get("/masuk", s.getMasuk)
		r.With(s.authLimits(loginLimitPerEmail)...).Post("/masuk", s.postMasuk)
		r.Get("/daftar", s.getDaftar)
		r.With(s.authLimits(registerLimitPerEmail)...).Post("/daftar", s.postDaftar)
		r.Post("/keluar", s.postKeluar)

		r.Route("/app", func(r chi.Router) {
			r.Use(s.requireLogin)
			r.Get("/", s.getBeranda)
			r.Get("/pengaturan", s.getPengaturan)
			r.Post("/pengaturan", s.postPengaturan)
			r.Post("/pengaturan/zona-waktu", s.postPengaturanZona)
			r.Get("/pengaturan/templat", s.getPengaturanTemplat)
			r.Post("/pengaturan/templat/{key}", s.postPengaturanTemplat)

			r.Get("/pelanggan", s.getPelangganList)
			r.Get("/pelanggan/baru", s.getPelangganBaru)
			r.Post("/pelanggan", s.postPelanggan)
			r.Get("/pelanggan/{id}", s.getPelanggan)
			r.Get("/pelanggan/{id}/ubah", s.getPelangganUbah)
			r.Post("/pelanggan/{id}", s.postPelangganUbah)

			r.Get("/prospek", s.getProspekList)
			r.Get("/prospek/baru", s.getProspekBaru)
			r.Post("/prospek", s.postProspek)
			r.Get("/prospek/{id}", s.getProspek)
			r.Get("/prospek/{id}/ubah", s.getProspekUbah)
			r.Post("/prospek/{id}", s.postProspekUbah)
			r.Post("/prospek/{id}/status", s.postProspekStatus)
			r.Post("/prospek/{id}/catatan", s.postProspekCatatan)
			r.Post("/prospek/{id}/follow-up", s.postProspekFollowUp)
			r.Post("/prospek/{id}/wa", s.postProspekWa)

			r.Get("/follow-up", s.getFollowUpList)
			r.Post("/follow-up/{id}/selesai", s.postFollowUpDone)
			r.Post("/follow-up/{id}/batal", s.postFollowUpUndo)

			r.Get("/penawaran", s.getPenawaranList)
			r.Get("/penawaran/baris", s.getPenawaranBaris)
			r.Get("/penawaran/baru", s.getPenawaranBaru)
			r.Post("/penawaran", s.postPenawaran)
			r.Get("/penawaran/{id}", s.getPenawaran)
			r.Get("/penawaran/{id}/ubah", s.getPenawaranUbah)
			r.Post("/penawaran/{id}", s.postPenawaranUbah)
			r.Post("/penawaran/{id}/kirim", s.postPenawaranAksi("kirim"))
			r.Post("/penawaran/{id}/terima", s.postPenawaranAksi("terima"))
			r.Post("/penawaran/{id}/tolak", s.postPenawaranAksi("tolak"))
			r.Post("/penawaran/{id}/tautan", s.postPenawaranTautan)
			r.Get("/penawaran/{id}/pdf", s.getPenawaranPDF)
		})
	})

	return r
}

// authLimits throttles a credential endpoint per client IP and per e-mail.
// Each call creates fresh counters, so each endpoint and router has its own.
func (s *server) authLimits(perEmail int) []func(http.Handler) http.Handler {
	deny := httprate.WithLimitHandler(s.tooManyRequests)
	byIP := httprate.LimitBy(loginLimitPerIP, time.Minute, func(r *http.Request) (string, error) {
		return "ip:" + ClientIP(r), nil
	}, deny)
	byEmail := httprate.LimitBy(perEmail, time.Minute, func(r *http.Request) (string, error) {
		return "email:" + auth.NormalizeEmail(r.FormValue("email")), nil
	}, deny)
	return []func(http.Handler) http.Handler{byIP, byEmail}
}

// noListing refuses directory paths so the embedded asset folder is never listed.
func noListing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func cacheFor(d time.Duration, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age="+strconv.Itoa(int(d.Seconds())))
		next.ServeHTTP(w, r)
	})
}

// handleHealthz answers "the process is up". It must not touch dependencies.
func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleReadyz answers "the process can serve traffic": the database responds
// within readyzTimeout.
func handleReadyz(db Pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), readyzTimeout)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status": "unavailable",
				"check":  "database",
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
