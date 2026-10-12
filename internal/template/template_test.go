package template_test

import (
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"kerjalancar/internal/template"
)

func TestRenderFillsPlaceholdersAndBlanksMissingOnes(t *testing.T) {
	body := "Halo {nama}, dari {usaha}: penawaran {nilai}. Lihat {tautan}. {nama} lagi. {lainnya} tetap."
	got := template.Render(body, template.Data{Nama: "Budi", Usaha: "Studio A", Nilai: "Rp15.000.000"})
	assert.Equal(t, "Halo Budi, dari Studio A: penawaran Rp15.000.000. Lihat . Budi lagi. {lainnya} tetap.", got)
	assert.NotContains(t, got, "{nama}")
	assert.NotContains(t, got, "{tautan}", "a known placeholder without data renders as empty")
}

func TestMissingPlaceholdersListsWhatWillBeEmpty(t *testing.T) {
	body := "Halo {nama}, {nilai} dan {tautan}"
	assert.Equal(t, []string{"nilai", "tautan"}, template.MissingPlaceholders(body, template.Data{Nama: "Budi"}))
	assert.Empty(t, template.MissingPlaceholders("tanpa placeholder", template.Data{}))
}

func TestWaLinkEncodesForWhatsApp(t *testing.T) {
	text := "Halo Budi & Rina + tim\nBaris 2 😊 #1 100%"
	link := template.WaLink("+6281234567890", text)
	require.True(t, strings.HasPrefix(link, "https://wa.me/6281234567890?text="), link)
	encoded := strings.TrimPrefix(link, "https://wa.me/6281234567890?text=")
	assert.NotContains(t, encoded, " ")
	assert.NotContains(t, encoded, "+", "a literal plus must be %2B, and spaces must be %20, not +")
	assert.NotContains(t, encoded, "\n")
	assert.NotContains(t, encoded, "#")
	assert.NotContains(t, encoded, "&")
	decoded, err := url.QueryUnescape(encoded)
	require.NoError(t, err)
	assert.Equal(t, text, decoded)
	assert.Equal(t, "https://wa.me/6591234567?text=Hai", template.WaLink("+6591234567", "Hai"), "foreign numbers keep their code")
}

func TestValidateNameAndBody(t *testing.T) {
	assert.Empty(t, template.Validate("Follow-up", "Halo {nama}"))
	errs := template.Validate("", "")
	assert.Equal(t, "Nama templat wajib diisi.", errs["name"])
	assert.Equal(t, "Isi pesan wajib diisi.", errs["body"])
	errs = template.Validate(strings.Repeat("n", 61), strings.Repeat("b", 1001))
	assert.Equal(t, "Nama templat maksimal 60 karakter.", errs["name"])
	assert.Equal(t, "Isi pesan maksimal 1000 karakter.", errs["body"])
}

func TestKeysAndSeeds(t *testing.T) {
	assert.Equal(t, []template.Key{template.KeyFollowUpPenawaran, template.KeyPengingatDP, template.KeyPengingatPelunasan}, template.Keys)
	assert.True(t, template.KeyPengingatDP.Valid())
	assert.False(t, template.Key("custom").Valid())
	assert.Equal(t, "Pengingat DP", template.KeyPengingatDP.Label())
	require.Len(t, template.Seeds, 3)
	for i, s := range template.Seeds {
		assert.Equal(t, template.Keys[i], s.Key)
		assert.Empty(t, template.Validate(s.Name, s.Body), "seed %s is valid", s.Key)
		assert.Contains(t, s.Body, "{nama}")
		assert.Equal(t, i+1, s.Position)
	}
}
