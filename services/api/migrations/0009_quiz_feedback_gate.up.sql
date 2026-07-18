ALTER TABLE reports
    ADD COLUMN IF NOT EXISTS target_snapshot jsonb,
    ADD COLUMN IF NOT EXISTS json_verified boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS postgres_verified boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS api_verified boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS version integer NOT NULL DEFAULT 1;

