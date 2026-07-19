# Notice migration runbook

Owner: Notice service. Migration `000001_notice` creates only the `notice_*` tables and `pgcrypto`; the service never runs DDL at startup.

## Precheck and impact

Run `SELECT to_regclass('public.notice_sources'), to_regclass('public.notice_versions');` and stop if either name is already owned by another application. Confirm the deployment role can create extensions, tables, functions, triggers, and indexes. This is a new empty domain, so the expected row count is zero and there is no backfill. The migration takes catalog locks only while creating new objects; it does not lock existing business tables. On the tested empty PostgreSQL 17 database it should complete in seconds.

## Apply and verify

Apply `000001_notice.up.sql` once through the deployment migration job. Parallel application instances may start only after that job succeeds. Verify:

```sql
SELECT to_regclass('public.notice_sources') IS NOT NULL;
SELECT to_regclass('public.notice_versions') IS NOT NULL;
SELECT to_regclass('public.notice_distributions') IS NOT NULL;
SELECT count(*) FROM notice_sources;
```

CI additionally checks empty Up, repeated Up, preservation of a pre-existing baseline table, Down/Up, real concurrent idempotency constraints, and a `pg_dump`/`pg_restore` recovery copy.

## Rollback and recovery

Before rollback, stop the API and Worker and capture `pg_dump --format=custom`. `000001_notice.down.sql` is destructive and is allowed only before production traffic, or after a verified snapshot and explicit operator approval. Restore the snapshot into a separate recovery database first, verify source/version/distribution counts, and only then point a deployment at the recovered database. After production adoption, prefer a forward corrective migration instead of Down.
