-- Career Opportunities digest mail (#397): generalize the verification-bound
-- mail_outbox so Career can enqueue a controlled job digest without reusing the
-- verification code payload. This is purely additive: the verification_code
-- path keeps its NOT NULL FK and CHECK constraints intact, and career_digest
-- rows simply use a NULL verification_code_id plus a recipient_user_id.

ALTER TABLE mail_outbox
    ALTER COLUMN verification_code_id DROP NOT NULL;

ALTER TABLE mail_outbox
    DROP CONSTRAINT IF EXISTS mail_outbox_kind_check;

ALTER TABLE mail_outbox
    ADD CONSTRAINT mail_outbox_kind_check
    CHECK (kind IN ('verification_code', 'career_digest'));

ALTER TABLE mail_outbox
    ADD COLUMN IF NOT EXISTS recipient_user_id uuid;
