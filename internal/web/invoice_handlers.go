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

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"kerjalancar/internal/invoice"
	"kerjalancar/internal/lead"
	"kerjalancar/internal/money"
	"kerjalancar/internal/quotation"
	"kerjalancar/internal/template"
)

const (
	msgInvoiceSaved      = "Tagihan tersimpan."
	msgInvoiceIssued     = "Tagihan diterbitkan; bagikan tautan atau PDF-nya lewat WhatsApp."
	msgInvoiceVoided     = "Tagihan dibatalkan."
	msgInvoiceLocked     = "Tagihan yang sudah terbit tidak dapat diubah."
	msgInvoiceStale      = "Tagihan ini diubah di tempat lain; muat ulang halaman lalu ulangi."
	msgInvoiceNoDueDate  = "Isi tanggal jatuh tempo sebelum menerbitkan tagihan."
	msgInvoiceEmpty      = "Total tagihan harus lebih dari nol sebelum diterbitkan."
	msgInvoiceHasPayment = "Tagihan ini sudah punya pembayaran. Batalkan pembayarannya dulu sebelum membatalkan tagihan."
	msgInvoiceVoidTwice  = "Tagihan ini sudah dibatalkan."
	msgInvoiceLeadLost   = "Prospek ini sudah Kalah; buka kembali prospek sebelum membuat tagihan."
	msgInvoiceNoLink     = "Tagihan belum terbit; tautan dibuat saat tagihan diterbitkan."
	msgPaymentRecorded   = "Pembayaran tercatat."
	msgPaymentDuplicate  = "Pembayaran ini sudah tercatat."
	msgPaymentNotIssued  = "Pembayaran hanya untuk tagihan yang sudah terbit."
	msgPaymentReversed   = "Pembayaran dibatalkan."
	msgPaymentReversed2  = "Pembayaran ini sudah dibatalkan sebelumnya."
	msgReasonRequired    = "Alasan wajib diisi (maksimal 300 karakter)."
	msgInvoiceSettled    = "Tagihan ini sudah lunas; tidak ada sisa untuk dibayar."
	msgPaymentFormStale  = "Formulir kedaluwarsa, muat ulang halaman."
	msgOverpaymentPrefix = "Jumlah melebihi sisa tagihan ("
	defaultDueDays       = 14
)

// invoiceFormData drives tagihan_form.html.
type invoiceFormData struct {
	Title       string
	Action      string
	Cancel      string
	Lead        lead.Lead
	Rows        []qRow
	Edit        bool
	Version     int
	QuotationID string // source quotation when creating from one
	FromNumber  string // its display number
}

// iView is an invoice prepared for templates (effective status, share URL).
type iView struct {
	I        invoice.Invoice
	Eff      invoice.Status
	ShareURL string
}

type invoiceListData struct {
	Page      invoice.Page
	Rows      []iView
	Q         string
	Chips     []chip
	Pager     Pager
	Status    string
	SumUnpaid int64 // sum of balances on the page's filter when it is the receivables view
}

type invoiceDetailData struct {
	V          iView
	Lead       lead.Lead
	Templates  []template.Template
	FromNumber string // source quotation number when there is one
	Err        string
	// Payment form state: the fresh idempotency key, kinds and methods, typed values and errors.
	PayKey     string
	Kinds      []invoice.PaymentKind
	Methods    []invoice.Method
	PayForm    map[string]string
	PayErrors  map[string]string
	ReverseErr map[string]string // payment id → error for its reversal form
}

var invoiceChips = []struct{ Key, Label string }{
	{"", "Belum lunas"}, {"terlambat", "Terlambat"}, {"lunas", "Lunas"}, {"draf", "Draf"}, {"dibatalkan", "Dibatalkan"}, {"semua", "Semua"},
}

func (s *server) iview(inv invoice.Invoice, today time.Time) iView {
	v := iView{I: inv, Eff: inv.Effective(today)}
	if inv.ShareActive() {
		v.ShareURL = s.BaseURL + "/t/" + inv.ShareToken
	}
	return v
}

func rowsFromItems(items []quotation.Item) []qRow {
	rows := make([]qRow, 0, len(items)+1)
	for _, it := range items {
		rows = append(rows, qRow{Desc: it.Description, Qty: money.FormatQty(it.QtyMilli), Unit: it.Unit, Price: strings.TrimPrefix(money.FormatIDR(it.UnitPrice), "Rp"), Errors: map[string]string{}})
	}
	return append(rows, qRow{Errors: map[string]string{}})
}

