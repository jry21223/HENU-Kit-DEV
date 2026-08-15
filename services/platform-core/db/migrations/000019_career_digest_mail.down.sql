-- Reverse the additive career_digest mail generalization. The original
-- mail_outbox required verification_code_id NOT NULL and kind='verification_code'.

ALTER TABLE mail_outbox
    DROP CONSTRAINT IF EXISTS mail_outbox_kind_check;

ALTER TABLE mail_outbox
    ADD CONSTRAINT mail_outbox_kind_check
    CHECK (kind = 'verification_code');

ALTER TABLE mail_outbox
    DROP COLUMN IF EXISTS recipient_user_id;

DELETE FROM mail_outbox WHERE kind = 'career_digest';

ALTER TABLE mail_outbox
    ALTER COLUMN verification_code_id SET NOT NULL;
