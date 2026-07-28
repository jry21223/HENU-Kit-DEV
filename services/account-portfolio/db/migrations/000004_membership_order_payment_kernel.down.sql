DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM account_portfolio_membership_orders) THEN
        RAISE EXCEPTION 'cannot roll back membership order payment kernel while durable orders exist';
    END IF;
END $$;

DROP INDEX IF EXISTS account_portfolio_membership_events_payment_fact_idx;
ALTER TABLE account_portfolio_membership_events
    DROP CONSTRAINT IF EXISTS account_portfolio_membership_events_source_check;
ALTER TABLE account_portfolio_membership_events
    DROP CONSTRAINT IF EXISTS account_portfolio_membership_events_payment_fact_fk;
ALTER TABLE account_portfolio_membership_events
    DROP COLUMN IF EXISTS payment_fact_id;
ALTER TABLE account_portfolio_membership_events
    ALTER COLUMN actor_user_id SET NOT NULL;
ALTER TABLE account_portfolio_membership_events
    ADD CONSTRAINT account_portfolio_membership_events_source_check CHECK (source = 'operator');

ALTER TABLE account_portfolio_memberships
    DROP CONSTRAINT IF EXISTS account_portfolio_memberships_payment_source_shape;
ALTER TABLE account_portfolio_memberships
    DROP CONSTRAINT IF EXISTS account_portfolio_memberships_payment_fact_fk;
ALTER TABLE account_portfolio_memberships
    DROP COLUMN IF EXISTS payment_fact_id;

DROP INDEX IF EXISTS account_portfolio_payment_audits_order_created_idx;
DROP TABLE IF EXISTS account_portfolio_payment_audits;
DROP TABLE IF EXISTS account_portfolio_payment_facts;
DROP INDEX IF EXISTS account_portfolio_payment_order_intents_pending_idx;
DROP TABLE IF EXISTS account_portfolio_payment_order_intents;

DROP INDEX IF EXISTS account_portfolio_membership_orders_user_idempotency_key_idx;
ALTER TABLE account_portfolio_membership_orders
    ADD CONSTRAINT account_portfolio_membership_orders_idempotency_key_key UNIQUE (idempotency_key);
ALTER TABLE account_portfolio_membership_orders
    DROP CONSTRAINT IF EXISTS account_portfolio_membership_orders_provider_valid;
ALTER TABLE account_portfolio_membership_orders
    DROP CONSTRAINT IF EXISTS account_portfolio_membership_orders_provider_event_sequence_nonnegative;
ALTER TABLE account_portfolio_membership_orders
    DROP CONSTRAINT IF EXISTS account_portfolio_membership_orders_version_positive;
ALTER TABLE account_portfolio_membership_orders
    DROP COLUMN IF EXISTS provider_event_sequence,
    DROP COLUMN IF EXISTS version;

-- ApplyMigrations records this version outside the business tables. A manual
-- rollback must clear the record so the payment kernel can be reconciled
-- safely on the next service start.
DO $$
BEGIN
    IF to_regclass('account_portfolio_schema_migrations') IS NOT NULL THEN
        DELETE FROM account_portfolio_schema_migrations
        WHERE version = '000004_membership_order_payment_kernel';
    END IF;
END $$;
