package web

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"kerjalancar/internal/customer"
	"kerjalancar/internal/followup"
	"kerjalancar/internal/lead"
	"kerjalancar/internal/money"
	"kerjalancar/internal/template"
)

const (
	msgLeadSaved       = "Prospek tersimpan."
	msgStatusSaved     = "Status diperbarui."
	msgNoteSaved       = "Catatan tersimpan."
	msgSameStatus      = "Status tidak berubah."
	msgClosedToClosed  = "Prospek yang sudah ditutup harus dibuka kembali dulu (pilih status aktif) sebelum ditutup dengan hasil lain."
	msgStatusUnknown   = "Status tidak dikenal."
	msgPickCustomer    = "Pilih pelanggan."
	msgCustomerMissing = "Pelanggan tidak ditemukan. Pilih ulang."
	msgCustomerNameReq = "Nama pelanggan wajib diisi."
	msgDupSelected     = " Pelanggan tersebut sudah dipilih di bawah; simpan lagi untuk melanjutkan."
)

var leadFields = []string{"customer_mode", "customer_id", "customer_name", "customer_phone",
	"title", "project_type", "description", "estimated_value", "source", "next_follow_up_on", "notes"}

type chip struct {
	Key    string
	Label  string
	URL    string
	Count  int
	Active bool
}

type leadListData struct {
	Page   lead.Page
	Q      string
	Status string
	Sort   string
	Chips  []chip
	Sorts  []chip
	Pager  Pager
}

type leadFormData struct {
	Title      string
	Action     string
	Cancel     string
	Customers  []customer.Customer
	Mode       string // "baru" or "ada"
	CustomerID string
	Edit       bool
	Duplicate  *customer.DuplicatePhoneError
}

type leadDetailData struct {
	Lead       lead.Lead
	Activities []lead.Activity
	StatusMsg  string
	StatusErr  string
	NoteMsg    string
	NoteErr    string
	Oob        bool // rendering an htmx response: mark out-of-band blocks
	// Follow-up panel: the open follow-up (nil when none) and its rendering.
	FollowUp    *fuRow
	FollowUpMsg string
	FollowUpErr string
	DefaultDue  string
	// WhatsApp block: the workspace's templates and which placeholders would be empty for this lead.
	Templates []template.Template
	WaMissing []string
	WaErr     string
}

