---
status: accepted
---

# Recover production from an exact degraded fixed-SHA baseline

The normal deployment path requires the currently active fixed-SHA release to
be healthy before it can serve as a rollback target. That rule prevents a new
release from removing the last known-good production state. It also creates a
bootstrap deadlock when production is already degraded and no healthy retained
fixed-SHA release exists.

## Decision

- The normal path remains unchanged and requires a healthy fixed-SHA rollback
  release.
- Recovery is available only to signed local artifacts through one explicit
  `--recover-degraded-baseline <full-current-sha>` argument carried unchanged
  by the WSL transport, activation entry, and watcher. Actions polling cannot
  infer or request this exception.
- The declared SHA must differ from the candidate, match the root-owned current
  release symlink through a root-owned, non-writable trust chain, name a
  complete retained release with an exact `RELEASE_SHA`, Compose contract, and
  executable deployment helper, and match the exact image inventory across
  running or stopped containers. Completeness is proven before health is
  evaluated. The baseline must then fail the normal health check; a healthy
  baseline uses the normal path.
- Signature verification, current `origin/main`, exact-SHA approval, production
  environment gates, restore-tested fresh backups, image inventory, Account
  permission grant, and deterministic smoke checks remain mandatory.
- Before image loading, the watcher writes an immutable root-only authorization
  record binding candidate SHA, previous SHA, timestamp, and verified backup.
  A separate immutable terminal record reports successful activation or
  restoration of the known degraded baseline. Re-entry with the same explicit
  recovery tuple must verify the authorization record and idempotently finish
  a missing terminal record before recording activation or permitting a new
  attempt.
- Candidate failure invokes the retained previous helper with the pre-release
  environment snapshot. Recovery proves the current symlink and exact image
  identities but does not claim the known degraded state is healthy.
- Recovery-aware long-lived helpers must first be installed through the
  existing reviewed trust-root bootstrap. Candidate runtime contents never
  replace production trust roots merely because they are release artifacts.

## Consequences

- A first recovery can move production forward without pretending that a
  degraded state is a healthy rollback release.
- The exception is one-shot, exact-SHA, auditable, and unavailable to routine
  unattended deployments.
- Operators retain the degraded baseline and its environment snapshot until
  the candidate completes health, smoke, and observation checks.
