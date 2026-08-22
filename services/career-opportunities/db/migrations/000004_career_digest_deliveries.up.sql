-- Durable retry ledger for one Platform Core digest enqueue per completed
-- Career search. Search completion never depends on mail availability; the
-- worker can safely replay with Platform Core's search-scoped idempotency key.

CREATE TABLE IF NOT EXISTS career_digest_deliveries (
    search_id uuid PRIMARY KEY REFERENCES career_searches(id) ON DELETE CASCADE,
    status text NOT NULL CHECK (status IN ('sending','retry','sent','skipped')),
    attempted_at timestamptz NOT NULL DEFAULT now(),
    enqueued_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- For searches completed before this ledger existed, email_sent_at is the
-- only durable evidence that a digest was requested and accepted. Preserve
-- those accepted enqueues as sent. Leave NULL legacy rows without a ledger so
-- the UI reports an unknown historical status instead of inventing intent or
-- flooding one user with old searches after deployment.
INSERT INTO career_digest_deliveries (
    search_id,
    status,
    attempted_at,
    enqueued_at,
    created_at,
    updated_at
)
SELECT
    id,
    'sent',
    COALESCE(completed_at, created_at),
    email_sent_at,
    COALESCE(completed_at, created_at),
    now()
FROM career_searches
WHERE status = 'completed' AND email_sent_at IS NOT NULL
ON CONFLICT (search_id) DO NOTHING;

CREATE INDEX IF NOT EXISTS idx_career_digest_deliveries_retry
    ON career_digest_deliveries (attempted_at)
    WHERE status IN ('sending','retry');
