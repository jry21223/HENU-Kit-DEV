# Automatic main releases

This opt-in path implements [ADR-0056](../adr/0056-automatic-actions-release-policy.md).
It schedules the existing full artifact activation entry. Production only
installs prebuilt artifacts; it does not compile repository source.

```text
successful main workflow_run -> HMAC receiver -> bounded notification
    -> systemd USR1 -> root scheduler -> independent GitHub API verification
    -> activate-henukit-release <exact SHA> --execute
    -> existing backup, approval, deployment, health and rollback gates
```

The scheduler also checks GitHub every 60 seconds. Notifications carry no root
authority and never select a command or approve a SHA.

The scheduler explicitly fixes the repository/workflow/branch environment for
the activator and carries the selected `actions:<run>:<attempt>` identity through
both the activation entry and watcher. A same-SHA rerun that changes this API
identity stops activation. These records describe the selected scheduling
intent, not the provenance of individual archives. The existing artifact
contract still verifies authenticated downloads, source SHA and checksums; its
SHA-keyed cache can reuse previously verified artifacts from the same SHA.
Do not report this scheduler audit as proof of an archive's exact run attempt.

## Before installation

Require the implementation merged into freshly fetched main, affected CI and
the complete release build green, and an exact-source runtime verified through
the provenance/trust bootstrap in [the artifact runbook](henukit-artifact-deployment.md).
Keep recoverable copies, owner/mode and hashes of every replaced binary, helper,
unit and Nginx configuration. Preserve the existing webhook secret.

Do not enable automatic full activation while a scoped marker or pending
rollback exists. The current membership-aware Portal release is mixed with
other independently released services. Use the reviewed
[first full-stack transition](henukit-stack-transition.md). Preserve its receipts,
Practice Console credentials and membership Nginx mount until that transition establishes a real full
rollback baseline. Never delete the marker, relabel mixed images healthy, or
invoke ordinary degraded recovery over that mixed state.

Set `CAREER_GETWORK_ENABLED=0` for the operator-requested disabled job-source
mode. Career remains available for retained history, profiles and resume tools;
new scans return `503 JOB_SOURCE_DISABLED` after authentication. The primary
runtime omits both crawler and relay, so unavailable WSL networking does not
block that mode. Keep every other service, schema, secret, backup and disk gate.
Changing the source mode back to `1` requires a separately verified relay and
credential contract. Do not report a disabled source as a successful scan.

## Install from a verified runtime

Install `bin/auto-deploy-henukit-actions.sh` at
`/usr/local/sbin/auto-deploy-henukit-actions` with root ownership and mode 0555.
Install the prebuilt `materials-runtime/bin/henukit-deploy-webhook` at the existing
`/usr/local/bin/henukit-deploy-webhook`. Install these runtime units at their
matching `/etc/systemd/system/` paths with root ownership and mode 0644:

- `henukit-actions-auto-deploy.service`
- `henukit-actions-webhook-receiver.service`
- `henukit-actions-webhook.path`
- `henukit-actions-webhook.service`

The receiver reuses the existing non-login `henukit-deploy` account and
`/etc/henukit-deploy/webhook-secret` through systemd credentials. Its new
loopback port is 10091 and its independent state directory is
`/var/lib/henukit-actions-webhook`. Keep the old FIFO and materials state intact.

The scheduler reuses `/etc/henukit/actions-watch.env`, the existing Actions read
token and full activation helper. Its unit allows read-only root home access
because the existing activation entry requires the approved MetaView SSH trust
and key; no key is copied into the artifact. A usable root SSH channel and the
approved gateway directory remain prerequisites, as in the original runbook.

Create `/etc/henukit/auto-deploy-policy.json` as a regular root-owned file at
0600 or 0400, under root-owned non-writable ancestor directories:

```json
{
  "format": "henukit-actions-auto-deploy-v1",
  "repository": "jry21223/HENU-Kit-DEV",
  "workflow": "deploy-henukit.yml",
  "branch": "main"
}
```

This policy explicitly authorizes routine current-main automatic attempts.
Omitting it, changing its identity fields, using a symlink, or weakening owner
and permissions fails closed. The scheduler itself must run as root. Its state
and the existing watcher state must be private root-owned directories at 0700.

## Switch scheduling and the webhook

After the reviewed scoped transition and all infrastructure gates pass, quiesce
the existing `henukit-actions-watch.service` through its instance/nonce handshake
at a safe boundary, then disable it. Do not stop an active deployment. The new
scheduler refuses to run a release while the old watcher is active or ambiguous.
Use the existing direct-deployment lock for operator releases.

Start the new receiver and verify its loopback readiness. In the `henukit.cn` HTTPS
vhost change only the exact `/webhooks/github` upstream from port 10090
to 10091. Keep TLS verification and the original secret; validate Nginx before
reloading, and keep operational status loopback-only.

