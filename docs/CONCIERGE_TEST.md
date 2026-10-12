# Concierge Test: "Pipeline & Tagihan" sheet with a human reminder

Purpose: before any software exists, find out whether a business owner will record real leads, a real quotation, and real payments in a second place, act on a daily follow-up list, come back on later days, and then consider paying. This is the only direct test of assumption A-04 (double entry) and risk R-01 before Week 1. Its cohort result is an input to Gate G1 (`VALIDATION_PLAN.md` §5).

Nothing is built. The "product" is a Google Sheet the owner controls plus a daily WhatsApp message from the founder listing who to follow up. The founder is the reminder engine.

## 1. Participants

- 3 to 5 owners recruited from interviews with pain score ≥ 2, either segment. Prefer those who showed an existing workaround (they already try).
- Each gets their own sheet in their own Google account. No shared sheet across businesses.
- Duration: 5 working days per participant, starting the day after the setup call. Started no later than Thursday 15 Oct so that 5 days of observation exist by the end of the G1 extension window if needed.

## 2. The sheet (three tabs; CSV templates in `templates/concierge/`)

| Tab | Columns | What the owner does |
|---|---|---|
| `Calon Pelanggan` (leads) | tanggal_masuk, nama, no_wa (opsional: inisial + 4 digit), kebutuhan, perkiraan_nilai, status (Baru / Dihubungi / Penawaran / Menang / Kalah), tanggal_penawaran, nilai_penawaran, follow_up_berikutnya, catatan | Adds each new real inquiry the day it arrives; updates status; sets the next follow-up date |
| `Tagihan` (invoices) | nama, total, dp_diterima, tanggal_dp, cicilan_diterima, sisa (formula), jatuh_tempo, status (Belum lunas / Lunas), catatan | Records DP and installments as they are received; `sisa` is a formula, never typed |
| `Follow-up Hari Ini` | Filter formula over leads with `follow_up_berikutnya <= today` and status not Menang/Kalah, plus `Tagihan` rows with `jatuh_tempo <= today` and status Belum lunas | Reads it; marks done by changing the date or status in the source tab |

Formulas to paste in `Follow-up Hari Ini` (Google Sheets):

```
=FILTER('Calon Pelanggan'!B:J, 'Calon Pelanggan'!I:I<=TODAY(), 'Calon Pelanggan'!F:F<>"Menang", 'Calon Pelanggan'!F:F<>"Kalah", 'Calon Pelanggan'!B:B<>"")
=FILTER('Tagihan'!A:I, 'Tagihan'!G:G<=TODAY(), 'Tagihan'!H:H="Belum lunas", 'Tagihan'!A:A<>"")
```

`sisa` in `Tagihan`: `=B2-D2-F2` (total − DP − cicilan).

The founder enables "Show edit history" and relies on cell edit timestamps to see *when* the owner entered data, not just what.

## 3. Steps

| Day | Founder | Owner |
|---|---|---|
| 0 (setup, 15 min call) | Sends consent message (§4), creates the sheet from the templates in the owner's account (or sends the template link for them to copy), enters the owner's 3–5 current open leads together with them, sets each follow-up date, shows the Hari Ini tab | Provides current open leads from their WhatsApp; agrees on a morning time for the reminder |
| 1–5, each morning (≤ 5 min) | Opens the sheet; sends a WhatsApp message: "Follow-up hari ini: 1) [nama] – penawaran dikirim 3 hari lalu; 2) [nama] – sisa Rp X jatuh tempo hari ini." Nothing else | Records any new inquiry; updates statuses; marks follow-ups by changing dates; records any DP received |
| 1–5, each evening (≤ 5 min) | Logs in the observation sheet: did the owner edit today (edit history), what, at what time; any WA questions from the owner | — |
| 3 | Sends one question only: "Ada yang menyulitkan dari sheet-nya?" | Replies or not (both are data) |
| 6 (debrief, 15 min call) | Asks: what did you record and what did you skip, why; which reminder was useful; what would make you keep doing this; re-asks the commitment question at Rp149,000/month with a start date | Gives feedback; answers the commitment question |
| 6 | Revokes own access to the sheet (§6); writes the participant summary with scores only | Keeps the sheet |

