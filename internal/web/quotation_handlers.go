package web

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"kerjalancar/internal/lead"
	"kerjalancar/internal/money"
	"kerjalancar/internal/quotation"
	"kerjalancar/internal/template"
)

const (
	msgQuotationSaved    = "Penawaran tersimpan."
	msgQuotationSent     = "Penawaran ditandai terkirim; bagikan tautan atau PDF-nya lewat WhatsApp."
	msgQuotationAccepted = "Penawaran ditandai diterima; prospek menjadi Menang."
	msgQuotationRejected = "Penawaran ditandai ditolak."
	msgQuotationLocked   = "Penawaran yang sudah diterima atau ditolak tidak dapat diubah."
	msgQuotationStale    = "Penawaran ini diubah di tempat lain; muat ulang halaman lalu ulangi."
	msgQuotationSame     = "Status tidak berubah."
	msgQuotationLeadLost = "Prospek ini sudah Kalah; buka kembali prospek sebelum menandai penawaran diterima."
	msgShareRevoked      = "Tautan dicabut; tautan lama tidak bisa dibuka lagi."
	msgShareRenewed      = "Tautan baru dibuat."
	defaultValidityDays  = 14
	emptyRows            = 3
)

// qRow is one editor row as typed (strings) with its errors.
type qRow struct {
	Desc, Qty, Unit, Price string
	Errors                 map[string]string
}

type quotationFormData struct {
	Title   string
	Action  string
	Cancel  string
	Lead    lead.Lead
	Rows    []qRow
	Edit    bool
	Version int
}

// qView is a quotation prepared for templates (effective status, share URL).
type qView struct {
	Q        quotation.Quotation
	Eff      quotation.Status
	ShareURL string
}

type quotationListData struct {
	Page  quotation.Page
	Rows  []qView
	Q     string
	Chips []chip
	Pager Pager
}

type quotationDetailData struct {
	V         qView
	Lead      lead.Lead
	Templates []template.Template
	Msg       string
	Err       string
}

func (s *server) qview(q quotation.Quotation, today time.Time) qView {
	v := qView{Q: q, Eff: q.EffectiveStatus(today)}
	if q.ShareActive() {
		v.ShareURL = s.BaseURL + "/p/" + q.ShareToken
	}
	return v
}

func rowsFromForm(r *http.Request) []qRow {
	desc, qty, unit, price := r.PostForm["desc"], r.PostForm["qty"], r.PostForm["unit"], r.PostForm["price"]
	n := len(desc)
	for _, l := range []int{len(qty), len(unit), len(price)} {
		if l > n {
			n = l
		}
	}
	at := func(list []string, i int) string {
		if i < len(list) {
			return strings.TrimSpace(list[i])
		}
		return ""
	}
	rows := make([]qRow, 0, n)
	for i := 0; i < n && i < quotation.MaxItems+10; i++ {
		rows = append(rows, qRow{Desc: at(desc, i), Qty: at(qty, i), Unit: at(unit, i), Price: at(price, i), Errors: map[string]string{}})
	}
	return rows
}

func rowsToInput(rows []qRow) []quotation.ItemInput {
	out := make([]quotation.ItemInput, 0, len(rows))
	for _, r := range rows {
		out = append(out, quotation.ItemInput{Description: r.Desc, Qty: r.Qty, Unit: r.Unit, UnitPrice: r.Price})
	}
	return out
}

// applyErrors distributes "items.N.field" messages to the rows; the rest stay in v.Errors.
func applyErrors(rows []qRow, errs map[string]string) map[string]string {
	rest := map[string]string{}
	for k, msg := range errs {
		if strings.HasPrefix(k, "items.") {
			parts := strings.SplitN(k, ".", 3)
			if len(parts) == 3 {
				if i, err := strconv.Atoi(parts[1]); err == nil && i < len(rows) {
					rows[i].Errors[parts[2]] = msg
					continue
				}
			}
		}
		rest[k] = msg
	}
	return rest
}

func rowsFromQuotation(q quotation.Quotation) []qRow {
	rows := make([]qRow, 0, len(q.Items)+1)
	for _, it := range q.Items {
		rows = append(rows, qRow{Desc: it.Description, Qty: money.FormatQty(it.QtyMilli), Unit: it.Unit, Price: strings.TrimPrefix(money.FormatIDR(it.UnitPrice), "Rp"), Errors: map[string]string{}})
	}
	rows = append(rows, qRow{Errors: map[string]string{}})
	return rows
}

func emptyRowsList(n int) []qRow {
	rows := make([]qRow, 0, n)
	for i := 0; i < n; i++ {
		rows = append(rows, qRow{Errors: map[string]string{}})
	}
	return rows
}

