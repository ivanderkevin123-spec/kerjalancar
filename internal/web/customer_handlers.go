package web

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"kerjalancar/internal/customer"
	"kerjalancar/internal/lead"
)

const (
	msgCustomerSaved  = "Pelanggan tersimpan."
	msgChangesSaved   = "Perubahan tersimpan."
	msgDuplicatePhone = "Nomor ini sudah terdaftar atas nama %s."
)

var customerFields = []string{"name", "phone", "email", "company", "notes"}

type customerListData struct {
	Page  customer.Page
	Q     string
	Pager Pager
}

type customerFormData struct {
	Title     string
	Action    string
	Cancel    string
	Duplicate *customer.DuplicatePhoneError
}

type customerDetailData struct {
	Customer customer.Customer
	Leads    []lead.Lead
}

// parseID reads the {id} route parameter; false means "treat as not found".
func parseID(r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	return id, err == nil
}

func (s *server) getPelangganList(w http.ResponseWriter, r *http.Request) {
	ws := currentWorkspace(r.Context())
	q := r.URL.Query().Get("q")
	page, err := s.Customers.List(r.Context(), ws.ID, customer.Query{Q: q, Page: parsePage(r)})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	v := s.appView(r, "Pelanggan", "pelanggan")
	v.Flash = s.Sessions.PopString(r.Context(), sessionFlashKey)
	v.Data = customerListData{Page: page, Q: q, Pager: newPager("/app/pelanggan", url.Values{"q": {q}}, page.Page, page.Pages)}
	s.render(w, r, http.StatusOK, "pelanggan_daftar", v)
}

func (s *server) getPelangganBaru(w http.ResponseWriter, r *http.Request) {
	v := s.appView(r, "Tambah pelanggan", "pelanggan")
	v.Data = customerFormData{Title: "Tambah pelanggan", Action: "/app/pelanggan", Cancel: "/app/pelanggan"}
	s.render(w, r, http.StatusOK, "pelanggan_form", v)
}

func (s *server) postPelanggan(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, r)
		return
	}
	ws := currentWorkspace(r.Context())
	form := formValues(r, customerFields...)
	v := s.appView(r, "Tambah pelanggan", "pelanggan")
	v.Form = form
	data := customerFormData{Title: "Tambah pelanggan", Action: "/app/pelanggan", Cancel: "/app/pelanggan"}

	c, err := s.Customers.Create(r.Context(), ws.ID, customerInput(form))
	if s.customerFormFailed(w, r, v, &data, err) {
		return
	}
	s.audit(r, "customer.create", slog.String("customer_id", c.ID.String()))
	s.Sessions.Put(r.Context(), sessionFlashKey, msgCustomerSaved)
	http.Redirect(w, r, "/app/pelanggan/"+c.ID.String(), http.StatusSeeOther)
}

func (s *server) getPelanggan(w http.ResponseWriter, r *http.Request) {
	ws := currentWorkspace(r.Context())
	id, ok := parseID(r)
	if !ok {
		s.notFound(w, r)
		return
	}
	c, err := s.Customers.Get(r.Context(), ws.ID, id)
	if errors.Is(err, customer.ErrNotFound) {
		s.notFound(w, r)
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	leads, err := s.Leads.ForCustomer(r.Context(), ws.ID, c.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	v := s.appView(r, c.Name, "pelanggan")
	v.Flash = s.Sessions.PopString(r.Context(), sessionFlashKey)
	v.Data = customerDetailData{Customer: c, Leads: leads}
	s.render(w, r, http.StatusOK, "pelanggan_detail", v)
}

func (s *server) getPelangganUbah(w http.ResponseWriter, r *http.Request) {
	ws := currentWorkspace(r.Context())
	id, ok := parseID(r)
	if !ok {
		s.notFound(w, r)
		return
	}
	c, err := s.Customers.Get(r.Context(), ws.ID, id)
	if errors.Is(err, customer.ErrNotFound) {
		s.notFound(w, r)
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	v := s.appView(r, "Ubah pelanggan", "pelanggan")
	v.Form = map[string]string{"name": c.Name, "phone": customer.DisplayPhone(c.Phone), "email": c.Email, "company": c.Company, "notes": c.Notes}
	v.Data = customerFormData{Title: "Ubah pelanggan", Action: "/app/pelanggan/" + c.ID.String(), Cancel: "/app/pelanggan/" + c.ID.String()}
	s.render(w, r, http.StatusOK, "pelanggan_form", v)
}

func (s *server) postPelangganUbah(w http.ResponseWriter, r *http.Request) {
	ws := currentWorkspace(r.Context())
	id, ok := parseID(r)
	if !ok {
		s.notFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, r)
		return
	}
	form := formValues(r, customerFields...)
	v := s.appView(r, "Ubah pelanggan", "pelanggan")
	v.Form = form
	data := customerFormData{Title: "Ubah pelanggan", Action: "/app/pelanggan/" + id.String(), Cancel: "/app/pelanggan/" + id.String()}

	c, err := s.Customers.Update(r.Context(), ws.ID, id, customerInput(form))
	if errors.Is(err, customer.ErrNotFound) {
		s.notFound(w, r)
		return
	}
	if s.customerFormFailed(w, r, v, &data, err) {
		return
	}
	s.audit(r, "customer.update", slog.String("customer_id", c.ID.String()))
	s.Sessions.Put(r.Context(), sessionFlashKey, msgChangesSaved)
	http.Redirect(w, r, "/app/pelanggan/"+c.ID.String(), http.StatusSeeOther)
}

// customerFormFailed renders the form again for validation and duplicate
// errors (true), passes nil through (false), and reports anything else as 500.
func (s *server) customerFormFailed(w http.ResponseWriter, r *http.Request, v *View, data *customerFormData, err error) bool {
	var verr *customer.ValidationError
	var dup *customer.DuplicatePhoneError
	switch {
	case err == nil:
		return false
	case errors.As(err, &verr):
		v.Errors = verr.Fields
	case errors.As(err, &dup):
		v.Errors["phone"] = duplicateMessage(dup)
		if dup.Name != "" {
			data.Duplicate = dup
		}
	default:
		s.serverError(w, r, err)
		return true
	}
	v.Data = *data
	s.render(w, r, http.StatusOK, "pelanggan_form", v)
	return true
}

// duplicateMessage names the owner of the number when known; a unique
// violation that slipped past the pre-check carries no name.
func duplicateMessage(dup *customer.DuplicatePhoneError) string {
	if dup.Name == "" {
		return "Nomor ini sudah terdaftar untuk pelanggan lain."
	}
	return fmt.Sprintf(msgDuplicatePhone, dup.Name)
}

func customerInput(f map[string]string) customer.Input {
	return customer.Input{Name: f["name"], Phone: f["phone"], Email: f["email"], Company: f["company"], Notes: f["notes"]}
}
