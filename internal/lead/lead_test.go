package lead_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kerjalancar/internal/lead"
)

func TestStatusesHaveLabelsAndOpenness(t *testing.T) {
	assert.Equal(t, []lead.Status{lead.StatusBaru, lead.StatusDihubungi, lead.StatusSurvei, lead.StatusPenawaran, lead.StatusMenang, lead.StatusKalah}, lead.Statuses)
	assert.Equal(t, "Baru", lead.StatusBaru.Label())
	assert.Equal(t, "Penawaran", lead.StatusPenawaran.Label())
	assert.True(t, lead.StatusSurvei.Open())
	assert.False(t, lead.StatusMenang.Open())
	assert.False(t, lead.StatusKalah.Open())
	_, ok := lead.ParseStatus("negosiasi")
	assert.False(t, ok, "negosiasi is not a status (D-035)")
	s, ok := lead.ParseStatus("kalah")
	assert.True(t, ok)
	assert.Equal(t, lead.StatusKalah, s)
}

func TestCanTransitionAllowsAnyChangeExceptBetweenClosedStatuses(t *testing.T) {
	cases := []struct {
		from, to lead.Status
		want     error
	}{
		{lead.StatusBaru, lead.StatusBaru, lead.ErrSameStatus},
		{lead.StatusMenang, lead.StatusKalah, lead.ErrClosedToClosed},
		{lead.StatusKalah, lead.StatusMenang, lead.ErrClosedToClosed},
		{lead.StatusBaru, lead.StatusMenang, nil},
		{lead.StatusBaru, lead.StatusPenawaran, nil},
		{lead.StatusPenawaran, lead.StatusDihubungi, nil}, // going back is allowed
		{lead.StatusKalah, lead.StatusBaru, nil},          // reopening is allowed
		{lead.StatusMenang, lead.StatusSurvei, nil},
	}
	for _, tc := range cases {
		err := lead.CanTransition(tc.from, tc.to)
		if tc.want == nil {
			assert.NoError(t, err, "%s → %s", tc.from, tc.to)
		} else {
			assert.ErrorIs(t, err, tc.want, "%s → %s", tc.from, tc.to)
		}
	}
}

func TestValidateNormalisesALeadForm(t *testing.T) {
	n, errs := lead.Validate(lead.Input{
		Title:          "  Renovasi dapur  ",
		ProjectType:    "renovasi",
		Description:    " Dapur 3x4 m ",
		EstimatedValue: "Rp 15.000.000",
		Source:         "whatsapp",
		NextFollowUpOn: "2026-10-20",
		Notes:          " ",
	})
	require.Empty(t, errs)
	assert.Equal(t, "Renovasi dapur", n.Title)
	assert.Equal(t, lead.ProjectType("renovasi"), n.ProjectType)
	assert.Equal(t, "Dapur 3x4 m", n.Description)
	require.NotNil(t, n.EstimatedValue)
	assert.Equal(t, int64(15_000_000), *n.EstimatedValue)
	assert.Equal(t, lead.Source("whatsapp"), n.Source)
	require.NotNil(t, n.NextFollowUpOn)
	assert.Equal(t, time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC), *n.NextFollowUpOn)
	assert.Empty(t, n.Notes)

	minimal, errs := lead.Validate(lead.Input{Title: "Kamar mandi"})
	require.Empty(t, errs, "everything but the title is optional")
	assert.Nil(t, minimal.EstimatedValue)
	assert.Nil(t, minimal.NextFollowUpOn)
	assert.Empty(t, minimal.Source)
}

func TestValidateReportsEveryBadFieldInIndonesian(t *testing.T) {
	long := make([]byte, 151)
	for i := range long {
		long[i] = 'x'
	}
	_, errs := lead.Validate(lead.Input{Title: "", ProjectType: "kapal", EstimatedValue: "15jt", Source: "tiktok", NextFollowUpOn: "20/10/2026"})
	assert.Equal(t, "Nama proyek wajib diisi.", errs["title"])
	assert.Equal(t, "Jenis proyek tidak dikenal.", errs["project_type"])
	assert.Equal(t, "Nilai estimasi tidak valid. Contoh: 15.000.000", errs["estimated_value"])
	assert.Equal(t, "Sumber tidak dikenal.", errs["source"])
	assert.Equal(t, "Tanggal tidak valid.", errs["next_follow_up_on"])

	_, errs = lead.Validate(lead.Input{Title: string(long), EstimatedValue: "-5"})
	assert.Equal(t, "Nama proyek maksimal 150 karakter.", errs["title"])
	assert.NotEmpty(t, errs["estimated_value"])

	big := make([]byte, 2001)
	for i := range big {
		big[i] = 'y'
	}
	_, errs = lead.Validate(lead.Input{Title: "ok", Description: string(big), Notes: string(big)})
	assert.Equal(t, "Deskripsi maksimal 2000 karakter.", errs["description"])
	assert.Equal(t, "Catatan maksimal 2000 karakter.", errs["notes"])
}

func TestValidateLostReason(t *testing.T) {
	long := make([]byte, 501)
	for i := range long {
		long[i] = 'z'
	}
	assert.Equal(t, "Alasan maksimal 500 karakter.", lead.ValidateLostReason(string(long)))
	assert.Empty(t, lead.ValidateLostReason("harga terlalu tinggi"))
}

func TestDescribeRendersActivitiesForTheTimeline(t *testing.T) {
	assert.Equal(t, "Prospek dibuat", lead.Activity{Kind: lead.KindCreated}.Describe())
	assert.Equal(t, "Status: Baru → Dihubungi", lead.Activity{Kind: lead.KindStatusChanged, Data: lead.ActivityData{From: lead.StatusBaru, To: lead.StatusDihubungi}}.Describe())
	assert.Equal(t, "Status: Penawaran → Kalah · Alasan: harga", lead.Activity{Kind: lead.KindStatusChanged, Data: lead.ActivityData{From: lead.StatusPenawaran, To: lead.StatusKalah, Reason: "harga"}}.Describe())
	assert.Equal(t, "Catatan ditambahkan", lead.Activity{Kind: lead.KindNote, Body: "telepon lagi besok"}.Describe())
	assert.Equal(t, "Penawaran #0003 dikirim (Rp15.000.000)", lead.Activity{Kind: lead.KindQuotationSent, Data: lead.ActivityData{Number: "0003", Total: "Rp15.000.000"}}.Describe())
	assert.Equal(t, "Penawaran #0003 diterima (Rp15.000.000)", lead.Activity{Kind: lead.KindQuotationAccepted, Data: lead.ActivityData{Number: "0003", Total: "Rp15.000.000"}}.Describe())
	assert.Equal(t, "Penawaran #0003 ditolak", lead.Activity{Kind: lead.KindQuotationRejected, Data: lead.ActivityData{Number: "0003"}}.Describe())
	upd := lead.Activity{Kind: lead.KindUpdated, Data: lead.ActivityData{Changes: []lead.Change{
		{Field: "estimated_value", From: "Rp10.000.000", To: "Rp12.000.000"},
		{Field: "source", From: "", To: "Instagram"},
		{Field: "description"},
	}}}
	assert.Equal(t, "Diperbarui: Nilai estimasi (Rp10.000.000 → Rp12.000.000), Sumber (– → Instagram), Deskripsi", upd.Describe())
}
