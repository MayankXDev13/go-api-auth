## Parent PRD

`issues/prd.md`

## What to build

Make the service deployable and continuously verified. Add a multi-stage `Dockerfile` that builds a small image running as a non-root user, a `docker-compose.yml` wiring the app together with Postgres (including running migrations before the server starts), and a GitHub Actions workflow that runs `go build`, `go vet ./...` and `go test ./...` on pushes and pull requests using a Postgres service container. See the "Ops" decision in the parent PRD.

## Acceptance criteria

- [ ] Multi-stage `Dockerfile` produces a small runtime image that runs as a non-root user.
- [ ] `docker-compose.yml` starts the app and Postgres, applies migrations, and exposes the API.
- [ ] `.env`/secrets are not baked into the image and `.dockerignore` excludes them.
- [ ] GitHub Actions workflow runs build, vet and tests on push and PR.
- [ ] CI runs tests against a Postgres service container and passes on a clean checkout.
- [ ] `docker compose up` yields a working `GET /healthz`.

## Blocked by

- Blocked by `issues/001-config-load-and-validate.md`
- Blocked by `issues/003-versioned-migrations-and-postgres-tests.md`

## User stories addressed

- User story 34
- User story 35
- User story 36