func (s *server) loadInvoice(w http.ResponseWriter, r *http.Request) (invoice.Invoice, bool) {
	ws := currentWorkspace(r.Context())
	id, ok := parseID(r)
	if !ok {
		s.notFound(w, r)
		return invoice.Invoice{}, false
	}
	inv, err := s.Invoices.Get(r.Context(), ws.ID, id)
	if errors.Is(err, invoice.ErrNotFound) {
		s.notFound(w, r)
		return invoice.Invoice{}, false
	}
	if err != nil {
		s.serverError(w, r, err)
		return invoice.Invoice{}, false
	}
	return inv, true
}

func (s *server) getTagihanList(w http.ResponseWriter, r *http.Request) {
	ws := currentWorkspace(r.Context())
	qv := r.URL.Query()
	status, q := qv.Get("status"), qv.Get("q")
	known := false
	for _, c := range invoiceChips {
		if c.Key == status {
			known = true
		}
	}
	if !known {
		status = ""
	}
	today := s.today(ws)
	page, err := s.Invoices.List(r.Context(), ws.ID, invoice.ListQuery{Status: status, Q: q, Page: parsePage(r)}, today)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	d := invoiceListData{Page: page, Q: q, Status: status}
	for _, it := range page.Items {
		d.Rows = append(d.Rows, s.iview(it, today))
		if status == "" || status == "terlambat" {
			d.SumUnpaid += it.Balance
		}
	}
	link := func(st string) string {
		p := url.Values{}
		if q != "" {
			p.Set("q", q)
		}
		if st != "" {
			p.Set("status", st)
		}
		if len(p) == 0 {
			return "/app/tagihan"
		}
		return "/app/tagihan?" + p.Encode()
	}
	for _, c := range invoiceChips {
		d.Chips = append(d.Chips, chip{Key: c.Key, Label: c.Label, URL: link(c.Key), Active: status == c.Key})
	}
	params := url.Values{}
	if q != "" {
		params.Set("q", q)
	}
	if status != "" {
		params.Set("status", status)
	}
	d.Pager = newPager("/app/tagihan", params, page.Page, page.Pages)
	v := s.appView(r, "Tagihan", "tagihan")
	v.Flash = s.Sessions.PopString(r.Context(), sessionFlashKey)
	v.Data = d
	s.render(w, r, http.StatusOK, "tagihan_daftar", v)
}

func (s *server) getTagihanBaru(w http.ResponseWriter, r *http.Request) {
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
	d := invoiceFormData{Title: "Buat tagihan", Action: "/app/tagihan", Cancel: "/app/prospek/" + l.ID.String(), Lead: l, Rows: emptyRowsList(emptyRows)}
	v := s.appView(r, "Buat tagihan", "tagihan")
	v.Form["due_on"] = s.today(ws).AddDate(0, 0, defaultDueDays).Format(invoice.DateLayout)
	if raw := r.URL.Query().Get("penawaran"); raw != "" {
		qid, err := uuid.Parse(raw)
		if err != nil {
			s.notFound(w, r)
			return
		}
		q, err := s.Quotations.Get(r.Context(), ws.ID, qid)
		if errors.Is(err, quotation.ErrNotFound) || (err == nil && q.LeadID != l.ID) {
			s.notFound(w, r)
			return
		}
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		d.QuotationID, d.FromNumber = q.ID.String(), q.Number()
		d.Rows = rowsFromItems(q.Items)
		v.Form["discount_pct"], v.Form["tax_pct"], v.Form["notes"] = strconv.Itoa(q.DiscountPct), strconv.Itoa(q.TaxPct), q.Notes
	}
	v.Data = d
	s.render(w, r, http.StatusOK, "tagihan_form", v)
}