func (s *server) getProspekList(w http.ResponseWriter, r *http.Request) {
	ws := currentWorkspace(r.Context())
	qv := r.URL.Query()
	q := qv.Get("q")
	status := qv.Get("status")
	if _, ok := lead.ParseStatus(status); !ok && status != "semua" {
		status = "aktif"
	}
	sort := qv.Get("urut")
	if sort != "nilai" && sort != "followup" {
		sort = "terbaru"
	}
	page, err := s.Leads.List(r.Context(), ws.ID, lead.ListQuery{Status: status, Q: q, Sort: sort, Page: parsePage(r)})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	sum, err := s.Leads.Summary(r.Context(), ws.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	link := func(st, srt string) string {
		p := url.Values{}
		if q != "" {
			p.Set("q", q)
		}
		if st != "aktif" {
			p.Set("status", st)
		}
		if srt != "terbaru" {
			p.Set("urut", srt)
		}
		if len(p) == 0 {
			return "/app/prospek"
		}
		return "/app/prospek?" + p.Encode()
	}
	total := 0
	for _, n := range sum.ByStatus {
		total += n
	}
	chips := []chip{{Key: "aktif", Label: "Aktif", Count: sum.OpenCount, URL: link("aktif", sort), Active: status == "aktif"}}
	for _, st := range lead.Statuses {
		chips = append(chips, chip{Key: string(st), Label: st.Label(), Count: sum.ByStatus[st], URL: link(string(st), sort), Active: status == string(st)})
	}
	chips = append(chips, chip{Key: "semua", Label: "Semua", Count: total, URL: link("semua", sort), Active: status == "semua"})
	sorts := []chip{
		{Key: "terbaru", Label: "Terbaru", URL: link(status, "terbaru"), Active: sort == "terbaru"},
		{Key: "nilai", Label: "Nilai tertinggi", URL: link(status, "nilai"), Active: sort == "nilai"},
		{Key: "followup", Label: "Follow-up terdekat", URL: link(status, "followup"), Active: sort == "followup"},
	}
	params := url.Values{"q": {q}}
	if status != "aktif" {
		params.Set("status", status)
	}
	if sort != "terbaru" {
		params.Set("urut", sort)
	}
	v := s.appView(r, "Prospek", "prospek")
	v.Flash = s.Sessions.PopString(r.Context(), sessionFlashKey)
	v.Data = leadListData{Page: page, Q: q, Status: status, Sort: sort, Chips: chips, Sorts: sorts,
		Pager: newPager("/app/prospek", params, page.Page, page.Pages)}
	s.render(w, r, http.StatusOK, "prospek_daftar", v)
}

func (s *server) newLeadForm(r *http.Request) (leadFormData, error) {
	ws := currentWorkspace(r.Context())
	customers, err := s.Customers.All(r.Context(), ws.ID)
	if err != nil {
		return leadFormData{}, err
	}
	return leadFormData{Title: "Catat prospek", Action: "/app/prospek", Cancel: "/app/prospek", Customers: customers, Mode: "baru"}, nil
}

func (s *server) getProspekBaru(w http.ResponseWriter, r *http.Request) {
	ws := currentWorkspace(r.Context())
	data, err := s.newLeadForm(r)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if pid, err := uuid.Parse(r.URL.Query().Get("pelanggan")); err == nil {
		if c, err := s.Customers.Get(r.Context(), ws.ID, pid); err == nil {
			data.Mode, data.CustomerID = "ada", c.ID.String()
		}
	}
	v := s.appView(r, data.Title, "prospek")
	v.Data = data
	s.render(w, r, http.StatusOK, "prospek_form", v)
}

func (s *server) postProspek(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, r)
		return
	}
	ws, user := currentWorkspace(r.Context()), currentUser(r.Context())
	form := formValues(r, leadFields...)
	data, err := s.newLeadForm(r)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	data.Mode = form["customer_mode"]
	if data.Mode != "ada" {
		data.Mode = "baru"
	}
	data.CustomerID = form["customer_id"]
	v := s.appView(r, data.Title, "prospek")
	v.Form = form

	in := lead.CreateInput{Input: leadInput(form)}
	_, v.Errors = lead.Validate(in.Input)
	if form["next_follow_up_on"] != "" {
		// The first follow-up obeys the same bounds as the scheduling form.
		if _, ferrs := followup.Validate(followup.Input{DueOn: form["next_follow_up_on"]}, s.today(ws)); ferrs["due_on"] != "" {
			v.Errors["next_follow_up_on"] = ferrs["due_on"]
		}
	}
	if data.Mode == "ada" {
		id, err := uuid.Parse(form["customer_id"])
		if err != nil {
			v.Errors["customer_id"] = msgPickCustomer
		}
		in.CustomerID = id
	} else {
		in.NewCustomer = &customer.Input{Name: form["customer_name"], Phone: form["customer_phone"]}
		if _, cerrs := customer.Validate(*in.NewCustomer); len(cerrs) > 0 {
			if cerrs["name"] != "" {
				v.Errors["customer_name"] = msgCustomerNameReq
			}
			if cerrs["phone"] != "" {
				v.Errors["customer_phone"] = cerrs["phone"]
			}
		}
	}
	if len(v.Errors) > 0 {
		v.Data = data
		s.render(w, r, http.StatusOK, "prospek_form", v)
		return
	}

	l, err := s.Leads.Create(r.Context(), ws.ID, user.ID, in)
	var dup *customer.DuplicatePhoneError
	var verr *lead.ValidationError
	var cverr *customer.ValidationError
	switch {
	case err == nil:
		s.audit(r, "lead.create", slog.String("lead_id", l.ID.String()), slog.String("customer_id", l.CustomerID.String()))
		s.Sessions.Put(r.Context(), sessionFlashKey, msgLeadSaved)
		http.Redirect(w, r, "/app/prospek/"+l.ID.String(), http.StatusSeeOther)
		return
	case errors.As(err, &dup):
		v.Errors["customer_phone"] = duplicateMessage(dup)
		if dup.Name != "" {
			data.Mode, data.CustomerID, data.Duplicate = "ada", dup.ID.String(), dup
			v.Errors["customer_phone"] += msgDupSelected
		}
	case errors.Is(err, lead.ErrCustomerNotFound):
		v.Errors["customer_id"] = msgCustomerMissing
	case errors.As(err, &verr):
		v.Errors = verr.Fields
	case errors.As(err, &cverr):
		v.Errors["customer_name"], v.Errors["customer_phone"] = cverr.Fields["name"], cverr.Fields["phone"]
	default:
		s.serverError(w, r, err)
		return
	}
	v.Data = data
	s.render(w, r, http.StatusOK, "prospek_form", v)
}

