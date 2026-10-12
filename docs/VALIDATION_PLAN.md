# Validation Plan — First 7 Days (2026-10-12 → 2026-10-18)

Goal: decide, with evidence, whether to build KerjaLancar for renovation/interior contractors (segment A), switch to wedding organizers/event vendors (B), narrow further, or stop. No product code is written until Gate G1 passes, except reversible toolchain setup.

Rules: no fabricated interviews, no paraphrased "typical customer" claims. Every finding links to a dated interview note with the interviewee's initials and segment. Opinions ("that sounds useful") are logged but score zero; only past behavior and commitments count.

## 1. Targets

| Item | Target | Minimum to make a decision |
|---|---|---|
| Interviews, segment A (contractors) | 7 | 5 |
| Interviews, segment B (WO / event vendors) | 5 | 3 |
| Workflow artifacts collected (sanitized screenshots of WA chats, sheets, quotations, invoices) | 6 | 3 |
| Paid-pilot commitments (named price, named start date) | 4 | 3 in the chosen segment |
| Competitor checklist completed (`COMPETITOR_RESEARCH.md`, section 4) | 100% of "must verify" items | 100% |

## 2. Recruiting

Screening criteria (ask before booking):

- Owner or the person who handles customer WhatsApp and quotations personally
- Business has existed ≥ 1 year and takes DP before work starts
- At least 5 new inquiries per month (A) or 10 per month (B)
- Not currently paying for a CRM (if they are, still interview: learn what it does for them)

Channels (the founder does the outreach; nothing is posted publicly by Claude):

- Personal and family network, first-degree referrals (highest hit rate)
- Instagram accounts of local contractors, interior studios, WO, decor, catering, MUA, photographers
- Facebook groups for renovasi/interior and for wedding vendors
- Marketplace service listings (Tokopedia/Shopee jasa, OLX jasa) for contractors who advertise
- Every interview ends by asking for two referrals

