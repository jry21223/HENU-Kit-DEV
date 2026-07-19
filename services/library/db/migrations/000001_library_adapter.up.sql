CREATE TABLE IF NOT EXISTS library_adapter_operations (
    id uuid PRIMARY KEY,
    client_id text NOT NULL,
    method text NOT NULL,
    normalized_route text NOT NULL,
    idempotency_key text NOT NULL,
    operation text NOT NULL,
    request_hash text NOT NULL,
    state text NOT NULL CHECK (state IN ('pending', 'succeeded', 'failed')),
    response jsonb,
    error_code text,
    actor_user_id uuid NOT NULL,
    request_id text NOT NULL,
    target_type text NOT NULL,
    target_id text,
    created_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    expires_at timestamptz NOT NULL DEFAULT (now() + interval '24 hours'),
    UNIQUE (client_id, actor_user_id, method, normalized_route, idempotency_key)
);

CREATE INDEX IF NOT EXISTS library_adapter_operations_expiry_idx
    ON library_adapter_operations (expires_at);

CREATE TABLE IF NOT EXISTS library_adapter_audit_events (
    id uuid PRIMARY KEY,
    operation_id uuid NOT NULL REFERENCES library_adapter_operations(id),
    actor_user_id uuid NOT NULL,
    request_id text NOT NULL,
    action text NOT NULL,
    target_type text NOT NULL,
    target_id text,
    outcome text NOT NULL CHECK (outcome IN ('attempted', 'succeeded', 'failed', 'unknown')),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE OR REPLACE FUNCTION library_adapter_append_only() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'library adapter audit is append-only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS library_adapter_audit_append_only ON library_adapter_audit_events;
CREATE TRIGGER library_adapter_audit_append_only
BEFORE UPDATE OR DELETE OR TRUNCATE ON library_adapter_audit_events
FOR EACH STATEMENT EXECUTE FUNCTION library_adapter_append_only();
