DROP TABLE IF EXISTS account_portfolio_service_nonces;
DROP TABLE IF EXISTS account_portfolio_ticket_messages;
DROP TABLE IF EXISTS account_portfolio_tickets;
DROP TABLE IF EXISTS account_portfolio_notifications;
DROP TABLE IF EXISTS account_portfolio_membership_orders;
DROP TABLE IF EXISTS account_portfolio_point_ledger;
DROP TABLE IF EXISTS account_portfolio_memberships;
DROP TABLE IF EXISTS account_portfolio_points;
DROP TABLE IF EXISTS account_portfolio_accounts;

-- ApplyMigrations records this version outside the business tables. A manual
-- rollback must clear that record too; otherwise the next service start would
-- skip this idempotent migration and leave the owner without its schema.
DO $$
BEGIN
    IF to_regclass('account_portfolio_schema_migrations') IS NOT NULL THEN
        DELETE FROM account_portfolio_schema_migrations
        WHERE version = '000001_account_portfolio';
    END IF;
END $$;
