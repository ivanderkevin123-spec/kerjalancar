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

func TestLineTotalRoundsHalfUpAtTheRupiah(t *testing.T) {
	cases := []struct {
		qtyMilli, unitPrice, want int64
	}{
		{2500, 1999, 4998}, // 4997.5 → up
		{1000, 1999, 1999}, // whole quantity
		{333, 1000, 333},   // 0.333 × 1000
		{1, 1, 0},          // 0.001 → 0
		{1500, 1, 2},       // 1.5 → 2
		{2000, 15_000_000, 30_000_000},
		{1_000_000_000, 10_000, 10_000_000_000}, // one million units
	}
	for _, tc := range cases {
		got, err := money.LineTotal(tc.qtyMilli, tc.unitPrice)
		require.NoError(t, err, "%d × %d", tc.qtyMilli, tc.unitPrice)
		assert.Equal(t, tc.want, got, "%d × %d", tc.qtyMilli, tc.unitPrice)
	}
	_, err := money.LineTotal(1_000_000_000, 10_000_000_000_000)
	assert.Error(t, err, "beyond MaxIDR is refused, never wrapped")
	_, err = money.LineTotal(0, 100)
	assert.Error(t, err, "quantity must be positive")
	_, err = money.LineTotal(1000, -1)
	assert.Error(t, err)
}

func TestPercentOfRoundsHalfUp(t *testing.T) {
	assert.Equal(t, int64(110_000), money.PercentOf(1_000_000, 11))
	assert.Equal(t, int64(500), money.PercentOf(4998, 10)) // 499.8 → 500
	assert.Equal(t, int64(1), money.PercentOf(1, 50))      // 0.5 → 1
	assert.Equal(t, int64(0), money.PercentOf(1, 49))
	assert.Equal(t, int64(0), money.PercentOf(123, 0))
	assert.Equal(t, int64(123), money.PercentOf(123, 100))
}

func TestParseQtyAcceptsCommaOrDotWithUpToThreeDecimals(t *testing.T) {
	for in, want := range map[string]int64{"2": 2000, "2,5": 2500, "2.5": 2500, " 0,125 ": 125, "1,25": 1250, "1000000": 1_000_000_000, "10,": 10_000} {
		got, err := money.ParseQty(in)
		require.NoError(t, err, "input %q", in)
		assert.Equal(t, want, got, "input %q", in)
	}
	for _, in := range []string{"", "0", "0,000", "-1", "abc", "1,2345", "1.000,5", "1000001", "2,5,5"} {
		_, err := money.ParseQty(in)
		assert.Error(t, err, "input %q", in)
	}
}

func TestFormatQtyUsesCommaAndTrimsZeros(t *testing.T) {
	assert.Equal(t, "2", money.FormatQty(2000))
	assert.Equal(t, "2,5", money.FormatQty(2500))
	assert.Equal(t, "0,125", money.FormatQty(125))
	assert.Equal(t, "1,25", money.FormatQty(1250))
	assert.Equal(t, "1000000", money.FormatQty(1_000_000_000))
}
