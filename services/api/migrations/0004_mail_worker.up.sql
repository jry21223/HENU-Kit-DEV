ALTER TABLE mail_deliveries ADD COLUMN IF NOT EXISTS recipient varchar(320) NOT NULL DEFAULT '';
ALTER TABLE mail_deliveries ADD COLUMN IF NOT EXISTS subject varchar(500) NOT NULL DEFAULT '';
ALTER TABLE mail_deliveries ADD COLUMN IF NOT EXISTS body text NOT NULL DEFAULT '';
ALTER TABLE mail_deliveries ADD COLUMN IF NOT EXISTS locked_at timestamptz;
ALTER TABLE mail_deliveries ADD COLUMN IF NOT EXISTS locked_by varchar(120) NOT NULL DEFAULT '';
ALTER TABLE mail_deliveries ADD COLUMN IF NOT EXISTS last_error_code varchar(120) NOT NULL DEFAULT '';
ALTER TABLE mail_deliveries ADD COLUMN IF NOT EXISTS version integer NOT NULL DEFAULT 1;
CREATE INDEX IF NOT EXISTS idx_mail_deliveries_claim ON mail_deliveries(status, category, next_retry_at, queued_at);

CREATE TABLE IF NOT EXISTS mail_attempts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
    delivery_id uuid NOT NULL REFERENCES mail_deliveries(id), attempt integer NOT NULL, status varchar(40) NOT NULL,
    error_code varchar(120) NOT NULL DEFAULT '', started_at timestamptz NOT NULL, ended_at timestamptz
);
CREATE INDEX IF NOT EXISTS idx_mail_attempts_delivery ON mail_attempts(delivery_id, attempt);

CREATE TABLE IF NOT EXISTS mail_suppressions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
    recipient_hash varchar(64) NOT NULL UNIQUE, reason_code varchar(120) NOT NULL, expires_at timestamptz
);
CREATE INDEX IF NOT EXISTS idx_mail_suppressions_expiry ON mail_suppressions(expires_at);
