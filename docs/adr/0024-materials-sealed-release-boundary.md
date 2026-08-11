---
status: accepted
amends: 0022, 0023
superseded_by: 0025
---

# Materials source sealing is a root-owned, non-activating boundary

ADR-0022 prepares an unprivileged candidate and ADR-0023 queues that
preparation. Neither artifact is safe as a root commit input: the
`henukit-deploy` account owns its candidate directory, and a completion marker
does not authorize publishing or catalog writes.

## Decision

- C0 introduces a repository-only, root-only sealing template. Its caller
  inputs are a constrained B01 attempt locator used for audit correlation and
  the exact lowercase 40-character SHA from the verified queued event. The
  locator is not a path and the template does not traverse, open, hash, parse,
  or copy a candidate directory. One root-owned, non-group/other-writable
  configuration file fixes the sealed-release root, allowed source repository,
  and full branch ref. It has no option or
  configuration for a candidate root, public root, Study database, command
  override, approval, service enablement, or activation.
- Before sealing, the template independently fetches the configured source
  ref, proves it resolves to the accepted event SHA, and constructs the raw
  reviewed-asset inventory from its new root-owned detached checkout. It
  rejects a source/ref/SHA mismatch, unsafe or duplicate manifest path/hash,
  invalid byte count, symbolic link, missing source file, or source
  size/hash mismatch. Source metadata has a bounded read size. This releases
  no derived Slides: `课件PPT` files are sealed only as raw reviewed assets and
  the receipt marks Slides as deferred. A later slice must bind independently
  bounded unprivileged conversion output to this sealed receipt.
- Sealing copies only verified regular files from that root-owned checkout into
  a newly-created root-owned release below a pre-existing root-owned sealed
  root. Every resolved sealed-root ancestor is root-owned and non-writable by
  group/other. It rejects symlinked or group/other-writable sealed paths, and
  rejects a pre-seeded release, receipt, or inventory unless every existing
  item is root-owned, non-writable by group/other, and byte-for-byte identical. It
  records a canonical UTF-8-bytewise-sorted inventory and tree digest plus a
  sealed receipt. The release ID and receipt digest depend only on the
  accepted source and inventory, never on the attempt token. A separate
  root-owned private audit record binds each constrained attempt locator to
  that immutable release/receipt identity. Nested output directories are
  fsynced before the atomic rename. Repeating the same source release is
  idempotent; a different receipt for the same release ID fails closed.
- Sealing itself is not activation and still cannot alter public or catalog
  state. ADR-0025 packages this boundary behind the fixed privileged runner;
  activation receives only the exact sealed release ID and receipt digest,
  never a candidate path.

## Consequences

- Root receives an independently checked, auditable release identity without
  treating an unprivileged candidate marker or its contents as authority.
- A configured source ref that advances before sealing fails closed; the
  latest-arrival queue may then process the newer accepted event SHA.
- ADR-0025 resolves the public-tree switch, maintenance fence, recovery journal,
  and forward rollback to a previously sealed release. Schema prerequisites
  remain separately reviewed and are never installed by runtime activation.
