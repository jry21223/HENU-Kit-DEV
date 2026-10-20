# ADR 0050: bounded Console management for question-bank versions

Status: proposed implementation for review; no production activation.

The former QuizCraft OAuth and browser workshop were retired. Merely adding
legacy OAuth environment variables cannot restore a valid caller. Console
already owns operator login and Platform authorization; QuizCraft owns immutable
content, review, publication and audit. Add an explicit default-off Console
management bridge, using a distinct actor/permission/resource-bound HMAC key
ring, rather than reviving product login or sharing Portal/summary credentials.

Console checks exact permission and requested bank scope for each call. The owner
accepts only enumerated lifecycle actions on a private console path, validates
signed raw bytes and identity before installing a trusted request context, then
uses existing Workshop handlers. Writes-disabled, optimistic version,
idempotency, human review and audit constraints continue to apply. Tokens never
cross into browser-visible responses or the owner.

The initial scope is API-only. No new account/permission grant or database schema
is required. Configuration and the two changed service images ship exclusively
through the formal exact-main, provenance-verified runtime release path. Turning
the bridge off closes management without affecting Portal practice. Restoring
application images does not undo content publication; content recovery uses the
validated-version rollback operation and retains immutable facts.

See ../development/quizcraft-console-management-spec.md and
../operations/quizcraft-console-management.md for acceptance and rollout.
