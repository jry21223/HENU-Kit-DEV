---
status: accepted
amends: 0002
---

# Materials release preparation is a pinned, unprivileged candidate

The legacy materials synchronizer can currently fetch a moving branch, replace
the Nginx-served tree, and then attempt conversion and catalog import. That is
not an acceptable release boundary: a failed or retargeted preparation can
leave public files ahead of the catalog, and its runtime import has been
creating schema prerequisites.

## Decision

- A materials delivery is identified by an explicit tuple of source repository,
  full `refs/heads/...` ref, and a lowercase 40-character Git SHA. The
  preparation command fetches that ref, independently resolves it, rejects a
  mismatch, then checks out the accepted SHA detached. It never substitutes the
  source repository's current default branch.
- Preparation runs as a non-root local process and writes only a newly-created
  candidate directory: detached source checkout, reviewed asset mirror,
  derived Slides JSON, and a completion record. The directory Nginx serves and
  the Study catalog are not command inputs and are never changed by this step.
  The CLI rejects a candidate below the default served root or any configured
  served root, including a path that reaches one through a symbolic link.
- Every reviewed asset must have a safe, non-dotfile relative `publicPath`, a
  regular in-checkout file, an exact byte count and SHA-256 match, and a unique
  path and SHA-256. Pending-review assets remain outside the candidate mirror.
  A validation or conversion failure produces no completion record and cannot
  publish a partial candidate.
- Runtime catalog import is data manipulation only. It begins with a
  read-only psql preflight that requires the reviewed `materials.sha256`,
  `materials.slides`, and active `storage_key` uniqueness prerequisite. The
  preflight fixes writes to `pg_catalog, public` and accepts the index only
  when PostgreSQL reports it valid, ready, live, unique, partial, and keyed by
  the base `storage_key` column (not an expression); it does not issue `CREATE`, `ALTER`, `DROP`, or any other
  schema mutation.
- The retired Study API has no reviewed, release-packaged migration runner.
  Therefore this change does not invent a runtime migration or use GORM to
  alter its production schema. An operator must install the independently
  reviewed prerequisite before import; missing prerequisites fail closed.

## Consequences

- This is preparation only. A later reviewed activation slice may atomically
  switch a fully prepared candidate and synchronize the catalog, with its own
  locking, rollback, and audit contract.
- The legacy Study catalog stays a migration-era derived compatibility target;
  this decision does not make Library its owner, expose Console writes, or
  activate a root service or production configuration.
- A ref that advances between webhook acceptance and preparation is rejected
  instead of silently deploying newer source. The delivery must be retried as
  a newly accepted SHA.
