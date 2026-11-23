DROP TABLE IF EXISTS portal_campus_audit_events;
DROP TABLE IF EXISTS portal_campus_exchanges;
DROP TABLE IF EXISTS portal_campus_contacts;

ALTER TABLE portal_campus_messages
    DROP COLUMN IF EXISTS hidden,
    DROP COLUMN IF EXISTS author_user_id;

ALTER TABLE portal_campus_items
    DROP COLUMN IF EXISTS version,
    DROP COLUMN IF EXISTS moderation_note,
    DROP COLUMN IF EXISTS accepted_at,
    DROP COLUMN IF EXISTS accepted_by_name,
    DROP COLUMN IF EXISTS accepted_by_user_id,
    DROP COLUMN IF EXISTS author_user_id;