func (s *server) postTagihan(w http.ResponseWriter, r *http.Request) {
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
	var fromQuotation *uuid.UUID
	fromNumber := ""
	if raw := strings.TrimSpace(r.PostFormValue("quotation_id")); raw != "" {
		qid, err := uuid.Parse(raw)
		if err != nil {
			s.notFound(w, r)
			return
		}
		fromQuotation = &qid
		if q, err := s.Quotations.Get(r.Context(), ws.ID, qid); err == nil && q.LeadID == l.ID {
			fromNumber = q.Number()
		}
	}
	rows := rowsFromForm(r)
	form := formValues(r, "discount_pct", "tax_pct", "due_on", "notes")
	n, errs := invoice.Validate(invoice.Input{Items: rowsToInput(rows), DiscountPct: form["discount_pct"], TaxPct: form["tax_pct"], DueOn: form["due_on"], Notes: form["notes"]})
	if len(errs) > 0 {
		v := s.appView(r, "Buat tagihan", "tagihan")
		v.Form = form
		v.Errors = applyErrors(rows, errs)
		d := invoiceFormData{Title: "Buat tagihan", Action: "/app/tagihan", Cancel: "/app/prospek/" + l.ID.String(), Lead: l, Rows: rows, FromNumber: fromNumber}
		if fromQuotation != nil {
			d.QuotationID = fromQuotation.String()
		}
		v.Data = d
		s.render(w, r, http.StatusOK, "tagihan_form", v)
		return
	}
	inv, err := s.Invoices.Create(r.Context(), ws.ID, user.ID, l.ID, fromQuotation, n)
	switch {
	case err == nil:
	case errors.Is(err, invoice.ErrLeadNotFound), errors.Is(err, invoice.ErrQuotationNotFound):
		s.notFound(w, r)
		return
	case errors.Is(err, invoice.ErrLeadClosed):
		s.Sessions.Put(r.Context(), sessionFlashKey, msgInvoiceLeadLost)
		http.Redirect(w, r, "/app/prospek/"+l.ID.String(), http.StatusSeeOther)
		return
	default:
		s.serverError(w, r, err)
		return
	}
	s.audit(r, "invoice.create", slog.String("invoice_id", inv.ID.String()), slog.String("lead_id", l.ID.String()), slog.Int("seq", inv.Seq))
	s.Sessions.Put(r.Context(), sessionFlashKey, msgInvoiceSaved)
	http.Redirect(w, r, "/app/tagihan/"+inv.ID.String(), http.StatusSeeOther)
}

