ALTER TABLE campus_notices ADD COLUMN IF NOT EXISTS review_reason varchar(1000) NOT NULL DEFAULT '';
ALTER TABLE campus_notices ADD COLUMN IF NOT EXISTS reviewed_by uuid;
ALTER TABLE campus_notices ADD COLUMN IF NOT EXISTS reviewed_at timestamptz;
CREATE INDEX IF NOT EXISTS idx_campus_notices_reviewed_by ON campus_notices(reviewed_by);
