# QuizCraft V2 recoverable migration artifacts

This guide implements the #159 preparation boundary only. It creates a separate `quizcraft_v2` database schema, freezes an import input, reconciles legacy content, and rehearses a restore. It does not alter Portal routes, enable Go writes, create a Practice Session/Answer, or cross the Go write promise point.

## Required boundaries

- `QUIZCRAFT_V2_DATABASE_URL` names the independent target database and uses its own least-privilege credentials. The migration, reconciliation, and restore commands reject a URL or connected database whose name is not exactly `quizcraft_v2`.
- `DATABASE_URL` remains the Go runtime service connection; it is not accepted as a migration-tool fallback.
- `LEGACY_DATABASE_URL` is read only for snapshot and catch-up operations.
- `QUIZCRAFT_RESTORE_ADMIN_URL` may create and drop only generated `*_restore_<random>` databases. It is required for a restore drill and is not a runtime service credential.
- The target and snapshot source must be physically distinct. The reconciliation command compares PostgreSQL system/database identity before importing.

## Schema, snapshot, and import

```bash
QUIZCRAFT_V2_DATABASE_URL='postgres://.../quizcraft_v2' go run ./cmd/migrate
QUIZCRAFT_V2_DATABASE_URL='postgres://.../quizcraft_v2' go run ./cmd/migrate

LEGACY_DATABASE_URL='postgres://.../quizcraft_legacy' \
go run ./cmd/reconcile -mode snapshot \
  -source-name quizcraft-legacy-production \
  -snapshot-file /root/quizcraft-migration/legacy-snapshot.json

QUIZCRAFT_V2_DATABASE_URL='postgres://.../quizcraft_v2' \
go run ./cmd/reconcile -mode full \
  -snapshot-file /root/quizcraft-migration/legacy-snapshot.json
```

The schema runner records every filename and SHA-256 in `quizcraft_schema_migrations`. A repeated run only reports `skipped`; a changed source file aborts. A complete released `000001`–`000008` schema with no history is adopted atomically and reported as `adopted`, then every later tracked artifact is applied (currently `000009` and `000010`). Adoption is intentionally limited to PostgreSQL 16 and verifies a frozen catalog fingerprint over the released columns, constraints, indexes, non-internal triggers, and three trigger functions; an incomplete or altered untracked baseline fails before the history relation is created. The snapshot artifact is create-only, has mode `0600`, carries the legacy database identity and a checksum over the full source snapshot, and must be retained with the resulting reconciliation report.

`full` takes a database-wide advisory lock and only starts when every V2 business table is empty (migration-history metadata is the sole exception). It therefore cannot mix two fresh source snapshots in one target; an interrupted run must use `resume` rather than starting another full import. The full report compares source metrics with the active target and also scans the entire V2 content universe: banks, active and inactive bank versions, questions, question versions, and immutable memberships. It is acceptable only when `state` is `passed`, `content_reconciled` is true, `differences` is empty, and `feedback_exception_count` is zero. Any other output is evidence of a blocked migration, not a partial success.

## Interruption recovery and final catch-up

If `full` emits a `run_id` but exits unsuccessfully, preserve its snapshot artifact and report. Do not create a second snapshot or run. Resume with the same artifact:

```bash
QUIZCRAFT_V2_DATABASE_URL='postgres://.../quizcraft_v2' \
go run ./cmd/reconcile -mode resume -run-id '<run UUID>' \
  -snapshot-file /root/quizcraft-migration/legacy-snapshot.json
```

Resume refuses a changed source name, cutoff, or snapshot checksum. Because imports retain deterministic bank/question/version IDs and migration receipts, repeated execution cannot create duplicate immutable facts. After full import, catch up the transactional legacy event log until the report is `caught_up=true`, `ready=true`, and `exception_count=0`:

```bash
QUIZCRAFT_V2_DATABASE_URL='postgres://.../quizcraft_v2' \
LEGACY_DATABASE_URL='postgres://.../quizcraft_legacy' \
go run ./cmd/reconcile -mode catch-up -run-id '<run UUID>'
```

## Backup and restore rehearsal

```bash
QUIZCRAFT_V2_DATABASE_URL='postgres://.../quizcraft_v2' \
QUIZCRAFT_RESTORE_ADMIN_URL='postgres://.../postgres' \
go run ./cmd/backuprestore \
  -backup-directory /root/quizcraft-migration/backups
```

The JSON output records the source, restore-admin, and generated-restoration database identities; the credential-free `pg_dump` and `pg_restore` argument arrays; and, for every verified table, the source and restored row counts plus deterministic content SHA-256 values. The drill rejects a restore whose table count or content summary differs from its source. It also records backup path/SHA-256 and start/completion/duration. The generated restore database is removed on completion; the backup remains for operator audit. A failed dump, checksum, restore, table-content comparison, or cleanup is a failed rehearsal and blocks cutover.

## Explicit non-goals

- No Portal or Gateway route changes.
- No public standalone QuizCraft Web restoration.
- No practice writes, V2 answer facts, learning state, ranking, or user mapping.
- No production traffic switch, Go write enablement, or write-promise crossing.

Final technical write freeze, exact release SHA, dual-database backups, browser gates, and the forward-repair boundary remain governed by [`cutover-runbook.md`](cutover-runbook.md).
