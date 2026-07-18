# Platform Core Context

## Owns

- Platform users and their account status.
- Core Sessions on the account origin and short-lived client exchange Sessions.
- Registered OAuth clients, exact callbacks, PKCE challenges, and single-use Authorization Codes.
- PostgreSQL identity facts and Redis-based short-lived coordination for this context.

## Does not own

- Console Session cookies or Console Gateway state.
- Product-local sessions, product content, or product database credentials.
- Study Legacy users, roles, routes, or API compatibility behavior.

## Current boundary

HC-05 delivers authorization for an already authenticated Core Session and the server-to-server code exchange. Core account login, email verification, role/Scope propagation, session administration, and Console Gateway integration remain planned. Tests seed users, OAuth clients, and Core Sessions directly into a dedicated test database; this is test setup, not a production bootstrap API.
