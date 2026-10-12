package customer_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kerjalancar/internal/customer"
)

func TestNormalizePhoneTurnsIndonesianFormatsIntoE164(t *testing.T) {
	cases := []struct{ in, want string }{
		{"0812-3456-7890", "+6281234567890"},
		{"0812 3456 7890", "+6281234567890"},
		{"+62 812 3456 7890", "+6281234567890"},
		{"+62 0812 3456 7890", "+6281234567890"}, // zero after the country code is a common typo
		{"62812-3456-7890", "+6281234567890"},
		{"812 3456 7890", "+6281234567890"}, // leading zero dropped by the user
		{"(021) 555-1234", "+62215551234"},  // Jakarta landline
		{"+65 9123 4567", "+6591234567"},    // foreign numbers are kept
		{"+1 (415) 555-0132", "+14155550132"},
	}
	for _, tc := range cases {
		got, err := customer.NormalizePhone(tc.in)
		require.NoError(t, err, "input %q", tc.in)
		assert.Equal(t, tc.want, got, "input %q", tc.in)
	}
}

func TestNormalizePhoneRejectsUnusableInput(t *testing.T) {
	for _, in := range []string{"", "   ", "12", "abc", "+0 812 3456", "0812", "08123456789012345", "+628123456789012345", "0812-3456-78ab", "++62812345678"} {
		_, err := customer.NormalizePhone(in)
		assert.Error(t, err, "input %q", in)
	}
}

func TestDisplayPhoneShowsIndonesianNumbersTheWayPeopleWriteThem(t *testing.T) {
	assert.Equal(t, "0812-3456-7890", customer.DisplayPhone("+6281234567890"))
	assert.Equal(t, "0812-3456-789", customer.DisplayPhone("+628123456789"))
	assert.Equal(t, "021-555-1234", customer.DisplayPhone("+62215551234"))
	assert.Equal(t, "+6591234567", customer.DisplayPhone("+6591234567"), "foreign numbers unchanged")
	assert.Equal(t, "", customer.DisplayPhone(""))
}

func TestDisplayPhoneNeverPanicsOnShortInput(t *testing.T) {
	for _, in := range []string{"+62", "+621", "+628", "+6281", "+6221", "+1"} {
		assert.NotPanics(t, func() { _ = customer.DisplayPhone(in) }, in)
	}
	assert.Equal(t, "+62", customer.DisplayPhone("+62"))
}

func TestWhatsAppDigitsStripsThePlus(t *testing.T) {
	assert.Equal(t, "6281234567890", customer.WhatsAppDigits("+6281234567890"))
	assert.Equal(t, "6591234567", customer.WhatsAppDigits("+6591234567"))
	assert.Equal(t, "", customer.WhatsAppDigits(""))
}
