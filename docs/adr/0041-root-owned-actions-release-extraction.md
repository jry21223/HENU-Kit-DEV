---
status: accepted
---

# Keep Actions releases root-owned across extraction and one historical cutover

GitHub Actions runtime archives record the builder's numeric UID. Extracting
those archives as root without overriding archive ownership preserves that UID
on production. The rollback trust boundary correctly rejects such a release:
its owner could replace the Compose contract, release marker, or materials
manifest after verification. This also leaves one bootstrap problem when the
currently healthy rollback release predates the root-owned boundary.

## Decision

- The production Actions watcher extracts every new runtime with
  `tar --no-same-owner`. A root watcher therefore creates a root-owned candidate
  regardless of the archive builder UID.
- Normal activation keeps requiring an exact healthy fixed-SHA rollback
  release. It does not use the degraded-baseline exception from ADR-0030.
- A healthy retained release that predates root-owned extraction may be adopted
  exactly once only when the operator supplies its observed non-root numeric UID
  through `HENUKIT_RETAINED_RELEASE_OWNER_UID` for that activation.
- The activation entry runs the reviewed ownership adopter in preflight and
  execute modes before it creates the exact-SHA approval. The adopter binds the
  previous SHA, candidate SHA, historical UID, metadata digest, and complete
  content digest in a root-only audit; it rejects mixed ownership, symlinks,
  special files, and byte drift.
- The adopter is a long-lived production helper installed from the same
  verified Actions runtime as the watcher and activation entry. Candidate
  scripts are not executed from an untrusted path.
- The historical-owner variable is invalid for degraded recovery and is omitted
  after the one retained release has been adopted. Future releases need no
  ownership exception because extraction is root-owned.

## Consequences

- Rollback contracts no longer depend on a GitHub runner or local builder UID.
- The healthy normal path remains healthy-only and exact-SHA; this decision does
  not weaken or infer ADR-0030 recovery authority.
- The one metadata mutation is recoverable and auditable, and occurs before
  approval, image loading, migrations, or application activation.
