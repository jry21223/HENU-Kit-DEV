ALTER TABLE mail_suppressions
    ADD COLUMN IF NOT EXISTS version integer NOT NULL DEFAULT 1;

