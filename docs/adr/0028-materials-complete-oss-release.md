---
status: accepted
amends: 0025, 0026, 0027
---

# Complete reviewed material releases are pinned in OSS before activation

The one-object ADR-0026 canary proves the private OSS transport but is not an
activation fact. A public Library release needs one durable identity spanning
every reviewed public-free object, the installed static and derived trees, and
the Library owner snapshot.

## Decision

- The complete publisher accepts only a sealed `release_id` and raw sealed
  receipt SHA-256. It first validates the entire local manifest, inventory,
  canonical tree, and every reviewed asset. It then publishes every approved
  object through the ADR-0026 boundary and verifies exact OSS VersionId, bytes,
  SHA-256, AES256 encryption, full readback, and anonymous denial. Pending-review
  objects are excluded. No release commit exists until every approved object is
  proven.
- The immutable `release-commit.json` binds the release and receipt identities,
  sealed metadata digests, complete bytewise-sorted asset set, derived Object
  keys, and exact OSS VersionIds. Per-object receipts or orphans are audit
  evidence only. Concurrent and repeated publication converges on the same
  exact-version commit; a conflicting receipt remains rejected.
- Activation constructs one versioned Library package from the raw sealed
  manifest, receipt, exact raw complete OSS commit, and the already verified
  installed derived inventory. `slides_sha256` is the SHA-256 of canonical
  derived asset entries; `index_sha256` is the SHA-256 of the raw
  `derived-inventory.json`. All identities must name the same release. The
  package contains no signed URL, token, client address, User-Agent, database
  authority, or caller-supplied storage configuration.
- The root wrapper fixes all paths and executables. The Library activation CLI
  accepts only `--bundle` and receives only its fixed database URL and ECS RAM
  role. Bucket `henukit`, Beijing endpoints, and IMDSv2 behavior remain fixed in
  the Library executable. Long-lived AccessKey, secret, STS, proxy, and
  caller-supplied Bucket/Endpoint variables are rejected.
- The ADR-0025 maintenance journal advances monotonically through `prepared`,
  `static_switched`, `library_running`, `library_committed`,
  `database_running`, and `database_committed`. It binds the OSS commit and
  Library bundle digests. A failure before `library_running` can restore the
  prior static pointer. Once `library_running` is durable, outcome is uncertain:
  the fence and journal remain, and restart may only replay the same release
  package until Library returns its idempotent result. The phase must never be
  downgraded. The active marker and maintenance fence change only after Library,
  legacy catalog, static, and derived state have converged on the same release.
- Rollback is a new forward activation of a retained complete release and its
  original receipt/OSS commit/package. Library records the previous active
  release and a new activation audit event. Neither OSS versions nor prior
  activation audit are deleted.

## Consequences

- A partial upload, exact-version mismatch, derived mismatch, Library failure,
  or catalog failure cannot produce an unfenced public activation.
- A post-Library-commit transport or response failure may temporarily require a
  fenced same-release recovery, but cannot silently expose a mixed release.
- This change adds verified code and release gates only. It does not upload a
  production object, execute a production activation, create a Bucket, change
  Bucket policy, or introduce a long-lived credential.
