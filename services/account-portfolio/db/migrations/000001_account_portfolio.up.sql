CREATE TABLE IF NOT EXISTS account_portfolio_accounts (
    user_id UUID PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS account_portfolio_points (
    user_id UUID PRIMARY KEY REFERENCES account_portfolio_accounts(user_id) ON DELETE RESTRICT,
    balance BIGINT NOT NULL DEFAULT 0 CHECK (balance >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS account_portfolio_memberships (
    user_id UUID PRIMARY KEY REFERENCES account_portfolio_accounts(user_id) ON DELETE RESTRICT,
    plan TEXT NOT NULL DEFAULT 'free' CHECK (plan IN ('free', 'lifetime')),
    source TEXT NOT NULL DEFAULT 'initial',
    granted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS account_portfolio_point_ledger (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES account_portfolio_accounts(user_id) ON DELETE RESTRICT,
    amount BIGINT NOT NULL CHECK (amount <> 0),
    reason TEXT NOT NULL,
    actor_user_id UUID,
    idempotency_key TEXT UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS account_portfolio_point_ledger_user_created_idx
    ON account_portfolio_point_ledger (user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS account_portfolio_membership_orders (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES account_portfolio_accounts(user_id) ON DELETE RESTRICT,
    plan TEXT NOT NULL CHECK (plan = 'lifetime'),
    amount_cents INTEGER NOT NULL CHECK (amount_cents = 990),
    status TEXT NOT NULL CHECK (status IN ('created', 'pending_payment', 'paid', 'closed', 'failed', 'refunded')),
    provider TEXT NOT NULL,
    provider_order_id TEXT UNIQUE,
    idempotency_key TEXT UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS account_portfolio_membership_orders_user_created_idx
    ON account_portfolio_membership_orders (user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS account_portfolio_notifications (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES account_portfolio_accounts(user_id) ON DELETE RESTRICT,
    title TEXT NOT NULL,
    body TEXT NOT NULL,
    kind TEXT NOT NULL,
    read_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS account_portfolio_notifications_user_created_idx
    ON account_portfolio_notifications (user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS account_portfolio_tickets (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES account_portfolio_accounts(user_id) ON DELETE RESTRICT,
    title TEXT NOT NULL,
    category TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('open', 'in_progress', 'resolved')),
    version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS account_portfolio_tickets_user_updated_idx
    ON account_portfolio_tickets (user_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS account_portfolio_ticket_messages (
    id UUID PRIMARY KEY,
    ticket_id UUID NOT NULL REFERENCES account_portfolio_tickets(id) ON DELETE RESTRICT,
    author_kind TEXT NOT NULL CHECK (author_kind IN ('user', 'operator')),
    body TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS account_portfolio_ticket_messages_ticket_created_idx
    ON account_portfolio_ticket_messages (ticket_id, created_at ASC);

CREATE TABLE IF NOT EXISTS account_portfolio_service_nonces (
    client_id TEXT NOT NULL,
    nonce TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (client_id, nonce)
);
CREATE INDEX IF NOT EXISTS account_portfolio_service_nonces_expires_idx
    ON account_portfolio_service_nonces (expires_at);
