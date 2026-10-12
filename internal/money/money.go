// Package money handles rupiah amounts as integers. No floats, ever (D-010).
package money

import (
	"errors"
	"strconv"
	"strings"
)

// MaxIDR is the largest amount the application accepts: Rp10 triliun.
const MaxIDR int64 = 10_000_000_000_000

var ErrInvalidAmount = errors.New("money: invalid rupiah amount")

// ParseIDR reads an amount the way Indonesians type it: "15000000",
// "15.000.000", "Rp 15.000.000" or "Rp15.000.000,00". Dots are thousand
// separators and must group exactly three digits; the only decimal part
// accepted is ",00". Anything else, a negative number, or more than MaxIDR
// is an error so a typo never becomes a silently wrong value.
func ParseIDR(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && strings.EqualFold(s[:2], "rp") {
		s = strings.TrimSpace(s[2:])
	}
	s = strings.ReplaceAll(s, " ", "")
	if i := strings.IndexByte(s, ','); i >= 0 {
		if dec := s[i+1:]; dec != "00" && dec != "0" {
			return 0, ErrInvalidAmount
		}
		s = s[:i]
	}
	if s == "" {
		return 0, ErrInvalidAmount
	}
	groups := strings.Split(s, ".")
	for i, g := range groups {
		if g == "" || !allDigits(g) {
			return 0, ErrInvalidAmount
		}
		if i == 0 && len(g) > 3 && len(groups) > 1 {
			return 0, ErrInvalidAmount
		}
		if i > 0 && len(g) != 3 {
			return 0, ErrInvalidAmount
		}
	}
	digits := strings.Join(groups, "")
	if len(digits) > 14 { // MaxIDR has 14 digits; refuse before strconv can overflow
		return 0, ErrInvalidAmount
	}
	v, err := strconv.ParseInt(digits, 10, 64)
	if err != nil || v > MaxIDR {
		return 0, ErrInvalidAmount
	}
	return v, nil
}

// FormatIDR renders an amount as "Rp15.000.000".
func FormatIDR(v int64) string {
	neg := v < 0
	if neg {
		v = -v
	}
	digits := strconv.FormatInt(v, 10)
	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	b.WriteString("Rp")
	first := len(digits) % 3
	if first == 0 {
		first = 3
	}
	b.WriteString(digits[:first])
	for i := first; i < len(digits); i += 3 {
		b.WriteByte('.')
		b.WriteString(digits[i : i+3])
	}
	return b.String()
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
