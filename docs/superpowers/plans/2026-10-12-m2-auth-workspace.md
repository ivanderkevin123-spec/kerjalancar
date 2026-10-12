# Milestone 2 — Authentication, Workspace, Responsive UI (short plan)

**Goal:** An owner registers with e-mail and password, gets a workspace, logs in and out, sees an honest empty dashboard and a settings page in Bahasa Indonesia on a phone or desktop. Nobody can read or change another workspace.

**Spec:** `docs/MVP_PLAN.md` (M2), `docs/ARCHITECTURE.md` §3–§5, §8. Decisions D-024..D-030 in `docs/DECISIONS.md`.

## Scope decisions (recorded in DECISIONS.md)

- Identity = e-mail (citext, unique) + password. Registration page exists; `REGISTRATION_OPEN` (default true in development/test, false in production) and optional `REGISTRATION_CODE`. Registration creates user + workspace + owner membership in one transaction.
- No invitations, no staff UI in M2 (schema keeps `memberships.role` owner/staff). Deferred until a pilot needs a second user.
- Sessions: `alexedwards/scs` v2.9.0 with `postgresstore` (table `sessions`), token renewed on login, destroyed on logout; cookie HttpOnly, SameSite=Lax, Secure in production, lifetime 30 d, idle timeout 7 d.
- Passwords: Argon2id via `golang.org/x/crypto` v0.58.0 (m=64 MiB, t=3, p=2, 16-byte salt, 32-byte key, PHC string); dummy verify when the e-mail is unknown.
- CSRF: `net/http.NewCrossOriginProtection` (Go 1.25+) with `BASE_URL` as trusted origin.
- Rate limits: `go-chi/httprate` v0.16.1, in-process: 10/min per IP and 5/min per e-mail on login and registration; 429 with an Indonesian page.
- No sqlc in M2 (hand-written pgx queries; revisit in M3). Tailwind v4 standalone binary lives in `.toolchain/bin` (git-ignored); built `app.css` and vendored `htmx.min.js` 2.0.11 are committed.

## Files

```
internal/db/migrations/00002_users_workspaces_sessions.sql
internal/auth/password.go, password_test.go          Argon2id hash/verify (PHC)
internal/auth/service.go, service_test.go            Register (tx), Authenticate, errors
internal/workspace/store.go, store_test.go           ForUser, Get(ws,user), Rename(ws,user,name); ErrNotFound on non-member
internal/config/config.go (+test)                    REGISTRATION_OPEN, REGISTRATION_CODE
internal/web/templates/{layouts,pages,partials}/*.html   base/auth/app layouts; masuk, daftar, beranda, pengaturan, 429, 404/500
internal/web/static/{htmx.min.js, app.css, src/app.css}
internal/web/templates.go, session.go, authz.go, auth_handlers.go, app_handlers.go, router.go (+tests)
cmd/server/main.go                                   wiring
Makefile (css target), docs/DEVELOPMENT.md, docs/DECISIONS.md, docs/PROJECT_STATUS.md, docs/MVP_PLAN.md
```

## Routes

`GET /` → `/app` or `/masuk` · `GET|POST /masuk` · `GET|POST /daftar` (if open) · `POST /keluar` · `GET /app` (beranda, empty state) · `GET|POST /app/pengaturan` (rename current workspace; `workspace_id` in forms is ignored) · `GET /static/*`.

Forms use `hx-boost` (full-page HTML on failure with field errors, 303 on success). The settings form uses `hx-post` and returns a fragment when `HX-Request: true`.

## Tasks (TDD each, commit each)

1. `auth.HashPassword` / `auth.VerifyPassword`: PHC round-trip, wrong password, tampered hash, params parsed from the string.
2. Migration 00002 + `auth.Service.Register/Authenticate` + `workspace.Store`: duplicate e-mail, atomic registration (workspace failure rolls back user), unknown e-mail timing path, cross-workspace Get/Rename → ErrNotFound. Migration test asserts versions 1 and 2 applied on the existing DB; fresh-DB race test already covers a new database.
3. Config: `RegistrationOpen` default by env, `RegistrationCode`.
4. Web: sessions (renew on login, destroy on logout), CSRF, rate limits, templates, handlers, authz middleware. Tests: register/login/logout happy path; wrong password and invalid input re-render with Indonesian errors and no enumeration; cookie attributes (HttpOnly, SameSite=Lax, Secure when configured); session rotation and invalidation; CSRF 403 on `Sec-Fetch-Site: cross-site`; 429 after limits; unauthenticated `/app` → 303 `/masuk`; cross-workspace rename attempt leaves the other workspace unchanged; HX-Request fragment vs full page; 404 page.
5. Wiring, assets (`make css`), manual check with curl, docs/decisions/status.
6. Review (self + code-review skill) and fix pass; full regression (`make test-db`), gofmt, vet, govulncheck.

## Verification

`GO=~/sdk/go1.27.2/bin/go make migrate && make run`, then register at `/daftar`, log in at `/masuk`, open `/app` on a 360 px viewport; `make test-db` green including M1 tests. Commands needing Docker, Postgres, network, or port binding run with the sandbox disabled (ledger rulings from M1).
