package web

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// maxFormBytes bounds the body of any form submission. Login forms are a
// few hundred bytes; the largest M3 form (lead with notes) is under 10 KiB.
const maxFormBytes = 64 << 10

type ctxIPKey struct{}

// clientIPMiddleware decides once per request which address identifies the
// client and stores it in the context for the logger and the rate limiter.
// r.RemoteAddr is never mutated. With trustProxy the address is the rightmost
// X-Forwarded-For entry: the one appended by the single trusted proxy in
// front of the app, which the client cannot forge. X-Real-IP is deliberately
// ignored: many proxies pass a client-supplied value through untouched.
// Without trustProxy the headers are ignored entirely.
func clientIPMiddleware(trustProxy bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), ctxIPKey{}, clientIP(r, trustProxy))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if ip := strings.TrimSpace(parts[len(parts)-1]); ip != "" {
				return ip
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ClientIP returns the address chosen by clientIPMiddleware ("" before it ran).
func ClientIP(r *http.Request) string {
	ip, _ := r.Context().Value(ctxIPKey{}).(string)
	return ip
}

// logPath is the request path as it may appear in logs: share tokens travel
// in the path (D-052), so "/p/<token>..." is masked to "/p/{token}...".
func logPath(p string) string {
	const prefix = "/p/"
	if !strings.HasPrefix(p, prefix) {
		return p
	}
	rest := p[len(prefix):]
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		return prefix + "{token}" + rest[i:]
	}
	return prefix + "{token}"
}

// requestLogger writes one structured line per request. It logs the masked
// path and never the query string.
func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
				next.ServeHTTP(w, r)
				return
			}
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			log.LogAttrs(r.Context(), slog.LevelInfo, "http request",
				slog.String("request_id", middleware.GetReqID(r.Context())),
				slog.String("method", r.Method),
				slog.String("path", logPath(r.URL.Path)),
				slog.Int("status", ww.Status()),
				slog.Int("bytes", ww.BytesWritten()),
				slog.Duration("duration", time.Since(start)),
				slog.String("ip", ClientIP(r)),
			)
		})
	}
}

// securityHeaders sets the baseline response headers on every response.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

// noStore keeps HTML pages out of browser and proxy caches, so the back
// button after logout cannot show a stale dashboard.
func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// limitBody refuses oversized form submissions before any parsing happens
// and caps unknown-length bodies at maxFormBytes.
func (s *server) limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			if r.ContentLength > maxFormBytes {
				s.tooLarge(w, r)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
		}
		next.ServeHTTP(w, r)
	})
}
