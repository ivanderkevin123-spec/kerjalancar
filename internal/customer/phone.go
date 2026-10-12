package customer

import (
	"errors"
	"regexp"
	"strings"
)

// ErrInvalidPhone is returned for input that cannot be read as a phone number.
var ErrInvalidPhone = errors.New("customer: invalid phone number")

// e164 matches what the database CHECK constraint accepts.
var e164 = regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)

// NormalizePhone turns the ways people write Indonesian numbers ("0812-3456-7890",
// "62812…", "+62 812…", "812…") into E.164 "+62812…". Numbers with another
// country code ("+65…") are kept with that code. The rest is rejected rather
// than guessed, so a stored number is always dialable (D-034).
func NormalizePhone(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", ErrInvalidPhone
	}
	plus := strings.HasPrefix(s, "+")
	if plus {
		s = s[1:]
	}
	var digits strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			digits.WriteRune(r)
		case r == ' ' || r == '-' || r == '.' || r == '(' || r == ')':
		default:
			return "", ErrInvalidPhone
		}
	}
	d := digits.String()

	var out string
	switch {
	case plus && strings.HasPrefix(d, "62"):
		out = indonesian(strings.TrimPrefix(d[2:], "0"))
	case plus:
		out = "+" + d
	case strings.HasPrefix(d, "0"):
		out = indonesian(d[1:])
	case strings.HasPrefix(d, "62") && len(d) >= 10:
		out = indonesian(strings.TrimPrefix(d[2:], "0"))
	case strings.HasPrefix(d, "8"):
		out = indonesian(d)
	default:
		return "", ErrInvalidPhone
	}
	if out == "" || !e164.MatchString(out) {
		return "", ErrInvalidPhone
	}
	return out, nil
}

// indonesian validates a national number (without the leading 0) and prefixes +62.
func indonesian(national string) string {
	if len(national) < 8 || len(national) > 12 || national[0] == '0' {
		return ""
	}
	return "+62" + national
}

// twoDigitAreaCodes are the Indonesian landline area codes with two digits;
// every other area code has three. Display only.
var twoDigitAreaCodes = map[string]bool{"21": true, "22": true, "24": true, "31": true, "61": true}

// DisplayPhone shows "+6281234567890" as "0812-3456-7890", the form Indonesian
// users recognise. Other country codes are shown unchanged.
func DisplayPhone(e164Number string) string {
	if !strings.HasPrefix(e164Number, "+62") {
		return e164Number
	}
	n := e164Number[3:]
	if len(n) < 3 {
		return e164Number
	}
	if n[0] == '8' { // mobile: 0812-3456-7890
		s := "0" + n
		if len(s) <= 8 {
			return s
		}
		return s[:4] + "-" + s[4:8] + "-" + s[8:]
	}
	area := n[:2]
	if !twoDigitAreaCodes[area] && len(n) > 3 {
		area = n[:3]
	}
	local := n[len(area):]
	if len(local) <= 4 {
		return "0" + area + "-" + local
	}
	return "0" + area + "-" + local[:len(local)-4] + "-" + local[len(local)-4:]
}

// WhatsAppDigits turns an E.164 number into the digits wa.me expects (no "+").
func WhatsAppDigits(e164Number string) string {
	return strings.TrimPrefix(e164Number, "+")
}
