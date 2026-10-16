# PRD: Harden and productionize go-api-auth

## Problem Statement

The todo + auth API works end to end and is covered by HTTP tests, but it is not safe or operable outside a laptop. Known problems:

- The app boots with a hardcoded JWT fallback secret, so if `JWT_SECRET` is ever unset in production anyone can forge access tokens.
- `.env` is not loaded and several `.env.example` variables (`JWT_ACCESS_TTL_MIN`, `JWT_REFRESH_TTL_DAYS`, `BCRYPT_COST`, `RATE_LIMIT_PER_MIN`) are ignored; TTLs and cost are hardcoded constants.
- `forgot-password` mints a reset token but never sends it anywhere, so the reset flow cannot be completed by a real user.
- Access tokens cannot be revoked; logout, change-password and reset only kill refresh tokens, leaving a stolen access token valid for up to 15 minutes.
- Refresh tokens rotate but reuse is not detected, so a stolen-and-replayed token is not treated as a compromise.
- There is no rate limiting on auth endpoints, no CORS, no cookie `Secure` flag, and no bcrypt 72-byte input guard.
- Migrations are `AutoMigrate` only, so schema cannot be versioned or rolled back.
- There is no graceful shutdown, no request logging/recovery/timeouts, no DB pool tuning, and no way to deploy (no Dockerfile/compose, no CI).

## Solution

Turn the working prototype into a deployable service with a single, validated configuration source; a real mail path for password reset; revocable access tokens; refresh-token family detection; rate limiting; versioned migrations; and the operational scaffolding (middleware, graceful shutdown, Docker, CI).

The app keeps the same public API surface, with a few deliberate additions: a `logout-all` endpoint, a `token_version` claim so password changes invalidate outstanding access tokens, and refresh-token family tracking. Configuration is read once at startup into a typed struct with fail-fast validation, and a `Mailer` interface decouples mail delivery from handlers so the reset flow is testable without an SMTP server.

## User Stories

1. As an operator, I want the app to refuse to start when `JWT_SECRET` is missing or too short, so that I never run production with a guessable signing key.
2. As an operator, I want the app to load a `.env` file in development and real environment variables in production, using the same code path, so that local setup is one command.
3. As an operator, I want all tuning knobs (access/refresh TTL, bcrypt cost, rate limit, port, database URL, SMTP settings) read from configuration, so that behavior can change without a rebuild.
4. As an operator, I want invalid or missing required configuration to fail fast at boot with a clear message, so that misconfiguration is caught before serving traffic.
5. As a developer, I want `BCRYPT_COST` to be honored, so that I can lower the cost in tests and raise it in production.
6. As a user, I want to receive a password-reset email containing a working link when I request one, so that I can recover my account.
7. As a user, I want the forgot-password response to stay generic whether or not my email exists, so that my account existence is not leaked.
8. As a developer, I want a `Mailer` interface with an SMTP implementation and a console/log implementation, so that tests and local dev do not require an SMTP server.
9. As a developer, I want the dev mailer to log the raw reset link, so that I can complete the reset flow locally.
10. As a user, I want my reset token to be single-use and to expire, so that a leaked link has limited value.
11. As a user, I want all my sessions invalidated after I reset my password, so that an attacker with an old token is locked out.
12. As a user, I want changing my password to invalidate my existing access tokens, so that a leaked token stops working immediately.
13. As a user, I want a way to log out of all devices at once, so that I can respond to a suspected compromise.
14. As a user, I want ordinary logout to revoke the refresh token presented, so that the session cannot be silently renewed.
15. As an operator, I want refresh tokens bound to a family, so that replaying a rotated token revokes the entire family and forces re-authentication.
16. As a user, I want a reused/compromised refresh token to be rejected, so that stolen tokens are not silently accepted.
17. As a user, I want my refresh cookie to be `HttpOnly`, `Secure` (in production) and `SameSite`, so that it is not readable by scripts or sent in cleartext.
18. As an operator, I want bcrypt inputs longer than 72 bytes to be rejected or safely handled, so that passwords are not silently truncated.
19. As an operator, I want auth endpoints rate limited, so that credential stuffing and token guessing are throttled.
20. As an operator, I want rate-limit responses to return `429` with sane headers, so that clients can back off.
21. As an operator, I want CORS configurable, so that only approved origins can call the API from a browser.
22. As a developer, I want the rate limiter behind an interface with an in-memory implementation, so that a distributed store can replace it later.
23. As an operator, I want schema migrations as versioned SQL with up/down, so that I can deploy and roll back deterministically.
24. As an operator, I want the app to stop using `AutoMigrate` at boot, so that production schema changes are explicit and reviewed.
25. As an operator, I want a migrate command/entrypoint separate from the server, so that migrations run before or independently of serving.
26. As an operator, I want the server to shut down gracefully on SIGINT/SIGTERM, so that in-flight requests finish and the port is released.
27. As an operator, I want the server to log each request with method, path, status and latency, so that I can observe traffic.
28. As an operator, I want panics recovered per request and logged, so that one bad request cannot crash the process.
29. As an operator, I want a request ID on every request and in the response, so that logs can be correlated.
30. As an operator, I want read/write/idle timeouts on the HTTP server, so that slow clients cannot exhaust resources.
31. As an operator, I want configurable DB connection pool limits, so that the service does not exhaust database connections.
32. As a developer, I want shared JSON/error response helpers, so that auth and todos do not duplicate response code.
33. As a developer, I want database errors surfaced as `500` responses instead of being silently ignored, so that failures are visible and debuggable.
34. As a developer, I want a multi-stage Dockerfile producing a small image, so that I can run the service in a container.
35. As a developer, I want a `docker-compose.yml` with the app and Postgres, so that I can run the whole stack locally.
36. As a developer, I want CI running build, vet, and tests on each push and PR, so that regressions are caught before merge.
37. As a developer, I want the test suite to run against Postgres, so that tests exercise the same database engine as production.
38. As a developer, I want the existing HTTP tests to keep passing after the refactor, so that behavior is preserved.
39. As a developer, I want new tests for config validation, mailer, rate limiting, token revocation and refresh family behavior, so that the hardening is verified.
40. As an operator, I want secrets and `.env` kept out of the image and repository, so that credentials are not leaked.

