---
status: proposed
amends: 0025, 0039
---

# Expired materials deliveries have an explicit root-admin recovery boundary

GitHub retains redeliverable deliveries for three days. A resolved disk or host
failure can therefore leave an approved source revision without a usable
webhook, while generic retry, queue editing and fabricated deliveries remain
prohibited.

## Decision

- Add a root-only, signed-runtime control command with `run`, `resync`, and
  `reconcile` modes. Normal queue consumption and administrative work share
  one trusted `flock` covering the complete operation. Normal `run` checks for
  an unresolved administrative request before invoking the queue consumer;
  it must not claim or change a queued delivery when recovery is pending.
  The binary also verifies the inherited lock before Store recovery/Claim.
  Root publication children inherit the descriptor; the unprivileged prepare
  process explicitly closes it. Runtime installation acquires
  the same lock before its first service/configuration/file mutation.
- `resync` is a separate authority, never a GitHub delivery. It binds an
  explicit request UUID, caller UID, descriptive operator label and reason,
  configured repository/ref, the current upstream SHA, full manifest digest,
  and previous complete release/receipt. Labels are not authentication facts.
  The operator must have separately verified fresh database backups and the
  retained rollback release using the existing release runbook.
- Read-only preflight checks the fixed source policy, current main, bounded
  exact-SHA manifest, public/review-only inventory, conservative disk peak,
  disabled/inactive watcher, idle runner, empty queue/candidate state and no
  maintenance fence or activation journal. Execute repeats those checks under
  the shared lock before creating any administrative intent.
- Execute records root-only, exclusive, fsynced request and stage evidence
  outside the webhook state tree. Final records are atomically published from
  fully written and fsynced temporary files without overwriting a destination. It invokes the existing prepare, independent
  seal, complete exact-version OSS publication and Library activation gates.
  Prepare/orchestrate use the explicit `--admin-request` correlation flag;
  no delivery is created and no receiver/queue metadata is edited.
- A persistent active request blocks normal consumption after failure or
  process death. It is not a replacement for the process lock. Reconcile can
  only complete the exact sealed target or fully reactivate the captured
  previous release through the existing activator. Unknown Library outcomes
  retain its fence/journal rules. A durable resolution and matching active
  release with no fence/journal are required before releasing the active
  request; operators must not delete it to make another attempt run.
- New code must merge into current main and pass normal review, CI, packaging,
  signing and production hash checks before installation. This decision adds
  no HTTP endpoint, service, account, credential, IAM capability, object ACL,
  converter, generic retry or unsigned production patch path.

## Consequences

The authenticated webhook remains the normal source authority. A root
administrator can explicitly authorize a current reviewed revision after its
delivery expires, with independent audit and the same publication guarantees.
Installing this capability does not itself authorize or prove a production
resync. Existing retained releases and database backups remain rollback gates.