The canonical hook URL is `https://henukit.cn/webhooks/github`; the scheduler's
`HENUKIT_PUBLIC_BASE_URL` is `https://henukit.cn`. Verify both the GitHub hook
configuration and the running scheduler environment after changing them.

Change the existing GitHub hook subscription from `push` to `workflow_run`.
Use GitHub's Ping delivery first. The receiver accepts only `completed/success`
main push runs of `.github/workflows/deploy-henukit.yml`, with both repository
identities matching. Other workflows, PR builds and unsuccessful runs do not
notify the scheduler. A valid notification means only that a wake-up was saved.

After `systemctl daemon-reload`, enable the new controller, receiver and path.
Verify a real completed workflow delivery, its receiver journal, the scheduler's
independent run/SHA selection, and the actual full release result. Require the
active aliases, service image set, full activation marker, mandatory product
checks and private release evidence to agree. Do not call Ping or HTTP 202 a
deployment.

### Active aliases are part of the existing activation transaction

The versioned topology is a fixed, root-owned outer symlink
`/opt/henukit-current → /opt/henukit/current` and a mutable, root-owned inner
symlink `/opt/henukit/current → /opt/henukit-releases/<accepted SHA>`.
Both paths must resolve to the extracted fixed-SHA directory whose `RELEASE_SHA`
agrees with the running release and full activation marker. The watcher and
stack transition update only the inner link. This keeps the existing scheduler's
`ProtectSystem=strict` and writable `/opt/henukit` boundary; it never requests
writable access to the parent `/opt`.

An operator-installed, regular root-owned `0400` file at
`<state>/active-alias-contract.json` is mandatory. Its exact schema is:

```text
format: henukit-active-alias-forwarder-v1
release_root: absolute canonical release directory
inner: absolute canonical mutable link path
outer: absolute canonical fixed link path
outer_snapshot: dev, ino, uid, gid, mode and exact raw text of the outer symlink
parents: exact dev, ino, uid, gid and mode for both link parents
```

No command in this release bootstraps, adopts or repairs that contract. Missing
authority, another schema, two legacy direct links, replaced outer inodes or
changed protected parents stop activation. The activator checks the contract
before remote gateway/environment/approval writes; the scheduler checks it before
recording a new attempt. A marker alone cannot justify an already-active return
or success audit. These checks do not enable another publisher, retry a failed
automatic attempt, or consume the retired source queue.

The existing automatic path retains `direct-deploy.lock → watcher.lock` order.
Within the watcher, the state-directory FD (shared with baseline adoption) and
the original source-deployment protocol lock are held until the invocation
finishes. Do not acquire the direct lock again after the watcher lock. The
source config remains `/etc/henukit-deploy/deploy.env`; its literal lock setting
must agree with `/run/lock/henukit-deploy.lock`. That existing empty lock must
keep the deployed `henukit-deploy` account's owner/group and mode `0640`.
The watcher and stack transition check and hold it; neither creates, unlinks,
chowns or repairs it.
If the protocol file, account, config, or its identity is unavailable, stop and
resolve that readiness failure through the original operator procedure. Do not
start the retired runner or edit its queue to make the check pass. Python 3 is
required; the private transaction runs in isolated interpreter mode.

The original eight-line rollback contract remains unchanged. A separate private
`<state>/active-aliases/<candidate>/<contract-identity>/` journal binds its exact
inode/hash, candidate/previous release authority, watcher source, source lock,
versioned topology contract identity/hash and the original inner snapshot.
The original rollback contract file and its parent directories are synchronized
before the alias intent. The exact original approval inode/content is bound to
that intent. Consumption synchronizes the consumed file and both approval
directories, then durably records `activation-started` before image loads or
the deployment helper. A newly created same-SHA approval cannot replay a pending
attempt whose original approval has already been consumed.

Each temporary inner symlink inode is recorded and synchronized **before** its
atomic replacement; the parent is synchronized on initial execution and resume.
One inner rename makes both resolved paths change together. Only after runtime
acceptance and durable alias publication can the full activation marker be
atomically written/synchronized, the rollback contract complete, and the
scheduler publish success. The stack transition keeps its original receipt and
adds private prepared-inode journals for its inner commit and rollback; it pins
the same topology authority and preserves the fixed outer inode.

For an interruption, first read the original pending contract, journal and live
runtime. Use the existing reviewed watcher reconciliation path for that attempt;
do not rerun a build or issue a new activation to guess its outcome. A candidate
already running can finish its owned metadata without another deployment.
Third targets, foreign inodes and changed authority stop reconciliation and
retain the evidence. A new exact-SHA approval after a completed rollback obtains
a distinct contract identity; it does not overwrite the old journal. An already
published unresolved attempt must be reconciled before a new approval can cause
another deployment.

