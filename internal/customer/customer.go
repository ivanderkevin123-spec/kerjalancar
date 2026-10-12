// Package customer owns contacts (people or businesses) of a workspace. Every
// read and write takes the workspace id from the session; nothing is unscoped.
package customer

import (
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Customer is one contact. Optional fields are "" when unset.
type Customer struct {
	ID          uuid.UUID
	WorkspaceID uuid.UUID
	Name        string
	Phone       string // E.164, or ""
	Email       string
	Company     string
	Notes       string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Input is what a form submits. Validate normalises it.
type Input struct {
	Name    string
	Phone   string
	Email   string
	Company string
	Notes   string
}

const (
	MaxNameLen    = 150
	MaxCompanyLen = 100
	MaxEmailLen   = 254
	MaxNotesLen   = 2000
)

const (
	msgNameRequired = "Nama wajib diisi."
	msgNameLong     = "Nama maksimal 150 karakter."
	msgPhoneInvalid = "Nomor telepon tidak valid. Contoh: 0812-3456-7890 atau +62812…"
	msgEmailInvalid = "Email tidak valid."
	msgCompanyLong  = "Nama perusahaan maksimal 100 karakter."
	msgNotesLong    = "Catatan maksimal 2000 karakter."
)

var ErrNotFound = errors.New("customer: not found")

// ValidationError carries field-level messages in Indonesian, keyed by form field.
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string { return fmt.Sprintf("customer: invalid input %v", e.Fields) }

// DuplicatePhoneError names the customer who already has the number, so the
// user can pick them instead of creating a duplicate (D-034).
type DuplicatePhoneError struct {
	ID   uuid.UUID
	Name string
}

func (e *DuplicatePhoneError) Error() string {
	return fmt.Sprintf("customer: phone already used by %s", e.Name)
}

// Validate trims, normalises (phone to E.164, e-mail lower-cased) and returns
// the field errors. An empty map means the input is acceptable.
func Validate(in Input) (Input, map[string]string) {
	errs := map[string]string{}
	in.Name = strings.TrimSpace(in.Name)
	in.Phone = strings.TrimSpace(in.Phone)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.Company = strings.TrimSpace(in.Company)
	in.Notes = strings.TrimSpace(in.Notes)

	switch {
	case in.Name == "":
		errs["name"] = msgNameRequired
	case utf8.RuneCountInString(in.Name) > MaxNameLen:
		errs["name"] = msgNameLong
	}
	if in.Phone != "" {
		p, err := NormalizePhone(in.Phone)
		if err != nil {
			errs["phone"] = msgPhoneInvalid
		} else {
			in.Phone = p
		}
	}
	if in.Email != "" && !validEmail(in.Email) {
		errs["email"] = msgEmailInvalid
	}
	if utf8.RuneCountInString(in.Company) > MaxCompanyLen {
		errs["company"] = msgCompanyLong
	}
	if utf8.RuneCountInString(in.Notes) > MaxNotesLen {
		errs["notes"] = msgNotesLong
	}
	return in, errs
}

// validEmail accepts one plain address (no display name) of sane length.
func validEmail(e string) bool {
	if len(e) > MaxEmailLen || strings.ContainsAny(e, " \t\r\n") {
		return false
	}
	addr, err := mail.ParseAddress(e)
	return err == nil && addr.Address == e && strings.Contains(e, "@")
}
