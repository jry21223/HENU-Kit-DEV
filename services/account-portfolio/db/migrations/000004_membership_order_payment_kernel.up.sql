ALTER TABLE account_portfolio_membership_orders
    ADD COLUMN IF NOT EXISTS version INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS provider_event_sequence BIGINT NOT NULL DEFAULT 0;

ALTER TABLE account_portfolio_membership_orders
    DROP CONSTRAINT IF EXISTS account_portfolio_membership_orders_version_positive;
ALTER TABLE account_portfolio_membership_orders
    ADD CONSTRAINT account_portfolio_membership_orders_version_positive CHECK (version > 0);
ALTER TABLE account_portfolio_membership_orders
    DROP CONSTRAINT IF EXISTS account_portfolio_membership_orders_provider_event_sequence_nonnegative;
ALTER TABLE account_portfolio_membership_orders
    ADD CONSTRAINT account_portfolio_membership_orders_provider_event_sequence_nonnegative CHECK (provider_event_sequence >= 0);
ALTER TABLE account_portfolio_membership_orders
    DROP CONSTRAINT IF EXISTS account_portfolio_membership_orders_provider_valid;
ALTER TABLE account_portfolio_membership_orders
    ADD CONSTRAINT account_portfolio_membership_orders_provider_valid CHECK (length(provider) BETWEEN 1 AND 80);
ALTER TABLE account_portfolio_membership_orders
    DROP CONSTRAINT IF EXISTS account_portfolio_membership_orders_status_check;
ALTER TABLE account_portfolio_membership_orders
    ADD CONSTRAINT account_portfolio_membership_orders_status_check CHECK (
        status IN ('created', 'pending_payment', 'paid', 'closed', 'failed', 'refunded')
    );

ALTER TABLE account_portfolio_membership_orders
    DROP CONSTRAINT IF EXISTS account_portfolio_membership_orders_idempotency_key_key;
CREATE UNIQUE INDEX IF NOT EXISTS account_portfolio_membership_orders_user_idempotency_key_idx
    ON account_portfolio_membership_orders (user_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE TABLE IF NOT EXISTS account_portfolio_payment_facts (
    id UUID PRIMARY KEY,
    order_id UUID NOT NULL REFERENCES account_portfolio_membership_orders(id) ON DELETE RESTRICT,
    provider TEXT NOT NULL CHECK (length(provider) BETWEEN 1 AND 80),
    provider_event_id TEXT NOT NULL CHECK (length(provider_event_id) BETWEEN 1 AND 200),
    external_order_id TEXT NOT NULL CHECK (length(external_order_id) BETWEEN 1 AND 200),
    status TEXT NOT NULL CHECK (status IN ('pending_payment', 'paid', 'closed', 'failed', 'refunded')),
    provider_sequence BIGINT NOT NULL CHECK (provider_sequence > 0),
    occurred_at TIMESTAMPTZ NOT NULL,
    payload_sha256 BYTEA NOT NULL CHECK (octet_length(payload_sha256) = 32),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT account_portfolio_payment_facts_provider_event_unique UNIQUE (provider, provider_event_id)
);
CREATE INDEX IF NOT EXISTS account_portfolio_payment_facts_order_sequence_idx
    ON account_portfolio_payment_facts (order_id, provider_sequence ASC, id ASC);

CREATE TABLE IF NOT EXISTS account_portfolio_payment_audits (
    id UUID PRIMARY KEY,
    order_id UUID REFERENCES account_portfolio_membership_orders(id) ON DELETE RESTRICT,
    payment_fact_id UUID REFERENCES account_portfolio_payment_facts(id) ON DELETE RESTRICT,
    provider TEXT NOT NULL CHECK (length(provider) BETWEEN 1 AND 80),
    outcome TEXT NOT NULL CHECK (outcome IN (
        'order_created',
        'order_creation_failed',
        'notification_applied',
        'notification_replayed',
        'notification_out_of_order',
        'notification_rejected',
        'notification_query_failed',
        'notification_unknown_order',
        'notification_invalid_transition'
    )),
    reason_code TEXT NOT NULL CHECK (reason_code ~ '^[a-z][a-z0-9_]{1,79}$'),
    payload_sha256 BYTEA NOT NULL CHECK (octet_length(payload_sha256) = 32),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS account_portfolio_payment_audits_order_created_idx
    ON account_portfolio_payment_audits (order_id, created_at ASC, id ASC)
    WHERE order_id IS NOT NULL;

ALTER TABLE account_portfolio_memberships
    ADD COLUMN IF NOT EXISTS payment_fact_id UUID;
ALTER TABLE account_portfolio_memberships
    DROP CONSTRAINT IF EXISTS account_portfolio_memberships_payment_fact_fk;
ALTER TABLE account_portfolio_memberships
    ADD CONSTRAINT account_portfolio_memberships_payment_fact_fk
        FOREIGN KEY (payment_fact_id) REFERENCES account_portfolio_payment_facts(id) ON DELETE RESTRICT;
ALTER TABLE account_portfolio_memberships
    DROP CONSTRAINT IF EXISTS account_portfolio_memberships_payment_source_shape;
ALTER TABLE account_portfolio_memberships
    ADD CONSTRAINT account_portfolio_memberships_payment_source_shape CHECK (
        (source = 'payment' AND payment_fact_id IS NOT NULL)
        OR
        (source <> 'payment' AND payment_fact_id IS NULL)
    );

ALTER TABLE account_portfolio_membership_events
    ALTER COLUMN actor_user_id DROP NOT NULL;
ALTER TABLE account_portfolio_membership_events
    ADD COLUMN IF NOT EXISTS payment_fact_id UUID;
ALTER TABLE account_portfolio_membership_events
    DROP CONSTRAINT IF EXISTS account_portfolio_membership_events_payment_fact_fk;
ALTER TABLE account_portfolio_membership_events
    ADD CONSTRAINT account_portfolio_membership_events_payment_fact_fk
        FOREIGN KEY (payment_fact_id) REFERENCES account_portfolio_payment_facts(id) ON DELETE RESTRICT;
ALTER TABLE account_portfolio_membership_events
    DROP CONSTRAINT IF EXISTS account_portfolio_membership_events_source_check;
ALTER TABLE account_portfolio_membership_events
    ADD CONSTRAINT account_portfolio_membership_events_source_check CHECK (
        (source = 'operator' AND actor_user_id IS NOT NULL AND payment_fact_id IS NULL)
        OR
        (source = 'payment' AND actor_user_id IS NULL AND payment_fact_id IS NOT NULL)
    );
CREATE UNIQUE INDEX IF NOT EXISTS account_portfolio_membership_events_payment_fact_idx
    ON account_portfolio_membership_events (payment_fact_id)
    WHERE payment_fact_id IS NOT NULL;
