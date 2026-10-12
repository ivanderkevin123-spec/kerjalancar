# Day 1 Execution Package (Monday 2026-10-12)

Everything the founder needs today. No paid ads, no public posts, no product claims. Keep the prospect list and all notes in a private Google Sheet or in `docs/private/` (git-ignored). The repo only ever receives anonymized findings.

## 1. Timeline for today

| Time | Task | Done when |
|---|---|---|
| 0:00–0:30 | Copy `templates/prospects.csv` into a private Google Sheet. Read §2–§4. | Sheet exists with the 18 columns |
| 0:30–2:00 | Build the list: 15 segment A, 10 segment B (§2). | 25 rows with source and score |
| 2:00–2:30 | Prioritise (§3). Mark the top 15 for today's outreach. | `priority_score` filled, sorted |
| 2:30–3:30 | Send 15 outreach messages (§5), one at a time, personalised first line. Log `contact_status` and `contact_date`. | 15 rows at `contacted` |
| 3:30–4:00 | Prepare interview slots for Tue–Fri (§6). Create `docs/private/interviews/` and copy the note template from `INTERVIEW_GUIDE.md`. | Calendar has 10 candidate slots |
| 4:00–4:15 | Pick 5 prospects to invite to the concierge test after their interview (`CONCIERGE_TEST.md`). | `concierge_candidate = yes` on 5 rows |
| End of day | Update `docs/PROJECT_STATUS.md` with counts: contacted, replied, booked. | Status committed (approval needed) |

## 2. How to identify 15 relevant prospects (per segment)

Work the warm sources first; they convert at a much higher rate than cold ones.

1. **Own network.** Message 5 people you know with: "Kenal kontraktor renovasi/interior atau WO yang usahanya sudah jalan ≥ 1 tahun?" One intro beats ten cold messages. Record the introducer in `public_contact_source`.
2. **Businesses you have hired.** Anyone who renovated, decorated, or photographed something for you or your family.
3. **Instagram.** Search hashtags and locations: `#renovasirumah #kontraktorrenovasi #jasarenovasi #desaininterior #interior[kota]`; for B: `#weddingorganizer[kota] #vendorwedding #dekorasipernikahan #wo[kota]`. Qualify by: WhatsApp number or "DM for price" in bio, posts in the last 30 days, visible before/after or event photos, replies to comments (they do their own chat).
4. **Google Maps.** "kontraktor renovasi [kota]", "jasa interior [kota]", "wedding organizer [kota]". Prefer 5–50 reviews (small, active); skip franchises and showrooms.
5. **Facebook groups** for renovasi/interior and wedding vendors. Read, do not post yet. Note members who answer questions or advertise services with a WA number.
6. **Marketplace service listings.** Tokopedia/Shopee "jasa renovasi", OLX jasa. These sellers already sell through chat.
7. **Bridestory vendor pages** (segment B) for vendors in your city with a WA contact.

Record for each: business name, segment, city, where you found them (URL), what you can see of their workflow (do they quote in DMs? mention DP?), and the public WA number or DM handle.

## 3. How to prioritise

Score each prospect; outreach in descending order. Target mix for today: 10 A, 5 B.

| Signal | Points |
|---|---|
| Warm intro available | +3 |
| Visibly sells through WhatsApp/DM (number in bio, "chat untuk harga") | +2 |
| Posted in the last 30 days | +1 |
| Team size 2–15 (photos of crew, mentions of "tim kami") | +1 |
| Mentions DP, termin, or pelunasan publicly | +1 |
| Same city (in-person possible) | +1 |
| Already a company with a showroom or sales team | −2 (not our user) |

## 4. How to recruit without paid advertising

- Referral chains: every interview ends with "dua teman seprofesi?" and a same-day intro request.
- Reciprocity: offer something real: a 20-minute call back later sharing (anonymized) how other businesses handle the same problems, or Rp50,000 pulsa. Say which one in the first message.
- Community posts only where group rules allow and only as "riset, bukan jualan"; never mass-broadcast.
- Respect a no: one polite follow-up after 48 hours, then stop.

## 5. Honest outreach

