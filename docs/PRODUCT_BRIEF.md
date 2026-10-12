# KerjaLancar — Product Brief (Phase A)

Date: 2026-10-12. Status: DRAFT for founder review. Nothing here is validated by customers yet.

Legend used throughout the docs: **[FACT]** verified by us or a cited source · **[ASSUMPTION]** we are acting as if true · **[HYPOTHESIS]** to be tested · **[RECOMMENDATION]** our proposal.

## 1. One-paragraph summary

KerjaLancar is a mobile-first web app for small Indonesian service businesses whose sales happen in WhatsApp. It gives the owner one place to record a lead in seconds, send a quotation as a link, get reminded to follow up, record DP (down payment) and remaining balance, and see who owes what. It does not send WhatsApp messages for the owner; it prepares them and opens WhatsApp with one tap (click-to-chat). It is an operational CRM, not accounting software.

## 2. Assessment of the business hypothesis

**The hypothesis.** Owners manage leads, quotations, follow-ups, invoices, deposits, and outstanding payments through WhatsApp, spreadsheets, and memory. A lightweight WhatsApp-first workflow tool reduces forgotten leads and overdue follow-ups without a full CRM.

**What supports it**

- [FACT] WhatsApp is the dominant business messaging channel in Indonesia. Meta cited a 2022 Kantar study that 7 of 10 Indonesians prefer business messaging to phone or email (see `COMPETITOR_RESEARCH.md`, sources). WhatsApp passed 3 billion monthly users globally in 2025.
- [FACT] The incumbent Indonesian WhatsApp CRM (Mekari Qontak) starts at Rp2,000,000/month with a 5-user minimum (fetched from qontak.com/en/pricing on 2026-10-12). That is priced for mid-size companies, not a 2–10 person contractor or wedding organizer. There is a visible price gap below it.
- [FACT] Paper.id's pricing page lists invoicing, purchase orders, receipts, and payment reminders (free tier) but does not list quotations or a lead pipeline. The affordable invoicing tools are invoice-first, not lead-first.
- [ASSUMPTION] High-ticket service businesses with multi-stage payments lose real money to a single forgotten follow-up or an unbilled balance. One lost Rp50 million project dwarfs a Rp150,000/month subscription. This is the ROI story and must be confirmed with specific incidents in interviews.

**What threatens it**

- [HYPOTHESIS, high risk] Double entry. The chat is already in WhatsApp. Any tool that asks the owner to retype what was just said in chat competes with "doing nothing". The free WhatsApp Business app already offers labels, quick replies, and a catalog. If recording a lead takes more than ~15 seconds on a phone, adoption will fail regardless of features.
- [HYPOTHESIS] Willingness to pay among micro businesses is low and tool fatigue is real. Many owners have tried an app and gone back to a notebook. Evidence from validation is required before building more than the skeleton.
- [HYPOTHESIS] The three candidate segments are not one market. Contractors need line-item quotations with revisions; wedding vendors need package quotations plus a date calendar; workshops need job cards and parts. A generic tool may be "fine" for all and "chosen" by none.
- [FACT] We have zero customer interviews and zero paying customers. Every statement about customers in this document is a hypothesis.

**Verdict.** The hypothesis is plausible and the price gap is real, but the deciding question is behavioral, not technical: will an owner record leads and payments in a second app while selling in WhatsApp? That is what the first seven days must answer.

## 3. Segment analysis

Scores are our reasoning, not data. Each must be revisited after interviews.

| Criterion | A. Renovation / interior contractors | B. Wedding organizers / event vendors | C. Automotive workshops |
|---|---|---|---|
| Pain per incident | Very high (projects Rp20M–500M+, DP + termin + pelunasan) | High (DP + pelunasan, dated events) | Low (per-job ticket small) |
| Pain frequency | Medium (5–30 leads/month, long cycle) | High (many DM/WA inquiries, seasonal peaks) | High volume but mostly walk-in |
| Fit with lead → quotation → follow-up → DP/balance flow | Strong | Strong, plus a date calendar need | Weak: estimate → job card → parts → pickup |
| Quotation complexity | High: RAB-style line items, revisions, add/remove work | Medium: packages with add-ons | Medium: parts + labor |
| Existing software in segment | Little tailored software; Excel RAB dominates. "Aplikasi 101" offers free estimates/quotes (international, localized) | Bridestory Pro gives vendors inquiry/message management inside the marketplace (third-party source, needs in-app verification) | Workshop apps exist (e.g., Bengkelbaik: vehicle data, service history, reminders) |
| Reachability for interviews in 7 days | Medium (Instagram, FB groups, marketplace listings, referrals) | High (Instagram-native, visible vendor communities) | Medium (physical visits) |
| Likely willingness to pay | Highest (bigger revenue, already pays for tools/materials) | Mixed: WO firms yes, micro vendors unclear | Low to medium; expects POS-style tooling |
| Scope-creep danger | RAB depth, progress billing, project management | Calendar, availability, vendor coordination, rundown | Inventory, parts, POS, accounting |

