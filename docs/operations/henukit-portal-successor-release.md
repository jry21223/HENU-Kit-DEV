# Portal successor over the active Practice cohort

This implements [ADR-0053](../adr/0053-portal-successor-release.md) and #612.
Read [the artifact runbook](henukit-artifact-deployment.md) and the existing
[Practice runbook](henukit-practice-scoped-release.md) first. This releases only
Portal; it does not deploy backend changes or accept deferred Career/getWork.

## Prepare trusted exact-source artifacts

Require a clean source tree freshly synchronized to current `origin/main`,
the reviewed implementation merged, and the newest exact-main `push` release
workflow/attempt entirely successful. Download only that run's Portal image,
runtime and getWork provenance artifacts through authenticated GitHub Actions.
Capture GitHub artifact IDs, sizes and ZIP digests, verify the authenticated
ZIP bytes and safe flat entries, then verify the extracted archive checksums.
Never substitute PR, local, failed-run, or mixed-SHA artifacts.

Use the artifact runbook's root-owned OS GitHub CLI attestation bootstrap
before extracting or executing any runtime. Verify the manifest's signature,
workflow, repository, main ref and exact source digest, then its runtime hash.
Validate runtime archive paths/types/duplicates before extraction into a new
root-owned private runtime directory. Retain the manifest, bundle and trusted
root sidecar privately. Verify the Portal tag and Linux/amd64 archive identity.
The existing provenance envelope binds the runtime; authenticated exact-run
artifact download binds the Portal image. Do not claim the getWork manifest
signs the Portal image.

The runtime must include `deploy-henukit-portal-successor.py` and its Food engine.
Verify both against the attested archive. Existing installed watcher and full
activation guards must match that runtime. If they differ, use only the
reviewed trust-helper bootstrap with recoverable root-owned copies. Do not
change a production guard merely to bypass a failed preflight.

Stage archives/checksums at `/opt/henukit-staging/<sha>` with trusted ownership
and modes. Preserve at least 4096 MiB free after staging and image loading.
No production builds, source compilation, database commands or package
installation are part of this entry.

## Preflight and activate

Capture the full aliases/marker, active Practice marker/receipt, all container
identities/states, current environment hash (never values), watcher state and
available disk. The Practice receipt must still identify its exact five active
images and retained helper/engine/Compose. The actual Portal environment and
Compose hash must agree with the retained Practice runtime rendered using
the current environment. An historical SMTP hash difference is not repaired.

Invoke only the provenance-verified runtime entry:

```bash
/usr/bin/python3 "$verified_runtime/bin/deploy-henukit-portal-successor.py" \
  --sha "$current_main_sha" --run-id "$successful_run_id" \
  --proof-dir "$verified_attestation_directory" \
  --acknowledge-getwork-deferred --preflight
```

Preflight creates no receipt or backup and loads/replaces no image. It checks
the current source/provenance, retained rollback image and configuration,
compatible Portal/shared resources, empty pending full-release queues,
healthy non-deferred services and stopped deferred services.

Repeat the same arguments with `--execute`. The entry rechecks under the safe
watcher handshake and both locks, durably backs up the current environment,
loads only the Portal image, verifies its archive ID/platform, then atomically
advances the shared scoped marker and runs `up -d --no-deps portal`.
The new root-private receipt is under
`/var/lib/henukit-scoped-releases/portal-successor/<sha>`.

Require the new Portal image/configuration, byte-identical current environment,
full aliases/marker, unchanged other containers, and public homepage, Library,
Food, Practice/catalog, account and legacy rejection probes. Independently
verify the actual mobile pages at 375px and 390px: Library batches/filter/detail
return, all five Practice tabs and touch targets, Food first shop/campus switch,
retained homepage explanation, reduced-motion skeletons and horizontal fit.
Record browser viewport simulation separately from real-device acceptance.

## Rollback and interrupted attempts

On mandatory activation failure the entry restores only the immediately
previous Portal image, using the retained Practice Compose and unchanged
current environment, verifies health/configuration, and atomically restores
the old Practice marker. It leaves the original receipt and all other
containers intact. A nonzero activation result remains a failed release even
if rollback succeeds.

Use the identical candidate runtime/arguments with `--rollback` for an
interrupted applying attempt or failed independent acceptance. Retain that
runtime, the predecessor runtime/image, receipt and current environment backup.
A repeated rollback verifies the restored state without replacing again.
The current environment, full baseline, predecessor receipt/Compose and
unselected containers must still match the successor's captured baseline.
Foreign marker or drift fails closed; do not rewrite receipts, replace the
marker by hand, or restore old mail/database state to make rollback pass.
`rollback_failed` requires explicit read-only reconciliation. The existing
scoped marker keeps the full watcher from releasing over unresolved state.

After successful acceptance record the exact Portal image, active successor
receipt/marker, retained full baseline, backups and mobile evidence. Do not
describe this as a full-stack or Career/getWork release. A later full release
must first reconcile scoped state through its reviewed release path.
