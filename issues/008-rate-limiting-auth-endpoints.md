## Parent PRD

`issues/prd.md`

## What to build

Throttle auth endpoints with an in-memory rate limiter behind an interface so a distributed store can replace it later. The limit comes from config (`RATE_LIMIT_PER_MIN`) and applies to auth routes (at minimum login, register, forgot-password, reset-password and refresh). Exceeding the limit returns `429` with a `Retry-After` header. Key by client IP, and additionally by email where relevant, so credential stuffing is throttled. See the "Rate limiting" decision in the parent PRD.

## Acceptance criteria

- [ ] An in-memory token-bucket limiter is implemented behind an interface.
- [ ] The configured `RATE_LIMIT_PER_MIN` drives the limit; it is applied to the auth endpoints listed above.
- [ ] Requests over the limit return `429` with a `Retry-After` header.
- [ ] Keying uses client IP (and email where applicable) so repeated attempts from one source are throttled.
- [ ] A test exceeding the configured limit on an auth endpoint asserts `429`, and a under-limit request still succeeds.
- [ ] The limiter interface allows a future non-in-memory implementation without changing handlers.

## Blocked by

- Blocked by `issues/001-config-load-and-validate.md`

## User stories addressed

- User story 19
- User story 20
- User story 22
