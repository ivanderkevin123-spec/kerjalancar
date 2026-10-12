# Roadmap — 8 weeks (2026-10-12 → 2026-12-13)

Each build week is one vertical slice: migration, logic, UI, tests, docs. A slice is done only when its acceptance criteria pass in CI and the founder has clicked through it on a phone. Gates can stop the plan.

| Week | Dates | Outcome | Acceptance criteria |
|---|---|---|---|
| 0 Validation | Oct 12–18 | Gate G1 decision | ≥ 12 interviews scored; concierge test run with 3–5 owners (`CONCIERGE_TEST.md`); `VALIDATION_FINDINGS.md` written; six-outcome thresholds in `VALIDATION_PLAN.md` §5 evaluated; D-012..D-017 decided; toolchain installs (Go 1.27, Postgres, sqlc, goose) only after the founder's explicit go-ahead |
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

## Traceability: slice → assumptions, risks, and the test that proves it

| Week | Validates assumptions | Mitigates risks | Proof |
|---|---|---|---|
| 0 | A-02, A-03, A-04, A-05, A-06, A-07, A-14 | R-02, R-03 | Scored interviews, concierge metrics, G1 record |
| 1 | A-13 | R-04 | Cross-tenant matrix for workspace and membership; session and CSRF tests; `govulncheck` |
| 2 | A-04, A-10 | R-01, R-05 | Time-to-record measured on a budget Android; cross-tenant matrix for customers and leads |
| 3 | A-07, A-14 | R-05, R-09, R-14, R-16 | Money rounding table tests; share-token tests; JS island stays within one controller |
| 4 | A-11, A-19 | R-09, R-11 | Balance computed in SQL equals Go; idempotency and reversal tests; `FOR UPDATE` concurrency test |
| 5 | A-12, A-15 | R-10 | Zone-boundary tests for "hari ini"; staging smoke tests after approval |
| 6 | A-05, A-08 | R-02, R-06 | Pilots paying month one; onboarding ≤ 2 min measured |
| 7 | A-16 | R-04, R-07, R-15 | Restore drill log; threat-model checklist; export/delete verified |
| 8 | A-05 | R-02, R-15 | Pilot metrics from events; G2 record |

## Slip policy

If a week slips, cut scope inside the slice (fewer fields, fewer views) before cutting tests or the cross-tenant matrix. Never move Week 1 security work later.
