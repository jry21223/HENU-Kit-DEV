# Account membership release boundary

The active deployment has a retained full release plus an active Practice/Portal
successor chain. Publishing only the QQ entitlement reader must not roll back
that chain or activate the deferred Career/getWork services.

The candidate is current main from the newest successful Actions release run.
Retain Actions attestation, archive hashes, the 4096 MiB free-disk floor,
root-owned state/configuration, watcher safe-boundary locks, and verified
Account Portfolio backup/isolated restore. No schema change is permitted.

The fixed scope is Account Portfolio plus the existing Nginx container. Keep
its image, ports, networks and unrelated routes; mount a new root-owned copy of
the current Nginx configuration with only the exact Bot reader route inserted.
Never edit the retained full release's configuration file. Add only the three
independent Bot reader environment keys to Account Portfolio. Every other
container ID/image/start/restart state remains unchanged.

Record the predecessor receipt digest and both before/after Compose snapshots
before replacement. Rollback restores the original two service configurations
without restoring user data or deleting credentials. After any actual switch,
retain the membership marker with the verified final two-container checkpoint,
including after rollback. Keep its immutable predecessor Portal receipt linked
by digest; never restore a marker that falsely claims its old container IDs.
A pre-switch rollback may retain the original marker only while its original
selected snapshot still matches.

The current Portal-only successor helper intentionally rejects this new scope.
Future Portal releases must first add and review membership-checkpoint support;
do not bypass the marker or rewrite old receipts to make them proceed.
The retained full aliases and full-release SHA remain unchanged. A successful
scope receipt is not full-stack or getWork acceptance.

The owner image uses its own Actions OIDC attestation. Download its proof with
`gh attestation download <owner-archive> --repo jry21223/HENU-Kit-DEV
--predicate-type https://github.com/jry21223/HENU-Kit-DEV/attestations/account-membership-image-v1`
in the private proof directory; retain the resulting digest-named JSONL as
`henukit-membership-image-<sha>.attestation.json`. Verify repository, workflow,
source ref/digest, predicate and hosted-runner identity before use. Do not add
an extra Actions artifact for this proof: the existing full-release watcher
downloads every artifact and correctly rejects unrecognized files.
