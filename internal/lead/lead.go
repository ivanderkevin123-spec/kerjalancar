// Package lead owns project opportunities: their pipeline status, validation,
// transitions, and the append-only activity history. Every store call is
// scoped by the workspace id taken from the session (D-033..D-036).
package lead

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"kerjalancar/internal/money"
)

// Status is a pipeline stage. Six stages; "negosiasi" is folded into
// penawaran (D-035).
type Status string

const (
	StatusBaru      Status = "baru"
	StatusDihubungi Status = "dihubungi"
	StatusSurvei    Status = "survei"
	StatusPenawaran Status = "penawaran"
	StatusMenang    Status = "menang"
	StatusKalah     Status = "kalah"
)

// Statuses in pipeline order, used for filters, selects and summaries.
var Statuses = []Status{StatusBaru, StatusDihubungi, StatusSurvei, StatusPenawaran, StatusMenang, StatusKalah}

var statusLabels = map[Status]string{
	StatusBaru: "Baru", StatusDihubungi: "Dihubungi", StatusSurvei: "Survei",
	StatusPenawaran: "Penawaran", StatusMenang: "Menang", StatusKalah: "Kalah",
}

func (s Status) Label() string { return statusLabels[s] }
func (s Status) Valid() bool   { _, ok := statusLabels[s]; return ok }

// Open reports whether the lead is still in play (not won or lost).
func (s Status) Open() bool { return s.Valid() && s != StatusMenang && s != StatusKalah }

// ParseStatus accepts the stored/form value of a status.
func ParseStatus(v string) (Status, bool) {
	s := Status(strings.TrimSpace(v))
	return s, s.Valid()
}

var (
	ErrNotFound         = errors.New("lead: not found")
	ErrCustomerNotFound = errors.New("lead: customer not found in workspace")
	ErrSameStatus       = errors.New("lead: status unchanged")
	ErrClosedToClosed   = errors.New("lead: reopen the lead before closing it the other way")
	ErrInvalidStatus    = errors.New("lead: unknown status")
)

// CanTransition applies the pipeline rule: any change to a different status is
// allowed, except directly between menang and kalah (reopen first).
func CanTransition(from, to Status) error {
	if !to.Valid() {
		return ErrInvalidStatus
	}
	if from == to {
		return ErrSameStatus
	}
	if !from.Open() && !to.Open() {
		return ErrClosedToClosed
	}
	return nil
}

// Source is where the lead came from.
type Source string

var Sources = []Source{"whatsapp", "instagram", "facebook", "referensi", "google", "marketplace", "lainnya"}

var sourceLabels = map[Source]string{
	"whatsapp": "WhatsApp", "instagram": "Instagram", "facebook": "Facebook", "referensi": "Referensi",
	"google": "Google / situs web", "marketplace": "Marketplace", "lainnya": "Lainnya",
}

func (s Source) Label() string { return sourceLabels[s] }
func (s Source) Valid() bool   { _, ok := sourceLabels[s]; return ok }

// ProjectType is a coarse classification useful for filtering later.
type ProjectType string

var ProjectTypes = []ProjectType{"renovasi", "interior", "bangun_baru", "perbaikan", "lainnya"}

var projectTypeLabels = map[ProjectType]string{
	"renovasi": "Renovasi", "interior": "Interior", "bangun_baru": "Bangun baru", "perbaikan": "Perbaikan", "lainnya": "Lainnya",
}

func (p ProjectType) Label() string { return projectTypeLabels[p] }
func (p ProjectType) Valid() bool   { _, ok := projectTypeLabels[p]; return ok }

// Lead is one project opportunity with its customer's name joined in for display.
type Lead struct {
	ID              uuid.UUID
	WorkspaceID     uuid.UUID
	CustomerID      uuid.UUID
	CustomerName    string
	CustomerPhone   string
	Title           string
	ProjectType     ProjectType
	Description     string
	EstimatedValue  *int64 // rupiah; nil = no estimate yet
	Status          Status
	Source          Source
	NextFollowUpOn  *time.Time // date only (UTC midnight)
	LostReason      string
	Notes           string
	StatusChangedAt time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Input is the raw lead form. Validate turns it into Normalized.
type Input struct {
	Title          string
	ProjectType    string
	Description    string
	EstimatedValue string
	Source         string
	NextFollowUpOn string // "2006-01-02"
	Notes          string
}

// Normalized is a validated lead form.
type Normalized struct {
	Title          string
	ProjectType    ProjectType
	Description    string
	EstimatedValue *int64
	Source         Source
	NextFollowUpOn *time.Time
	Notes          string
}

const (
	MaxTitleLen       = 150
	MaxDescriptionLen = 2000
	MaxNotesLen       = 2000
	MaxLostReasonLen  = 500
	DateLayout        = "2006-01-02"
)

const (
	msgTitleRequired = "Nama proyek wajib diisi."
	msgTitleLong     = "Nama proyek maksimal 150 karakter."
	msgTypeUnknown   = "Jenis proyek tidak dikenal."
	msgDescLong      = "Deskripsi maksimal 2000 karakter."
	msgValueInvalid  = "Nilai estimasi tidak valid. Contoh: 15.000.000"
	msgSourceUnknown = "Sumber tidak dikenal."
	msgDateInvalid   = "Tanggal tidak valid."
	msgNotesLong     = "Catatan maksimal 2000 karakter."
	msgReasonLong    = "Alasan maksimal 500 karakter."
)

// ValidationError carries field-level Indonesian messages keyed by form field.
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string { return fmt.Sprintf("lead: invalid input %v", e.Fields) }

// Validate trims and converts a lead form. Only the title is required.
func Validate(in Input) (Normalized, map[string]string) {
	errs := map[string]string{}
	n := Normalized{
		Title:       strings.TrimSpace(in.Title),
		Description: strings.TrimSpace(in.Description),
		Notes:       strings.TrimSpace(in.Notes),
	}
	switch {
	case n.Title == "":
		errs["title"] = msgTitleRequired
	case utf8.RuneCountInString(n.Title) > MaxTitleLen:
		errs["title"] = msgTitleLong
	}
	if v := strings.TrimSpace(in.ProjectType); v != "" {
		n.ProjectType = ProjectType(v)
		if !n.ProjectType.Valid() {
			errs["project_type"] = msgTypeUnknown
		}
	}
	if utf8.RuneCountInString(n.Description) > MaxDescriptionLen {
		errs["description"] = msgDescLong
	}
	if v := strings.TrimSpace(in.EstimatedValue); v != "" {
		amount, err := money.ParseIDR(v)
		if err != nil {
			errs["estimated_value"] = msgValueInvalid
		} else {
			n.EstimatedValue = &amount
		}
	}
	if v := strings.TrimSpace(in.Source); v != "" {
		n.Source = Source(v)
		if !n.Source.Valid() {
			errs["source"] = msgSourceUnknown
		}
	}
	if v := strings.TrimSpace(in.NextFollowUpOn); v != "" {
		d, err := time.Parse(DateLayout, v)
		if err != nil {
			errs["next_follow_up_on"] = msgDateInvalid
		} else {
			n.NextFollowUpOn = &d
		}
	}
	if utf8.RuneCountInString(n.Notes) > MaxNotesLen {
		errs["notes"] = msgNotesLong
	}
	return n, errs
}

// ValidateLostReason returns a message for an over-long reason, or "".
func ValidateLostReason(reason string) string {
	if utf8.RuneCountInString(strings.TrimSpace(reason)) > MaxLostReasonLen {
		return msgReasonLong
	}
	return ""
}
