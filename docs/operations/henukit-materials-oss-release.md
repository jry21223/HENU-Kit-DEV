# Complete OSS material release operations

This runbook describes the release boundary implemented for Issue #334. The
repository change does not itself upload or activate a production release.

## Preconditions

- The sealed release and raw `sealed-release.json` digest have passed the
  ADR-0024 review boundary.
- Bucket `henukit` is private, Standard, ZRS, versioned, and SSE-OSS AES256 in
  `cn-beijing`. The publishing and Library activation processes run on ECS RAM
  roles through IMDSv2; do not configure an AccessKey, secret, or caller STS
  token.
- The root-owned activation configuration includes the sealed/public roots,
  OSS audit root, activation staging root, fixed Library database URL, and
  dedicated Library activation RAM role. Existing installations must add the
  new #334 keys before activation; missing keys fail closed.

## Publication and activation

1. Run `/usr/local/libexec/henukit/henukit-materials-publish-release-oss` as
   root with only `--release-id` and `--receipt-sha256`. The wrapper fixes the
   publisher binary, root-owned configuration, Beijing Bucket authority, and a
   clean IMDSv2-only environment. Keep its `release-commit.json`; per-object
   receipts are not activation evidence.
2. Inspect the commit: every reviewed non-`待复核` manifest path must have its
   size, SHA-256, derived Object key, and exact OSS VersionId. An empty reviewed
   release is valid and has `asset_count: 0`.
3. Run the root activation wrapper with the same two identity arguments. It
   regenerates and pins the Library bundle, verifies derived slides/index for
   the same release, and keeps the maintenance fence until Library, legacy
   catalog, static tree, and active marker converge.
4. Do not remove `activation-journal.json` or `.maintenance` by hand. A
   `library_running` or later failure is uncertain. Restart only the same
   release identity; the Library CLI replays the exact package. A different
   release is rejected while recovery is pending.

## Forward rollback

Select a retained prior release only after the current activation is complete.
Re-run the same publication verification and activation entry with that prior
release's original receipt digest. This is a new forward activation: it retains
the newer OSS versions, release commits, installed derived tree, Library audit,
and activation history. Never delete an Object version to perform rollback.

## Failure evidence

- No `release-commit.json`: publication is incomplete; do not activate.
- Per-object receipt without a release commit: an unactivated verified object
  or orphan may remain. Diagnose and clean only by its exact key and VersionId.
- Maintenance fence plus journal: recovery is required. Preserve the journal,
  bundle, commit receipt, and Library stderr; replay the same release.
- Failed or invalid Library response: assume the database may already have
  committed. Keep the fence and reconcile by same-release replay.
