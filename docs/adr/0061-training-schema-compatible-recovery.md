---
status: accepted
amends: 0013, 0051, 0056
---

# Release training schema through the original owner with compatible recovery

Training uses canonical Practice attempts. The old runtime does not recognize
training purpose, content or statistical exclusions. Authored training content
already requires compatible consumers, including an unpublished draft before a
learner session exists. An old database backup cannot authorize losing accepted
writes.

The existing full release watcher remains the sole publisher. Its maintenance
window stops the sole Core writer before a fresh native PostgreSQL16 snapshot,
verified custom dump, isolated restore and Core versioned migration. It retains
all exact-main, artifact, approval, source lease, preservation, image, alias,
materials and rollback gates. Pure native-database authority is shared with the
existing scoped Practice entry; that entry retains its unchanged-migration gate
and no part of its application-key or publishing path is used here.

A Core-owned read-only release-state command recognizes only healthy tracked
schemas11/12 with exact history/checksums and reviewed catalog fingerprints.
Absence of training tables is safe only for the known schema11 baseline.
Unpublished authored content, sessions or attempts require the source-bound
`english-training-data-v1` capability for Core, Portal Gateway and Portal.
Unknown schema, database identity, capability or evidence fails closed.

Fixed Core-owned SQL emits anonymous row digests and catalog evidence. The
release owner compares the same exported snapshot used by pg_dump with an
isolated peer-admin restore. Ops does not own business table SQL. All new code,
SQL and capability declarations travel through existing verified Linux/amd64
artifacts. No production compilation, credentials, grants, HBA changes,
configuration rewriting or enablement is introduced.

Before every normal or interrupted rollback the original immediately previous
target remains selected. Stop writes again and recheck live Core facts and that
target's verified capability before replacement. The initial old target is
usable only before training facts; thereafter use a compatible target or forward
recovery. Do not restore an old backup or automatically down-migrate accepted
training. The original eight-line rollback contract remains intact; mutually
bound private evidence joins it to schema, backup and capability checks.

Healthy first runtime/schema activation precedes explicit normal Console
import, validate and publish with the real operator cookie. Deployment does not
seed or publish content. See the [operational procedure](../operations/henukit-quizcraft-schema-release.md).
