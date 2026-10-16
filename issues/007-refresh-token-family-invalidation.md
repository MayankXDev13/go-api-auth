## Parent PRD

`issues/prd.md`

## What to build

Detect refresh-token theft by binding tokens to a family. Add a family identifier to `RefreshToken`; login starts a new family, and every rotation keeps the same family while marking the old row rotated/revoked. If an already-rotated token is presented again, revoke the entire family and return `401`, forcing re-authentication. See the "Refresh-token families" decision and user stories 15-16 in the parent PRD.

## Acceptance criteria

- [ ] `RefreshToken` has a family identifier, added via a versioned migration with a down step.
- [ ] Login starts a new family; each successful refresh keeps the family and marks the presented token rotated/revoked.
- [ ] Presenting an already-rotated token returns `401` and revokes all tokens in that family.
- [ ] Sibling tokens in a reused family stop working after the reuse is detected.
- [ ] Normal rotation still works and the newest token remains valid.
- [ ] Tests cover reuse detection and family-wide revocation.

## Blocked by

- Blocked by `issues/003-versioned-migrations-and-postgres-tests.md`

## User stories addressed

- User story 15
- User story 16
