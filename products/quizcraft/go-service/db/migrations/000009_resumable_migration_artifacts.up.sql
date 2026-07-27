ALTER TABLE quizcraft_migration_runs
    ADD COLUMN IF NOT EXISTS source_snapshot_sha256 text NOT NULL DEFAULT ''
        CHECK (source_snapshot_sha256 = '' OR source_snapshot_sha256 ~ '^[0-9a-f]{64}$'),
    ADD COLUMN IF NOT EXISTS resume_attempt_count integer NOT NULL DEFAULT 1
        CHECK (resume_attempt_count >= 1);
