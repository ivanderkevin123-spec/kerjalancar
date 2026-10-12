package lead

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// Kind classifies an activity. New kinds (M4 follow-ups, M5 quotations) are
// added by widening the CHECK constraint in a later migration.
type Kind string

const (
	KindCreated       Kind = "created"
	KindStatusChanged Kind = "status_changed"
	KindNote          Kind = "note"
	KindUpdated       Kind = "updated"
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

func dash(s string) string {
	if s == "" {
		return "–"
	}
	return s
}
