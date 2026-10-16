## Parent PRD

`issues/prd.md`

## What to build

Make access tokens revocable before their 15-minute expiry using a per-user token version. Add a `token_version` column to `User` and embed it as a claim in access tokens; `RequireAuth` rejects a token whose version is lower than the user's current version. Change-password, reset-password and a new `logout-all` endpoint bump `token_version`. Ordinary logout continues to revoke only the presented refresh token. Add `POST /api/v1/auth/logout-all` so a user can end every session at once. See the "Token revocation" decision in the parent PRD.

## Acceptance criteria

- [ ] `User` has a `token_version` column, added via a versioned migration with a down step.
- [ ] Access tokens carry the token version; `RequireAuth` rejects tokens with a stale version with `401`.
- [ ] `PUT /api/v1/auth/change-password` bumps `token_version`, invalidating previously issued access tokens.
- [ ] `POST /api/v1/auth/reset-password` bumps `token_version` and revokes refresh tokens.
- [ ] New `POST /api/v1/auth/logout-all` bumps `token_version` and revokes all of the user's refresh tokens.
- [ ] Ordinary `logout` still revokes only the presented refresh token and leaves other sessions valid.
- [ ] Tests cover: access token rejected after change-password/reset/logout-all; ordinary logout does not affect other sessions.

## Blocked by

- Blocked by `issues/001-config-load-and-validate.md`
- Blocked by `issues/003-versioned-migrations-and-postgres-tests.md`

## User stories addressed

- User story 11
- User story 12
- User story 13
- User story 14
