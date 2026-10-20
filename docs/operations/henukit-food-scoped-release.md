# Fixed-scope Food author-edit release

Use this path only with explicit authorization to publish author editing while
deferring getWork. It implements [ADR-0049](../adr/0049-food-author-edit-scoped-release.md).
It is operated from the existing Mac deployment environment with the existing
GitHub Actions Linux/amd64 build source; it does not require a WSL builder.

## Prerequisites

1. Review and merge the change, require all affected CI and the complete main
   release workflow, then freshly resolve the exact current main SHA/run.
2. Use the normal authenticated Actions artifact preparation and SHA256 gates
   from [the artifact runbook](henukit-artifact-deployment.md). Keep the full
   release approval absent. Never compile application code on production.
3. Use the runbook's OS GitHub CLI attestation bootstrap to verify the exact
   runtime before extracting or executing it. The existing getWork handoff
   attestation binds that runtime; this does not install or activate getWork.
   Retain the attestation bundle and trusted-root sidecar privately.
4. Through the reviewed root trust-helper bootstrap, atomically install only
   the runtime's updated `watch-henukit-actions.sh` and
   `activate-henukit-release.sh` at their existing root-managed destinations.
   Preserve owner/mode and a recoverable copy. The scoped preflight requires
   their bytes to match the attested runtime, so full/scoped coordination is
   present before any app replacement. The live watcher is subsequently
   quiesced through its existing handshake and resumed with the updated guard.

## Execute

Invoke only the helper from the provenance-verified root-owned runtime:

```bash
/usr/bin/python3 "$verified_runtime/bin/deploy-henukit-food-scoped.py" \
  --sha "$current_main_sha" --run-id "$successful_run_id" \
  --proof-dir "$verified_attestation_directory" \
  --acknowledge-getwork-deferred --preflight
```

Use the same identity and proof directory with `--execute` after preflight.
The root-owned, non-writable `approvals/consumed` and `approvals/revoked`
directories contain retained audit history. Preflight preserves them and rejects
untrusted substitutes, every other approval entry, and pending rollback records.
There is no service override, migration option, health bypass or permission
override. A test-root option is limited to non-root temporary fixtures whose
command adapters are inside that fixture; it is refused by a root process.

Execution backs up configuration and Food, restores the archive into an
isolated verification database, and loads only the selected images after their
archive/image identity checks. It records private intent, switches the three
explicit services, verifies readiness/authentication and unchanged unselected
containers, then commits the scoped receipt. Existing reverse proxy Docker DNS
must support service IP convergence; the bounded probes cover that seam.

Perform the user-visible edit acceptance from the operator's Mac browser using
only the authenticated author's own content. Cover initial values, validation,
photo retention/replacement, success and failure recovery as appropriate;
never use another person's Post as a production canary. Keep operational and
personal evidence private. Portal Summary metadata remains on the full baseline; the scoped receipt and
actual image identities identify this release. Report those separately from
the retained full-stack baseline and deferred getWork acceptance.

## Rollback and interrupted attempts

Run the same retained helper and identity with `--rollback`. It validates the
private receipt, retained image IDs, original Compose bytes, configuration and
full aliases, then restores only the three services with original configuration.
No application-data dump is automatically restored. A changed full release or
another scoped marker fails closed rather than overwriting another release.

Successful rollback removes the scoped marker and retains its private audit.
Repeated successful execution or rollback verifies rather than repeats the
container replacement. An interrupted `applying` record is not retried as a new
deployment: inspect it and explicitly roll back. Failure to verify rollback
keeps the marker and failed state for operator recovery.

The full approval entry and normal watcher pause while a scoped marker exists.
Do not remove that marker by hand or relabel a mixed runtime healthy. Once the
scoped units are rolled back and the deferred dependency is restored, proceed
through the normal full-release gates.