// detailInvoice renders the page. payForm/payErrors re-render a refused
// payment with its values; reverseErr marks one reversal form.
func (s *server) detailInvoice(w http.ResponseWriter, r *http.Request, inv invoice.Invoice, errMsg string, payForm, payErrors map[string]string, reverseErr map[string]string) {
	ws := currentWorkspace(r.Context())
	l, err := s.Leads.Get(r.Context(), ws.ID, inv.LeadID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	tpls, err := s.MessageTemplates.ForWorkspace(r.Context(), ws.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	today := s.today(ws)
	d := invoiceDetailData{V: s.iview(inv, today), Lead: l, Templates: tpls, Err: errMsg, Kinds: invoice.Kinds, Methods: invoice.Methods, PayForm: payForm, PayErrors: payErrors, ReverseErr: reverseErr}
	if inv.QuotationID != nil {
		if q, err := s.Quotations.Get(r.Context(), ws.ID, *inv.QuotationID); err == nil {
			d.FromNumber = q.Number()
		}
	}
	if d.PayForm == nil {
		d.PayForm = map[string]string{}
	}
	if d.PayErrors == nil {
		d.PayErrors = map[string]string{}
	}
	if d.ReverseErr == nil {
		d.ReverseErr = map[string]string{}
	}
	if inv.AcceptsPayments() {
		key, err := invoice.NewIdempotencyKey()
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		d.PayKey = key
		if d.PayForm["received_on"] == "" {
			d.PayForm["received_on"] = today.Format(invoice.DateLayout)
		}
		if d.PayForm["kind"] == "" {
			d.PayForm["kind"] = string(invoice.KindDP)
			if inv.Paid > 0 {
				d.PayForm["kind"] = string(invoice.KindPelunasan)
			}
		}
	}
	v := s.appView(r, "Tagihan #"+inv.Number(), "tagihan")
	if errMsg == "" && len(payErrors) == 0 && len(reverseErr) == 0 {
		v.Flash = s.Sessions.PopString(r.Context(), sessionFlashKey)
	}
	v.Data = d
	s.render(w, r, http.StatusOK, "tagihan_detail", v)
}

func (s *server) getTagihan(w http.ResponseWriter, r *http.Request) {
	inv, ok := s.loadInvoice(w, r)
	if !ok {
		return
	}
	s.detailInvoice(w, r, inv, "", nil, nil, nil)
}

func (s *server) getTagihanUbah(w http.ResponseWriter, r *http.Request) {
	inv, ok := s.loadInvoice(w, r)
	if !ok {
		return
	}
	if !inv.Editable() {
		s.Sessions.Put(r.Context(), sessionFlashKey, msgInvoiceLocked)
		http.Redirect(w, r, "/app/tagihan/"+inv.ID.String(), http.StatusSeeOther)
		return
	}
	ws := currentWorkspace(r.Context())
	l, err := s.Leads.Get(r.Context(), ws.ID, inv.LeadID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	v := s.appView(r, "Ubah tagihan #"+inv.Number(), "tagihan")
	v.Form = map[string]string{"discount_pct": strconv.Itoa(inv.DiscountPct), "tax_pct": strconv.Itoa(inv.TaxPct), "notes": inv.Notes}
	if inv.DueOn != nil {
		v.Form["due_on"] = inv.DueOn.Format(invoice.DateLayout)
	}
	v.Data = invoiceFormData{Title: "Ubah tagihan #" + inv.Number(), Action: "/app/tagihan/" + inv.ID.String(), Cancel: "/app/tagihan/" + inv.ID.String(), Lead: l, Rows: rowsFromItems(inv.Items), Edit: true, Version: inv.Version}
	s.render(w, r, http.StatusOK, "tagihan_form", v)
}

func (s *server) postTagihanUbah(w http.ResponseWriter, r *http.Request) {
	inv, ok := s.loadInvoice(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, r)
		return
	}
	ws, user := currentWorkspace(r.Context()), currentUser(r.Context())
	rows := rowsFromForm(r)
	form := formValues(r, "discount_pct", "tax_pct", "due_on", "notes", "version")
	version, _ := strconv.Atoi(form["version"])
	n, errs := invoice.Validate(invoice.Input{Items: rowsToInput(rows), DiscountPct: form["discount_pct"], TaxPct: form["tax_pct"], DueOn: form["due_on"], Notes: form["notes"]})
	if len(errs) == 0 {
		_, err := s.Invoices.Update(r.Context(), ws.ID, user.ID, inv.ID, n, version)
		switch {
		case err == nil:
			s.audit(r, "invoice.update", slog.String("invoice_id", inv.ID.String()))
			s.Sessions.Put(r.Context(), sessionFlashKey, msgChangesSaved)
			http.Redirect(w, r, "/app/tagihan/"+inv.ID.String(), http.StatusSeeOther)
			return
		case errors.Is(err, invoice.ErrLocked):
			s.Sessions.Put(r.Context(), sessionFlashKey, msgInvoiceLocked)
			http.Redirect(w, r, "/app/tagihan/"+inv.ID.String(), http.StatusSeeOther)
			return
		case errors.Is(err, invoice.ErrStale):
			errs = map[string]string{"form": msgInvoiceStale}
		case errors.Is(err, invoice.ErrNotFound), errors.Is(err, invoice.ErrLeadNotFound):
			s.notFound(w, r)
			return
		default:
			s.serverError(w, r, err)
			return
		}
	}
	l, err := s.Leads.Get(r.Context(), ws.ID, inv.LeadID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	v := s.appView(r, "Ubah tagihan #"+inv.Number(), "tagihan")
	v.Form = form
	v.Errors = applyErrors(rows, errs)
	v.Data = invoiceFormData{Title: "Ubah tagihan #" + inv.Number(), Action: "/app/tagihan/" + inv.ID.String(), Cancel: "/app/tagihan/" + inv.ID.String(), Lead: l, Rows: rows, Edit: true, Version: version}
	s.render(w, r, http.StatusOK, "tagihan_form", v)
}

// postTagihanAksi handles terbit and batal; each answers with a redirect and a flash.
func (s *server) postTagihanAksi(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		inv, ok := s.loadInvoice(w, r)
		if !ok {
			return
		}
		ws, user := currentWorkspace(r.Context()), currentUser(r.Context())
		var err error
		var msg string
		if action == "terbit" {
			_, err = s.Invoices.Issue(r.Context(), ws.ID, user.ID, inv.ID)
			msg = msgInvoiceIssued
		} else {
			_, err = s.Invoices.Void(r.Context(), ws.ID, user.ID, inv.ID)
			msg = msgInvoiceVoided
		}
		switch {
		case err == nil:
			s.audit(r, "invoice."+action, slog.String("invoice_id", inv.ID.String()), slog.String("lead_id", inv.LeadID.String()))
		case errors.Is(err, invoice.ErrNoDueDate):
			msg = msgInvoiceNoDueDate
		case errors.Is(err, invoice.ErrEmpty):
			msg = msgInvoiceEmpty
		case errors.Is(err, invoice.ErrHasPayments):
			msg = msgInvoiceHasPayment
		case errors.Is(err, invoice.ErrLocked):
			msg = msgInvoiceLocked
			if action == "batal" {
				msg = msgInvoiceVoidTwice
			}
		case errors.Is(err, invoice.ErrNotFound), errors.Is(err, invoice.ErrLeadNotFound):
			s.notFound(w, r)
			return
		default:
			s.serverError(w, r, err)
			return
		}
		s.Sessions.Put(r.Context(), sessionFlashKey, msg)
		http.Redirect(w, r, "/app/tagihan/"+inv.ID.String(), http.StatusSeeOther)
	}
}

func (s *server) postTagihanTautan(w http.ResponseWriter, r *http.Request) {
	inv, ok := s.loadInvoice(w, r)
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
		_, err = s.Invoices.RevokeShare(r.Context(), ws.ID, inv.ID)
		msg = msgShareRevoked
	case "baru":
		_, err = s.Invoices.RenewShare(r.Context(), ws.ID, inv.ID)
		msg = msgShareRenewed
	default:
		s.badRequest(w, r)
		return
	}
	switch {
	case err == nil:
		s.audit(r, "invoice.share", slog.String("invoice_id", inv.ID.String()), slog.String("aksi", r.PostFormValue("aksi")))
	case errors.Is(err, invoice.ErrLocked):
		msg = msgInvoiceNoLink
	case errors.Is(err, invoice.ErrNotFound):
		s.notFound(w, r)
		return
	default:
		s.serverError(w, r, err)
		return
	}
	s.Sessions.Put(r.Context(), sessionFlashKey, msg)
	http.Redirect(w, r, "/app/tagihan/"+inv.ID.String(), http.StatusSeeOther)
}

// postTagihanBayar records a payment. Validation errors and an overpayment
// re-render the page with the typed values; a repeated submission (same key)
// is answered like a success with its own message.
func (s *server) postTagihanBayar(w http.ResponseWriter, r *http.Request) {
	inv, ok := s.loadInvoice(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, r)
		return
	}
	ws, user := currentWorkspace(r.Context()), currentUser(r.Context())
	form := formValues(r, "kind", "method", "amount", "received_on", "note", "key")
	p, errs := invoice.ValidatePayment(invoice.PaymentInput{Kind: form["kind"], Method: form["method"], Amount: form["amount"], ReceivedOn: form["received_on"], Note: form["note"], IdempotencyKey: form["key"]}, s.today(ws))
	if len(errs) > 0 {
		s.detailInvoice(w, r, inv, "", form, errs, nil)
		return
	}
	pay, created, err := s.Invoices.RecordPayment(r.Context(), ws.ID, user.ID, inv.ID, p)
	var over *invoice.OverpaymentError
	switch {
	case err == nil:
		msg := msgPaymentRecorded
		if !created {
			msg = msgPaymentDuplicate
		}
		s.audit(r, "invoice.payment", slog.String("invoice_id", inv.ID.String()), slog.String("payment_id", pay.ID.String()), slog.Bool("created", created))
		s.Sessions.Put(r.Context(), sessionFlashKey, msg)
		http.Redirect(w, r, "/app/tagihan/"+inv.ID.String(), http.StatusSeeOther)
	case errors.As(err, &over) && over.Balance <= 0:
		// A stale form after the invoice settled: the form is gone, so say it in a flash.
		s.Sessions.Put(r.Context(), sessionFlashKey, msgInvoiceSettled)
		http.Redirect(w, r, "/app/tagihan/"+inv.ID.String(), http.StatusSeeOther)
	case errors.As(err, &over):
		errs = map[string]string{"amount": msgOverpaymentPrefix + money.FormatIDR(over.Balance) + ")."}
		s.detailInvoice(w, r, inv, "", form, errs, nil)
	case errors.Is(err, invoice.ErrKeyMismatch):
		s.detailInvoice(w, r, inv, "", form, map[string]string{"form": msgPaymentFormStale}, nil)
	case errors.Is(err, invoice.ErrNotIssued):
		s.Sessions.Put(r.Context(), sessionFlashKey, msgPaymentNotIssued)
		http.Redirect(w, r, "/app/tagihan/"+inv.ID.String(), http.StatusSeeOther)
	case errors.Is(err, invoice.ErrNotFound):
		s.notFound(w, r)
	default:
		s.serverError(w, r, err)
	}
}

