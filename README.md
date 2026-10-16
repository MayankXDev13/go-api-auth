# go-api-auth, todo plus auth in Go

I am building this as a small todo API with real auth. Go 1.26, chi for routing, GORM for models. Postgres in prod. SQLite in memory for the TDD loop because Docker was down when I started.

## What works now

- [x] `GET /healthz` returns `{"status":"ok"}`
- [x] register, login, refresh, logout, me, change-password
- [x] forgot and reset password
- [x] todos CRUD, plus complete toggle, filters, search, pagination

Every route above is covered by an HTTP test with `httptest`. `go test ./...` is green.

## How it is laid out

Go does not use `src/` for this. I keep app code under `cmd/` and `internal/`. The compiler blocks outside imports of `internal/`, which is what I want.

```
cmd/api/main.go          # entrypoint: open db, migrate, serve
internal/server/         # assembles the one root router
internal/auth/           # User, tokens, middleware, register/login/etc
internal/todos/          # Todo model and CRUD
```

`server.NewRouter(db)` is the single place the app is wired: healthz plus `auth.Register` and `todos.Register`. Neither package mounts itself at `/`, so the old two-`Mount("/")` clash that panicked at boot is gone. `auth.NewRouter`/`todos.NewRouter` still exist as thin wrappers for tests.

Migrations are still `AutoMigrate` in `main.go`. A `migrations/` dir with golang-migrate is still TODO.

## How to run it

```bash
go test ./... -v
go run ./cmd/api        # listens on :8080, or $PORT
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

- `forgot-password` builds a reset token but does not send mail yet; the raw token is not surfaced.
- `.env` is not loaded by the app, and `JWT_ACCESS_TTL_MIN`, `BCRYPT_COST`, `RATE_LIMIT_PER_MIN` are not wired (TTLs are constants in `token.go`).
- No rate limiting, CORS, or graceful shutdown.
