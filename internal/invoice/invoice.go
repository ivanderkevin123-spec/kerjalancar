// Package invoice owns invoices, their copied items, append-only payments and
// reversals (Milestone 6, D-056..D-060). An invoice belongs to one lead and
// that lead's customer; items are copied from a quotation or typed in.
// Payments are the owner's notes of money received (D-011): never edited,
// undone only by a reversal row with a reason. The balance is computed in SQL.
package invoice

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"kerjalancar/internal/money"
	"kerjalancar/internal/quotation"
)

// Status is the stored state (draft, issued, void) or, from Effective, the
// derived state shown to people (lunas, terlambat).
type Status string

const (
	StatusDraft   Status = "draft"
	StatusIssued  Status = "issued"
	StatusVoid    Status = "void"
	StatusPaid    Status = "lunas"     // derived: issued and nothing left to pay
	StatusOverdue Status = "terlambat" // derived: issued, unpaid, past the due date
)

var statusLabels = map[Status]string{
	StatusDraft: "Draf", StatusIssued: "Terbit", StatusVoid: "Dibatalkan", StatusPaid: "Lunas", StatusOverdue: "Terlambat",
}

func (s Status) Label() string { return statusLabels[s] }

// PaymentKind is what the payment is for, as the contractor says it.
type PaymentKind string

const (
	KindDP        PaymentKind = "dp"
	KindCicilan   PaymentKind = "cicilan"
	KindPelunasan PaymentKind = "pelunasan"
)

// Kinds in display order.
var Kinds = []PaymentKind{KindDP, KindCicilan, KindPelunasan}

var kindLabels = map[PaymentKind]string{KindDP: "DP", KindCicilan: "Cicilan", KindPelunasan: "Pelunasan"}

func (k PaymentKind) Label() string { return kindLabels[k] }
func (k PaymentKind) Valid() bool   { _, ok := kindLabels[k]; return ok }

// Method is how the money arrived; informational only (no bank data, D-011).
type Method string

const (
	MethodTransfer Method = "transfer"
	MethodTunai    Method = "tunai"
	MethodLainnya  Method = "lainnya"
)

// Methods in display order.
var Methods = []Method{MethodTransfer, MethodTunai, MethodLainnya}

var methodLabels = map[Method]string{MethodTransfer: "Transfer", MethodTunai: "Tunai", MethodLainnya: "Lainnya"}

func (m Method) Label() string { return methodLabels[m] }
func (m Method) Valid() bool   { _, ok := methodLabels[m]; return ok }

// Reversal undoes one payment; the payment row itself never changes.
type Reversal struct {
	ID             uuid.UUID
	Reason         string
	ReversedBy     *uuid.UUID
	ReversedByName string
	CreatedAt      time.Time
}

// Payment is one recorded receipt. Reversal is nil while it counts.
type Payment struct {
	ID             uuid.UUID
	InvoiceID      uuid.UUID
	Kind           PaymentKind
	Method         Method
	Amount         int64
	ReceivedOn     time.Time
	Note           string
	RecordedBy     *uuid.UUID
	RecordedByName string
	CreatedAt      time.Time
	Reversal       *Reversal
}

// Active reports whether the payment still counts toward the balance.
func (p Payment) Active() bool { return p.Reversal == nil }

// Invoice is one bill for a lead. Paid and Balance come from SQL.
type Invoice struct {
	ID             uuid.UUID
	WorkspaceID    uuid.UUID
	LeadID         uuid.UUID
	CustomerID     uuid.UUID
	QuotationID    *uuid.UUID
	LeadTitle      string
	CustomerName   string
	CustomerPhone  string
	WorkspaceName  string
	Seq            int
	Status         Status
	DueOn          *time.Time
	Notes          string
	DiscountPct    int
	TaxPct         int
	Subtotal       int64
	DiscountAmount int64
	TaxAmount      int64
	Total          int64
	Paid           int64
	Balance        int64
	ShareToken     string
	ShareRevokedAt *time.Time
	FirstOpenedAt  *time.Time
	OpenCount      int
	IssuedAt       *time.Time
	VoidedAt       *time.Time
	Version        int
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Items          []quotation.Item
	Payments       []Payment
}

// Number is the display number ("0001"), per workspace.
func (i Invoice) Number() string { return quotation.FormatNumber(i.Seq) }

// Effective is the status shown for today (a date from followup.Today).
func (i Invoice) Effective(today time.Time) Status {
	if i.Status != StatusIssued {
		return i.Status
	}
	if i.Balance <= 0 {
		return StatusPaid
	}
	if i.DueOn != nil && i.DueOn.Before(today) {
		return StatusOverdue
	}
	return StatusIssued
}

// Editable: items and terms change only while the invoice is a draft.
func (i Invoice) Editable() bool { return i.Status == StatusDraft }

// AcceptsPayments: issued and not yet settled.
func (i Invoice) AcceptsPayments() bool { return i.Status == StatusIssued && i.Balance > 0 }

// ShareActive reports whether the public link currently works.
func (i Invoice) ShareActive() bool {
	return i.Status == StatusIssued && i.ShareToken != "" && i.ShareRevokedAt == nil
}

// Date is the document date: when it was issued, else when it was created.
func (i Invoice) Date() time.Time {
	if i.IssuedAt != nil {
		return *i.IssuedAt
	}
	return i.CreatedAt
}

