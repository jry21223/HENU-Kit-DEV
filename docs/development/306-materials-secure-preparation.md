# #306 Materials canonical synchronization executable spec

Source: GitHub issue #306. Sections A01, A02, B01, and C0 preserve the reviewed
prerequisite contracts. D01 below is the canonical end-to-end activation path;
the earlier statements that activation remained future or inert are superseded.

## Public seams

1. `node scripts/ops/prepare-henukit-materials.mjs --repository <url> --ref <refs/heads/name> --sha <40-lowercase-hex> --candidate-dir <new-directory>` prepares an unprivileged candidate.
2. `node scripts/ops/import-henukit-materials.mjs --manifest <path> --release-id <approved-id> | psql -v ON_ERROR_STOP=1 -f -` is the derived-catalog import boundary; the privileged activation wrapper supplies the private PostgreSQL service configuration.
3. `henukit-deploy-webhook materials-serve` is the sole materials receiver.
4. `henukit-deploy-webhook materials-run` invokes only the fixed privileged orchestrator.
5. `henukit-materials-activate --release-id <id> --receipt-sha256 <digest>` activates or rolls back to one sealed release.

## HC-306-A01: prepare one immutable candidate

The CLI must fetch only the supplied branch ref, prove that its resolved commit
is the supplied SHA, and use a detached checkout at that SHA. It writes a
completion record only after all raw-asset validation succeeds. Online preview
conversion is disabled by ADR-0031 and is not a preparation step.

Acceptance criteria:

- [ ] The CLI rejects a non-full branch ref, noncanonical SHA, or resolved-SHA mismatch.
- [ ] The CLI validates every reviewed asset's path, byte count, SHA-256, file type, duplicate path, and duplicate SHA-256 before it becomes a candidate asset.
- [ ] `课件PPT` remains an original downloadable asset; preparation has no converter or Slides output path.
- [ ] The CLI rejects a candidate at or below every protected served root, including through a symbolic link.
- [ ] A failed checkout or validation leaves the existing served tree and Study catalog untouched and creates no ready marker.
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

Dependencies: HC-306-A01 is independent; both slices are prerequisites for D01.

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
- [ ] The materials receiver remains unprivileged. The root queue runner invokes
  only the fixed orchestrator; preparation is run as `henukit-deploy`, and an
  event cannot select commands, paths, public roots, or database targets.
- [ ] Tests prove generic FIFO remains unchanged; materials A/B/C coalescing,
  active A plus B/C, duplicate delivery, recovery, and concurrent queue access
  retain the one-running/one-waiting invariant.
- [ ] Tests prove the receiver is `henukit-deploy`, the runner is a confined
  root unit, and its fixed chain alone may cross the seal/activation boundary.
- [ ] CI runs the affected Go, Node, and systemd-template checks.

Dependencies: HC-306-A01.

B01 alone does not authorize activation; D01 supplies the reviewed privileged
boundary. Database migration, Console ownership, and Git-topology inference
remain out of scope.

## HC-306-C0: seal one source-derived release without activation

This slice creates a root-only repository template for making an independently
verified, immutable raw-material release associated with one B01 attempt. It
is not a root commit path and does not change any public or database state.

Public seam:

1. `henukit-materials-seal --attempt .attempt.<10-alphanumeric-characters> --sha <40-lowercase-hex>`
   accepts only the constrained B01 attempt locator, which is audit correlation
   rather than a filesystem locator or release input, plus the exact SHA from
   the authenticated queued event. The fixed root-owned configuration supplies
   a pre-existing sealed root, source repository, and full source branch ref.
   Its successful output is a canonical sealed release ID and receipt digest.
   The attempt token is kept in a separate root-owned audit record and cannot
   affect that canonical identity.

Acceptance criteria:

- [ ] The command accepts no caller-selected source URL/ref, command, public
  root, Study target, approval, or activation flag. Its fixed configuration is
  a regular root-owned file that is not writable by group or other. The sealed
  root, every resolved sealed-root ancestor, existing release, receipt, and
  inventory must also be non-symlinked, root-owned, and not writable by group
  or other before reuse.
- [ ] It independently resolves the configured source ref to the accepted
  lowercase 40-character event SHA, uses a new root-owned detached checkout, and
  validates the fixed-source manifest plus every reviewed raw asset's path,
  byte count, SHA-256, and duplicate boundary with a deterministic UTF-8
  bytewise tree digest.
- [ ] C0 does not traverse, open, hash, parse, copy, or otherwise consume a
  B01 candidate directory. `--attempt` is a syntax-validated audit correlation
  token only. `课件PPT` remains a raw sealed asset; derived Slides are disabled,
  not deferred, and no later conversion slice is part of this product boundary.
- [ ] A malformed source manifest, source/ref/SHA mismatch, unsafe source
  path/hash, or malicious pre-seeded sealed release fails closed without
  leaving a receipt or mutating public/Study sentinel state.
- [ ] A successful seal writes only a newly-created root-owned sealed release
  and canonical inventory through an atomic receipt boundary, with every new
  output directory fsynced before rename. Its SHA/manifest identity is
  idempotent across different attempt tokens; each accepted token appends only
  a root-owned audit correlation record. A different receipt for that identity
  is rejected.
- [ ] The preparation wrapper does not invoke sealing or reference a
  database/public-tree target. Only D01's fixed root orchestrator may invoke
  the seal wrapper, and sealing itself cannot publish or write the catalog.
- [ ] Tests use temporary local Git fixtures and disposable local Linux/Docker
  only where ownership behavior requires it; they never contact a material
  source repository, a production host, or a production database.

Dependencies: HC-306-A01 and HC-306-B01.

Out of scope: Study/catalog writes or migrations, public-tree/Nginx changes,
approval or activation, service installation/enablement, root production
actions, rollback, Console or Library ownership, and Git-topology ordering.

## HC-306-D01: canonical privileged activation and rollback

- `/webhooks/materials`, `henukit-materials-webhook.service`, its credential
  secret, latest-arrival state directory, path unit, and root runner are the
  only supported materials delivery path. Retired sync scripts are forbidden.
- The root runner validates repository/ref against root-owned configuration and
  validates the accepted full SHA from the authenticated queue event, runs A01
  as `henukit-deploy`, performs C0 sealing as root, and activates only the
  returned release ID plus receipt digest.
- Activation holds the fixed lock, creates `.maintenance`, writes the durable
  journal, durably installs the release, switches the internal recovery pointer,
  imports immutable release-prefixed catalog keys transactionally, writes
  `ACTIVE_RELEASE`, then removes journal and fence.
- `database_running` is uncertain and remains fenced. Retry exactly the same
  approved release; never delete the journal or fence manually.
  `database_committed` recovery finalizes without a second catalog transaction.
- Explicit rollback invokes the same activation wrapper with a retained prior
  sealed release ID and matching receipt digest. This forward reconciliation
  replaces both public tree and catalog; manual symlink or SQL rollback is invalid.
