# First full release from the retained mixed stack

Use this entry once to reconcile the retained Portal v3, membership and Practice
checkpoints into one whole-application release. Routine releases continue to use
`activate-henukit-release`; the automatic scheduler never invokes this transition.
This document is an implementation procedure, not evidence of a production run.

An independently accepted materials control plane may have a different version
from the full application alias; see [ADR-0059](../adr/0059-independent-materials-runtime-baseline.md).

## Source and retained authority

Require freshly fetched, merged `origin/main`, its newest complete successful
push workflow and exact-SHA runtime, all image checksums, the existing verified
OIDC manifest/bundle and `trusted_root.jsonl`. Use the existing reviewed trust
bootstrap for installed full-release guards. The transition verifies every
runtime file it executes or imports against the attested archive. Never run an
unmerged helper or copy candidate control-plane bytes around that bootstrap.

Keep the current full aliases and every scoped receipt intact. The entry accepts
only the active membership-aware Portal v3 chain over the retained Practice
release. It reconstructs a private mixed Compose configuration from each actual
owner's retained runtime, the original Practice Console application credential
and the membership Account/Nginx configuration. Current images, environment,
Compose hashes and mounts must match that authority.

Render the retained full layer with the unique, valid UTC
`PORTAL_DEPLOYED_AT` already recorded in the root application environment.
Practice and Portal overlays retain their own receipt timestamps. A scoped
release timestamp must not replace the full layer's metadata. The candidate
uses one timestamp consistently in its rendered configuration and receipt;
the environment identity remains guarded throughout preparation and recovery.

The candidate must preserve PostgreSQL/Redis configuration, membership Bot
credentials and routes, Practice Console credentials, and every owner's exact
migration inventory. Unknown services, schema differences, untrusted paths,
unfinished release state and insufficient disk space stop preflight. This is
not a schema migration or a grant-provisioning entry.

The selected cohort is the eighteen application services plus Nginx. PostgreSQL,
Redis, the getWork crawler and the WSL relay are outside that cohort. Career
starts with `CAREER_GETWORK_ENABLED=0`; scans must fail after authentication with
`503 JOB_SOURCE_DISABLED`. Retained profiles, history and resume tools remain.

## Preflight and execution

Prepare the verified artifacts under the existing private staging directory and
the verified runtime under `/opt/henukit-releases/<sha>`. Supply the existing
private proof directory containing the exact manifest, bundle and trust root.
Run the preflight as root, naming the exact reviewed tuple:

```bash
python3 /opt/henukit-releases/<sha>/bin/deploy-henukit-stack-transition.py \
  --sha <full-current-main-sha> \
  --run-id <newest-successful-main-run-id> \
  --proof-dir <root-owned-verified-proof-directory> \
  --acknowledge-getwork-disabled \
  --preflight
```

When production inspection proves a separately installed materials runtime,
name its complete baseline explicitly with all three additional arguments:

```text
--materials-baseline-sha 6008f76346fcc4dab1652505c371d62ebde3d30d
--materials-baseline-runtime /opt/henukit-releases/6008f76346fcc4dab1652505c371d62ebde3d30d
--materials-baseline-proof-dir /var/lib/henukit-release-proofs/materials-baseline-6008f76346fcc4dab1652505c371d62ebde3d30d
```

The private proof directory must retain the original runtime archive and
checksum, v1 manifest and attestation bundle, and a copy of the already approved
`trusted_root.jsonl`. Reverify the historical certificate's run/attempt and
source ancestry. The complete retained runtime and all installed materials
targets must match one baseline; a per-file fallback is prohibited. The
application aliases continue to identify the original application baseline.
Keep these arguments for execution; recovery binds the recorded selection and
refuses a conflicting baseline. No old deployment code is executed.

Review the actual preflight result and retained baseline. Use identical arguments
with `--execute` only after the repository's source, Account, workflow,
production configuration, operator-role and payment-gateway gates pass. A
missing prerequisite is a release failure; do not manufacture an approval or
change credentials to make preflight pass.

Execution obtains the existing direct-deployment and watcher locks and safely
quiesces the old watcher. It preserves private mixed/candidate configurations,
environment backup and a durable transaction record under
`/var/lib/henukit-scoped-releases/stack-transition/<sha>/`. It takes fresh owner
database backups and proves isolated restores, retaining the existing host
QuizCraft database identity and snapshot route. It never restores an old
database over production user data as an application rollback.

Materials runtime installation uses the real attested installer and its retained
before-images. The entry accepts only an already modern materials baseline;
legacy activation keys or retired source files require their separately reviewed
migration first. Materials publication state and files are not interchangeable
with application activation.

After image, configuration, health, public-route, authentication and disabled
scan acceptance, it advances both full aliases and the last-activated SHA. It
archives its owned scoped marker in the transaction audit. Original checkpoint
receipts remain byte-identical. Enable the [automatic scheduler](henukit-actions-auto-deploy.md)
only after those actual production results agree.

## Recovery

On failure, the entry restores the real mixed service images/configuration,
private environment, previous stopped states, materials runtime, full aliases
and owned scoped marker. The new rollback record owns any recreated container
IDs; historical receipts are never rewritten to claim those IDs.

Repeat the same tuple with `--rollback` for explicit recovery. An interrupted
nonterminal execution reconciles its owned transaction instead of starting a
second deployment. An already active tuple verifies its settled state without
reloading images. A rolled-back tuple requires inspection and a newly reviewed
candidate. Preserve all audits, backups and installer records; do not delete
them to force retry, and do not use the ordinary degraded-baseline recovery
entry over a mixed state.
