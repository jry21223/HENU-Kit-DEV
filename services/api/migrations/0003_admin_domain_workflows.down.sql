DROP INDEX IF EXISTS idx_campus_notices_reviewed_by;
ALTER TABLE campus_notices DROP COLUMN IF EXISTS reviewed_at;
ALTER TABLE campus_notices DROP COLUMN IF EXISTS reviewed_by;
ALTER TABLE campus_notices DROP COLUMN IF EXISTS review_reason;
