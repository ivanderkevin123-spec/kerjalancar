# Project Status

Updated: 2026-10-12 (night)

## Current phase

Build-first MVP (D-019). **Milestone 1 (bootstrap) is complete** on branch `m1-bootstrap`, pending the final whole-branch review and merge to `main`. Next: Milestone 2 plan (auth, workspace, layout). Validation track runs in parallel (`DAY1_EXECUTION.md`; Gate G1 checkpoint Sunday 2026-10-18).

## Completed work

- Discovery package and MVP plan: commits `f55494f`, `3f03a49`, `1be3dde` on `main`
- M1 on `m1-bootstrap` (7 tasks, one commit each): module + local Postgres + Makefile + lint config + dev docs; `internal/config`; `internal/web` (chi router, request logger without query strings, `/healthz`, `/readyz`, graceful `Serve`); `internal/db` (pgx pool, embedded goose migrations, `testdb` harness with rollback per test); `cmd/server`, `cmd/migrate`; CI workflow
- Environment: Go 1.27.2 installed at `~/sdk/go1.27.2` (founder-approved tarball, PATH untouched); Postgres 17 running in Docker via `docker-compose.yml` with `kerjalancar` and `kerjalancar_test` databases

## Tests executed (2026-10-12, Go 1.27.2 darwin/arm64, Postgres 17 in Docker)

| Command | Result |
|---|---|
| `GO=~/sdk/go1.27.2/bin/go make test-db` (`go test -race -count=1 ./...` with `REQUIRE_DB=1`) | `ok internal/config`, `ok internal/db`, `ok internal/web`; 19 tests passed, 0 failed, 0 skipped |
| `go vet ./...` | clean |
| `gofmt -l .` | clean |
| `go run golang.org/x/vuln/cmd/govulncheck@latest ./...` | No vulnerabilities found |
| `make lint` | `go vet` only: golangci-lint not installed locally |
| Manual: `make migrate` twice, `make migrate-status`, server start, `curl /healthz` `/readyz`, `docker compose stop db` → `/readyz` 503 in 0.045 s, restart → 200, `kill -TERM` → exit 0, `DATABASE_URL=` and `PORT=abc` → exit 1 | all as expected |

Ledger with per-task evidence: `.superpowers/sdd/2026-10-12-m1-bootstrap/progress.md` (git-ignored, local).

## How to run locally

```bash
cp .env.example .env
docker compose up -d db
GO=~/sdk/go1.27.2/bin/go make migrate
GO=~/sdk/go1.27.2/bin/go make run      # then: curl -i localhost:8080/healthz ; curl localhost:8080/readyz
GO=~/sdk/go1.27.2/bin/go make test-db
```

Add `export PATH=$HOME/sdk/go1.27.2/bin:$PATH` to your shell for this project to drop the `GO=` prefix.

## Known defects and blockers

- golangci-lint is not installed locally; CI will run it (v2.14.0). Install needs the founder's go-ahead (`brew install golangci-lint` or the project-local binary).
- No GitHub remote yet; the CI workflow is authored but unexecuted. Founder creates a private repository and pushes `main` (external action).
- Claude Code sandbox limits: Go module downloads, Docker, Postgres connections, and port-binding tests run with the sandbox disabled per ledger rulings; everything else sandboxed.
- Zero customer evidence yet; validation track in parallel. Secondary-source competitor pricing still unverified (`COMPETITOR_RESEARCH.md` §4).

## Decisions awaiting approval

- Merge `m1-bootstrap` into `main` after the final review (routine; will proceed unless the founder objects)
- GitHub repository creation and first push (external; founder)
- golangci-lint local install (founder)

## Next three actions

1. Final whole-branch review of M1, fix pass if needed, merge to `main`
2. Write the detailed Milestone 2 plan (auth with Argon2id, sessions, CSRF, workspaces, memberships, invitations, responsive layout with Tailwind and htmx 2.0.11, `sqlc`) and execute it
3. Founder: Day 1 outreach (`DAY1_EXECUTION.md`), create the private GitHub repository, decide on golangci-lint install