// loadLead resolves {id} within the current workspace or writes the 404 page.
func (s *server) loadLead(w http.ResponseWriter, r *http.Request) (lead.Lead, bool) {
	ws := currentWorkspace(r.Context())
	id, ok := parseID(r)
	if !ok {
		s.notFound(w, r)
		return lead.Lead{}, false
	}
	l, err := s.Leads.Get(r.Context(), ws.ID, id)
	if errors.Is(err, lead.ErrNotFound) {
		s.notFound(w, r)
		return lead.Lead{}, false
	}
	if err != nil {
		s.serverError(w, r, err)
		return lead.Lead{}, false
	}
	return l, true
}

// detailView builds the lead page model with its activity timeline and follow-up panel.
func (s *server) detailView(r *http.Request, l lead.Lead, data leadDetailData) (*View, error) {
	ws := currentWorkspace(r.Context())
	acts, err := s.Leads.Activities(r.Context(), ws.ID, l.ID, lead.MaxActivities)
	if err != nil {
		return nil, err
	}
	today, loc := s.today(ws), location(ws.Timezone)
	open, ok, err := s.FollowUps.OpenForLead(r.Context(), ws.ID, l.ID)
	if err != nil {
		return nil, err
	}
	data.Lead, data.Activities = l, acts
	data.DefaultDue = followup.DefaultDue(today).Format(followup.DateLayout)
	if data.Templates, err = s.MessageTemplates.ForWorkspace(r.Context(), ws.ID); err != nil {
		return nil, err
	}
	waData := s.waData(r, l)
	for _, tpl := range data.Templates {
		for _, p := range template.MissingPlaceholders(tpl.Body, waData) {
			data.WaMissing = appendUnique(data.WaMissing, p)
		}
	}
	v := s.appView(r, l.Title, "prospek")
	v.Form["status"] = string(l.Status)
	v.Form["lost_reason"] = l.LostReason
	v.Form["due_on"], v.Form["note"] = data.DefaultDue, ""
	if ok {
		row := s.rowView(open, today, loc)
		data.FollowUp = &row
		v.Form["due_on"], v.Form["note"] = open.DueOn.Format(followup.DateLayout), open.Note
	}
	v.Data = data
	return v, nil
}

func (s *server) getProspek(w http.ResponseWriter, r *http.Request) {
	l, ok := s.loadLead(w, r)
	if !ok {
		return
	}
	v, err := s.detailView(r, l, leadDetailData{})
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	v.Flash = s.Sessions.PopString(r.Context(), sessionFlashKey)
	s.render(w, r, http.StatusOK, "prospek_detail", v)
}

func (s *server) postProspekStatus(w http.ResponseWriter, r *http.Request) {
	l, ok := s.loadLead(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, r)
		return
	}
	ws, user := currentWorkspace(r.Context()), currentUser(r.Context())
	st := strings.TrimSpace(r.PostFormValue("status"))
	reason := strings.TrimSpace(r.PostFormValue("lost_reason"))
	hx := r.Header.Get("HX-Request") == "true"

	data := leadDetailData{}
	updated, err := s.Leads.ChangeStatus(r.Context(), ws.ID, user.ID, l.ID, lead.Status(st), reason, s.today(ws))
	var verr *lead.ValidationError
	switch {
	case err == nil:
		s.audit(r, "lead.status", slog.String("lead_id", l.ID.String()), slog.String("from", string(l.Status)), slog.String("to", st))
		if !hx {
			s.Sessions.Put(r.Context(), sessionFlashKey, msgStatusSaved)
			http.Redirect(w, r, "/app/prospek/"+l.ID.String(), http.StatusSeeOther)
			return
		}
		l, data.StatusMsg = updated, msgStatusSaved
	case errors.Is(err, lead.ErrSameStatus):
		data.StatusErr = msgSameStatus
	case errors.Is(err, lead.ErrClosedToClosed):
		data.StatusErr = msgClosedToClosed
	case errors.Is(err, lead.ErrInvalidStatus):
		data.StatusErr = msgStatusUnknown
	case errors.As(err, &verr):
		data.StatusErr = verr.Fields["lost_reason"]
	case errors.Is(err, lead.ErrNotFound):
		s.notFound(w, r)
		return
	default:
		s.serverError(w, r, err)
		return
	}
	v, err := s.detailView(r, l, data)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if data.StatusErr != "" {
		if _, ok := lead.ParseStatus(st); ok {
			v.Form["status"] = st
		}
		v.Form["lost_reason"] = reason
	}
	if hx {
		d := v.Data.(leadDetailData)
		d.Oob = true
		v.Data = d
		s.fragment(w, r, "prospek_detail", "prospek_status_oob", v)
		return
	}
	s.render(w, r, http.StatusOK, "prospek_detail", v)
}

