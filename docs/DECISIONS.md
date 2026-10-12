# Decision Log

Format follows the founder's other projects: ID, date, decision, rationale, decided by. Status: **PROPOSED** (needs founder approval) · **APPROVED** · **SUPERSEDED**.

| ID | Date | Decision | Rationale | By | Status |
|---|---|---|---|---|---|
| D-001 | 2026-10-12 | Stack: Go modular monolith with server-rendered HTML, Turbo + Stimulus, Tailwind, PostgreSQL. Not Rails, despite the brief naming Active Job and RSpec/Minitest. | Founder's Go expertise; boring and reliable; MVP is forms and lists. Trade-offs in `ARCHITECTURE.md` §1 | Claude (proposal) | PROPOSED |
| D-002 | 2026-10-12 | Primary validation segment: A (renovation/interior contractors). Interview B (WO/event vendors) in parallel as a hedge. Defer C (workshops). | `PRODUCT_BRIEF.md` §3; confidence ~55% | Claude (proposal) | PROPOSED |
| D-003 | 2026-10-12 | Price anchors for willingness-to-pay tests: Rp149,000/month per workspace (≤ 3 users); second anchor Rp249,000 for contractors; 14-day trial; pilots paid from month one at a discount | Gap between Paper.id Rp90k and Qontak Rp2M; commitments, not opinions | Claude (proposal) | PROPOSED |
| D-004 | 2026-10-12 | Upgrade the local Go toolchain from 1.17.5 to 1.27.x via Homebrew, keeping `/usr/local/go` untouched; set PATH per project (direnv or shell alias) so other Go projects are unaffected | Go 1.22+ routing and 1.25+ CSRF support are needed; founder's other projects may pin 1.17 | Claude (proposal) | PROPOSED |
| D-005 | 2026-10-12 | Hosting provider and managed Postgres chosen only at Week 5 with an explicit cost line; candidates Fly.io (sin), Railway (Singapore), Render (Singapore). Nothing provisioned before approval | No spend before validation | Claude (proposal) | PROPOSED |
| D-006 | 2026-10-12 | No public landing page or public sign-up during validation; recruitment is by direct outreach and referral | Publishing is externally visible; interviews beat sign-ups as evidence | Claude (proposal) | PROPOSED |
| D-007 | 2026-10-12 | Git repository initialised in `kerjalancar/`; first commit of Phase A docs to be made by or with the founder's approval | Reproducibility; founder controls commits | Claude (proposal) | PROPOSED |
| D-008 | 2026-10-12 | Documentation language: engineering docs in English; customer-facing material, interview script, and all UI copy in Bahasa Indonesia | Matches the brief's language and the users' language | Claude (proposal) | PROPOSED |
| D-009 | 2026-10-12 | v1 has no background job queue and no PDF generation: reminders are queries; quotations and invoices are shareable pages with print CSS | YAGNI; both can be added when pilots ask | Claude (proposal) | PROPOSED |
| D-010 | 2026-10-12 | Money stored as integer rupiah; quantities in thousandths; payments append-only with reversals and idempotency keys | Correctness and audit | Claude (proposal) | PROPOSED |
| D-011 | 2026-10-12 | KerjaLancar is an operational CRM, not accounting software; no ledger, tax computation, bank reconciliation, or fund custody | `PRODUCT_BRIEF.md` §5; founder brief | Claude (proposal) | PROPOSED |
