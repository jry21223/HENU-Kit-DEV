INSERT INTO permission_codes (code, description) VALUES
    ('platform.operations.read', 'Read bounded Platform Operations data within platform Scope'),
    ('platform.operations.write', 'Manage Platform Operations state within platform Scope')
ON CONFLICT (code) DO UPDATE SET description = EXCLUDED.description, status = 'active';

CREATE TABLE IF NOT EXISTS platform_operations_idempotency (
    service_id text NOT NULL REFERENCES oauth_clients(id) ON DELETE RESTRICT,
    actor_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    operation text NOT NULL CHECK (operation IN ('session_revoke', 'access_update')),
    idempotency_key text NOT NULL CHECK (length(idempotency_key) BETWEEN 8 AND 200),
    request_hash bytea NOT NULL CHECK (octet_length(request_hash) = 32),
    response_payload jsonb NOT NULL CHECK (jsonb_typeof(response_payload) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (service_id, actor_user_id, operation, idempotency_key)
);

CREATE TABLE IF NOT EXISTS platform_operations_audit_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    request_id text NOT NULL CHECK (request_id ~ '^req_[A-Za-z0-9_-]+$'),
    operation text NOT NULL CHECK (operation IN ('session_revoke', 'access_update')),
    resource_kind text NOT NULL CHECK (resource_kind IN ('session', 'user')),
    resource_id uuid NOT NULL,
    result_payload jsonb NOT NULL CHECK (jsonb_typeof(result_payload) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS platform_operations_audit_resource_idx
ON platform_operations_audit_events (resource_kind, resource_id, created_at DESC);

CREATE OR REPLACE FUNCTION reject_platform_operations_audit_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'platform operations audit events are append-only';
END;
$$;

DROP TRIGGER IF EXISTS platform_operations_audit_events_immutable ON platform_operations_audit_events;
CREATE TRIGGER platform_operations_audit_events_immutable
BEFORE UPDATE OR DELETE OR TRUNCATE ON platform_operations_audit_events
FOR EACH STATEMENT EXECUTE FUNCTION reject_platform_operations_audit_mutation();
