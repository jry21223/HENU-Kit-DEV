---
status: accepted
amends: 0026, 0027
---

# Library activates one complete verified OSS owner catalog

ADR-0026 proved one sealed asset could be published privately without changing
an active surface. ADR-0027 then made Library the owner of public-free download
grants and Download Start facts, but #333 intentionally used only a controlled
active fixture. A complete release needs one fail-closed transition from sealed
review evidence and exact OSS object versions to the owner catalog.

## Decision

- The activation package binds the original reviewed manifest bytes, original
  sealed receipt bytes, original complete OSS release-commit bytes, release ID,
  complete reviewed object inventory, exact
  OSS VersionIds, and the same-release derived Slides and index digests. Library
  recomputes manifest, receipt, OSS commit, catalog, and activation digests; callers cannot
  supply alternate bytes, sizes, hashes, Bucket, Endpoint, access state, status,
  material IDs, or storage policy.
- Manifest roles beginning with `待复核` are excluded. Every remaining asset
  must have exactly one object row keyed by reviewed public path. Equal content
  at distinct paths is valid; missing, extra, unsafe, or duplicate paths are
  invalid. Stable material IDs derive from the reviewed public path.
- Before the database transition, Library HEADs every exact Object key and
  VersionId through the fixed internal endpoint and verifies bytes, SHA-256
  metadata, AES256 encryption, returned VersionId, and denied anonymous access.
  Any failed check leaves the previous catalog active.
- A transaction-scoped advisory lock serializes every activation and rollback.
  The transaction inserts an immutable release snapshot when first seen,
  retires the previous active identity, activates exactly one target identity,
  and appends one audit event. An exact replay of the already-active package is
  a no-op. Reusing a release ID with different evidence fails closed. Rollback
  is a forward activation of an exact retained package and creates a new audit
  event; it never deletes the failed or newer release history.
- `GET /api/v1/public-materials` is the exact signed owner read. One repeatable
  database snapshot returns the complete active public-free collection, stable
  per-material Download Start counts, and the global append-only ledger count.
  It accepts no search or filter. With no active release it returns an explicit
  empty success and a nullable release identity; a dependency failure fails the
  whole response instead of preserving a partial collection or false zero.
- The activation executable accepts one strict JSON bundle file (at most 500
  reviewed materials) and fixed
  database/ECS RAM role configuration. It does not accept Bucket, Endpoint,
  Object key overrides, static AccessKeys, secrets, STS tokens, SQL, or a
  production HTTP activation route. Production publication, authorization,
  canary, and observed rollback remain release gates rather than implications
  of this implementation.

## Consequences

- OSS bytes remain immutable release inputs while Library owns the only active
  catalog pointer and its business aggregates.
- Search and presentation filters cannot change “收录资料” or “累计下载”; both
  values come from the same unfiltered owner snapshot.
- A green local activation test proves transaction and verification behavior,
  not that a production release was uploaded, activated, or downloaded.
