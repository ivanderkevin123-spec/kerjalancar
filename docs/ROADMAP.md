# Roadmap — 8 weeks (2026-10-12 → 2026-12-13)

Each build week is one vertical slice: migration, logic, UI, tests, docs. A slice is done only when its acceptance criteria pass in CI and the founder has clicked through it on a phone. Gates can stop the plan.

| Week | Dates | Outcome | Acceptance criteria |
|---|---|---|---|
| 0 Validation | Oct 12–18 | Gate G1 decision; toolchain ready | ≥ 12 interviews scored; `VALIDATION_FINDINGS.md` written; thresholds in `VALIDATION_PLAN.md` §5 evaluated; Go 1.27 + Postgres + sqlc + goose installed (reversible); decisions D-001..D-008 recorded |
| 1 Skeleton + auth | Oct 19–25 | A business can sign up, log in, and invite one staff member; nobody can see another workspace | Repo, Dockerfile, CI green with Postgres; migrations; register/login/logout/reset; workspace + membership + roles; session security; cross-tenant test harness passes for `workspace` and `membership`; health check; structured logs |
| 2 Customers + leads | Oct 26–Nov 1 | Owner records a lead in ≤ 15 s on a phone and moves it through the pipeline | Customer CRUD with WA number validation; lead stages Baru → Dihubungi → Penawaran → Menang/Kalah; activity log; list/search; empty states; cross-tenant matrix extended; mobile layout checked on a budget Android |
| 3 Quotations | Nov 2–8 | Owner sends a quotation as a link and sees whether it was opened | Line items with integer-rupiah math and rounding tests; validity date; status draft/sent/accepted/rejected/expired; public share page with revocable token and print CSS; "opened at" tracking; WA template "kirim penawaran" with wa.me link |
| 4 Invoices + payments | Nov 9–15 | Owner records DP and installments and always knows the outstanding balance | Invoice from quotation or blank; append-only payments with idempotency; reversal flow; balance computed in SQL and asserted; due dates in workspace zone; receivables list; audit events |
| 5 Follow-ups + templates | Nov 16–22 | Owner opens the app and sees exactly who to contact today | Follow-up tasks on leads and invoices; "Hari ini" and "Terlambat" lists; auto follow-up suggestion after quotation sent; template library with placeholders; one-tap wa.me; staging deployment **(needs approval, D-005)**; smoke tests |
| 6 Dashboard + onboarding + pilots | Nov 23–29 | 3–5 pilot businesses live and paying | Dashboard (pipeline value, quotations by status, receivables, today's follow-ups); onboarding to first lead in ≤ 2 min; Indonesian copy review; trial/plan flags; manual billing invoice; pilots onboarded in person or by call; founder usage dashboard |
| 7 Hardening | Nov 30–Dec 6 | Pilots trust the data; incidents are diagnosable | Threat-model review done; backup restore drill executed and documented; error monitoring wired; rate limits verified; data export; privacy notice; fixes from pilot feedback; optional RLS |
| 8 Subscription-readiness + G2 | Dec 7–13 | Gate G2 decision with usage and payment evidence | Pilot metrics computed from events (`VALIDATION_PLAN.md` §7); month-2 invoices sent; churn calls done; decision recorded; roadmap for next phase or stop |

## Gates

- **G1 (Oct 18):** proceed / narrow / pivot / stop per `VALIDATION_PLAN.md` §5. Weeks 1–8 do not start without it.
- **Staging approval (Week 5):** provisioning any hosted resource requires the founder's explicit approval and a stated monthly cost.
- **G2 (Dec 13):** continue / narrow / pivot / stop per `VALIDATION_PLAN.md` §8.

## Slip policy

If a week slips, cut scope inside the slice (fewer fields, fewer views) before cutting tests or the cross-tenant matrix. Never move Week 1 security work later.
