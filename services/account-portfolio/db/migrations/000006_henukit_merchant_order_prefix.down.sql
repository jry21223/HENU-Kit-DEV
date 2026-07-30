DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM account_portfolio_payment_order_intents) THEN
        RAISE EXCEPTION 'refusing to remove HNK merchant order numbers while payment intents exist';
    END IF;
END $$;

ALTER TABLE account_portfolio_payment_order_intents
    DROP CONSTRAINT IF EXISTS account_payment_intent_merchant_order_format;
ALTER TABLE account_portfolio_payment_order_intents
    ALTER COLUMN merchant_order_id TYPE UUID USING merchant_order_id::uuid;

-- Keep the runner's version source of truth aligned with the rolled-back
-- schema so the additive migration is reconciled on the next service start.
DO $$
BEGIN
    IF to_regclass('account_portfolio_schema_migrations') IS NOT NULL THEN
        DELETE FROM account_portfolio_schema_migrations
        WHERE version = '000006_henukit_merchant_order_prefix';
    END IF;
END $$;