func (s *server) loadQuotation(w http.ResponseWriter, r *http.Request) (quotation.Quotation, bool) {
	ws := currentWorkspace(r.Context())
	id, ok := parseID(r)
	if !ok {
		s.notFound(w, r)
		return quotation.Quotation{}, false
	}
	q, err := s.Quotations.Get(r.Context(), ws.ID, id)
	if errors.Is(err, quotation.ErrNotFound) {
		s.notFound(w, r)
		return quotation.Quotation{}, false
	}
	if err != nil {
		s.serverError(w, r, err)
		return quotation.Quotation{}, false
	}
	return q, true
}

func (s *server) getPenawaranList(w http.ResponseWriter, r *http.Request) {
	ws := currentWorkspace(r.Context())
	qv := r.URL.Query()
	status, q := qv.Get("status"), qv.Get("q")
	if !quotation.Status(status).Valid() {
		status = "semua"
	}
	today := s.today(ws)
	page, err := s.Quotations.List(r.Context(), ws.ID, quotation.ListQuery{Status: status, Q: q, Page: parsePage(r)}, today)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	d := quotationListData{Page: page, Q: q}
	for _, it := range page.Items {
		d.Rows = append(d.Rows, s.qview(it, today))
	}
	link := func(st string) string {
		p := url.Values{}
		if q != "" {
			p.Set("q", q)
		}
		if st != "semua" {
			p.Set("status", st)
		}
		if len(p) == 0 {
			return "/app/penawaran"
		}
		return "/app/penawaran?" + p.Encode()
	}
	d.Chips = append(d.Chips, chip{Key: "semua", Label: "Semua", URL: link("semua"), Active: status == "semua"})
	for _, st := range quotation.Statuses {
		d.Chips = append(d.Chips, chip{Key: string(st), Label: st.Label(), URL: link(string(st)), Active: status == string(st)})
	}
	params := url.Values{"q": {q}}
	if status != "semua" {
		params.Set("status", status)
	}
	d.Pager = newPager("/app/penawaran", params, page.Page, page.Pages)
	v := s.appView(r, "Penawaran", "penawaran")
	v.Flash = s.Sessions.PopString(r.Context(), sessionFlashKey)
	v.Data = d
	s.render(w, r, http.StatusOK, "penawaran_daftar", v)
}

// getPenawaranBaris answers the "tambah baris" htmx request with one empty row.
func (s *server) getPenawaranBaris(w http.ResponseWriter, r *http.Request) {
	v := s.appView(r, "Baris", "penawaran")
	v.Data = qRow{Errors: map[string]string{}}
	s.fragment(w, r, "penawaran_form", "penawaran_baris_baru", v)
}

func (s *server) getPenawaranBaru(w http.ResponseWriter, r *http.Request) {
	ws := currentWorkspace(r.Context())
	leadID, err := uuid.Parse(r.URL.Query().Get("prospek"))
	if err != nil {
		s.notFound(w, r)
		return
	}
	l, err := s.Leads.Get(r.Context(), ws.ID, leadID)
	if errors.Is(err, lead.ErrNotFound) {
		s.notFound(w, r)
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	v := s.appView(r, "Buat penawaran", "penawaran")
	v.Form["valid_until"] = s.today(ws).AddDate(0, 0, defaultValidityDays).Format(quotation.DateLayout)
	v.Data = quotationFormData{Title: "Buat penawaran", Action: "/app/penawaran", Cancel: "/app/prospek/" + l.ID.String(), Lead: l, Rows: emptyRowsList(emptyRows)}
	s.render(w, r, http.StatusOK, "penawaran_form", v)
}

func (s *server) postPenawaran(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, r)
		return
	}
	ws, user := currentWorkspace(r.Context()), currentUser(r.Context())
	leadID, err := uuid.Parse(r.PostFormValue("lead_id"))
	if err != nil {
		s.notFound(w, r)
		return
	}
	l, err := s.Leads.Get(r.Context(), ws.ID, leadID)
	if errors.Is(err, lead.ErrNotFound) {
		s.notFound(w, r)
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	rows := rowsFromForm(r)
	form := formValues(r, "discount_pct", "tax_pct", "valid_until", "notes")
	n, errs := quotation.Validate(quotation.Input{Items: rowsToInput(rows), DiscountPct: form["discount_pct"], TaxPct: form["tax_pct"], ValidUntil: form["valid_until"], Notes: form["notes"]})
	if len(errs) > 0 {
		v := s.appView(r, "Buat penawaran", "penawaran")
		v.Form = form
		v.Errors = applyErrors(rows, errs)
		v.Data = quotationFormData{Title: "Buat penawaran", Action: "/app/penawaran", Cancel: "/app/prospek/" + l.ID.String(), Lead: l, Rows: rows}
		s.render(w, r, http.StatusOK, "penawaran_form", v)
		return
	}
	q, err := s.Quotations.Create(r.Context(), ws.ID, user.ID, l.ID, n)
	if errors.Is(err, quotation.ErrLeadNotFound) {
		s.notFound(w, r)
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, "quotation.create", slog.String("quotation_id", q.ID.String()), slog.String("lead_id", l.ID.String()), slog.Int("seq", q.Seq))
	s.Sessions.Put(r.Context(), sessionFlashKey, msgQuotationSaved)
	http.Redirect(w, r, "/app/penawaran/"+q.ID.String(), http.StatusSeeOther)
}

