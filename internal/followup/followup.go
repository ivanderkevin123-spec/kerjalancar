// Package followup owns follow-up reminders on leads: when they are due in
// the workspace's calendar, how they are bucketed into the lists the owner
// sees, and the rules for scheduling, finishing and undoing them (D-043..D-048).
package followup

import (
	"strings"
	"time"
	"unicode/utf8"
)

// Bucket is where a due date lands relative to the workspace's today.
type Bucket string

const (
	BucketOverdue  Bucket = "terlambat"
	BucketToday    Bucket = "hari_ini"
	BucketUpcoming Bucket = "mendatang" // the next seven days
	BucketLater    Bucket = "nanti"
)

// Today returns the calendar day of now in loc, as a date (UTC midnight), the
// same representation the database `date` column scans into.
func Today(now time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	n := now.In(loc)
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}

// BucketOf compares two dates (UTC midnight).
func BucketOf(dueOn, today time.Time) Bucket {
	days := int(dueOn.Sub(today).Hours() / 24)
	switch {
	case days < 0:
		return BucketOverdue
	case days == 0:
		return BucketToday
	case days <= 7:
		return BucketUpcoming
	}
	return BucketLater
}

// DefaultDue is the date pre-filled when a lead reaches penawaran (D-035).
func DefaultDue(today time.Time) time.Time { return today.AddDate(0, 0, 3) }

const (
	DateLayout = "2006-01-02"
	MaxNoteLen = 500
	maxYears   = 1
)

const (
	msgDateRequired = "Tanggal wajib diisi."
	msgDateInvalid  = "Tanggal tidak valid."
	msgDateTooFar   = "Tanggal terlalu jauh (maksimal satu tahun)."
	msgNoteLong     = "Catatan maksimal 500 karakter."
)

// Input is the raw scheduling form.
type Input struct {
	DueOn string
	Note  string
}

// Normalized is a validated scheduling form.
type Normalized struct {
	DueOn time.Time
	Note  string
}

// Validate parses the date and bounds it to one year around today; a date in
// the recent past is allowed (it simply shows as overdue).
func Validate(in Input, today time.Time) (Normalized, map[string]string) {
	errs := map[string]string{}
	n := Normalized{Note: strings.TrimSpace(in.Note)}
	raw := strings.TrimSpace(in.DueOn)
	switch raw {
	case "":
		errs["due_on"] = msgDateRequired
	default:
		d, err := time.Parse(DateLayout, raw)
		switch {
		case err != nil:
			errs["due_on"] = msgDateInvalid
		case d.After(today.AddDate(maxYears, 0, 0)) || d.Before(today.AddDate(-maxYears, 0, 0)):
			errs["due_on"] = msgDateTooFar
		default:
			n.DueOn = d
		}
	}
	if utf8.RuneCountInString(n.Note) > MaxNoteLen {
		errs["note"] = msgNoteLong
	}
	return n, errs
}
