CREATE TABLE IF NOT EXISTS food_submissions (
    id uuid PRIMARY KEY,
    venue_name text NOT NULL CHECK (char_length(venue_name) BETWEEN 1 AND 160),
    item_name text NOT NULL CHECK (char_length(item_name) BETWEEN 1 AND 160),
    description text NOT NULL CHECK (char_length(description) <= 2000),
    status text NOT NULL CHECK (status IN ('pending', 'approved', 'rejected')),
    version integer NOT NULL DEFAULT 1 CHECK (version >= 1),
    submitted_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS food_anomaly_tickets (
    id uuid PRIMARY KEY,
    venue_name text NOT NULL CHECK (char_length(venue_name) BETWEEN 1 AND 160),
    kind text NOT NULL CHECK (kind IN ('duplicate', 'spam', 'quality', 'location')),
    details text NOT NULL CHECK (char_length(details) <= 2000),
    severity text NOT NULL CHECK (severity IN ('low', 'medium', 'high')),
    status text NOT NULL CHECK (status IN ('open', 'resolved', 'dismissed')),
    version integer NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS food_tier_adjustments (
    id uuid PRIMARY KEY,
    venue_name text NOT NULL CHECK (char_length(venue_name) BETWEEN 1 AND 160),
    current_tier text NOT NULL CHECK (current_tier IN ('featured', 'recommended', 'standard', 'watch')),
    proposed_tier text NOT NULL CHECK (proposed_tier IN ('featured', 'recommended', 'standard', 'watch')),
    reason text NOT NULL CHECK (char_length(reason) <= 2000),
    status text NOT NULL CHECK (status IN ('pending', 'confirmed', 'rejected')),
    version integer NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS food_operations (
    id uuid PRIMARY KEY,
    client_id text NOT NULL,
    actor_user_id uuid NOT NULL,
    method text NOT NULL,
    normalized_route text NOT NULL,
    idempotency_key text NOT NULL,
    operation text NOT NULL,
    request_hash text NOT NULL,
    request_id text NOT NULL,
    target_type text NOT NULL,
    target_id uuid NOT NULL,
    state text NOT NULL CHECK (state IN ('succeeded', 'failed')),
    response jsonb,
    error_code text,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (client_id, actor_user_id, method, normalized_route, idempotency_key)
);

CREATE TABLE IF NOT EXISTS food_audit_events (
    id uuid PRIMARY KEY,
    operation_id uuid NOT NULL REFERENCES food_operations(id),
    actor_user_id uuid NOT NULL,
    request_id text NOT NULL,
    action text NOT NULL,
    target_type text NOT NULL,
    target_id uuid NOT NULL,
    note text NOT NULL,
    outcome text NOT NULL CHECK (outcome IN ('succeeded', 'failed')),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE OR REPLACE FUNCTION food_audit_append_only() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'Food audit is append-only';
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS food_audit_append_only ON food_audit_events;
CREATE TRIGGER food_audit_append_only
BEFORE UPDATE OR DELETE OR TRUNCATE ON food_audit_events
FOR EACH STATEMENT EXECUTE FUNCTION food_audit_append_only();
