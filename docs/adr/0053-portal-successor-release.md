---
status: accepted
---

# Release Portal over the active Practice cohort

The mobile presentation changes require only Portal. ADR-0051's five-service
Practice cohort is already active while Career/getWork remains deferred.
Rolling that cohort back would remove independent delivered functionality;
its original global-environment snapshot also predates an authorized mail
migration. Neither rewriting that receipt nor restoring the old environment
is a valid way to publish a frontend change.

## Decision

Add one fixed Portal-only successor to an active ADR-0051 cohort. Reuse its
authenticated exact-main Actions runtime/image gates, existing watcher locks,
private durable receipts and shared scoped marker. Accept only the known
Practice format and exact five-service scope, and prove its active image IDs,
retained helper/engine/Compose hashes and full baseline before proceeding.

The successor captures the current effective environment and all unselected
container identities. Retained and candidate Portal configuration, including
shared resources, must agree apart from the candidate image. Back up that
current environment without editing it. Portal has no database authority, so
this path executes no database command, migration, or database restore.

Replace only Portal. Keep full-stack aliases and the full marker unchanged,
and advance the existing scoped marker atomically to the successor receipt.
That receipt distinguishes the full baseline from the previous Portal image,
retains the predecessor receipt digest/path, and leaves the predecessor
receipt byte-identical. Public acceptance and effective configuration checks
apply to both activation and rollback.

On failure restore the immediately previous Portal image with its retained
Compose and the current environment, then restore the Practice marker.
Unselected identity/configuration drift blocks automatic repair and leaves a
blocking failure receipt for reconciliation. Rollback never restores an old
database, old SMTP configuration, or an obsolete full-stack Portal image.

## Consequences

This narrow entry is not a general cohort graph or full release. A second
successor or another predecessor format is rejected and requires a reviewed
extension. Portal Summary continues to identify the retained full baseline;
the scoped marker, receipt and actual image identify this Portal release.
Career/getWork acceptance remains deferred. The old Practice rollback must
not be invoked while the successor owns the marker.

See [the operations runbook](../operations/henukit-portal-successor-release.md).