## Implementation Decisions

- **Config module (`internal/config`)**: single typed `Config` struct populated once at startup. Loads a `.env` file via `godotenv` when present (non-fatal if absent), then reads real environment variables (which win). Exposes `Load() (Config, error)` that validates required fields (`JWT_SECRET` length, `DATABASE_URL` for non-dev/test, port range, positive TTLs, bcrypt cost range). No configuration is read at request time.
- **Mailer module (`internal/mailer`)**: `Mailer` interface with a `Send(ctx, to, subject, body)`-style method (exact signature decided in the issue). Two implementations: SMTP (using the SMTP values in config) and a console/log mailer used in development and tests. Injected into the auth handler. The forgot-password handler builds the reset link from a configurable base URL and calls the mailer; it still returns a generic 200.
- **Token revocation**: `User` gains a `token_version` integer column, embedded as a claim in access JWTs. `RequireAuth` rejects a token whose version is below the user's current version. Password change, password reset, and `logout-all` bump `token_version`. Ordinary logout revokes the presented refresh token only. A new `POST /api/v1/auth/logout-all` returns the user to a logged-out-everywhere state. Adding the claim/column makes existing access tokens invalid on deploy (acceptable; noted in Further Notes).
- **Refresh-token families**: `RefreshToken` gains a family identifier. Login starts a family; each rotation keeps the same family and marks the old row rotated/revoked. Presenting an already-rotated token revokes the whole family. Reuse is rejected with 401.
- **Rate limiting**: an in-memory token-bucket limiter behind an interface, keyed by client IP (and email where relevant) applied to auth endpoints. Returns 429 with `Retry-After`. Single-instance only; the interface allows a future distributed implementation.
- **Middleware & server lifecycle**: the root router gains request-ID, structured request logging, panic recovery, and CORS middleware. `cmd/api` builds an `http.Server` with read/write/idle timeouts and performs graceful shutdown on SIGINT/SIGTERM. CORS allowed origins come from config.
- **Migrations**: replace boot-time `AutoMigrate` with versioned SQL migration files and a `migrate` command/entrypoint that embeds and runs them (golang-migrate, embed or file source decision in the issue). The server no longer mutates schema at boot.
- **Database module**: opening the DB, applying pool limits from config, and the Postgres/SQLite driver selection are consolidated.
- **Code quality**: shared HTTP response helpers extracted to a common internal package used by both `auth` and `todos`; ignored `db` errors are replaced with proper `500` handling.
- **Ops**: multi-stage `Dockerfile` with a non-root runtime user; `docker-compose.yml` wiring the app and Postgres; GitHub Actions workflow running `go build`, `go vet ./...`, and `go test ./...`.
- Public API: unchanged except the added `logout-all` endpoint. Auth request/response shapes stay the same.
- `JWT_ACCESS_TTL_MIN`, `JWT_REFRESH_TTL_DAYS`, `BCRYPT_COST`, `RATE_LIMIT_PER_MIN` become live; the constants in `token.go` are removed in favor of config values passed into handlers.

## Testing Decisions

- A good test exercises external behavior through the HTTP surface or a module's public interface; it does not assert on private fields or query construction.
- All tests run against **Postgres** (via the compose service / CI service container). The prior sqlite-in-memory test helper is replaced. Migrations are applied as part of test setup so tests exercise the real schema.
- Existing HTTP tests (register, login, refresh rotation, logout, change-password, forgot/reset, todos CRUD, ownership isolation, complete toggle, healthz, composed-router wiring) are preserved and migrated to Postgres; they are the behavioral regression guard.
- New tests to add:
  - Config: `Load` rejects missing/short `JWT_SECRET` and invalid values; env overrides `.env`.
  - Mailer: forgot-password with a fake mailer records a message containing a usable reset link; the response stays generic for unknown emails.
  - Revocation: after change-password/reset/logout-all, a previously valid access token is rejected; ordinary logout leaves other sessions' refresh tokens valid.
  - Refresh family: replaying a rotated refresh token returns 401 and revokes sibling tokens in the family.
  - Rate limiting: exceeding the configured limit on an auth endpoint returns 429.
  - Middleware: a request receives a request ID echo and a panicking handler yields 500 without crashing the server.
- Migrations get a test that runs up then down on a throwaway Postgres database and verifies the schema is created and removed.

## Out of Scope

- Email verification / verify-email flow.
- Redis or any distributed rate-limit or session store.
- Multi-tenant/organization concepts.
- OAuth/social login, MFA/2FA, WebAuthn.
- Observability beyond request logs (no metrics/tracing/APM).
- Kubernetes manifests, Terraform, or cloud-provider-specific deployment.
- UI / frontend of any kind.
- Changing the externally documented request/response shapes other than adding `logout-all`.

## Further Notes

- Deploying the `token_version` change invalidates all existing access tokens and the refresh-family column change invalidates existing refresh rows; users will be asked to log in again. This is accepted for a pre-1.0 service.
- The in-memory rate limiter assumes a single instance; a distributed deployment needs the replaced interface implementation.
- Tests depend on a reachable Postgres (compose locally, service container in CI); document the required connection string.
- After this PRD, the README "Known gaps" section should be updated to reflect what is now handled.
