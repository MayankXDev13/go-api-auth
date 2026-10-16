# go-api-auth, todo plus auth in Go

I am building this as a small todo API with real auth. Go 1.26, chi for routing, GORM for models. Postgres in prod and in the test suite (each test package gets a throwaway database).

## What works now

- [x] `GET /healthz` returns `{"status":"ok"}`
- [x] register, login, refresh, logout, me, change-password
- [x] forgot and reset password
- [x] todos CRUD, plus complete toggle, filters, search, pagination
- [x] per-user token revocation, refresh-token family reuse detection
- [x] rate limiting, CORS, request logging/recovery, graceful shutdown

Every route above is covered by an HTTP test with `httptest`. `go test ./...` is green.

## How it is laid out

Go does not use `src/` for this. I keep app code under `cmd/` and `internal/`. The compiler blocks outside imports of `internal/`, which is what I want.

```
cmd/api/main.go          # entrypoint: open db, serve, graceful shutdown
cmd/migrate/main.go      # applies versioned migrations (up/down)
migrations/              # golang-migrate SQL files, embedded in the binary
internal/server/         # assembles the one root router + middleware
internal/config/         # typed env config, loaded and validated at boot
internal/database/       # open pool, run migrations
internal/auth/           # User, tokens, middleware, register/login/etc
internal/todos/          # Todo model and CRUD
internal/mailer/         # Mailer interface: console (dev) and SMTP
internal/ratelimit/      # in-memory limiter + middleware
internal/cors/           # origin-allowlist CORS middleware
internal/httpx/          # shared JSON / error response helpers
internal/testutil/       # per-package throwaway Postgres for tests
```

`server.NewRouter(db, cfg, mail)` is the single place the app is wired: middleware, healthz, plus `auth.Register` and `todos.Register`. Neither package mounts itself at `/`, so the old two-`Mount("/")` clash that panicked at boot is gone. `auth.NewRouter`/`todos.NewRouter` still exist as thin wrappers for tests.

Schema changes live in `migrations/` and are applied with `go run ./cmd/migrate up`. The app no longer calls `AutoMigrate`.

## How to run it

```bash
go test ./... -v
go run ./cmd/migrate up  # apply schema
go run ./cmd/api         # listens on :8080, or $PORT
```

Copy env before you run:

```bash
cp .env.example .env
```

Tests need a reachable Postgres. They use `TEST_DATABASE_URL` (falling back to `DATABASE_URL`), create a throwaway database per test package, migrate it, and drop it afterwards.

## Configuration

All config is read from the environment (with a local `.env` loaded on top when present) and validated at boot.

```
PORT                     listen port (default 8080)
APP_ENV                  development | test | production (Secure cookies in production)
DATABASE_URL             Postgres DSN
TEST_DATABASE_URL        Postgres DSN for tests (defaults to DATABASE_URL)
JWT_SECRET               >=32 bytes, required
JWT_ACCESS_TTL_MIN       access token TTL (default 15)
JWT_REFRESH_TTL_DAYS     refresh token TTL (default 7)
JWT_RESET_TTL_MIN        password-reset token TTL (default 30)
BCRYPT_COST              bcrypt cost (default 12)
APP_BASE_URL             base URL used to build password-reset links
CORS_ORIGINS             comma-separated allowed browser origins (empty disables CORS)
MAIL_FROM, SMTP_HOST, SMTP_PORT, SMTP_USER, SMTP_PASS   mailer (console when SMTP_HOST unset)
RATE_LIMIT_PER_MIN       per-IP limit on auth endpoints (0 disables)
DB_MAX_OPEN_CONNS        connection pool max open (default 25)
DB_MAX_IDLE_CONNS        connection pool max idle (default 5)
```

## Auth API

```
POST /api/v1/auth/register         {"name","email","password>=8"} -> 201 {id,name,email}
POST /api/v1/auth/login            {"email","password"} -> 200 {access_token,refresh_token,user}, sets refresh cookie
POST /api/v1/auth/refresh          body or cookie refresh_token -> 200 {access_token,refresh_token}, rotates
POST /api/v1/auth/logout           revokes the refresh token
POST /api/v1/auth/logout-all       revokes all refresh tokens + bumps token version
GET  /api/v1/auth/me               Bearer access token -> 200 {id,name,email,created_at}
PUT  /api/v1/auth/change-password  Bearer + {"current_password","new_password>=8"}
POST /api/v1/auth/forgot-password  {"email"} -> always 200 (no user enumeration)
POST /api/v1/auth/reset-password   {"token","new_password>=8"}, single use, revokes sessions
```

Access tokens are HS256 JWTs (15 min). Refresh tokens are opaque, stored hashed (SHA-256), 7 days, and rotate on every refresh.

Revocation: each user has a `token_version`; `change-password`, `reset-password` and `logout-all` bump it, so outstanding access tokens stop working immediately. Reusing an already-rotated refresh token revokes the whole token family.

## Todos API (all require `Authorization: Bearer <access>`)

```
POST   /api/v1/todos                    {"title","description","due_date"} -> 201
GET    /api/v1/todos                    ?completed=&search=&page=&limit= -> {data,meta}
GET    /api/v1/todos/{id}
PUT    /api/v1/todos/{id}
DELETE /api/v1/todos/{id}               -> 204
PATCH  /api/v1/todos/{id}/complete
```

Todos are scoped to the owner; another user gets 404, never someone else's row.

## Known gaps

- `forgot-password` sends the reset link through the configured mailer (`Console` in dev, SMTP when `SMTP_HOST` is set); the raw token is never returned in the response.
- The in-memory rate limiter is per-process; a multi-instance deploy needs a shared store.