**[RECOMMENDATION] Primary segment: A, renovation and interior contractors.** Confidence: medium-low, roughly 55% that A is the right first niche. Reasons: highest pain per incident, cleanest fit with the core flow, weakest incumbent, highest plausible willingness to pay.

**Hedge: interview B in parallel** during the same seven days (roughly 7 interviews for A, 5 for B). The marginal cost of five interviews is low, and B has the better reach if A proves hard to recruit. **Defer C** unless A and B both fail: it fits the hypothesis least and already has operational software.

**Main uncertainties for A**

1. Quotation depth. If most contractors require a full RAB (uraian, volume, satuan, harga satuan, jumlah) with revisions and "pekerjaan tambah-kurang", the quotation module becomes the product and the 6–8 week budget is at risk. Mitigation: quotation v1 = simple line items; RAB import from Excel is a later feature if demanded.
2. Lead volume may be too low for "forgotten leads" to hurt. The pain may sit in follow-up timing and unbilled balances instead. Interviews must locate the pain precisely.
3. Owners are often on site, phone-only, with poor connectivity. Mobile web must work on mid-range Android with intermittent data.

## 4. Proposed MVP scope

In scope for v1 (matches the founder's checklist):

1. Authentication, workspace per business, tenant-scoped authorization, owner + staff roles
2. Customers: name, WhatsApp number, source, notes, interaction history
3. Leads: pipeline stages Baru → Dihubungi → Penawaran → Menang / Kalah, with estimated value and activity log
4. Quotations: line items, totals, validity date, status, public share link with a token, print-friendly view (browser "Save as PDF")
5. Invoices: line items, due date; payments recorded manually as DP, cicilan, or pelunasan; outstanding balance computed server-side
6. Follow-ups: tasks with due dates, "Follow-up hari ini" list, overdue list, due-date reminders for invoices, all in-app
7. WhatsApp templates with placeholders and click-to-chat (`wa.me`) links; the owner sends the message
8. Dashboard: pipeline value, quotations by status, outstanding receivables, today's follow-ups
9. Onboarding (first lead in under 2 minutes), audit-relevant activity history, responsive mobile-first UI in Bahasa Indonesia
10. Subscription readiness: trial period, plan flag, manual billing. No payment gateway in v1.

Explicitly out of v1 unless validated: WhatsApp Business API automation, AI chat extraction, native apps, accounting (ledger, tax reports), payment custody or fund transfers, integrations, push notifications, email digests, multi-currency, advanced analytics.

## 5. Boundary: operational CRM vs accounting software

KerjaLancar records what the owner *says* happened: a lead arrived, a quotation went out, a customer says they paid Rp10,000,000 DP. It computes outstanding balances from those records. It does not reconcile bank statements, does not produce a general ledger, does not compute PPN or PPh obligations, and does not generate financial statements. A recorded payment is a claim entered by the owner, never proof that money moved. Owners who need bookkeeping export records to Kledo, Jurnal, or their accountant. This boundary keeps the product honest, keeps us out of regulated territory, and is repeated in the UI copy near every payment form.

## 6. Pricing hypothesis (to be tested, not decided)

[HYPOTHESIS] Rp149,000/month per workspace, up to 3 users, with a 14-day trial. Anchors: Paper.id PaperPlus Rp90,000/month, Kledo Pro Rp159,900/month (third-party comparison, verify), Qontak from Rp2,000,000/month. A second anchor of Rp249,000/month will be tested with contractors. The interview guide asks for a commitment, not an opinion.

## 7. What would make us stop

If after 12 interviews fewer than half of the primary segment can describe a specific, recent, costly incident of a forgotten lead, missed follow-up, or unbilled balance, the problem is not painful enough to pay for, and we stop or pivot before writing product code. Full thresholds are in `VALIDATION_PLAN.md`.
