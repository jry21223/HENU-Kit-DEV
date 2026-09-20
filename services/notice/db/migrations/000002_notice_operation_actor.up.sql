-- Existing rows predate actor ownership and remain readable only to no caller.
-- They expire after 24 hours; keeping the column nullable avoids a table rewrite.
SET lock_timeout = '5s';

ALTER TABLE notice_operations
    ADD COLUMN IF NOT EXISTS actor_user_id uuid;