The founder never enters data for the owner after Day 0. If the owner asks the founder to enter something, the founder declines politely and notes it: that is the double-entry signal.

## 4. Consent message (WhatsApp, Bahasa Indonesia)

> Pak/Bu [nama], terima kasih untuk obrolannya. Boleh saya coba sesuatu selama 5 hari kerja? Saya buatkan satu Google Sheet di akun Bapak/Ibu untuk mencatat calon pelanggan, penawaran, dan DP, lalu tiap pagi saya kirim daftar siapa yang perlu di-follow-up hari itu. Tidak ada aplikasi, tidak ada biaya. Datanya tetap milik Bapak/Ibu; saya hanya melihat sheet itu selama 5 hari untuk belajar, tidak menyalin ke mana pun, dan akses saya dihapus setelah selesai. Nomor WA pelanggan boleh ditulis sebagian saja (misalnya inisial dan 4 digit terakhir). Kalau setuju, balas "setuju" dan kita atur 15 menit untuk mulai.

Proceed only on an explicit "setuju". Keep the reply.

## 5. Success metrics

Per participant (from edit history and the debrief):

| # | Metric | Threshold |
|---|---|---|
| M1 | Records a real new lead themselves within 24 h of it arriving (not the Day 0 leads) | ≥ 1 |
| M2 | Logs a real quotation (date and amount) on a lead | ≥ 1 |
| M3 | Acts on the Hari Ini list (status or date changed on a listed row the same day) | ≥ 1 |
| M4 | Returns to the sheet on later days without being asked to edit (the morning message is a reminder to *act*, not to edit) | edits on ≥ 2 distinct days after Day 1 |
| M5 | Records a DP or installment when one occurs (if any occurred) | yes, if applicable |
| M6 | Gives at least one specific improvement in the debrief | ≥ 1 |
| M7 | Commitment ladder at the debrief | ≥ 3 |

Cohort thresholds feeding G1: PROCEED needs M1 and M4 met by ≥ 2 of 3 (or ≥ 3 of 5) participants and M7 ≥ 3 for ≥ 2. STOP condition: 0 participants meet M1. REVISE condition: M1 met but M2/M3 mostly unmet with debrief comments that the quotation or payment steps do not match their work.

Also record, per participant: median time of day of edits (tells us when owners have a moment), number of leads skipped (ask in debrief: "ada yang masuk tapi tidak dicatat? kenapa?"), and whether the WA number was entered fully or partially (appetite for sharing customer data).

## 6. Privacy safeguards

- The sheet lives in the owner's Google account; the owner is the controller of their customers' data. The founder's access is editor for Day 0, viewer afterwards, and is removed on Day 6. Our understanding is that this keeps the founder's role to a short, consented processing on the owner's behalf under UU PDP 27/2022; this is not legal advice and is listed for verification (`COMPETITOR_RESEARCH.md` §4 item 15).
- Customer phone numbers are optional and may be partial. The founder never copies customer rows anywhere, never screenshots the sheet, and never contacts the owner's customers.
- The founder's observation log contains counts, timestamps, and the owner's initials only. It lives in `docs/private/concierge/` (git-ignored).
- Anonymized results (metrics M1–M7 per participant code, quotes with no names) go into `VALIDATION_FINDINGS.md`.
- Participants can stop at any time with one message; their sheet stays theirs.

## 7. Limitations (state them in the findings)

- Hawthorne effect: a founder watching and messaging daily inflates usage versus a product with no human behind it. Treat M4 as an upper bound.
- A human-written WhatsApp reminder is more compelling than an in-app list. This overstates the value of reminders and says nothing about push notifications.
- Spreadsheets on a phone are clumsy; this understates adoption of a purpose-built mobile form. M1 failures should be probed for "ribet di HP" versus "tidak kepikiran".
- n = 3–5 and self-selected volunteers; results are directional only.
- Five days misses weekly rhythms (e.g., Friday payment runs) and seasonality (wedding peaks).
- The founder's time: ~15 min per participant per day. Five participants is the ceiling during interview week.

## 8. What the result changes

- Confirms or weakens A-04 and A-15 in `ASSUMPTION_REGISTER.md`.
- Supplies the minimal field set owners actually fill (which columns stayed empty), which bounds the Week 2 data model.
- Produces the first candidates for paid pilots (M7 ≥ 3).
