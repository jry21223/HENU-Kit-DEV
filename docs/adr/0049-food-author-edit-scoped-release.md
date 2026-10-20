---
status: accepted
---

# Release author editing as three fixed deploy units

Food Post author editing changes Portal, Portal Gateway and Food without a
schema migration or a changed service configuration contract. A separately
unavailable Job Source MCP must not be reported healthy merely to publish that
change. An explicitly authorized operator may instead release these three
units while deferring getWork acceptance.

## Decision

- The scope is fixed in code to `portal`, `portal-gateway` and `food`. There is
  no arbitrary service selector. Every unselected container retains its ID,
  image, process start/exit state and restart count, including the stopped
  Career/getWork group.
- The candidate must still be current main from the newest successful main
  push workflow. Keep authenticated Actions downloads and checksum checks;
  additionally verify the existing Actions attestation that binds the runtime.
  Verify the executed helper and its critical runtime inputs against that
  archive, and image archive tag/config identities before Docker load.
- Require unchanged selected-service configuration and shared resource
  declarations, identical Food migrations, a root-only configuration backup,
  isolated restore verification of the Food backup, and the normal free-disk
  floor. This path applies no migrations or materials runtime installation.
- Quiesce the normal watcher at its acknowledged safe boundary and serialize
  with the existing deployment locks. Replace only the three named services
  with `--no-deps`. Preserve unrelated configuration, identities and services.
- Write a durable intent before switching. The scoped receipt explicitly says
  `getwork_acceptance=deferred` and `whole_application_release=false`. Keep the
  full release aliases and last-activated marker unchanged. The scoped marker
  blocks full approval and polling until explicit scoped rollback.
- Require readiness, public product/health routes, existing authentication and
  legacy-route rejection, and unauthenticated rejection at both new edit
  routes. Retry bounded readiness/DNS convergence; never broaden accepted
  final status codes to make a failed check pass. The exact same-origin
  `/quiz/` canonical 308 hop is allowed only when `/quiz` still returns 404.
  Require complete responses, allowing up to 60 seconds per request within a
  210-second total probe deadline shared across retries and the canonical hop;
  retain the 30-attempt ceiling. Response headers alone never pass acceptance.
- On failure restore only the retained three images with their original configuration.
  Preserve user writes; database restore is recovery evidence, not an automatic
  rollback of application data. Recheck every unselected container. A failed
  rollback remains recorded and blocks further full releases.
- Repeating a completed candidate verifies the existing scoped state without
  replacing containers. An interrupted intent requires explicit rollback;
  matching orphaned watcher quiescence can be recovered using the existing
  exact-SHA/instance/nonce protocol.

A verified successor may recover only an exact pinned predecessor
source/helper digest pair in failed or already completed rollback state. Preserve
the original helper identity and record the recovery helper digest; all
existing rollback gates remain mandatory. Other helper/source identities and
active receipts remain refused.

## Consequences

The version is a per-unit scoped release, not a full application release or a
claim that Career/getWork works. Before a later full release, roll back the
scoped units, restore the separately deferred runtime dependency, and use the
normal full-release path. Retain source, images and private receipts until
that reconciliation is complete.