Outreach message (Bahasa Indonesia, send from the founder's own WhatsApp):

> Halo Pak/Bu [nama], saya Kevin, sedang riset cara usaha [renovasi/WO] mengelola calon pelanggan dan pembayaran lewat WhatsApp. Bukan jualan, saya ingin belajar dari pengalaman Bapak/Ibu selama 30 menit (telepon/WA call/ketemu). Sebagai terima kasih saya kirim [pulsa/kopi Rp50 ribu]. Kira-kira ada waktu minggu ini?

Incentive is optional; decide per contact. Do not promise the product.

## 3. Daily schedule

| Day | Date | Activities | Output |
|---|---|---|---|
| 1 | Mon 12 Oct | Founder reviews Phase A docs, approves decisions D-001..D-008. Build prospect list of 25 (15 A, 10 B). Send 15 outreach messages. | Prospect sheet, approvals |
| 2 | Tue 13 Oct | Send remaining outreach. First 2–3 interviews. Complete competitor "must verify" items 1–6. | Interview notes #1–3 |
| 3 | Wed 14 Oct | 3 interviews. Collect workflow artifacts. Competitor items 7–12. | Notes #4–6, artifacts |
| 4 | Thu 15 Oct | 3 interviews. Mid-point review: is A recruitable? If < 3 A interviews booked by now, shift effort to B. | Notes #7–9, mid-point memo |
| 5 | Fri 16 Oct | 2–3 interviews. Re-contact the strongest 4 with the pilot offer (price + start date). | Notes #10–12, commitment log |
| 6 | Sat 17 Oct | Synthesis: fill scoring sheet, tally thresholds, write `docs/VALIDATION_FINDINGS.md`. | Findings doc |
| 7 | Sun 18 Oct | Gate G1 decision with the founder. Update `DECISIONS.md`, `ROADMAP.md`, `PROJECT_STATUS.md`. | Go / narrow / pivot / stop |

If interviews slip, extend by up to 4 days rather than lowering the minimums.

## 4. Methods

1. **Problem interviews** (primary). Script in `INTERVIEW_GUIDE.md`. 30 minutes. Record with consent or take verbatim notes. Score the same day.
2. **Workflow artifact review.** Ask for sanitized screenshots: how a lead looks in their WA, their quotation document, their payment tracking (sheet, notebook, WA label). Count fields they actually track. This tells us the minimum data model and whether double entry is survivable.
3. **Commitment test.** For interviewees who scored pain ≥ 2, present the one-page concept (verbally or a static mock), then ask: "Kalau mulai [tanggal], Rp149.000/bulan, Bapak/Ibu ikut?" Log the answer on the commitment ladder: (4) pays first month now · (3) agrees to price and a start date in writing (WA message) · (2) wants it, no date · (1) polite interest · (0) no. We do not collect money before a pilot start; a level-3 WA message is the evidence.
4. **Concierge sheet (optional, cheap).** Offer 2–3 owners a Google Sheet "Pipeline & Tagihan" template today and check after 5 days whether they entered anything. Anyone who fills a sheet unprompted is a strong pilot candidate; anyone who does not is a warning about double entry.

## 5. Evidence thresholds and decision framework (Gate G1)

Count only the primary segment unless noted.

**PROCEED (build for A)** requires all of:

- ≥ 5 A interviews completed with scored notes
- ≥ 4 of them describe a *specific* incident in the last 6 months of a lost lead, missed follow-up, or unbilled/late balance, with an estimated rupiah cost
- ≥ 4 already use a manual workaround (sheet, notebook, WA labels, calendar) for the same problem
- ≥ 3 commitments at level ≥ 3 at Rp149,000/month or higher, with a start date on or before 2026-11-23 (pilot week)
- Competitor checklist complete and no tool found that already covers lead → quotation → follow-up → DP/balance for this segment at ≤ Rp200,000/month with Indonesian UI
- The top-3 requested capabilities across interviews fall inside the MVP scope (lead capture, follow-up reminders, quotation link, DP/balance tracking)

**NARROW** if PROCEED fails only on commitments or on scope: pick the sub-niche where the incidents cluster (e.g., interior studios rather than general contractors) and run 5 more interviews there over 4 days.

**PIVOT to B** if A fails on pain or workaround criteria but B meets the same thresholds with ≥ 3 interviews. Re-run the commitment test for B before building.

**STOP (or re-think the problem)** if neither segment reaches ≥ 50% specific-incident rate, or no one commits at any price ≥ Rp99,000/month. Write up what we learned and what alternative problem the interviewees kept raising.

## 6. Interview scoring (per interviewee)

| Field | Scale |
|---|---|
| Pain specificity | 0 none · 1 vague · 2 specific incident · 3 specific incident with rupiah cost |
| Frequency | 0 rare · 1 quarterly · 2 monthly · 3 weekly |
| Current workaround | 0 none · 1 memory/WA scroll · 2 notebook or WA labels · 3 spreadsheet or app |
| Commitment ladder | 0–4 as defined above |
| Fit with MVP | 0 needs something else · 1 partial · 2 core flow matches |
| Must-have outside MVP | free text (calendar, RAB, inventory...) |

A "strong signal" interviewee scores pain ≥ 2, workaround ≥ 2, commitment ≥ 3.

## 7. Pilot metrics (defined now, measured from Week 6)

Each is an event or query in the app, not a survey.

| Metric | Definition |
|---|---|
| Time to first successful workflow | Minutes from workspace creation to first lead with a scheduled follow-up |
| Leads recorded / followed up | Leads created per week; follow-ups completed on or before due date ÷ follow-ups due |
| Quotations created / accepted | Count per week; accepted ÷ sent |
| Outstanding followed up | Overdue invoices that received a logged follow-up within 3 days of due date ÷ overdue invoices |
| Weekly active businesses | Workspaces with ≥ 1 write action in the week |
| Repeat usage | Workspaces active in ≥ 3 of the last 4 weeks |
| Willingness to pay | Pilots that pay the first invoice by its due date ÷ pilots |
| Pilot → paid conversion | Paid month 2 ÷ pilots started |
| Churn reasons | Mandatory free-text on cancellation plus a founder call |

Sign-ups are not a metric here on purpose.

## 8. Gate G2 (end of Week 8): continue, narrow, pivot, or stop

Continue if ≥ 3 of 5 pilots are weekly-active for 3 consecutive weeks and ≥ 2 pay for month 2 unprompted. Narrow if usage is high but only in one sub-niche or one feature (follow the feature). Pivot if pilots use the tool but will not pay (consider the feature they use as a different product). Stop if < 2 pilots reach weekly-active by Week 8.
