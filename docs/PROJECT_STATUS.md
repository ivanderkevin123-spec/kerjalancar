# Project Status

Updated: 2026-10-12 (night, after Milestone 2)

## Current phase

Build-first MVP (D-019). **Milestone 2 (authentication, workspace, responsive UI) is implemented and tested on branch `m2-auth-workspace`**; the review fix pass, built CSS, and docs are in the working tree awaiting the founder's approval to commit and merge. Next: Milestone 3 (customers, leads, pipeline, activity). Validation track continues in parallel; Gate G1 checkpoint remains Sunday 2026-10-18.

## What works now (try it locally)

- Register at `/daftar` (name, business name, e-mail, password; optional invitation code), land on an honest empty dashboard at `/app`, rename the business at `/app/pengaturan` (htmx fragment), log out at `Keluar`, log in again at `/masuk`.
- Responsive layout: header and side nav on desktop, bottom nav on phones; Indonesian copy; validation, success, empty, 404/403/429/500 states.
- Security: Argon2id (64 MiB, t=3, p=2) with per-hash parameters; server-side sessions in Postgres with token rotation on login and destruction on logout; cookies HttpOnly, SameSite=Lax, Secure in production; CSRF via `net/http.CrossOriginProtection`; rate limits per IP and per e-mail; client IP from headers only with `TRUSTED_PROXY=true`; no credentials or tokens in logs.

```bash
cp .env.example .env            # once
docker compose up -d db
export PATH=$HOME/sdk/go1.27.2/bin:$PATH
make migrate && make run        # http://localhost:8080 (takes ~2 s to start)
make test-db                    # full suite against Postgres
```

## Commits on `m2-auth-workspace` (from `main` 0267e2d)

`81b3792` Argon2id hashing, plan, decisions, htmx · `73e2821` schema 00002, auth service, workspace store · `442dbe5` registration config · `c6f4afc` web layer · `7b0cc11` server wiring, Makefile targets.

Uncommitted (awaiting approval): review fixes (`internal/web/router.go`, `templates.go`, `review_fix_test.go`, `internal/db/migrate_test.go`), built `internal/web/static/app.css`, `.gitignore` (node_modules symlink), `Makefile` (`css-npm`), docs (`DEVELOPMENT`, `MVP_PLAN`, `ARCHITECTURE`, `DECISIONS` D-031/D-032, this file).

## Tests executed (2026-10-12, Go 1.27.2 darwin/arm64, Postgres 17 in Docker)

| Command | Result |
|---|---|
| `make test-db` (`go test -race -count=1 ./...`, `REQUIRE_DB=1`) | 6 packages ok; **58 tests passed, 0 failed, 0 skipped** (M1 regression included) |
| M2 coverage | registration/login/logout flow; wrong password and unknown e-mail give one message; validation copy; duplicate e-mail generic; registration closed → 404; invitation code; cookie attributes (HttpOnly, SameSite=Lax, Path, Secure when configured); session rotation on login and invalidation after rotation and logout; CSRF 403 on `Sec-Fetch-Site: cross-site`; 429 per e-mail (5 login / 10 registration) and per IP (10); unauthenticated `/app` → 303 `/masuk`; cross-workspace rename with another workspace's id leaves it untouched; htmx fragment vs full page; 404 page; static assets cached, no listing, no source; migrations 1+2 on the existing database and on a fresh database (4 concurrent migrators) |
| `go vet ./...`, `gofmt -l` | clean |
| `govulncheck ./...` | GO-2026-5932 in `golang.org/x/crypto` v0.58.0 is module-level only (no reachable symbol), no fix released; re-check at M3 |
| `make lint` | `go vet` only (golangci-lint not installed; CI runs v2.14.0) |
| Live check with curl | `/` → 303; `/masuk` 200; register → 303 `/app`; dashboard shows name, business, empty state, success flash; settings fragment 200 "Nama usaha tersimpan."; cross-origin POST 403; 6th wrong login 429; logout → `/app` 303; `app.css` 15,903 B; 404 page; server log has no password, token, or query string |

## Security review (M2, self-review against the brief's checklist)

| Area | Result |
|---|---|
| Authentication and session | Argon2id PHC with injectable params (production defaults asserted by test); dummy verify on unknown e-mail; scs sessions in Postgres, 30 d lifetime, 7 d idle, rotation on login, destroy on logout |
| Broken object-level authorization, tenant isolation | Workspace resolved from session membership only; `Get`/`Rename` are membership-scoped SQL; form `workspace_id` ignored (test); owner-only rename |
| CSRF / XSS | `CrossOriginProtection` on every state change with `BASE_URL` trusted origin (test); `html/template` escaping; no `template.HTML` |
| SQL injection | Parameterised pgx queries only |
| Mass assignment | Handlers read named fields only; password never echoed |
| Rate limiting | httprate per IP and per e-mail; 429 page (tests) |
| Secrets and logs | No `DATABASE_URL`, password, token, or query string in logs (tests + live check); `.env` ignored |
| Static files | Only `app.css` and `htmx.min.js` embedded; no directory listing (test) |
| Known gaps (deferred) | Session tokens stored unhashed (D-026); no password reset (founder-assisted during pilots); chi Recoverer logs outside slog (M1 minor); two DB queries per authenticated request |

## Known defects and blockers

- Tailwind standalone download from GitHub stalls on this machine; CSS was built via the Node fallback (D-031). Either path produces the committed `app.css`.
- golangci-lint not installed locally (CI runs it). GitHub remote still missing; CI unexecuted.
- Zero customer evidence yet (validation track).

## Decisions awaiting approval

- Commit the review fix pass + CSS + docs (summary above) and merge `m2-auth-workspace` into `main`.
- D-031 (CSS fallback via Node) and D-032 (registration limiter) recorded as routine; say so if you disagree.

## Next three actions

1. Founder approves the commit and merge; Claude merges, re-runs the suite on `main`, closes the plan workspace.
2. Milestone 3 plan: customers, leads, pipeline, activity log (`sqlc` reconsidered as the query count grows).
3. Founder: Day 1 outreach, GitHub repository, golangci-lint decision.
