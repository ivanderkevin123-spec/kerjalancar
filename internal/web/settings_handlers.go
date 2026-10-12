package web

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"kerjalancar/internal/template"
	"kerjalancar/internal/workspace"
)

const (
	msgTimezoneSaved   = "Zona waktu tersimpan."
	msgTimezoneUnknown = "Zona waktu tidak dikenal."
	msgTemplateSaved   = "Templat tersimpan."
)

type zoneOption struct {
	Value, Label string
	Selected     bool
}

type pengaturanData struct {
	Zones     []zoneOption
	ZoneMsg   string
	ZoneErr   string
	ZoneSaved bool
}

func (s *server) pengaturanData(ws workspace.Workspace, selected string) pengaturanData {
	if selected == "" {
		selected = ws.Timezone
	}
	d := pengaturanData{}
	for _, tz := range workspace.Timezones {
		d.Zones = append(d.Zones, zoneOption{Value: tz, Label: workspace.TimezoneLabel(tz), Selected: tz == selected})
	}
	return d
}

// postPengaturanZona changes the workspace time zone (owner only, whitelisted).
func (s *server) postPengaturanZona(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, r)
		return
	}
	ws, user := currentWorkspace(r.Context()), currentUser(r.Context())
	tz := r.PostFormValue("timezone")
	hx := r.Header.Get("HX-Request") == "true"
	updated, err := s.Workspaces.UpdateTimezone(r.Context(), ws.ID, user.ID, tz)
	data := s.pengaturanData(ws, "")
	switch {
	case err == nil:
		s.audit(r, "workspace.timezone", slog.String("timezone", updated.Timezone))
		if !hx {
			s.Sessions.Put(r.Context(), sessionFlashKey, msgTimezoneSaved)
			http.Redirect(w, r, "/app/pengaturan", http.StatusSeeOther)
			return
		}
		ws = updated
		data = s.pengaturanData(updated, "")
		data.ZoneMsg = msgTimezoneSaved
	case errors.Is(err, workspace.ErrInvalidTimezone):
		data.ZoneErr = msgTimezoneUnknown
	case errors.Is(err, workspace.ErrNotFound):
		s.notFound(w, r)
		return
	default:
		s.serverError(w, r, err)
		return
	}
	v := s.appView(r, "Pengaturan", "pengaturan")
	v.Workspace = &ws
	v.Form["name"] = ws.Name
	v.Data = data
	if hx {
		s.fragment(w, r, "pengaturan", "pengaturan_zona", v)
		return
	}
	s.render(w, r, http.StatusOK, "pengaturan", v)
}

// Templates page.

type templateCard struct {
	T       template.Template
	Preview string
	Missing []string
	Msg     string
	Errors  map[string]string
	Form    map[string]string
}

type templatesData struct {
	Cards []templateCard
	Card  *templateCard // one card for an htmx response
}

// sampleData previews a template with obvious placeholder data.
func sampleData(ws workspace.Workspace) template.Data {
	return template.Data{Nama: "Budi", Usaha: ws.Name, Nilai: "Rp15.000.000"}
}

func (s *server) templateCard(ws workspace.Workspace, t template.Template) templateCard {
	return templateCard{T: t, Preview: template.Render(t.Body, sampleData(ws)), Missing: template.MissingPlaceholders(t.Body, sampleData(ws)),
		Form: map[string]string{"name": t.Name, "body": t.Body}, Errors: map[string]string{}}
}

func (s *server) getPengaturanTemplat(w http.ResponseWriter, r *http.Request) {
	ws := currentWorkspace(r.Context())
	list, err := s.MessageTemplates.ForWorkspace(r.Context(), ws.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	d := templatesData{}
	for _, t := range list {
		d.Cards = append(d.Cards, s.templateCard(ws, t))
	}
	v := s.appView(r, "Templat pesan", "pengaturan")
	v.Flash = s.Sessions.PopString(r.Context(), sessionFlashKey)
	v.Data = d
	s.render(w, r, http.StatusOK, "templat", v)
}

func (s *server) postPengaturanTemplat(w http.ResponseWriter, r *http.Request) {
	ws := currentWorkspace(r.Context())
	key, ok := template.ParseKey(chi.URLParam(r, "key"))
	if !ok {
		s.notFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, r)
		return
	}
	form := formValues(r, "name", "body")
	hx := r.Header.Get("HX-Request") == "true"
	updated, err := s.MessageTemplates.Update(r.Context(), ws.ID, key, form["name"], form["body"])
	var verr *template.ValidationError
	var card templateCard
	switch {
	case err == nil:
		s.audit(r, "template.update", slog.String("template", string(key)))
		if !hx {
			s.Sessions.Put(r.Context(), sessionFlashKey, msgTemplateSaved)
			http.Redirect(w, r, "/app/pengaturan/templat", http.StatusSeeOther)
			return
		}
		card = s.templateCard(ws, updated)
		card.Msg = msgTemplateSaved
	case errors.As(err, &verr):
		cur, gerr := s.MessageTemplates.Get(r.Context(), ws.ID, key)
		if gerr != nil {
			s.serverError(w, r, gerr)
			return
		}
		card = s.templateCard(ws, cur)
		card.Form, card.Errors = form, verr.Fields
	case errors.Is(err, template.ErrNotFound):
		s.notFound(w, r)
		return
	default:
		s.serverError(w, r, err)
		return
	}
	v := s.appView(r, "Templat pesan", "pengaturan")
	if hx {
		v.Data = templatesData{Card: &card}
		s.fragment(w, r, "templat", "templat_card_oob", v)
		return
	}
	list, err := s.MessageTemplates.ForWorkspace(r.Context(), ws.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	d := templatesData{}
	for _, t := range list {
		c := s.templateCard(ws, t)
		if t.Key == key {
			c = card
		}
		d.Cards = append(d.Cards, c)
	}
	v.Data = d
	s.render(w, r, http.StatusOK, "templat", v)
}
