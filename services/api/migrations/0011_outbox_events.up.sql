CREATE TABLE IF NOT EXISTS outbox_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz,
    aggregate_type varchar(80) NOT NULL, aggregate_id varchar(160) NOT NULL, event_type varchar(160) NOT NULL,
    payload jsonb NOT NULL, status varchar(32) NOT NULL DEFAULT 'pending', available_at timestamptz NOT NULL DEFAULT now(), published_at timestamptz,
    locked_at timestamptz, attempt_count integer NOT NULL DEFAULT 0, last_error_code varchar(120) NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_outbox_events_pending ON outbox_events(status, available_at);

