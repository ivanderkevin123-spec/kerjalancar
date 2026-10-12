package web

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"kerjalancar/internal/auth"
	"kerjalancar/internal/customer"
	"kerjalancar/internal/followup"
	"kerjalancar/internal/lead"
	"kerjalancar/internal/money"
	"kerjalancar/internal/quotation"
	"kerjalancar/internal/workspace"
)

//go:embed templates
var templateFS embed.FS

// Only the two served assets are embedded; the Tailwind source stays out of the binary.
//
//go:embed static/app.css static/htmx.min.js
var staticFS embed.FS

// pageLayouts maps each page template to the layout that wraps it.
var pageLayouts = map[string]string{
	"masuk":            "auth",
	"daftar":           "auth",
	"error":            "auth",
	"beranda":          "app",
	"pengaturan":       "app",
	"pelanggan_daftar": "app",
	"pelanggan_form":   "app",
	"pelanggan_detail": "app",
	"prospek_daftar":   "app",
	"prospek_form":     "app",
	"prospek_detail":   "app",
	"followup_daftar":  "app",
	"templat":          "app",
	"penawaran_daftar": "app",
	"penawaran_form":   "app",
	"penawaran_detail": "app",
	"share_penawaran":  "share",
}

var monthsID = [...]string{"", "Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}

// formatDateID renders a date as "20 Okt 2026".
func formatDateID(t time.Time) string {
	return fmt.Sprintf("%d %s %d", t.Day(), monthsID[t.Month()], t.Year())
}

// templateFuncs are the display helpers available to every template.
var templateFuncs = template.FuncMap{
	// rupiah formats an optional amount; nil renders as "".
	"rupiah": func(v *int64) string {
		if v == nil {
			return ""
		}
		return money.FormatIDR(*v)
	},
	"rupiahInt": money.FormatIDR,
	"telepon":   customer.DisplayPhone,
	// tanggal renders an optional date (stored as UTC midnight).
	"tanggal": func(t *time.Time) string {
		if t == nil {
			return ""
		}
		return formatDateID(*t)
	},
	// waktuTanggal renders only the date of a timestamp in the zone: "12 Okt 2026".
	"waktuTanggal": func(t time.Time, loc *time.Location) string {
		if loc != nil {
			t = t.In(loc)
		}
		return formatDateID(t)
	},
	// waktu renders a timestamp in the workspace zone: "12 Okt 2026, 14.05".
	"waktu": func(t time.Time, loc *time.Location) string {
		if loc != nil {
			t = t.In(loc)
		}
		return formatDateID(t) + t.Format(", 15.04")
	},
	// lewat reports whether a follow-up date is before the workspace's today (View.Today).
	"lewat": func(d *time.Time, today time.Time) bool {
		return d != nil && d.Before(today)
	},
	"badgeClass": func(st lead.Status) string {
		switch st {
		case lead.StatusDihubungi:
			return "bg-sky-50 text-sky-700 ring-sky-200"
		case lead.StatusSurvei:
			return "bg-amber-50 text-amber-700 ring-amber-200"
		case lead.StatusPenawaran:
			return "bg-teal-50 text-teal-700 ring-teal-200"
		case lead.StatusMenang:
			return "bg-emerald-50 text-emerald-700 ring-emerald-200"
		case lead.StatusKalah:
			return "bg-rose-50 text-rose-700 ring-rose-200"
		}
		return "bg-slate-100 text-slate-700 ring-slate-200"
	},
	"bucketLabel": func(b followup.Bucket) string {
		switch b {
		case followup.BucketOverdue:
			return "terlambat"
		case followup.BucketToday:
			return "hari ini"
		case followup.BucketUpcoming:
			return "mendatang"
		}
		return "nanti"
	},
	"bucketClass": func(b followup.Bucket) string {
		switch b {
		case followup.BucketOverdue:
			return "bg-rose-50 text-rose-700 ring-rose-200"
		case followup.BucketToday:
			return "bg-teal-50 text-teal-700 ring-teal-200"
		}
		return "bg-slate-100 text-slate-700 ring-slate-200"
	},
	"qty": money.FormatQty,
	"qBadgeClass": func(st quotation.Status) string {
		switch st {
		case quotation.StatusSent:
			return "bg-sky-50 text-sky-700 ring-sky-200"
		case quotation.StatusAccepted:
			return "bg-emerald-50 text-emerald-700 ring-emerald-200"
		case quotation.StatusRejected:
			return "bg-rose-50 text-rose-700 ring-rose-200"
		case quotation.StatusExpired:
			return "bg-amber-50 text-amber-700 ring-amber-200"
		}
		return "bg-slate-100 text-slate-700 ring-slate-200"
	},
	"statuses":     func() []lead.Status { return lead.Statuses },
	"sources":      func() []lead.Source { return lead.Sources },
	"projectTypes": func() []lead.ProjectType { return lead.ProjectTypes },
}

// Templates holds one parsed template set per page.
type Templates struct {
	pages map[string]*template.Template
}

// LoadTemplates parses every page with its layout and the shared partials.
func LoadTemplates() (*Templates, error) {
	t := &Templates{pages: make(map[string]*template.Template, len(pageLayouts))}
	for page, layout := range pageLayouts {
		tmpl, err := template.New(page).Funcs(templateFuncs).ParseFS(templateFS,
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
	// Loc is the workspace time zone for rendering timestamps; Today is the
	// workspace's calendar day (UTC midnight) from the injectable clock.
	Loc   *time.Location
	Today time.Time
	// Data holds the page-specific view model (one struct type per page).
	Data any
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
