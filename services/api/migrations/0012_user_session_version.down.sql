ALTER TABLE users
    DROP COLUMN IF EXISTS version,
    DROP COLUMN IF EXISTS token_version;