A normal rollback verifies the actual previous runtime, restores only the inner
link owned by that transaction and records the verified previous marker. Explicit
degraded-baseline recovery keeps its original weaker *health* claim: it restores
the known image/link baseline and preserves the prior full-activation marker;
it does not promote an unhealthy baseline to a successful full activation.
Historical failed automatic audits remain immutable after manual reconciliation.
Report the subsequent journal/rollback outcome as additional evidence rather
than relabeling the failed scheduling attempt.

### Legacy migration is a separate operator action

Installing these source files alone does not migrate a live direct-link setup.
Before any production bootstrap, review and authorize the topology change,
inspect the installed service/drop-ins and every alias writer, reconcile all
pending full/scoped work, and prove the current runtime/marker/direct links agree.
Use the original direct → watcher → directory → source leases. A separately
reviewed bootstrap must persist the original outer target/identity and prepared
replacement inode, replace only that owned outer link, synchronize `/opt`, then
install and synchronize the immutable contract under the existing private state
directory. Runtime containers and the inner link remain unchanged. Do not modify
the service sandbox, directory ownership/mode, credentials or old source queue.

Before a subsequent activation, an owned bootstrap may be reverted only by its
saved exact inode/hash proof; preserve foreign objects and stop. After activation,
normal runtime rollback preserves the fixed outer link and restores the verified
previous inner target/marker. Do not use bootstrap rollback to undo later work.
Historical direct-link acceptance records and their validator remain historical;
accept the new topology under its distinct versioned contract. Unresolved legacy
transition receipts must be reconciled before bootstrap, not silently rewritten
to claim the new authority.

The existing explicit `--recover-transition` for the originally pinned failed
transition remains available **before** migration. It retains the original
receipt/helper SHA, attested historical bytes, successor Actions/recovery proof
and all original rollback gates. It may restore the original two protected
direct links only when the versioned contract is truly absent (including no
dangling symlink) and neither `<state>/active-alias-bootstrap` nor the new
`<state>/active-aliases` journal path exists. Any present, malformed, dangling or
started migration authority blocks this legacy writer. Ordinary activation never
uses this fallback, and no historical receipt gains fabricated new fields.

The separately approved bootstrap must establish the fixed private
`<state>/active-alias-bootstrap` path and durably record its intent and prepared
inode before the first outer rename. Write the terminal receipt in that same
path only after the outer link and contract are synchronized and live acceptance
passes. The path is a downgrade fence even if the contract is
later missing. Do not remove it to enable legacy recovery. The earlier one-time
metadata-reconciliation records under `alias-completions` preserve the old direct
topology; they are not a forwarder bootstrap. New normal and rollback writers
always retain the fixed outer inode and use their own exact prepared inner inode.

After an audited legacy recovery completes, a normal new preflight may verify
that historical terminal evidence read-only under the new contract. It still
verifies the pinned helper, original recovery audit/proof, actual restored runtime
and complete current alias chain. The new intent links the old receipt/audit
hashes to the current topology contract identity and inner snapshot. This verifier
cannot execute recovery writes or change the old receipt; the legacy writer
remains unavailable after migration.

Permission regression tests probe the actual fixture filesystem before setting
SUID/SGID. A Mac filesystem that immediately strips a requested special bit can
report that specific case as unavailable; this is not a passing security check.
Linux and CI must support and run both cases, or fail the gate. The normal mode,
foreign-byte, backup and proof checks still run on the Mac. Production acceptance
requires the full supported-filesystem gate; local skips cannot authorize release.

## Failure, interruption and shutdown

The scheduler synchronizes immutable `<sha>.attempted.json` before calling the
activator and records `<sha>.activated.json` or `<sha>.failed.json` afterward.
These root-only audits bind the selected run/attempt, policy and
activation-helper hashes, explicitly labelled `scheduling_intent`.
A crash after the first record suppresses automatic retry, even if no terminal
record exists. Preserve these records. Do not delete them to force a retry.
Review the existing rollback/approval state and use the existing explicit
activation command for an operator retry, or publish a newly reviewed main SHA.

Changing the policy does not stop an already started activation. A normal
`systemctl stop henukit-actions-auto-deploy.service` waits for the current
activation/rollback to finish because the unit signals only the scheduler and
has no forced stop timeout. Do not add a timeout that kills a release halfway.
After it has stopped, the new path can be disabled and the prior scheduler
restored only after any pending release state is reconciled.

No production install or end-to-end deployment is claimed by this document.
The disabled source mode and reviewed mixed-baseline transition must be
verified before this setup is enabled on production.
