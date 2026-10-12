# Validation Plan — First 7 Days (2026-10-12 → 2026-10-18)

Goal: decide, with evidence, whether KerjaLancar should keep building for renovation/interior contractors (segment A), switch to wedding organizers/event vendors (B), narrow further, revise scope, or pause. Under D-019 (build-first, 2026-10-12) development runs in parallel from Milestone 1; Gate G1 is the checkpoint for the validation track, and its NARROW / REVISE / PIVOT / STOP outcomes change scope or pause work rather than gate the first commit.

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

Outreach message, prospect sourcing, prioritisation, and scheduling steps are in `DAY1_EXECUTION.md` (single canonical copy of the WhatsApp message). Incentive is optional; decide per contact. Never imply the product exists.

## 3. Daily schedule

| Day | Date | Activities | Output |
|---|---|---|---|
| 1 | Mon 12 Oct | D-001..D-011 approved (done). Run `DAY1_EXECUTION.md`: prospect list of 25 (15 A, 10 B) in a private copy of `templates/prospects.csv`, first 15 outreach messages, 3–5 concierge invitations. | Prospect sheet, outreach log |
| 2 | Tue 13 Oct | Send remaining outreach. First 2–3 interviews. Concierge setup calls with the first 2 volunteers. Competitor "must verify" items 1–6. | Interview notes #1–3, concierge sheets live |
| 3 | Wed 14 Oct | 3 interviews. Collect workflow artifacts. Competitor items 7–12. | Notes #4–6, artifacts |
| 4 | Thu 15 Oct | 3 interviews. Mid-point review: is A recruitable? If < 3 A interviews booked by now, shift effort to B. | Notes #7–9, mid-point memo |
| 5 | Fri 16 Oct | 2–3 interviews. Re-contact the strongest 4 with the pilot offer (price + start date). | Notes #10–12, commitment log |
| 6 | Sat 17 Oct | Synthesis: fill scoring sheet, tally thresholds, write `docs/VALIDATION_FINDINGS.md`. | Findings doc |
| 7 | Sun 18 Oct | Gate G1 decision with the founder. Update `DECISIONS.md`, `ROADMAP.md`, `PROJECT_STATUS.md`. | Go / narrow / pivot / stop |

If interviews slip, extend by up to 4 days rather than lowering the minimums (see EXTEND in §5). The concierge test (`CONCIERGE_TEST.md`) runs in parallel from Day 2: setup calls Days 2–3, observation through Day 7; its cohort result is an input to G1.

## 4. Methods

1. **Problem interviews** (primary). Script in `INTERVIEW_GUIDE.md`. 30 minutes. Record with consent or take verbatim notes. Score the same day.
2. **Workflow artifact review.** Ask for sanitized screenshots: how a lead looks in their WA, their quotation document, their payment tracking (sheet, notebook, WA label). Count fields they actually track. This tells us the minimum data model and whether double entry is survivable.
3. **Commitment test.** For interviewees who scored pain ≥ 2, present the one-page concept (verbally or a static mock), then ask: "Kalau mulai [tanggal], Rp149.000/bulan, Bapak/Ibu ikut?" Log the answer on the commitment ladder: (4) pays first month now · (3) agrees to price and a start date in writing (WA message) · (2) wants it, no date · (1) polite interest · (0) no. We do not collect money before a pilot start; a level-3 WA message is the evidence.
4. **Concierge test (required, 3–5 owners).** Protocol, metrics, limitations, and privacy safeguards in `CONCIERGE_TEST.md`. It is the only direct test before software exists of whether an owner will enter real leads and payments in a second place (A-04). Its cohort result feeds Gate G1 (§5).

## 5. Evidence thresholds and decision framework (Gate G1, Sunday 2026-10-18)

The date is a checkpoint, not an approval. Count the primary segment (A) unless noted. "Specific incident" means a dated event in the last 6 months with an estimated rupiah cost. "Commitment" means commitment-ladder level ≥ 3: price and start date agreed in writing on WhatsApp. Evaluate the outcomes in the order below; the first whose conditions hold is the decision.

**STOP** (or re-think the problem) when, across both segments:

- ≥ 8 interviews completed and < 50% describe a specific incident, or
- 0 commitments at any price ≥ Rp99,000/month after ≥ 8 interviews and the concierge test, or
- Concierge: 0 of ≥ 3 participants record a real lead themselves within 5 days

Write up what we learned and what alternative problem interviewees kept raising. Development pauses pending the founder's decision (D-019 makes this a founder call, not an automatic halt).

**PIVOT to B** when A fails the PROCEED pain or workaround criteria below but B, with ≥ 3 interviews, meets them and has ≥ 2 commitments. Run 3 more B interviews and re-run the commitment test before Week 1.

**REVISE PRODUCT** when pain and workaround criteria are met but the product does not fit:

- ≥ 50% of A interviewees rank a capability outside the MVP (full RAB with volumes and unit analysis, progress billing, event calendar) as must-have, or
- Concierge participants record leads but say the quotation or payment flow does not match how they work

Revise `PRODUCT_BRIEF.md` scope and re-gate within 5 days. If a complete RAB system is the must-have, reassess the niche rather than adding the subsystem (D-002 is re-opened).

**EXTEND** when evidence is insufficient rather than negative:

- Fewer than 5 scored A interviews, or fewer than 3 B interviews when B matters for the decision, or
- Pain criterion met by 3 of 5 (borderline), or commitments at 1–2 (borderline), or
- Concierge has < 3 participants or < 5 observation days

Extend by up to 7 days with a named target (e.g., "3 more A interviews, 2 more concierge participants"). Maximum two extensions; after that choose between PIVOT and STOP. Insufficient evidence never counts as validation; under D-019 building continues on the committed scope, but segment-specific features and pilot invitations wait for evidence.

**NARROW** when PROCEED criteria are met overall but the evidence clusters:

- ≥ 2 of 3 commitments come from one sub-type (e.g., interior studios rather than general contractors), or
- Must-have features split by sub-type

Name the sub-niche, run 5 more interviews there over ≤ 4 days, then PROCEED for that sub-niche only.

**PROCEED** (confirm A as the pilot segment and continue building) requires all of:

- ≥ 5 A interviews completed with scored notes
- ≥ 4 of them describe a specific incident of a lost lead, missed follow-up, or unbilled/late balance
- ≥ 4 already use a manual workaround (sheet, notebook, WA labels, calendar) for the same problem
- ≥ 3 commitments at Rp149,000/month or higher with a start date on or before 2026-11-23 (pilot week)
- Concierge: ≥ 2 of 3 (or ≥ 3 of 5) participants record a real lead themselves within 24 h and return on ≥ 2 later days unprompted
- Competitor checklist (`COMPETITOR_RESEARCH.md` §4 "must verify") complete and no tool found that already covers lead → quotation → follow-up → DP/balance for this segment at ≤ Rp200,000/month with Indonesian UI
- The top-3 requested capabilities across interviews fall inside the MVP scope

Record the outcome, the counts behind it, and the interview IDs in `VALIDATION_FINDINGS.md` and `DECISIONS.md`.

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
