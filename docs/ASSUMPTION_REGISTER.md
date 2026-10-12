# Assumption Register

Status: **FACT** (verified, source noted) · **ASSUMED** (acting as if true, not yet tested) · **HYPOTHESIS** (explicitly to be tested) · **REJECTED**.

| ID | Assumption | Area | Status | Evidence / source | How we test it | Deadline |
|---|---|---|---|---|---|---|
| A-01 | Owners of small service businesses sell primarily through WhatsApp chat | Market | ASSUMED | Kantar 2022 via Meta Indonesia (secondary); widely observed | Interview Q5; artifact review | G1 |
| A-02 | Owners lose real money to forgotten leads and missed follow-ups | Customer | HYPOTHESIS | None yet | Interview Q3, scoring "pain ≥ 2" rate ≥ 4/5 | G1 |
| A-03 | Owners lose money or time to unbilled or late balances after DP | Customer | HYPOTHESIS | None yet | Interview Q4 | G1 |
| A-04 | Owners will record a lead in a second place if it takes ≤ 15 seconds on a phone | Product | HYPOTHESIS | None yet; main adoption risk (R-01) | Concierge test (`CONCIERGE_TEST.md`): self-recorded real lead within 24 h and return on ≥ 2 later days; then pilot "time to first lead" and weekly writes | G1, W7 |
| A-05 | Owners will pay ≥ Rp149,000/month per workspace | Commercial | HYPOTHESIS | Price gap between Paper.id (Rp90k) and Qontak (Rp2M) is a FACT; willingness is not | Commitment ladder ≥ 3 for ≥ 3 interviewees; pilot invoices paid | G1, W8 |
| A-06 | Segment A (renovation/interior) has the highest pain per incident and weakest incumbent | Market | ASSUMED | Reasoning in `PRODUCT_BRIEF.md` §3; no incumbent found in search | Parallel A/B interviews | G1 |
| A-07 | A simple line-item quotation (no full RAB) is acceptable for v1 to most contractors | Product | HYPOTHESIS | None | Artifact review of their current quotations; Q2 probes | G1 |
| A-08 | Click-to-chat (wa.me) links with prefilled text are sufficient; owners prefer sending messages themselves | Product | ASSUMED | wa.me links are an official WhatsApp feature; API costs money and needs approval | Interview Q7 ranking; pilot usage of templates | W7 |
| A-09 | No WhatsApp Business API, no automated sending in v1 | Scope | FACT (decision) | Founder brief | — | — |
| A-10 | Owners are usually on mid-range Android with intermittent connectivity | Tech | ASSUMED | General market knowledge, not measured | Ask device in interviews; test on a budget Android | W2 |
| A-11 | IDR has no sub-unit in practice; store money as integer rupiah | Tech | FACT | Rupiah sen are not used in commerce | — | — |
| A-12 | A workspace-level time zone (WIB default) with date-only due dates is enough | Tech | ASSUMED | Indonesia has 3 zones; businesses operate locally | Pilot workspaces outside WIB | W7 |
| A-13 | A single Postgres with workspace_id scoping and composite foreign keys is sufficient isolation for v1 | Tech | ASSUMED | Standard practice; RLS optional hardening | Cross-tenant test suite; threat model review | W1, W7 |
| A-14 | PDF can be replaced in v1 by a print-friendly HTML page plus share link | Product | HYPOTHESIS | Owners currently send PDFs or photos (to confirm) | Interview Q2; pilot feedback | G1, W6 |
| A-15 | In-app reminder lists (today / overdue) are enough; no push, email, or WA reminders in v1 | Product | HYPOTHESIS | None | Pilot: do owners open the app daily without a nudge? | W7 |
| A-16 | Hosting in Singapore is acceptable for pilot customers' data | Legal | ASSUMED | UU PDP 27/2022 exists; specifics not yet verified | Checklist item 15 in `COMPETITOR_RESEARCH.md` | Before staging |
| A-17 | Founder can run interviews and build in parallel over 8 weeks | Delivery | ASSUMED | Solo founder | Weekly status; slip triggers roadmap cut | Weekly |
| A-18 | Go server-rendered HTML + HTMX delivers the MVP faster and with less operational surface than Next.js + Go API for a solo founder | Tech | ASSUMED | Workflow inventory in `ARCHITECTURE.md` §1.1; founder's Go and Ruby background; effort estimate is judgment, not measured | Decision D-012; revisit if the Week 3 quotation editor misses its acceptance criteria because of HTMX limits | G1 / W3 |
| A-19 | A recorded payment is a claim by the owner, not evidence of a bank transfer | Product | FACT (principle) | Founder brief; no bank integration | UI copy + docs | — |
| A-20 | Existing pricing of Kledo/Kommo/Bigin as recorded is current | Market | SECONDARY | Third-party pages | Checklist items 3–5 | G1 |
