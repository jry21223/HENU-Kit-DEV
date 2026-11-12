# Preserve existing release authority

Issue #645, prerequisite to #640. This is a mode of the original full
`activate-henukit-release` owner and its watcher. It is not an installer,
credential provisioner, scoped bootstrap or second publisher. No production
operation was performed to develop this change.

The 2026-11-12 07:27 UTC read-only baseline recorded both forwarder aliases and
all 18 application images at `c81827f5b2ca4c40cdca3b41ca9badf0fb75730f`, 21
containers, no direct/watcher lock holder and an active automatic scheduler.
That observation does not reserve a later release window.

## Contract

After the updated long-lived helpers have been installed and checked, invoke
only the existing entry with its explicit option:

```bash
sudo HENUKIT_ENV_FILE=/opt/henukit/.env.henukit \
  GH_TOKEN_FILE=/etc/henukit/github-actions-read.token \
  HENUKIT_ACCOUNT_OPERATOR_ROLE_CODE=operations-operator \
  /usr/local/sbin/activate-henukit-release <full-current-main-sha> \
    --preserve-existing --execute
```

Use the already configured operator role; the mode adds no role default or
persistent setting. Signed local bundles can use the original entry's
`--local-artifacts <verified-bundle>` before `--preserve-existing --execute`,
after the existing WSL/trust/transfer gates. The current WSL transport does not
forward this option; do not use its default execute path for a preservation
release. No alternate transfer or publisher is introduced here.

The initial check validates the existing environment, declared Compose service
environments and their actual running values, the selected active role and its
eight already-granted Account permissions. Role revision, that role's bindings
and grant audit are read in a PostgreSQL repeatable-read, read-only transaction.
It reads no other role's bindings or grants. Existing file paths, device/inode,
owner/group/mode, size/timestamps and content digests are bound in a root-only
0400 `<state>/prepared/<sha>.preserve-existing.json`. It contains digests and
identity metadata, never credential values or copies of credential files.

The environment, existing GitHub token, original source deployment config,
webhook secret and materials activation config stay at their original paths.
Source config's existing protected 0640 profile remains valid; secrets require
0400 or 0600. Legacy materials credential paths or retired activation keys
block this mode before preparation, preventing the binary installer from
retiring or rewriting those sources. Untracked Compose secrets/configs or bind
sources are rejected; read-only artifact `infra` files and the existing protected `/opt/henukit-materials/public` directory mount are accepted. The public directory must already exist and its identity is bound.

The candidate must declare exactly the same effective service environments as
the accepted baseline, except release SHA/version/deployment timestamps. Ambient
Compose overrides must produce identical configuration. Existing invalid flags,
missing keys, changed services, new credential sources or configuration drift
stop the release; this mode does not fill missing configuration. The baseline
and candidate patch manifests must agree with the installed protected EasyPay
patch marker. Via the established SSH trust, a read-only probe compares the
existing enabled tenant pair's digest and required allowlists and checks loopback
health. It returns only remote file identity/digests, never the tenant pair.

No `set_account_env_value`, tenant-fetch shell helper, environment backup/copy,
EasyPay patch installer, owner grant, key generator or firewall write runs in
this mode. Existing relay firewall restrictions are verified read-only when
that service is enabled. A missing restriction is a blocker, not a repair path.
The normal binary-only runtime installation and its program backups remain.

The watcher rechecks preservation before preparation, candidate backup,
approval consumption, final runtime acceptance and completed rollback. The
same original environment is the rollback input; it is never restored by
renaming a copy over the original. The original prepared backup metadata carries the receipt binding before
approval; the existing eight-line rollback contract carries a
`preserve-existing:<receipt-identity-digest>` mode value after rollback capture.
This digest binds the receipt's original device/inode, owner/group/mode and
content. Those original attempt authorities select preservation on restart,
even without the process environment or receipt. Missing or same-byte replaced
receipts fail closed instead of falling back to grant. A normal activator invocation cannot downgrade an existing
preservation attempt. Changed authority stops reconciliation for operator
review; it never attempts an old credential-mutating owner to make progress.

All original current-main, newest successful Actions identity, signature and
artifact checks, runtime boundary/provenance checks, exact approval, fresh
backup/isolated restore, migration, disk, source lease, materials state, health,
forwarder aliases and rollback gates remain in their original owner. This mode
requires a healthy accepted full-stack baseline; it cannot combine Notice URL
changes, degraded recovery or ownership adoption. Normal post-release product
acceptance is still required.

## Make the mode effective before automatic publication

