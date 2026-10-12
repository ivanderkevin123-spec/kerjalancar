package web

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"kerjalancar/internal/followup"
	"kerjalancar/internal/quotation"
)

// Public share pages: no session, GET only, rate-limited, never cached. A
// token that is unknown, revoked or expired answers the 404 page; an expired
// one says so, since the customer may hold a printed copy.

type shareData struct {
	V qView
}

// shareLoc is the zone of the public pages: the customer's zone is unknown,
// and WIB (Indonesia's westernmost) keeps an offer valid through its last
// calendar day anywhere in the country.
func shareLoc() *time.Location { return location("Asia/Jakarta") }

func (s *server) loadShared(w http.ResponseWriter, r *http.Request) (quotation.Quotation, time.Time, bool) {
	today := followup.Today(s.now(), shareLoc())
	q, err := s.Quotations.GetByToken(r.Context(), chi.URLParam(r, "token"), today)
	switch {
	case err == nil:
		return q, today, true
	case errors.Is(err, quotation.ErrExpired):
		s.errorPage(w, r, http.StatusNotFound, "Penawaran tidak berlaku lagi", "Masa berlaku penawaran ini sudah lewat. Hubungi penyedia jasa untuk penawaran yang diperbarui.")
	case errors.Is(err, quotation.ErrNotFound):
		s.notFound(w, r)
	default:
		s.serverError(w, r, err)
	}
	return quotation.Quotation{}, today, false
}

func (s *server) getPublicQuotation(w http.ResponseWriter, r *http.Request) {
	q, _, ok := s.loadShared(w, r)
	if !ok {
		return
	}
	if err := s.Quotations.RecordOpen(r.Context(), q.WorkspaceID, q.ID); err != nil {
		s.serverError(w, r, err)
		return
	}
	v := newView("Penawaran #" + q.Number() + " · " + q.WorkspaceName)
	v.Loc = shareLoc()
	v.Data = shareData{V: qView{Q: q, Eff: q.Status}}
	s.render(w, r, http.StatusOK, "share_penawaran", v)
}

func (s *server) getPublicQuotationPDF(w http.ResponseWriter, r *http.Request) {
	q, today, ok := s.loadShared(w, r)
	if !ok {
		return
	}
	s.writePDF(w, r, q, shareLoc(), today)
}
