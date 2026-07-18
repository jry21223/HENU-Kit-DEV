INSERT INTO permission_codes (code, description) VALUES
    ('platform.operations_inbox.read', 'Read Operations Inbox items within granted Scope'),
    ('platform.operations_inbox.write', 'Create and update Operations Inbox items within granted Scope')
ON CONFLICT (code) DO NOTHING;

CREATE TABLE operations_inbox_items (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source_product_code text NOT NULL CHECK (source_product_code ~ '^[a-z][a-z0-9-]{1,63}$'),
    source_resource_type text NOT NULL CHECK (source_resource_type ~ '^[a-z][a-z0-9_-]{1,63}$'),
    source_resource_id text NOT NULL CHECK (length(source_resource_id) BETWEEN 1 AND 200),
    source_resource_url text CHECK (source_resource_url IS NULL OR length(source_resource_url) BETWEEN 1 AND 1000),
    owner_user_id uuid REFERENCES users(id) ON DELETE RESTRICT,
    priority text NOT NULL CHECK (priority IN ('low', 'normal', 'high', 'urgent')),
    sla_due_at timestamptz,
    status text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'in_progress', 'blocked', 'resolved', 'cancelled')),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    created_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    updated_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (source_product_code, source_resource_type, source_resource_id)
);

CREATE INDEX operations_inbox_product_status_idx
ON operations_inbox_items (source_product_code, status, updated_at DESC, id);

CREATE TABLE operations_inbox_idempotency (
    actor_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    operation text NOT NULL CHECK (operation IN ('create', 'update')),
    idempotency_key text NOT NULL CHECK (length(idempotency_key) BETWEEN 8 AND 200),
    request_hash bytea NOT NULL CHECK (octet_length(request_hash) = 32),
    item_id uuid NOT NULL REFERENCES operations_inbox_items(id) ON DELETE RESTRICT,
    response_version bigint NOT NULL CHECK (response_version > 0),
    response_payload jsonb NOT NULL CHECK (jsonb_typeof(response_payload) = 'object'),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (actor_user_id, operation, idempotency_key)
);

CREATE TABLE operations_inbox_audit_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    item_id uuid NOT NULL REFERENCES operations_inbox_items(id) ON DELETE RESTRICT,
    actor_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    request_id text NOT NULL CHECK (request_id ~ '^req_[A-Za-z0-9_-]+$'),
    action text NOT NULL CHECK (action IN ('created', 'updated')),
    from_version bigint CHECK (from_version IS NULL OR from_version > 0),
    to_version bigint NOT NULL CHECK (to_version > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT operations_inbox_audit_version_shape CHECK (
        (action = 'created' AND from_version IS NULL AND to_version = 1)
        OR (action = 'updated' AND from_version IS NOT NULL AND to_version = from_version + 1)
    )
);

CREATE INDEX operations_inbox_audit_item_created_idx
ON operations_inbox_audit_events (item_id, created_at, id);

CREATE FUNCTION reject_operations_inbox_audit_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'operations inbox audit events are append-only';
END;
$$;

CREATE TRIGGER operations_inbox_audit_events_immutable
BEFORE UPDATE OR DELETE OR TRUNCATE ON operations_inbox_audit_events
FOR EACH STATEMENT EXECUTE FUNCTION reject_operations_inbox_audit_mutation();
