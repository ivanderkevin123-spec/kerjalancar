# Roadmap — build-first with parallel validation (2026-10-12 → 2026-12-13)

Strategy per D-019: development starts now in small milestones (M1–M7, detailed in `MVP_PLAN.md`); customer validation runs alongside (`VALIDATION_PLAN.md`, `DAY1_EXECUTION.md`, `CONCIERGE_TEST.md`). A milestone is done only when its acceptance criteria pass in tests the founder can re-run and the feature has been clicked through on a phone. Gates can narrow, revise, pivot, or pause scope.

| Week | Dates | Build track | Validation track | Acceptance (build) |
|---|---|---|---|---|
| 0 | Oct 12–18 | M1 bootstrap; M2 auth + workspace + layout (starts once the toolchain is available) | Day 1 package, 12 interviews, concierge test, Gate G1 on Oct 18 | M1: `make test` green against Postgres, `/healthz` and `/readyz` live, migrations run; M2: sign-up/login/logout, invite staff, cross-tenant matrix passes for workspace and membership |
| 1 | Oct 19–25 | M3 customers, leads, pipeline, activity | G1 outcome applied to scope; referrals; concierge debriefs | Lead recorded in ≤ 15 s on a budget Android; cross-tenant matrix extended |
| 2 | Oct 26–Nov 1 | M4 follow-ups, today/overdue lists, WA templates with `wa.me` | Show M3/M4 build to 3 interviewees for reaction (no real data) | "Hari ini" and "Terlambat" correct at zone boundaries; one-tap WA |
| 3 | Nov 2–8 | M5 quotations, totals, share link, PDF | Collect 3 real quotation samples (sanitized) to test the PDF layout | Rounding table tests; token tests; PDF opens on Android WhatsApp |
| 4 | Nov 9–15 | M6 invoices, DP/installments, balances, dashboard | Pilot shortlist from commitments (ladder ≥ 3) | Balance in SQL equals Go; idempotency and reversal tests; dashboard numbers reconciled |
| 5 | Nov 16–22 | M7 security review, e2e, backup/restore drill, deployment prep; **staging deploy needs approval and a cost line (D-005)** | Pilot onboarding calls scheduled | Threat-model checklist done; restore drill logged; Playwright critical flows green; smoke tests on staging |
| 6 | Nov 23–29 | Pilot fixes | 3–5 pilots live, paying month one | Pilots can run the full loop without founder help |
| 7 | Nov 30–Dec 6 | Pilot fixes, usage dashboard | Weekly pilot calls, churn reasons | Pilot metrics computed from events |
| 8 | Dec 7–13 | Subscription-readiness (manual billing) | Gate G2 | G2 record: continue / narrow / pivot / stop (`VALIDATION_PLAN.md` §8) |

## Gates

- **G1 (Oct 18):** validation checkpoint; outcomes in `VALIDATION_PLAN.md` §5. Can narrow the segment, revise scope, pivot, or pause; does not gate M1–M2.
- **Toolchain (now):** installing Go 1.27 and running Postgres need the founder's choice (`PROJECT_STATUS.md`, D-022).
- **Staging approval (Week 5):** provisioning any hosted resource requires explicit approval with a stated monthly cost.
- **G2 (Dec 13):** continue / narrow / pivot / stop with pilot usage and payment evidence.

## Traceability: milestone → assumptions, risks, and the test that proves it

| Milestone | Validates assumptions | Mitigates risks | Proof |
|---|---|---|---|
| M1 | A-13 (groundwork) | R-04 (groundwork) | Config tests; `/readyz` DB check; migration test against Postgres; CI workflow |
| M2 | A-13 | R-04 | Cross-tenant matrix for workspace and membership; session, CSRF, Argon2id, login rate-limit tests |
| M3 | A-04, A-10 | R-01, R-05 | Time-to-record measured on a budget Android; cross-tenant matrix for customers and leads |
| M4 | A-08, A-12, A-15 | R-08, R-10 | Zone-boundary tests for "hari ini"; `wa.me` URL encoding tests |
| M5 | A-07, A-14 | R-05, R-09, R-14, R-16 | Money rounding table tests; share-token tests; PDF golden test; JS island stays within one controller |
| M6 | A-11, A-19 | R-09, R-11 | Balance computed in SQL equals Go; idempotency and reversal tests; `FOR UPDATE` concurrency test; dashboard reconciliation test |
| M7 | A-16 | R-04, R-07, R-15 | Threat-model checklist; restore drill log; Playwright critical flows; export/delete verified |
| Pilots | A-02, A-03, A-05 | R-02, R-15 | Pilot metrics from events; G2 record |

## Slip policy

If a milestone slips, cut scope inside it (fewer fields, fewer views) before cutting tests or the cross-tenant matrix. Never move M2 security work later. Validation interviews are not cut to make room for code.
