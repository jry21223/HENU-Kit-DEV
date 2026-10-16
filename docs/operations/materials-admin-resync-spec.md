# Materials administrative recovery specification

The accepted 454-material catalog must remain available while the approved
471-material revision is recoverable after its GitHub delivery expires.

The production CLI is `henukit-materials-control resync --sha SHA
--manifest-sha256 DIGEST --request-id UUID --operator LABEL --reason TEXT
--preflight|--execute`. `run` is the systemd queue-consumer entry. `reconcile`
accepts the same request ID, an operator/reason, `--outcome finish|rollback`,
and preflight/execute. Recovery never impersonates a GitHub push.

Acceptance seams are the installed control CLI, its shared process lock before
queue consumption, the explicit orchestration correlation, and the signed
runtime packaging/install contract. Tests must observe publication calls and
durable requests/results rather than private helper implementation.

Required behavior:

1. Preflight is read-only and rejects non-root/untrusted configuration,
   source or manifest drift, insufficient disk, active watcher/runner,
   queued/running/candidate work, unresolved admin work and activation fences.
2. Execute repeats preflight under the common lock; request IDs cannot be
   overwritten or replayed. Request audit records actual OS UID separately
   from the descriptive operator label and binds previous release/receipt.
3. Only a pinned current main manifest can proceed. Report public assets and
   review-only exclusions; preserve all existing content safety checks and
   complete publisher/activation gates. Bind the full manifest hash after seal
   and before any OSS Put, not only its shortened release ID prefix.
4. The webhook queue is never written by admin commands. Normal work cannot
   claim deliveries while an admin process or unresolved request exists.
5. Failure/process death leaves an unresolved request. Reconcile only uses
   the captured exact target or previous receipt with the existing activator;
   terminal resolution is durable before unlocking future work. A crash
   between resolution and unlock must converge without reactivation.
6. Package, checksum and install the control/helper through the existing
   signed runtime, with recoverable replaced-file backups and a fixed systemd
   entry. No public route, credential, IAM, preview or conversion is added.

Production source currently approved by the task is
`99e75e4838aae00ef5c10e803666189de4975886`, manifest SHA256
`644d91ce6dfb9b435fb8a1f6ef9236099361ecebbab25ab6ff05f8516c682300`:
471 public assets, 44 public subjects, and 39 review-only exclusions. This is
execution context, not a hard-coded application allowlist.
