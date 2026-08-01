ALTER TABLE account_portfolio_payment_order_intents
    DROP CONSTRAINT IF EXISTS account_portfolio_payment_order_intents_checkout_url_shape;

ALTER TABLE account_portfolio_payment_order_intents
    DROP COLUMN IF EXISTS checkout_url_expires_at,
    DROP COLUMN IF EXISTS checkout_url;
