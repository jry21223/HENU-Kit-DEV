# Domain Docs

How engineering skills consume this Monorepo's domain documentation.

## Before exploring

1. Read `CONTEXT-MAP.md` at the repository root.
2. Read every linked `CONTEXT.md` relevant to the requested work.
3. Read system-wide ADRs under `docs/adr/` that affect the area.
4. Read context-specific ADRs beside the owning context when they exist.

If a context file or ADR directory does not exist, proceed silently. Domain files are created lazily when terms or decisions are resolved.

## Layout

- `CONTEXT-MAP.md`: context index and relationships.
- Root `CONTEXT.md`: current management-plane and cross-product operations glossary during the extraction phase.
- `docs/adr/`: system-wide architectural decisions.
- `<owning-app-or-service>/CONTEXT.md`: target location for a product or service glossary once its implementation directory exists.
- `<owning-app-or-service>/docs/adr/`: context-specific decisions.

Target contexts include HENUKit Console, Console Gateway, Platform Core, Portal, Notice, Library, QuizCraft, Food, and Study Legacy. Split terms from the transitional root glossary as the owning directories are established; do not create empty context files in advance.

## Vocabulary

Use the glossary's canonical terms in issue titles, specifications, code, tests, and reviews. Avoid synonyms explicitly listed under `_Avoid_`.

If a required concept is absent, reconsider whether it belongs to the project language or record the gap for `domain-modeling`.

## ADR conflicts

If proposed work conflicts with an ADR, surface the conflict explicitly. Do not silently override accepted decisions.
