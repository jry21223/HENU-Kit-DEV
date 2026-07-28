DROP TABLE IF EXISTS account_portfolio_command_idempotency;

DROP INDEX IF EXISTS account_portfolio_notifications_ticket_created_idx;
DROP INDEX IF EXISTS account_portfolio_notifications_ticket_event_unique_idx;
ALTER TABLE account_portfolio_notifications
    DROP COLUMN IF EXISTS ticket_event_id,
    DROP COLUMN IF EXISTS ticket_id;

DROP TABLE IF EXISTS account_portfolio_ticket_events;

ALTER TABLE account_portfolio_ticket_messages
    DROP CONSTRAINT IF EXISTS account_portfolio_ticket_messages_author_shape;
ALTER TABLE account_portfolio_ticket_messages
    DROP COLUMN IF EXISTS operator_user_id;

-- ApplyMigrations records this version outside the business tables. A manual
-- rollback must clear the record so the additive support-ticket facts can be
-- reconciled safely on the next service start.
DO $$
BEGIN
    IF to_regclass('account_portfolio_schema_migrations') IS NOT NULL THEN
        DELETE FROM account_portfolio_schema_migrations
        WHERE version = '000002_support_ticket_commands';
    END IF;
END $$;