func (s *server) postTagihanPembayaranBatal(w http.ResponseWriter, r *http.Request) {
	inv, ok := s.loadInvoice(w, r)
	if !ok {
		return
	}
	pid, err := uuid.Parse(chi.URLParam(r, "pid"))
	if err != nil {
		s.notFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.badRequest(w, r)
		return
	}
	// The payment must be one of this invoice's; the store scopes by workspace.
	found := false
	for _, p := range inv.Payments {
		if p.ID == pid {
			found = true
		}
	}
	if !found {
		s.notFound(w, r)
		return
	}
	ws, user := currentWorkspace(r.Context()), currentUser(r.Context())
	reason := strings.TrimSpace(r.PostFormValue("reason"))
	_, err = s.Invoices.ReversePayment(r.Context(), ws.ID, user.ID, pid, reason)
	switch {
	case err == nil:
		s.audit(r, "invoice.payment.reverse", slog.String("invoice_id", inv.ID.String()), slog.String("payment_id", pid.String()))
		s.Sessions.Put(r.Context(), sessionFlashKey, msgPaymentReversed)
		http.Redirect(w, r, "/app/tagihan/"+inv.ID.String(), http.StatusSeeOther)
	case errors.Is(err, invoice.ErrInvalidReason):
		s.detailInvoice(w, r, inv, "", nil, nil, map[string]string{pid.String(): msgReasonRequired})
	case errors.Is(err, invoice.ErrAlreadyReversed):
		s.Sessions.Put(r.Context(), sessionFlashKey, msgPaymentReversed2)
		http.Redirect(w, r, "/app/tagihan/"+inv.ID.String(), http.StatusSeeOther)
	case errors.Is(err, invoice.ErrNotFound):
		s.notFound(w, r)
	default:
		s.serverError(w, r, err)
	}
}

