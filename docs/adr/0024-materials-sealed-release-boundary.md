---
status: accepted
amends: 0022, 0023
---

# Materials source sealing is a root-owned, non-activating boundary

ADR-0022 prepares an unprivileged candidate and ADR-0023 queues that
preparation. Neither artifact is safe as a root commit input: the
`henukit-deploy` account owns its candidate directory, and a completion marker
does not authorize publishing or catalog writes.

## Decision

- C0 introduces a repository-only, root-only sealing template. Its sole caller
  input is a constrained B01 attempt locator used for audit correlation; it is
  not a path and the template does not traverse, open, hash, parse, or copy a
  candidate directory. One root-owned, non-group/other-writable configuration
  file fixes the sealed-release root, allowed source repository, full branch
  ref, and exact lowercase 40-character source SHA. It has no option or
  configuration for a candidate root, public root, Study database, command
  override, approval, service enablement, or activation.
- Before sealing, the template independently fetches the configured source
  ref, proves it resolves to the configured exact SHA, and constructs the raw
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
  configured source and inventory, never on the attempt token. A separate
  root-owned private audit record binds each constrained attempt locator to
  that immutable release/receipt identity. Nested output directories are
  fsynced before the atomic rename. Repeating the same source release is
  idempotent; a different receipt for the same release ID fails closed.
- This is not activation. The template is not installed, enabled, packaged as
  a production runtime action, or callable from B01. It does not alter the
  public tree, Nginx, the Study catalog, schema, approval state, or any root
  service. A later, separately approved activation decision must bind an
  exact sealed receipt digest rather than a candidate path.

## Consequences

- Root receives an independently checked, auditable release identity without
  treating an unprivileged candidate marker or its contents as authority.
- A configured source ref that advances before sealing fails closed until a
  later root-owned configuration change selects its newly accepted SHA.
- This decision does not resolve catalog source ownership, a database
  migration owner, a public-tree switch, a maintenance fence, or rollback.
  Those remain prerequisites for a later commit/activation slice.