// Input is the raw invoice form; items and percentages share the quotation editor.
type Input struct {
	Items       []quotation.ItemInput
	DiscountPct string
	TaxPct      string
	DueOn       string
	Notes       string
}

// Normalized is a validated form with totals.
type Normalized struct {
	Items       []quotation.NormalizedItem
	DiscountPct int
	TaxPct      int
	DueOn       *time.Time
	Notes       string
	quotation.Totals
}

const (
	MaxNoteLen = 300
	DateLayout = quotation.DateLayout
)

const (
	msgDateInvalid   = "Tanggal tidak valid."
	msgDateFuture    = "Tanggal tidak boleh di masa depan."
	msgKindInvalid   = "Jenis pembayaran tidak valid."
	msgMethodInvalid = "Metode tidak valid."
	msgAmountInvalid = "Jumlah tidak valid. Contoh: 5.000.000"
	msgNoteLong      = "Catatan maksimal 300 karakter."
	msgFormStale     = "Formulir kedaluwarsa, muat ulang halaman."
)

// Validate normalises the invoice form. The due date may stay empty on a
// draft; Issue refuses an invoice without one.
func Validate(in Input) (Normalized, map[string]string) {
	qn, errs := quotation.Validate(quotation.Input{Items: in.Items, DiscountPct: in.DiscountPct, TaxPct: in.TaxPct, Notes: in.Notes})
	if errs == nil {
		errs = map[string]string{}
	}
	n := Normalized{Items: qn.Items, DiscountPct: qn.DiscountPct, TaxPct: qn.TaxPct, Notes: qn.Notes, Totals: qn.Totals}
	if d := strings.TrimSpace(in.DueOn); d != "" {
		t, err := time.Parse(DateLayout, d)
		if err != nil {
			errs["due_on"] = msgDateInvalid
		} else {
			n.DueOn = &t
		}
	}
	return n, errs
}

// PaymentInput is the raw payment form.
type PaymentInput struct {
	Kind           string
	Method         string
	Amount         string
	ReceivedOn     string
	Note           string
	IdempotencyKey string
}

// NormalizedPayment is a validated payment form.
type NormalizedPayment struct {
	Kind           PaymentKind
	Method         Method
	Amount         int64
	ReceivedOn     time.Time
	Note           string
	IdempotencyKey string
}

var keyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{22}$`)

// ValidatePayment checks the form against today (the workspace's day): the
// receipt date may not be in the future, the amount is positive rupiah, and
// the idempotency key must be one the server issued with the form.
func ValidatePayment(in PaymentInput, today time.Time) (NormalizedPayment, map[string]string) {
	errs := map[string]string{}
	p := NormalizedPayment{Kind: PaymentKind(strings.TrimSpace(in.Kind)), Method: Method(strings.TrimSpace(in.Method)), Note: strings.TrimSpace(in.Note), IdempotencyKey: strings.TrimSpace(in.IdempotencyKey)}
	if !p.Kind.Valid() {
		errs["kind"] = msgKindInvalid
	}
	if !p.Method.Valid() {
		errs["method"] = msgMethodInvalid
	}
	amount, err := money.ParseIDR(in.Amount)
	if err != nil || amount <= 0 {
		errs["amount"] = msgAmountInvalid
	}
	p.Amount = amount
	d := strings.TrimSpace(in.ReceivedOn)
	t, err := time.Parse(DateLayout, d)
	switch {
	case d == "" || err != nil:
		errs["received_on"] = msgDateInvalid
	case t.After(today):
		errs["received_on"] = msgDateFuture
	default:
		p.ReceivedOn = t
	}
	if utf8.RuneCountInString(p.Note) > MaxNoteLen {
		errs["note"] = msgNoteLong
	}
	if !keyPattern.MatchString(p.IdempotencyKey) {
		errs["form"] = msgFormStale
	}
	return p, errs
}

// NewIdempotencyKey returns 16 random bytes as 22 base64url characters; the
// payment form carries one so a double submit records one payment.
func NewIdempotencyKey() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("invoice: key: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// OverpaymentError says how much could still be paid.
type OverpaymentError struct {
	Balance int64
}

func (e *OverpaymentError) Error() string {
	return fmt.Sprintf("invoice: amount exceeds the balance of %d", e.Balance)
}

func (e *OverpaymentError) Is(target error) bool { return target == ErrOverpayment }

var (
	ErrNotFound          = errors.New("invoice: not found")
	ErrLeadNotFound      = errors.New("invoice: lead not found in workspace")
	ErrLeadClosed        = errors.New("invoice: lead was lost; no invoices for it")
	ErrQuotationNotFound = errors.New("invoice: quotation not found for this lead")
	ErrLocked            = errors.New("invoice: only drafts change; issued and void invoices are fixed")
	ErrStale             = errors.New("invoice: edited elsewhere; reload")
	ErrNotIssued         = errors.New("invoice: payments need an issued invoice")
	ErrNoDueDate         = errors.New("invoice: a due date is needed to issue")
	ErrEmpty             = errors.New("invoice: total must be above zero to issue")
	ErrOverpayment       = errors.New("invoice: amount exceeds the balance")
	ErrHasPayments       = errors.New("invoice: reverse its payments before voiding")
	ErrAlreadyReversed   = errors.New("invoice: payment already reversed")
	ErrInvalidReason     = errors.New("invoice: a reason of 1-300 characters is required")
	ErrKeyMismatch       = errors.New("invoice: idempotency key belongs to another invoice")
)
