ALTER TABLE account_portfolio_memberships
    ADD COLUMN IF NOT EXISTS version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE account_portfolio_memberships
    DROP CONSTRAINT IF EXISTS account_portfolio_memberships_version_positive;
ALTER TABLE account_portfolio_memberships
    ADD CONSTRAINT account_portfolio_memberships_version_positive CHECK (version > 0);

CREATE TABLE IF NOT EXISTS account_portfolio_membership_events (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES account_portfolio_accounts(user_id) ON DELETE RESTRICT,
    kind TEXT NOT NULL CHECK (kind IN ('grant', 'revoke')),
    from_plan TEXT NOT NULL CHECK (from_plan IN ('free', 'lifetime')),
    to_plan TEXT NOT NULL CHECK (to_plan IN ('free', 'lifetime')),
    source TEXT NOT NULL CHECK (source = 'operator'),
    actor_user_id UUID NOT NULL,
    reason TEXT NOT NULL CHECK (length(reason) BETWEEN 1 AND 1000),
    idempotency_key TEXT NOT NULL CHECK (length(idempotency_key) BETWEEN 8 AND 200),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT account_portfolio_membership_events_transition_shape CHECK (
        (kind = 'grant' AND from_plan = 'free' AND to_plan = 'lifetime')
        OR
        (kind = 'revoke' AND from_plan = 'lifetime' AND to_plan = 'free')
    )
);
CREATE INDEX IF NOT EXISTS account_portfolio_membership_events_user_created_idx
    ON account_portfolio_membership_events (user_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS account_portfolio_membership_events_actor_created_idx
    ON account_portfolio_membership_events (actor_user_id, created_at DESC, id DESC);

ALTER TABLE account_portfolio_notifications
    ADD COLUMN IF NOT EXISTS membership_event_id UUID REFERENCES account_portfolio_membership_events(id) ON DELETE RESTRICT;
CREATE UNIQUE INDEX IF NOT EXISTS account_portfolio_notifications_membership_event_unique_idx
    ON account_portfolio_notifications (membership_event_id)
    WHERE membership_event_id IS NOT NULL;
