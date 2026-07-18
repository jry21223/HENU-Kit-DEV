ALTER TABLE campus_notices
    ADD COLUMN IF NOT EXISTS audience jsonb NOT NULL DEFAULT '["all_verified_users"]'::jsonb;
