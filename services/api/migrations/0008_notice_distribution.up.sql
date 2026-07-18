CREATE TABLE IF NOT EXISTS notice_email_subscriptions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
    user_id uuid NOT NULL UNIQUE REFERENCES users(id), enabled boolean NOT NULL DEFAULT false, version integer NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS notice_distribution_receipts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
    notice_id uuid NOT NULL REFERENCES campus_notices(id), user_id uuid NOT NULL REFERENCES users(id), channel varchar(24) NOT NULL,
    CONSTRAINT uq_notice_distribution UNIQUE (notice_id, user_id, channel)
);

ALTER TABLE mail_deliveries ADD COLUMN IF NOT EXISTS recipient_user_id uuid REFERENCES users(id);
CREATE INDEX IF NOT EXISTS idx_mail_deliveries_recipient_user_id ON mail_deliveries(recipient_user_id);

