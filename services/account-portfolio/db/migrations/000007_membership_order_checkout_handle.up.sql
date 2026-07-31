-- The checkout handle is the WeChat weixin:// payment URI a browser opens to
-- scan the QR code. Persisting it lets a user who navigated away return to the
-- same code instead of abandoning the order and creating a duplicate.
--
-- The constraint is the durable form of ADR-0019's browser-safety rule: only a
-- weixin:// URI may ever be stored, so no gateway regression can persist a URL
-- that carries the private merchant order number.
ALTER TABLE account_portfolio_payment_order_intents
    ADD COLUMN IF NOT EXISTS checkout_url TEXT,
    ADD COLUMN IF NOT EXISTS checkout_url_expires_at TIMESTAMPTZ;

ALTER TABLE account_portfolio_payment_order_intents
    DROP CONSTRAINT IF EXISTS account_portfolio_payment_order_intents_checkout_url_shape;

ALTER TABLE account_portfolio_payment_order_intents
    ADD CONSTRAINT account_portfolio_payment_order_intents_checkout_url_shape CHECK (
        (checkout_url IS NULL AND checkout_url_expires_at IS NULL)
        OR
        (checkout_url LIKE 'weixin://%' AND length(checkout_url) BETWEEN 12 AND 512 AND checkout_url_expires_at IS NOT NULL)
    );
