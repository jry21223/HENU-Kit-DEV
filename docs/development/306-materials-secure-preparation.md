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

## HC-306-C0: seal one source-derived release without activation

This slice creates a root-only repository template for making an independently
verified, immutable raw-material release associated with one B01 attempt. It
is not a root commit path and does not change any public or database state.

Public seam:

1. `henukit-materials-seal --attempt .attempt.<10-alphanumeric-characters>`
   accepts only the constrained B01 attempt locator, which is audit correlation
   rather than a filesystem locator or release input. The fixed root-owned
   configuration supplies a pre-existing sealed root, source repository, full
   source branch ref, and exact lowercase SHA. Its successful output is a
   canonical sealed release ID and receipt digest. The attempt token is kept in
   a separate root-owned audit record and cannot affect that canonical identity.

Acceptance criteria:

- [ ] The command accepts no caller-selected source URL/ref, command, public
  root, Study target, approval, or activation flag. Its fixed configuration is
  a regular root-owned file that is not writable by group or other. The sealed
  root, every resolved sealed-root ancestor, existing release, receipt, and
  inventory must also be non-symlinked, root-owned, and not writable by group
  or other before reuse.
- [ ] It independently resolves the configured source ref to its configured
  lowercase 40-character SHA, uses a new root-owned detached checkout, and
  validates the fixed-source manifest plus every reviewed raw asset's path,
  byte count, SHA-256, and duplicate boundary with a deterministic UTF-8
  bytewise tree digest.
- [ ] C0 does not traverse, open, hash, parse, copy, or otherwise consume a
  B01 candidate directory. `--attempt` is a syntax-validated audit correlation
  token only. `课件PPT` remains a raw sealed asset; all derived Slides are
  explicitly deferred to a later independently bounded conversion slice.
- [ ] A malformed source manifest, source/ref/SHA mismatch, unsafe source
  path/hash, or malicious pre-seeded sealed release fails closed without
  leaving a receipt or mutating public/Study sentinel state.
- [ ] A successful seal writes only a newly-created root-owned sealed release
  and canonical inventory through an atomic receipt boundary, with every new
  output directory fsynced before rename. Its SHA/manifest identity is
  idempotent across different attempt tokens; each accepted token appends only
  a root-owned audit correlation record. A different receipt for that identity
  is rejected.
- [ ] B01 receiver/runner units and the fixed preparation wrapper do not invoke
  sealing or reference a database/public-tree target. The seal template is not
  installed, enabled, packaged for activation, or connected to an approval.
- [ ] Tests use temporary local Git fixtures and disposable local Linux/Docker
  only where ownership behavior requires it; they never contact a material
  source repository, a production host, or a production database.

Dependencies: HC-306-A01 and HC-306-B01.

Out of scope: Study/catalog writes or migrations, public-tree/Nginx changes,
approval or activation, service installation/enablement, root production
actions, rollback, Console or Library ownership, and Git-topology ordering.
