// Package template owns WhatsApp message templates: the three fixed slots per
// workspace, placeholder rendering, and wa.me link building (D-046, D-047).
// The server never sends a message; it prepares a link the owner taps.
package template

import (
	"net/url"
	"strings"
	"unicode/utf8"

	"kerjalancar/internal/customer"
)

// Key identifies one of the fixed template slots.
type Key string

const (
	KeyFollowUpPenawaran  Key = "followup_penawaran"
	KeyPengingatDP        Key = "pengingat_dp"
	KeyPengingatPelunasan Key = "pengingat_pelunasan"
)

// Keys in display order.
var Keys = []Key{KeyFollowUpPenawaran, KeyPengingatDP, KeyPengingatPelunasan}

var labels = map[Key]string{
	KeyFollowUpPenawaran: "Follow-up penawaran", KeyPengingatDP: "Pengingat DP", KeyPengingatPelunasan: "Pengingat pelunasan",
}

func (k Key) Valid() bool           { _, ok := labels[k]; return ok }
func (k Key) Label() string         { return labels[k] }
func ParseKey(v string) (Key, bool) { k := Key(strings.TrimSpace(v)); return k, k.Valid() }

// Seed is the default content of a slot; applied to every new workspace.
type Seed struct {
	Key      Key
	Name     string
	Body     string
	Position int
}

// Seeds are the defaults in Indonesian. The migration seeds existing
// workspaces with the same text; keep both in sync.
var Seeds = []Seed{
	{KeyFollowUpPenawaran, "Follow-up penawaran", "Halo {nama}, saya dari {usaha}. Apakah penawaran untuk proyek senilai {nilai} sudah sempat dilihat? Jika ada yang perlu disesuaikan, kabari saya ya.", 1},
	{KeyPengingatDP, "Pengingat DP", "Halo {nama}, terima kasih sudah mempercayakan proyek ini ke {usaha}. Sebagai pengingat, DP sebesar {nilai} bisa ditransfer agar jadwal pengerjaan dapat kami kunci.", 2},
	{KeyPengingatPelunasan, "Pengingat pelunasan", "Halo {nama}, pekerjaan dari {usaha} sudah selesai. Mohon pelunasan sebesar {nilai}. Terima kasih atas kepercayaannya.", 3},
}

// Data fills the placeholders. Empty fields render as "" (never the token).
type Data struct {
	Nama   string
	Usaha  string
	Nilai  string
	Tautan string
}

// Placeholders in the order they are documented to users.
var Placeholders = []string{"nama", "usaha", "nilai", "tautan"}

func (d Data) value(name string) string {
	switch name {
	case "nama":
		return d.Nama
	case "usaha":
		return d.Usaha
	case "nilai":
		return d.Nilai
	case "tautan":
		return d.Tautan
	}
	return ""
}

// Render replaces the known placeholders; unknown {tokens} are the user's text and stay.
func Render(body string, d Data) string {
	pairs := make([]string, 0, 2*len(Placeholders))
	for _, p := range Placeholders {
		pairs = append(pairs, "{"+p+"}", d.value(p))
	}
	return strings.NewReplacer(pairs...).Replace(body)
}

// MissingPlaceholders lists the known placeholders used in body whose data is empty.
func MissingPlaceholders(body string, d Data) []string {
	var out []string
	for _, p := range Placeholders {
		if strings.Contains(body, "{"+p+"}") && d.value(p) == "" {
			out = append(out, p)
		}
	}
	return out
}

// WaLink builds https://wa.me/<digits>?text=<encoded>. Spaces become %20
// (a "+" would be shown literally by some WhatsApp clients).
func WaLink(e164, text string) string {
	return "https://wa.me/" + customer.WhatsAppDigits(e164) + "?text=" + strings.ReplaceAll(url.QueryEscape(text), "+", "%20")
}

const (
	MaxNameLen = 60
	MaxBodyLen = 1000
)

const (
	msgNameRequired = "Nama templat wajib diisi."
	msgNameLong     = "Nama templat maksimal 60 karakter."
	msgBodyRequired = "Isi pesan wajib diisi."
	msgBodyLong     = "Isi pesan maksimal 1000 karakter."
)

// Validate checks an edited template.
func Validate(name, body string) map[string]string {
	errs := map[string]string{}
	name, body = strings.TrimSpace(name), strings.TrimSpace(body)
	switch {
	case name == "":
		errs["name"] = msgNameRequired
	case utf8.RuneCountInString(name) > MaxNameLen:
		errs["name"] = msgNameLong
	}
	switch {
	case body == "":
		errs["body"] = msgBodyRequired
	case utf8.RuneCountInString(body) > MaxBodyLen:
		errs["body"] = msgBodyLong
	}
	return errs
}
