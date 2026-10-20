---
status: accepted
---

# Release Practice and its Console bridge as a fixed cohort

The H3C delivery requires image rendering in Portal and an authenticated
Console-to-QuizCraft management boundary. The production full-stack baseline
has a deferred Career/getWork group. The candidate Portal also exposes Food
author editing, whose routes require the same-candidate Portal Gateway and
Food; baseline `11bf9e6` lacks both backend contracts. ADR-0049 already makes
those three services atomic. Their fixed union with the Console/QuizCraft
bridge is therefore five services. Releasing this dependency closure must not
activate that group or imply its acceptance.

## Decision

- Add a fixed `portal`, `portal-gateway`, `food`, `console-gateway`, `quizcraft`
  cohort using the existing
  scoped release engine. Keep the Food entry and behavior intact. There is no
  service, migration, host, or health bypass option.
- Require current merged main, the newest successful main push workflow,
  authenticated exact-run Actions downloads, runtime attestation, archive and
  image identities, trusted helpers, the watcher handshake, durable receipts,
  and the existing rollback and disk gates.
- Keep full-stack aliases, the full release marker, and every unselected
  container unchanged. Share the existing scoped marker so the installed
  full-release guard cannot overlook this cohort. The private receipt names
  the real scope and explicitly defers getWork acceptance.
- Introduce only the dedicated QuizCraft Console application key. Generate it
  in the reviewed helper after both database backup/restore gates, preserve it in a root-owned
  0600 receipt file, and inject it only into the candidate Compose environment.
  Keep the global environment byte-identical. Do not reuse Session, OAuth,
  signing, or other service credentials, or create accounts/grants.
- Permit only the reviewed management environment additions in candidate
  Compose. All other selected service and shared-resource settings must match
  the retained baseline. Verify effective configuration after replacement.
- Prove both running database targets and unchanged Food migrations. Reuse the
  existing Food engine's local URL binding, container PostgreSQL dump and
  isolated restore; retain Food's digest, bytes and restored post count in its
  own receipt entry. Save the global configuration once for both owners.
  Prove the running QuizCraft database target and unchanged migrations. Back
  up that existing database with the installed matching PostgreSQL tools and
  restore it into an isolated verification database before replacing a service.
  The owner URL's host-gateway route from its exact running container network
  namespace, host loopback route, and existing
  postgres admin connection must identify the same PostgreSQL 16 instance and
  database. Pin the network namespace FD to the container ID/PID/start time and
  Docker sandbox device/inode, verify the kernel network type, and recheck the
  binding around the read-only query. Trusted host `nsenter` enters only that
  network namespace and executes the installed host PG16 tool. Do not change HBA,
  roles, credentials, grants or container configuration to make a host-origin
  gateway probe pass. Preserve every URL component except the derived host route. Use an
  exported repeatable-read snapshot for the dump and comparison counts so new
  student attempts cannot race the backup check. Never migrate or publish
  content through SQL in this path. Retain QuizCraft's digest, bytes and counts
  in a separate receipt entry; neither backup may overwrite the other's proof.
- Require candidate health, public Practice/catalog and legacy rejection,
  unauthenticated Console management rejection, Food author-edit GET/PUT
  rejection, and configuration identity. Real authenticated Food author edit
  read/save and existing list/detail/create behavior need separate acceptance.
  The later authenticated import/read/validate/publish journey is separately
  required for content acceptance.
- Roll back only these images with the original effective configuration on a
  failed release gate. Retain the application key and backups privately for
  audit; never restore an old database over new student attempts automatically.

## Consequences

This is a scoped runtime release. Portal Summary still identifies the retained
full-stack baseline; the receipt and actual image identities identify the
Practice cohort. A later full release must first reconcile this scoped state
through its reviewed rollback path.

The implementation entry is `scripts/ops/deploy-henukit-practice-scoped.py`;
see [the operations runbook](../operations/henukit-practice-scoped-release.md).
The private Practice receipt pins both the entry and imported engine hashes;
it does not inherit Food predecessor-recovery permissions. Complete migration
inventories and Git blob identities are compared against the exact baseline
and candidate GitHub commits and the attested candidate runtime.
