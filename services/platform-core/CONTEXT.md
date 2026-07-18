# Platform Core Context

## Owns

- Platform users and their account status.
- Core Sessions on the account origin and short-lived client exchange Sessions.
- Registered OAuth clients, exact callbacks, PKCE challenges, and single-use Authorization Codes.
- Permission codes, authorization roles, user Scope grants, authorization revisions, and authorization audit events.
- Verification-code security facts and the encrypted critical mail Outbox.
- PostgreSQL identity facts and Redis-based short-lived coordination for this context.

## Does not own

- Console Session cookies or Console Gateway state.
- Product-local sessions, product content, or product database credentials.
- Study Legacy users, roles, routes, or API compatibility behavior.

## Current boundary

HC-05 delivers authorization for an already authenticated Core Session and the server-to-server code exchange. HC-06 adds server-authenticated, default-deny permission and Scope checks with transactional audit and next-request revocation propagation. HC-07 adds API-first student-email verification, single-use hash-only codes, multi-dimensional fail-closed rate limits, an encrypted PostgreSQL Outbox, authenticated provider delivery receipts, immutable mail audits/dead letters, controlled requeue, and a separately deployable provider worker with retry and recovery. Turning a verified code into account/bootstrap Session state, session-administration APIs, role/grant management APIs, and Console Gateway integration remain planned. Tests seed users, OAuth clients, Sessions, roles, permissions, and grants directly into a dedicated test database; this is test setup, not a production management API.
