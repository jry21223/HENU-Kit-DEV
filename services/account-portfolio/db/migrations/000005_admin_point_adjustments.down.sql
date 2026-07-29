DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM account_portfolio_point_adjustment_audits) THEN
        RAISE EXCEPTION 'cannot roll back point adjustments while durable audits exist';
    END IF;
END $$;

DROP TRIGGER IF EXISTS account_portfolio_point_adjustment_audits_immutable ON account_portfolio_point_adjustment_audits;
DROP TRIGGER IF EXISTS account_portfolio_point_ledger_immutable ON account_portfolio_point_ledger;
DROP FUNCTION IF EXISTS account_portfolio_reject_point_fact_mutation();

ALTER TABLE account_portfolio_points
    DROP CONSTRAINT IF EXISTS account_portfolio_points_json_safe_balance;
ALTER TABLE account_portfolio_point_ledger
    DROP CONSTRAINT IF EXISTS account_portfolio_point_ledger_json_safe_amount;

DROP INDEX IF EXISTS account_portfolio_notifications_point_ledger_unique_idx;
ALTER TABLE account_portfolio_notifications
    DROP COLUMN IF EXISTS point_ledger_id;

DROP INDEX IF EXISTS account_portfolio_point_ledger_audit_unique_idx;
ALTER TABLE account_portfolio_point_ledger
    DROP COLUMN IF EXISTS audit_id;

DROP TABLE IF EXISTS account_portfolio_point_adjustment_audits;

-- ApplyMigrations records this version outside the business tables. A manual
-- rollback must clear the record so this additive owner boundary can be
-- reconciled safely on the next service start.
DO $$
BEGIN
    IF to_regclass('account_portfolio_schema_migrations') IS NOT NULL THEN
        DELETE FROM account_portfolio_schema_migrations
        WHERE version = '000005_admin_point_adjustments';
    END IF;
END $$;
