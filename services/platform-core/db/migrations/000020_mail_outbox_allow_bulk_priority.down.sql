-- Restore the verification-bound priority constraint. The digest rows written
-- under 'bulk' are deleted first by 000019's down (kind = 'career_digest'), so
-- no row remains that the restored CHECK would reject.

ALTER TABLE mail_outbox
    DROP CONSTRAINT IF EXISTS mail_outbox_priority_check;

ALTER TABLE mail_outbox
    ADD CONSTRAINT mail_outbox_priority_check
    CHECK (priority = 'critical');
