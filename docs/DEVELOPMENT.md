# Development setup

Prerequisites: Go 1.27.x on PATH, Docker Desktop running (for Postgres), GNU make.

1. `cp .env.example .env`
2. `make db-up` — starts Postgres 17 on 127.0.0.1:5432 and creates `kerjalancar` and `kerjalancar_test`
3. `make migrate` — applies migrations to `kerjalancar`
4. `make run` — serves http://localhost:8080 ; check `curl -i localhost:8080/healthz` and `/readyz`
5. `make test` — unit tests plus database tests (the test database is migrated automatically by the test harness)
6. `make lint` — `go vet` plus `golangci-lint` v2.14.0 (the CI version; uses the installed binary if present, otherwise `go run` builds it into the module cache on first use, a few minutes)
7. Open http://localhost:8080 → `/masuk`. Register at `/daftar` (open outside production). The server needs about two seconds to start: it hashes a dummy password at production Argon2id cost before listening. Then: `/app/prospek` (pipeline), `/app/prospek/baru` (record a lead, creating the customer inline), `/app/pelanggan` (customers), `/app/follow-up` (today's and overdue follow-ups), `/app/pengaturan/templat` (WhatsApp templates), `/app/penawaran` (quotations; create one from a lead page, send it, open its `/p/{token}` link in a private window to see the customer view, download the PDF). `/app/tagihan` (invoices; from a won lead's page choose "Buat tagihan dari penawaran #000N" or a blank one, Terbitkan, record a DP with "Catat pembayaran", open the `/t/{token}` link in a private window, download the PDF, reverse a payment with a reason). The dashboard's "Piutang" card and the follow-up page's Terlambat tab show what is owed. `PDF_SAMPLE_DIR=/some/dir go test -run TestWriteSamplePDF ./internal/quotation` writes a sample PDF for eyeballing. The WhatsApp button opens `wa.me` in a new tab; on a laptop without WhatsApp Web it shows WhatsApp's own landing page.

CSS: `internal/web/static/app.css` is a committed build artifact. After changing templates or `static/src/app.css`, rebuild it with `make css` (needs the Tailwind v4 standalone CLI: `make css-tool` downloads it into `.toolchain/bin`; on Linux set `TAILWIND_PLATFORM=linux-x64`). Fallback when that download is slow: `make css-npm` uses Node 22 via a git-ignored `.toolchain/tw` install (Node 22 must be on PATH, e.g. `nvm use 22`; the system Node 10 cannot run Tailwind v4). Commit the rebuilt `app.css` with the template change. htmx is vendored at `internal/web/static/htmx.min.js` (2.0.11).

Environment variables: see `.env.example`. `APP_ENV` is `development`, `test`, or `production`; `BASE_URL` must be https in production. `TRUSTED_PROXY=true` only when a proxy in front of the app overwrites `X-Forwarded-For`; otherwise client IPs in logs and rate limits could be forged. `REGISTRATION_OPEN` defaults to true outside production and false in production; `REGISTRATION_CODE`, when set, is required on the registration form (share it with pilots). Session cookies are `Secure` only when `APP_ENV=production`, so production must run behind HTTPS.

Using a Go toolchain that is not on PATH (for example `~/sdk/go1.27.2`): `GO=~/sdk/go1.27.2/bin/go make test`.

Troubleshooting:
- `make test` skips database tests: `TEST_DATABASE_URL` is not set; copy `.env.example` to `.env`.
- `connection refused` on 5432: `make db-up`, then `docker compose ps` until the `db` service is healthy.
- Database tests (`internal/auth`, `internal/workspace`, `internal/web`) need `TEST_DATABASE_URL`; they create rows with unique e-mails and never clean the test database, which is fine for a throwaway database.
- Tests that bind local ports or connect to Postgres fail with "operation not permitted" inside the Claude Code sandbox: run `make test` from your own terminal, or set `sandbox.network.allowLocalBinding: true`. Inside the sandbox, `go vet`/`go build` work offline with `GOCACHE=$PWD/.toolchain/gocache GOMODCACHE=$PWD/.toolchain/modcache GOPROXY=off` (the module cache lives there).
- `make test` runs packages in parallel against one test database; migrations serialise through goose's Postgres session lock (D-041), so a new migration is applied exactly once.
