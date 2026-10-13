package web

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgconn"
)

// fragment writes one named template for an htmx swap; a failure becomes a logged 500.
func (s *server) fragment(w http.ResponseWriter, r *http.Request, page, name string, v *View) {
	if err := s.Templates.Fragment(w, http.StatusOK, page, name, v); err != nil {
		// Fragment buffers its output, so nothing has been written yet and the
		// full error page (retargeted to the body for htmx) can still be sent.
		s.serverError(w, r, fmt.Errorf("fragment %s/%s: %w", page, name, err))
	}
}

// render writes a page; a template failure becomes a logged 500 with no partial output.
func (s *server) render(w http.ResponseWriter, r *http.Request, status int, page string, v *View) {
	if err := s.Templates.Render(w, status, page, v); err != nil {
		s.Logger.ErrorContext(r.Context(), "render page",
			slog.String("request_id", middleware.GetReqID(r.Context())), slog.String("page", page), slog.Any("err", err))
		http.Error(w, "Terjadi kesalahan. Coba lagi beberapa saat.", http.StatusInternalServerError)
	}
}

// errorPage renders the shared error page. It must not touch the session:
// some callers (CSRF denial, rate limiting) run before the session is loaded.
func (s *server) errorPage(w http.ResponseWriter, r *http.Request, status int, heading, message string) {
	if r.Header.Get("HX-Request") == "true" {
		// Replace the whole page, whatever fragment the request targeted.
		w.Header().Set("HX-Retarget", "body")
		w.Header().Set("HX-Reswap", "innerHTML")
	}
	v := newView(heading)
	v.Heading = heading
	v.Message = message
	v.RegistrationOpen = s.RegistrationOpen
	s.render(w, r, status, "error", v)
}

func (s *server) notFound(w http.ResponseWriter, r *http.Request) {
	s.errorPage(w, r, http.StatusNotFound, "Halaman tidak ditemukan", "Alamat yang Anda buka tidak ada atau sudah dipindahkan.")
}

func (s *server) methodNotAllowed(w http.ResponseWriter, r *http.Request) {
	s.errorPage(w, r, http.StatusMethodNotAllowed, "Metode tidak didukung", "Permintaan ini tidak dapat diproses.")
}

func (s *server) badRequest(w http.ResponseWriter, r *http.Request) {
	s.errorPage(w, r, http.StatusBadRequest, "Permintaan tidak valid", "Data formulir tidak dapat dibaca. Muat ulang halaman dan coba lagi.")
}

func (s *server) forbidden(w http.ResponseWriter, r *http.Request) {
	s.errorPage(w, r, http.StatusForbidden, "Permintaan ditolak", "Permintaan berasal dari situs lain. Muat ulang halaman dan coba lagi.")
}

func (s *server) tooLarge(w http.ResponseWriter, r *http.Request) {
	s.errorPage(w, r, http.StatusRequestEntityTooLarge, "Data terlalu besar", "Data yang dikirim terlalu besar. Periksa isian lalu coba lagi.")
}

func (s *server) tooManyRequests(w http.ResponseWriter, r *http.Request) {
	s.errorPage(w, r, http.StatusTooManyRequests, "Terlalu banyak percobaan", "Tunggu satu menit lalu coba lagi.")
}

func (s *server) serverError(w http.ResponseWriter, r *http.Request, err error) {
	// A deadlock (40P01) or serialization failure (40001) means two people
	// changed the same rows at once; Postgres aborted one of them and nothing
	// was written. That is a "try again", not a server fault.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == "40P01" || pgErr.Code == "40001") {
		s.Logger.WarnContext(r.Context(), "transaction conflict",
			slog.String("request_id", middleware.GetReqID(r.Context())), slog.String("path", logPath(r.URL.Path)), slog.String("code", pgErr.Code))
		s.errorPage(w, r, http.StatusConflict, "Perubahan bertabrakan", "Perubahan Anda bertabrakan dengan perubahan lain pada data yang sama dan tidak disimpan. Muat ulang halaman, lalu coba lagi.")
		return
	}
	s.Logger.ErrorContext(r.Context(), "request failed",
		slog.String("request_id", middleware.GetReqID(r.Context())), slog.String("path", logPath(r.URL.Path)), slog.Any("err", err))
	s.errorPage(w, r, http.StatusInternalServerError, "Terjadi kesalahan", "Coba lagi beberapa saat. Jika berulang, hubungi tim KerjaLancar.")
}
