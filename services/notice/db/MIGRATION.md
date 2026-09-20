# Notice migration runbook

Owner: Notice service. Migration `000001_notice` creates only the `notice_*` tables and `pgcrypto`; later numbered migrations evolve that owner schema. The service never runs DDL at startup.

## Precheck and impact

For a fresh install, run `SELECT to_regclass('public.notice_sources'), to_regclass('public.notice_versions');` and stop if either name is already owned by another application. Confirm the deployment role can create extensions, tables, functions, triggers, and indexes.

For an upgrade, record `SELECT count(*), min(expires_at), max(expires_at) FROM notice_operations WHERE expires_at > now();` and `SELECT pg_size_pretty(pg_total_relation_size('notice_operations'));`. `000002` adds a nullable column without a table rewrite; `000003` and `000004` build indexes concurrently. `000005` briefly takes the table lock required to remove the legacy primary key, with a five-second lock timeout so deployment fails instead of waiting indefinitely. Schedule the change when Notice write traffic is low, and stop rather than retrying blindly if that lock cannot be acquired. No business facts are backfilled or deleted on Up.

## Apply and verify

Apply every numbered Up migration in order through the deployment migration job. Parallel application instances may start only after that job succeeds. Verify:

```sql
SELECT to_regclass('public.notice_sources') IS NOT NULL;
SELECT to_regclass('public.notice_versions') IS NOT NULL;
SELECT to_regclass('public.notice_distributions') IS NOT NULL;
SELECT count(*) FROM notice_sources;
```

CI additionally checks empty Up, repeated Up, preservation of a pre-existing baseline table, Down/Up, real concurrent idempotency constraints, and a `pg_dump`/`pg_restore` recovery copy.

`000002_notice_operation_actor` adds nullable ownership to the transient 24-hour operation cache without rewriting old rows. `000003_notice_operation_actor_index` builds the replacement actor-scoped unique index concurrently while the legacy primary key still protects live traffic. `000004_notice_operation_legacy_index` concurrently adds a partial unique index for still-running old binaries, whose inserts have no actor. Only after both indexes are valid and ready does `000005_notice_operation_legacy_key` remove the legacy global primary key. Old rows have no owner and are intentionally invisible until they expire. Do not deploy the actor-scoped application until all four migrations succeed.

## Rollback and recovery

Before rollback, stop the API and Worker and capture `pg_dump --format=custom`. `000001_notice.down.sql` is destructive and is allowed only before production traffic, or after a verified snapshot and explicit operator approval. Restore the snapshot into a separate recovery database first, verify source/version/distribution counts, and only then point a deployment at the recovered database. After production adoption, prefer a forward corrective migration instead of Down.

Rolling back `000005` truncates only `notice_operations`, because an older application cannot safely interpret actor-scoped keys before its global primary key is restored. That loses unresolved-operation lookup for at most 24 hours, so stop API traffic first and reconcile any pending Console commands before a rollback. Then roll back `000004`, `000003`, and `000002` in reverse order.
