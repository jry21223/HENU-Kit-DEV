# Study migration 0002: materials sync expand

Owner: Study Legacy database (`final_review_v2` / the configured Study database).

Prerequisite: the existing Study baseline must already contain
`public.materials(storage_key, deleted_at)`. Before Up, verify that active rows
have no duplicate `storage_key`; the migration repeats this check while holding
the migration advisory lock and fails closed on a conflict.

Up adds the nullable `sha256` and `slides` columns, the active storage-key
unique index, and the singleton `public.henukit_materials_sync_state` recovery
table. Existing rows are not rewritten. The table contains at most one row.
After Up, the migration owner must grant the configured sync DML identity only
the `SELECT`, `INSERT`, and `UPDATE` privileges it needs on the marker and Study
catalogue tables; the sync identity must not retain schema/DDL privileges.

Lock impact: `ALTER TABLE` briefly takes an access-exclusive lock on
`public.materials`; index creation scans active materials rows. Keep the
materials watcher disabled and schedule the migration in the Study maintenance
window. Estimated data growth is one marker row plus hashes/JSON written by
later sync transactions.

Verification SQL:

```sql
SELECT to_regclass('public.materials_storage_key_active_idx');
SELECT to_regclass('public.henukit_materials_sync_state');
SELECT column_name
FROM information_schema.columns
WHERE table_schema = 'public'
  AND table_name = 'materials'
  AND column_name IN ('sha256', 'slides')
ORDER BY column_name;
```

Rollback: stop the watcher, run the Down file to remove only the private marker
table, and keep the additive columns/index for compatibility and retained data.
The materials driver then fails its schema preflight before changing public
files. Restoring sync requires reapplying Up and verifying the queries above.
