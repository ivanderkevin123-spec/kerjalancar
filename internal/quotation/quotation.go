// Package quotation owns quotations: items in thousandths with server-computed
// integer-rupiah totals (D-010), per-workspace numbering, status rules, and
// share links (D-050..D-054).
package quotation

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"kerjalancar/internal/money"
)

// Status is a stored state; StatusExpired is computed from valid_until.
type Status string

const (
	StatusDraft    Status = "draft"
	StatusSent     Status = "sent"
	StatusAccepted Status = "accepted"
	StatusRejected Status = "rejected"
	StatusExpired  Status = "expired"
)

// Statuses in display order (for filters).
var Statuses = []Status{StatusDraft, StatusSent, StatusAccepted, StatusRejected, StatusExpired}

var statusLabels = map[Status]string{
	StatusDraft: "Draf", StatusSent: "Terkirim", StatusAccepted: "Diterima", StatusRejected: "Ditolak", StatusExpired: "Kedaluwarsa",
}

func (s Status) Label() string { return statusLabels[s] }
func (s Status) Valid() bool   { _, ok := statusLabels[s]; return ok }

// Item is one line of a quotation.
type Item struct {
	ID          uuid.UUID
	Position    int
	Description string
	QtyMilli    int64
	Unit        string
	UnitPrice   int64
	LineTotal   int64
}

// Quotation is one offer to a customer for a lead, with joined display fields.
type Quotation struct {
	ID             uuid.UUID
	WorkspaceID    uuid.UUID
	LeadID         uuid.UUID
	CustomerID     uuid.UUID
	LeadTitle      string
	CustomerName   string
	CustomerPhone  string
	WorkspaceName  string
	Seq            int
	Status         Status
	ValidUntil     *time.Time
	Notes          string
	DiscountPct    int
	TaxPct         int
	Subtotal       int64
	DiscountAmount int64
	TaxAmount      int64
	Total          int64
	ShareToken     string // "" when none; see ShareActive
	ShareRevokedAt *time.Time
	FirstOpenedAt  *time.Time
	OpenCount      int
	SentAt         *time.Time
	AcceptedAt     *time.Time
	RejectedAt     *time.Time
	Version        int
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Items          []Item
}

// Number renders the per-workspace sequence as "0001".
func (q Quotation) Number() string { return FormatNumber(q.Seq) }

// FormatNumber pads the sequence to four digits.
func FormatNumber(seq int) string { return fmt.Sprintf("%04d", seq) }

// EffectiveStatus is the stored status, or expired for a sent quotation whose
// validity ended before today (a date at UTC midnight).
func (q Quotation) EffectiveStatus(today time.Time) Status {
	if q.Status == StatusSent && q.ValidUntil != nil && q.ValidUntil.Before(today) {
		return StatusExpired
	}
	return q.Status
}

// Editable: items and terms may change while the quotation is a draft or sent.
func (q Quotation) Editable() bool { return q.Status == StatusDraft || q.Status == StatusSent }

// ShareActive reports whether the share link currently works (issued, not revoked).
func (q Quotation) ShareActive() bool { return q.ShareToken != "" && q.ShareRevokedAt == nil }

// ItemInput is one raw row of the editor.
type ItemInput struct {
	Description string
	Qty         string
	Unit        string
	UnitPrice   string
}

// Blank reports an untouched row, which the editor ignores.
func (i ItemInput) Blank() bool {
	return strings.TrimSpace(i.Description) == "" && strings.TrimSpace(i.Qty) == "" && strings.TrimSpace(i.Unit) == "" && strings.TrimSpace(i.UnitPrice) == ""
}

// Input is the raw quotation form.
type Input struct {
	Items       []ItemInput
	DiscountPct string
	TaxPct      string
	ValidUntil  string
	Notes       string
}

// NormalizedItem is a validated row with its line total.
type NormalizedItem struct {
	Description string
	QtyMilli    int64
	Unit        string
	UnitPrice   int64
	LineTotal   int64
}

// Totals are the server-computed amounts in rupiah.
type Totals struct {
	Subtotal       int64
	DiscountAmount int64
	TaxAmount      int64
	Total          int64
}

// Normalized is a validated form with totals.
type Normalized struct {
	Items       []NormalizedItem
	DiscountPct int
	TaxPct      int
	ValidUntil  *time.Time
	Notes       string
	Totals
}

const (
	MaxDescriptionLen = 300
	MaxUnitLen        = 20
	MaxNotesLen       = 2000
	MaxItems          = 100
	DateLayout        = "2006-01-02"
)

