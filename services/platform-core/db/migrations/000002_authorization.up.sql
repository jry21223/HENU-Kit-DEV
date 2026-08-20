ALTER TABLE users
ADD COLUMN IF NOT EXISTS authorization_revision bigint NOT NULL DEFAULT 1 CHECK (authorization_revision > 0);

CREATE TABLE permission_codes (
    code text PRIMARY KEY CHECK (code ~ '^[a-z][a-z0-9]*(\.[a-z][a-z0-9_-]*)+$'),
    description text NOT NULL,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'retired')),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE authorization_roles (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code text NOT NULL UNIQUE CHECK (code ~ '^[a-z][a-z0-9-]{1,63}$'),
    display_name text NOT NULL CHECK (length(display_name) BETWEEN 1 AND 120),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'revoked')),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE role_permissions (
    role_id uuid NOT NULL REFERENCES authorization_roles(id) ON DELETE CASCADE,
    permission_code text NOT NULL REFERENCES permission_codes(code) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (role_id, permission_code)
);

CREATE TABLE user_role_grants (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id uuid NOT NULL REFERENCES authorization_roles(id) ON DELETE RESTRICT,
    scope_kind text NOT NULL CHECK (scope_kind IN ('platform', 'product', 'resource')),
    product_code text,
    resource_type text,
    resource_id text,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'revoked')),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT user_role_grants_scope_shape CHECK (
        (scope_kind = 'platform' AND product_code IS NULL AND resource_type IS NULL AND resource_id IS NULL)
        OR
        (scope_kind = 'product' AND product_code IS NOT NULL AND resource_type IS NULL AND resource_id IS NULL)
        OR
        (scope_kind = 'resource' AND product_code IS NOT NULL AND resource_type IS NOT NULL AND resource_id IS NOT NULL)
    ),
    CONSTRAINT user_role_grants_revocation_shape CHECK (
        (status = 'active' AND revoked_at IS NULL)
        OR (status = 'revoked' AND revoked_at IS NOT NULL)
    )
);

CREATE UNIQUE INDEX user_role_grants_one_active_scope_idx
ON user_role_grants (
    user_id, role_id, scope_kind,
    COALESCE(product_code, ''), COALESCE(resource_type, ''), COALESCE(resource_id, '')
)
WHERE status = 'active';

CREATE TABLE authorization_audit_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    session_id uuid NOT NULL REFERENCES sessions(id) ON DELETE RESTRICT,
    request_id text NOT NULL CHECK (request_id ~ '^req_[A-Za-z0-9_-]+$'),
    service_id text NOT NULL REFERENCES oauth_clients(id) ON DELETE RESTRICT,
    permission_code text NOT NULL,
    target_kind text NOT NULL CHECK (target_kind IN ('platform', 'product', 'resource')),
    target_product_code text,
    target_resource_type text,
    target_resource_id text,
    decision text NOT NULL CHECK (decision IN ('allowed', 'denied')),
    reason_code text NOT NULL CHECK (reason_code ~ '^[A-Z0-9_]+$'),
    grant_id uuid REFERENCES user_role_grants(id) ON DELETE RESTRICT,
    authorization_revision bigint CHECK (authorization_revision > 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT authorization_audit_target_shape CHECK (
        (target_kind = 'platform' AND target_product_code IS NULL AND target_resource_type IS NULL AND target_resource_id IS NULL)
        OR
        (target_kind = 'product' AND target_product_code IS NOT NULL AND target_resource_type IS NULL AND target_resource_id IS NULL)
        OR
        (target_kind = 'resource' AND target_product_code IS NOT NULL AND target_resource_type IS NOT NULL AND target_resource_id IS NOT NULL)
    ),
    CONSTRAINT authorization_audit_decision_shape CHECK (
        (decision = 'allowed' AND reason_code = 'GRANTED' AND grant_id IS NOT NULL AND authorization_revision IS NOT NULL)
        OR
        (decision = 'denied' AND grant_id IS NULL)
    )
);

CREATE INDEX authorization_audit_actor_created_idx ON authorization_audit_events (actor_user_id, created_at DESC);
CREATE INDEX authorization_audit_request_idx ON authorization_audit_events (request_id);
