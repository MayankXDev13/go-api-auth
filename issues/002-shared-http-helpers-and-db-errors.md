## Parent PRD

`issues/prd.md`

## What to build

Remove the duplicated JSON/error response helpers in `auth` and `todos` by extracting one shared internal package used by both. In the same pass, stop dropping database errors: any `Create`, `Save`, `Update` or `Delete` that fails must return `500` and be logged instead of being silently ignored and returning `200`. External request/response shapes must not change. See the "Code quality" decision in the parent PRD.

## Acceptance criteria

- [ ] One shared response helper package is used by both `auth` and `todos`; the duplicate `writeJSON`/`writeErr` definitions are removed.
- [ ] Database failures on write paths (register, refresh rotation, change-password, reset-password, todos create/update/delete/complete) return `500` and are logged.
- [ ] No `_ =` discards remain on `db` calls that affect the response outcome.
- [ ] Existing response bodies and status codes are unchanged for the success paths.
- [ ] Existing HTTP tests still pass.

## Blocked by

None - can start immediately.

## User stories addressed

- User story 32
- User story 33
