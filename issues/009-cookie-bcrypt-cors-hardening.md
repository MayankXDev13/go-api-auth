## Parent PRD

`issues/prd.md`

## What to build

Harden browser-facing behavior and password input. Set the `Secure` attribute on the refresh cookie when running in production (driven by config). Reject bcrypt password inputs longer than 72 bytes with `400` rather than silently truncating them, on all password-setting paths (register, change-password, reset-password). Add configurable CORS so only approved origins can call the API from a browser. See the "Browser & input hardening" scope in the parent PRD.

## Acceptance criteria

- [ ] The refresh cookie is set with `Secure` when the environment is production, and `HttpOnly` and `SameSite` are set as before.
- [ ] Passwords longer than 72 bytes are rejected with `400` on register, change-password and reset-password; no silent truncation.
- [ ] CORS is configurable via allowed origins from config and applied as middleware.
- [ ] Tests cover: `Secure` flag present in production mode; over-length password rejected; allowed origin gets CORS headers and a disallowed origin does not.

## Blocked by

- Blocked by `issues/001-config-load-and-validate.md`

## User stories addressed

- User story 17
- User story 18
- User story 21
