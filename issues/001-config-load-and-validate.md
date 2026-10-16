## Parent PRD

`issues/prd.md`

## What to build

Give the service a single configuration source. A typed `Config` is loaded once at startup: read `.env` when present (non-fatal if absent), then let real environment variables override it. Validate required values before the server starts and refuse to boot on failure. Move the hardcoded auth constants (access/refresh TTL, bcrypt cost) and the port/database settings onto this config and pass them into handlers. There must be no fallback signing secret: a missing or too-short `JWT_SECRET` must stop boot. See the "Config module" and "Further Notes" sections of the parent PRD.

## Acceptance criteria

- [ ] A single typed `Config` exposes port, database URL, JWT secret, access TTL, refresh TTL, bcrypt cost, rate limit, CORS origins and SMTP settings.
- [ ] `.env` is loaded when present and never overrides real environment variables.
- [ ] A missing or shorter-than-32-byte `JWT_SECRET` causes a clear startup failure; the hardcoded fallback secret is removed.
- [ ] Invalid values (non-positive TTL, out-of-range bcrypt cost, non-numeric/out-of-range port, missing required production values) cause a clear startup failure.
- [ ] Access/refresh TTLs and bcrypt cost are driven by config, not constants; `BCRYPT_COST` is honored (lower in tests, higher in production).
- [ ] The server listens on the configured port and uses the configured database URL.
- [ ] `go test ./...` stays green.

## Blocked by

None - can start immediately.

## User stories addressed

- User story 1
- User story 2
- User story 3
- User story 4
- User story 5
- User story 40
