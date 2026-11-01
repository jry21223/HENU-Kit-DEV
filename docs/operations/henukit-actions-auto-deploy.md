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
