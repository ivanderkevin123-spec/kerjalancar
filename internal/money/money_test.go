package money_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kerjalancar/internal/money"
)

func TestParseIDRAcceptsCommonIndonesianInputs(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"15000000", 15_000_000},
		{"15.000.000", 15_000_000},
		{"Rp 15.000.000", 15_000_000},
		{"Rp15.000.000", 15_000_000},
		{"Rp15.000.000,00", 15_000_000},
		{" 1 500 ", 1500},
		{"0", 0},
		{"10000000000000", 10_000_000_000_000}, // Rp10 triliun, the ceiling
	}
	for _, tc := range cases {
		got, err := money.ParseIDR(tc.in)
		require.NoError(t, err, "input %q", tc.in)
		assert.Equal(t, tc.want, got, "input %q", tc.in)
	}
}

func TestParseIDRRejectsAmbiguousOrOutOfRangeInputs(t *testing.T) {
	for _, in := range []string{"", "   ", "-1", "1,5", "15.000.000,50", "15jt", "abc", "10000000000001", "1e6", "Rp"} {
		_, err := money.ParseIDR(in)
		assert.Error(t, err, "input %q", in)
	}
}

func TestFormatIDRUsesDotThousandsAndNoSpace(t *testing.T) {
	assert.Equal(t, "Rp0", money.FormatIDR(0))
	assert.Equal(t, "Rp999", money.FormatIDR(999))
	assert.Equal(t, "Rp1.500", money.FormatIDR(1500))
	assert.Equal(t, "Rp15.000.000", money.FormatIDR(15_000_000))
	assert.Equal(t, "Rp1.000.000.000.000", money.FormatIDR(1_000_000_000_000))
}

func TestFormatIDRRoundTripsParse(t *testing.T) {
	for _, v := range []int64{0, 7, 1234, 98_765_432_100} {
		got, err := money.ParseIDR(money.FormatIDR(v))
		require.NoError(t, err)
		assert.Equal(t, v, got)
	}
}
