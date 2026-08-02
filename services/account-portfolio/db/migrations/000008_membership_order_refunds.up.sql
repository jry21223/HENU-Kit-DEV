-- The public refund identifier is a random UUID persisted here; the gateway
-- correlation (out_refund_no) is derived deterministically from the merchant
-- order and never crosses the service boundary, so a browser can never rebuild
-- the private HNK merchant order number from a refund id (ADR-0019). The
-- unique (provider_name, merchant_order_id) anchor means one merchant order
-- resolves to exactly one refund record, so a retried refund reuses the same
-- public id instead of minting a second one.
CREATE TABLE IF NOT EXISTS account_portfolio_membership_order_refunds (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL REFERENCES account_portfolio_membership_orders(id) ON DELETE RESTRICT,
    provider_name TEXT NOT NULL CHECK (length(provider_name) BETWEEN 1 AND 80),
    merchant_order_id TEXT NOT NULL CHECK (merchant_order_id ~ '^HNK[A-Z2-7]{29}$'),
    out_refund_no TEXT NOT NULL CHECK (length(out_refund_no) BETWEEN 1 AND 200),
    status TEXT NOT NULL CHECK (status IN ('processing', 'succeeded', 'closed', 'abnormal')),
    amount_cents INTEGER NOT NULL CHECK (amount_cents = 990),
    entitlement_revoked BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT account_portfolio_membership_order_refunds_provider_merchant_unique UNIQUE (provider_name, merchant_order_id)
);
CREATE INDEX IF NOT EXISTS account_portfolio_membership_order_refunds_order_created_idx
    ON account_portfolio_membership_order_refunds (order_id, created_at DESC, id DESC);
