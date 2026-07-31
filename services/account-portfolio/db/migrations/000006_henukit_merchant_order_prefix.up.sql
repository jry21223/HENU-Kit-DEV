-- No real Payment Provider has been enabled yet. Refuse to reinterpret any
-- durable merchant order intent: a non-empty table means the payment promise
-- point was crossed somewhere and needs an explicit forward migration.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM account_portfolio_payment_order_intents) THEN
        RAISE EXCEPTION 'cannot enable HNK merchant order numbers after payment intents exist';
    END IF;
END $$;

ALTER TABLE account_portfolio_payment_order_intents
    ALTER COLUMN merchant_order_id TYPE TEXT USING merchant_order_id::text;

ALTER TABLE account_portfolio_payment_order_intents
    DROP CONSTRAINT IF EXISTS account_payment_intent_merchant_order_format;
ALTER TABLE account_portfolio_payment_order_intents
    ADD CONSTRAINT account_payment_intent_merchant_order_format CHECK (
        merchant_order_id ~ '^HNK[A-Z2-7]{29}$'
    );
