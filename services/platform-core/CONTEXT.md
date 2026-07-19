# Platform Core Context

## Owns

- Platform users and their account status.
- Core Sessions on the account origin and short-lived client exchange Sessions.
- Registered OAuth clients, exact callbacks, PKCE challenges, and single-use Authorization Codes.
- Permission codes, authorization roles, user Scope grants, authorization revisions, and authorization audit events.
- Verification-code security facts and the encrypted critical mail Outbox.
- Operations Inbox coordination metadata and immutable source-resource references; source product content remains with its owner.
- PostgreSQL identity facts and Redis-based short-lived coordination for this context.

## Does not own

- Console Session cookies or Console Gateway state.
- Product-local sessions, product content, or product database credentials.
- Study Legacy users, roles, routes, or API compatibility behavior.

## Current boundary

HC-05 through HC-08 establish identity authorization, verified-email mail delivery, and reference-only Operations Inbox coordination. HC-12 adds `platform.operations.read` and `platform.operations.write`, both requiring platform Scope, plus a bounded operational snapshot and audited mutation APIs for Session revocation and optimistic account status/role/Scope replacement. Durable idempotency records distinguish replay, conflicting payloads, and unknown outcomes; append-only operation audits accompany the existing authorization audits. Responses omit Session hashes/tokens, recipient ciphertext, provider identifiers, mail secrets, verification secrets, and source-product content.
