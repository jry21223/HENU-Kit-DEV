DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM account_portfolio_membership_order_refunds) THEN
        RAISE EXCEPTION 'cannot roll back membership order refunds while durable refund records exist';
    END IF;
END $$;

DROP INDEX IF EXISTS account_portfolio_membership_order_refunds_order_created_idx;
DROP TABLE IF EXISTS account_portfolio_membership_order_refunds;

-- ApplyMigrations records this version outside the business tables. A manual
-- rollback must clear the record so this additive refund boundary can be
-- reconciled safely on the next service start.
DO $$
BEGIN
    IF to_regclass('account_portfolio_schema_migrations') IS NOT NULL THEN
        DELETE FROM account_portfolio_schema_migrations
        WHERE version = '000008_membership_order_refunds';
    END IF;
END $$;