Merging this PR while the old scheduler is active is unsafe. The existing
scheduler invokes the installed activator; candidate runtime extraction does
not update that long-lived owner. The watcher verifies the artifact before
executing the candidate's `bin/deploy-henukit-artifact.sh`, but that candidate
helper neither replaces the installed watcher/activator nor protects their
old pre-activation credential writes. The updated scheduler explicitly invokes
`--preserve-existing`; installing only the candidate application is insufficient.

Use the original operation boundaries, under the orchestrator's existing normal
release authority. The following are rollout requirements, not actions taken by
this ticket:

1. Recheck aliases, full baseline, controller state, held locks and pending
   approval/rollback journals. Reconcile any existing attempt first. The
   [automatic runbook](henukit-actions-auto-deploy.md#failure-interruption-and-shutdown)
   already specifies: “A normal `systemctl stop
   henukit-actions-auto-deploy.service` waits for the current activation/rollback
   to finish”. Stop at that boundary and prove the scheduler has exited before
   merging this prerequisite. Keep the old watcher inactive. Stopping an idle
   controller is an ordinary release-window operation; no policy, credential,
   service sandbox, host trust or permission change is needed.
2. While scheduling is held, review and merge this isolated prerequisite, then
   fetch the exact new current main and require the complete Linux release CI.
   Obtain its verified fixed artifacts through the original runbook. Never
   execute this unmerged branch on production or install from an unverified
   extraction. If preparation fails, keep scheduling held; do not restart the
   old mutating owner.
3. Update the long-lived programs through the existing reviewed software
   installation boundary. The artifact runbook's Actions bootstrap permits
   installation “from a verified GitHub Actions runtime artifact”. Its local
   fallback instead requires the separate reviewed exact-main checkout and
   independently approved SHA256 record, and explicitly forbids copying trust
   roots out of a candidate bundle. Follow the applicable source rule; these
   are not interchangeable. Retain recoverable copies and hashes of the old
   *programs*. Install and verify, in this order, root:root mode 0555:

   | Exact source basename | Existing/new installed program path |
   | --- | --- |
   | `henukit-preserve-existing.py` | `/usr/local/sbin/henukit-preserve-existing.py` (new verifier program) |
   | `watch-henukit-actions.sh` | `/usr/local/sbin/watch-henukit-actions` |
   | `activate-henukit-release.sh` | `/usr/local/sbin/activate-henukit-release` |
   | `auto-deploy-henukit-actions.sh` | `/usr/local/sbin/auto-deploy-henukit-actions` |

   The verifier must be beside the two owners. Extend the exact-main approved
   program hash list with that basename; do not change allowed signers, signing
   keys, SSH keys/known_hosts, tokens, policies, environment files, permissions,
   units/drop-ins or forwarder authority. This is a software update using
   existing trust, not signer rotation or security provisioning. The original
   [bootstrap](henukit-artifact-deployment.md#out-of-band-local-trust-root-bootstrap)
   and [scheduler install](henukit-actions-auto-deploy.md#install-from-a-verified-runtime)
   already own installation of long-lived programs; this change adds one
   verifier and an explicit invocation mode to that same boundary.
4. Verify all four installed program hashes against the reviewed exact-source
   record, with existing owners/modes and paths, and prove the installed
   scheduler invokes the updated activator with `--preserve-existing`.
   Recheck readiness using only existing authorities. Then restart the same
   automatic scheduler through the original procedure, or use the original
   direct lock/owner for one exact activation. No persistent mode flag,
   independent timer, new policy, alternate service or publisher is needed.
   Only after this installation/verification checkpoint may product merges
   resume. Keep exact hashes, drained state and final mode in release evidence.

If any required source verification, existing authority, readiness or installation
permission is unavailable, keep publication held and report the exact missing
operation. No credential/security mutation is an acceptable workaround. Merely
passing local tests or CI does not establish that the production owner changed.

## Verification

The existing Node activator/watcher/scheduler contracts run alongside
`python3 -m unittest discover -s scripts/ops/tests -p test_preserve_existing_release.py`.
They execute the real scripts with bounded Docker/SSH fixtures and real files,
including successful activation, failed preparation, rollback, file replacement,
missing grants and effective configuration drift. Linux CI remains necessary for
privileged/foreign-owner and actual filesystem permission cases unavailable on
macOS. No production acceptance is claimed by these local fixtures.

Native QuizCraft schema releases additionally require the same-owner
[Core backup and compatible-recovery gates](henukit-quizcraft-schema-release.md).
The original environment file and preservation receipt remain their authority;
no new credential or runtime enablement is permitted.
