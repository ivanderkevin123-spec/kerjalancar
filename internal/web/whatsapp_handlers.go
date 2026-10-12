package web

import (
	"log/slog"
	"net/http"

	"kerjalancar/internal/lead"
	"kerjalancar/internal/money"
	"kerjalancar/internal/template"
)

const msgNoPhone = "Pelanggan ini belum punya nomor WhatsApp."

// waData fills a template for a lead: the customer's current name, the
// workspace name, the estimate (if any). {tautan} stays empty until M5.
func (s *server) waData(r *http.Request, l lead.Lead) template.Data {
	d := template.Data{Nama: l.CustomerName, Usaha: currentWorkspace(r.Context()).Name}
	if l.EstimatedValue != nil {
		d.Nilai = money.FormatIDR(*l.EstimatedValue)
	}
	return d
}

// postProspekWa records that a WhatsApp message was opened for the lead and
// sends the browser to wa.me with the rendered template (D-047). The number
// is the customer's current one, read now; the text never reaches the log.
func (s *server) postProspekWa(w http.ResponseWriter, r *http.Request) {
	l, ok := s.loadLead(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, r)
		return
	}
	key, ok := template.ParseKey(r.PostFormValue("template"))
	if !ok {
		s.notFound(w, r)
		return
	}
	ws, user := currentWorkspace(r.Context()), currentUser(r.Context())
	if l.CustomerPhone == "" {
		s.Sessions.Put(r.Context(), sessionFlashKey, msgNoPhone)
		http.Redirect(w, r, "/app/prospek/"+l.ID.String(), http.StatusSeeOther)
		return
	}
	tpl, err := s.MessageTemplates.Get(r.Context(), ws.ID, key)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if err := s.Leads.RecordWhatsApp(r.Context(), ws.ID, user.ID, l.ID, key.Label()); err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, "lead.whatsapp", slog.String("lead_id", l.ID.String()), slog.String("template", string(key)))
	http.Redirect(w, r, template.WaLink(l.CustomerPhone, template.Render(tpl.Body, s.waData(r, l))), http.StatusSeeOther)
}
