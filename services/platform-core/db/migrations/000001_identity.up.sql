CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email_verified boolean NOT NULL DEFAULT false,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'deleted')),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE oauth_clients (
    id text PRIMARY KEY,
    secret_hash bytea NOT NULL CHECK (octet_length(secret_hash) = 32),
    redirect_uris text[] NOT NULL CHECK (cardinality(redirect_uris) > 0),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('core', 'client_exchange')),
    token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
    client_id text REFERENCES oauth_clients(id) ON DELETE CASCADE,
    parent_session_id uuid REFERENCES sessions(id) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT sessions_kind_fields CHECK (
        (kind = 'core' AND client_id IS NULL AND parent_session_id IS NULL)
        OR
        (kind = 'client_exchange' AND client_id IS NOT NULL AND parent_session_id IS NOT NULL)
    )
);

CREATE INDEX sessions_active_user_idx ON sessions (user_id, expires_at) WHERE revoked_at IS NULL;

CREATE TABLE authorization_codes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code_hash bytea NOT NULL UNIQUE CHECK (octet_length(code_hash) = 32),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    client_id text NOT NULL REFERENCES oauth_clients(id) ON DELETE CASCADE,
    core_session_id uuid NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    redirect_uri text NOT NULL,
    code_challenge text NOT NULL,
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX authorization_codes_expiry_idx ON authorization_codes (expires_at) WHERE used_at IS NULL;
