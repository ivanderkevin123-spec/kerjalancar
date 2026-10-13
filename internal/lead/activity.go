package lead

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Kind classifies an activity. New kinds (M4 follow-ups, M5 quotations, M6
// invoices and payments) are rows in lead_activity_kinds (migration 00005+).
type Kind string

const (
	KindCreated             Kind = "created"
	KindStatusChanged       Kind = "status_changed"
	KindNote                Kind = "note"
	KindUpdated             Kind = "updated"
	KindFollowUpScheduled   Kind = "follow_up_scheduled"
	KindFollowUpRescheduled Kind = "follow_up_rescheduled"
	KindFollowUpDone        Kind = "follow_up_done"
	KindFollowUpUndone      Kind = "follow_up_undone"
	KindWhatsAppOpened      Kind = "whatsapp_opened"
	KindQuotationCreated    Kind = "quotation_created"
	KindQuotationSent       Kind = "quotation_sent"
	KindQuotationAccepted   Kind = "quotation_accepted"
	KindQuotationRejected   Kind = "quotation_rejected"
	KindInvoiceCreated      Kind = "invoice_created"
	KindInvoiceIssued       Kind = "invoice_issued"
	KindInvoiceVoided       Kind = "invoice_voided"
	KindPaymentRecorded     Kind = "payment_recorded"
	KindPaymentReversed     Kind = "payment_reversed"
)

// Change records one edited field for an "updated" activity. From/To are
// display strings for short fields; long text fields carry no values.
type Change struct {
	Field string `json:"field"`
	From  string `json:"from,omitempty"`
	To    string `json:"to,omitempty"`
}

// ActivityData is the structured part of an activity, stored as jsonb.
type ActivityData struct {
	From    Status   `json:"from,omitempty"`
	To      Status   `json:"to,omitempty"`
	Reason  string   `json:"reason,omitempty"`
	Changes []Change `json:"changes,omitempty"`
	// Follow-up fields: dates as "2006-01-02"; Auto marks a close done by the system when a lead closed.
	DueOn     string `json:"due_on,omitempty"`
	PrevDueOn string `json:"prev_due_on,omitempty"`
	Auto      bool   `json:"auto,omitempty"`
	// Template is the label of the WhatsApp template that was opened.
	Template string `json:"template,omitempty"`
	// Quotation and invoice fields: the number ("0001") and the total as formatted rupiah.
	Number string `json:"number,omitempty"`
	Total  string `json:"total,omitempty"`
	// Payment fields: the amount as formatted rupiah and the kind label ("DP").
	Amount string `json:"amount,omitempty"`
	Label  string `json:"label,omitempty"`
}

// Activity is one line of a lead's business history (D-036).
type Activity struct {
	ID          uuid.UUID
	WorkspaceID uuid.UUID
	LeadID      uuid.UUID
	Kind        Kind
	ActorID     *uuid.UUID
	ActorName   string // joined from users; "" when the actor is gone
	Body        string // note text
	Data        ActivityData
	CreatedAt   time.Time
}

var fieldLabels = map[string]string{
	"title": "Nama proyek", "project_type": "Jenis proyek", "description": "Deskripsi",
	"estimated_value": "Nilai estimasi", "source": "Sumber", "next_follow_up_on": "Tanggal follow-up",
	"notes": "Catatan", "customer": "Pelanggan",
}

// FieldLabel returns the Indonesian label of a lead field.
func FieldLabel(field string) string {
	if l, ok := fieldLabels[field]; ok {
		return l
	}
	return field
}

// Describe renders the one-line Indonesian summary shown in the timeline.
func (a Activity) Describe() string {
	switch a.Kind {
	case KindCreated:
		return "Prospek dibuat"
	case KindStatusChanged:
		s := "Status: " + a.Data.From.Label() + " → " + a.Data.To.Label()
		if a.Data.Reason != "" {
			s += " · Alasan: " + a.Data.Reason
		}
		return s
	case KindNote:
		return "Catatan ditambahkan"
	case KindFollowUpScheduled:
		return "Follow-up dijadwalkan: " + dateLabel(a.Data.DueOn)
	case KindFollowUpRescheduled:
		return "Follow-up dijadwalkan ulang: " + dateLabel(a.Data.PrevDueOn) + " → " + dateLabel(a.Data.DueOn)
	case KindFollowUpDone:
		if a.Data.Auto {
			return "Follow-up selesai (prospek ditutup)"
		}
		return "Follow-up selesai"
	case KindFollowUpUndone:
		return "Follow-up dibuka kembali"
	case KindWhatsAppOpened:
		return "Pesan WhatsApp dibuka: " + a.Data.Template
	case KindQuotationCreated:
		return "Penawaran #" + a.Data.Number + " dibuat (" + a.Data.Total + ")"
	case KindQuotationSent:
		return "Penawaran #" + a.Data.Number + " dikirim (" + a.Data.Total + ")"
	case KindQuotationAccepted:
		return "Penawaran #" + a.Data.Number + " diterima (" + a.Data.Total + ")"
	case KindQuotationRejected:
		return "Penawaran #" + a.Data.Number + " ditolak"
	case KindInvoiceCreated:
		return "Tagihan #" + a.Data.Number + " dibuat (" + a.Data.Total + ")"
	case KindInvoiceIssued:
		return "Tagihan #" + a.Data.Number + " diterbitkan (" + a.Data.Total + ")"
	case KindInvoiceVoided:
		return "Tagihan #" + a.Data.Number + " dibatalkan"
	case KindPaymentRecorded:
		return "Pembayaran " + a.Data.Amount + " dicatat untuk tagihan #" + a.Data.Number + " (" + a.Data.Label + ")"
	case KindPaymentReversed:
		return "Pembayaran " + a.Data.Amount + " dibatalkan untuk tagihan #" + a.Data.Number
	case KindUpdated:
		parts := make([]string, 0, len(a.Data.Changes))
		for _, c := range a.Data.Changes {
			label := FieldLabel(c.Field)
			if c.From == "" && c.To == "" {
				parts = append(parts, label)
				continue
			}
			parts = append(parts, label+" ("+dash(c.From)+" → "+dash(c.To)+")")
		}
		return "Diperbarui: " + strings.Join(parts, ", ")
	}
	return string(a.Kind)
}

// dateLabel renders a stored "2006-01-02" as "23 Okt 2026".
func dateLabel(s string) string {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return dash(s)
	}
	months := [...]string{"", "Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}
	return fmt.Sprintf("%d %s %d", t.Day(), months[t.Month()], t.Year())
}

func dash(s string) string {
	if s == "" {
		return "–"
	}
	return s
}
