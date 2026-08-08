# Notice Service Context

## Owns

- Whitelisted Notice sources and their provenance.
- Immutable Notice content versions.
- Review decisions, audiences, distribution state, idempotency facts, and audit events.

## Does not own

- Platform accounts, Console Sessions, permission grants, or product Scope.
- Console presentation state or Gateway forwarding state.
- Platform Core user notifications and read preferences.

## Current boundary

HC-13 materializes Notice as an independent PostgreSQL data owner. Console Gateway verifies `notice.*` permissions with Platform Core for product Scope `notice`, then signs bounded HTTP requests carrying the verified actor. Portal Gateway uses a distinct snapshot-only credential for its actor-bound `notice.read`; that credential cannot reach Notice management, review, or distribution routes. Notice rechecks the signed route permission, owns final idempotency and optimistic revisions, and keeps content versions immutable.
