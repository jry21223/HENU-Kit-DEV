ALTER TABLE account_portfolio_ticket_messages
    ADD COLUMN IF NOT EXISTS operator_user_id UUID;

ALTER TABLE account_portfolio_ticket_messages
    DROP CONSTRAINT IF EXISTS account_portfolio_ticket_messages_author_shape;
ALTER TABLE account_portfolio_ticket_messages
    ADD CONSTRAINT account_portfolio_ticket_messages_author_shape CHECK (
        (author_kind = 'user' AND operator_user_id IS NULL)
        OR
        (author_kind = 'operator' AND operator_user_id IS NOT NULL)
    );

CREATE TABLE IF NOT EXISTS account_portfolio_ticket_events (
    id UUID PRIMARY KEY,
    ticket_id UUID NOT NULL REFERENCES account_portfolio_tickets(id) ON DELETE RESTRICT,
    kind TEXT NOT NULL CHECK (kind IN ('operator_reply', 'status_transition', 'reopened')),
    actor_kind TEXT NOT NULL CHECK (actor_kind IN ('user', 'operator')),
    actor_user_id UUID NOT NULL,
    message_id UUID UNIQUE REFERENCES account_portfolio_ticket_messages(id) ON DELETE RESTRICT,
    from_status TEXT NOT NULL CHECK (from_status IN ('open', 'in_progress', 'resolved')),
    to_status TEXT NOT NULL CHECK (to_status IN ('open', 'in_progress', 'resolved')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT account_portfolio_ticket_events_kind_shape CHECK (
        (kind = 'operator_reply' AND actor_kind = 'operator' AND message_id IS NOT NULL)
        OR
        (kind = 'status_transition' AND actor_kind = 'operator' AND message_id IS NULL)
        OR
        (kind = 'reopened' AND actor_kind = 'user' AND message_id IS NULL)
    )
);
CREATE INDEX IF NOT EXISTS account_portfolio_ticket_events_ticket_created_idx
    ON account_portfolio_ticket_events (ticket_id, created_at ASC, id ASC);

ALTER TABLE account_portfolio_notifications
    ADD COLUMN IF NOT EXISTS ticket_id UUID REFERENCES account_portfolio_tickets(id) ON DELETE RESTRICT,
    ADD COLUMN IF NOT EXISTS ticket_event_id UUID REFERENCES account_portfolio_ticket_events(id) ON DELETE RESTRICT;
CREATE UNIQUE INDEX IF NOT EXISTS account_portfolio_notifications_ticket_event_unique_idx
    ON account_portfolio_notifications (ticket_event_id)
    WHERE ticket_event_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS account_portfolio_notifications_ticket_created_idx
    ON account_portfolio_notifications (ticket_id, created_at DESC)
    WHERE ticket_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS account_portfolio_command_idempotency (
    client_id TEXT NOT NULL CHECK (length(client_id) BETWEEN 1 AND 120),
    actor_user_id UUID NOT NULL,
    operation TEXT NOT NULL CHECK (operation ~ '^[a-z][a-z0-9_]{1,79}$'),
    idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 8 AND 200),
    request_hash BYTEA NOT NULL CHECK (octet_length(request_hash) = 32),
    response_status SMALLINT NOT NULL CHECK (response_status BETWEEN 200 AND 299),
    response_payload JSONB NOT NULL CHECK (jsonb_typeof(response_payload) = 'object'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (client_id, actor_user_id, operation, idempotency_key)
);
