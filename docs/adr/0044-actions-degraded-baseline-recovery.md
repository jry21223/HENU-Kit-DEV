---
status: accepted
---

# Permit explicit Actions recovery from an exact degraded baseline

ADR-0030 made degraded-baseline recovery available only to SSH-signed local
artifacts. Production now uses GitHub Actions as its primary fixed-SHA build
source, so that restriction can deadlock an otherwise verified current-main
release behind a baseline that is healthy under its historical contract but
does not satisfy a newly added release health contract.

## Decision

- This ADR amends only ADR-0030's artifact-source restriction. Every exact
  baseline, approval, backup, rollback, materials-state, permission, smoke, and
  immutable-audit requirement remains unchanged.
- The recovery candidate may be the newest completed successful deployment
  workflow run only when its SHA still equals a fresh lookup of the current
  `main` branch head. The activation entry repeats those checks before
  preparation and before activation.
- The operator must name the candidate and exact degraded baseline explicitly
  through `--recover-degraded-baseline`. Watch mode and ordinary Actions polling
  cannot infer, retain, or request the exception.
- Actions artifacts keep their existing authenticated workflow-download and
  checksum contract. They are not relabeled as locally signed artifacts and do
  not enter the mutually exclusive SSH-signed local trust path.
- An unconsumed approval is resumable only when its root-only prepared evidence
  and recovery binding name the same candidate and exact baseline, and its
  artifact identity still names the same Actions run database ID and run
  attempt, or the same signed local manifest digest. Normal activation still
  refuses every pre-existing approval.
- A stale branch head, incomplete or unsuccessful workflow, healthy or
  mismatched baseline, missing recovery binding, or source-mode mismatch fails
  before approval reuse or activation.

## Consequences

- A GitHub Actions-built current-main release can use the same explicit,
  one-shot degraded-baseline recovery contract without introducing a local
  signing identity or weakening provenance checks.
- Automated polling remains unable to invoke the exception.
- The local signed recovery path from ADR-0030 remains available and unchanged.
