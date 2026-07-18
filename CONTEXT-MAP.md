# HENU Kit Context Map

## Current contexts

- [Management Plane and Cross-Product Operations](./CONTEXT.md) — accepted language for HENUKit Console, Platform Operations, module boundaries, QuizCraft learning behavior, and migration-era legacy separation.
- [HENUKit Console](./apps/console/CONTEXT.md) — application boundary, bundle isolation, and Console-owned presentation terms.
- [Study Legacy Admin](./apps/study-legacy-admin/CONTEXT.md) — preserved legacy behavior, rollback entrypoint, and retirement boundary.

## Extraction targets

As implementation directories are established, move resolved terms into context-owned glossaries without duplicating definitions:

- `services/console-gateway/CONTEXT.md` — Console Gateway and Console Session edge behavior.
- `services/platform-core/CONTEXT.md` — accounts, access context, sessions, Operations Inbox, mail, and audit.
- `apps/portal/CONTEXT.md` — Portal and Portal Configuration.
- `services/notice/CONTEXT.md` — notice sources, immutable versions, review, audience, and distribution.
- `services/library/CONTEXT.md` — courses, materials, downloads, submissions, review, and correction.
- `products/quizcraft/CONTEXT.md` — Practice Core, favorites, rankings, feedback, and Question Bank Workshop.
- `services/food/CONTEXT.md` — food submissions, calibration, anomalies, and tier adjustment.

Do not create empty context files. Move a term when its owning implementation context is materialized or when the term is next changed.

## Relationships

- **HENUKit Console → Console Gateway**: the browser uses one Console-specific API and does not call internal product services directly.
- **Console Gateway → Platform Core and product services**: the Gateway validates Console access, aggregates summaries, and forwards controlled operations without owning business data.
- **Platform Core → all products**: supplies identity, permission codes, scopes, sessions, mail infrastructure, audit, and Operations Inbox references.
- **Notice, Library, QuizCraft, and Food → Console Gateway**: each remains the sole data owner and exposes versioned contracts.
- **Study Legacy Admin → Study Legacy API**: remains physically separate from HENUKit Console during migration and retirement.
