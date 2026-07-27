ALTER TABLE quizcraft_migration_runs
    DROP COLUMN IF EXISTS resume_attempt_count,
    DROP COLUMN IF EXISTS source_snapshot_sha256;

-- The schema-history runner records 000009 only after its up migration
-- commits. A permitted raw rollback must remove that ledger row as well, or a
-- subsequent cmd/migrate run would incorrectly skip the missing columns.
DO $$ BEGIN
    DELETE FROM quizcraft_schema_migrations WHERE version = '000009';
EXCEPTION WHEN undefined_table THEN NULL;
END $$;