func (s *server) detailQuotation(w http.ResponseWriter, r *http.Request, q quotation.Quotation, msg, errMsg string) {
	ws := currentWorkspace(r.Context())
	l, err := s.Leads.Get(r.Context(), ws.ID, q.LeadID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	tpls, err := s.MessageTemplates.ForWorkspace(r.Context(), ws.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	v := s.appView(r, "Penawaran #"+q.Number(), "penawaran")
	if msg == "" && errMsg == "" {
		v.Flash = s.Sessions.PopString(r.Context(), sessionFlashKey)
	}
	v.Data = quotationDetailData{V: s.qview(q, s.today(ws)), Lead: l, Templates: tpls, Msg: msg, Err: errMsg}
	s.render(w, r, http.StatusOK, "penawaran_detail", v)
}

func (s *server) getPenawaran(w http.ResponseWriter, r *http.Request) {
	q, ok := s.loadQuotation(w, r)
	if !ok {
		return
	}
	s.detailQuotation(w, r, q, "", "")
}

func (s *server) getPenawaranUbah(w http.ResponseWriter, r *http.Request) {
	q, ok := s.loadQuotation(w, r)
	if !ok {
		return
	}
	if !q.Editable() {
		s.Sessions.Put(r.Context(), sessionFlashKey, msgQuotationLocked)
		http.Redirect(w, r, "/app/penawaran/"+q.ID.String(), http.StatusSeeOther)
		return
	}
	ws := currentWorkspace(r.Context())
	l, err := s.Leads.Get(r.Context(), ws.ID, q.LeadID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	v := s.appView(r, "Ubah penawaran #"+q.Number(), "penawaran")
	v.Form = map[string]string{"discount_pct": strconv.Itoa(q.DiscountPct), "tax_pct": strconv.Itoa(q.TaxPct), "notes": q.Notes}
	if q.ValidUntil != nil {
		v.Form["valid_until"] = q.ValidUntil.Format(quotation.DateLayout)
	}
	v.Data = quotationFormData{Title: "Ubah penawaran #" + q.Number(), Action: "/app/penawaran/" + q.ID.String(), Cancel: "/app/penawaran/" + q.ID.String(), Lead: l, Rows: rowsFromQuotation(q), Edit: true, Version: q.Version}
	s.render(w, r, http.StatusOK, "penawaran_form", v)
}

func (s *server) postPenawaranUbah(w http.ResponseWriter, r *http.Request) {
	q, ok := s.loadQuotation(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, r)
		return
	}
	ws, user := currentWorkspace(r.Context()), currentUser(r.Context())
	rows := rowsFromForm(r)
	form := formValues(r, "discount_pct", "tax_pct", "valid_until", "notes", "version")
	version, _ := strconv.Atoi(form["version"])
	n, errs := quotation.Validate(quotation.Input{Items: rowsToInput(rows), DiscountPct: form["discount_pct"], TaxPct: form["tax_pct"], ValidUntil: form["valid_until"], Notes: form["notes"]})
	var err error
	if len(errs) == 0 {
		_, err = s.Quotations.Update(r.Context(), ws.ID, user.ID, q.ID, n, version)
		switch {
		case err == nil:
			s.audit(r, "quotation.update", slog.String("quotation_id", q.ID.String()))
			s.Sessions.Put(r.Context(), sessionFlashKey, msgChangesSaved)
			http.Redirect(w, r, "/app/penawaran/"+q.ID.String(), http.StatusSeeOther)
			return
		case errors.Is(err, quotation.ErrLocked):
			s.Sessions.Put(r.Context(), sessionFlashKey, msgQuotationLocked)
			http.Redirect(w, r, "/app/penawaran/"+q.ID.String(), http.StatusSeeOther)
			return
		case errors.Is(err, quotation.ErrStale):
			errs = map[string]string{"form": msgQuotationStale}
		case errors.Is(err, quotation.ErrNotFound):
			s.notFound(w, r)
			return
		default:
			s.serverError(w, r, err)
			return
		}
	}
	l, err := s.Leads.Get(r.Context(), ws.ID, q.LeadID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	v := s.appView(r, "Ubah penawaran #"+q.Number(), "penawaran")
	v.Form = form
	v.Errors = applyErrors(rows, errs)
	v.Data = quotationFormData{Title: "Ubah penawaran #" + q.Number(), Action: "/app/penawaran/" + q.ID.String(), Cancel: "/app/penawaran/" + q.ID.String(), Lead: l, Rows: rows, Edit: true, Version: version}
	s.render(w, r, http.StatusOK, "penawaran_form", v)
}

// status actions: kirim, terima, tolak; each answers with a redirect and a flash.
func (s *server) postPenawaranAksi(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q, ok := s.loadQuotation(w, r)
		if !ok {
			return
		}
		ws, user := currentWorkspace(r.Context()), currentUser(r.Context())
		var err error
		var msg string
		switch action {
		case "kirim":
			_, err = s.Quotations.Send(r.Context(), ws.ID, user.ID, q.ID)
			msg = msgQuotationSent
		case "terima":
			_, err = s.Quotations.Accept(r.Context(), ws.ID, user.ID, q.ID, s.today(ws))
			msg = msgQuotationAccepted
		default:
			_, err = s.Quotations.Reject(r.Context(), ws.ID, user.ID, q.ID)
			msg = msgQuotationRejected
		}
		switch {
		case err == nil:
			s.audit(r, "quotation."+action, slog.String("quotation_id", q.ID.String()), slog.String("lead_id", q.LeadID.String()))
		case errors.Is(err, quotation.ErrSameStatus):
			msg = msgQuotationSame
		case errors.Is(err, quotation.ErrLocked):
			msg = msgQuotationLocked
		case errors.Is(err, quotation.ErrLeadClosed):
			msg = msgQuotationLeadLost
		case errors.Is(err, quotation.ErrNotFound), errors.Is(err, quotation.ErrLeadNotFound), errors.Is(err, lead.ErrNotFound):
			s.notFound(w, r)
			return
		default:
			s.serverError(w, r, err)
			return
		}
		s.Sessions.Put(r.Context(), sessionFlashKey, msg)
		http.Redirect(w, r, "/app/penawaran/"+q.ID.String(), http.StatusSeeOther)
	}
}

func (s *server) postPenawaranTautan(w http.ResponseWriter, r *http.Request) {
	q, ok := s.loadQuotation(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, r)
		return
	}
	ws := currentWorkspace(r.Context())
	var err error
	var msg string
	switch r.PostFormValue("aksi") {
	case "cabut":
		_, err = s.Quotations.RevokeShare(r.Context(), ws.ID, q.ID)
		msg = msgShareRevoked
	case "baru":
		_, err = s.Quotations.RenewShare(r.Context(), ws.ID, q.ID)
		msg = msgShareRenewed
	default:
		s.badRequest(w, r)
		return
	}
	if errors.Is(err, quotation.ErrNotFound) {
		s.notFound(w, r)
		return
	}
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.audit(r, "quotation.share", slog.String("quotation_id", q.ID.String()), slog.String("aksi", r.PostFormValue("aksi")))
	s.Sessions.Put(r.Context(), sessionFlashKey, msg)
	http.Redirect(w, r, "/app/penawaran/"+q.ID.String(), http.StatusSeeOther)
}

func (s *server) getPenawaranPDF(w http.ResponseWriter, r *http.Request) {
	q, ok := s.loadQuotation(w, r)
	if !ok {
		return
	}
	ws := currentWorkspace(r.Context())
	s.writePDF(w, r, q, location(ws.Timezone), s.today(ws))
}

// writePDF streams the document as a download, dated in loc and with the
// status effective on today; a render failure becomes a 500 page.
func (s *server) writePDF(w http.ResponseWriter, r *http.Request, q quotation.Quotation, loc *time.Location, today time.Time) {
	var buf bytes.Buffer
	if err := quotation.RenderPDF(&buf, q, quotation.PDFOptions{Compress: true, CreatedAt: q.UpdatedAt, Loc: loc, Today: today}); err != nil {
		s.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="penawaran-`+q.Number()+`.pdf"`)
	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	w.WriteHeader(http.StatusOK)
	_, _ = buf.WriteTo(w)
}
