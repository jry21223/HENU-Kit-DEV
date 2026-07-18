ALTER TABLE mail_deliveries
    ADD COLUMN IF NOT EXISTS enqueue_key varchar(64),
    ADD COLUMN IF NOT EXISTS request_hash varchar(64);

CREATE UNIQUE INDEX IF NOT EXISTS uq_mail_deliveries_enqueue_key
    ON mail_deliveries(enqueue_key)
    WHERE enqueue_key IS NOT NULL AND enqueue_key <> '';

