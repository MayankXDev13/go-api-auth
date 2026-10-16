## Parent PRD

`issues/prd.md`

## What to build

Add the operational server layer: request-ID, structured request logging, panic recovery and request timeout middleware on the root router; an `http.Server` with read/write/idle timeouts; graceful shutdown on SIGINT/SIGTERM; and configurable database connection pool limits. A panicking handler must yield `500` without crashing the process. See the "Middleware & server lifecycle" and "Database module" decisions in the parent PRD.

## Acceptance criteria

- [ ] Every request gets a request ID, echoed in the response and available to logs.
- [ ] Each request is logged with method, path, status and latency.
- [ ] A panicking handler is recovered, logged, and returns `500` without crashing the server.
- [ ] The `http.Server` sets read, write and idle timeouts.
- [ ] The server shuts down gracefully on SIGINT/SIGTERM, finishing in-flight requests and releasing the port.
- [ ] Database connection pool limits (max open/idle, max lifetime) come from config.
- [ ] Tests cover: request ID echo, and that a panicking handler returns `500` while the server stays up.

## Blocked by

- Blocked by `issues/001-config-load-and-validate.md`

## User stories addressed

- User story 26
- User story 27
- User story 28
- User story 29
- User story 30
- User story 31
