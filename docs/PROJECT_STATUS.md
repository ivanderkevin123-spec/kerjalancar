# Project Status

Updated: 2026-10-12 (evening)

## Current phase

Build-first MVP (D-019). Discovery package finalized and committed (`f55494f`, `3f03a49`). Founder approved D-001..D-017 and D-019, D-020, D-023. Milestone plan written (`MVP_PLAN.md`); detailed Milestone 1 plan written (`superpowers/plans/2026-10-12-m1-bootstrap.md`). **M1 execution is blocked on the environment** (below). Validation track continues in parallel (`DAY1_EXECUTION.md`, Gate G1 on 2026-10-18).

## Completed work

- Phase A discovery package (10 docs) and audit pass: committed
- Build-first alignment: D-019..D-023 recorded; `VALIDATION_PLAN.md`, `PRODUCT_BRIEF.md`, `ROADMAP.md` updated so G1 is a checkpoint, not a gate on the first commit
- `MVP_PLAN.md`: M1–M7 with outcomes, scope, acceptance criteria, tests, deferred items
- M1 bootstrap plan: 7 tasks with code, tests, run commands, commit messages; library versions pinned after checking proxy.golang.org, go.dev, and GitHub releases on 2026-10-12 (Go 1.27.2, chi 5.3.2, pgx 5.11.0, goose 3.29.0, testify 1.12.1, htmx 2.0.11, Tailwind v4 standalone, golangci-lint 2.14.0, postgres:17-alpine)
- Confirmed `net/http.NewCrossOriginProtection` exists since Go 1.25 (pkg.go.dev)

## Environment facts (re-verified 2026-10-12 evening)

| Item | State | Needed by |
|---|---|---|
| Go | 1.17.5 at `/usr/local/go` (amd64 under Rosetta on an M1). Latest stable 1.27.2 | M1 Task 1 |
| PostgreSQL | Not installed; no daemon | M1 Task 1 |
| Docker | CLI 20.10.11 present; daemon not running (Docker Desktop not started) | M1 Task 1 if Postgres runs in Docker |
| Homebrew | Intel build at `/usr/local/bin/brew` (would install amd64 Go) | — |
| Node | nvm has v22 | M7 (Playwright) |
| GitHub CLI / remote | None; repo has no remote | M1 Task 6 (first CI run) |
| Claude Code sandbox | Blocks local port binding; tests that listen on a port must run from the founder's terminal or with `sandbox.network.allowLocalBinding: true` | M1 Task 5 test |

## Blocker: toolchain and database. Founder's choice needed (D-022)

Nothing below has been run. Pick one option per blocker, or run the commands yourself.

**Go 1.27.2**

- Option A (recommended): official native arm64 tarball into your home directory, no system change, removable with `rm -rf ~/sdk/go1.27.2`:

  ```bash
  mkdir -p ~/sdk && curl -fL https://go.dev/dl/go1.27.2.darwin-arm64.tar.gz | tar -xz -C ~/sdk && mv ~/sdk/go ~/sdk/go1.27.2 && ~/sdk/go1.27.2/bin/go version
  ```

  The project then uses it via `GO=~/sdk/go1.27.2/bin/go make …` or a line in your shell for this project only: `export PATH=$HOME/sdk/go1.27.2/bin:$PATH`. Your other projects keep 1.17.5.
- Option B: `brew install go` (Intel Homebrew → amd64 binary under Rosetta; may shadow 1.17.5 depending on PATH order).
- Option C: no host Go at all; build and test inside `golang:1.27` containers (needs Docker running; slower).

**PostgreSQL 17**

- Option A (recommended): start Docker Desktop yourself (sandbox cannot launch apps), then `make db-up` uses `docker-compose.yml` from M1 Task 1. No install.
- Option B: `brew install postgresql@17 && brew services start postgresql@17` (system service).
- Option C: Postgres.app.

If you choose A + A, I need either (1) you to run the Go tarball command and start Docker Desktop, or (2) your approval for me to run the tarball command outside the sandbox (it writes to `~/sdk`, outside my allowed paths). Starting Docker Desktop is yours in either case.

## Tests executed

None. No product code exists yet; M1 Task 1 has not started. No test results are claimed.

## Known defects and blockers

- Toolchain and database: see above
- Zero customer evidence yet; validation track runs in parallel
- Secondary-source competitor pricing still needs manual verification (`COMPETITOR_RESEARCH.md` §4)
- CI workflow can only be verified after a GitHub repository exists (founder creates it; private recommended)

## Decisions awaiting approval

- D-022: toolchain install method and Postgres runtime (choice above)
- Approval to commit this planning pass (files listed in the session summary)

## Next three actions

1. Founder picks the toolchain and database options (or runs the two commands) and approves the planning commit
2. Claude executes M1 Tasks 1–7 from the plan, reporting tests as they run; port-binding tests run from the founder's terminal if the sandbox blocks them
3. Founder runs Day 1 of the validation track (`DAY1_EXECUTION.md`); Claude then writes the M2 detailed plan (auth, workspace, layout)
