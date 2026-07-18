DROP INDEX IF EXISTS uq_mail_deliveries_enqueue_key;
ALTER TABLE mail_deliveries
    DROP COLUMN IF EXISTS request_hash,
    DROP COLUMN IF EXISTS enqueue_key;

