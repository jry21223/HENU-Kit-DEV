-- Career Opportunities: async Work Radar search jobs (issue #392).
--
-- A search is created queued and immediately returned to the actor; a
-- background worker claims it, advances queued -> running -> completed, or
-- lands on failed with a stable browser-safe error code. The profile is
-- snapshotted at creation and never re-read, so later profile edits cannot
-- change an already-created task. No `lifetime` flag is stored here: Career
-- never becomes a second membership truth source.

CREATE TABLE IF NOT EXISTS career_searches (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL,
    status text NOT NULL CHECK (status IN ('queued','running','completed','failed')),
    stage text CHECK (stage IN ('crawling','matching','rendering')),
    profile_snapshot jsonb NOT NULL,
    started_at timestamptz,
    completed_at timestamptz,
    failed_at timestamptz,
    error_code text,
    error_message text,
    email_sent_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_career_searches_user_created ON career_searches (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_career_searches_queued ON career_searches (status) WHERE status = 'queued';

CREATE TABLE IF NOT EXISTS career_search_results (
    search_id uuid PRIMARY KEY REFERENCES career_searches(id) ON DELETE CASCADE,
    payload jsonb NOT NULL,
    source_count integer NOT NULL DEFAULT 0 CHECK (source_count >= 0),
    job_count integer NOT NULL DEFAULT 0 CHECK (job_count >= 0),
    matched_count integer NOT NULL DEFAULT 0 CHECK (matched_count >= 0),
    summary text NOT NULL DEFAULT '' CHECK (char_length(summary) <= 4000),
    created_at timestamptz NOT NULL DEFAULT now()
);

-- One durable, actor-scoped idempotency ledger per create call. Replaying the
-- same Idempotency-Key returns the original search without a second row.
CREATE TABLE IF NOT EXISTS career_search_operations (
    id uuid PRIMARY KEY,
    client_id text NOT NULL,
    actor_user_id uuid NOT NULL,
    idempotency_key text NOT NULL,
    request_hash text NOT NULL,
    request_id text NOT NULL,
    search_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (client_id, actor_user_id, idempotency_key)
);
