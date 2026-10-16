# go-api-auth

A todo API with real authentication, written in Go. chi handles routing, GORM handles models, and Postgres stores data in production (sqlite is the local fallback). The test suite runs against Postgres, with a throwaway database per package, so tests exercise the same engine as production.

## What it does

- `GET /healthz` returns `{"status":"ok"}`.
- Auth: register, login, refresh, logout, logout-all, me, change-password, forgot-password, reset-password.
- Todos: create, read, update, delete, complete toggle, plus filters, search, and pagination.
- Per-user token revocation and refresh-token family reuse detection.
- Rate limiting on the auth endpoints, configurable CORS, request IDs, structured request logging, panic recovery, and graceful shutdown. Logs are human-readable text in development and JSON in production.

Every route has an HTTP test built on `httptest`. `go test ./...` is green.

## Layout

App code lives in `cmd/` and `internal/`. Go blocks imports of `internal/` from outside the module, which is what I want.

```
cmd/api/main.go           entrypoint: open db, serve, shut down on SIGINT/SIGTERM
cmd/migrate/main.go       apply or roll back migrations (up/down)
migrations/               golang-migrate SQL, embedded in the binary
internal/server/          the one root router and cross-cutting middleware
internal/config/          typed env config, loaded and validated at boot
internal/logger/          slog logger: JSON in production, text otherwise
internal/database/        open the pool, run migrations
internal/auth/            User, tokens, middleware, register/login/etc
internal/todos/           Todo model and CRUD
internal/mailer/          Mailer interface with console and SMTP backends
internal/ratelimit/       in-memory limiter and middleware
internal/cors/            origin-allowlist CORS middleware
internal/csrf/            double-submit CSRF middleware for cookie auth
internal/httpx/           shared JSON and error response helpers
internal/testutil/        throwaway Postgres database for tests
```

`server.NewRouter(db, cfg, mail)` is the single place the app is wired: middleware, healthz, then `auth.Register` and `todos.Register`. No package mounts itself at `/`, so the two-`Mount("/")` clash that used to panic at boot is gone. `auth.NewRouter` and `todos.NewRouter` still exist as thin wrappers for tests.

Schema changes live in `migrations/` and are applied with `go run ./cmd/migrate up`. The app no longer calls `AutoMigrate`.

## Running it

```bash
cp .env.example .env
go run ./cmd/migrate up   # create the schema
go run ./cmd/api          # listens on :8080, or $PORT
```

Config comes from the environment, with a local `.env` loaded on top when present, and is validated at boot. Bad values fail fast instead of at the first request.

| Variable | Purpose | Default |
| --- | --- | --- |
| `PORT` | listen port | `8080` |
| `APP_ENV` | `development`, `test`, or `production` | `development` |
| `LOG_LEVEL` | `debug`, `info`, `warn`, or `error` | `info` |
| `DATABASE_URL` | Postgres DSN; falls back to a local `app.db` (sqlite) when unset | required in production |
| `TEST_DATABASE_URL` | Postgres DSN for tests | falls back to `DATABASE_URL` |
| `JWT_SECRET` | HS256 signing key, at least 32 bytes | required |
| `JWT_ACCESS_TTL_MIN` | access token lifetime, minutes | `15` |
| `JWT_REFRESH_TTL_DAYS` | refresh token lifetime, days | `7` |
| `JWT_RESET_TTL_MIN` | password-reset token lifetime, minutes | `30` |
| `BCRYPT_COST` | bcrypt work factor | `12` |
| `APP_BASE_URL` | base URL for password-reset links | `http://localhost:8080` |
| `CORS_ORIGINS` | comma-separated allowed browser origins; empty disables CORS | empty |
| `MAIL_FROM`, `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASS` | mailer; console output when `SMTP_HOST` is unset | console |
| `RATE_LIMIT_PER_MIN` | per-IP limit on auth endpoints; `0` disables | `60` |
| `DB_MAX_OPEN_CONNS` | connection pool max open | `25` |
| `DB_MAX_IDLE_CONNS` | connection pool max idle | `5` |

## Auth API

```
POST /api/v1/auth/register         {"name","email","password>=8"} -> 201 {id,name,email}
POST /api/v1/auth/login             {"email","password"} -> 200 {access_token,refresh_token,user}, sets refresh cookie
POST /api/v1/auth/refresh           body or cookie refresh_token -> 200 {access_token,refresh_token}, rotates
POST /api/v1/auth/logout            revokes the refresh token
POST /api/v1/auth/logout-all        revokes all refresh tokens and bumps token version
GET  /api/v1/auth/me                Bearer access token -> 200 {id,name,email,created_at}
PUT  /api/v1/auth/change-password   Bearer + {"current_password","new_password>=8"}
POST /api/v1/auth/forgot-password   {"email"} -> always 200, no user enumeration
POST /api/v1/auth/reset-password    {"token","new_password>=8"}, single use, revokes sessions
```

Access tokens are HS256 JWTs that live 15 minutes. Refresh tokens are opaque, stored as SHA-256 hashes, live 7 days, and rotate on every refresh. Passwords longer than 72 bytes are rejected, since bcrypt only reads that much.

Browser clients can avoid localStorage entirely. Login and refresh set three cookies: `access_token` and `refresh_token` (both `HttpOnly`) plus a readable `csrf_token`. Requests are then authenticated from the access cookie. Because cookies are sent automatically, unsafe requests that use them must also send an `X-CSRF-Token` header equal to the `csrf_token` cookie; a missing or wrong value returns 403. Bearer-token requests are exempt, since a browser never attaches the `Authorization` header on its own. All cookies are `SameSite=Lax` and `Secure` when `APP_ENV=production`.

Revocation works through a per-user `token_version` claim. `change-password`, `reset-password`, and `logout-all` bump it, so tokens issued earlier stop working right away. Reusing an already-rotated refresh token revokes the entire token family.

## Todos API

All routes need `Authorization: Bearer <access>`.

```
POST   /api/v1/todos                    {"title","description","due_date"} -> 201
GET    /api/v1/todos                    ?completed=&search=&page=&limit= -> {data,meta}
GET    /api/v1/todos/{id}
PUT    /api/v1/todos/{id}
DELETE /api/v1/todos/{id}               -> 204
PATCH  /api/v1/todos/{id}/complete
```

Listing defaults to 20 per page and caps at 100. Todos belong to their owner; requesting someone else's row returns 404, not a different user's data.

## Tests

`go test ./...` needs a reachable Postgres. Set `TEST_DATABASE_URL`, or let it fall back to `DATABASE_URL`. Each test package creates its own database, runs the real migrations, truncates between tests, and drops the database when the package finishes, so packages run in parallel safely.

## API spec

`openapi.yaml` holds an OpenAPI 3.0 description of every route, including request bodies, response shapes, and the auth schemes. Point Swagger UI, Postman, or `openapi-generator` at it.

## Known gaps

- The rate limiter keeps counters in memory, so it is per-process. A multi-instance deploy would need a shared store.
- `forgot-password` sends the reset link through the configured mailer and never returns the raw token in the response.
- There is no email-verification flow yet.