func (s *server) postProspekCatatan(w http.ResponseWriter, r *http.Request) {
	l, ok := s.loadLead(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, r)
		return
	}
	ws, user := currentWorkspace(r.Context()), currentUser(r.Context())
	body := r.PostFormValue("body")
	hx := r.Header.Get("HX-Request") == "true"

	data := leadDetailData{}
	note, err := s.Leads.AddNote(r.Context(), ws.ID, user.ID, l.ID, body)
	var verr *lead.ValidationError
	switch {
	case err == nil:
		s.audit(r, "lead.note", slog.String("lead_id", l.ID.String()), slog.String("activity_id", note.ID.String()))
		if !hx {
			s.Sessions.Put(r.Context(), sessionFlashKey, msgNoteSaved)
			http.Redirect(w, r, "/app/prospek/"+l.ID.String(), http.StatusSeeOther)
			return
		}
		data.NoteMsg = msgNoteSaved
	case errors.As(err, &verr):
		data.NoteErr = verr.Fields["body"]
	case errors.Is(err, lead.ErrNotFound):
		s.notFound(w, r)
		return
	default:
		s.serverError(w, r, err)
		return
	}
	v, err := s.detailView(r, l, data)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if data.NoteErr != "" {
		v.Form["body"] = body
	}
	if hx {
		s.fragment(w, r, "prospek_detail", "prospek_catatan_oob", v)
		return
	}
	s.render(w, r, http.StatusOK, "prospek_detail", v)
}

func (s *server) getProspekUbah(w http.ResponseWriter, r *http.Request) {
	l, ok := s.loadLead(w, r)
	if !ok {
		return
	}
	v := s.appView(r, "Ubah prospek", "prospek")
	v.Form = leadForm(l)
	v.Data = leadFormData{Title: "Ubah prospek", Action: "/app/prospek/" + l.ID.String(), Cancel: "/app/prospek/" + l.ID.String(), Edit: true}
	s.render(w, r, http.StatusOK, "prospek_form", v)
}

func (s *server) postProspekUbah(w http.ResponseWriter, r *http.Request) {
	l, ok := s.loadLead(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, r)
		return
	}
	ws, user := currentWorkspace(r.Context()), currentUser(r.Context())
	form := formValues(r, leadFields...)
	form["next_follow_up_on"] = "" // not on the edit form; follow-ups are scheduled from the detail page
	updated, err := s.Leads.Update(r.Context(), ws.ID, user.ID, l.ID, leadInput(form))
	var verr *lead.ValidationError
	switch {
	case err == nil:
		s.audit(r, "lead.update", slog.String("lead_id", updated.ID.String()))
		s.Sessions.Put(r.Context(), sessionFlashKey, msgChangesSaved)
		http.Redirect(w, r, "/app/prospek/"+updated.ID.String(), http.StatusSeeOther)
	case errors.As(err, &verr):
		v := s.appView(r, "Ubah prospek", "prospek")
		v.Form, v.Errors = form, verr.Fields
		v.Data = leadFormData{Title: "Ubah prospek", Action: "/app/prospek/" + l.ID.String(), Cancel: "/app/prospek/" + l.ID.String(), Edit: true}
		s.render(w, r, http.StatusOK, "prospek_form", v)
	case errors.Is(err, lead.ErrNotFound):
		s.notFound(w, r)
	default:
		s.serverError(w, r, err)
	}
}

func appendUnique(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

func leadInput(f map[string]string) lead.Input {
	return lead.Input{Title: f["title"], ProjectType: f["project_type"], Description: f["description"],
		EstimatedValue: f["estimated_value"], Source: f["source"], NextFollowUpOn: f["next_follow_up_on"], Notes: f["notes"]}
}

// leadForm turns a stored lead back into form values for the edit page.
func leadForm(l lead.Lead) map[string]string {
	f := map[string]string{"title": l.Title, "project_type": string(l.ProjectType), "description": l.Description,
		"source": string(l.Source), "notes": l.Notes}
	if l.EstimatedValue != nil {
		f["estimated_value"] = strings.TrimPrefix(money.FormatIDR(*l.EstimatedValue), "Rp")
	}
	return f
}
