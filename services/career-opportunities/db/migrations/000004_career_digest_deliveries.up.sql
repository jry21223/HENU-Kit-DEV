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

-- Searches completed before this ledger existed cannot be replayed safely:
-- email_sent_at proves an accepted legacy enqueue, while a missing timestamp
-- has no durable evidence that mail was requested. Record both cases honestly
-- so old rows never appear to be permanently sending.
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
    CASE WHEN email_sent_at IS NOT NULL THEN 'sent' ELSE 'skipped' END,
    COALESCE(completed_at, created_at),
    email_sent_at,
    COALESCE(completed_at, created_at),
    now()
FROM career_searches
WHERE status = 'completed'
ON CONFLICT (search_id) DO NOTHING;

CREATE INDEX IF NOT EXISTS idx_career_digest_deliveries_retry
    ON career_digest_deliveries (attempted_at)
    WHERE status IN ('sending','retry');
