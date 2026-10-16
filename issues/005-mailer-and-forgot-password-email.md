## Parent PRD

`issues/prd.md`

## What to build

Deliver the password-reset email end to end. Introduce a `Mailer` interface with an SMTP implementation and a console/log implementation, injected into the auth handler. The forgot-password handler builds the reset link from a configurable base URL and sends it through the mailer, while still returning a generic `200` regardless of whether the email exists. The dev/console mailer logs the raw reset link so the flow is completable locally. The existing single-use, expiring reset token behavior is retained and verified. See the "Mailer module" decision in the parent PRD.

## Acceptance criteria

- [ ] A `Mailer` interface with two implementations: SMTP (using configured SMTP settings) and console/log (used in dev and tests).
- [ ] The auth handler depends on the `Mailer` interface, not a concrete mailer.
- [ ] `forgot-password` sends a reset link containing a usable token via the mailer.
- [ ] `forgot-password` returns the same generic `200` body for unknown emails and mailer failures (no user enumeration).
- [ ] The console mailer logs the raw reset link for local use.
- [ ] A test with a fake mailer asserts a message with a usable reset link was sent, and that the full reset flow succeeds with that token.
- [ ] Reset tokens remain single-use and expiring; a reused/expired token is rejected.

## Blocked by

- Blocked by `issues/001-config-load-and-validate.md`

## User stories addressed

- User story 6
- User story 7
- User story 8
- User story 9
- User story 10
