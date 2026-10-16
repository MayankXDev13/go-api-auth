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
internal/server/         # assembles the one root router
internal/auth/           # User, tokens, middleware, register/login/etc
internal/todos/          # Todo model and CRUD
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
