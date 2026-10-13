// Package money handles rupiah amounts as integers. No floats, ever (D-010).
package money

import (
	"errors"
	"fmt"
	"math/big"
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

// MaxQtyMilli is the largest quantity (one million units, in thousandths).
const MaxQtyMilli int64 = 1_000_000_000

var ErrInvalidQty = errors.New("money: invalid quantity")

// LineTotal is qty (in thousandths) × unit price, rounded half up to the
// rupiah. Computed in big integers so large values never wrap; a result
// above MaxIDR is an error.
func LineTotal(qtyMilli, unitPrice int64) (int64, error) {
	if qtyMilli <= 0 || qtyMilli > MaxQtyMilli || unitPrice < 0 || unitPrice > MaxIDR {
		return 0, ErrInvalidAmount
	}
	p := new(big.Int).Mul(big.NewInt(qtyMilli), big.NewInt(unitPrice))
	p.Add(p, big.NewInt(500)) // half up
	p.Quo(p, big.NewInt(1000))
	if !p.IsInt64() || p.Int64() > MaxIDR {
		return 0, ErrInvalidAmount
	}
	return p.Int64(), nil
}

// PercentOf is amount × pct / 100 rounded half up; pct is a whole percentage.
func PercentOf(amount int64, pct int) int64 {
	if amount <= 0 || pct <= 0 {
		return 0
	}
	return (amount*int64(pct) + 50) / 100
}

// ParseQty reads a quantity typed with a decimal comma or dot and up to three
// decimals ("2", "2,5", "0,125") into thousandths. Thousand separators are not
// accepted, so "1.000,5" is an error rather than a guess.
func ParseQty(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, ErrInvalidQty
	}
	sep := strings.IndexAny(s, ",.")
	whole, frac := s, ""
	if sep >= 0 {
		whole, frac = s[:sep], s[sep+1:]
	}
	if whole == "" || !allDigits(whole) || len(whole) > 7 || !allDigits(frac) || len(frac) > 3 {
		return 0, ErrInvalidQty
	}
	for len(frac) < 3 {
		frac += "0"
	}
	w, _ := strconv.ParseInt(whole, 10, 64)
	f, _ := strconv.ParseInt(frac, 10, 64)
	v := w*1000 + f
	if v <= 0 || v > MaxQtyMilli {
		return 0, ErrInvalidQty
	}
	return v, nil
}

// FormatQty renders thousandths as "2", "2,5" or "0,125".
func FormatQty(milli int64) string {
	whole, frac := milli/1000, milli%1000
	if frac == 0 {
		return strconv.FormatInt(whole, 10)
	}
	return strconv.FormatInt(whole, 10) + "," + strings.TrimRight(fmt.Sprintf("%03d", frac), "0")
}
