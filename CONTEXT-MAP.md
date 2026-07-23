# HENU Kit Context Map

## Current contexts

- [Management Plane and Cross-Product Operations](./CONTEXT.md) — accepted language for HENUKit Console, Platform Operations, module boundaries, QuizCraft learning behavior, and migration-era legacy separation.
- [HENUKit Console](./apps/console/CONTEXT.md) — application boundary, bundle isolation, and Console-owned presentation terms.
- [Study Legacy Admin](./apps/study-legacy-admin/CONTEXT.md) — preserved legacy behavior, rollback entrypoint, and retirement boundary.
- [Platform Core](./services/platform-core/CONTEXT.md) — platform identity ownership, Core Session, OAuth client, Authorization Code, and Redis coordination boundaries.
- [Console Gateway](./services/console-gateway/CONTEXT.md) — Console-local authorization callback, Session cookie, and verified access-context boundary.
- [Library Compatibility](./services/library/CONTEXT.md) — Library Module terms, bounded legacy translation, idempotency, and degradation semantics.
- [Food Operations](./services/food/CONTEXT.md) — Food-owned submissions, anomaly tickets, tier adjustments, idempotency, and stale semantics.
- [QuizCraft](./products/quizcraft/CONTEXT.md) — Practice Core, stable question identity, versioned banks, explicit imports, and workshop boundaries.
- [Portal](./apps/portal/CONTEXT.md) — Portal main site, module sections, sub-sites, design language, and deterministic SSR.
- [Portal Gateway](./services/portal-gateway/CONTEXT.md) — Portal OAuth session, read-only product proxying, and portal-scoped permissions.
- [Portal API](./services/portal-api/CONTEXT.md) — User-facing data API for Portal, contract derived from frontend mock interfaces.

## Extraction targets

As implementation directories are established, move resolved terms into context-owned glossaries without duplicating definitions:

- [Notice Service](./services/notice/CONTEXT.md) — notice sources, immutable versions, review, audience, and distribution.

Do not create empty context files. Move a term when its owning implementation context is materialized or when the term is next changed.

## Relationships

- **HENUKit Console → Console Gateway**: the browser uses one Console-specific API and does not call internal product services directly.
- **Console Gateway → Platform Core and product services**: the Gateway validates Console access, aggregates summaries, and forwards controlled operations without owning business data.
- **Platform Core → all products**: supplies identity, permission codes, scopes, sessions, mail infrastructure, audit, and Operations Inbox references.
- **Notice, Library, QuizCraft, and Food → Console Gateway**: each remains the sole data owner and exposes versioned contracts.
- **Study Legacy Admin → Study Legacy API**: remains physically separate from HENUKit Console during migration and retirement.
