# Fixed Practice and Console bridge release

This path implements [ADR-0051](../adr/0051-practice-scoped-release.md) for the
fixed `portal`, `portal-gateway`, `food`, `console-gateway`, `quizcraft` cohort.
The new Portal exposes author editing, requiring the same-candidate Portal
Gateway and Food contracts (ADR-0049). This five-service dependency closure
keeps the existing feature usable while releasing Practice. It runs from the existing
Mac deployment environment using authenticated GitHub Actions Linux/amd64
artifacts. Review the [artifact runbook](henukit-artifact-deployment.md) first.
Keep getWork acceptance deferred and do not create a full-release approval.

## Source and trust

Merge the reviewed implementation, require all affected CI and the complete
main release workflow, freshly resolve current `origin/main`, and select the
newest successful `push` run and attempt at exactly that SHA. Prepare the exact
Actions artifact set through the artifact runbook. Use the root-owned OS
GitHub CLI attestation bootstrap before extracting or executing any runtime.
Retain the getWork Actions manifest, attestation and trusted-root sidecar
privately; this provenance envelope does not activate getWork.

The candidate runtime contains both scoped Python helpers, both full-release
coordination guards, and the complete QuizCraft up/down migration source. The
Practice entry verifies its own bytes and the imported Food engine against the
attested runtime. The imported engine is checked for trusted ownership and
parent permissions before import. Use the reviewed trust-helper bootstrap to
install only the exact attested `watch-henukit-actions.sh` and
`activate-henukit-release.sh` at their existing root-managed paths if necessary.
Preserve ownership, mode and recoverable copies. Never invoke a local edited
helper on production.

## Preflight and execution

Run from the provenance-verified root-owned runtime:

```bash
/usr/bin/python3 "$verified_runtime/bin/deploy-henukit-practice-scoped.py" \
  --sha "$current_main_sha" --run-id "$successful_run_id" \
  --proof-dir "$verified_attestation_directory" \
  --acknowledge-getwork-deferred --preflight
```

Preflight performs no application/database mutation and creates no receipt or
application key. It verifies the current full aliases/marker, all selected
baseline images, healthy running unselected containers, stopped Career/getWork
containers, empty pending approvals/rollback queue, disk capacity, installed
guards and retained rollback images. Historic consumed/revoked approvals stay
intact. The shared `scoped-food-release` marker prevents an installed watcher
or full activation from overlooking either fixed cohort.

Candidate Compose may add only the reviewed QuizCraft Console environment keys.
The preflight render uses a safe placeholder for the dedicated application key;
all other selected settings and root/shared resources must equal baseline.
Baseline management keys must be absent. Running baseline environments and
Compose config hashes must match retained Compose. Caller-provided `PG*` and
`QUIZCRAFT_CONSOLE_*` environment values are ignored.

QuizCraft must use its existing `host.docker.internal` Docker host-gateway
route to the host PostgreSQL 16 `quizcraft_v2` database. The helper derives only
the host portion for gateway and loopback probes (using the trusted Docker
container hosts file bound to its exact ID); credentials, database, port,
and query remain identical. The gateway probe must originate in the running
QuizCraft container's network namespace: a host-origin connection has a different
source address and can legitimately be rejected by the existing PostgreSQL HBA.
The helper opens and pins that exact PID's network namespace, verifies its kernel
network type and device/inode agreement with Docker's retained `SandboxKey`, and
passes only that FD to trusted root-owned `/usr/bin/nsenter`. Only the network
namespace is entered; the installed host PG16 `psql` executes the read-only
identity query with the same owner environment. No container executable is used.
Container ID, PID, start time, running state, network settings and both namespace
bindings must remain unchanged before and after the query; drift fails closed.
It compares the owner's container-origin gateway connection, host loopback
connection and peer-authenticated existing postgres admin connection using
database OID, server startup, port, database and version. Snapshot/dump stay on
the proven host loopback route and isolated restore uses the existing admin.
For QuizCraft, only installed `/usr/lib/postgresql/16/bin` tools,
`/usr/bin/nsenter` and `/usr/sbin/runuser` are used. The Docker
PostgreSQL service is not this database. No database role, password, grant,
account, HBA rule or host configuration is changed. A failed route proof must
not be bypassed or repaired by changing production authentication rules.

The exact GitHub Contents inventories and Git blob hashes of every QuizCraft
migration must match between baseline and candidate commits. The packaged
source must match that inventory, blob hashes and attested archive bytes. The
baseline runtime need not package QuizCraft migrations. Food's conservative
unchanged-migration gate also remains. There is no migration option.

Repeat the identical arguments with `--execute` only after preflight and the
release evidence are reviewed. Execution safely quiesces the watcher and holds
both existing direct-deploy and watcher locks, then:

1. Copies and hashes the global environment privately exactly once without
   editing it. Reuses the Food engine's binding of the running Food URL to its
   container PostgreSQL authority, custom-format dump and isolated restore.
   Food's `food.dump` hash/bytes/restored post count are durably retained under
   `database_backups.food`. A failure stops before any application key or image
   load. The Food container database/tools are PostgreSQL 17; they are separate
   from QuizCraft's host PostgreSQL 16.
2. Exports a read-only repeatable-read QuizCraft owner snapshot, dumps that exact snapshot,
   enumerates the archive, restores through the existing postgres admin into a
   unique `henukit_practice_verify_<sha-prefix>_<random>` database, and compares
   bank/version/question/membership/session/attempt/audit counts with the same
   snapshot. Only that unique verification database is dropped. Student writes
   after the snapshot are retained. Archive bytes, hash and restored counts are
   recorded privately under `database_backups.quizcraft`, without overwriting
   Food's proof or the configuration backup.
3. Generates one dedicated application HMAC secret only after both isolated
   restores and their durable evidence pass,
   writes `quizcraft-console-application.json` root-owned `0600` inside the
   private receipt directory, and binds its digest. It is neither a signing key
   nor a Session/OAuth credential. Candidate-only Compose environment enables
   the bridge with client `console-gateway-quizcraft-management`, key ID
   `quizcraft-console-management-v1`, Gateway endpoint `http://quizcraft:10089`,
   and the same dedicated secret in the Owner key ring. The global
   `/opt/henukit/.env.henukit` stays byte-identical.
4. Loads only the five fixed-SHA images after archive/tag/platform/OCI identity
   checks, durably records intent and marker, and runs `compose up -d --no-deps`
   for exactly these five services.
5. Verifies image identities, effective private environment and Compose hashes,
   unchanged unselected containers/full aliases/global environment, public
   Practice/catalog and legacy `404` rejection, and unauthenticated
   `https://console.henukit.cn/api/v1/quizcraft/workshop/catalog` rejection `401`.
   The Owner internal management catalog must also reject an unsigned direct
   request with `401` at its private Docker address. Both unauthenticated Food
   author-edit GET and update PUT must return `401` through Portal Gateway.

The Owner's `/api/v1/console/workshop/catalog` still requires the dedicated
service HMAC plus verified actor/permission context. This release path does not
synthesize Console Sessions, JWTs, actors or permission grants. Content import,
readback, validate and publish must subsequently use the actual operator's
normal Console cookie through the Console Gateway. No SQL content publication
is part of deployment. Complete real authenticated content and browser
acceptance separately before reporting the H3C banks live.

Also perform real author-side Food acceptance using only the operator's own
post through the normal Portal Session: existing list/detail/create behavior,
the edit-read response and initial form values, then a reviewed own-post save
with version checks and persisted readback. Never use another person's post as
a canary. Unauthenticated `401` proves the new route boundary, but it does not
replace this authenticated journey.

The private receipt is
`/var/lib/henukit-scoped-releases/practice-console/<sha>/receipt.json`, format
`henukit-practice-scoped-v1`. It binds both helper hashes, Compose hashes, source
run/attempt, actual five-service image IDs, DB identity, migration blob inventory,
both distinct database backups,
application-secret-file digest, effective-configuration digest and deferred
getWork status. Never print the secret file, container environment or full
rendered Compose. Portal Summary remains on the full baseline; report that
baseline separately from the Practice receipt and actual cohort image SHAs.

## Rollback and interruption

Use the same verified helper and exact SHA/run/proof tuple with `--rollback`.
Both helper hashes, retained Compose/image identity, global environment, aliases
and private application file are pinned. Restore only the five baseline images
with their original effective configuration; the candidate application key is
not injected into baseline Compose. Retain the secret and both database archives
privately for audit. An old database is never automatically restored over new
student attempts.

A failed mandatory activation check triggers this same scoped rollback. A
successful rollback removes only its own marker and retains all audit records.
An unselected-container or environment change fails closed rather than being
repaired by this helper. Failed rollback retains marker and `rollback_failed`
receipt for operator recovery. Do not remove the marker or edit receipts.

Repeated active execution and terminal rollback verify existing state without
replacing containers again. An interrupted attempt requires explicit inspection
and rollback; it cannot resume as a fresh deployment. A backup/key/image-loading
failure before activation leaves a private non-active attempt and does not
replace containers. Roll it back or inspect it explicitly before any successor
release. This initial Practice helper accepts no predecessor recovery hashes;
a future recovery helper must pin both predecessor helper identities through a
reviewed change. Reconcile the scoped marker through this rollback path before
any full release or getWork activation.

The `--test-root` option is only for non-root private temporary fixtures and
fixture-owned command adapters. It is rejected for root and cannot select a
production host, service, database, migration or health bypass.