// postTagihanWa opens WhatsApp with {nilai} = the remaining balance and
// {tautan} = this invoice's link; the lead's history records the template.
func (s *server) postTagihanWa(w http.ResponseWriter, r *http.Request) {
	inv, ok := s.loadInvoice(w, r)
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
	if inv.CustomerPhone == "" {
		s.Sessions.Put(r.Context(), sessionFlashKey, msgNoPhone)
		http.Redirect(w, r, "/app/tagihan/"+inv.ID.String(), http.StatusSeeOther)
		return
	}
	tpl, err := s.MessageTemplates.Get(r.Context(), ws.ID, key)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if err := s.Leads.RecordWhatsApp(r.Context(), ws.ID, user.ID, inv.LeadID, key.Label()); err != nil {
		s.serverError(w, r, err)
		return
	}
	data := template.Data{Nama: inv.CustomerName, Usaha: ws.Name, Nilai: money.FormatIDR(inv.Balance)}
	if inv.ShareActive() {
		data.Tautan = s.BaseURL + "/t/" + inv.ShareToken
	}
	s.audit(r, "invoice.whatsapp", slog.String("invoice_id", inv.ID.String()), slog.String("template", string(key)))
	// The target host is fixed (https://wa.me/); only the number and the message text vary.
	http.Redirect(w, r, template.WaLink(inv.CustomerPhone, template.Render(tpl.Body, data)), http.StatusSeeOther) //nolint:gosec // G710: fixed wa.me host
}

func (s *server) getTagihanPDF(w http.ResponseWriter, r *http.Request) {
	inv, ok := s.loadInvoice(w, r)
	if !ok {
		return
	}
	ws := currentWorkspace(r.Context())
	s.writeInvoicePDF(w, r, inv, location(ws.Timezone), s.today(ws))
}

func (s *server) writeInvoicePDF(w http.ResponseWriter, r *http.Request, inv invoice.Invoice, loc *time.Location, today time.Time) {
	var buf bytes.Buffer
	if err := invoice.RenderPDF(&buf, inv, invoice.PDFOptions{Compress: true, CreatedAt: inv.UpdatedAt, Loc: loc, Today: today}); err != nil {
		s.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="tagihan-`+inv.Number()+`.pdf"`)
	w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
	w.WriteHeader(http.StatusOK)
	_, _ = buf.WriteTo(w)
}
