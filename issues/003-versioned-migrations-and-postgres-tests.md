## Parent PRD

`issues/prd.md`

## What to build

Replace boot-time `AutoMigrate` with versioned SQL migrations that have up and down steps, and provide a migrate command/entrypoint separate from the server. The server must no longer mutate schema at boot. In the same slice, move the test suite from sqlite in-memory to Postgres so tests run against the real engine: a test helper (or compose service) supplies Postgres and applies migrations as part of setup. Existing HTTP tests must keep passing, now on Postgres. See the "Migrations" and "Testing Decisions" sections of the parent PRD.

## Acceptance criteria

- [ ] A `migrations/` directory holds versioned up/down SQL covering `users`, `refresh_tokens`, `password_reset_tokens` and `todos`.
- [ ] A migrate command/entrypoint applies migrations up and supports rolling back down.
- [ ] The server no longer calls `AutoMigrate` at boot.
- [ ] The test helper runs against Postgres and applies migrations; the sqlite in-memory test setup is removed.
- [ ] All existing HTTP tests (register, login, refresh, logout, change-password, forgot/reset, todos CRUD, ownership isolation, complete toggle, healthz, composed router) pass against Postgres.
- [ ] A test verifies migrations up create the schema and down removes it on a throwaway database.

## Blocked by

None - can start immediately.

## User stories addressed

- User story 23
- User story 24
- User story 25
- User story 37
