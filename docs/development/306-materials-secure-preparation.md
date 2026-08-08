# #306-A Materials secure preparation executable addendum

Source: GitHub issue #306. This addendum freezes only the first vertical slice
of that issue; it does not replace the issue's later queue, activation,
rollback, or production-runner work.

## Public seams

1. `node scripts/ops/prepare-henukit-materials.mjs --repository <url> --ref <refs/heads/name> --sha <40-lowercase-hex> --candidate-dir <new-directory>` prepares an unprivileged candidate.
2. `node scripts/ops/import-henukit-materials.mjs --manifest <path> | psql -v ON_ERROR_STOP=1 -f -` is the legacy derived-catalog import boundary.

## HC-306-A01: prepare one immutable candidate

The CLI must fetch only the supplied branch ref, prove that its resolved commit
is the supplied SHA, and use a detached checkout at that SHA. It writes a
completion record only after all validation and derived conversion succeeds.

Acceptance criteria:

- [ ] The CLI rejects a non-full branch ref, noncanonical SHA, or resolved-SHA mismatch.
- [ ] The CLI validates every reviewed asset's path, byte count, SHA-256, file type, duplicate path, and duplicate SHA-256 before it becomes a candidate asset.
- [ ] `课件PPT` conversion receives the candidate mirror and candidate Slides directory, never the served public tree.
- [ ] The CLI rejects a candidate at or below every protected served root, including through a symbolic link.
- [ ] A failed checkout, validation, or conversion leaves the existing served tree and Study catalog untouched and creates no ready marker.
- [ ] Tests use only temporary local Git repositories and local fixture files; they never contact the material source repository or production.

Dependencies: none for local preparation.

Out of scope: webhook installation, queue/coalescing, root service activation,
Nginx switch, Console or Library ownership change, database writes, production
configuration, deployment, and migration of old PR #259.

## HC-306-A02: make the derived import fail closed without DDL

The import generator must output a read-only schema preflight before its
transactional DML. The preflight requires the reviewed columns and partial
unique index relied on by the material upsert.

Acceptance criteria:

- [ ] Generated runtime SQL contains no schema-changing statement.
- [ ] Missing `materials.sha256`, `materials.slides`, or a valid, ready, live
  partial `materials_storage_key_active_idx` whose sole key is the base
  `storage_key` column stops psql before the transaction begins.
- [ ] The preflight and DML use the same fixed `pg_catalog, public` search path.
- [ ] The command tells the operator to apply the reviewed prerequisite rather
  than attempting to create it at runtime.
- [ ] No new `services/api` migration is added until that legacy service has a
  reviewed, release-packaged migration owner.

Dependencies: HC-306-A01 is independent; both slices are required before a
future activation slice.
