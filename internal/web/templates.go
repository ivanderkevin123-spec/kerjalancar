package web

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"kerjalancar/internal/auth"
	"kerjalancar/internal/workspace"
)

//go:embed templates
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// pageLayouts maps each page template to the layout that wraps it.
var pageLayouts = map[string]string{
	"masuk":      "auth",
	"daftar":     "auth",
	"error":      "auth",
	"beranda":    "app",
	"pengaturan": "app",
}

// Templates holds one parsed template set per page.
type Templates struct {
	pages map[string]*template.Template
}

// LoadTemplates parses every page with its layout and the shared partials.
func LoadTemplates() (*Templates, error) {
	t := &Templates{pages: make(map[string]*template.Template, len(pageLayouts))}
	for page, layout := range pageLayouts {
		tmpl, err := template.New(page).ParseFS(templateFS,
			"templates/layouts/"+layout+".html",
			"templates/partials/*.html",
			"templates/pages/"+page+".html",
		)
		if err != nil {
			return nil, fmt.Errorf("templates: %s: %w", page, err)
		}
		t.pages[page] = tmpl
	}
	return t, nil
}

// View is the data every template receives. Fields a page does not use stay zero.
type View struct {
	Title            string
	Nav              string
	User             *auth.User
	Workspace        *workspace.Workspace
	Flash            string
	Errors           map[string]string
	Form             map[string]string
	RegistrationOpen bool
	RequireCode      bool
	Saved            bool
	Heading          string
	Message          string
	Year             int
}

func newView(title string) *View {
	return &View{Title: title, Errors: map[string]string{}, Form: map[string]string{}, Year: time.Now().Year()}
}

// Render writes a full page. The output is buffered so a template error can
// never leave the client with half a page and a 200 status.
func (t *Templates) Render(w http.ResponseWriter, status int, page string, data *View) error {
	tmpl, ok := t.pages[page]
	if !ok {
		return fmt.Errorf("templates: unknown page %q", page)
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, pageLayouts[page], data); err != nil {
		return fmt.Errorf("templates: render %s: %w", page, err)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, err := buf.WriteTo(w)
	return err
}

// Fragment writes one named template of a page, for htmx partial swaps.
func (t *Templates) Fragment(w http.ResponseWriter, status int, page, name string, data *View) error {
	tmpl, ok := t.pages[page]
	if !ok {
		return fmt.Errorf("templates: unknown page %q", page)
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return fmt.Errorf("templates: fragment %s/%s: %w", page, name, err)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, err := buf.WriteTo(w)
	return err
}
