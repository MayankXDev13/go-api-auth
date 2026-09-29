# go-api-auth, todo plus auth in Go

I am building this as a small todo API with real auth. Go 1.26, chi for routing, GORM for models. Postgres in prod. SQLite in memory for the TDD loop because Docker was down when I started.

## What works now

- [x] `GET /healthz` returns `{"status":"ok"}`
- [x] `POST /api/v1/auth/register` returns 201 on good input, 400 on bad input, 409 on duplicate
- [ ] `POST /api/v1/auth/login` with JWT, then `GET /api/v1/auth/me`
- [ ] forgot and reset password, then todos CRUD

I test through HTTP with httptest. I do not poke at internals. The handlers take a `*gorm.DB`, so the same code runs on SQLite locally and Postgres in prod.

## How it is laid out

Go does not use `src/` for this. I keep app code under `cmd/` and `internal/`. The compiler blocks outside imports of `internal/`, which is what I want.

```
cmd/api/main.go          # entrypoint, still TODO
internal/server/         # base router and /healthz
internal/auth/           # User model and register handler
internal/todos/          # next up
migrations/              # golang-migrate, still TODO
```

## How to run it

```bash
go test ./... -v
go run ./cmd/api        # lands once main.go exists
```

Copy env before you run:

```bash
cp .env.example .env
```

## Auth API so far

```
POST /api/v1/auth/register {"name","email","password>=8"} gives 201 {id,name,email}
  400 means bad input, 409 means that email is taken
```

I plan `/login`, `/refresh`, `/logout`, `/me`, `/forgot-password`, `/reset-password`, and `/todos` next. Login is up first.
