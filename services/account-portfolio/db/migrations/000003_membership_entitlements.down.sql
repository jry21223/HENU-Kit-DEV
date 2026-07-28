DROP INDEX IF EXISTS account_portfolio_notifications_membership_event_unique_idx;
ALTER TABLE account_portfolio_notifications
    DROP COLUMN IF EXISTS membership_event_id;

DROP TABLE IF EXISTS account_portfolio_membership_events;

ALTER TABLE account_portfolio_memberships
    DROP CONSTRAINT IF EXISTS account_portfolio_memberships_version_positive;
ALTER TABLE account_portfolio_memberships
    DROP COLUMN IF EXISTS version;

-- ApplyMigrations records this version outside the business tables. A manual
-- rollback must clear the record so the additive membership facts can be
-- reconciled safely on the next service start.
DO $$
BEGIN
    IF to_regclass('account_portfolio_schema_migrations') IS NOT NULL THEN
        DELETE FROM account_portfolio_schema_migrations
        WHERE version = '000003_membership_entitlements';
    END IF;
END $$;
