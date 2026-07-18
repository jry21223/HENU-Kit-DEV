ALTER TABLE reports
    DROP COLUMN IF EXISTS version,
    DROP COLUMN IF EXISTS api_verified,
    DROP COLUMN IF EXISTS postgres_verified,
    DROP COLUMN IF EXISTS json_verified,
    DROP COLUMN IF EXISTS target_snapshot;

