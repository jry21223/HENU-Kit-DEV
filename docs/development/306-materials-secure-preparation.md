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

## HC-306-B01: queue only the newest accepted candidate preparation

This slice adds a materials-instance receiver/runner seam for the candidate
command in HC-306-A01. It does not make a candidate public or authorize a
catalog import.

Acceptance criteria:

- [ ] `henukit-deploy-webhook materials-serve` and
  `henukit-deploy-webhook materials-run` select a materials-only queue policy;
  generic `serve`, `run`, and `retry` retain their existing FIFO behavior.
- [ ] A materials state directory has zero or one running delivery and zero or
  one waiting delivery. A newly accepted delivery atomically replaces the
  waiting delivery without interrupting the running preparation.
- [ ] “Newest” is the last accepted webhook delivery in arrival order, not a
  Git topology or commit-time inference. A replaced delivery remains
  deduplicated during terminal retention, and restart recovery never requeues
  an old running delivery ahead of a newer waiting one.
- [ ] The materials receiver and runner use the same unprivileged
  `henukit-deploy` account. The runner invokes only a fixed wrapper with a
  configuration-bound source repository, allowed ref, and candidate root; an
  event cannot select commands, paths, public roots, or database targets.
- [ ] Tests prove generic FIFO remains unchanged; materials A/B/C coalescing,
  active A plus B/C, duplicate delivery, recovery, and concurrent queue access
  retain the one-running/one-waiting invariant.
- [ ] Tests prove the materials runner passes only fixed candidate-preparation
  inputs and the unit templates run as `henukit-deploy` without root, Docker,
  psql, public-tree, or Study-catalog write access.
- [ ] CI runs the affected Go, Node, and systemd-template checks.

Dependencies: HC-306-A01.

Out of scope: enabling or installing a service, production packaging or
deployment, public-tree activation, Nginx switching, Study catalog import,
database migration, root runtime processes, Console or Library ownership, and
Git-topology ordering.
