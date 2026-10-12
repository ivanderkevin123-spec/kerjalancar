# Development setup

Prerequisites: Go 1.27.x on PATH, Docker Desktop running (for Postgres), GNU make.

1. `cp .env.example .env`
2. `make db-up` — starts Postgres 17 on 127.0.0.1:5432 and creates `kerjalancar` and `kerjalancar_test`
3. `make migrate` — applies migrations to `kerjalancar`
4. `make run` — serves http://localhost:8080 ; check `curl -i localhost:8080/healthz` and `/readyz`
5. `make test` — unit tests plus database tests (the test database is migrated automatically by the test harness)
6. `make lint` — `go vet` and, if installed, `golangci-lint`

Environment variables: see `.env.example`. `APP_ENV` is `development`, `test`, or `production`; `BASE_URL` must be https in production. `TRUSTED_PROXY=true` only when a proxy in front of the app overwrites `X-Forwarded-For`; otherwise client IPs in logs and rate limits could be forged.

Using a Go toolchain that is not on PATH (for example `~/sdk/go1.27.2`): `GO=~/sdk/go1.27.2/bin/go make test`.

Troubleshooting:
- `make test` skips database tests: `TEST_DATABASE_URL` is not set; copy `.env.example` to `.env`.
- `connection refused` on 5432: `make db-up`, then `docker compose ps` until the `db` service is healthy.
- Tests that bind local ports fail with "operation not permitted" inside the Claude Code sandbox: run `make test` from your own terminal, or set `sandbox.network.allowLocalBinding: true`.
