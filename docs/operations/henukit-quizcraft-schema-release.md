# Native QuizCraft schema and compatible recovery

This extends the [fixed-artifact full release](henukit-artifact-deployment.md)
owner and [preservation mode](henukit-preserve-existing-release.md), implementing
[ADR-0061](../adr/0061-training-schema-compatible-recovery.md). The implementer
performs no production actions. Full release authorization and exact-head CI
remain mandatory.

Keep the scheduler drained through its original safe boundary before merging
or installing a release-owner prerequisite. Install the updated watcher and the
two companions below through the existing reviewed exact-main or verified
Actions-runtime software installation boundary. Retain program before-images and
hashes. All programs remain root:root 0555 at the existing trusted parent path:

| Artifact source | Installed program |
| --- | --- |
| `bin/watch-henukit-actions.sh` | `/usr/local/sbin/watch-henukit-actions` |
| `bin/henukit-quizcraft-release.py` | `/usr/local/sbin/henukit-quizcraft-release.py` |
| `bin/henukit-quizcraft-database.py` | `/usr/local/sbin/henukit-quizcraft-database.py` |

Keep the HC-645 preservation verifier, activator and scheduler installation
checkpoint. Extend the reviewed program hash record with both companion
basenames. Do not alter signing/SSH trust, credentials, grants, environment files,
policies, unit security settings, HBA or network rules. Never install an unmerged
branch or execute an unverified candidate. This is a software update within the
original owner; no second publisher or recurring controller is created.

The original runtime packager compiles `quizcraft-migrate` and
`quizcraft-release-state` in its existing Linux/amd64 build container and packages
the exact up/down SQL, fixed owner backup verifiers and full-SHA capability
inventory. The production Core image need not contain a compiler or migration
runner. Production only executes the verified runtime payload.

Preparation uses the real current Core container, not an arbitrary DSN supplied
by the caller. Its container-origin probe pins the exact opened network namespace
FD, kernel type, PID/start/restart/network and Docker SandboxKey device/inode.
Trusted host nsenter enters only that network namespace. The unchanged owner
credentials through gateway and loopback and existing postgres peer-admin must
identify the same database OID/name, server start, port and PostgreSQL16 version.
Only installed `/usr/lib/postgresql/16/bin`, `/usr/bin/nsenter` and
`/usr/sbin/runuser` are used. A host-origin gateway probe is not a substitute.
A failed proof is not repaired by changing authentication or network settings.

Under the original watcher/state/source locks and exact-SHA approval, the owner:

1. Verifies candidate artifact bytes, exact schema history and capability for all
   three consumers. Binds the original immediately previous target, original
   environment file, approval/rollback contract, artifact and database identity.
2. Records maintenance intent, stops the sole real Core writer and proves no
   unexpected database connections/writers remain. No write or migration flag is
   enabled, and no environment file is rewritten. After stopping, recheck the
   retained stopped writer and same loopback/peer database identity; its former
   namespace is not represented as a new running-container proof.
3. Exports a read-only repeatable-read snapshot. The fixed Core SQL returns only
   sorted anonymous SHA256 row digests and empty-table sentinels, including
   catalog and migration history. Dump that exact snapshot, enumerate and hash
   its custom archive, restore into only a unique
   `henukit_quizcraft_verify_<sha-prefix>_<random>` database via existing postgres
   peer admin, compare all content/catalog digests and drop only that verifier
   database. Keep the retained dump and proof private. A failure blocks migration.
4. Runs Core `ApplyVersionedMigrations`, retaining its target, advisory-lock,
   per-migration transaction and checksum rules. Repetition is skip-only.
   Recheck exact schema12 and stopped writer before the original full helper
   starts fixed images. Directly calling that helper cannot skip this window.
5. Verifies runtime/schema health before the original alias/activation marker.
   Normal Console content import, validate and publish occur subsequently using
   the real operator session. No SQL content seed is part of deployment.

The original Platform/Account backups and all other release gates still run.
Native Core proofs live under the existing backup root; candidate-bound 0400
identity/phase records live at
`<state-root>/quizcraft-releases/<candidate-sha>/`. Their original inode/digests
are anchored in the original private backup metadata, whose inode is bound back
to the identity. Receipts contain credential digests, never another credential
copy. Missing, replaced or unanchored evidence stops recovery. Do not edit or
remove receipts, prepared metadata, dumps or pending rollback contracts. Retain
the original verified runtime archive for the active and immediately previous
training-capable releases; their capability proof binds those exact retained bytes.

Schema11 is safe only with exact tracked history and catalog proof. Schema12
with **any authored training content**, even an unpublished unit with no
sessions/attempts, requires training-aware consumers. An old runtime can return
only before those facts exist. Every rollback, including a failed activation,
apparently still-healthy prior runtime and interrupted/degraded recovery, first
stops the writer and re-reads live Core state. Its bound original immediately
previous target must carry verified capability once facts exist. A missing or
changed new capability is not interpreted as a legacy target. All prior image,
environment, materials, alias and preservation gates also remain.

Initial pre-content failure may return the previous compatible service state;
the additive schema can remain. Migration12 down refuses authored content or
sessions. Once training facts have been accepted, keep maintenance and use a
compatible target or forward recovery. Never restore a previous database backup
over student writes. Unknown database/schema, failed freeze or uncertain evidence
is an operator-recovery hold, not permission to restart an incompatible binary.

Local verification covers the real Core probe and PG16 dump/restore SQL plus
full-owner subprocess failure/interruption/capability paths. Linux CI retains
namespace/foreign-owner checks. Only final exact-main artifacts and real normal
Console/browser acceptance establish production delivery.
