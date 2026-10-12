package followup_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kerjalancar/internal/followup"
)

var (
	jakarta  = time.FixedZone("WIB", 7*3600)
	jayapura = time.FixedZone("WIT", 9*3600)
)

func date(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

// 16:59 UTC on 19 Oct is 23:59 WIB on the 19th but already 01:59 WIT on the 20th.
func TestTodayDependsOnTheWorkspaceZone(t *testing.T) {
	instant := time.Date(2026, 10, 19, 16, 59, 0, 0, time.UTC)
	assert.Equal(t, date(2026, 10, 19), followup.Today(instant, jakarta))
	assert.Equal(t, date(2026, 10, 20), followup.Today(instant, jayapura))
	assert.Equal(t, date(2026, 10, 19), followup.Today(instant, nil), "nil zone falls back to UTC")
}

func TestBucketOfClassifiesAgainstToday(t *testing.T) {
	today := date(2026, 10, 20)
	assert.Equal(t, followup.BucketOverdue, followup.BucketOf(date(2026, 10, 19), today))
	assert.Equal(t, followup.BucketOverdue, followup.BucketOf(date(2026, 1, 1), today))
	assert.Equal(t, followup.BucketToday, followup.BucketOf(date(2026, 10, 20), today))
	assert.Equal(t, followup.BucketUpcoming, followup.BucketOf(date(2026, 10, 21), today))
	assert.Equal(t, followup.BucketUpcoming, followup.BucketOf(date(2026, 10, 27), today), "seven days ahead is still upcoming")
	assert.Equal(t, followup.BucketLater, followup.BucketOf(date(2026, 10, 28), today))
}

// The MVP acceptance case: a follow-up due on 2026-10-20 is "today" at 00:00 WIT, not at 00:00 WIB.
func TestDueDateAtMidnightBoundary(t *testing.T) {
	due := date(2026, 10, 20)
	midnightWIB := time.Date(2026, 10, 19, 17, 0, 0, 0, time.UTC) // 00:00 WIB on the 20th
	assert.Equal(t, followup.BucketToday, followup.BucketOf(due, followup.Today(midnightWIB, jakarta)))
	assert.Equal(t, followup.BucketToday, followup.BucketOf(due, followup.Today(midnightWIB, jayapura)), "02:00 WIT on the 20th")
	justBeforeWIT := time.Date(2026, 10, 19, 14, 59, 0, 0, time.UTC) // 23:59 WIT on the 19th, 21:59 WIB
	assert.Equal(t, followup.BucketUpcoming, followup.BucketOf(due, followup.Today(justBeforeWIT, jayapura)))
	assert.Equal(t, followup.BucketUpcoming, followup.BucketOf(due, followup.Today(justBeforeWIT, jakarta)))
}

func TestValidateDatesAndNote(t *testing.T) {
	today := date(2026, 10, 20)
	n, errs := followup.Validate(followup.Input{DueOn: "2026-10-23", Note: "  telepon lagi  "}, today)
	require.Empty(t, errs)
	assert.Equal(t, date(2026, 10, 23), n.DueOn)
	assert.Equal(t, "telepon lagi", n.Note)

	_, errs = followup.Validate(followup.Input{DueOn: ""}, today)
	assert.Equal(t, "Tanggal wajib diisi.", errs["due_on"])
	_, errs = followup.Validate(followup.Input{DueOn: "23/10/2026"}, today)
	assert.Equal(t, "Tanggal tidak valid.", errs["due_on"])
	_, errs = followup.Validate(followup.Input{DueOn: "2027-10-21"}, today)
	assert.Equal(t, "Tanggal terlalu jauh (maksimal satu tahun).", errs["due_on"])
	_, errs = followup.Validate(followup.Input{DueOn: "2025-10-19"}, today)
	assert.Equal(t, "Tanggal terlalu jauh (maksimal satu tahun).", errs["due_on"])
	_, errs = followup.Validate(followup.Input{DueOn: "2026-10-19"}, today)
	assert.Empty(t, errs, "yesterday is allowed: it simply shows as overdue")
	_, errs = followup.Validate(followup.Input{DueOn: "2026-10-23", Note: strings.Repeat("x", 501)}, today)
	assert.Equal(t, "Catatan maksimal 500 karakter.", errs["note"])
}

func TestDefaultDueDateIsThreeDaysAhead(t *testing.T) {
	assert.Equal(t, date(2026, 10, 23), followup.DefaultDue(date(2026, 10, 20)))
}
