DROP INDEX IF EXISTS idx_mail_deliveries_recipient_user_id;
ALTER TABLE mail_deliveries DROP COLUMN IF EXISTS recipient_user_id;
DROP TABLE IF EXISTS notice_distribution_receipts;
DROP TABLE IF EXISTS notice_email_subscriptions;

