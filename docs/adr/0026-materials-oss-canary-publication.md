---
status: accepted
amends: 0024, 0025
---

# One sealed material may be published to private OSS without activation

ADR-0024 produces an immutable sealed release ID and receipt digest. ADR-0025
currently passes that identity directly to local public-tree and catalog
activation. The OSS migration needs a smaller proof first: one reviewed public
free asset must reach the configured private Bucket without changing either
active surface.

## Decision

- The repository exposes one root-only canary wrapper. Its complete caller
  input is `--release-id`, `--receipt-sha256`, and `--asset-sha256`. It accepts
  no branch, candidate or sealed path, public path, Bucket, Endpoint, Object
  key, credential, executable override, or activation option. The asset digest
  must identify exactly one ordinary reviewed asset in both the sealed manifest
  and inventory. The receipt must bind the fixed public source
  `https://github.com/jry21223/HENU-Final-Review.git` at `refs/heads/main`;
  within that public free-material source, only roles not prefixed with
  `待复核` are eligible. This ticket does not infer access policy from a path.
- Root-owned configuration fixes the sealed root, a separate publication-audit
  root, Bucket `henukit`, region `cn-beijing`, the Beijing internal Endpoint,
  and an ECS RAM role. The wrapper starts the fixed publisher with a clean
  environment and explicitly constructed ECS RAM role authority. AccessKey,
  secret, and STS token configuration is unsupported.
- Before writing an object, the publisher revalidates the receipt digest,
  release identity, manifest and inventory digests, canonical tree, selected
  regular file, safe path, byte count, and SHA-256. It also reads and requires
  the existing Bucket to be private, Standard, ZRS, versioning Enabled, and
  SSE-OSS AES256. It has no operation for creating the Bucket or changing ACL,
  policy, versioning, redundancy, storage class, or encryption settings.
- The Object key is derived from the sealed release identity, receipt digest,
  asset digest, and reviewed public path. A replay first reads and verifies the
  existing object. Upload success is followed by HEAD and full-content readback
  verification; user metadata or ETag alone is not SHA-256 evidence. Anonymous
  access to the exact canary object must remain denied.
- After all sealed local bytes are proven and before any OSS call, a durable
  release-level binding reserves that release ID for exactly one receipt
  digest. This reservation is not publication success or activation; it
  remains after a failed remote attempt so another receipt cannot create a
  second orphan under the same release. A canonical root-owned publication receipt records
  `published_not_activated`. It is written only after remote verification and
  binds the release, receipt, asset, Object key, size, digest, and exact OSS
  version evidence. The same input replays to the same logical result. A
  different receipt cannot claim an existing release identity.
- OSS versioning preserves overwritten versions but disables
  `x-oss-forbid-overwrite` as a write-once guarantee. The publisher therefore
  relies on content-derived keys, exact pre-write verification, the fixed
  root-only audit boundary, and the publication receipt; it must not describe
  `ForbidOverwrite` as immutable storage proof.
- This slice remains inert. The production installer does not install the
  wrapper or publisher, and the materials orchestrator, systemd units,
  activation journal, public root, catalog importer, and Nginx configuration do
  not call it. Wiring publication into activation belongs to the later atomic
  OSS release ticket.

## Consequences

- A fixed sealed identity can prove private OSS transport and readback without
  exposing a new public download URL or changing the current Library catalog.
- Upload or verification failure may leave the release-level identity binding
  and an unactivated remote object version for audit, but cannot create a
  publication success receipt or activation record.
  Cleanup is restricted to exact unactivated keys and version IDs.
- The canary does not bulk-publish the manifest, generate Slides, issue signed
  browser downloads, write download statistics, install a production service,
  or authorize production activation.