Rules: say it is research; say there is no product; say how long; say what they get; ask for a time. Never say "kami punya aplikasi", never attach screenshots, never promise features.

WhatsApp message (personalise the first line with something you saw on their page):

> Halo Pak/Bu [nama], saya Kevin. Saya lihat [proyek/acara terbaru di IG]. Saya sedang riset kecil tentang bagaimana usaha [renovasi/wedding] mengatur calon pelanggan, penawaran, dan pembayaran lewat WhatsApp. Belum ada produk, jadi bukan jualan; saya ingin belajar dari pengalaman Bapak/Ibu sekitar 20–30 menit lewat telepon/WA call. Sebagai terima kasih, saya kirim [pulsa Rp50 ribu / ringkasan cara usaha lain mengatasinya]. Kira-kira Selasa atau Rabu ada waktu?

Follow-up after 48 hours, once:

> Halo Pak/Bu [nama], menyambung pesan kemarin. Kalau minggu ini padat, boleh usulkan hari lain. Terima kasih.

Via warm intro, let the introducer send the first message; you send the second.

## 6. Scheduling and conducting interviews

- Offer two concrete slots; owners on site prefer late afternoon or evening. Confirm the morning of.
- Phone or WA call by default; video only if they offer; in person for 2–3 same-city prospects (you will see their notebook or sheet).
- Open `INTERVIEW_GUIDE.md` and the note template before the call. Ask for consent to record in the first minute.
- Run sections 1–6 before any mention of a product or price. Section 7 (price) only in the last 5 minutes.
- Ask one question, then stay silent. Let pauses work.
- If they ask "jadi aplikasinya seperti apa?", answer: "Belum ada. Saya ingin tahu dulu masalahnya nyata atau tidak." Then return to their story.

## 7. Recording observations

- Fill the note template within 2 hours of the call, while memory is fresh. Verbatim quotes in quotation marks; your interpretation in a separate "Interpretation" line.
- File name: `docs/private/interviews/2026-10-13-AB-A.md` (date, initials, segment). Never commit.
- Score the five dimensions the same day. Enter the scores and the `most_recent_incident`, `problem_frequency`, `current_workaround`, `time_or_financial_impact`, `required_features`, and `wtp_evidence` columns in the prospect sheet.
- Save sanitized artifacts (customer names blacked out) in `docs/private/artifacts/`.

## 8. Avoiding leading questions

See the table in `INTERVIEW_GUIDE.md` ("Pertanyaan mengarahkan vs netral"). Three checks before asking anything: Is it about the past, not the future? Does it name a specific moment ("kapan terakhir")? Could the answer embarrass my idea? If the answer to the third is "no", the question is probably leading.

## 9. Stated interest versus behavioral evidence

| Counts as evidence | Does not count |
|---|---|
| A dated incident with a rupiah figure | "Sering banget lupa" |
| Shows you their notebook, sheet, or WA labels | "Saya pakai spreadsheet" with nothing to show |
| Replies "oke" on WhatsApp to a price and a start date | "Menarik, nanti kabari" |
| Enters a real lead in the concierge sheet unprompted | Says they will use it |
| Introduces you to a peer the same day | "Nanti saya tanya teman" |
| Asks what happens to their data, how to export, how staff log in | Compliments about the idea |

## 10. Following up with interviewees

- Within 24 hours: thank-you message with one sentence of what you learned from them (shows you listened), plus the pilot summary if they reached section 7 positively, asking for an "oke" reply.
- Same message: the referral ask, specific ("teman yang juga sering dapat klien dari IG").
- Log `next_action` and a date. Re-contact 2–3 weeks later only if they agreed (section 8 of the guide).
- Concierge candidates: send the consent message from `CONCIERGE_TEST.md` §4 and book the 15-minute setup call for the next day.

## 11. End-of-day checklist

- [ ] 25 prospects listed, scored, sourced
- [ ] 15 messages sent, each personalised, none claiming a product
- [ ] Replies and bookings logged in the sheet
- [ ] 10 interview slots held for Tue–Fri
- [ ] 5 concierge candidates marked
- [ ] `PROJECT_STATUS.md` counts updated (contacted / replied / booked)