const (
	msgItemsRequired  = "Tambahkan minimal satu baris."
	msgTooManyItems   = "Maksimal 100 baris."
	msgDescRequired   = "Uraian wajib diisi."
	msgDescLong       = "Uraian maksimal 300 karakter."
	msgQtyInvalid     = "Jumlah tidak valid (contoh: 2,5)."
	msgUnitLong       = "Satuan maksimal 20 karakter."
	msgPriceInvalid   = "Harga satuan tidak valid. Contoh: 150.000"
	msgDiscountRange  = "Diskon harus 0–100 persen."
	msgTaxRange       = "Pajak harus 0–100 persen."
	msgDateInvalid    = "Tanggal tidak valid."
	msgNotesLong      = "Catatan maksimal 2000 karakter."
	msgTotalTooLarge  = "Total melebihi batas."
	msgLineTooLarge   = "Jumlah baris melebihi batas."
	msgOverflowFields = "items"
)

// ValidationError carries field messages keyed by form field ("items.0.qty").
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string { return fmt.Sprintf("quotation: invalid input %v", e.Fields) }

// Validate normalises the form, computes line totals and the grand totals.
// Blank rows are skipped; row indexes in error keys refer to the submitted rows.
func Validate(in Input) (Normalized, map[string]string) {
	errs := map[string]string{}
	n := Normalized{Notes: strings.TrimSpace(in.Notes)}
	for i, row := range in.Items {
		if row.Blank() {
			continue
		}
		key := "items." + strconv.Itoa(i) + "."
		it := NormalizedItem{Description: strings.TrimSpace(row.Description), Unit: strings.TrimSpace(row.Unit)}
		switch {
		case it.Description == "":
			errs[key+"description"] = msgDescRequired
		case utf8.RuneCountInString(it.Description) > MaxDescriptionLen:
			errs[key+"description"] = msgDescLong
		}
		qty, err := money.ParseQty(row.Qty)
		if err != nil {
			errs[key+"qty"] = msgQtyInvalid
		}
		it.QtyMilli = qty
		if utf8.RuneCountInString(it.Unit) > MaxUnitLen {
			errs[key+"unit"] = msgUnitLong
		}
		price, err := money.ParseIDR(row.UnitPrice)
		if err != nil {
			errs[key+"unit_price"] = msgPriceInvalid
		}
		it.UnitPrice = price
		if qty > 0 && price >= 0 && errs[key+"qty"] == "" && errs[key+"unit_price"] == "" {
			lt, err := money.LineTotal(qty, price)
			if err != nil {
				errs[key+"unit_price"] = msgLineTooLarge
			}
			it.LineTotal = lt
		}
		n.Items = append(n.Items, it)
	}
	switch {
	case len(n.Items) == 0:
		errs["items"] = msgItemsRequired
	case len(n.Items) > MaxItems:
		errs["items"] = msgTooManyItems
	}
	n.DiscountPct = percent(in.DiscountPct, "discount_pct", msgDiscountRange, errs)
	n.TaxPct = percent(in.TaxPct, "tax_pct", msgTaxRange, errs)
	if v := strings.TrimSpace(in.ValidUntil); v != "" {
		d, err := time.Parse(DateLayout, v)
		if err != nil {
			errs["valid_until"] = msgDateInvalid
		} else {
			n.ValidUntil = &d
		}
	}
	if utf8.RuneCountInString(n.Notes) > MaxNotesLen {
		errs["notes"] = msgNotesLong
	}
	if len(errs) == 0 {
		totals, err := ComputeTotals(n.Items, n.DiscountPct, n.TaxPct)
		if err != nil {
			errs[msgOverflowFields] = msgTotalTooLarge
		}
		n.Totals = totals
	}
	return n, errs
}

func percent(raw, field, msg string, errs map[string]string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 0 || v > 100 {
		errs[field] = msg
		return 0
	}
	return v
}

// ComputeTotals: subtotal = Σ line totals; discount = pct of subtotal (half up);
// tax = pct of (subtotal − discount) (half up); total = subtotal − discount + tax.
func ComputeTotals(items []NormalizedItem, discountPct, taxPct int) (Totals, error) {
	var t Totals
	for _, it := range items {
		t.Subtotal += it.LineTotal
		if t.Subtotal > money.MaxIDR {
			return Totals{}, money.ErrInvalidAmount
		}
	}
	t.DiscountAmount = money.PercentOf(t.Subtotal, discountPct)
	taxable := t.Subtotal - t.DiscountAmount
	t.TaxAmount = money.PercentOf(taxable, taxPct)
	t.Total = taxable + t.TaxAmount
	if t.Total > money.MaxIDR {
		return Totals{}, money.ErrInvalidAmount
	}
	return t, nil
}
